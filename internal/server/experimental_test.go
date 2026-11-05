package server

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/cycle"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// allFeatures turns every experimental feature on: the fixtures of the
// cycle's, the agents' and the worktrees' tests.
var allFeatures = []string{config.FeatureCycle, config.FeatureAgents, config.FeatureWorktrees}

// Off, a feature's routes are not there: 404, the error naming the switch.
func TestExperimentalRoutesOff(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	id, err := b.NewIssue("An issue", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	// a [cycle] table alone doesn't turn the cycle on
	ts := newTestServer(t, b, &config.Config{Cycle: cycle.Config{On: true}})
	for _, route := range []struct{ method, path, feature string }{
		{"GET", "/api/issues/" + id[:8] + "/cycle", "cycle"},
		{"POST", "/api/issues/" + id[:8] + "/cycle/approve-plan", "cycle"},
		{"GET", "/api/issues/" + id[:8] + "/tests", "cycle"},
		{"GET", "/api/checks/setup", "cycle"},
		{"GET", "/api/work", "cycle"},
		{"PUT", "/api/me/view", "cycle"},
		{"GET", "/api/issues/" + id[:8] + "/agent", "agents"},
		{"POST", "/api/issues/" + id[:8] + "/agent/go", "agents"},
		{"GET", "/api/worktrees", "worktrees"},
		{"POST", "/api/worktrees/init", "worktrees"},
	} {
		req, _ := http.NewRequest(route.method, ts.URL+route.path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		var body struct{ Error, Code string }
		_ = json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound || body.Code != "experimental" || !strings.Contains(body.Error, "git config board.experimental "+route.feature) {
			t.Errorf("%s %s: %d %+v, want 404 naming the switch for %s", route.method, route.path, resp.StatusCode, body, route.feature)
		}
	}

	// /api/me says none is on, and the issue page is the classic one
	var me meResponse
	getJSON(t, ts.URL+"/api/me", &me)
	if me.Experimental == nil || len(me.Experimental) != 0 || me.View != "classic" {
		t.Errorf("me: experimental %v, view %q; want none, classic", me.Experimental, me.View)
	}
}

// On, the routes answer as they always did, and /api/me lists the feature.
func TestExperimentalRoutesOn(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	id, err := b.NewIssue("An issue", twoCriteria, []string{"status:ready"})
	if err != nil {
		t.Fatal(err)
	}
	cfg := cycleConfig(nil, cycle.Config{On: true})
	cfg.Features = []string{config.FeatureCycle}
	ts := newTestServer(t, b, cfg)
	var c cycleAnswer
	if resp := getJSON(t, ts.URL+"/api/issues/"+id[:8]+"/cycle", &c); resp.StatusCode != http.StatusOK || c.Step == 0 {
		t.Errorf("cycle: %d, step %d", resp.StatusCode, c.Step)
	}
	var me meResponse
	getJSON(t, ts.URL+"/api/me", &me)
	if !slices.Equal(me.Experimental, []string{"cycle"}) || me.View != "cycle" {
		t.Errorf("me: experimental %v, view %q; want cycle, cycle", me.Experimental, me.View)
	}
	// agents stay off: the route is not there
	resp := getJSON(t, ts.URL+"/api/issues/"+id[:8]+"/agent", nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("agent: %d, want 404", resp.StatusCode)
	}
}

// Settings › Experimental writes [experimental], and the route opens.
func TestExperimentalSection(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), config.FileName)
	if err := os.WriteFile(path, []byte("[repos]\nlist = [\"o/r\"]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	s := &apiServer{cfg: cfg, b: storetest.New(t), me: "guilbep", cfgPath: path}
	ts := serve(t, s)
	var m configModel
	getJSON(t, ts.URL+"/api/config/model", &m)
	if len(m.Experimental.Features) != 0 || len(m.Experimental.Known) != 3 {
		t.Fatalf("model: %+v", m.Experimental)
	}
	body, _ := json.Marshal(map[string][]string{"features": {"worktrees"}})
	req, _ := http.NewRequest("PUT", ts.URL+"/api/config/section/experimental?base="+m.Hash, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	out, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save: %d %s", resp.StatusCode, out)
	}
	text, _ := os.ReadFile(path)
	if !strings.Contains(string(text), "[experimental]\nfeatures = [\"worktrees\"]\n") {
		t.Errorf("file:\n%s", text)
	}
	if !s.config().Experimental(config.FeatureWorktrees) {
		t.Error("the server still has worktrees off")
	}
}
