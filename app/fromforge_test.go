package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
)

// fakeGitHub serves EPFL-ENAC/test's labels and Projects, as GitHub has
// them, at GitHubAPI, with a token in the environment: labels nil
// answers 404, project the GraphQL answer. Serial: it sets GitHubAPI and
// the environment.
func fakeGitHub(t *testing.T, labels, project []byte) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/EPFL-ENAC/test/labels":
			if labels == nil {
				http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
				return
			}
			_, _ = w.Write(labels)
		case "/graphql":
			_, _ = w.Write(project)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	saved := GitHubAPI
	t.Cleanup(func() { GitHubAPI = saved })
	GitHubAPI = srv.URL
	t.Setenv("GITHUB_TOKEN", "test")
	return srv.URL
}

// noToken leaves the forges no token: none in the environment, no
// credential helper.
func noToken(t *testing.T) {
	t.Helper()
	for _, k := range []string{"GITHUB_TOKEN", "GH_TOKEN", "GITLAB_TOKEN"} {
		t.Setenv(k, "")
	}
	t.Setenv("HOME", t.TempDir())
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_CONFIG_GLOBAL", filepath.Join(t.TempDir(), "none"))
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// originRepo is a new repository whose origin is url.
func originRepo(t *testing.T, url string) string {
	t.Helper()
	root := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", url}} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return root
}

// v1's init, on the same fake GitHub, wrote these files and said these
// lines (testdata/init/v1-*: recorded from 7a546cc's git board init,
// the forge's URL as FORGE): git-board's init writes and says the same.
func TestInitFromTheForgeIsV1s(t *testing.T) {
	enacLabels := readFile(t, "testdata/labels/enac-labels.json")
	extraLabels := readFile(t, "testdata/init/labels-extra.json")
	enacProject := readFile(t, "testdata/init/enac-project.json")
	extraProject := readFile(t, "testdata/init/project-extra.json")
	for _, c := range []struct {
		name, template  string
		labels, project []byte
		token           bool
	}{
		{"enac", "enac", enacLabels, enacProject, true},
		{"default-enac", "", enacLabels, enacProject, true},
		{"default-extra", "", extraLabels, extraProject, true},
		{"enac-extra", "enac", extraLabels, extraProject, true},
		{"labels-only", "", enacLabels, []byte(`{"data":{"repository":{"projectsV2":{"nodes":[]}}}}`), true},
		{"project-error", "", enacLabels, []byte(`{"errors":[{"message":"Your token has not been granted the required scopes"}]}`), true},
		{"labels-error", "", nil, enacProject, true},
		{"notoken", "", enacLabels, nil, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			url := fakeGitHub(t, c.labels, c.project)
			if !c.token {
				noToken(t)
			}
			t.Setenv("XDG_CONFIG_HOME", t.TempDir())
			root := originRepo(t, "git@github.com:EPFL-ENAC/test.git")
			path, lines, err := InitConfig(t.Context(), root, c.template)
			if err != nil {
				t.Fatalf("%v\n%s", err, strings.Join(lines, "\n"))
			}
			if want := readFile(t, "testdata/init/v1-"+c.name+".toml"); string(readFile(t, path)) != string(want) {
				t.Errorf("the file isn't v1's:\n%s", readFile(t, path))
			}
			got := strings.ReplaceAll(strings.Join(lines, "\n")+"\n", url, "http://FORGE")
			want := string(readFile(t, "testdata/init/v1-"+c.name+".out"))
			if c.name == "notoken" { // the adapter's own error, v1's line around it
				prefix, suffix := "EPFL-ENAC/test not read (github: ", "): the template's labels and fields\n"
				if len(lines) != 1 || !strings.HasPrefix(got, prefix) || !strings.HasSuffix(got, suffix) || !strings.HasPrefix(want, prefix) || !strings.HasSuffix(want, suffix) {
					t.Errorf("said %q, v1 said %q", got, want)
				}
				return
			}
			if got != want {
				t.Errorf("said:\n%s\nv1 said:\n%s", got, want)
			}
		})
	}
}

// A board started from EPFL-ENAC/test agrees with it: every label the
// file wants is GitHub's as it is, or lives in the Project; ENAC's
// template, with no drift from it on the first day.
func TestAFileFromTheForgeAgreesWithIt(t *testing.T) {
	labels := readFile(t, "testdata/labels/enac-labels.json")
	fakeGitHub(t, labels, readFile(t, "testdata/init/enac-project.json"))
	repo := forge.Table{Kind: "github", Host: "github.com", Project: "EPFL-ENAC/test"}
	f, notes := ReadForge(t.Context(), repo)
	if len(notes) > 0 || len(f.Labels) != 12 || f.Project == nil {
		t.Fatalf("Read: %d labels, project %v, notes %q", len(f.Labels), f.Project, notes)
	}
	tmpl, _ := starter("enac")
	text, notes, err := MergeForge(tmpl, f)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(notes)
	cfg, err := checkText(text)
	if err != nil {
		t.Fatal(err)
	}
	gp, _ := cfg.Tools.GithubProject.Get()
	if gp.Owner != "EPFL-ENAC" || gp.Number != 53 {
		t.Errorf("github-project = %+v", gp)
	}
	if tp, _ := cfg.Tools.Template.Get(); tp.Name != "enac" {
		t.Errorf("template = %+v", tp)
	}
	for _, f := range Lint(nil, cfg) {
		if f.Check == CheckTemplate {
			t.Errorf("drift: %s", f.Message)
		}
	}
	for field, want := range map[string][]string{
		"status": {"backlog", "ready", "in-progress", "in-review", "ready-for-validation", "validated"},
		"prio":   {"P0", "P1", "P2", "P3"},
		"moscow": {"Must", "Should", "Could", "Wont"},
		"hold":   {"blocked", "paused"},
	} {
		if got := cfg.Schema.Fields[field].Options; !slices.Equal(got, want) {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
	if cfg.Schema.Fields["moscow"].Project["Wont"] != "Won't" || cfg.Schema.Fields["prio"].Descriptions["P0"] == "" {
		t.Errorf("moscow %+v, prio %+v", cfg.Schema.Fields["moscow"], cfg.Schema.Fields["prio"])
	}
	if c := PlanLabels("github.com/EPFL-ENAC/test", DesiredLabels(cfg.Schema), f.Labels, cfg.Held); len(c) > 0 {
		t.Errorf("lint --create-labels would change %+v", c)
	}
	if !strings.Contains(text, "# The workflow, in order") {
		t.Error("the template's comments are gone")
	}
}

// No token: the template, and a note saying what wasn't read.
func TestNoTokenIsTheTemplate(t *testing.T) {
	noToken(t)
	f, notes := ReadForge(t.Context(), forge.Table{Kind: "github", Host: "github.com", Project: "EPFL-ENAC/test"})
	if len(notes) == 0 || f.Project != nil || len(f.Labels) > 0 {
		t.Fatalf("notes %q, forge %+v", notes, f)
	}
}

// A GitLab origin starts the file from its labels, through its adapter:
// GitLab has no Projects, and init says so. Serial: it swaps
// OpenLabeler.
func TestInitFromAGitLabOrigin(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	var asked []forge.Table
	was := OpenLabeler
	t.Cleanup(func() { OpenLabeler = was })
	OpenLabeler = func(_ context.Context, f forge.Table) (forge.Labeler, error) {
		asked = append(asked, f)
		g := fake.New(f.Host + "/" + f.Project)
		for _, l := range []forge.Label{
			{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
			{Name: "prio:P0", Color: "B60205", Description: "on fire"},
			{Name: "needs-design", Color: "f9d0c4"},
		} {
			g.AddLabel(l)
		}
		return g, nil
	}
	root := originRepo(t, "git@gitlab.epfl.ch:enac/it4r/test.git")
	path, lines, err := InitConfig(t.Context(), root, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(asked) != 1 || asked[0].Kind != "gitlab" || asked[0].Host != "gitlab.epfl.ch" || asked[0].Project != "enac/it4r/test" {
		t.Errorf("asked %+v", asked)
	}
	want := []string{
		"from enac/it4r/test: 3 labels, Project none",
		"enac/it4r/test is on GitLab, which has no Projects: the template's fields",
	}
	if !slices.Equal(lines, want) {
		t.Errorf("said %q, want %q", lines, want)
	}
	cfg, err := checkText(string(readFile(t, path)))
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Schema
	if s.Fields["prio"].Colors["p0"] != "#b60205" || s.Fields["prio"].Descriptions["p0"] != "on fire" {
		t.Errorf("prio %+v", s.Fields["prio"])
	}
	if s.Fields["kind"].Colors["bug"] != "#d73a4a" || !slices.Contains(s.Labels.List, "needs-design") {
		t.Errorf("kind %+v, labels %+v", s.Fields["kind"], s.Labels)
	}
	if gp, _ := cfg.Tools.GithubProject.Get(); gp.Number != 0 {
		t.Errorf("github-project %+v", gp)
	}
}

// A forge with nothing to read, or no origin on a forge: the template,
// as it is.
func TestInitWithoutAForgeIsTheTemplate(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	was := OpenLabeler
	t.Cleanup(func() { OpenLabeler = was })
	OpenLabeler = func(_ context.Context, f forge.Table) (forge.Labeler, error) { return fake.New("empty"), nil }
	for _, url := range []string{"git@github.com:o/empty.git", "/elsewhere/repo.git"} {
		root := originRepo(t, url)
		path, lines, err := InitConfig(t.Context(), root, "")
		if err != nil {
			t.Fatal(err)
		}
		if len(lines) != 0 || string(readFile(t, path)) != DefaultConfig {
			t.Errorf("%s: said %q, the file the default: %v", url, lines, string(readFile(t, path)) == DefaultConfig)
		}
	}
}

// MergeForge writes only through conf's checks: a template that doesn't
// read is refused.
func TestMergeForgeChecksTheTemplate(t *testing.T) {
	t.Parallel()
	if _, _, err := MergeForge("[branches]\nkinds = { bug = \"fix\" }\n", FromForge{Labels: []forge.Label{{Name: "x"}}}); err == nil || !strings.Contains(err.Error(), "the template") {
		t.Errorf("err = %v", err)
	}
}
