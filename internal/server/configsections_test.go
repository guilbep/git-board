package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The settings' forms: one GET for every section typed, one PUT per
// section — only its tables change in the file, the save is live, the
// base hash holds the usual compare-and-swap.

type configModelResponse struct {
	Hash   string `json:"hash"`
	Exists bool   `json:"exists"`
	Fields map[string]struct {
		Single       bool              `json:"single"`
		Options      []string          `json:"options"`
		Numeric      bool              `json:"numeric"`
		Type         string            `json:"type"`
		Unit         string            `json:"unit"`
		Colors       map[string]string `json:"colors"`
		Descriptions map[string]string `json:"descriptions"`
		Project      map[string]string `json:"project"`
	} `json:"fields"`
	Labels struct {
		List         []string          `json:"list"`
		Colors       map[string]string `json:"colors"`
		Descriptions map[string]string `json:"descriptions"`
	} `json:"labels"`
	Views map[string]struct {
		Filter string `json:"filter"`
	} `json:"views"`
	Sprints struct {
		LengthDays   int    `json:"length_days"`
		StartWeekday string `json:"start_weekday"`
		List         []struct {
			Slug  string `json:"slug"`
			Title string `json:"title"`
			Start string `json:"start"`
		} `json:"list"`
	} `json:"sprints"`
	Roles    map[string]string `json:"roles"`
	Branches struct {
		Base  string            `json:"base"`
		Name  string            `json:"name"`
		Kinds map[string]string `json:"kinds"`
	} `json:"branches"`
	Cycle struct {
		On         bool              `json:"on"`
		Ready      string            `json:"ready"`
		InProgress []string          `json:"in-progress"`
		Kinds      map[string]string `json:"kinds"`
		Tests      map[string]struct {
			Run []string `json:"run"`
		} `json:"tests"`
	} `json:"cycle"`
	Repos struct {
		List      []string `json:"list"`
		Assignees string   `json:"assignees"`
	} `json:"repos"`
	Uploads struct {
		Prefix string `json:"prefix"`
	} `json:"uploads"`
	Release struct {
		Environments []string `json:"environments"`
	} `json:"release"`
}

type sectionSave struct {
	Check struct {
		OK       bool     `json:"ok"`
		Warnings []string `json:"warnings"`
	} `json:"check"`
	Model configModelResponse `json:"model"`
}

const sectionFile = "# our board\n" +
	"[fields.status]\nsingle = true\noptions = [\"backlog\", \"ready\"]\n\n" +
	"[labels]\nlist = [\"needs-design\"]\n\n" +
	"[roles]\npm = \"claire\"\n\n" +
	"[views.open]\nlayout = \"table\"\n"

func TestConfigModelServesEverySection(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	s.cfgPath = filepath.Join(t.TempDir(), ".git-board.toml")
	if err := os.WriteFile(s.cfgPath, []byte(sectionFile), 0o644); err != nil {
		t.Fatal(err)
	}
	var m configModelResponse
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)
	if !m.Exists || m.Hash == "" {
		t.Fatalf("GET %+v", m)
	}
	if f := m.Fields["status"]; !f.Single || strings.Join(f.Options, ",") != "backlog,ready" {
		t.Errorf("fields: %+v", f)
	}
	if strings.Join(m.Labels.List, ",") != "needs-design" {
		t.Errorf("labels: %+v", m.Labels)
	}
	if m.Roles["pm"] != "claire" {
		t.Errorf("roles: %+v", m.Roles)
	}
	if v := m.Views["open"]; v.Filter != "" {
		t.Errorf("views: %+v", m.Views)
	}
	// the sections without a table in the file: zero values, empty maps
	if m.Branches.Base != "" || len(m.Branches.Kinds) != 0 {
		t.Errorf("branches: %+v", m.Branches)
	}
	if m.Cycle.On || len(m.Cycle.Tests) != 0 {
		t.Errorf("cycle: %+v", m.Cycle)
	}
	if len(m.Repos.List) != 0 || m.Repos.Assignees != "native" {
		t.Errorf("repos: %+v", m.Repos)
	}
	if len(m.Release.Environments) != 0 {
		t.Errorf("release: %+v", m.Release)
	}
}

func TestConfigSectionSavesM2(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	s.cfgPath = filepath.Join(t.TempDir(), ".git-board.toml")
	if err := os.WriteFile(s.cfgPath, []byte(sectionFile), 0o644); err != nil {
		t.Fatal(err)
	}
	var m configModelResponse
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)

	// branches: base, name, kinds
	var saved sectionSave
	bbody := map[string]any{
		"base":  "trunk",
		"name":  "{kind}/{number}-{slug}",
		"kinds": map[string]string{"bug": "fix", "feat": "feat"},
	}
	if code := call(t, "PUT", ts.URL+"/api/config/section/branches?base="+m.Hash, bbody, &saved); code != http.StatusOK {
		t.Fatalf("branches save: %d", code)
	}
	got, _ := os.ReadFile(s.cfgPath)
	want := sectionFile + "\n[branches]\nbase = \"trunk\"\nname = \"{kind}/{number}-{slug}\"\n\n[branches.kinds]\nbug = \"fix\"\nfeat = \"feat\"\n"
	if string(got) != want {
		t.Errorf("branches in file:\n%q\nwant\n%q", got, want)
	}

	// cycle: on with stages, a suite with its env
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)
	cbody := map[string]any{
		"on": true,
		"cycle": map[string]any{
			"ready":       "ready",
			"in-progress": []string{"in-progress"},
			"tests": map[string]any{
				"unit": map[string]any{
					"run":   []string{"go", "test", "./..."},
					"where": "ci",
					"env":   map[string]string{"CGO": "0"},
				},
			},
		},
	}
	if code := call(t, "PUT", ts.URL+"/api/config/section/cycle?base="+m.Hash, cbody, &saved); code != http.StatusOK {
		t.Fatalf("cycle save: %d", code)
	}
	got, _ = os.ReadFile(s.cfgPath)
	if !strings.Contains(string(got), "[cycle]\n") || !strings.Contains(string(got), "[cycle.tests.unit]\n") ||
		!strings.Contains(string(got), "where = \"ci\"") || !strings.Contains(string(got), "[cycle.tests.unit.env]\nCGO = \"0\"") {
		t.Errorf("cycle in file:\n%s", got)
	}
	// the fields' and branches' tables are untouched
	if !strings.Contains(string(got), "[branches]\nbase = \"trunk\"") {
		t.Errorf("branches went:\n%s", got)
	}

	// cycle off: every [cycle.*] table goes
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)
	if code := call(t, "PUT", ts.URL+"/api/config/section/cycle?base="+m.Hash, map[string]any{"on": false}, &saved); code != http.StatusOK {
		t.Fatalf("cycle off: %d", code)
	}
	got, _ = os.ReadFile(s.cfgPath)
	if strings.Contains(string(got), "[cycle") {
		t.Errorf("cycle stayed off:\n%s", got)
	}

	// release: environments, images, chart, deploy
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)
	rbody := map[string]any{
		"environments": []string{"dev", "prod"},
		"checks":       "make ci",
		"images":       []map[string]string{{"name": "board", "context": "."}},
		"chart":        map[string]string{"path": "chart", "registry": "registry.epfl.ch/x"},
		"deploy": []map[string]string{
			{"repo": "epfl-enac/app-deploy", "path": "apps/app/{env}", "registry": "registry.epfl.ch/x"},
		},
	}
	if code := call(t, "PUT", ts.URL+"/api/config/section/release?base="+m.Hash, rbody, &saved); code != http.StatusOK {
		t.Fatalf("release save: %d", code)
	}
	got, _ = os.ReadFile(s.cfgPath)
	if !strings.Contains(string(got), "environments = [\"dev\", \"prod\"]") || !strings.Contains(string(got), "[[release.deploy]]") ||
		!strings.Contains(string(got), "[release.chart]") {
		t.Errorf("release in file:\n%s", got)
	}
	// live: the board's settings serve it at once
	var settings struct {
		Uploads struct {
			Enabled bool `json:"enabled"`
		} `json:"uploads"`
	}
	call(t, "GET", ts.URL+"/api/settings", nil, &settings)

	// uploads and repos round-trip too
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)
	if code := call(t, "PUT", ts.URL+"/api/config/section/repos?base="+m.Hash, map[string]any{"list": []string{"epfl-enac/one"}}, &saved); code != http.StatusOK {
		t.Fatalf("repos save: %d", code)
	}
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)
	if code := call(t, "PUT", ts.URL+"/api/config/section/uploads?base="+m.Hash, map[string]any{"prefix": "gb/x"}, &saved); code != http.StatusOK {
		t.Fatalf("uploads save: %d", code)
	}
	got, _ = os.ReadFile(s.cfgPath)
	if !strings.Contains(string(got), "[repos]\nlist = [\"epfl-enac/one\"]") || !strings.Contains(string(got), "prefix = \"gb/x\"") {
		t.Errorf("repos/uploads in file:\n%s", got)
	}
}

func TestConfigSectionSaves(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	s.cfgPath = filepath.Join(t.TempDir(), ".git-board.toml")
	if err := os.WriteFile(s.cfgPath, []byte(sectionFile), 0o644); err != nil {
		t.Fatal(err)
	}
	var m configModelResponse
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)

	// fields: one changed, one added, the rest of the file byte for byte
	var saved sectionSave
	body := map[string]any{
		"set": map[string]any{
			"status": map[string]any{"single": true, "options": []string{"backlog", "ready", "done"}},
			"prio":   map[string]any{"single": true, "options": []string{"p0", "p1"}, "colors": map[string]string{"p0": "red"}},
		},
	}
	if code := call(t, "PUT", ts.URL+"/api/config/section/fields?base="+m.Hash, body, &saved); code != http.StatusOK {
		t.Fatalf("fields save: %d", code)
	}
	got, _ := os.ReadFile(s.cfgPath)
	want := "# our board\n" +
		"[fields.status]\nsingle = true\noptions = [\"backlog\", \"ready\", \"done\"]\n\n" +
		"[fields.prio]\nsingle = true\noptions = [\"p0\", \"p1\"]\n\n[fields.prio.colors]\np0 = \"red\"\n\n" +
		"[labels]\nlist = [\"needs-design\"]\n\n" +
		"[roles]\npm = \"claire\"\n\n" +
		"[views.open]\nlayout = \"table\"\n"
	if string(got) != want {
		t.Errorf("file now:\n%q\nwant\n%q", got, want)
	}
	if !saved.Check.OK || saved.Model.Fields["status"].Options[2] != "done" || len(saved.Model.Fields) != 2 {
		t.Errorf("answer: ok %v, fields %+v", saved.Check.OK, saved.Model.Fields)
	}

	// labels: the list and a description, the fields' tables untouched
	var labels sectionSave
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)
	lbody := map[string]any{
		"list":         []string{"needs-design", "regression"},
		"descriptions": map[string]string{"regression": "came back"},
	}
	if code := call(t, "PUT", ts.URL+"/api/config/section/labels?base="+m.Hash, lbody, &labels); code != http.StatusOK {
		t.Fatalf("labels save: %d", code)
	}
	got, _ = os.ReadFile(s.cfgPath)
	if !strings.Contains(string(got), "list = [\"needs-design\", \"regression\"]") || !strings.Contains(string(got), "[labels.descriptions]\nregression = \"came back\"") {
		t.Errorf("labels in file:\n%s", got)
	}
	if strings.Contains(string(got), "options = [\"p0\", \"p1\"]\n\n[fields.p") && !strings.Contains(string(got), "[fields.status]") {
		t.Errorf("the fields went: %s", got)
	}

	// sprints: keys and entries
	var sprints sectionSave
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)
	sbody := map[string]any{
		"length_days": 14,
		"list": []map[string]any{
			{"slug": "s12", "title": "Sprint 12", "start": "2026-10-05T00:00:00Z"},
		},
	}
	if code := call(t, "PUT", ts.URL+"/api/config/section/sprints?base="+m.Hash, sbody, &sprints); code != http.StatusOK {
		t.Fatalf("sprints save: %d", code)
	}
	got, _ = os.ReadFile(s.cfgPath)
	if !strings.Contains(string(got), "start = 2026-10-05") || !strings.Contains(string(got), "length_days = 14") {
		t.Errorf("sprints in file:\n%s", got)
	}

	// roles: the form owns the table; a role left out is gone
	var roles sectionSave
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)
	if code := call(t, "PUT", ts.URL+"/api/config/section/roles?base="+m.Hash, map[string]string{"lead-dev": "alice"}, &roles); code != http.StatusOK {
		t.Fatalf("roles save: %d", code)
	}
	got, _ = os.ReadFile(s.cfgPath)
	if !strings.Contains(string(got), "lead-dev = \"alice\"") || strings.Contains(string(got), "claire") {
		t.Errorf("roles in file:\n%s", got)
	}

	// views: a rename is set the new, remove the old
	var views sectionSave
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)
	vbody := map[string]any{
		"set":    map[string]any{"mine": map[string]any{"layout": "board", "filter": "assignee:@me"}},
		"remove": []string{"open"},
	}
	if code := call(t, "PUT", ts.URL+"/api/config/section/views?base="+m.Hash, vbody, &views); code != http.StatusOK {
		t.Fatalf("views save: %d", code)
	}
	got, _ = os.ReadFile(s.cfgPath)
	if strings.Contains(string(got), "[views.open]") || !strings.Contains(string(got), "[views.mine]") {
		t.Errorf("views in file:\n%s", got)
	}
	// live, no restart: the board serves the new view at once
	var served map[string]any
	call(t, "GET", ts.URL+"/api/views", nil, &served)
	if _, ok := served["mine"]; !ok {
		t.Errorf("not served at once: %v", served)
	}
}

func TestConfigSectionConflictsAndRejects(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	s.cfgPath = filepath.Join(t.TempDir(), ".git-board.toml")
	if err := os.WriteFile(s.cfgPath, []byte(sectionFile), 0o644); err != nil {
		t.Fatal(err)
	}
	var m configModelResponse
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)

	// a stale base: someone saved since the form read
	if code := call(t, "PUT", ts.URL+"/api/config/section/roles?base=stale", map[string]string{"pm": "bob"}, nil); code != http.StatusConflict {
		t.Errorf("a stale base: %d, want 409", code)
	}
	// a section the file doesn't have a form for
	if code := call(t, "PUT", ts.URL+"/api/config/section/spaceships?base="+m.Hash, map[string]any{}, nil); code != http.StatusBadRequest {
		t.Errorf("an unknown section: %d, want 400", code)
	}
	// a field's name the file can't hold bare
	bad := map[string]any{"set": map[string]any{"my field": map[string]any{}}}
	if code := call(t, "PUT", ts.URL+"/api/config/section/fields?base="+m.Hash, bad, nil); code != http.StatusBadRequest {
		t.Errorf("a spaced field name: %d, want 400", code)
	}
	// a value the check refuses: the file is untouched
	broken := map[string]any{"set": map[string]any{"status": map[string]any{"type": "nonsense"}}}
	if code := call(t, "PUT", ts.URL+"/api/config/section/fields?base="+m.Hash, broken, nil); code != http.StatusBadRequest {
		t.Errorf("a bad type: %d, want 400", code)
	}
	got, _ := os.ReadFile(s.cfgPath)
	if string(got) != sectionFile {
		t.Errorf("a refused save changed the file:\n%q", got)
	}
	// a sync holds the write, as the raw editor's save does, as the raw editor's save is
	s.syncing.Store(true)
	if code := call(t, "PUT", ts.URL+"/api/config/section/roles?base="+m.Hash, map[string]string{"pm": "bob"}, nil); code != http.StatusConflict {
		t.Errorf("during a sync: %d, want 409", code)
	}
}
