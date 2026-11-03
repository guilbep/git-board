package main

import (
	"io"
	"net/http"
	"net/http/httptest"
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

func TestInitFromATemplate(t *testing.T) {
	enac, _ := config.Template("enac")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, enac) }))
	defer srv.Close()
	file := filepath.Join(t.TempDir(), "mine.toml")
	if err := os.WriteFile(file, []byte(enac), 0o644); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(t.TempDir(), "broken.toml")
	if err := os.WriteFile(broken, []byte("[branches]\nkinds = { bug = \"fix\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, spec := range []string{"enac", file, srv.URL + "/enac.toml"} {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Chdir(root)
		if out, err := execute(t, []string{"init", "--template", spec}); err != nil {
			t.Fatalf("init --template %s: %v\n%s", spec, err, out)
		}
		if got, _ := os.ReadFile(filepath.Join(root, config.FileName)); string(got) != enac {
			t.Errorf("%s: the file isn't ENAC's template", spec)
		}
	}
	for _, spec := range []string{"nope", broken} {
		root := t.TempDir()
		os.Mkdir(filepath.Join(root, ".git"), 0o755)
		t.Chdir(root)
		if _, err := execute(t, []string{"init", "--template", spec}); err == nil {
			t.Errorf("init --template %s succeeded", spec)
		}
		if _, err := os.Stat(filepath.Join(root, config.FileName)); err == nil {
			t.Errorf("%s: a file was written", spec)
		}
	}
}
