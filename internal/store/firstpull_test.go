package store_test

import (
	"testing"

	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// A clone opens on its remote's board: git clone brings no refs/board/*,
// so the first open pulls them (docs/prd/clone-shows-board.md).
func TestPullIfEmptyOnAFreshClone(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	storetest.NewIssue(t, theirs, "made before the clone")
	if _, err := theirs.Push("origin"); err != nil {
		t.Fatal(err)
	}
	n, err := mine.PullIfEmpty("origin")
	if err != nil || n != 1 {
		t.Fatalf("PullIfEmpty = %d, %v; want 1", n, err)
	}
	// once fetched, an open fetches no more
	if n, err := mine.PullIfEmpty("origin"); err != nil || n != 0 {
		t.Errorf("second open: %d, %v; want 0", n, err)
	}
}

// A board with issues of its own is never pulled on its own: its open
// shows what waits, and Pull is a click.
func TestPullIfEmptyLeavesABoardWithIssues(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	storetest.NewIssue(t, theirs, "theirs")
	if _, err := theirs.Push("origin"); err != nil {
		t.Fatal(err)
	}
	storetest.NewIssue(t, mine, "mine, not pushed")
	if n, err := mine.PullIfEmpty("origin"); err != nil || n != 0 {
		t.Fatalf("PullIfEmpty = %d, %v; want 0", n, err)
	}
	if is, _ := mine.ListIssues(); len(is) != 1 {
		t.Errorf("the board changed: %d issues", len(is))
	}
}

// No origin, no pull, no error: a board kept in one clone.
func TestPullIfEmptyWithoutTheRemote(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	if n, err := b.PullIfEmpty(store.BoardRemote); err != nil || n != 0 {
		t.Errorf("PullIfEmpty = %d, %v", n, err)
	}
}

// A remote with no board yet: nothing pulled, nothing said.
func TestPullIfEmptyWhenTheRemoteHasNoBoard(t *testing.T) {
	t.Parallel()
	mine, _ := storetest.NewPair(t)
	if n, err := mine.PullIfEmpty("origin"); err != nil || n != 0 {
		t.Errorf("PullIfEmpty = %d, %v", n, err)
	}
}
