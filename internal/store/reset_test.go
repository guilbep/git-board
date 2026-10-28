package store_test

import (
	"errors"
	"strings"
	"testing"

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
