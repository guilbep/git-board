package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
)

var (
	ghForge = config.Forge{Name: "gh", Kind: "github", Host: "github.com", Project: "guilbep/git-bug-sync-test", Login: "guilbep", Sync: true}
	glForge = config.Forge{Name: "gl", Kind: "gitlab", Host: "gitlab.epfl.ch", Project: "42400", Login: "guilbert", Sync: true}
)

// forgeServer is newIssueServer with these forges, in a settings file of
// the test's own.
func forgeServer(t *testing.T, forges ...config.Forge) (*httptest.Server, *apiServer) {
	t.Helper()
	ts, s := newIssueServer(t)
	text := ""
	for _, f := range forges {
		text = config.WithForge(text, f)
	}
	if err := config.Save(s.cfgPath, []byte(text)); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(s.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	s.cfg = cfg
	return ts, s
}

type settingsBody struct {
	Uploads struct {
		Enabled bool `json:"enabled"`
	} `json:"uploads"`
	GithubProject config.GithubProject `json:"github_project"`
	ConfigPath    string               `json:"config_path"`
	Cycle         config.Stages        `json:"cycle"`
}

func TestSettingsListAndToggleBridgeSync(t *testing.T) {
	t.Setenv("ACCESS_KEY_ID", "")
	gl := glForge
	gl.Sync = false
	ts, s := forgeServer(t, ghForge, gl)
	s.cfg.GithubProject = config.GithubProject{Owner: "guilbep", Number: 1}
	s.cfg.Fields = map[string]config.Field{"status": {Options: []string{"backlog", "ready", "in-progress", "qa", "done"}}}

	var got settingsBody
	if code := call(t, "GET", ts.URL+"/api/settings", nil, &got); code != http.StatusOK {
		t.Fatalf("GET: %d", code)
	}
	var whole wholeStatus
	if code := call(t, "GET", ts.URL+"/api/sync/all", nil, &whole); code != http.StatusOK {
		t.Fatalf("GET: %d", code)
	}
	want := []bridge{
		{Name: "gh", Target: "github", Project: "guilbep/git-bug-sync-test", Where: "https://github.com/guilbep/git-bug-sync-test", Sync: true},
		{Name: "gl", Target: "gitlab", Project: "42400", Where: "https://gitlab.epfl.ch/projects/42400"},
	}
	if !reflect.DeepEqual(whole.Forges, want) || whole.Remote != nil || whole.Project.Project != "guilbep/1" {
		t.Errorf("forges %+v\nwant %+v\n%+v", whole.Forges, want, whole)
	}
	if got.Uploads.Enabled || got.GithubProject.Number != 1 || got.ConfigPath == "" {
		t.Errorf("settings %+v", got)
	}
	// the stages, guessed from the status names, so the page can say so
	if !got.Cycle.Guessed || got.Cycle.Ready != "ready" || !reflect.DeepEqual(got.Cycle.Validation, []string{"qa"}) || got.Cycle.Closing != "done" {
		t.Errorf("cycle %+v", got.Cycle)
	}

	if code := call(t, "PUT", ts.URL+"/api/bridges/gl/sync", map[string]bool{"sync": true}, nil); code != http.StatusOK {
		t.Fatalf("PUT: %d", code)
	}
	if code := call(t, "PUT", ts.URL+"/api/bridges/gh/sync", map[string]bool{"sync": false}, nil); code != http.StatusOK {
		t.Fatalf("PUT: %d", code)
	}
	if bridges := s.syncForges(); len(bridges) != 1 || bridges[0].Name != "gl" {
		t.Errorf("sync bridges after the toggles: %+v, want gl only", bridges)
	}
	if saved, err := config.Load(s.cfgPath); err != nil || len(saved.Forges) != 2 || saved.Forges[0].Sync || !saved.Forges[1].Sync {
		t.Errorf("the file has %+v (%v)", saved.Forges, err)
	}
	if code := call(t, "PUT", ts.URL+"/api/bridges/nope/sync", map[string]bool{"sync": true}, nil); code != http.StatusNotFound {
		t.Errorf("an unknown bridge: %d, want 404", code)
	}

	s.syncing.Store(true)
	if code := call(t, "PUT", ts.URL+"/api/bridges/gh/sync", map[string]bool{"sync": true}, nil); code != http.StatusConflict {
		t.Errorf("during a forge sync: %d, want 409", code)
	}
}

// Settings › Sync's Project line: Save writes the table, the file's comments
// kept, write = true for the same Project alone, and drops the last check;
// a bad owner or number changes nothing; Remove takes the table out.
func TestSetGithubProject(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	src := "# the board\n[github-project]\nowner = \"old\"\nnumber = 1\nwrite = true\n\n# who does what\n[roles]\nlead-dev = \"guilbep\"\n"
	if err := config.Save(s.cfgPath, []byte(src)); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(s.cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	s.cfg, s.last = cfg, &syncReport{Title: "Old"}
	read := func() string {
		data, err := os.ReadFile(s.cfgPath)
		if err != nil {
			t.Fatal(err)
		}
		return string(data)
	}

	for _, bad := range []map[string]any{{"owner": "-EPFL", "number": 40}, {"owner": "EPFL/ENAC", "number": 40}, {"owner": "", "number": 40}, {"owner": "EPFL-ENAC", "number": 0}, {"owner": "EPFL-ENAC", "number": 4.5}} {
		if code := call(t, "PUT", ts.URL+"/api/github-project", bad, nil); code != http.StatusBadRequest {
			t.Errorf("%v: %d, want 400", bad, code)
		}
	}
	if got := read(); got != src {
		t.Fatalf("a refused save wrote:\n%s", got)
	}

	var st syncStatus
	if code := call(t, "PUT", ts.URL+"/api/github-project", map[string]any{"owner": "EPFL-ENAC", "number": 40}, &st); code != http.StatusOK {
		t.Fatalf("PUT: %d", code)
	}
	if st.Project != "EPFL-ENAC/40" || st.Write || st.Last != nil {
		t.Errorf("status %+v, want EPFL-ENAC/40, read only, no check yet", st)
	}
	if want := strings.Replace(strings.Replace(src, `"old"`, `"EPFL-ENAC"`, 1), "number = 1\nwrite = true", "number = 40", 1); read() != want {
		t.Errorf("the file:\n%s\nwant:\n%s", read(), want)
	}
	if gp := s.config().GithubProject; gp.Owner != "EPFL-ENAC" || gp.Number != 40 {
		t.Errorf("served %+v", gp)
	}
	// the same Project again, write = true set by hand: it stays
	if err := config.Save(s.cfgPath, []byte(strings.Replace(read(), "number = 40", "number = 40\nwrite = true", 1))); err != nil {
		t.Fatal(err)
	}
	if cfg, err = config.Load(s.cfgPath); err != nil {
		t.Fatal(err)
	}
	s.setConfig(cfg)
	if code := call(t, "PUT", ts.URL+"/api/github-project", map[string]any{"owner": "epfl-enac", "number": 40}, &st); code != http.StatusOK || !st.Write {
		t.Errorf("the same Project: %d, %+v, want still written to", code, st)
	}

	if code := call(t, "DELETE", ts.URL+"/api/github-project", nil, &st); code != http.StatusOK || st.Project != "" {
		t.Fatalf("DELETE: %d, %+v", code, st)
	}
	if got := read(); got != "# the board\n\n# who does what\n[roles]\nlead-dev = \"guilbep\"\n" {
		t.Errorf("after Remove:\n%q", got)
	}
}
