package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/conf"
)

// v1's init tests (7a546cc: cmd/git-board/init_test.go). They chdir and
// set the environment: serial.

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

	if out, errs, code := run(t, "init"); code != 0 {
		t.Fatalf("init: exit %d\n%s%s", code, out, errs)
	}
	if got, err := os.ReadFile(filepath.Join(root, conf.FileName)); err != nil || string(got) != app.DefaultConfig {
		t.Errorf("the file isn't the default: %v", err)
	}
	if _, _, code := run(t, "init"); code == 0 {
		t.Error("a second init succeeded, want it to refuse to overwrite")
	}
}

func TestInitFromATemplate(t *testing.T) {
	enac, err := app.StartConfig("enac")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.WriteString(w, enac) }))
	defer srv.Close()
	file := filepath.Join(t.TempDir(), "mine.toml")
	if err := os.WriteFile(file, []byte(enac), 0o644); err != nil {
		t.Fatal(err)
	}
	broken := filepath.Join(t.TempDir(), "broken.toml")
	if err := os.WriteFile(broken, []byte("[branches]\nkinds = { bug = \"fix\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	for _, spec := range []string{"enac", file, srv.URL + "/enac.toml"} {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Chdir(root)
		if out, errs, code := run(t, "init", "--template", spec); code != 0 {
			t.Fatalf("init --template %s: exit %d\n%s%s", spec, code, out, errs)
		}
		if got, _ := os.ReadFile(filepath.Join(root, conf.FileName)); string(got) != enac {
			t.Errorf("%s: the file isn't ENAC's template", spec)
		}
	}
	for _, spec := range []string{"nope", broken} {
		root := t.TempDir()
		if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Chdir(root)
		if _, _, code := run(t, "init", "--template", spec); code == 0 {
			t.Errorf("init --template %s succeeded", spec)
		}
		if _, err := os.Stat(filepath.Join(root, conf.FileName)); err == nil {
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
	enac, err := app.StartConfig("enac")
	if err != nil {
		t.Fatal(err)
	}

	fresh := t.TempDir()
	if err := os.Mkdir(filepath.Join(fresh, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(fresh)
	if out, errs, code := run(t, "init"); code != 0 {
		t.Fatalf("init: exit %d\n%s%s", code, out, errs)
	}
	if got, _ := os.ReadFile(filepath.Join(fresh, conf.FileName)); string(got) != enac {
		t.Error("a plain init didn't write the global file's template")
	}

	board := t.TempDir()
	if err := os.Mkdir(filepath.Join(board, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(board, conf.FileName)
	if err := os.WriteFile(path, []byte(app.DefaultConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(board)
	if _, _, code := run(t, "init"); code == 0 {
		t.Error("init over a board succeeded")
	}
	if got, _ := os.ReadFile(path); string(got) != app.DefaultConfig {
		t.Error("the board's file changed")
	}
	f, err := conf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := app.ReadConfig(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	if p := cfg.Schema.Fields["prio"].Options; len(p) == 0 || p[0] == "P0" {
		t.Errorf("the board's prio is %q: the template leaked in", p)
	}
}

// Origin on GitHub: the file starts as its labels and Project, the
// template under them, and init says so, as v1's did.
func TestInitFromTheForge(t *testing.T) {
	labels, err := os.ReadFile("../../app/testdata/labels/enac-labels.json")
	if err != nil {
		t.Fatal(err)
	}
	project, err := os.ReadFile("../../app/testdata/init/enac-project.json")
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/EPFL-ENAC/test/labels":
			_, _ = w.Write(labels)
		case "/graphql":
			_, _ = w.Write(project)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	saved := app.GitHubAPI
	t.Cleanup(func() { app.GitHubAPI = saved })
	app.GitHubAPI = srv.URL
	t.Setenv("GITHUB_TOKEN", "test")
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", "git@github.com:EPFL-ENAC/test.git"}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	t.Chdir(root)
	out, errs, code := run(t, "init", "--template", "enac")
	if code != 0 {
		t.Fatalf("init: exit %d\n%s%s", code, out, errs)
	}
	path := filepath.Join(root, conf.FileName)
	want := "from EPFL-ENAC/test: 12 labels, Project EPFL-ENAC/53, ENAC-IT4R Template\nwrote " + path + ": edit it in Settings › Project file (git board), or by hand\n"
	if out != want {
		t.Errorf("out = %q, want %q", out, want)
	}
	f, err := conf.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := app.ReadConfig(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	gp, _ := cfg.Tools.GithubProject.Get()
	if gp.Number != 53 || cfg.Schema.Fields["moscow"].Project["Wont"] != "Won't" {
		t.Errorf("github-project %+v, moscow %+v", gp, cfg.Schema.Fields["moscow"])
	}
	if strings.Contains(out, "v0.11") {
		t.Errorf("out names v0.11: %q", out)
	}
}
