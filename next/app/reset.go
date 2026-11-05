package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/guilbep/git-board/next/store/git"
	"github.com/guilbep/git-board/next/sync"
)

// Starting over, as v1's git board reset --hard and push --prune: on the
// git side alone, no forge touched.
//
// The reset's mark: written by reset --hard, a commit of a nonce at the
// reset's time, so that two resets never write the same one. It goes to
// the remote with push --prune, and a clone takes the remote's with reset
// --hard <remote>. A pull or a push between two boards whose marks differ
// refuses: merged, the old board and the new one would hold every issue
// twice, both copies linked to the same forge issue.
const (
	resetRef    = "refs/board/reset"
	switchedRef = "refs/board/switched"
)

// resetCopy is where a remote's mark is fetched to.
func resetCopy(remote string) string { return "refs/board-remotes/" + remote + "/reset" }

// ErrStartedOver: this clone's board and the remote's started over apart.
var ErrStartedOver = errors.New("a board started over")

// ErrNoStartOver: push --prune is a start-over's second step, and this
// clone has none for the remote.
var ErrNoStartOver = errors.New("no start-over here to push")

type startedOver struct{ msg string }

func (s startedOver) Error() string        { return s.msg }
func (s startedOver) Is(target error) bool { return target == ErrStartedOver }

// ResetReport is what a reset or a push --prune deleted, and where it
// backed it up.
type ResetReport struct {
	Deleted int    // refs deleted, or moved to the remote's
	Backup  string // the bundle of them, "" when none
	Me      bool   // board.me was unset: its person is gone
}

// refsUnder answers the refs under prefix, name -> hash.
func (b *Board) refsUnder(ctx context.Context, prefix string) (map[string]string, error) {
	out, err := gitRun(ctx, b.Dir, "for-each-ref", "--format=%(objectname) %(refname)", prefix)
	if err != nil {
		return nil, err
	}
	refs := map[string]string{}
	for line := range strings.Lines(out) {
		if h, name, ok := strings.Cut(strings.TrimSpace(line), " "); ok {
			refs[name] = h
		}
	}
	return refs, nil
}

// markTime is when a mark was written; the zero time for none.
func (b *Board) markTime(ctx context.Context, h string) time.Time {
	if h == "" {
		return time.Time{}
	}
	out, err := gitRun(ctx, b.Dir, "show", "-s", "--format=%ct", h)
	if err != nil {
		return time.Time{}
	}
	sec, _ := strconv.ParseInt(strings.TrimSpace(out), 10, 64)
	return time.Unix(sec, 0)
}

// fetchResetMark takes the remote's mark to its copy here, and drops the
// copy of a remote that has none.
func (b *Board) fetchResetMark(ctx context.Context, remote string) error {
	_, err := gitRun(ctx, b.Dir, "fetch", "--no-write-fetch-head", "--quiet", remote, "+"+resetRef+":"+resetCopy(remote))
	if err == nil || !strings.Contains(err.Error(), "couldn't find remote ref") {
		return err
	}
	if h, err := b.ref(ctx, resetCopy(remote)); err != nil || h == "" {
		return err
	}
	_, err = gitRun(ctx, b.Dir, "update-ref", "-d", resetCopy(remote))
	return err
}

// resetGuard fetches the remote's mark and compares it with this clone's:
// the same, or none on either side, goes on. A clone with no issue nor
// person and no mark takes the remote's, as a new clone does. Otherwise
// it refuses, saying which side started over and what settles it.
func (b *Board) resetGuard(ctx context.Context, remote string) error {
	if err := b.fetchResetMark(ctx, remote); err != nil {
		return err
	}
	ours, err := b.ref(ctx, resetRef)
	if err != nil {
		return err
	}
	theirs, err := b.ref(ctx, resetCopy(remote))
	if err != nil || ours == theirs {
		return err
	}
	if ours == "" {
		issues, err := b.refsUnder(ctx, "refs/board/issue/")
		if err != nil {
			return err
		}
		persons, err := b.refsUnder(ctx, "refs/board/person/")
		if err != nil {
			return err
		}
		if len(issues)+len(persons) == 0 {
			_, err := gitRun(ctx, b.Dir, "update-ref", resetRef, theirs, "")
			return err
		}
	}
	when := func(t time.Time) string { return t.Format("2006-01-02 15:04") }
	oursAt, theirsAt := b.markTime(ctx, ours), b.markTime(ctx, theirs)
	if theirs != "" && (ours == "" || theirsAt.After(oursAt)) {
		return startedOver{fmt.Sprintf("%s's board was started over on %s: git board reset --hard %s takes it", remote, when(theirsAt), remote)}
	}
	return startedOver{fmt.Sprintf("this clone's board was started over on %s, %s's wasn't: git board push --prune makes %s match, git board reset --hard %s goes back to it",
		when(oursAt), remote, remote, remote)}
}

// writeResetMark sets a new mark, whatever was there.
func (b *Board) writeResetMark(ctx context.Context) error {
	nonce := make([]byte, 16)
	rand.Read(nonce)
	blob, err := gitRunIn(ctx, b.Dir, hex.EncodeToString(nonce), "hash-object", "-w", "--stdin")
	if err != nil {
		return err
	}
	tree, err := gitRunIn(ctx, b.Dir, "100644 blob "+blob+"\tnonce\n", "mktree")
	if err != nil {
		return err
	}
	c, err := gitRunIn(ctx, b.Dir, "", "commit-tree", tree, "-m", "reset")
	if err != nil {
		return err
	}
	_, err = gitRun(ctx, b.Dir, "update-ref", resetRef, c)
	return err
}

// gitRunIn runs git with stdin, and answers its output trimmed.
func gitRunIn(ctx context.Context, dir, stdin string, args ...string) (string, error) {
	out, err := gitStdin(ctx, dir, []byte(stdin), args...)
	return strings.TrimSpace(out), err
}

// needGit refuses starting over on a board not kept in git.
func (b *Board) needGit() (*git.Store, error) {
	g, ok := b.Kernel.Store().(*git.Store)
	if !ok {
		return nil, missing("starting over needs a board kept in git; this board is kept in " + b.DatastoreName())
	}
	return g, nil
}

// ResetHard starts this clone's board over. Without a remote it empties
// it: every ref under refs/board/ but the switch's mark and the numbers'
// counter goes, with the fetched copies, and a new reset's mark is
// written. With one it makes the board the remote's, marks included.
// Either first writes every ref it deletes or moves to a bundle, drops
// the fetched copies, and unsets board.me once its person is gone.
func (b *Board) ResetHard(ctx context.Context, remote string) (ResetReport, error) {
	var rep ResetReport
	g, err := b.needGit()
	if err != nil {
		return rep, err
	}
	if remote != "" {
		if _, err := b.remoteOf(ctx, "git board reset", remote); err != nil {
			return rep, err
		}
	}
	// the sync's journal says what it pulled into the refs that go
	if err := sync.Forget(g); err != nil {
		return rep, err
	}
	all, err := b.refsUnder(ctx, "refs/")
	if err != nil {
		return rep, err
	}
	var refs, copies []string
	for name := range all {
		switch {
		case strings.HasPrefix(name, "refs/board-remotes/"):
			copies = append(copies, name)
		case strings.HasPrefix(name, "refs/board/") && (remote != "" || name != switchedRef && name != seqRef):
			refs = append(refs, name)
		}
	}
	slices.Sort(refs)
	slices.Sort(copies)
	if rep.Backup, err = b.backup(ctx, "board-backup", append(slices.Clone(refs), copies...)); err != nil {
		return rep, err
	}
	if err := b.dropRefs(ctx, copies, all); err != nil {
		return rep, err
	}
	if remote == "" {
		if err := b.dropRefs(ctx, refs, all); err != nil {
			return rep, err
		}
		if err := b.writeResetMark(ctx); err != nil {
			return rep, err
		}
		rep.Deleted = len(refs)
	} else {
		if _, err := gitRun(ctx, b.Dir, "fetch", "--prune", "--no-write-fetch-head", "--quiet", remote, "+refs/board/*:refs/board/*"); err != nil {
			return rep, fmt.Errorf("taking %s's board: %w", remote, err)
		}
		if _, err := g.Fetch(ctx, remote, Kinds...); err != nil {
			return rep, err
		}
		after, err := b.refsUnder(ctx, "refs/board/")
		if err != nil {
			return rep, err
		}
		for _, name := range refs {
			if after[name] != all[name] {
				rep.Deleted++
			}
		}
	}
	me := strings.TrimSpace(b.GitConfig(ctx, "board.me"))
	if me == "" {
		return rep, nil
	}
	if h, err := b.ref(ctx, "refs/board/person/"+me); err != nil || h != "" {
		return rep, err
	}
	if _, err := gitRun(ctx, b.Dir, "config", "--local", "--unset-all", "board.me"); err != nil {
		return rep, err
	}
	rep.Me = true
	return rep, nil
}

// backup writes refs to a bundle in the git directory, named
// <name>-<time>.bundle to the millisecond, and answers its path: "" for
// no ref.
func (b *Board) backup(ctx context.Context, name string, refs []string) (string, error) {
	if len(refs) == 0 {
		return "", nil
	}
	common, err := gitRun(ctx, b.Dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return "", err
	}
	path := filepath.Join(strings.TrimSpace(common), name+"-"+time.Now().Format("20060102-150405.000")+".bundle")
	_, err = gitStdin(ctx, b.Dir, []byte(strings.Join(refs, "\n")+"\n"), "bundle", "create", "--quiet", path, "--stdin")
	return path, err
}

// dropRefs deletes refs, each only if it still holds what all says, in
// one transaction.
func (b *Board) dropRefs(ctx context.Context, refs []string, all map[string]string) error {
	if len(refs) == 0 {
		return nil
	}
	var del strings.Builder
	for _, name := range refs {
		fmt.Fprintf(&del, "delete %s %s\n", name, all[name])
	}
	_, err := gitStdin(ctx, b.Dir, []byte(del.String()), "update-ref", "--stdin")
	return err
}

// PushPrune makes the remote's board this clone's: a start-over's second
// step, refused (ErrNoStartOver) unless this clone's reset's mark is one
// the remote hasn't. It fetches the remote's board to refs/board-pruned/,
// writes it to a bundle and drops it, then pushes every ref under
// refs/board/, forced, deleting there what this clone doesn't have. A
// numbers' counter the remote moved on is taken first, so that no number
// is handed out twice. Unguarded: it is one of the ways out of a guard.
func (b *Board) PushPrune(ctx context.Context, remote string) (ResetReport, error) {
	var rep ResetReport
	g, err := b.needGit()
	if err != nil {
		return rep, err
	}
	if _, err := b.remoteOf(ctx, "git board push", remote); err != nil {
		return rep, err
	}
	if err := b.fetchResetMark(ctx, remote); err != nil {
		return rep, err
	}
	mark, err := b.ref(ctx, resetRef)
	if err != nil {
		return rep, err
	}
	theirMark, err := b.ref(ctx, resetCopy(remote))
	switch {
	case err != nil:
		return rep, err
	case mark == "":
		return rep, fmt.Errorf("%w: this clone's board didn't start over; git board reset --hard first", ErrNoStartOver)
	case mark == theirMark:
		return rep, fmt.Errorf("%w: %s has this clone's start-over already", ErrNoStartOver, remote)
	}
	if _, err := gitRun(ctx, b.Dir, "fetch", "--no-write-fetch-head", "--quiet", remote, "+refs/board/*:refs/board-pruned/*"); err != nil {
		return rep, fmt.Errorf("fetching %s's board to back it up: %w", remote, err)
	}
	theirs, err := b.refsUnder(ctx, "refs/board-pruned/")
	if err != nil {
		return rep, err
	}
	if seq, ok := theirs["refs/board-pruned/seq"]; ok {
		ours, err := b.ref(ctx, seqRef)
		if err != nil {
			return rep, err
		}
		// the remote's counter moved on from this clone's: take it
		if b.behind(ctx, ours, seq) && ours != seq {
			if _, err := gitRun(ctx, b.Dir, "update-ref", seqRef, seq); err != nil {
				return rep, err
			}
		}
	}
	ours, err := b.refsUnder(ctx, "refs/board/")
	if err != nil {
		return rep, err
	}
	names := make([]string, 0, len(theirs))
	for name := range theirs {
		names = append(names, name)
		if _, here := ours["refs/board/"+strings.TrimPrefix(name, "refs/board-pruned/")]; !here {
			rep.Deleted++
		}
	}
	slices.Sort(names)
	if rep.Backup, err = b.backup(ctx, remote+"-board-backup", names); err != nil {
		return rep, err
	}
	if err := b.dropRefs(ctx, names, theirs); err != nil {
		return rep, err
	}
	if _, err := gitRun(ctx, b.Dir, "push", "--quiet", "--prune", "--force", remote, "refs/board/*:refs/board/*"); err != nil {
		return rep, fmt.Errorf("pushing to %s: %w", remote, err)
	}
	copies, err := b.refsUnder(ctx, "refs/board-remotes/"+remote+"/")
	if err != nil {
		return rep, err
	}
	names = names[:0]
	for name := range copies {
		names = append(names, name)
	}
	slices.Sort(names)
	if err := b.dropRefs(ctx, names, copies); err != nil {
		return rep, err
	}
	if _, err := g.Fetch(ctx, remote, Kinds...); err != nil {
		return rep, err
	}
	return rep, b.fetchResetMark(ctx, remote)
}
