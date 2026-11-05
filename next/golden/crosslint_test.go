package golden

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// lintLines are the lint command lines run on each golden board.
var lintLines = []string{"lint", "lint --strict", "lint --apply"}

// TestCrossLint: every golden board lints as v1 linted it.
func TestCrossLint(t *testing.T) {
	t.Parallel()
	bin := binary(t)
	for _, b := range boards(t) {
		t.Run(b.Name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			dir, home, _ := checkout(t, b)
			var tr transcript
			for _, line := range lintLines {
				r, err := Run(ctx, bin, dir, home, Case{Line: line})
				if err != nil {
					t.Fatal(err)
				}
				if line == "lint" && b.Name == "basic" && !bytes.Contains(r.Stdout, []byte("missing-value:")) {
					t.Errorf("lint finds nothing on basic:\n%s", r.Stdout)
				}
				tr.add(line, r)
			}
			tr.check(t, "lint-"+b.Name)
		})
	}
}

// cycleConfig turns the cycle on, with a sign-off and a kind's rule as a
// string: lint says how it read them.
const cycleConfig = `[fields.status]
options = ["todo", "in-progress", "in-review", "validated", "done"]
single = true

[roles]
lead = "alice"

[experimental]
features = ["cycle"]

[cycle]
validation = ["validated"]
signoffs = { validated = "lead" }

[cycle.kinds]
bug = "failing-test"
feat = { tests = "none" }
`

// TestCrossLintCycle: with the cycle on, lint says v1's read: lines.
func TestCrossLintCycle(t *testing.T) {
	t.Parallel()
	bin := binary(t)
	ctx := context.Background()
	tmp := t.TempDir()
	dir, home := filepath.Join(tmp, "repo"), filepath.Join(tmp, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Git(ctx, ".", "init", "-q", dir); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, ".git-board.toml"), cycleConfig)
	r, err := Run(ctx, bin, dir, home, Case{Line: "lint"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(r.Stdout, []byte("[cycle.signoffs] validated")) {
		t.Errorf("lint says no read: lines:\n%s", r.Stdout)
	}
	var tr transcript
	tr.add("lint", r)
	tr.check(t, "lint-cycle")
}
