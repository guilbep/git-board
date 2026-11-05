package golden

import (
	"bytes"
	"context"
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
