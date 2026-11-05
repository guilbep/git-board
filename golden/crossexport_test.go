package golden

import (
	"bytes"
	"context"
	"maps"
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

// TestCrossExport: a board exports to Markdown files and imports them
// back as v1 did: each step prints and exits as it did, new ids aside,
// and the files end as they did, ids and times of now aside.
func TestCrossExport(t *testing.T) {
	t.Parallel()
	bin := binary(t)
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
			crossExport(t, bin, byName[c.board], c.md, c.steps)
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

func crossExport(t *testing.T, bin string, board Board, mdDir string, exportSteps []any) {
	ctx := context.Background()
	dir, home, _ := checkout(t, board)
	md := filepath.Join(dir, mdDir)
	var tr transcript
	for _, s := range exportSteps {
		line, ok := s.(string)
		if !ok {
			s.(func(*testing.T, string))(t, md)
			continue
		}
		r, err := Run(ctx, bin, dir, home, Case{Line: line})
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		r.Stdout = shortIDHex.ReplaceAll(r.Stdout, []byte("ID"))
		if strings.HasPrefix(line, "export") { // by id, and new ids differ
			r.Stdout = sortLines(r.Stdout)
		}
		tr.add(line, r)
	}
	files := map[string]string{}
	paths, _ := filepath.Glob(filepath.Join(md, "*.md"))
	for _, p := range paths {
		data, _ := os.ReadFile(p)
		files[shortIDHex.ReplaceAllString(filepath.Base(p), "ID")] = nowish.ReplaceAllString(shortIDHex.ReplaceAllString(string(data), "ID"), "NOW")
	}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		tr.file(name, []byte(files[name]))
	}
	tr.check(t, "export-"+board.Name)
}

// nowish is a time of today's, as an import stamps a write.
var nowish = regexp.MustCompile(time.Now().UTC().Format("2006-01-02") + `T\d\d:\d\d:\d\dZ`)

func sortLines(b []byte) []byte {
	lines := strings.SplitAfter(string(b), "\n")
	slices.Sort(lines)
	return []byte(strings.Join(lines, ""))
}
