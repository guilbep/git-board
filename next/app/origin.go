package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/guilbep/git-board/next/domain/issues"
)

// Numbers on git (docs/spec/datastores.md, The counter): origin's
// refs/board/seq hands them out. A pull takes origin's counter, or sends
// this clone's when it is strictly ahead; a push numbers the issues origin
// then has, by pushing the counter on top of the one fetched, not forced:
// a clone that pushed first took those numbers, and this one numbers again.
// As v1's internal/store/numbers.go.

const seqRef = "refs/board/seq"

func seqCopy(remote string) string { return "refs/board-remotes/" + remote + "/seq" }

// ErrNumbersAhead: this clone's counter has numbers the remote hasn't.
var ErrNumbersAhead = errors.New("refs/board/seq has numbers the remote hasn't")

// ref is where a ref is, "" for nowhere.
func (b *Board) ref(ctx context.Context, name string) (string, error) {
	out, err := gitRun(ctx, b.Dir, "for-each-ref", "--format=%(objectname)", name)
	return strings.TrimSpace(out), err
}

// behind says ours is theirs or one of its ancestors: taking theirs then
// drops nothing.
func (b *Board) behind(ctx context.Context, ours, theirs string) bool {
	if ours == "" || ours == theirs {
		return true
	}
	_, err := gitRun(ctx, b.Dir, "merge-base", "--is-ancestor", ours, theirs)
	return err == nil
}

// remoteSeq fetches the remote's counter, and says where it is: "" for
// none.
func (b *Board) remoteSeq(ctx context.Context, remote string) (string, error) {
	if _, err := gitRun(ctx, b.Dir, "fetch", "--no-write-fetch-head", "--quiet", remote, "+"+seqRef+":"+seqCopy(remote)); err != nil {
		if strings.Contains(err.Error(), "couldn't find remote ref") {
			return "", nil
		}
		return "", fmt.Errorf("fetching the numbers from %s: %w", remote, err)
	}
	return b.ref(ctx, seqCopy(remote))
}

// setSeq points this clone's counter at h.
func (b *Board) setSeq(ctx context.Context, h string) error {
	ours, err := b.ref(ctx, seqRef)
	if err != nil || ours == h {
		return err
	}
	if h == "" {
		_, err = gitRun(ctx, b.Dir, "update-ref", "-d", seqRef, ours)
		return err
	}
	_, err = gitRun(ctx, b.Dir, "update-ref", seqRef, h, ours)
	return err
}

// fetchNumbers takes the remote's counter; ErrNumbersAhead when this
// clone's isn't it nor behind it.
func (b *Board) fetchNumbers(ctx context.Context, remote string) error {
	theirs, err := b.remoteSeq(ctx, remote)
	if err != nil || theirs == "" {
		return err
	}
	ours, err := b.ref(ctx, seqRef)
	if err != nil {
		return err
	}
	if !b.behind(ctx, ours, theirs) {
		return fmt.Errorf("%w: git push %s %s first; or, to drop them, git update-ref %s %s",
			ErrNumbersAhead, remote, seqRef, seqRef, seqCopy(remote))
	}
	return b.setSeq(ctx, theirs)
}

// pullNumbers is a pull's step on origin: this clone's counter sent when
// strictly ahead of the remote's, else the remote's taken. It says what
// it sent.
func (b *Board) pullNumbers(ctx context.Context, remote string) (string, error) {
	if remote != NumbersRemote {
		return "", nil
	}
	theirs, err := b.remoteSeq(ctx, remote)
	if err != nil || theirs == "" {
		return "", err
	}
	ours, err := b.ref(ctx, seqRef)
	if err != nil {
		return "", err
	}
	if ours != "" && ours != theirs && b.behind(ctx, theirs, ours) {
		if _, err := gitRun(ctx, b.Dir, "push", "--quiet", remote, seqRef+":"+seqRef); err != nil {
			return "", fmt.Errorf("sending the numbers to %s: %w", remote, err)
		}
		return "sent refs/board/seq: numbers " + remote + " hadn't", b.fetchNumbers(ctx, remote)
	}
	return "", b.fetchNumbers(ctx, remote)
}

// pullCheck is Pull's check: a fetch, which merges nothing, and what it
// would do with the numbers.
func (b *Board) pullCheck(ctx context.Context, remote string) (string, error) {
	if _, _, err := b.Fetch(ctx, remote); err != nil {
		return "", err
	}
	if err := b.switched(ctx); err != nil {
		return "", err
	}
	if remote != NumbersRemote {
		return "", nil
	}
	theirs, err := b.remoteSeq(ctx, remote)
	if err != nil || theirs == "" {
		return "", err
	}
	ours, err := b.ref(ctx, seqRef)
	if err != nil {
		return "", err
	}
	if ours != "" && ours != theirs && b.behind(ctx, theirs, ours) {
		return "would send refs/board/seq: numbers " + remote + " hasn't", nil
	}
	return "", nil
}

// counterAt is this clone's counter and where it is; ok false for none.
func (b *Board) counterAt(ctx context.Context) (c issues.Counter, at string, ok bool, err error) {
	data, at, err := b.Kernel.Store().Caps().Counter.Count(ctx)
	if err != nil || data == nil {
		return c, at, false, err
	}
	c, err = issues.ParseCounter(data)
	return c, at, err == nil, err
}

// pushCounter commits c on parent ("" for the first) and pushes it. When
// the push fails, the remote's counter says what happened: ours (it
// landed), someone else's (rejected: they took the numbers), or still
// parent (a failure, the counter as it was).
func (b *Board) pushCounter(ctx context.Context, remote string, c issues.Counter, parent string) (rejected bool, err error) {
	commit, err := b.Kernel.Store().Caps().Counter.SetCount(ctx, c.Bytes(), parent)
	if err != nil {
		return false, err
	}
	if _, err := gitRun(ctx, b.Dir, "push", "--quiet", remote, seqRef+":"+seqRef); err != nil {
		err = fmt.Errorf("pushing the numbers to %s: %w", remote, err)
		theirs, ferr := b.remoteSeq(ctx, remote)
		switch {
		case ferr != nil:
			return false, errors.Join(err, ferr, b.setSeq(ctx, parent))
		case theirs == commit:
			return false, nil
		case theirs == parent:
			return false, errors.Join(err, b.setSeq(ctx, parent))
		}
		return true, errors.Join(err, b.setSeq(ctx, theirs))
	}
	return false, nil
}

// numberIssues gives the issues the remote has without a number the next
// ones, oldest first, and says how many; one made here waits for its
// push. A project that doesn't number its issues touches no remote.
func (b *Board) numberIssues(ctx context.Context, remote string) (int, error) {
	if _, _, ok, err := b.counterAt(ctx); !ok {
		return 0, err
	}
	var last error
	for range 5 {
		if err := b.fetchNumbers(ctx, remote); err != nil {
			return 0, err
		}
		n, rejected, err := b.numberOnce(ctx, remote)
		if !rejected {
			return n, err
		}
		last = err
	}
	return 0, fmt.Errorf("%w, sync again: %w", issues.ErrNumbersMoved, last)
}

func (b *Board) numberOnce(ctx context.Context, remote string) (int, bool, error) {
	c, head, _, err := b.counterAt(ctx)
	if err != nil {
		return 0, false, err
	}
	all, err := b.Issues.Unnumbered(ctx, c)
	if err != nil {
		return 0, false, err
	}
	out, err := gitRun(ctx, b.Dir, "for-each-ref", "--format=%(refname)", "refs/board-remotes/"+remote+"/issue/")
	if err != nil {
		return 0, false, err
	}
	there := map[string]bool{}
	for _, name := range strings.Fields(out) {
		there[strings.TrimPrefix(name, "refs/board-remotes/"+remote+"/issue/")] = true
	}
	var todo []string
	for _, id := range all {
		if there[id] {
			todo = append(todo, id)
		}
	}
	if len(todo) == 0 {
		return 0, false, nil
	}
	next := c.Next()
	for i, id := range todo {
		c.ByID[id] = next + i
	}
	rejected, err := b.pushCounter(ctx, remote, c, head)
	if err != nil {
		return 0, rejected, err
	}
	return len(todo), false, nil
}

// initOrigin makes the project number its issues, prefix-N from start:
// the counter's first commit, pushed to origin. Pushing it is the switch.
// When origin has one already, or another clone pushed one first, it
// says so (ErrNumbered) and this clone takes that one.
func (b *Board) initOrigin(ctx context.Context, prefix string, start int) error {
	if err := b.fetchNumbers(ctx, NumbersRemote); err != nil {
		return err
	}
	numbered := func() error {
		c, _, ok, err := b.counterAt(ctx)
		if err == nil && ok {
			err = fmt.Errorf("%w: %s, from %d", issues.ErrNumbered, c.Prefix, c.Start)
		}
		return err
	}
	if err := numbered(); err != nil {
		return err
	}
	rejected, err := b.pushCounter(ctx, NumbersRemote, issues.Counter{Prefix: prefix, Start: start, ByID: map[string]int{}, GitHub: b.Issues.GitHub}, "")
	if rejected {
		if e := numbered(); e != nil {
			return e
		}
	}
	return err
}
