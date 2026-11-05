package mdfiles_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/issues/mdfiles"
	"github.com/guilbep/git-board/next/domain/people"
	"github.com/guilbep/git-board/next/kernel"
	"github.com/guilbep/git-board/next/store/md"
	"github.com/guilbep/git-board/next/view"
)

var ctx = context.Background()

// board is a Markdown folder's board in a temporary folder, with Ada
// writing, and its files.
type board struct {
	dir     string
	svc     *issues.Service
	files   *mdfiles.Folder
	mu      sync.Mutex
	notices []string
}

func open(t *testing.T, dir, folder string, me kernel.ID) *board {
	t.Helper()
	ds, err := md.Open(dir, folder)
	if err != nil {
		t.Fatal(err)
	}
	v, err := view.Open(ctx, filepath.Join(t.TempDir(), "view.db"))
	if err != nil {
		t.Fatal(err)
	}
	kb, err := kernel.Open(ctx, ds, v, issues.Kind, people.Kind)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { kb.Close() })
	b := &board{dir: dir, svc: &issues.Service{Board: kb, Schema: &issues.Schema{}, Me: me}}
	b.files = mdfiles.Open(ds.Dir(), folder, ds.LockFiles, b.svc, func(format string, args ...any) {
		b.mu.Lock()
		defer b.mu.Unlock()
		b.notices = append(b.notices, strings.TrimSpace(fmtf(format, args...)))
	})
	return b
}

func newBoard(t *testing.T) *board {
	t.Helper()
	dir, folder := filepath.Join(t.TempDir(), "issues"), t.TempDir()
	b := open(t, dir, folder, "")
	data, _ := json.Marshal(map[string]string{"name": "Ada King", "email": "ada@example.com"})
	p, err := people.Kind.Create(ctx, b.svc.Board, "", kernel.Op{Kind: "create", Time: 1, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	b.svc.Me = p.ID
	return b
}

// file is the one *.md file whose name holds part.
func (b *board) file(t *testing.T, part string) (string, string) {
	t.Helper()
	paths, _ := filepath.Glob(filepath.Join(b.dir, "*.md"))
	var found []string
	for _, p := range paths {
		if strings.Contains(filepath.Base(p), part) {
			found = append(found, p)
		}
	}
	if len(found) != 1 {
		t.Fatalf("files %v: want one with %q", paths, part)
	}
	data, err := os.ReadFile(found[0])
	if err != nil {
		t.Fatal(err)
	}
	return found[0], string(data)
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

func TestRenderAndReadBack(t *testing.T) {
	t.Parallel()
	b := newBoard(t)
	one, err := b.svc.New(ctx, issues.NewReq{Title: "First one", Body: "the body"})
	if err != nil {
		t.Fatal(err)
	}
	two, err := b.svc.New(ctx, issues.NewReq{Title: "Second / one!", Assign: []string{"prio=p1"}, Force: true})
	if err != nil {
		t.Fatal(err)
	}
	path, text := b.file(t, "first-one")
	if want := one.ID[:7] + "-first-one.md"; filepath.Base(path) != want {
		t.Fatalf("named %s, want %s", filepath.Base(path), want)
	}
	for _, want := range []string{"id: " + one.ID + "\n", "title: First one\n", "state: open\n", "  author: Ada King\n", "\nthe body\n\n<!-- git-board comments -->\n"} {
		if !strings.Contains(text, want) {
			t.Fatalf("no %q in\n%s", want, text)
		}
	}
	if _, text := b.file(t, "second-one"); !strings.Contains(text, "fields:\n  prio:\n    - p1\n") {
		t.Fatalf("the field:\n%s", text)
	}

	// a hand edit: read in by the next write, to another issue
	edit(t, path, "title: First one", "title: First, by hand", "the body", "the body, by hand", "  author: Ada King\n", "  author: Ada King\n  assignees:\n    - bob\n")
	if err := os.WriteFile(path, append(mustRead(t, path), "\n### Bob · 2025-01-02T03:04:05Z\n\nA comment by hand.\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.svc.Comment(ctx, two.ID, "a write elsewhere"); err != nil {
		t.Fatal(err)
	}
	th, err := b.svc.Show(ctx, one.ID)
	if err != nil {
		t.Fatal(err)
	}
	if th.Title != "First, by hand" || th.Events[0].Body != "the body, by hand" || !slices.Contains(th.Labels, "assignee:bob") {
		t.Fatalf("after the hand edit: %q %q %v", th.Title, th.Events[0].Body, th.Labels)
	}
	if last := th.Events[slices.IndexFunc(th.Events, func(e issues.Event) bool { return e.Kind == "comment" })]; last.Kind != "comment" || last.Body != "A comment by hand." || last.Author.Name != "Bob" || last.At.Unix() != time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC).Unix() {
		t.Fatalf("the hand's comment: %+v", last)
	}
	// renamed: the old name goes
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("the old file: %v", err)
	}
	if _, text := b.file(t, "first-by-hand"); !strings.Contains(text, "### Bob · 2025-01-02T03:04:05Z\n\nA comment by hand.\n\n<!-- git-board comment ") {
		t.Fatalf("the file after:\n%s", text)
	}

	// a deleted issue's file goes
	if _, err := b.svc.Delete(ctx, two.ID); err != nil {
		t.Fatal(err)
	}
	if paths, _ := filepath.Glob(filepath.Join(b.dir, "*second*")); len(paths) != 0 {
		t.Fatalf("a deleted issue's file stays: %v", paths)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

// A file written by hand, without an id, is a new issue: its id is
// written in, and its file named as the render names it.
func TestNewFileByHand(t *testing.T) {
	t.Parallel()
	b := newBoard(t)
	one, err := b.svc.New(ctx, issues.NewReq{Title: "One"})
	if err != nil {
		t.Fatal(err)
	}
	hand := "---\ntitle: Written by hand\nstate: closed\nlabels:\n  - plain\npeople:\n  author: Chloé\ncreated: 2025-03-03T09:00:00Z\nupdated: 2025-03-03T10:00:00Z\n---\n\nIts body.\n"
	if err := os.WriteFile(filepath.Join(b.dir, "mine.md"), []byte(hand), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.dir, "README.md"), []byte("not an issue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.svc.Comment(ctx, one.ID, "a write"); err != nil {
		t.Fatal(err)
	}
	list, err := b.svc.List(ctx, issues.ListReq{Filter: "is:closed"})
	if err != nil || len(list) != 1 || list[0].Title != "Written by hand" || !slices.Equal(list[0].Labels, []string{"plain"}) || list[0].CreatedAt.Unix() != time.Date(2025, 3, 3, 9, 0, 0, 0, time.UTC).Unix() {
		t.Fatalf("the new issue: %+v, %v", list, err)
	}
	if _, err := os.Stat(filepath.Join(b.dir, "mine.md")); !os.IsNotExist(err) {
		t.Fatalf("the hand's file stays under its name: %v", err)
	}
	if _, text := b.file(t, "written-by-hand"); !strings.Contains(text, "id: "+list[0].ID) || !strings.Contains(text, "author: Chloé") {
		t.Fatalf("its file:\n%s", text)
	}
	if string(mustRead(t, filepath.Join(b.dir, "README.md"))) != "not an issue\n" {
		t.Fatal("a file that isn't an issue's was touched")
	}
}

// A hand edit of an issue changed here since its file was written is a
// conflict: the board's value stays, the file takes it back, and it is
// told. A new comment goes in either way.
func TestConflict(t *testing.T) {
	t.Parallel()
	b := newBoard(t)
	one, err := b.svc.New(ctx, issues.NewReq{Title: "One"})
	if err != nil {
		t.Fatal(err)
	}
	path, _ := b.file(t, "one")
	edit(t, path, "title: One", "title: One by hand", "updated: ", "updated: 2001-01-01T00:00:00Z\nx: ")
	if err := os.WriteFile(path, append(mustRead(t, path), "\n### Ada King · 2025-01-02T03:04:05Z\n\nStill read in.\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.svc.Comment(ctx, one.ID, "a write"); err != nil {
		t.Fatal(err)
	}
	th, err := b.svc.Show(ctx, one.ID)
	if err != nil || th.Title != "One" || len(th.Events) != 3 || th.Events[1].Body != "Still read in." {
		t.Fatalf("after: %+v, %v", th, err)
	}
	if want := "markdown: " + filepath.Base(path) + ": title: changed here since the file's updated, the board's kept"; !slices.Contains(b.notices, want) {
		t.Fatalf("notices %q, want %q", b.notices, want)
	}
	// "2001" alone may be in an id: any 4 hex digits are, 1 time in 600
	if _, text := b.file(t, "one"); !strings.Contains(text, "title: One\n") || strings.Contains(text, "2001-01-01") {
		t.Fatalf("the file after:\n%s", text)
	}
}

// A write reads only the files that changed since they were written or
// read; a process of its own too, from what the folder's state keeps.
func TestReadsOnlyWhatChanged(t *testing.T) {
	t.Parallel()
	b := newBoard(t)
	var ids []string
	for _, title := range []string{"a", "b", "c", "d"} {
		is, err := b.svc.New(ctx, issues.NewReq{Title: title})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, is.ID)
	}
	// the files just written are read once more, to be sure of their stat
	time.Sleep(1100 * time.Millisecond)
	if _, _, err := b.svc.Comment(ctx, ids[0], "settle"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	before := mdfiles.Parsed(b.files)
	if _, _, err := b.svc.Comment(ctx, ids[1], "one more"); err != nil {
		t.Fatal(err)
	}
	if n := mdfiles.Parsed(b.files) - before; n != 1 { // a's file, written by the settle
		t.Fatalf("read %d files, want 1", n)
	}
	path, _ := b.file(t, "-c.md")
	edit(t, path, "title: c", "title: c by hand")
	before = mdfiles.Parsed(b.files)
	if _, _, err := b.svc.Comment(ctx, ids[3], "and another"); err != nil {
		t.Fatal(err)
	}
	if n := mdfiles.Parsed(b.files) - before; n != 2 { // b's, and c's by hand
		t.Fatalf("read %d files, want 2", n)
	}
	if th, err := b.svc.Show(ctx, ids[2]); err != nil || th.Title != "c by hand" {
		t.Fatalf("c: %q, %v", th.Title, err)
	}
}

// A file whose mtime a tool set back (cp -p, rsync -t) changed just now
// still: it is read each write, until it has settled a second.
func TestMtimeSetBack(t *testing.T) {
	t.Parallel()
	b := newBoard(t)
	if _, err := b.svc.New(ctx, issues.NewReq{Title: "One"}); err != nil {
		t.Fatal(err)
	}
	two, err := b.svc.New(ctx, issues.NewReq{Title: "Two"})
	if err != nil {
		t.Fatal(err)
	}
	path, _ := b.file(t, "-one.md")
	old := time.Now().Add(-time.Hour)
	os.Chtimes(path, old, old)
	// one's, its stat moved, then kept; and two's, written just now, each
	// time: one's again the second time, as it changed less than a second
	// before it was kept
	for i := range 2 {
		before := mdfiles.Parsed(b.files)
		if _, _, err := b.svc.Comment(ctx, two.ID, "a write"); err != nil {
			t.Fatal(err)
		}
		if n := mdfiles.Parsed(b.files) - before; n != 2 {
			t.Fatalf("write %d read %d files, want 2", i+1, n)
		}
	}
}

func fmtf(format string, args ...any) string { return strings.TrimSpace(sprintf(format, args...)) }

var sprintf = fmt.Sprintf
