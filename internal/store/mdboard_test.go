package store_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/mdstore"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// A board kept in Markdown files: its packs in issues/.board, a file per
// issue rendered after each write, a hand edit read in with the next write
// or an import; a git board copies in with its ids.
func TestMarkdownBoard(t *testing.T) {
	t.Parallel()
	dir := storetest.Repo(t, false)
	storetest.Init(t, dir) // dev, in git first
	g := storetest.Open(t, dir)
	first, err := g.NewIssue("Kept in git first", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if res := g.TestDatastore(t.Context(), "markdown:issues"); !res.OK {
		t.Fatalf("test: %+v", res)
	}
	if n, err := g.CopyGitTo(t.Context(), "markdown:issues"); err != nil || n != 2 {
		t.Fatalf("copy: %d, %v; want the issue and dev", n, err)
	}
	folder := filepath.Join(dir, "issues")
	if files, _ := filepath.Glob(filepath.Join(folder, "*.md")); len(files) != 1 {
		t.Fatalf("rendered %v, want one file", files)
	}

	storetest.Git(t, dir, "config", store.DatastoreKey, "markdown:issues")
	b := storetest.Open(t, dir)
	if c := b.Capabilities(); c.Datastore != "Markdown files" || c.Replica || c.Counter {
		t.Fatalf("capabilities %+v", c)
	}
	if _, err := b.GetIssue(first); err != nil {
		t.Fatalf("the copied issue: %v", err)
	}
	id, err := b.NewIssue("Kept in files", "the body", []string{"prio:p1"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(folder, mdstore.Board, "issue", id+".jsonl")); err != nil {
		t.Fatalf("the packs: %v", err)
	}
	files, _ := filepath.Glob(filepath.Join(folder, "*kept-in-files.md"))
	if len(files) != 1 {
		t.Fatalf("no file rendered for the new issue")
	}
	// a hand edit, read in with the next write
	edit(t, files[0], "the body", "the body, by hand")
	if _, err := b.AddComment(first, "a write elsewhere"); err != nil {
		t.Fatal(err)
	}
	if th, err := b.Thread(id); err != nil || th.Events[0].Body != "the body, by hand" {
		t.Fatalf("after the next write: %+v, %v", th.Events, err)
	}
	if refs := storetest.Git(t, dir, "for-each-ref", "refs/board/issue/"); !strings.Contains(refs, first) || strings.Contains(refs, id) {
		t.Fatalf("git refs %q: the first issue's, untouched, and nothing of the new one", refs)
	}
	if err := b.Pull("origin"); err == nil || !strings.Contains(err.Error(), "Markdown files") {
		t.Fatalf("pull: %v", err)
	}
	if _, err := b.ResetHard(""); err == nil {
		t.Fatal("start over: no error")
	} else if !isMissing(err) {
		t.Fatalf("start over: %v", err)
	}
}

func isMissing(err error) bool {
	for e := err; e != nil; {
		if e == datastore.ErrMissing {
			return true
		}
		u, ok := e.(interface{ Unwrap() error })
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}

// Two writes to one issue in one second both stay: the files are read in
// before a write, while they still say what the board held. A hand edit is
// read in by the next write to its own issue.
func TestMarkdownBoardWritesInARow(t *testing.T) {
	t.Parallel()
	dir := storetest.Repo(t, false)
	storetest.Git(t, dir, "config", store.DatastoreKey, "markdown:issues")
	storetest.Init(t, dir)
	b := storetest.Open(t, dir)
	id, err := b.NewIssue("Title", "body", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.ApplyLabelChange(id, []string{"prio:p1"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := b.SetTitle(id, "Title 2"); err != nil {
		t.Fatal(err)
	}
	is, err := b.GetIssue(id)
	if err != nil || is.Title != "Title 2" || len(is.Labels) != 1 {
		t.Fatalf("issue %+v, %v; want both writes", is, err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "issues", "*.md"))
	if len(files) != 1 {
		t.Fatalf("files %v", files)
	}
	edit(t, files[0], "body", "body, by hand")
	if _, err := b.AddComment(id, "a write to it"); err != nil {
		t.Fatal(err)
	}
	if evs := eventsOf(t, b, id); evs[0].Body != "body, by hand" {
		t.Fatalf("description %q, want the hand edit", evs[0].Body)
	}
	if data, _ := os.ReadFile(files[0]); !strings.Contains(string(data), "a write to it") || !strings.Contains(string(data), "body, by hand") {
		t.Fatalf("the file after:\n%s", data)
	}
}

// The forge sync writes the board past the backend's writes: the files
// follow it too, so a hand edit made after the sync is read in, not lost.
func TestMarkdownBoardFollowsAForgeSync(t *testing.T) {
	t.Parallel()
	dir := storetest.Repo(t, false)
	storetest.Git(t, dir, "config", store.DatastoreKey, "markdown:issues")
	storetest.Init(t, dir)
	b := storetest.Open(t, dir)
	other, err := b.NewIssue("Other", "x", nil)
	if err != nil {
		t.Fatal(err)
	}
	f := fake.New("github.com/o/r")
	f.Add(forge.Issue{Author: forge.Account{ID: "7", Login: "ann"}, Title: "Pulled", Body: "pulled body", Status: "open", Created: 1000})
	if _, err := b.SyncForges(t.Context(), run(f)); err != nil {
		t.Fatal(err)
	}
	files, _ := filepath.Glob(filepath.Join(dir, "issues", "*pulled.md"))
	if len(files) != 1 {
		t.Fatalf("no file rendered for the pulled issue")
	}
	edit(t, files[0], "pulled body", "pulled body, by hand")
	if _, err := b.AddComment(other, "a write elsewhere"); err != nil {
		t.Fatal(err)
	}
	list, err := b.ListIssues()
	if err != nil {
		t.Fatal(err)
	}
	for _, is := range list {
		if is.Title != "Pulled" {
			continue
		}
		th, err := b.Thread(is.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got := th.Events[0].Body; got != "pulled body, by hand" {
			t.Errorf("body %q, want the hand edit", got)
		}
	}
}
