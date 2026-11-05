package golden

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// resetStep is a command line run in one of two clones of basic that
// share an origin.
type resetStep struct {
	clone int // 0 or 1
	line  string
}

// resetSteps start one clone's board over and bring the other to it:
// reset --hard and push --prune, each with the refusals between.
var resetSteps = []resetStep{
	{0, "push"},
	{1, "pull"},
	{0, "reset"},
	{0, "reset --hard nope"},
	{0, "push --prune"},
	{0, "reset --hard"},
	{0, "pull"},
	{0, "push"},
	{0, "push --prune"},
	{0, "push --prune"},
	{1, "pull"},
	{1, "push"},
	{1, "new \"written after the start-over\""},
	{1, "reset --hard origin"},
	{1, `ls "is:open,closed" --json id`},
	{1, "pull"},
	{1, "push"},
	{0, "datastore drop"},
	{0, "reset --hard origin"},
	{0, "pull"},
}

var (
	bundlePath = regexp.MustCompile(`\S+\.bundle`)
	markTime   = regexp.MustCompile(`\d{4}-\d\d-\d\d \d\d:\d\d`)
)

// TestCrossReset: a board starts over as v1 started it over, on two
// clones of basic with one origin: each step prints and exits as it did,
// the bundles' paths and the marks' times aside. No ls on the emptied
// clone: v1's first pull of an empty board spoke there.
func TestCrossReset(t *testing.T) {
	t.Parallel()
	bin := binary(t)
	var board Board
	for _, b := range boards(t) {
		if b.Name == "basic" {
			board = b
		}
	}
	ctx := context.Background()
	origin := filepath.Join(t.TempDir(), "origin.git")
	if err := Git(ctx, ".", "init", "-q", "--bare", origin); err != nil {
		t.Fatal(err)
	}
	var dirs, homes [2]string
	for i := range dirs {
		dirs[i], homes[i], _ = checkout(t, board)
		if err := Git(ctx, dirs[i], "remote", "add", "origin", origin); err != nil {
			t.Fatal(err)
		}
	}
	var tr transcript
	for _, s := range resetSteps {
		r, err := Run(ctx, bin, dirs[s.clone], homes[s.clone], Case{Line: s.line})
		if err != nil {
			t.Fatalf("%s: %v", s.line, err)
		}
		for _, out := range []*[]byte{&r.Stdout, &r.Stderr} {
			*out = bundlePath.ReplaceAll(*out, []byte("<bundle>"))
			*out = markTime.ReplaceAll(*out, []byte("<when>"))
			*out = shortIDHex.ReplaceAll(*out, []byte("ID"))
		}
		tr.add(fmt.Sprintf("clone %d: %s", s.clone, s.line), r)
	}
	tr.check(t, "reset")
}

// dropSteps throw a forge-only board's copy away, refused while an issue
// isn't on a forge, then forced: the person stays.
var dropSteps = []string{
	"me --new",
	`new "local only"`,
	"datastore drop",
	"reset --hard",
	"push --prune",
	"pull",
	"datastore drop --force",
	"me",
	`ls "is:open,closed" --json id`,
}

// TestCrossDrop: a forge-only board's copy drops as v1 dropped it.
func TestCrossDrop(t *testing.T) {
	t.Parallel()
	bin := binary(t)
	ctx := context.Background()
	tmp := t.TempDir()
	dir, home := filepath.Join(tmp, "repo"), filepath.Join(tmp, "home")
	for _, args := range [][]string{{"init", "-q", dir}, {"-C", dir, "config", "user.name", "Ann"},
		{"-C", dir, "config", "user.email", "ann@example.org"}, {"-C", dir, "config", "board.datastore", "forge"}} {
		if err := Git(ctx, ".", args...); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	var tr transcript
	for _, line := range dropSteps {
		r, err := Run(ctx, bin, dir, home, Case{Line: line})
		if err != nil {
			t.Fatalf("%s: %v", line, err)
		}
		r.Stdout = shortIDHex.ReplaceAll(r.Stdout, []byte("ID"))
		tr.add(line, r)
	}
	tr.check(t, "drop")
}
