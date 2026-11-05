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

// TestCrossInit: init prints and exits as v1 did, each step, the paths
// aside, and writes the files v1 wrote.
func TestCrossInit(t *testing.T) {
	t.Parallel()
	bin := binary(t)
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
	var tr transcript
	for _, s := range initSteps {
		r, err := Run(ctx, bin, filepath.Join(root, s[0]), home, Case{Line: s[1]})
		if err != nil {
			t.Fatalf("%s: %v", s[1], err)
		}
		for _, out := range []*[]byte{&r.Stdout, &r.Stderr} {
			*out = bytes.ReplaceAll(*out, []byte(root), []byte("$ROOT"))
		}
		tr.add("in "+s[0]+": "+s[1], r)
	}
	for _, d := range []string{"a", "b", "c", "d"} {
		data, _ := os.ReadFile(filepath.Join(root, d, ".git-board.toml"))
		if d != "d" && !strings.Contains(string(data), "[") {
			t.Errorf("%s/.git-board.toml: init wrote none", d)
		}
		tr.file(d+"/.git-board.toml", data)
	}
	tr.check(t, "init")
}
