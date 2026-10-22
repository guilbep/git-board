package proof

import (
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
)

func TestKey(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	c1 := commit(t, dir, map[string]string{"c.go": "package c", "a/b/d.go": "package b", "c.gox": "x", "README": "r"})
	go1 := config.Suite{Run: []string{"go", "test", "./..."}, Files: []string{"**/*.go"}}
	k1, err := Key(ctx(t), dir, c1, "go", go1)
	if err != nil {
		t.Fatal(err)
	}
	if again, _ := Key(ctx(t), dir, c1, "go", go1); again != k1 {
		t.Errorf("same inputs: %s then %s", k1, again)
	}

	text, _ := KeyText(ctx(t), dir, c1, "go", config.Suite{Run: []string{"sh", "-c", "a<b&&c"}, Dir: "web", Files: []string{"**/*.go"}, Env: map[string]string{"B": "2", "A": "1"}})
	blob := func(p string) string { return run(t, dir, "rev-parse", c1+":"+p) }
	want := "git-board-ci/key/v1\ncheck go\nrun [\"sh\",\"-c\",\"a<b&&c\"]\ndir web\nenv A=1\nenv B=2\n" +
		"file 100644 " + blob("a/b/d.go") + " a/b/d.go\nfile 100644 " + blob("c.go") + " c.go\n"
	if string(text) != want {
		t.Errorf("key text:\n%s\nwant:\n%s", text, want)
	}

	outside := commit(t, dir, map[string]string{"README": "changed", "c.gox": "y"})
	if k, _ := Key(ctx(t), dir, outside, "go", go1); k != k1 {
		t.Error("a change outside files changed the key")
	}
	inside := commit(t, dir, map[string]string{"a/b/d.go": "package b // changed"})
	if k, _ := Key(ctx(t), dir, inside, "go", go1); k == k1 {
		t.Error("a change under files kept the key")
	}
	for name, s := range map[string]config.Suite{
		"env": {Run: go1.Run, Files: go1.Files, Env: map[string]string{"CGO_ENABLED": "0"}},
		"dir": {Run: go1.Run, Files: go1.Files, Dir: "web"},
		"run": {Run: []string{"go", "test", "-race", "./..."}, Files: go1.Files},
	} {
		if k, _ := Key(ctx(t), dir, c1, "go", s); k == k1 {
			t.Errorf("%s: the key didn't change", name)
		}
	}
	if k, _ := Key(ctx(t), dir, c1, "vet", go1); k == k1 {
		t.Error("another check name kept the key")
	}

	all, _ := Files(ctx(t), dir, c1, nil)
	if len(all) != 4 {
		t.Errorf("no globs: %v, want every file", all)
	}
	if _, err := Key(ctx(t), dir, "--output=x", "go", go1); err == nil || !strings.Contains(err.Error(), "not a revision") {
		t.Errorf("an option as the commit: %v", err)
	}
}

func TestTouched(t *testing.T) {
	t.Parallel()
	dir := repo(t)
	base := commit(t, dir, map[string]string{"web/a.ts": "a", "x.go": "x"})
	run(t, dir, "checkout", "-q", "-b", "feat")
	head := commit(t, dir, map[string]string{"internal/y.go": "y", "docs/z.md": "z"})
	got, err := Touched(ctx(t), dir, base, head, []string{"**/*.go"})
	if err != nil || strings.Join(got, ",") != "internal/y.go" {
		t.Errorf("touched go: %v %v", got, err)
	}
	if got, _ := Touched(ctx(t), dir, base, head, []string{"web/**"}); len(got) != 0 {
		t.Errorf("touched web: %v, want none", got)
	}
}
