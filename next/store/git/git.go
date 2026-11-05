// Package git is the git datastore (docs/spec/storage.md): an entity is
// a chain of commits under refs/board/<kind>/<id>, each holding one pack.
//
// It reads refs itself, the loose ones first, then packed-refs, as git
// does, so a listing keeps every ref git moves or packs meanwhile. It
// reads objects through `git cat-file --batch-command`, one process per
// CPU for a bulk load, walking each entity level by level. It writes
// objects in process, as loose objects, and moves refs with `git
// update-ref`, the compare-and-swap (ADR 0004, ADR 0006).
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/next/kernel/pack"
	"github.com/guilbep/git-board/next/store"
)

// Store is a git repository's board.
type Store struct {
	gitDir    string // this worktree's
	commonDir string // every worktree's: refs, objects, the board's folder
	warm      sync.Once
	warmed    chan warmed // a cat-file Warm started, for the next load
}

var _ store.Datastore = (*Store)(nil)

// Open opens the board of the repository at dir, or above it, as git
// finds it. A linked worktree reads its main repository's board.
func Open(ctx context.Context, dir string) (*Store, error) {
	if s, ok := find(dir); ok {
		return s, nil
	}
	cmd := exec.CommandContext(ctx, "git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-dir", "--git-common-dir")
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s: not a git repository: %s", dir, strings.TrimSpace(stderr.String()))
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(lines) != 2 {
		return nil, fmt.Errorf("%s: git rev-parse said %q", dir, out)
	}
	return &Store{gitDir: lines[0], commonDir: lines[1], warmed: make(chan warmed, 1)}, nil
}

// find is the repository holding dir, found as git finds it in the
// common case, without starting git: a .git directory, or a .git file
// naming a worktree's, up from dir. Anything else (GIT_DIR and its
// kind, a bare repository) is git rev-parse's to answer.
func find(dir string) (*Store, bool) {
	for _, env := range []string{"GIT_DIR", "GIT_COMMON_DIR", "GIT_WORK_TREE", "GIT_CEILING_DIRECTORIES", "GIT_DISCOVERY_ACROSS_FILESYSTEM"} {
		if _, set := os.LookupEnv(env); set {
			return nil, false
		}
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, false
	}
	for {
		dot := filepath.Join(dir, ".git")
		fi, err := os.Stat(dot)
		switch {
		case err == nil && fi.IsDir():
			return gitDirAt(dot)
		case err == nil:
			b, err := os.ReadFile(dot)
			gd, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: ")
			if err != nil || !ok {
				return nil, false
			}
			if !filepath.IsAbs(gd) {
				gd = filepath.Join(dir, gd)
			}
			return gitDirAt(filepath.Clean(gd))
		case !errors.Is(err, fs.ErrNotExist):
			return nil, false
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return nil, false
		}
		dir = parent
	}
}

// gitDirAt is the store of a git directory: its common one is itself, or
// what its commondir file names (a linked worktree's).
func gitDirAt(gitDir string) (*Store, bool) {
	if _, err := os.Stat(filepath.Join(gitDir, "HEAD")); err != nil {
		return nil, false
	}
	common := gitDir
	if b, err := os.ReadFile(filepath.Join(gitDir, "commondir")); err == nil {
		common = strings.TrimSpace(string(b))
		if !filepath.IsAbs(common) {
			common = filepath.Join(gitDir, common)
		}
		common = filepath.Clean(common)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, false
	}
	return &Store{gitDir: gitDir, commonDir: common, warmed: make(chan warmed, 1)}, true
}

// Caps: a version is a commit, which names its packs for good; the
// counter is refs/board/seq.
func (s *Store) Caps() store.Caps { return store.Caps{Pinned: true, Counter: s} }

// Folder is <common dir>/board, shared by every worktree.
func (s *Store) Folder() string { return filepath.Join(s.commonDir, "board") }

// Close ends a walker Warm started and no load took.
func (s *Store) Close() error {
	select {
	case w := <-s.warmed:
		if w.err == nil {
			return w.w.close()
		}
	default:
	}
	return nil
}

// GitDir is this worktree's git directory: its config.
func (s *Store) GitDir() string { return s.gitDir }

func refName(kind, id string) string { return "refs/board/" + kind + "/" + id }

// Heads lists the kinds' entities. The loose refs are read before
// packed-refs: `git pack-refs` writes packed-refs before it removes them,
// so a ref packed meanwhile is in one or the other. A directory is read
// without a stat of each entry: a <ref>.lock renamed over its ref between
// the two would fail it (fbda0b3, which once listed no issue at all).
func (s *Store) Heads(ctx context.Context, kinds ...string) ([]store.Head, error) {
	kinds = slices.Sorted(slices.Values(kinds))
	loose := make([]map[string]string, len(kinds))
	for i, kind := range kinds {
		var err error
		if loose[i], err = s.loose(kind); err != nil {
			return nil, err
		}
	}
	data, err := os.ReadFile(filepath.Join(s.commonDir, "packed-refs"))
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	packed := string(data) // every id and version below is a piece of it
	sortedRefs := strings.HasPrefix(packed, "# pack-refs with:") && strings.Contains(packed[:strings.IndexByte(packed+"\n", '\n')], " sorted")
	out := make([]store.Head, 0, len(packed)/82+8)
	for i, kind := range kinds {
		prefix := "refs/board/" + kind + "/"
		from := 0
		if sortedRefs {
			from = seek(packed, prefix)
		}
		start := len(out)
		for line := range lines(packed[from:]) {
			// "<hash> refs/board/<kind>/<id>"; a header or a peeled line doesn't parse
			if len(line) < 41 || line[40] != ' ' {
				continue
			}
			name := line[41:]
			if !strings.HasPrefix(name, prefix) {
				if sortedRefs && name > prefix {
					break // past the kind's refs
				}
				continue
			}
			id, v := name[len(prefix):], line[:40]
			if _, there := loose[i][id]; there || !store.IsID(id) || !isHash(v) {
				continue // a loose ref wins over its packed copy
			}
			out = append(out, store.Head{Kind: kind, ID: id, Version: v})
		}
		for id, v := range loose[i] {
			out = append(out, store.Head{Kind: kind, ID: id, Version: v})
		}
		if len(loose[i]) > 0 || !sortedRefs {
			slices.SortFunc(out[start:], func(a, b store.Head) int { return strings.Compare(a.ID, b.ID) })
		}
	}
	return out, nil
}

// loose are a kind's loose refs, by id.
func (s *Store) loose(kind string) (map[string]string, error) {
	dir := filepath.Join(s.commonDir, "refs", "board", kind)
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for _, e := range entries {
		id := e.Name()
		if !store.IsID(id) || e.IsDir() {
			continue // a .lock, a name that isn't an id (storage.md, Refs)
		}
		b, err := os.ReadFile(filepath.Join(dir, id))
		if errors.Is(err, fs.ErrNotExist) { // deleted, or packed: then in packed-refs
			continue
		}
		if err != nil {
			return nil, err
		}
		if h := string(bytes.TrimSpace(b)); isHash(h) { // not a symbolic ref, nor one being written
			out[id] = h
		}
	}
	return out, nil
}

// lines are s's lines, without their line feeds.
func lines(s string) func(func(string) bool) {
	return func(yield func(string) bool) {
		for len(s) > 0 {
			i := strings.IndexByte(s, '\n')
			if i < 0 {
				yield(s)
				return
			}
			if !yield(s[:i]) {
				return
			}
			s = s[i+1:]
		}
	}
}

// seek is where, in a sorted packed-refs, the refs from prefix on start:
// a binary search over its lines, as git reads it. Peeled lines (^) and
// the header sort before any name, so a line it lands on is skipped to
// the next whole record.
func seek(packed, prefix string) int {
	lo, hi := 0, len(packed)
	for lo < hi {
		mid := lo + (hi-lo)/2
		start := strings.LastIndexByte(packed[:mid], '\n') + 1
		end := strings.IndexByte(packed[start:], '\n')
		if end < 0 {
			end = len(packed) - start
		}
		line, next := packed[start:start+end], start+end+1
		if len(line) < 41 || line[40] != ' ' || line[41:] < prefix {
			lo = next
		} else {
			hi = start
		}
	}
	return min(lo, len(packed))
}

// isHash says h is a SHA-1 in lowercase hex: a ref's value, a version.
func isHash(h string) bool { return store.IsID(h) }

// Add writes pack in a commit on prev, then moves the ref there if it is
// still at prev. A pack whose clock isn't 1 + prev's (1 with no prev) is
// refused: a reader would refuse the entity after it.
func (s *Store) Add(ctx context.Context, kind, id string, data []byte, prev string) (string, error) {
	if !store.IsID(id) || kind == "" || strings.ContainsAny(kind, "/.") {
		return "", fmt.Errorf("git: bad name %s", refName(kind, id))
	}
	clock, empty, err := pack.Header(data)
	if err != nil {
		return "", fmt.Errorf("git: %s: pack: %w", refName(kind, id), err)
	}
	if empty {
		return "", fmt.Errorf("git: %s: %w", refName(kind, id), pack.ErrEmpty)
	}
	want := uint64(1)
	var parents []string
	if prev != "" {
		if !isHash(prev) {
			return "", fmt.Errorf("git: %s: bad version %q", refName(kind, id), prev)
		}
		head, err := s.node(ctx, prev)
		if err != nil {
			return "", fmt.Errorf("git: %s: %w", refName(kind, id), err)
		}
		want, parents = head.clock+1, []string{prev}
	}
	if clock != want {
		return "", fmt.Errorf("git: %s: a pack of clock %d on one of clock %d", refName(kind, id), clock, want-1)
	}
	commit, err := s.writePack(data, parents)
	if err != nil {
		return "", err
	}
	old := prev
	if old == "" {
		old = zero
	}
	if err := s.updateRef(ctx, refName(kind, id), commit, old); err != nil {
		return "", err
	}
	return commit, nil
}

const zero = "0000000000000000000000000000000000000000"

// node reads one commit and its pack, not its parents: Add's check of
// prev.
func (s *Store) node(ctx context.Context, commit string) (*node, error) {
	w, err := s.startWalker(ctx)
	if err != nil {
		return nil, err
	}
	defer w.close()
	e := &entity{ref: store.Ref{Version: commit}, seen: map[string]bool{}}
	todo := []request{{e: e, kind: reqCommit, oid: commit}}
	for len(todo) > 0 && e.err == nil {
		rq := todo[0]
		if err := w.send(todo[:1]); err != nil {
			return nil, err
		}
		oid, typ, body, err := w.read()
		if err != nil {
			return nil, err
		}
		todo = w.answer(todo[1:], rq, oid, typ, body)
		if rq.kind == reqCommit { // its parents stay unread
			todo = slices.DeleteFunc(todo, func(r request) bool { return r.kind == reqCommit })
		}
	}
	if e.err != nil {
		return nil, e.err
	}
	return e.nodes[0], nil
}

// writePack writes data as a blob, in a tree as "pack", in a commit on
// parents with an empty identity and no message, as v1 writes them
// (storage.md, Commits).
func (s *Store) writePack(data []byte, parents []string) (string, error) {
	blob, err := s.writeObject("blob", data)
	if err != nil {
		return "", err
	}
	raw, _ := hexDecode(blob)
	tree, err := s.writeObject("tree", append([]byte("100644 pack\x00"), raw...))
	if err != nil {
		return "", err
	}
	return s.writeCommit(tree, parents)
}

// writeCommit writes a commit of tree on parents, with an empty identity
// and no message, as v1 writes the board's.
func (s *Store) writeCommit(tree string, parents []string) (string, error) {
	now := time.Now()
	when := fmt.Sprintf("%d %s", now.Unix(), now.Format("-0700"))
	var c bytes.Buffer
	fmt.Fprintf(&c, "tree %s\n", tree)
	for _, p := range parents {
		fmt.Fprintf(&c, "parent %s\n", p)
	}
	fmt.Fprintf(&c, "author  <> %s\ncommitter  <> %s\n\n", when, when)
	return s.writeObject("commit", c.Bytes())
}

// updateRef moves name to next if it is still at prev (zero: absent).
// git's refusal, a moved ref, an existing one, a lock held, is ErrMoved.
func (s *Store) updateRef(ctx context.Context, name, next, prev string) error {
	_, err := s.git(ctx, nil, "update-ref", name, next, prev)
	if err == nil {
		return nil
	}
	for _, m := range []string{"but expected", "reference already exists", ".lock': File exists", "unable to resolve reference"} {
		if strings.Contains(err.Error(), m) {
			return fmt.Errorf("%w: %s", store.ErrMoved, err)
		}
	}
	return err
}

// git runs git on the repository, in the C locale: updateRef reads its
// refusals by their English text. A cancelled git gets SIGINT, on which it
// removes its lock files.
func (s *Store) git(ctx context.Context, stdin []byte, args ...string) ([]byte, error) {
	cmd := s.command(ctx, args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func (s *Store) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--git-dir", s.gitDir}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	cmd.Cancel = func() error { return interrupt(cmd.Process) }
	cmd.WaitDelay = 5 * time.Second
	return cmd
}
