package server

import (
	"net/http"
	"reflect"
	"testing"

	"github.com/git-bug/git-bug/bridge/core"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

func addBridge(t *testing.T, s *apiServer, name string, kv map[string]string) {
	t.Helper()
	for k, v := range kv {
		if err := core.SetBridgeConfig(s.b.TestCache(), name, k, v); err != nil {
			t.Fatal(err)
		}
	}
}

type settingsBody struct {
	Bridges []gitbugio.Bridge `json:"bridges"`
	Uploads struct {
		Enabled bool `json:"enabled"`
	} `json:"uploads"`
	GithubProject config.GithubProject `json:"github_project"`
	ConfigPath    string               `json:"config_path"`
	Cycle         config.Stages        `json:"cycle"`
}

func TestSettingsListAndToggleBridgeSync(t *testing.T) {
	t.Setenv("ACCESS_KEY_ID", "")
	ts, s := newIssueServer(t)
	s.cfg.GithubProject = config.GithubProject{Owner: "guilbep", Number: 1}
	s.cfg.Fields = map[string]config.Field{"status": {Options: []string{"backlog", "ready", "in-progress", "qa", "done"}}}
	addBridge(t, s, "gh", map[string]string{"target": "github", "owner": "guilbep", "project": "git-bug-sync-test", "default-login": "guilbep", "sync": "true"})
	addBridge(t, s, "gl", map[string]string{"target": "gitlab", "project-id": "42400", "base-url": "https://gitlab.epfl.ch/", "default-login": "guilbert"})

	var got settingsBody
	if code := call(t, "GET", ts.URL+"/api/settings", nil, &got); code != http.StatusOK {
		t.Fatalf("GET: %d", code)
	}
	want := []gitbugio.Bridge{
		{Name: "gh", Target: "github", Where: "https://github.com/guilbep/git-bug-sync-test", Sync: true, CanSync: true},
		{Name: "gl", Target: "gitlab", Where: "https://gitlab.epfl.ch/ project 42400", CanSync: true},
	}
	if len(got.Bridges) != 2 || got.Bridges[0] != want[0] || got.Bridges[1] != want[1] {
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
	bridges, _ := s.b.SyncBridges()
	if len(bridges) != 1 || bridges[0].Name != "gl" {
		t.Errorf("sync bridges after the toggles: %+v, want gl only", bridges)
	}
	if code := call(t, "PUT", ts.URL+"/api/bridges/nope/sync", map[string]bool{"sync": true}, nil); code != http.StatusNotFound {
		t.Errorf("an unknown bridge: %d, want 404", code)
	}

	s.syncing.Store(true)
	if code := call(t, "PUT", ts.URL+"/api/bridges/gh/sync", map[string]bool{"sync": true}, nil); code != http.StatusConflict {
		t.Errorf("during a forge sync: %d, want 409", code)
	}
}
