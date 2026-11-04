package main

import (
	"errors"
	"testing"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// Issue #84: a Markdown board starts its numbers and numbers on main; on
// another branch, git board numbers says why it waits. Serial: it chdirs.
func TestNumbersOnAMarkdownBoard(t *testing.T) {
	dir := storetest.Repo(t, false)
	storetest.Git(t, dir, "switch", "-q", "-c", "main")
	storetest.Git(t, dir, "config", store.DatastoreKey, "markdown:issues")
	storetest.Init(t, dir)
	t.Chdir(dir)

	if out, err := execute(t, []string{"numbers", "init", "--prefix", "GB", "--start", "1"}); err != nil || out != "numbering as GB-N from 1, in Markdown files\n" {
		t.Fatalf("numbers init: %v\n%s", err, out)
	}
	if _, err := execute(t, []string{"new", "On main"}); err != nil {
		t.Fatal(err)
	}
	if out, err := execute(t, []string{"numbers"}); err != nil || out != "numbered 1 issue(s)\n" {
		t.Fatalf("numbers on main: %v\n%s", err, out)
	}
	storetest.Git(t, dir, "add", "-A")
	storetest.Git(t, dir, "commit", "-q", "-m", "the board")
	storetest.Git(t, dir, "switch", "-q", "-c", "feat/csv")
	if _, err := execute(t, []string{"new", "On a branch"}); err != nil {
		t.Fatal(err)
	}
	_, err := execute(t, []string{"numbers"})
	if !errors.Is(err, datastore.ErrMissing) || err.Error() != "project numbers are handed out on main alone: this checkout is on feat/csv. Its issues take their numbers once merged into main" {
		t.Fatalf("numbers on feat/csv: %v", err)
	}
}
