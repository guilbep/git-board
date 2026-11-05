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

// releaseReads are what is read after the writes.
var releaseReads = []string{"release", "release notes", "release notes --all", `ls "is:open,closed" --json`}

// TestCrossReleases: the release record's writes print what v1 printed,
// and the notes read after them as v1 read them after its own. On a
// checkout of the releases board, the writes, then the reads.
func TestCrossReleases(t *testing.T) {
	t.Parallel()
	bin := binary(t)
	var board Board
	for _, b := range boards(t) {
		if b.Name == "releases" {
			board = b
		}
	}
	if board.Name == "" {
		t.Fatal("no releases golden board")
	}
	ctx := context.Background()
	dir, home, _ := checkout(t, board)
	var tr transcript
	for _, line := range releaseWrites {
		r, err := Run(ctx, bin, dir, home, Case{Line: line})
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		tr.add(line, r)
	}
	for _, line := range releaseReads {
		r, err := Run(ctx, bin, dir, home, Case{Line: line})
		if err != nil {
			t.Fatal(err)
		}
		if line == "release notes --all" && !bytes.Contains(r.Stdout, []byte("yanked: found a bug")) {
			t.Errorf("the notes don't say the yank:\n%s", r.Stdout)
		}
		tr.add(line, readNorm(r))
	}
	tr.check(t, "releases")
}
