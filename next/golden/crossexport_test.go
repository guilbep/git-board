package golden

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// exportSteps export basic to Markdown files, hand-edit them, and import
// them back: a step is a command line, or an edit of the folder (a func).
var exportSteps = []any{
	"export",
	"import --md nope",
	"export --md md",
	"export --md md",
	"import --md md",
	func(t *testing.T, md string) { // a title, a new comment, a new issue, a file that isn't one
		paths, _ := filepath.Glob(filepath.Join(md, "*.md"))
		for _, p := range paths {
			data, _ := os.ReadFile(p)
			if bytes.Contains(data, []byte("\nstate: open\n")) && bytes.Contains(data, []byte("<!-- git-board comments -->")) {
				data = regexp.MustCompile(`(?m)^title: .*$`).ReplaceAll(data, []byte("title: Edited by hand"))
				data = append(data, "\n### Alice · 2025-06-01T10:00:00Z\n\nA comment by hand.\n"...)
				if err := os.WriteFile(p, data, 0o644); err != nil {
					t.Fatal(err)
				}
				break
			}
		}
		write(t, filepath.Join(md, "zz-new.md"), "---\ntitle: Made by hand\nstate: open\nlabels: [docs]\npeople:\n  author: Alice\ncreated: 2025-06-01T09:00:00Z\nupdated: 2025-06-01T09:00:00Z\n---\n\nIts body.\n")
		write(t, filepath.Join(md, "README.md"), "# not an issue\n")
	},
	"import --md md",
	"import --md md",
	"export --md md",
}

func write(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestCrossExport: v1 and v2 export a board to Markdown files and import
// them back the same: each step prints the same and exits the same, new
// ids aside, and the files end the same, ids and times of now aside.
func TestCrossExport(t *testing.T) {
	t.Parallel()
	one, two := binaries(t)
	byName := map[string]Board{}
	for _, b := range boards(t) {
		byName[b.Name] = b
	}
	for _, c := range []struct {
		board string
		md    string
		steps []any
	}{{"basic", "md", exportSteps}, {"markdown", "issues", mdImportSteps}} {
		t.Run(c.board, func(t *testing.T) {
			t.Parallel()
			crossExport(t, one, two, byName[c.board], c.md, c.steps)
		})
	}
}

// mdImportSteps import on a Markdown folder board: its own folder, by
// default or named, after a hand edit.
var mdImportSteps = []any{
	"import",
	func(t *testing.T, md string) {
		write(t, filepath.Join(md, "zz-new.md"), "---\ntitle: Made by hand\nstate: open\npeople:\n  author: Alice\ncreated: 2025-06-01T09:00:00Z\nupdated: 2025-06-01T09:00:00Z\n---\n\nIts body.\n")
	},
	"import --md issues",
	"import",
	"export --md out",
}

func crossExport(t *testing.T, one, two string, board Board, mdDir string, exportSteps []any) {
	said := map[string][]Result{}
	files := map[string]map[string]string{}
	for _, writer := range []struct{ name, bin string }{{"v2", two}, {"v1", one}} {
		t.Run(writer.name, func(t *testing.T) {
			ctx := context.Background()
			dir, home, _ := checkout(t, board)
			md := filepath.Join(dir, mdDir)
			for _, s := range exportSteps {
				line, ok := s.(string)
				if !ok {
					s.(func(*testing.T, string))(t, md)
					continue
				}
				r, err := Run(ctx, writer.bin, dir, home, Case{Line: line})
				if err != nil {
					t.Fatalf("%s %s: %v", writer.name, line, err)
				}
				r.Stdout = shortIDHex.ReplaceAll(r.Stdout, []byte("ID"))
				said[writer.name] = append(said[writer.name], r)
			}
			files[writer.name] = map[string]string{}
			paths, _ := filepath.Glob(filepath.Join(md, "*.md"))
			for _, p := range paths {
				data, _ := os.ReadFile(p)
				files[writer.name][strings.Replace(filepath.Base(p), filepath.Base(p), shortIDHex.ReplaceAllString(filepath.Base(p), "ID"), 1)] = nowish.ReplaceAllString(shortIDHex.ReplaceAllString(string(data), "ID"), "NOW")
			}
		})
	}
	if t.Failed() {
		return
	}
	i := 0
	for _, s := range exportSteps {
		line, ok := s.(string)
		if !ok {
			continue
		}
		a, b := said["v1"][i], said["v2"][i]
		i++
		if strings.HasPrefix(line, "export") { // by id, and new ids differ
			a.Stdout, b.Stdout = sortLines(a.Stdout), sortLines(b.Stdout)
		}
		if a.Exit != b.Exit || !bytes.Equal(a.Stdout, b.Stdout) || !bytes.Equal(a.Stderr, b.Stderr) {
			t.Errorf("%s: v1 and v2 differ\nexit %d / %d\nstderr:\n%s\nstdout:\n%s",
				line, a.Exit, b.Exit, Diff(a.Stderr, b.Stderr), Diff(a.Stdout, b.Stdout))
		}
	}
	for name, a := range files["v1"] {
		if b, ok := files["v2"][name]; !ok {
			t.Errorf("%s: v1 has it, v2 hasn't", name)
		} else if a != b {
			t.Errorf("%s: v1 and v2 differ\n%s", name, Diff([]byte(a), []byte(b)))
		}
	}
	for name := range files["v2"] {
		if _, ok := files["v1"][name]; !ok {
			t.Errorf("%s: v2 has it, v1 hasn't", name)
		}
	}
}

// nowish is a time of today's, as an import stamps a write.
var nowish = regexp.MustCompile(time.Now().UTC().Format("2006-01-02") + `T\d\d:\d\d:\d\dZ`)

func sortLines(b []byte) []byte {
	lines := strings.SplitAfter(string(b), "\n")
	slices.Sort(lines)
	return []byte(strings.Join(lines, ""))
}
