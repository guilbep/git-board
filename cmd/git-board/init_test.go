package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/guilbep/git-board/internal/config"
)

func TestInitWritesTheDefaultAtTheRootOnce(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "web", "src")
	for _, d := range []string{filepath.Join(root, ".git"), sub} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(sub)

	if out, err := execute(t, []string{"init"}); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	got, err := os.ReadFile(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != config.Default {
		t.Error("the file isn't the default")
	}

	if _, err := execute(t, []string{"init"}); err == nil {
		t.Error("a second init succeeded, want it to refuse to overwrite")
	}
}
