package server

import (
	"net/http"
	"net/http/httptest"
	"reflect"
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
	Bridges []bridge `json:"bridges"`
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
	want := []bridge{
		{Name: "gh", Target: "github", Where: "https://github.com/guilbep/git-bug-sync-test", Sync: true, CanSync: true},
		{Name: "gl", Target: "gitlab", Where: "https://gitlab.epfl.ch/projects/42400", CanSync: true},
	}
	if !reflect.DeepEqual(got.Bridges, want) {
		t.Errorf("bridges %+v\nwant %+v", got.Bridges, want)
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
