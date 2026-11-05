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

// TestCrossLint: v1 and v2 lint every golden board the same.
func TestCrossLint(t *testing.T) {
	t.Parallel()
	one, two := binaries(t)
	for _, b := range boards(t) {
		t.Run(b.Name, func(t *testing.T) {
			t.Parallel()
			ctx := context.Background()
			dir, home, _ := checkout(t, b)
			for _, line := range lintLines {
				a, err := Run(ctx, one, dir, home, Case{Line: line})
				if err != nil {
					t.Fatal(err)
				}
				c, err := Run(ctx, two, dir, home, Case{Line: line})
				if err != nil {
					t.Fatal(err)
				}
				if line == "lint" && b.Name == "basic" && !bytes.Contains(a.Stdout, []byte("missing-value:")) {
					t.Errorf("v1 finds nothing on basic:\n%s", a.Stdout)
				}
				if a.Exit != c.Exit || !bytes.Equal(a.Stdout, c.Stdout) || !bytes.Equal(a.Stderr, c.Stderr) {
					t.Errorf("%s: v1 and v2 differ\nexit %d / %d\nstderr:\n%s\nstdout:\n%s",
						line, a.Exit, c.Exit, Diff(a.Stderr, c.Stderr), Diff(a.Stdout, c.Stdout))
				}
			}
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

// TestCrossLintCycle: with the cycle on, v1 and v2 say the same read:
// lines.
func TestCrossLintCycle(t *testing.T) {
	t.Parallel()
	one, two := binaries(t)
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
	a, err := Run(ctx, one, dir, home, Case{Line: "lint"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := Run(ctx, two, dir, home, Case{Line: "lint"})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(a.Stdout, []byte("[cycle.signoffs] validated")) {
		t.Errorf("v1 says no read: lines:\n%s", a.Stdout)
	}
	if a.Exit != b.Exit || !bytes.Equal(a.Stdout, b.Stdout) || !bytes.Equal(a.Stderr, b.Stderr) {
		t.Errorf("v1 and v2 differ\nexit %d / %d\nstderr:\n%s\nstdout:\n%s", a.Exit, b.Exit, Diff(a.Stderr, b.Stderr), Diff(a.Stdout, b.Stdout))
	}
}
