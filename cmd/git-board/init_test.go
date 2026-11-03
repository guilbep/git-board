package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghlabels"
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
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

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

// The global file's template starts a new board, and never changes one
// that exists: init refuses, the file and what loads from it stay.
func TestGlobalTemplateLeavesABoardAlone(t *testing.T) {
	xdg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", xdg)
	if err := os.MkdirAll(filepath.Join(xdg, "git-board"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(xdg, "git-board", "config.toml"), []byte(`template = "enac"`), 0o644); err != nil {
		t.Fatal(err)
	}
	enac, _ := config.Template("enac")

	fresh := t.TempDir()
	os.Mkdir(filepath.Join(fresh, ".git"), 0o755)
	t.Chdir(fresh)
	if out, err := execute(t, []string{"init"}); err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	if got, _ := os.ReadFile(filepath.Join(fresh, config.FileName)); string(got) != enac {
		t.Error("a plain init didn't write the global file's template")
	}

	board := t.TempDir()
	os.Mkdir(filepath.Join(board, ".git"), 0o755)
	path := filepath.Join(board, config.FileName)
	if err := os.WriteFile(path, []byte(config.Default), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(board)
	if _, err := execute(t, []string{"init"}); err == nil {
		t.Error("init over a board succeeded")
	}
	if got, _ := os.ReadFile(path); string(got) != config.Default {
		t.Error("the board's file changed")
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if p := cfg.Fields["prio"].Options; len(p) == 0 || p[0] == "P0" {
		t.Errorf("the board's prio is %q: the template leaked in", p)
	}
}

// Origin on GitHub: the file starts as its labels and Project, the
// template under them.
func TestInitFromTheForge(t *testing.T) {
	labels, _ := os.ReadFile("../../internal/ghlabels/testdata/enac-labels.json")
	project, _ := os.ReadFile("../../internal/fromforge/testdata/enac-project.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/EPFL-ENAC/test/labels":
			w.Write(labels)
		case "/graphql":
			w.Write(project)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	saved := ghlabels.API
	t.Cleanup(func() { ghlabels.API = saved })
	ghlabels.API = srv.URL
	t.Setenv("GITHUB_TOKEN", "test")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "git@github.com:EPFL-ENAC/test.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	t.Chdir(root)
	out, err := execute(t, []string{"init", "--template", "enac"})
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	if !strings.Contains(out, "12 labels, Project EPFL-ENAC/53") {
		t.Errorf("out = %q", out)
	}
	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GithubProject.Number != 53 || cfg.Fields["moscow"].Project["Wont"] != "Won't" {
		t.Errorf("github-project %+v, moscow %+v", cfg.GithubProject, cfg.Fields["moscow"])
	}
}
