package golden

import (
	"bytes"
	"context"
	"testing"
)

// releaseWrites are the release commands that write, on the releases
// board, whose origin is itself: each fetches, writes the record, and
// pushes it.
var releaseWrites = []string{
	`release yank v0.1.0 --reason "found a bug"`,
	`release yank v0.2.0-rc.1 --reason "still broken"`,
	`release yank v9.9.9 --reason "no such tag"`,
}

// releaseReads are what both versions read after the other's writes.
var releaseReads = []string{"release", "release notes", "release notes --all", `ls "is:open,closed" --json`}

// TestCrossReleases: what one version writes of a release record, the
// other reads the same. On a checkout of the releases board, one version
// yanks, then v1 and v2 read the notes and agree; then the reverse. Each
// write prints the same, exits the same, on both.
func TestCrossReleases(t *testing.T) {
	t.Parallel()
	one, two := binaries(t)
	var board Board
	for _, b := range boards(t) {
		if b.Name == "releases" {
			board = b
		}
	}
	if board.Name == "" {
		t.Fatal("no releases golden board")
	}
	said := map[string][]Result{}
	for _, writer := range []struct{ name, bin string }{{"v2", two}, {"v1", one}} {
		t.Run(writer.name+" writes", func(t *testing.T) {
			ctx := context.Background()
			dir, home, _ := checkout(t, board)
			for _, line := range releaseWrites {
				r, err := Run(ctx, writer.bin, dir, home, Case{Line: line})
				if err != nil {
					t.Fatalf("%s %s: %v", writer.name, line, err)
				}
				said[writer.name] = append(said[writer.name], r)
			}
			for _, line := range releaseReads {
				a, err := Run(ctx, one, dir, home, Case{Line: line})
				if err != nil {
					t.Fatal(err)
				}
				b, err := Run(ctx, two, dir, home, Case{Line: line})
				if err != nil {
					t.Fatal(err)
				}
				if a.Exit != b.Exit || !bytes.Equal(a.Stderr, b.Stderr) || !jsonEqual(a.Stdout, b.Stdout) {
					t.Errorf("after %s's writes, %s: v1 and v2 differ\nexit %d / %d\nstderr:\n%s\nstdout:\n%s",
						writer.name, line, a.Exit, b.Exit, Diff(a.Stderr, b.Stderr), Diff(a.Stdout, b.Stdout))
				}
				if line == "release notes --all" && !bytes.Contains(a.Stdout, []byte("yanked: found a bug")) {
					t.Errorf("after %s's writes, v1's notes don't say the yank:\n%s", writer.name, a.Stdout)
				}
			}
		})
	}
	if t.Failed() {
		return
	}
	for i, line := range releaseWrites {
		a, b := said["v1"][i], said["v2"][i]
		if a.Exit != b.Exit || !bytes.Equal(a.Stdout, b.Stdout) || !bytes.Equal(a.Stderr, b.Stderr) {
			t.Errorf("%s: v1 and v2 differ\nexit %d / %d\nstderr:\n%s\nstdout:\n%s",
				line, a.Exit, b.Exit, Diff(a.Stderr, b.Stderr), Diff(a.Stdout, b.Stdout))
		}
	}
}
