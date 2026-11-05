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
// version wrote: what v2 renders after its writes is what v1 renders of
// the board they left, from no file, and the reverse; byte for byte, but
// for the last write's own times and ids.
func TestMarkdownFiles(t *testing.T) {
	t.Parallel()
	one, two := binaries(t)
	board := markdownBoard(t)
	for _, c := range []struct{ name, writer, renderer string }{{"v2 writes", two, one}, {"v1 writes", one, two}} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			dir, home, _ := checkout(t, board)
			for _, line := range mdWrites {
				if _, err := Run(ctx, c.writer, dir, home, Case{Line: line}); err != nil {
					t.Fatal(err)
				}
			}
			again := copyCheckout(t, dir)
			for name := range mdFiles(t, again) {
				os.Remove(filepath.Join(again, "issues", name))
			}
			// one write more on each: the writer's renders what it wrote,
			// the other's every file, from none
			for _, w := range []struct{ bin, dir string }{{c.writer, dir}, {c.renderer, again}} {
				if r, err := Run(ctx, w.bin, w.dir, home, Case{Line: renderAll}); err != nil || r.Exit != 0 {
					t.Fatalf("%s: exit %d, %v\n%s", renderAll, r.Exit, err, r.Stderr)
				}
			}
			sameFiles(t, mdFiles(t, dir), mdFiles(t, again), unstamped)
		})
	}
}

// renderAll is a write: it reads the files in first, and renders them
// last.
const renderAll = `comment c82cfa3 -b "the files rendered"`

// TestMarkdownHandEdits: hand edits of a Markdown folder's files are
// read in by the next write the same way by v1 and v2: the same
// operations (their times and ids aside, a write's own), the same
// conflicts told, the same files after; and what one read in, the other
// reads the same.
func TestMarkdownHandEdits(t *testing.T) {
	t.Parallel()
	one, two := binaries(t)
	board := markdownBoard(t)
	type after struct {
		write Result
		files map[string][]byte
		shows map[string][]byte
	}
	got := map[string]after{}
	for _, w := range []struct{ name, bin string }{{"v1", one}, {"v2", two}} {
		ctx := context.Background()
		dir, home, _ := checkout(t, board)
		handEdits(t, filepath.Join(dir, "issues"))
		r, err := Run(ctx, w.bin, dir, home, Case{Line: `comment fe82974 -b "after the hand edits"`})
		if err != nil {
			t.Fatal(err)
		}
		a := after{write: r, files: mdFiles(t, dir), shows: map[string][]byte{}}
		ids, err := board.IssueIDs(ctx, dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(ids) != 4 {
			t.Fatalf("%s: %d issues, want the hand's new one too", w.name, len(ids))
		}
		for _, id := range ids {
			var outs [2]Result
			for i, bin := range []string{one, two} {
				if outs[i], err = Run(ctx, bin, dir, home, Case{Line: "show " + id + " --json"}); err != nil {
					t.Fatal(err)
				}
			}
			if outs[0].Exit != 0 && outs[0].Exit == outs[1].Exit { // deleted
				continue
			}
			if outs[0].Exit != outs[1].Exit || !jsonEqual(outs[0].Stdout, outs[1].Stdout) {
				t.Errorf("after %s read the hand edits in, show %.7s: v1 and v2 differ:\n%s", w.name, id, Diff(outs[0].Stdout, outs[1].Stdout))
			}
			title := regexp.MustCompile(`"title": "([^"]*)"`).FindSubmatch(outs[0].Stdout)
			if title == nil {
				t.Fatalf("show %s: no title:\n%s", id, outs[0].Stdout)
			}
			a.shows[string(title[1])] = canon(decode(outs[1].Stdout))
		}
		got[w.name] = a
	}
	v1, v2 := got["v1"], got["v2"]
	if v1.write.Exit != v2.write.Exit || !bytes.Equal(v1.write.Stderr, v2.write.Stderr) {
		t.Errorf("the write: v1 and v2 differ\nexit %d / %d\nstderr:\n%s", v1.write.Exit, v2.write.Exit, Diff(v1.write.Stderr, v2.write.Stderr))
	}
	if a, b := mapKeys(v1.shows), mapKeys(v2.shows); !slices.Equal(a, b) {
		t.Fatalf("the issues: v1 %v, v2 %v", a, b)
	}
	for title, s := range v1.shows {
		if a, b := unstamped(s), unstamped(v2.shows[title]); !bytes.Equal(a, b) {
			t.Errorf("%q, as v1 and v2 read the hand edits in:\n%s", title, Diff(a, b))
		}
	}
	sameFiles(t, v1.files, v2.files, unstamped)
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

// packFiles are a checkout's Markdown folder's pack files, as one
// string.
func packFiles(t *testing.T, dir string) string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(dir, "issues", ".board", "*", "*.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&b, "%s\n%s\n", p, data)
	}
	return b.String()
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
		t.Fatalf("the files: v1's %v, v2's %v", a, b)
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
