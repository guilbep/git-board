package golden

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// writes are the commands the cross-write test runs on the basic board,
// one writer at a time: each of M2's writes, with the cases a write
// refuses.
var writes = []string{
	`new "Cross write one" prio=p1 -b "a body, with ünicode"`,
	`new "Cross write two" status=ready kind:bug assignee=bob`,
	`comment 26a4ba8 -b "a comment from the cross test"`,
	`comment 22d74e7 -b "another"`,
	`comment 22d74e7 --edit-last -b "another, edited"`,
	`edit 398523d --title "Retitled by the cross test"`,
	`edit 398523d --title "Retitled by the cross test"`,
	`edit 398523d -b "a new body"`,
	`set 26a4ba8 prio=p0 est=3`,
	`set 26a4ba8 prio=p9`,
	`set 26a4ba8 nosuch=x`,
	`set 26a4ba8 assignee=+chloe`,
	`set 26a4ba8 assignee=-chloe`,
	`move 26a4ba8 in-review`,
	`move 22d74e7 --next`,
	`move 22d74e7 --prev`,
	`move 398523d --prev`,
	`close 999eab2 -c "closing it" --reason not-planned`,
	`close 22d74e7 --duplicate-of 9062b9a`,
	`close 22d74e7 --duplicate-of 22d74e7`,
	`reopen 999eab2 -c "back again"`,
	`set 398523d assignee=+@me status=done`,
	`set 398523d status=ready`,
	`set 22c49b4 est=`,
	`set 22c49b4 kind=bug`,
	`new`,
	`new "Bad field" prio=p9`,
	`comment 9062b9a -b ""`,
	`comment 999eab2 --delete nosuch --yes`,
	`delete 427cb1b --yes`,
	`comment 427cb1b -b "on a deleted issue"`,
	`set 427cb1b prio=p1`,
	`comment 427cb1b --edit-last -b "on a deleted issue"`,
	`delete 0000000 --yes`,
	`undo 26a4ba8`,
	`undo 26a4ba8`,
	`undo 26a4ba8`,
	`undo 999eab2`,
	`undo 999eab2 --force`,
	`undo 22c49b4`,
	`undo --json`,
	`undo`,
	`undo e958491`,
	`labels`,
	`me`,
}

var shortIDHex = regexp.MustCompile(`\b[0-9a-f]{7,64}\b`)

// binaries builds v1's git-board and git-board-next.
func binaries(t *testing.T) (v1, v2 string) {
	t.Helper()
	if testing.Short() {
		t.Skip("builds both binaries: not with -short")
	}
	// go test caches a result by the files the test reads: stat every
	// source both binaries build from, so an edit runs the test again.
	for _, root := range []string{"../..", ".."} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && (d.Name() == "testdata" || d.Name() == "node_modules" || strings.HasPrefix(d.Name(), ".")) && path != root {
				return filepath.SkipDir
			}
			if !d.IsDir() && (strings.HasSuffix(path, ".go") || d.Name() == "go.mod" || d.Name() == "go.sum") {
				_, err = os.Stat(path)
			}
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	dir := t.TempDir()
	v1, v2 = filepath.Join(dir, "git-board"), filepath.Join(dir, "git-board-next")
	for bin, pkg := range map[string]string{v1: "../../cmd/git-board", v2: "../cmd/git-board-next"} {
		if out, err := exec.Command("go", "build", "-o", bin, pkg).CombinedOutput(); err != nil {
			t.Fatalf("go build %s: %v\n%s", pkg, err, out)
		}
	}
	return v1, v2
}

// TestCrossWrites: what one version writes, the other reads the same. On
// a checkout of the basic board, v2 runs the writes, then v1 and v2 read
// the board (ls, and show and log of every issue) and agree; then the
// reverse, v1 writing. Each write prints the same, exits the same, on
// both, its new ids aside.
func TestCrossWrites(t *testing.T) {
	t.Parallel()
	one, two := binaries(t)
	var basic Board
	for _, b := range boards(t) {
		if b.Name == "basic" {
			basic = b
		}
	}
	if basic.Name == "" {
		t.Fatal("no basic golden board")
	}
	said := map[string][]Result{}
	for _, writer := range []struct{ name, bin string }{{"v2", two}, {"v1", one}} {
		t.Run(writer.name+" writes", func(t *testing.T) {
			ctx := context.Background()
			dir, home, _ := checkout(t, basic)
			for _, line := range writes {
				r, err := Run(ctx, writer.bin, dir, home, Case{Line: line})
				if err != nil {
					t.Fatalf("%s %s: %v", writer.name, line, err)
				}
				said[writer.name] = append(said[writer.name], r)
			}
			ids, err := basic.IssueIDs(ctx, dir)
			if err != nil {
				t.Fatal(err)
			}
			cases := []Case{{"ls", `ls "is:open,closed" --json`}, {"labels", "labels --json"}}
			for _, id := range ids {
				cases = append(cases, Case{"show " + id[:7], "show " + id + " --json"}, Case{"log " + id[:7], "log " + id + " --json"})
			}
			for _, c := range cases {
				a, err := Run(ctx, one, dir, home, c)
				if err != nil {
					t.Fatal(err)
				}
				b, err := Run(ctx, two, dir, home, c)
				if err != nil {
					t.Fatal(err)
				}
				if a.Exit != b.Exit || !bytes.Equal(a.Stderr, b.Stderr) || !jsonEqual(a.Stdout, b.Stdout) {
					a.Stdout, b.Stdout = canon(decode(a.Stdout)), canon(decode(b.Stdout))
					t.Errorf("after %s's writes, %s: v1 and v2 differ\nexit %d / %d\nstderr:\n%s\nstdout:\n%s",
						writer.name, c.Line, a.Exit, b.Exit, Diff(a.Stderr, b.Stderr), Diff(a.Stdout, b.Stdout))
				}
			}
		})
	}
	if t.Failed() {
		return
	}
	for i, line := range writes {
		a, b := said["v1"][i], said["v2"][i]
		a.Stdout, b.Stdout = shortIDHex.ReplaceAll(a.Stdout, []byte("ID")), shortIDHex.ReplaceAll(b.Stdout, []byte("ID"))
		if a.Exit != b.Exit || !bytes.Equal(a.Stdout, b.Stdout) || !bytes.Equal(a.Stderr, b.Stderr) {
			t.Errorf("%s: v1 and v2 differ\nexit %d / %d\nstderr:\n%s\nstdout:\n%s",
				line, a.Exit, b.Exit, Diff(a.Stderr, b.Stderr), Diff(a.Stdout, b.Stdout))
		}
	}
}

// forged are the keys v2 has from sync records, M4 (pending.txt): left
// out of the comparison until then.
var forged = []string{"forges", "github_url", "pending", "shown_title"}

// jsonEqual compares v1's output and v2's as JSON when both are, as
// bytes else: key order and the forged keys aside, and the number v1
// takes from a forge (below the counter's start, M4) that v2 lacks.
func jsonEqual(one, two []byte) bool {
	var x, y any
	if json.Unmarshal(one, &x) != nil || json.Unmarshal(two, &y) != nil {
		return bytes.Equal(one, two)
	}
	return same(x, y)
}

func same(x, y any) bool {
	switch x := x.(type) {
	case map[string]any:
		y, ok := y.(map[string]any)
		if !ok {
			return false
		}
		x, y = maps.Clone(x), maps.Clone(y)
		if f, _ := x["forges"].([]any); len(f) > 0 && y["number"] == nil {
			delete(x, "number")
		}
		for _, k := range forged {
			delete(x, k)
			delete(y, k)
		}
		if len(x) != len(y) {
			return false
		}
		for k, v := range x {
			w, ok := y[k]
			if !ok || !same(v, w) {
				return false
			}
		}
		return true
	case []any:
		y, ok := y.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !same(x[i], y[i]) {
				return false
			}
		}
		return true
	}
	return reflect.DeepEqual(x, y)
}

// canon is v as JSON with the forged keys out, to show a difference.
func canon(v any) []byte {
	var strip func(any)
	strip = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for _, k := range forged {
				delete(v, k)
			}
			for _, e := range v {
				strip(e)
			}
		case []any:
			for _, e := range v {
				strip(e)
			}
		}
	}
	strip(v)
	out, _ := json.MarshalIndent(v, "", "  ") // maps marshal with sorted keys
	return out
}

func decode(b []byte) any {
	var v any
	if json.Unmarshal(b, &v) != nil {
		return string(b)
	}
	return v
}
