package store

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/people"
)

// The reset's mark: written by git board reset --hard, a commit of a
// nonce at the reset's time, so that two resets never write the same one.
// It goes to the remote with git board push --prune, and a clone takes the
// remote's with git board reset --hard <remote>. A pull or a push between
// two boards whose marks differ refuses: merged, the old board and the
// new one would hold every issue twice, both copies linked to the same
// forge issue.
const resetRef = "refs/board/reset"

// resetCopy is where a remote's mark is fetched to.
func resetCopy(remote string) string { return "refs/board-remotes/" + remote + "/reset" }

// ErrStartedOver: this clone's board and the remote's started over
// apart, and the error says which and how to settle it.
var ErrStartedOver = errors.New("a board started over")

type startedOver struct{ msg string }

func (s startedOver) Error() string        { return s.msg }
func (s startedOver) Is(target error) bool { return target == ErrStartedOver }

func when(t time.Time) string { return t.Format("2006-01-02 15:04") }

// markTime is when a mark was written; the zero time for none.
func (b *Backend) markTime(h gitstore.Hash) time.Time {
	if h == gitstore.ZeroHash {
		return time.Time{}
	}
	out, err := b.run(nil, "show", "-s", "--format=%ct", h.String())
	if err != nil {
		return time.Time{}
	}
	sec, _ := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
	return time.Unix(sec, 0)
}

// fetchResetMark takes the remote's mark to its copy here, and drops the
// copy of a remote that has none.
func (b *Backend) fetchResetMark(remote string) error {
	_, err := b.run(nil, "fetch", "--no-write-fetch-head", "--quiet", remote, "+"+resetRef+":"+resetCopy(remote))
	if err == nil || !strings.Contains(err.Error(), "couldn't find remote ref") {
		return err
	}
	if h, err := b.s.Ref(resetCopy(remote)); err != nil || h == gitstore.ZeroHash {
		return err
	}
	_, err = b.run(nil, "update-ref", "-d", resetCopy(remote))
	return err
}

// ResetGuard compares this clone's mark with the remote's, as last
// fetched: the same, or none on either side, goes on. A clone with no
// issue nor person and no mark takes the remote's, as a new clone does.
// Otherwise it refuses, saying which side started over and the command
// that settles it.
func (b *Backend) ResetGuard(remote string) error {
	ours, err := b.s.Ref(resetRef)
	if err != nil {
		return err
	}
	theirs, err := b.s.Ref(resetCopy(remote))
	if err != nil || ours == theirs {
		return err
	}
	if ours == gitstore.ZeroHash {
		issues, err := b.s.Refs("refs/board/issue/")
		if err != nil {
			return err
		}
		persons, err := b.s.Refs("refs/board/person/")
		if err != nil {
			return err
		}
		if len(issues)+len(persons) == 0 {
			return b.s.UpdateRef(ctx, resetRef, theirs, gitstore.ZeroHash)
		}
	}
	oursAt, theirsAt := b.markTime(ours), b.markTime(theirs)
	if theirs != gitstore.ZeroHash && (ours == gitstore.ZeroHash || theirsAt.After(oursAt)) {
		return startedOver{fmt.Sprintf("%s's board was started over on %s: git board reset --hard %s takes it", remote, when(theirsAt), remote)}
	}
	return startedOver{fmt.Sprintf("this clone's board was started over on %s, %s's wasn't: git board push --prune makes %s match, git board reset --hard %s goes back to it",
		when(oursAt), remote, remote, remote)}
}

// guarded fetches the remote's mark, then compares: a push's guard.
func (b *Backend) guarded(remote string) error {
	if err := b.fetchResetMark(remote); err != nil {
		return err
	}
	return b.ResetGuard(remote)
}

// writeResetMark sets a new mark, whatever was there.
func writeResetMark(b *Backend) (gitstore.Hash, error) {
	nonce := make([]byte, 16)
	rand.Read(nonce) // never fails, since Go 1.24
	blob, err := b.s.WriteBlob([]byte(hex.EncodeToString(nonce)))
	if err != nil {
		return gitstore.ZeroHash, err
	}
	tree, err := b.s.WriteTree([]gitstore.TreeEntry{{Name: "nonce", Blob: blob}})
	if err != nil {
		return gitstore.ZeroHash, err
	}
	c, err := b.s.WriteCommit(gitstore.Commit{Tree: tree})
	if err != nil {
		return gitstore.ZeroHash, err
	}
	old, err := b.s.Ref(resetRef)
	if err != nil {
		return gitstore.ZeroHash, err
	}
	return c, b.s.UpdateRef(ctx, resetRef, c, old)
}

// ResetReport is what a reset or a push --prune deleted, and where it
// backed it up.
type ResetReport struct {
	Deleted int    // refs deleted, or moved to the remote's
	Backup  string // the bundle of them, "" when none
	Me      bool   // board.me was unset: its person is gone
}

// ResetHard starts this clone's board over. Without a remote it empties
// it: every ref under refs/board/ but the switch's mark and the numbers'
// counter goes, with the fetched copies, and a new reset's mark is
// written. With one it makes the board the remote's, marks included.
// Either first writes every ref it deletes or moves to a bundle, drops
// the fetched copies (a fetch doesn't prune them, and a pull would merge
// one of a board gone), and unsets board.me once its person is gone.
func (b *Backend) ResetHard(remote string) (ResetReport, error) {
	var rep ResetReport
	if remote != "" {
		if err := b.checkRemote(remote); err != nil {
			return rep, err
		}
	}
	all, err := b.s.Refs("refs/")
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
	if rep.Backup, err = b.backup("board-backup", append(slices.Clone(refs), copies...)); err != nil {
		return rep, err
	}
	if err := b.dropRefs(copies, all); err != nil {
		return rep, err
	}
	if remote == "" {
		if err := b.dropRefs(refs, all); err != nil {
			return rep, err
		}
		if _, err := writeResetMark(b); err != nil {
			return rep, err
		}
		rep.Deleted = len(refs)
	} else {
		if _, err := b.run(nil, "fetch", "--prune", "--no-write-fetch-head", "--quiet", remote, "+refs/board/*:refs/board/*"); err != nil {
			return rep, fmt.Errorf("taking %s's board: %w", remote, err)
		}
		if err := b.Fetch(remote); err != nil {
			return rep, err
		}
		after, err := b.s.Refs("refs/board/")
		if err != nil {
			return rep, err
		}
		for _, name := range refs {
			if after[name] != all[name] {
				rep.Deleted++
			}
		}
	}
	id, err := people.Me(ctx, b.s)
	if err != nil || id == "" {
		return rep, err
	}
	if h, err := b.s.Ref(oplog.RefName("person", id)); err != nil || h != gitstore.ZeroHash {
		return rep, err
	}
	if _, err := b.run(nil, "config", "--local", "--unset-all", "board.me"); err != nil {
		return rep, err
	}
	rep.Me = true
	return rep, nil
}

// backup writes refs to a bundle in the git directory, named
// <name>-<time>.bundle to the millisecond, and answers its path: "" for
// no ref.
func (b *Backend) backup(name string, refs []string) (string, error) {
	if len(refs) == 0 {
		return "", nil
	}
	path := filepath.Join(b.s.CommonDir(), name+"-"+time.Now().Format("20060102-150405.000")+".bundle")
	_, err := b.run([]byte(strings.Join(refs, "\n")+"\n"), "bundle", "create", "--quiet", path, "--stdin")
	return path, err
}

// dropRefs deletes refs, each only if it still holds what all says, in
// one transaction.
func (b *Backend) dropRefs(refs []string, all map[string]gitstore.Hash) error {
	if len(refs) == 0 {
		return nil
	}
	var del strings.Builder
	for _, name := range refs {
		fmt.Fprintf(&del, "delete %s %s\n", name, all[name])
	}
	_, err := b.run([]byte(del.String()), "update-ref", "--stdin")
	return err
}
