package store_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// exported is the one file an export of b wrote to a new folder.
func exported(t *testing.T, b *store.Backend) (dir, path string) {
	t.Helper()
	dir = t.TempDir()
	written, err := b.ExportMarkdown(dir, nil)
	if err != nil || len(written) != 1 {
		t.Fatalf("export: %v, %v", written, err)
	}
	return dir, filepath.Join(dir, written[0])
}

func edit(t *testing.T, path string, pairs ...string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for i := 0; i < len(pairs); i += 2 {
		if !strings.Contains(s, pairs[i]) {
			t.Fatalf("%s holds no %q:\n%s", path, pairs[i], s)
		}
		s = strings.Replace(s, pairs[i], pairs[i+1], 1)
	}
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func eventsOf(t *testing.T, b *store.Backend, id string) []store.Event {
	t.Helper()
	th, err := b.Thread(id)
	if err != nil {
		t.Fatal(err)
	}
	return th.Events
}

func bodies(evs []store.Event, kind string) []string {
	var out []string
	for _, e := range evs {
		if e.Kind == kind {
			out = append(out, e.Body)
		}
	}
	return out
}

// Edited by hand, a file's changes come back as the issue's operations;
// a second import finds nothing new.
func TestImportMarkdownEdits(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	id, err := b.NewIssue("Export CSV", "the body", []string{"prio:p2", "needs-triage"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddComment(id, "first"); err != nil {
		t.Fatal(err)
	}
	dir, path := exported(t, b)
	edit(t, path,
		"title: Export CSV", "title: Export CSV and TSV",
		"the body", "the body, edited",
		"state: open", "state: closed",
		"p2", "p1",
		"needs-triage", "ready",
		"first\n", "first, edited\n",
	)
	data, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(data, []byte("\n### dev · 2026-11-04T10:00:00Z\n\nwritten by hand\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	rep, err := b.ImportMarkdown(dir)
	if err != nil || len(rep.Updated) != 1 || len(rep.Conflicts) != 0 || len(rep.Created) != 0 {
		t.Fatalf("import: %+v, %v", rep, err)
	}
	is, err := b.GetIssue(id)
	if err != nil {
		t.Fatal(err)
	}
	if is.Title != "Export CSV and TSV" || is.Status != "closed" || !slices.Contains(is.Labels, "prio:p1") || !slices.Contains(is.Labels, "ready") || slices.Contains(is.Labels, "needs-triage") {
		t.Fatalf("issue %+v", is)
	}
	evs := eventsOf(t, b, id)
	if got := bodies(evs, "created"); len(got) != 1 || got[0] != "the body, edited" {
		t.Fatalf("description %q", got)
	}
	if got := bodies(evs, "comment"); !slices.Equal(got, []string{"first, edited", "written by hand"}) {
		t.Fatalf("comments %q", got)
	}

	// the folder written again holds what was imported: nothing more to read
	if _, err := b.ExportMarkdown(dir, nil); err != nil {
		t.Fatal(err)
	}
	if rep, err := b.ImportMarkdown(dir); err != nil || rep.Unchanged != 1 || len(rep.Updated) != 0 {
		t.Fatalf("again: %+v, %v", rep, err)
	}
}

// A field the file changes on an issue that changed since its export is a
// conflict, the board's value kept; a new comment goes in anyway.
func TestImportMarkdownConflict(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	id, err := b.NewIssue("Before", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	dir, path := exported(t, b)
	time.Sleep(1100 * time.Millisecond) // updated is in seconds
	if err := b.SetTitle(id, "Changed here"); err != nil {
		t.Fatal(err)
	}
	edit(t, path, "title: Before", "title: Changed there")
	data, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(data, []byte("\n### dev · 2026-11-04T10:00:00Z\n\nstill goes in\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := b.ImportMarkdown(dir)
	if err != nil || len(rep.Conflicts) != 1 || !strings.Contains(rep.Conflicts[0], "title") {
		t.Fatalf("import: %+v, %v", rep, err)
	}
	if is, _ := b.GetIssue(id); is.Title != "Changed here" {
		t.Fatalf("title %q, want the board's", is.Title)
	}
	if got := bodies(eventsOf(t, b, id), "comment"); !slices.Equal(got, []string{"still goes in"}) {
		t.Fatalf("comments %q", got)
	}
}

// A file with no id is a new issue as it says, people made by name; its
// new id is written back, so a second import makes nothing. A board that
// never had an issue reads another's export the same way.
func TestImportMarkdownNew(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	dir := t.TempDir()
	path := filepath.Join(dir, "new-one.md")
	file := `---
title: Written by hand
state: open
fields:
  prio: [p1]
labels: [docs]
people:
  author: Alice
  assignees: [bob]
created: 2026-10-01T09:00:00Z
updated: 2026-10-01T09:00:00Z
---

What it is about.

### Carol · 2026-10-02T09:00:00Z

A first comment.
`
	if err := os.WriteFile(path, []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# not an issue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rep, err := b.ImportMarkdown(dir)
	if err != nil || len(rep.Created) != 1 || len(rep.Skipped) != 1 {
		t.Fatalf("import: %+v, %v", rep, err)
	}
	issues, err := b.ListIssues()
	if err != nil || len(issues) != 1 {
		t.Fatalf("issues %+v, %v", issues, err)
	}
	is := issues[0]
	th, _ := b.Thread(is.ID)
	if is.Title != "Written by hand" || th.Author.Name != "Alice" || !is.CreatedAt.Equal(time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)) ||
		!slices.Contains(is.Labels, "prio:p1") || !slices.Contains(is.Labels, "docs") || !slices.Contains(is.Labels, "assignee:bob") {
		t.Fatalf("issue %+v by %+v", is, th.Author)
	}
	if evs := eventsOf(t, b, is.ID); evs[len(evs)-1].Author.Name != "Carol" || evs[len(evs)-1].Body != "A first comment." {
		t.Fatalf("comment %+v", evs[len(evs)-1])
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "id: "+is.ID) {
		t.Fatalf("the id isn't written back:\n%s", data)
	}
	if rep, err := b.ImportMarkdown(dir); err != nil || len(rep.Created) != 0 || rep.Unchanged != 1 {
		t.Fatalf("again: %+v, %v", rep, err)
	}

	// another board reads this folder: the same issue, made there
	other := storetest.New(t)
	if rep, err := other.ImportMarkdown(dir); err != nil || len(rep.Created) != 1 {
		t.Fatalf("other board: %+v, %v", rep, err)
	}
}
