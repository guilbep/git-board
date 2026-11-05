package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// A deleted comment leaves the thread and search; a deleted issue leaves
// lists, search and lookups, and is a not found.
func TestDelete(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "Kept")
	secret, err := b.AddComment(id, "password=hunter2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddComment(id, "fine"); err != nil {
		t.Fatal(err)
	}
	if err := b.DeleteComment(id, secret); err != nil {
		t.Fatal(err)
	}
	th, err := b.Thread(id)
	if err != nil {
		t.Fatal(err)
	}
	var bodies []string
	for _, e := range th.Events {
		if e.Kind == "comment" {
			bodies = append(bodies, e.Body)
		}
	}
	if !slices.Equal(bodies, []string{"fine"}) {
		t.Errorf("comments = %q", bodies)
	}
	if hits, _ := b.SearchText("hunter2"); len(hits) != 0 {
		t.Errorf("search finds the deleted comment: %v", hits)
	}
	if err := b.DeleteComment(id, secret); !errors.Is(err, store.ErrNoSuchComment) {
		t.Errorf("deleting it again = %v, want no such comment", err)
	}
	if err := b.DeleteComment(id, th.Events[0].ID); !errors.Is(err, store.ErrNoSuchComment) {
		t.Errorf("deleting the description = %v, want no such comment", err)
	}
	if err := b.EditComment(id, secret, "back", ""); !errors.Is(err, store.ErrNoSuchComment) {
		t.Errorf("editing the deleted comment = %v, want no such comment", err)
	}

	gone := storetest.NewIssue(t, b, "Gone")
	whole, err := b.DeleteIssue(gone[:8])
	if err != nil || whole != gone {
		t.Fatalf("delete issue = %q, %v", whole, err)
	}
	if _, err := b.GetIssue(gone); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("get the deleted issue = %v, want not found", err)
	}
	issues, err := b.ListIssues()
	if err != nil || len(issues) != 1 || issues[0].ID != id {
		t.Errorf("issues = %+v, %v", issues, err)
	}
	if _, err := b.DeleteIssue(gone); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("deleting it again = %v, want not found", err)
	}
}

// On a Markdown board, a deleted issue's file goes; brought back (from a
// branch), it is skipped, not imported as a new issue.
func TestDeleteOnMarkdownBoard(t *testing.T) {
	t.Parallel()
	dir := storetest.Repo(t, false)
	storetest.Init(t, dir)
	storetest.Git(t, dir, "config", store.DatastoreKey, "markdown:issues")
	b := storetest.Open(t, dir)
	id, err := b.NewIssue("Leaked", "token", nil)
	if err != nil {
		t.Fatal(err)
	}
	folder := filepath.Join(dir, "issues")
	files, _ := filepath.Glob(filepath.Join(folder, "*.md"))
	if len(files) != 1 {
		t.Fatalf("files = %v", files)
	}
	data, err := os.ReadFile(files[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.DeleteIssue(id); err != nil {
		t.Fatal(err)
	}
	if left, _ := filepath.Glob(filepath.Join(folder, "*.md")); len(left) != 0 {
		t.Fatalf("files after the delete = %v", left)
	}
	if err := os.WriteFile(files[0], data, 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := b.ImportMarkdown(folder)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Created) != 0 || len(rep.Skipped) != 1 {
		t.Errorf("import = %+v, want the file skipped", rep)
	}
	if issues, _ := b.ListIssues(); len(issues) != 0 {
		t.Errorf("issues = %+v", issues)
	}
}
