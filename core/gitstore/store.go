// Package gitstore reads and writes git objects and refs for the core.
package gitstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/filemode"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/storage/filesystem"
)

// Hash is a git object id.
type Hash = plumbing.Hash

// ZeroHash is the hash of no object. It stands for an absent ref.
var ZeroHash = plumbing.ZeroHash

// ErrRefMoved is returned when a ref is not where the write read it.
var ErrRefMoved = errors.New("gitstore: ref moved since it was read")

// Store is a git repository.
type Store struct {
	// ponytail: one lock serializes go-git access per Store; per-object locking if it ever shows in a profile
	mu     sync.Mutex
	repo   *git.Repository
	fs     *filesystem.Storage
	gitDir string
	// commonDir is the absolute git common dir.
	commonDir string
}

// TreeEntry is a blob in a tree, with mode 100644.
type TreeEntry struct {
	Name string
	Blob Hash
}

// Commit is a commit's tree and parents. Author, committer and message are
// the entity commit's fixed ones.
type Commit struct {
	Tree    Hash
	Parents []Hash
}

// Open opens the repository at path. The path is tried as a repository root
// first, a bare one included, so a bare repository inside another's worktree
// opens as itself; only then is a worktree found upward from it. A linked
// worktree reads and writes its main repository's refs and objects, as git
// does.
func Open(path string) (*Store, error) {
	opts := git.PlainOpenOptions{EnableDotGitCommonDir: true}
	repo, err := git.PlainOpenWithOptions(path, &opts)
	if errors.Is(err, git.ErrRepositoryNotExists) {
		opts.DetectDotGit = true
		repo, err = git.PlainOpenWithOptions(path, &opts)
	}
	if err != nil {
		return nil, fmt.Errorf("gitstore: open %s: %w", path, err)
	}
	fs, ok := repo.Storer.(*filesystem.Storage)
	if !ok {
		return nil, fmt.Errorf("gitstore: open %s: not a filesystem repository", path)
	}
	s := &Store{repo: repo, fs: fs, gitDir: fs.Filesystem().Root()}
	cmd := s.command(context.Background(), "rev-parse", "--git-common-dir")
	cmd.Dir = s.gitDir // a relative answer is relative to this
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("gitstore: open %s: common dir: %w: %s", path, err, strings.TrimSpace(stderr.String()))
	}
	s.commonDir = strings.TrimSpace(string(out))
	if !filepath.IsAbs(s.commonDir) {
		s.commonDir = filepath.Join(s.gitDir, s.commonDir)
	}
	return s, nil
}

func (s *Store) put(o plumbing.EncodedObject) (Hash, error) {
	return s.repo.Storer.SetEncodedObject(o)
}

// WriteBlob stores data as a blob.
func (s *Store) WriteBlob(data []byte) (Hash, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	o := s.repo.Storer.NewEncodedObject()
	o.SetType(plumbing.BlobObject)
	w, err := o.Writer()
	if err != nil {
		return ZeroHash, err
	}
	if _, err := w.Write(data); err != nil {
		return ZeroHash, err
	}
	if err := w.Close(); err != nil {
		return ZeroHash, err
	}
	return s.put(o)
}

// ReadBlob returns a blob's bytes.
func (s *Store) ReadBlob(h Hash) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, err := retry(s, func() (*object.Blob, error) { return s.repo.BlobObject(h) })
	if err != nil {
		return nil, fmt.Errorf("gitstore: blob %s: %w", h, err)
	}
	r, err := b.Reader()
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// WriteTree stores a tree of blobs. Git wants the entries sorted by name.
func (s *Store) WriteTree(entries []TreeEntry) (Hash, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t := object.Tree{}
	for _, e := range entries {
		t.Entries = append(t.Entries, object.TreeEntry{Name: e.Name, Mode: filemode.Regular, Hash: e.Blob})
	}
	// ponytail: sorted by plain name; right for blobs, not for subtrees.
	sort.Slice(t.Entries, func(i, j int) bool { return t.Entries[i].Name < t.Entries[j].Name })
	o := s.repo.Storer.NewEncodedObject()
	if err := t.Encode(o); err != nil {
		return ZeroHash, err
	}
	return s.put(o)
}

// ReadTree returns a tree's entries.
func (s *Store) ReadTree(h Hash) ([]TreeEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, err := retry(s, func() (*object.Tree, error) { return s.repo.TreeObject(h) })
	if err != nil {
		return nil, fmt.Errorf("gitstore: tree %s: %w", h, err)
	}
	var out []TreeEntry
	for _, e := range t.Entries {
		out = append(out, TreeEntry{Name: e.Name, Blob: e.Hash})
	}
	return out, nil
}

// WriteCommit stores an entity commit.
func (s *Store) WriteCommit(c Commit) (Hash, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sig := object.Signature{Name: "git-board", Email: "git-board@localhost", When: time.Now()}
	obj := object.Commit{Author: sig, Committer: sig, Message: "git-board", TreeHash: c.Tree, ParentHashes: c.Parents}
	o := s.repo.Storer.NewEncodedObject()
	if err := obj.Encode(o); err != nil {
		return ZeroHash, err
	}
	return s.put(o)
}

// ReadCommit returns a commit's tree and parents.
func (s *Store) ReadCommit(h Hash) (Commit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, err := retry(s, func() (*object.Commit, error) { return s.repo.CommitObject(h) })
	if err != nil {
		return Commit{}, fmt.Errorf("gitstore: commit %s: %w", h, err)
	}
	return Commit{Tree: c.TreeHash, Parents: c.ParentHashes}, nil
}

// retry reads again (s.mu held) after re-reading the pack index when an object is not
// found: go-git reads the index once, so a pack written later by git (a
// fetch, a gc) stays hidden until then.
func retry[T any](s *Store, read func() (T, error)) (T, error) {
	v, err := read()
	if errors.Is(err, plumbing.ErrObjectNotFound) {
		s.fs.Reindex()
		return read()
	}
	return v, err
}

// Ref returns where a ref points, or ZeroHash when it is absent.
func (s *Store) Ref(name string) (Hash, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, err := s.repo.Reference(plumbing.ReferenceName(name), false)
	if errors.Is(err, plumbing.ErrReferenceNotFound) {
		return ZeroHash, nil
	}
	if err != nil {
		return ZeroHash, err
	}
	return r.Hash(), nil
}

// Refs returns the refs whose names start with prefix, loose or packed. The
// prefix starts with refs/. A lock file isn't a ref, and an empty ref file is
// one being written: both are left out, where go-git's own listing fails.
func (s *Store) Refs(prefix string) (map[string]Hash, error) {
	dir := path.Dir(prefix + "x")
	if dir != "refs" && !strings.HasPrefix(dir, "refs/") {
		return nil, fmt.Errorf("gitstore: refs prefix %q is not under refs/", prefix)
	}
	// The filesystem sends refs/ and packed-refs to the common dir, as git does.
	fs := s.fs.Filesystem()
	out := map[string]Hash{}
	read := func(name string) ([]byte, error) {
		f, err := fs.Open(name)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		return io.ReadAll(f)
	}
	packed, err := read("packed-refs")
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, line := range strings.Split(string(packed), "\n") {
		// Header and peeled lines ("# ...", "^<hash>") don't parse.
		if h, name, _ := strings.Cut(line, " "); plumbing.IsHash(h) && strings.HasPrefix(name, prefix) {
			out[name] = plumbing.NewHash(h)
		}
	}
	var walk func(dir string) error
	walk = func(dir string) error {
		entries, err := fs.ReadDir(dir)
		if os.IsNotExist(err) { // never made, or just removed
			return nil
		}
		if err != nil {
			return err
		}
		for _, e := range entries {
			name := dir + "/" + e.Name()
			if e.IsDir() {
				if err := walk(name); err != nil {
					return err
				}
				continue
			}
			if strings.HasSuffix(name, ".lock") || !strings.HasPrefix(name, prefix) {
				continue
			}
			b, err := read(name)
			if os.IsNotExist(err) {
				continue
			}
			if err != nil {
				return err
			}
			// A symbolic ref ("ref: ...") or an empty file isn't listed.
			if h := strings.TrimSpace(string(b)); plumbing.IsHash(h) {
				out[name] = plumbing.NewHash(h) // a loose ref wins over its packed copy
			}
		}
		return nil
	}
	if err := walk(dir); err != nil {
		return nil, err
	}
	return out, nil
}

// UpdateRef moves a ref to next if it still points at prev. A prev of
// ZeroHash means the ref must not exist. It is ErrRefMoved when the ref moved,
// exists, no longer exists, or another process holds its lock file.
func (s *Store) UpdateRef(ctx context.Context, name string, next, prev Hash) error {
	_, err := s.git(ctx, "update-ref", name, next.String(), prev.String())
	if err == nil {
		return nil
	}
	if refused(err.Error(), prev) {
		return fmt.Errorf("%w: %s", ErrRefMoved, err)
	}
	return err
}

// refused reports whether git's message says the ref moved, exists, is gone
// (only a non-zero prev expects it) or is locked by another process.
func refused(msg string, prev Hash) bool {
	for _, m := range []string{"but expected", "reference already exists", ".lock': File exists"} {
		if strings.Contains(msg, m) {
			return true
		}
	}
	return prev != ZeroHash && strings.Contains(msg, "unable to resolve reference")
}

// CommonDir is the absolute git common dir, shared by every worktree.
func (s *Store) CommonDir() string { return s.commonDir }

// Config returns a local git config value, or "" when it is unset.
func (s *Store) Config(ctx context.Context, key string) (string, error) {
	out, err := s.git(ctx, "config", "--local", "--get", key)
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 { // 1 is git's "unset"
		return "", nil
	}
	return strings.TrimSuffix(out, "\n"), err
}

// SetConfig sets a local git config value.
func (s *Store) SetConfig(ctx context.Context, key, value string) error {
	_, err := s.git(ctx, "config", "--local", key, value)
	return err
}

// command builds a git command on this repository. It runs in the C locale:
// UpdateRef matches git's refusals by their English text.
func (s *Store) command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "git", append([]string{"--git-dir", s.gitDir}, args...)...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	// On cancel git gets SIGINT, on which it removes its lock files; the
	// default SIGKILL would leave <ref>.lock behind. Still running 5 s later,
	// it is killed.
	cmd.Cancel = func() error { return cmd.Process.Signal(os.Interrupt) }
	cmd.WaitDelay = 5 * time.Second
	return cmd
}

// git runs git on this repository and returns its stdout. An error wraps
// git's stderr.
func (s *Store) git(ctx context.Context, args ...string) (string, error) {
	cmd := s.command(ctx, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gitstore: git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
