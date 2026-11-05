package golden

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestCrossFirstPull: a fresh git clone of a board v1 made (git clone
// fetches no refs/board/*) opens on its whole board, in v1 and in v2
// alike: the first command pulls origin's board and says so as v1 does,
// and every issue shows as v1 shows it on the board it made. (v1's first
// pull leaves origin's numbers behind, its git board pull takes them; v2's
// takes them at once, so the clone's issues are origin's, numbers and
// all: docs/prd/clone-shows-board.md.)
func TestCrossFirstPull(t *testing.T) {
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
	ctx := context.Background()
	origin, home, _ := checkout(t, basic)
	toml, err := os.ReadFile(filepath.Join(origin, ".git-board.toml"))
	if err != nil {
		t.Fatal(err)
	}
	// a fresh clone each: the first command is the one that pulls
	clone := func(name string) string {
		dir := filepath.Join(filepath.Dir(origin), name)
		if err := Git(ctx, "", "clone", "-q", origin, dir); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), toml, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := Git(ctx, dir, "config", "board.me", basic.Me); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	line := Case{Line: `ls "is:open,closed" --json`}
	a, err := Run(ctx, one, clone("v1"), home, line)
	if err != nil {
		t.Fatal(err)
	}
	made, err := Run(ctx, one, origin, home, line)
	if err != nil {
		t.Fatal(err)
	}
	dir := clone("v2")
	b, err := Run(ctx, two, dir, home, line)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(b.Stderr, []byte("a fresh clone: pulled ")) {
		t.Errorf("v2 doesn't say it pulled: %q", b.Stderr)
	}
	if a.Exit != b.Exit || !bytes.Equal(a.Stderr, b.Stderr) || !jsonEqual(made.Stdout, b.Stdout) {
		t.Errorf("a fresh clone's first ls: v1 and v2 differ\nexit %d / %d\nstderr:\n%s\nstdout (v1 on origin, v2 on the clone):\n%s",
			a.Exit, b.Exit, Diff(a.Stderr, b.Stderr), Diff(canon(decode(made.Stdout)), canon(decode(b.Stdout))))
	}
	// the second open pulls nothing, and says nothing
	again, err := Run(ctx, two, dir, home, line)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Stderr) != 0 || !jsonEqual(again.Stdout, b.Stdout) {
		t.Errorf("the second ls: stderr %q, the same issues: %v", again.Stderr, jsonEqual(again.Stdout, b.Stdout))
	}
}
