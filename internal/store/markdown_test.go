package store_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/model"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

var mdComment = regexp.MustCompile(`(?s)### (.+?) · (\S+)\n\n(.*?)\n*<!-- git-board comment (\S+) -->\n`)

func who(p model.Person) string {
	for _, s := range []string{p.Name, p.Login} {
		if s != "" {
			return s
		}
	}
	return p.ID
}

// TestExportMarkdownReadsBack exports a board, reads the folder back and
// finds every issue, comment and field; a second run writes nothing.
func TestExportMarkdownReadsBack(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	a, err := b.NewIssue("Export CSV: à la carte!", "the **body**", []string{"status:ready", "prio:p1", "assignee:alice", "needs-triage", "bug"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddComment(a, "first"); err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddComment(a, "second\n\nwith two paragraphs"); err != nil {
		t.Fatal(err)
	}
	c, err := b.NewIssue("Export CSV: à la carte!", "", nil) // same name
	if err != nil {
		t.Fatal(err)
	}
	if err := b.SetStatus(c, "closed"); err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "notes.md"), []byte("mine\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	flat := func(l string) bool { return l == "bug" }
	written, err := b.ExportMarkdown(dir, flat)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) != 2 {
		t.Fatalf("written %v, want 2 files", written)
	}

	issues, err := b.ListIssues()
	if err != nil {
		t.Fatal(err)
	}
	read := map[string]string{} // id -> body
	for _, name := range written {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		f, body, err := store.ParseMarkdown(data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		i := slices.IndexFunc(issues, func(is model.Issue) bool { return is.ID == f.ID })
		if i < 0 {
			t.Fatalf("%s: id %s isn't an issue", name, f.ID)
		}
		is := issues[i]
		if f.Title != is.Title || f.State != is.Status || !f.Updated.Equal(is.UpdatedAt.Truncate(time.Second)) {
			t.Errorf("%s: front %+v, issue %+v", name, f, is)
		}
		read[f.ID] = body
		if f.ID != a {
			continue
		}
		if !strings.HasPrefix(name, a[:7]+"-export-csv-la-carte") {
			t.Errorf("name %s", name)
		}
		want := map[string][]string{"status": {"ready"}, "prio": {"p1"}, "kind": {"bug"}}
		for k, v := range want {
			if !slices.Equal(f.Fields[k], v) {
				t.Errorf("fields %v, want %v", f.Fields, want)
			}
		}
		if len(f.Fields) != len(want) || !slices.Equal(f.Labels, []string{"needs-triage"}) || !slices.Equal(f.People.Assignees, []string{"alice"}) {
			t.Errorf("fields %v labels %v assignees %v", f.Fields, f.Labels, f.People.Assignees)
		}
	}
	if len(read) != 2 {
		t.Fatalf("read %d issues, want 2", len(read))
	}

	th, err := b.Thread(a)
	if err != nil {
		t.Fatal(err)
	}
	// the description ends at the comments' line
	if desc, _, _ := strings.Cut(read[a], "\n<!-- git-board comments -->"); strings.TrimSpace(desc) != "the **body**" {
		t.Errorf("description %q", desc)
	}
	got := mdComment.FindAllStringSubmatch(read[a], -1)
	var comments []model.Event
	for _, e := range th.Events {
		if e.Kind == "comment" {
			comments = append(comments, e)
		}
	}
	if len(got) != len(comments) {
		t.Fatalf("read %d comments, want %d:\n%s", len(got), len(comments), read[a])
	}
	for i, e := range comments {
		at := e.At.UTC().Format(time.RFC3339)
		if g := got[i]; g[1] != who(e.Author) || g[2] != at || g[3] != e.Body || g[4] != e.ID {
			t.Errorf("comment %d read %q, want %s %s %q %s", i, g[1:], who(e.Author), at, e.Body, e.ID)
		}
	}

	if written, err := b.ExportMarkdown(dir, flat); err != nil || len(written) != 0 {
		t.Fatalf("second run wrote %v, %v", written, err)
	}

	// A new title: the file moves, the old one goes, ours stays.
	if err := b.SetTitle(c, "Renamed"); err != nil {
		t.Fatal(err)
	}
	if written, err := b.ExportMarkdown(dir, flat); err != nil || len(written) != 1 || !strings.HasSuffix(written[0], "-renamed.md") {
		t.Fatalf("after rename wrote %v, %v", written, err)
	}
	left, _ := filepath.Glob(filepath.Join(dir, "*.md"))
	if len(left) != 3 {
		t.Errorf("files %v, want both issues and notes.md", left)
	}
}
