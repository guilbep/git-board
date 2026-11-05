package golden

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestMarkdownFiles: a Markdown folder's files are the same whichever
// way they came: what the binary renders after its writes, one file at a
// time, is what it renders of the board they left from no file; byte for
// byte, but for the last write's own times and ids. (Until v0.12 v1
// rendered the other side, and v2's writes v1's files.)
func TestMarkdownFiles(t *testing.T) {
	t.Parallel()
	bin := binary(t)
	board := markdownBoard(t)
	ctx := context.Background()
	dir, home, _ := checkout(t, board)
	for _, line := range mdWrites {
		if _, err := Run(ctx, bin, dir, home, Case{Line: line}); err != nil {
			t.Fatal(err)
		}
	}
	again := copyCheckout(t, dir)
	for name := range mdFiles(t, again) {
		os.Remove(filepath.Join(again, "issues", name))
	}
	// one write more on each: the first renders what it wrote, the
	// other every file, from none
	for _, at := range []string{dir, again} {
		if r, err := Run(ctx, bin, at, home, Case{Line: renderAll}); err != nil || r.Exit != 0 {
			t.Fatalf("%s: exit %d, %v\n%s", renderAll, r.Exit, err, r.Stderr)
		}
	}
	sameFiles(t, mdFiles(t, dir), mdFiles(t, again), unstamped)
}

// renderAll is a write: it reads the files in first, and renders them
// last.
const renderAll = `comment c82cfa3 -b "the files rendered"`

// TestMarkdownHandEdits: hand edits of a Markdown folder's files are
// read in by the next write as v1 read them in: the same operations
// (their times and ids aside, a write's own), the same conflicts told,
// the same files after.
func TestMarkdownHandEdits(t *testing.T) {
	t.Parallel()
	bin := binary(t)
	board := markdownBoard(t)
	ctx := context.Background()
	dir, home, _ := checkout(t, board)
	handEdits(t, filepath.Join(dir, "issues"))
	r, err := Run(ctx, bin, dir, home, Case{Line: `comment fe82974 -b "after the hand edits"`})
	if err != nil {
		t.Fatal(err)
	}
	var tr transcript
	tr.add(`comment fe82974 -b "after the hand edits"`, Result{Exit: r.Exit, Stderr: r.Stderr})
	ids, err := board.IssueIDs(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 4 {
		t.Fatalf("%d issues, want the hand's new one too", len(ids))
	}
	shows := map[string][]byte{}
	for _, id := range ids {
		out, err := Run(ctx, bin, dir, home, Case{Line: "show " + id + " --json"})
		if err != nil {
			t.Fatal(err)
		}
		if out.Exit != 0 { // deleted
			continue
		}
		title := regexp.MustCompile(`"title": "([^"]*)"`).FindSubmatch(out.Stdout)
		if title == nil {
			t.Fatalf("show %s: no title:\n%s", id, out.Stdout)
		}
		shows[string(title[1])] = unstamped(canon(decode(out.Stdout)))
	}
	for _, title := range mapKeys(shows) {
		tr.add("show "+title, Result{Stdout: shows[title]})
	}
	files := map[string][]byte{}
	for name, data := range mdFiles(t, dir) {
		files[string(unstamped([]byte(name)))] = unstamped(data)
	}
	for _, name := range mapKeys(files) {
		tr.file(name, files[name])
	}
	tr.check(t, "md-hand-edits")
}

// handEdits edits the golden Markdown board's files as a hand does: a
// title, a description, a comment, fields and labels, new comments, one
// edit on a file older than its issue (a conflict), and a new file.
func handEdits(t *testing.T, dir string) {
	t.Helper()
	first := filepath.Join(dir, "1-kept-in-a-folder-merged-by-union.md")
	replace(t, first,
		"title: Kept in a folder, merged by union", "title: Kept in a folder, by hand",
		"    - p1", "    - p0",
		"  - plain", "  - plain\n  - extra",
		"still the body.", "still the body, by hand.",
		"A comment.", "A comment, by hand.")
	appendTo(t, first, "\n### Dana · 2025-04-01T08:00:00Z\n\nA new comment, by a new person.\n\n### Bob · 2025-04-01T09:00:00Z\n\nAnd one by Bob.\n")
	second := filepath.Join(dir, "2-n-c-d.md")
	replace(t, second, "state: closed", "state: open", "updated: 2025-03-03T11:05:00Z", "updated: 2025-03-03T11:00:00Z", "\n\n<!-- git-board comments -->", "\n\nA line, by hand.\n\n<!-- git-board comments -->")
	appendTo(t, second, "\n### ada-l · 2025-04-02T08:00:00Z\n\nRead in, in a conflict too.\n")
	draft := "---\ntitle: A draft, by hand\nstate: open\nfields:\n  prio:\n    - p2\nlabels:\n  - plain\npeople:\n  author: Bob\n  assignees:\n    - ada\ncreated: 2025-04-03T08:00:00Z\nupdated: 2025-04-03T08:00:00Z\n---\n\nIts body.\n\n### Chloé Dubois-Ångström · 2025-04-03T09:00:00Z\n\nIts comment.\n"
	if err := os.WriteFile(filepath.Join(dir, "draft.md"), []byte(draft), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Not an issue\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func replace(t *testing.T, path string, pairs ...string) {
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

func appendTo(t *testing.T, path, text string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, append(data, text...), 0o644); err != nil {
		t.Fatal(err)
	}
}

func markdownBoard(t *testing.T) Board {
	t.Helper()
	for _, b := range boards(t) {
		if b.Name == "markdown" {
			return b
		}
	}
	t.Fatal("no markdown golden board")
	return Board{}
}

// copyCheckout copies a checkout, its git directory too, beside it.
func copyCheckout(t *testing.T, dir string) string {
	t.Helper()
	to := filepath.Join(t.TempDir(), "repo")
	if out, err := exec.Command("cp", "-a", dir, to).CombinedOutput(); err != nil {
		t.Fatalf("cp: %v\n%s", err, out)
	}
	return to
}

// mdFiles are the *.md files of a checkout's Markdown folder, by name.
func mdFiles(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "issues", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string][]byte{}
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out[filepath.Base(p)] = data
	}
	return out
}

// sameFiles compares two folders' files, each name and bytes through norm.
func sameFiles(t *testing.T, want, got map[string][]byte, norm func([]byte) []byte) {
	t.Helper()
	names := func(m map[string][]byte) []string {
		var out []string
		for n := range m {
			out = append(out, string(norm([]byte(n))))
		}
		slices.Sort(out)
		return out
	}
	if a, b := names(want), names(got); !slices.Equal(a, b) {
		t.Fatalf("the files: %v, then %v", a, b)
	}
	byNorm := map[string][]byte{}
	for n, data := range got {
		byNorm[string(norm([]byte(n)))] = data
	}
	for n, data := range want {
		if a, b := norm(data), norm(byNorm[string(norm([]byte(n)))]); !bytes.Equal(a, b) {
			t.Errorf("%s differs:\n%s", n, Diff(a, b))
		}
	}
}

// unstamped is b without what a write stamps: a time of this year (its
// operations'), an id (ids are hashes of packs with times in them).
func unstamped(b []byte) []byte {
	b = regexp.MustCompile(fmt.Sprintf(`%d-\d\d-\d\dT\d\d:\d\d:\d\dZ`, time.Now().UTC().Year())).ReplaceAll(b, []byte("NOW"))
	return shortIDHex.ReplaceAll(b, []byte("ID"))
}

func mapKeys[V any](m map[string]V) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// TestMarkdownCopy: a board copied from git into a Markdown folder
// renders the files v1 rendered, byte for byte: the generated board
// (basic holds a pack a folder refuses, as v1 does).
func TestMarkdownCopy(t *testing.T) {
	t.Parallel()
	bin := binary(t)
	var from Board
	for _, b := range boards(t) {
		if b.Name == "generated" {
			from = b
		}
	}
	ctx := context.Background()
	dir, home, _ := checkout(t, from)
	if err := Git(ctx, dir, "config", "board.datastore", "markdown:issues"); err != nil {
		t.Fatal(err)
	}
	r, err := Run(ctx, bin, dir, home, Case{Line: "datastore copy"})
	if err != nil || r.Exit != 0 {
		t.Fatalf("datastore copy: exit %d, %v\n%s", r.Exit, err, r.Stderr)
	}
	files := mdFiles(t, dir)
	if len(files) < 10 {
		t.Fatalf("%d files rendered: want the generated board's", len(files))
	}
	var tr transcript
	for _, name := range mapKeys(files) {
		tr.file(name, files[name])
	}
	tr.check(t, "md-copy")
}
