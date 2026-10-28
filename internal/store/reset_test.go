package store_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// pushed pushes b's board to origin, failing the test on an error.
func pushedTo(t *testing.T, b *store.Backend) {
	t.Helper()
	if _, err := b.PushStep("origin"); err != nil {
		t.Fatalf("push: %v", err)
	}
}

// markPushed writes a reset's mark in b and sends it alone to origin, as
// git board push --prune would with an empty board.
func markPushed(t *testing.T, b *store.Backend) {
	t.Helper()
	if _, err := store.WriteResetMark(b); err != nil {
		t.Fatal(err)
	}
	storetest.Git(t, b.GitDir(), "push", "-q", "origin", "refs/board/reset")
}

func issueCount(t *testing.T, b *store.Backend) int {
	t.Helper()
	is, err := b.ListIssues()
	if err != nil {
		t.Fatal(err)
	}
	return len(is)
}

// A clone that missed a start-over can't merge the old board into the
// new one, nor send it: it's told to take origin's.
func TestResetGuardRefusesACloneLeftBehind(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	storetest.NewIssue(t, mine, "mine")
	storetest.NewIssue(t, theirs, "theirs")
	pushedTo(t, mine)
	pushedTo(t, theirs)
	if err := mine.Pull("origin"); err != nil {
		t.Fatal(err)
	}
	markPushed(t, mine)

	before := issueCount(t, theirs)
	err := theirs.Pull("origin")
	if !errors.Is(err, store.ErrStartedOver) || !strings.HasPrefix(err.Error(), "origin's board was started over on ") ||
		!strings.HasSuffix(err.Error(), ": git board reset --hard origin takes it") {
		t.Fatalf("pull: %v", err)
	}
	if _, err := theirs.Push("origin"); !errors.Is(err, store.ErrStartedOver) {
		t.Fatalf("push: %v", err)
	}
	if _, err := theirs.PullStep("origin", true); !errors.Is(err, store.ErrStartedOver) {
		t.Fatalf("the whole sync's check: %v", err)
	}
	if after := issueCount(t, theirs); after != before {
		t.Errorf("the refused pull merged: %d issues, then %d", before, after)
	}
}

// The clone that started over and didn't prune origin is told how.
func TestResetGuardTellsTheOneThatStartedOver(t *testing.T) {
	t.Parallel()
	mine, _ := storetest.NewPair(t)
	if _, err := store.WriteResetMark(mine); err != nil {
		t.Fatal(err)
	}
	err := mine.Pull("origin")
	if !errors.Is(err, store.ErrStartedOver) || !strings.HasPrefix(err.Error(), "this clone's board was started over on ") ||
		!strings.Contains(err.Error(), ", origin's wasn't: git board push --prune makes origin match, git board reset --hard origin goes back to it") {
		t.Fatalf("pull: %v", err)
	}
}

// A new clone, with no board and no mark, takes origin's as it pulls.
func TestResetGuardLetsANewCloneIn(t *testing.T) {
	t.Parallel()
	mine, _ := storetest.NewPair(t)
	storetest.NewIssue(t, mine, "mine")
	pushedTo(t, mine)
	markPushed(t, mine)

	dir := storetest.Repo(t, false)
	storetest.Git(t, dir, "remote", "add", "origin", strings.TrimSpace(storetest.Git(t, mine.GitDir(), "remote", "get-url", "origin")))
	fresh := storetest.Open(t, dir)
	if err := fresh.Pull("origin"); err != nil {
		t.Fatalf("pull: %v", err)
	}
	ours, _ := mine.Store().Ref("refs/board/reset")
	if got, _ := fresh.Store().Ref("refs/board/reset"); got != ours || issueCount(t, fresh) != 1 {
		t.Fatalf("mark %s (want %s), %d issues", got, ours, issueCount(t, fresh))
	}
}

func TestResetGuardIsQuietWithoutMarks(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	storetest.NewIssue(t, mine, "mine")
	pushedTo(t, mine)
	if err := theirs.Pull("origin"); err != nil {
		t.Fatal(err)
	}
	pushedTo(t, theirs)
}

// The board goes, bar the switch's mark and the numbers' counter, after a
// bundle of what goes: the issues, the people, the fetched copies, and
// board.me with its person.
func TestResetHardEmptiesTheBoard(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	a := storetest.NewIssue(t, b, "a")
	storetest.NewIssue(t, b, "b")
	c, err := store.StoreCommit(b, "x")
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"refs/board/switched", "refs/board-remotes/origin/issue/" + a} {
		if err := b.Store().UpdateRef(t.Context(), ref, c, gitstore.ZeroHash); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.WriteCounter(b, []byte("{}")); err != nil {
		t.Fatal(err)
	}
	seq, _ := b.Store().Ref("refs/board/seq")

	rep, err := b.ResetHard("")
	if err != nil {
		t.Fatal(err)
	}
	if n := issueCount(t, b); n != 0 {
		t.Errorf("%d issues left", n)
	}
	if got, _ := b.Store().Ref("refs/board/switched"); got != c {
		t.Errorf("the switch's mark: %s", got)
	}
	if got, _ := b.Store().Ref("refs/board/seq"); got != seq {
		t.Errorf("the counter: %s", got)
	}
	if got, _ := b.Store().Ref("refs/board/reset"); got == gitstore.ZeroHash {
		t.Error("no reset's mark")
	}
	if copies, _ := b.Store().Refs("refs/board-remotes/"); len(copies) != 0 {
		t.Errorf("copies left: %v", copies)
	}
	if me, _ := people.Me(t.Context(), b.Store()); !rep.Me || me != "" {
		t.Errorf("board.me %q, Me %v", me, rep.Me)
	}
	if heads := storetest.Git(t, b.GitDir(), "bundle", "list-heads", rep.Backup); !strings.Contains(heads, "refs/board/issue/"+a) {
		t.Errorf("the backup: %q", heads)
	}
}

func TestResetHardTakesTheRemote(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	storetest.NewIssue(t, mine, "kept")
	pushedTo(t, mine)
	if err := theirs.Pull("origin"); err != nil {
		t.Fatal(err)
	}
	storetest.NewIssue(t, theirs, "local only")

	if _, err := theirs.ResetHard("origin"); err != nil {
		t.Fatal(err)
	}
	is, _ := theirs.ListIssues()
	if len(is) != 1 || is[0].Title != "kept" {
		t.Fatalf("issues %+v", is)
	}
}

func TestResetHardTakesAnEmptyRemote(t *testing.T) {
	t.Parallel()
	_, theirs := storetest.NewPair(t)
	storetest.NewIssue(t, theirs, "local only")
	if _, err := theirs.ResetHard("origin"); err != nil {
		t.Fatal(err)
	}
	if n := issueCount(t, theirs); n != 0 {
		t.Errorf("%d issues left", n)
	}
}

func TestResetHardKeepsMeWhenTheRemoteHasIt(t *testing.T) {
	t.Parallel()
	_, theirs := storetest.NewPair(t)
	storetest.NewIssue(t, theirs, "theirs")
	pushedTo(t, theirs)
	before, _ := people.Me(t.Context(), theirs.Store())

	rep, err := theirs.ResetHard("origin")
	if err != nil {
		t.Fatal(err)
	}
	if after, _ := people.Me(t.Context(), theirs.Store()); rep.Me || after != before || before == "" {
		t.Errorf("board.me %q, then %q; Me %v", before, after, rep.Me)
	}
}

// The whole start-over: one clone empties its board and gives it to
// origin; the other is refused until it takes origin's, and the issues
// it had fetched don't come back with its next pull.
func TestPushPruneMakesTheRemoteMatch(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	storetest.NewIssue(t, mine, "mine")
	pushedTo(t, mine)
	storetest.NewIssue(t, theirs, "theirs")
	if err := theirs.Pull("origin"); err != nil {
		t.Fatal(err)
	}
	pushedTo(t, theirs)
	origin := strings.TrimSpace(storetest.Git(t, mine.GitDir(), "remote", "get-url", "origin"))

	if _, err := mine.ResetHard(""); err != nil {
		t.Fatal(err)
	}
	rep, err := mine.PushPrune("origin")
	if err != nil {
		t.Fatal(err)
	}
	if left := storetest.Git(t, origin, "for-each-ref", "refs/board/issue"); left != "" {
		t.Errorf("origin's issues left: %q", left)
	}
	ours, _ := mine.Store().Ref("refs/board/reset")
	if theirsMark := strings.TrimSpace(storetest.Git(t, origin, "rev-parse", "refs/board/reset")); theirsMark != ours.String() {
		t.Errorf("origin's mark %s, want %s", theirsMark, ours)
	}
	if heads := storetest.Git(t, mine.GitDir(), "bundle", "list-heads", rep.Backup); strings.Count(heads, "refs/board-pruned/issue/") != 2 || rep.Deleted < 2 {
		t.Errorf("the backup (%d deleted): %q", rep.Deleted, heads)
	}

	if err := theirs.Pull("origin"); !errors.Is(err, store.ErrStartedOver) {
		t.Fatalf("the clone left behind pulled: %v", err)
	}
	if _, err := theirs.ResetHard("origin"); err != nil {
		t.Fatal(err)
	}
	if err := theirs.Pull("origin"); err != nil {
		t.Fatal(err)
	}
	if n := issueCount(t, theirs); n != 0 {
		t.Errorf("%d issues came back", n)
	}
}

// A ref origin moved since this clone's reset, its counter numbered by a
// teammate, doesn't stop the push: it makes origin match.
func TestPushPruneOverADivergedRef(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	if err := store.WriteCounter(mine, []byte("1")); err != nil {
		t.Fatal(err)
	}
	storetest.Git(t, mine.GitDir(), "push", "-q", "origin", "refs/board/seq")
	if err := store.WriteCounter(theirs, []byte("2")); err != nil {
		t.Fatal(err)
	}
	storetest.Git(t, theirs.GitDir(), "push", "-q", "-f", "origin", "refs/board/seq")

	if _, err := mine.ResetHard(""); err != nil {
		t.Fatal(err)
	}
	if _, err := mine.PushPrune("origin"); err != nil {
		t.Fatal(err)
	}
	origin := strings.TrimSpace(storetest.Git(t, mine.GitDir(), "remote", "get-url", "origin"))
	ours, _ := mine.Store().Ref("refs/board/seq")
	if got := strings.TrimSpace(storetest.Git(t, origin, "rev-parse", "refs/board/seq")); got != ours.String() {
		t.Errorf("origin's counter %s, want %s", got, ours)
	}
}
