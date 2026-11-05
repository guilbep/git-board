package golden

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestCrossFirstPull: a fresh git clone of a board (git clone fetches
// no refs/board/*) opens on its whole board: the first command pulls
// origin's board and says so as v1 did, and every issue shows as on
// origin, numbers and all (docs/prd/clone-shows-board.md); the second
// pulls nothing, and says nothing.
func TestCrossFirstPull(t *testing.T) {
	t.Parallel()
	bin := binary(t)
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
	dir := filepath.Join(filepath.Dir(origin), "clone")
	if err := Git(ctx, "", "clone", "-q", origin, dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), toml, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Git(ctx, dir, "config", "board.me", basic.Me); err != nil {
		t.Fatal(err)
	}
	line := Case{Line: `ls "is:open,closed" --json`}
	made, err := Run(ctx, bin, origin, home, line)
	if err != nil {
		t.Fatal(err)
	}
	first, err := Run(ctx, bin, dir, home, line)
	if err != nil {
		t.Fatal(err)
	}
	var tr transcript
	tr.add(line.Line, Result{Exit: first.Exit, Stderr: first.Stderr}) // what v1 said
	tr.check(t, "first-pull")
	if !bytes.Contains(first.Stderr, []byte("a fresh clone: pulled ")) {
		t.Errorf("the first ls doesn't say it pulled: %q", first.Stderr)
	}
	if !bytes.Equal(made.Stdout, first.Stdout) {
		t.Errorf("a fresh clone's first ls isn't origin's:\n%s", Diff(made.Stdout, first.Stdout))
	}
	again, err := Run(ctx, bin, dir, home, line)
	if err != nil {
		t.Fatal(err)
	}
	if len(again.Stderr) != 0 || !bytes.Equal(again.Stdout, first.Stdout) {
		t.Errorf("the second ls: stderr %q, the same issues: %v", again.Stderr, bytes.Equal(again.Stdout, first.Stdout))
	}
}
