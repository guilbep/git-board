package golden

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// initSteps run init in fresh repositories (a, b, c), outside one (out),
// with the templates: a step's dir, then its command line.
var initSteps = [][2]string{
	{"out", "init"},
	{"a", "init"},
	{"a", "init"},
	{"a/sub", "init"},
	{"b", "init --template nope"},
	{"b", "init --template enac"},
	{"c", "init --template ../bad.toml"},
	{"c", "init --template ../good.toml"},
	{"d", "init --template ../missing.toml"},
}

// TestCrossInit: v1 and v2 init the same: each step prints the same and
// exits the same, the paths aside, and the files written are the same.
func TestCrossInit(t *testing.T) {
	t.Parallel()
	one, two := binaries(t)
	said := map[string][]Result{}
	files := map[string]map[string]string{}
	for _, writer := range []struct{ name, bin string }{{"v2", two}, {"v1", one}} {
		ctx := context.Background()
		root := t.TempDir()
		home := filepath.Join(root, "home")
		for _, d := range []string{"home", "out", "a/sub", "b", "c", "d"} {
			if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
				t.Fatal(err)
			}
		}
		for _, d := range []string{"a", "b", "c", "d"} {
			if err := Git(ctx, filepath.Join(root, d), "init", "-q"); err != nil {
				t.Fatal(err)
			}
		}
		write(t, filepath.Join(root, "bad.toml"), "[fields.prio]\noptions = 3\n")
		write(t, filepath.Join(root, "good.toml"), "[views.mine]\nfilter = \"assignee:@me\"\n")
		for _, s := range initSteps {
			r, err := Run(ctx, writer.bin, filepath.Join(root, s[0]), home, Case{Line: s[1]})
			if err != nil {
				t.Fatalf("%s %s: %v", writer.name, s[1], err)
			}
			for _, out := range []*[]byte{&r.Stdout, &r.Stderr} {
				*out = bytes.ReplaceAll(*out, []byte(root), []byte("$ROOT"))
			}
			said[writer.name] = append(said[writer.name], r)
		}
		files[writer.name] = map[string]string{}
		for _, d := range []string{"a", "b", "c", "d"} {
			data, _ := os.ReadFile(filepath.Join(root, d, ".git-board.toml"))
			files[writer.name][d] = string(data)
		}
	}
	for i, s := range initSteps {
		a, b := said["v1"][i], said["v2"][i]
		if a.Exit != b.Exit || !bytes.Equal(a.Stdout, b.Stdout) || !bytes.Equal(a.Stderr, b.Stderr) {
			t.Errorf("in %s, %s: v1 and v2 differ\nexit %d / %d\nstderr:\n%s\nstdout:\n%s",
				s[0], s[1], a.Exit, b.Exit, Diff(a.Stderr, b.Stderr), Diff(a.Stdout, b.Stdout))
		}
	}
	for d, a := range files["v1"] {
		if b := files["v2"][d]; a != b {
			t.Errorf("%s/.git-board.toml: v1 and v2 differ\n%s", d, Diff([]byte(a), []byte(b)))
		}
		if d != "d" && !strings.Contains(a, "[") {
			t.Errorf("%s/.git-board.toml: v1 wrote none", d)
		}
	}
}
