package fromforge_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/fromforge"
	"github.com/guilbep/git-board/internal/ghlabels"
)

// fakeGitHub answers EPFL-ENAC/test's labels and Project as GitHub has
// them (docs/prd/board-template.md).
func fakeGitHub(t *testing.T) {
	labels, _ := os.ReadFile("../ghlabels/testdata/enac-labels.json")
	project, _ := os.ReadFile("testdata/enac-project.json")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/repos/EPFL-ENAC/test/labels":
			w.Write(labels)
		case r.URL.Path == "/graphql":
			w.Write(project)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	saved := ghlabels.API
	t.Cleanup(func() { ghlabels.API = saved })
	ghlabels.API = srv.URL
	t.Setenv("GITHUB_TOKEN", "test")
}

// A board started from EPFL-ENAC/test agrees with it: every label the
// file wants is GitHub's as it is, or lives in the Project.
func TestAFileFromTheForgeAgreesWithIt(t *testing.T) {
	fakeGitHub(t)
	f, notes := fromforge.Read(context.Background(), "EPFL-ENAC/test")
	if len(notes) > 0 || len(f.Labels) != 12 || f.Project == nil {
		t.Fatalf("Read: %d labels, project %v, notes %q", len(f.Labels), f.Project, notes)
	}
	tmpl, _ := config.Template("enac")
	text, notes, err := fromforge.Merge(tmpl, f)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(notes)
	cfg, _, err := config.Check([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GithubProject.Owner != "EPFL-ENAC" || cfg.GithubProject.Number != 53 {
		t.Errorf("github-project = %+v", cfg.GithubProject)
	}
	for field, want := range map[string][]string{
		"status": {"backlog", "ready", "in-progress", "in-review", "ready-for-validation", "validated"},
		"prio":   {"P0", "P1", "P2", "P3"},
		"moscow": {"Must", "Should", "Could", "Wont"},
		"hold":   {"blocked", "paused"},
	} {
		if got := cfg.Fields[field].Options; !slices.Equal(got, want) {
			t.Errorf("%s = %q, want %q", field, got, want)
		}
	}
	if cfg.Fields["moscow"].Project["Wont"] != "Won't" || cfg.Fields["prio"].Descriptions["P0"] == "" {
		t.Errorf("moscow %+v, prio %+v", cfg.Fields["moscow"], cfg.Fields["prio"])
	}
	key := ghlabels.Key("EPFL-ENAC/test")
	for _, want := range ghlabels.Desired(cfg) {
		if cfg.Held(key, want.Name) {
			continue
		}
		if s := ghlabels.State(want, f.Labels); s != "same" {
			t.Errorf("%s: %s (want %s %q)", want.Name, s, want.Color, want.Description)
		}
	}
	if !strings.Contains(text, "# The workflow, in order") {
		t.Error("the template's comments are gone")
	}
}

// No token: the template, and a note saying what wasn't read.
func TestNoTokenIsTheTemplate(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("HOME", t.TempDir()) // no credential helper
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	f, notes := fromforge.Read(context.Background(), "EPFL-ENAC/test")
	if len(notes) == 0 || f.Project != nil || len(f.Labels) > 0 {
		t.Fatalf("notes %q, forge %+v", notes, f)
	}
}
