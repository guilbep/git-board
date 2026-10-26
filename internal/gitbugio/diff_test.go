package gitbugio_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/gitbugio"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// diffRepo: dev, and fix/1-x off it with two commits (a change, a rename,
// a binary, a generated file; then a line on top and one at the end),
// dev one commit further.
type diffRepo struct {
	b              *gitbugio.Backend
	dir            string
	base, c1, head string
}

func newDiffRepo(t *testing.T) diffRepo {
	t.Helper()
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "dev")
	write(t, dir, "a.txt", "1\n2\n3\n4\n5\n")
	write(t, dir, "old.txt", strings.Repeat("the same line\n", 20))
	write(t, dir, ".gitattributes", "gen/* linguist-generated\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "start")
	base := git(t, dir, "rev-parse", "HEAD")

	git(t, dir, "switch", "-q", "-c", "fix/1-x")
	write(t, dir, "a.txt", "1\n2\nthree\n4\n5\n")
	git(t, dir, "mv", "old.txt", "new.txt")
	write(t, dir, "bin.dat", "\x00\x01\x02")
	write(t, dir, "gen/out.txt", strings.Repeat("x\n", 10))
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "c1")
	c1 := git(t, dir, "rev-parse", "HEAD")
	write(t, dir, "a.txt", "zero\n1\n2\nthree\n4\n5\n6\n")
	git(t, dir, "commit", "-q", "-am", "c2")
	head := git(t, dir, "rev-parse", "HEAD")

	git(t, dir, "switch", "-q", "dev")
	write(t, dir, "other.txt", "o\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "dev moves")

	b, err := gitbugio.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	return diffRepo{b, dir, base, c1, head}
}

func TestMergeBaseAndRevParse(t *testing.T) {
	t.Parallel()
	r := newDiffRepo(t)
	if got, err := r.b.MergeBase("dev", "fix/1-x"); err != nil || got != r.base {
		t.Fatalf("MergeBase = %q, %v; want %q", got, err, r.base)
	}
	if got, err := r.b.RevParse("fix/1-x"); err != nil || got != r.head {
		t.Fatalf("RevParse = %q, %v; want %q", got, err, r.head)
	}
	if got, err := r.b.RevParse(r.head[:7]); err != nil || got != r.head {
		t.Fatalf("RevParse(short) = %q, %v; want the full SHA", got, err)
	}
	if _, err := r.b.RevParse("--output=x"); err == nil {
		t.Fatal("RevParse took an option for a revision")
	}
	if _, err := r.b.RevParse("nope"); err == nil {
		t.Fatal("RevParse found a revision that doesn't exist")
	}
}

func TestDiffStat(t *testing.T) {
	t.Parallel()
	r := newDiffRepo(t)
	files, err := r.b.DiffStat(r.base, r.head)
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]gitbugio.FileStat{}
	for _, f := range files {
		by[f.Path] = f
	}
	if len(by) != 4 {
		t.Fatalf("files = %+v, want a.txt, new.txt, bin.dat, gen/out.txt", files)
	}
	if a := by["a.txt"]; a.Added != 3 || a.Deleted != 1 || a.Blob != git(t, r.dir, "rev-parse", r.head+":a.txt") {
		t.Errorf("a.txt = %+v", a)
	}
	if n := by["new.txt"]; n.From != "old.txt" || n.Added != 0 || n.Deleted != 0 {
		t.Errorf("new.txt = %+v, want renamed from old.txt", n)
	}
	if bin := by["bin.dat"]; !bin.Binary {
		t.Errorf("bin.dat = %+v, want binary", bin)
	}
	if g := by["gen/out.txt"]; !g.Generated || g.Added != 10 {
		t.Errorf("gen/out.txt = %+v, want generated, 10 added", g)
	}
	if by["a.txt"].Generated {
		t.Error("a.txt marked generated")
	}
}

func TestDiffFile(t *testing.T) {
	t.Parallel()
	r := newDiffRepo(t)
	full, truncated, err := r.b.DiffFile(r.base, r.head, 0, "a.txt")
	if err != nil || truncated || !strings.Contains(full, "+three") || !strings.Contains(full, "-3") {
		t.Fatalf("DiffFile = %q, %v, %v", full, truncated, err)
	}
	cut, truncated, err := r.b.DiffFile(r.base, r.head, 3, "a.txt")
	if err != nil || !truncated || strings.Count(cut, "\n") != 3 {
		t.Fatalf("DiffFile(limit 3) = %q, %v, %v; want 3 lines, truncated", cut, truncated, err)
	}
	renamed, _, err := r.b.DiffFile(r.base, r.head, 0, "new.txt", "old.txt")
	if err != nil || !strings.Contains(renamed, "rename from old.txt") {
		t.Fatalf("DiffFile(rename) = %q, %v", renamed, err)
	}
}

func TestLog(t *testing.T) {
	t.Parallel()
	r := newDiffRepo(t)
	log, err := r.b.Log(r.base, r.head)
	if err != nil {
		t.Fatal(err)
	}
	var subjects []string
	for _, c := range log {
		subjects = append(subjects, c.Subject)
	}
	if !slices.Equal(subjects, []string{"c2", "c1"}) || log[0].SHA != r.head || log[0].Author != "dev" || log[0].Date.IsZero() {
		t.Fatalf("Log = %+v, want c2 then c1", log)
	}
}

func TestFileLinesAndHasCommit(t *testing.T) {
	t.Parallel()
	r := newDiffRepo(t)
	lines, err := r.b.FileLines(r.head, "a.txt")
	if err != nil || !slices.Equal(lines, []string{"zero", "1", "2", "three", "4", "5", "6"}) {
		t.Fatalf("FileLines = %q, %v", lines, err)
	}
	if !r.b.HasCommit(r.c1) {
		t.Error("HasCommit(c1) = false")
	}
	if r.b.HasCommit(strings.Repeat("0", 40)) {
		t.Error("HasCommit(0000…) = true")
	}
}

func TestLineAt(t *testing.T) {
	t.Parallel()
	r := newDiffRepo(t)
	for _, c := range []struct {
		from, path string
		line, want int
		ok         bool
	}{
		{r.base, "a.txt", 2, 3, true},    // a line on top moves it down
		{r.base, "a.txt", 3, 0, false},   // changed: outdated
		{r.base, "a.txt", 5, 6, true},    // below the change, and the line on top
		{r.c1, "a.txt", 4, 5, true},      // "4" at c1 is line 5 at head
		{r.c1, "a.txt", 5, 6, true},      // the line added at the end is below it
		{r.head, "a.txt", 4, 4, true},    // no change
		{r.base, "old.txt", 1, 0, false}, // gone at head (renamed)
	} {
		got, ok, err := r.b.LineAt(c.from, r.head, c.path, c.line)
		if err != nil || got != c.want || ok != c.ok {
			t.Errorf("LineAt(%s, %s:%d) = %d, %v, %v; want %d, %v", c.from[:7], c.path, c.line, got, ok, err, c.want, c.ok)
		}
	}
}

// A path is a file's name, never a pattern (app/[id] matches app/i as a
// glob), and a textconv filter of the repository's never rewrites it.
func TestDiffPathsAsWritten(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "dev")
	git(t, dir, "config", "diff.upper.textconv", "sed s/l/L/g")
	write(t, dir, ".gitattributes", "*.txt diff=upper\n")
	write(t, dir, "app/[id]/page.tsx", "1\n2\n")
	write(t, dir, "app/i/page.tsx", "1\n2\n")
	write(t, dir, "t.txt", "low\n")
	git(t, dir, "add", ".")
	git(t, dir, "commit", "-q", "-m", "start")
	from := git(t, dir, "rev-parse", "HEAD")
	write(t, dir, "app/i/page.tsx", "one\n2\n")
	write(t, dir, "t.txt", "lower\n")
	git(t, dir, "commit", "-q", "-am", "c1")
	to := git(t, dir, "rev-parse", "HEAD")
	b, err := gitbugio.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })

	if d, _, err := b.DiffFile(from, to, 0, "app/[id]/page.tsx"); err != nil || d != "" {
		t.Errorf("DiffFile(app/[id]/page.tsx) = %q, %v; want nothing, it didn't change", d, err)
	}
	if _, ok, err := b.LineAt(from, to, "app/[id]/page.tsx", 1); err != nil || !ok {
		t.Errorf("LineAt(app/[id]/page.tsx:1) outdated by app/i's change (%v)", err)
	}
	t.Chdir(dir) // where git reads .gitattributes from, as the server runs
	if d, _, err := b.DiffFile(from, to, 0, "t.txt"); err != nil || !strings.Contains(d, "+lower") {
		t.Errorf("DiffFile(t.txt) = %q, %v; want the file's own text", d, err)
	}
}
