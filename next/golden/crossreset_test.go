package golden

import (
	"bytes"
	"context"
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

// TestCrossReset: v1 and v2 start a board over the same, on two clones
// of basic with one origin: each step prints the same and exits the
// same, the bundles' paths and the marks' times aside. No ls on the
// emptied clone: v1's first pull of an empty board speaks there.
func TestCrossReset(t *testing.T) {
	t.Parallel()
	one, two := binaries(t)
	var board Board
	for _, b := range boards(t) {
		if b.Name == "basic" {
			board = b
		}
	}
	said := map[string][]Result{}
	for _, writer := range []struct{ name, bin string }{{"v2", two}, {"v1", one}} {
		t.Run(writer.name, func(t *testing.T) {
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
			for _, s := range resetSteps {
				r, err := Run(ctx, writer.bin, dirs[s.clone], homes[s.clone], Case{Line: s.line})
				if err != nil {
					t.Fatalf("%s %s: %v", writer.name, s.line, err)
				}
				for _, out := range []*[]byte{&r.Stdout, &r.Stderr} {
					*out = bundlePath.ReplaceAll(*out, []byte("<bundle>"))
					*out = markTime.ReplaceAll(*out, []byte("<when>"))
					*out = shortIDHex.ReplaceAll(*out, []byte("ID"))
				}
				said[writer.name] = append(said[writer.name], r)
			}
		})
	}
	if t.Failed() {
		return
	}
	for i, s := range resetSteps {
		a, b := said["v1"][i], said["v2"][i]
		if a.Exit != b.Exit || !bytes.Equal(a.Stdout, b.Stdout) || !bytes.Equal(a.Stderr, b.Stderr) {
			t.Errorf("clone %d, %s: v1 and v2 differ\nexit %d / %d\nstderr:\n%s\nstdout:\n%s",
				s.clone, s.line, a.Exit, b.Exit, Diff(a.Stderr, b.Stderr), Diff(a.Stdout, b.Stdout))
		}
	}
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

// TestCrossDrop: v1 and v2 drop a forge-only board's copy the same.
func TestCrossDrop(t *testing.T) {
	t.Parallel()
	one, two := binaries(t)
	said := map[string][]Result{}
	for _, writer := range []struct{ name, bin string }{{"v2", two}, {"v1", one}} {
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
		for _, line := range dropSteps {
			r, err := Run(ctx, writer.bin, dir, home, Case{Line: line})
			if err != nil {
				t.Fatalf("%s %s: %v", writer.name, line, err)
			}
			r.Stdout = shortIDHex.ReplaceAll(r.Stdout, []byte("ID"))
			said[writer.name] = append(said[writer.name], r)
		}
	}
	for i, line := range dropSteps {
		a, b := said["v1"][i], said["v2"][i]
		if a.Exit != b.Exit || !bytes.Equal(a.Stdout, b.Stdout) || !bytes.Equal(a.Stderr, b.Stderr) {
			t.Errorf("%s: v1 and v2 differ\nexit %d / %d\nstderr:\n%s\nstdout:\n%s",
				line, a.Exit, b.Exit, Diff(a.Stderr, b.Stderr), Diff(a.Stdout, b.Stdout))
		}
	}
}
