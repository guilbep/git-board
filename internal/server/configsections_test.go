package server

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/store/storetest"
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
	ViewOrder  []string `json:"view_order"`
	FieldOrder []string `json:"field_order"`
	Sprints    struct {
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

func TestFieldUsageCountsTheIssuesPerField(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	one := storetest.NewIssue(t, s.b, "one")
	storetest.NewIssue(t, s.b, "two")
	for _, id := range []string{one} {
		if code := call(t, "POST", ts.URL+"/api/issues/"+id+"/labels", map[string]any{"add": []string{"status:ready", "est:2", "plain"}}, nil); code != http.StatusOK {
			t.Fatalf("labels: %d", code)
		}
	}
	var usage map[string]int
	if code := call(t, "GET", ts.URL+"/api/fields/usage", nil, &usage); code != http.StatusOK {
		t.Fatalf("usage: %d", code)
	}
	if usage["status"] != 1 || usage["est"] != 1 {
		t.Errorf("usage = %v, want status and est at 1 (a plain label is no field)", usage)
	}
	if _, ok := usage["plain"]; ok {
		t.Errorf("a plain label is not a field: %v", usage)
	}
}

func TestConfigSectionRealizesTheOrder(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	s.cfgPath = filepath.Join(t.TempDir(), ".git-board.toml")
	file := "# our board\n" +
		"[fields.zulu]\nnumeric = true\noptions = [\"1\", \"2\"]\n\n" +
		"# the middle one\n[fields.est]\nsingle = true\n\n[fields.est.colors]\nzulu = \"teal\"\n\n" +
		"[fields.alpha]\n\n" +
		"[labels]\nlist = [\"needs-design\"]\n\n" +
		"[views.open]\nlayout = \"table\"\n\n[views.mine]\nlayout = \"board\"\n"
	if err := os.WriteFile(s.cfgPath, []byte(file), 0o644); err != nil {
		t.Fatal(err)
	}
	var m configModelResponse
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)
	if got, want := strings.Join(m.FieldOrder, ","), "zulu,est,alpha"; got != want {
		t.Fatalf("field_order = %q, want %q", got, want)
	}
	if got, want := strings.Join(m.ViewOrder, ","), "open,mine"; got != want {
		t.Fatalf("view_order = %q, want %q", got, want)
	}

	// the list's order wins: alpha first, and a same-order save doesn't
	// move a line — it once reversed the views on every save
	save := func(section string, body map[string]any) configModelResponse {
		var out sectionSave
		if code := call(t, "PUT", ts.URL+"/api/config/section/"+section+"?base="+m.Hash, body, &out); code != http.StatusOK {
			t.Fatalf("%s save: %d", section, code)
		}
		return out.Model
	}
	m = save("fields", map[string]any{"set": map[string]any{}, "remove": []string{}, "order": []string{"alpha", "est", "zulu"}})
	if got, want := strings.Join(m.FieldOrder, ","), "alpha,est,zulu"; got != want {
		t.Fatalf("field_order = %q, want %q", got, want)
	}
	m = save("fields", map[string]any{"set": map[string]any{}, "remove": []string{}, "order": []string{"alpha", "est", "zulu"}})
	if got, want := strings.Join(m.FieldOrder, ","), "alpha,est,zulu"; got != want {
		t.Fatalf("second save, field_order = %q, want %q", got, want)
	}
	m = save("views", map[string]any{"set": map[string]any{}, "remove": []string{}, "order": []string{"mine", "open"}})
	if got, want := strings.Join(m.ViewOrder, ","), "mine,open"; got != want {
		t.Fatalf("view_order = %q, want %q", got, want)
	}
	m = save("views", map[string]any{"set": map[string]any{}, "remove": []string{}, "order": []string{"mine", "open"}})
	if got, want := strings.Join(m.ViewOrder, ","), "mine,open"; got != want {
		t.Fatalf("second save, view_order = %q, want %q", got, want)
	}

	// the rest of the file is untouched: the labels, the comments, the
	// subtable with its field
	got, _ := os.ReadFile(s.cfgPath)
	for _, want := range []string{"# our board", "list = [\"needs-design\"]", "# the middle one", "[fields.est.colors]", "zulu = \"teal\""} {
		if !strings.Contains(string(got), want) {
			t.Errorf("the file lost %q:\n%s", want, got)
		}
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

// The forges' Fields table: the model answers the [forges.<name>] tables
// whole, and the forge-fields section writes the mappings — the inline
// tables and the values subtables, the rest of the file kept.
func TestConfigSectionForgeFields(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	s.cfgPath = filepath.Join(t.TempDir(), ".git-board.toml")
	src := sectionFile + "\n[forges.gh]\nkind = \"github\"\nproject = \"o/r\"\nsync = true\n"
	if err := os.WriteFile(s.cfgPath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var m struct {
		Hash   string `json:"hash"`
		Forges []struct {
			Name   string `json:"name"`
			Kind   string `json:"kind"`
			Fields map[string]struct {
				To     string            `json:"to"`
				Field  string            `json:"field"`
				As     string            `json:"as"`
				Values map[string]string `json:"values"`
			} `json:"fields"`
		} `json:"forges"`
	}
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)
	if len(m.Forges) != 1 || m.Forges[0].Name != "gh" {
		t.Fatalf("forges: %+v", m.Forges)
	}

	// the mapping lands, the forge and the fields stay
	var saved sectionSave
	body := map[string]any{"forge": "gh", "set": map[string]any{
		"status": map[string]any{"to": "project", "field": "Status"},
	}, "remove": []string{}}
	if code := call(t, "PUT", ts.URL+"/api/config/section/forge-fields?base="+m.Hash, body, &saved); code != http.StatusOK {
		t.Fatalf("save: %d", code)
	}
	text, _ := os.ReadFile(s.cfgPath)
	for _, want := range []string{"[forges.gh]", "sync = true", `status = { to = "project", field = "Status" }`, "[fields.status]"} {
		if !strings.Contains(string(text), want) {
			t.Errorf("the file lost %q:\n%s", want, text)
		}
	}
	// a mapping that fits nothing is refused, the file untouched
	bad := map[string]any{"forge": "gh", "set": map[string]any{
		"status": map[string]any{"to": "weight"},
	}, "remove": []string{}}
	if code := call(t, "PUT", ts.URL+"/api/config/section/forge-fields?base="+saved.Model.Hash, bad, nil); code != http.StatusBadRequest {
		t.Errorf("weight on a select: %d, want 400", code)
	}
	got, _ := os.ReadFile(s.cfgPath)
	if !strings.Contains(string(got), `status = { to = "project", field = "Status" }`) {
		t.Errorf("a refused save changed the file:\n%s", got)
	}
	// a bogus forge name is refused
	if code := call(t, "PUT", ts.URL+"/api/config/section/forge-fields?base="+saved.Model.Hash, map[string]any{"forge": "my forge", "set": map[string]any{}, "remove": []string{}}, nil); code != http.StatusBadRequest {
		t.Errorf("a spaced forge name: %d, want 400", code)
	}
}

// With no [forges] table the model's forges are those found in the
// remotes, never null (the Fields page reads them); a mapping saved on a
// found one writes its table first, so the file still loads. Serial: it
// answers for GitHub.
func TestConfigSectionForgeFieldsOnAFoundForge(t *testing.T) {
	storetest.GitHub(t, map[string]string{"o/r": "o/r"})
	ts, s := forgeServer(t)
	if err := os.WriteFile(s.cfgPath, []byte(sectionFile), 0o644); err != nil {
		t.Fatal(err)
	}
	type model struct {
		Hash   string `json:"hash"`
		Forges []struct {
			Name       string `json:"name"`
			Discovered bool   `json:"discovered"`
		} `json:"forges"`
	}
	var raw map[string]json.RawMessage
	call(t, "GET", ts.URL+"/api/config/model", nil, &raw)
	if string(raw["forges"]) != "[]" {
		t.Errorf("no forge: %s, want []", raw["forges"])
	}

	storetest.ForgeOrigin(t, s.b.GitDir(), "git@github.com:o/r.git")
	var m model
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)
	if len(m.Forges) != 1 || m.Forges[0].Name != "github" || !m.Forges[0].Discovered {
		t.Fatalf("forges: %+v", m.Forges)
	}
	body := map[string]any{"forge": "github", "set": map[string]any{"status": map[string]any{"to": "label"}}, "remove": []string{}}
	if code, msg := callError(t, "PUT", ts.URL+"/api/config/section/forge-fields?base="+m.Hash, body); code != http.StatusOK {
		t.Fatalf("save: %d %s", code, msg)
	}
	text, _ := os.ReadFile(s.cfgPath)
	for _, want := range []string{"[forges.github]\nkind = \"github\"\nproject = \"o/r\"\nsync = true\n", `status = { to = "label" }`} {
		if !strings.Contains(string(text), want) {
			t.Errorf("the file lacks %q:\n%s", want, text)
		}
	}
	m = model{} // omitempty: a stale discovered would stay
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)
	if len(m.Forges) != 1 || m.Forges[0].Discovered {
		t.Errorf("after the save: %+v", m.Forges)
	}
	if code, _ := callError(t, "PUT", ts.URL+"/api/config/section/forge-fields?base="+m.Hash, map[string]any{"forge": "nope", "set": map[string]any{}, "remove": []string{}}); code != http.StatusBadRequest {
		t.Errorf("an unknown forge: %d, want 400", code)
	}
}

// The model never answers null at its top: the pages read every key as
// it is (view_order.flatMap, forges.find), a file without views or forges
// included.
func TestConfigModelHasNoNulls(t *testing.T) {
	t.Parallel()
	for name, text := range map[string]string{"empty": "", "fields only": "[fields.prio]\noptions = [\"p0\"]\n"} {
		ts, s := newIssueServer(t)
		s.cfgPath = filepath.Join(t.TempDir(), ".git-board.toml")
		if err := os.WriteFile(s.cfgPath, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		var raw map[string]json.RawMessage
		call(t, "GET", ts.URL+"/api/config/model", nil, &raw)
		for k, v := range raw {
			if string(v) == "null" {
				t.Errorf("%s: %s is null", name, k)
			}
		}
	}
}

// One editor for a field and its places (docs/prd/fields-one-editor.md):
// the fields section writes the field and its mappings in one write; a
// bad mapping refuses the whole save; a removed field takes its mappings
// with it, on every forge.
func TestConfigSectionFieldsWithTheirPlaces(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	s.cfgPath = filepath.Join(t.TempDir(), ".git-board.toml")
	src := sectionFile + "\n[forges.gh]\nkind = \"github\"\nproject = \"o/r\"\nsync = true\n\n[forges.lab]\nkind = \"gitlab\"\nproject = \"42\"\nsync = true\n"
	if err := os.WriteFile(s.cfgPath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	var m struct {
		Hash string `json:"hash"`
	}
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)

	var saved sectionSave
	body := map[string]any{
		"set":    map[string]any{"prio": map[string]any{"single": true, "options": []string{"p0", "p1"}}},
		"remove": []string{},
		"order":  []string{},
		"forges": map[string]any{
			"gh":  map[string]any{"set": map[string]any{"prio": map[string]any{"to": "project", "field": "Priority", "values": map[string]string{"p0": "Urgent"}}}, "remove": []string{}},
			"lab": map[string]any{"set": map[string]any{"prio": map[string]any{"to": "label"}}, "remove": []string{}},
		},
	}
	if code := call(t, "PUT", ts.URL+"/api/config/section/fields?base="+m.Hash, body, &saved); code != http.StatusOK {
		t.Fatalf("save: %d", code)
	}
	text, _ := os.ReadFile(s.cfgPath)
	for _, want := range []string{"[fields.prio]", `prio = { to = "project", field = "Priority" }`, "[forges.gh.fields.prio.values]", `p0 = "Urgent"`, `prio = { to = "label" }`, "[fields.status]"} {
		if !strings.Contains(string(text), want) {
			t.Errorf("the file lacks %q:\n%s", want, text)
		}
	}

	// a place that fits nothing refuses the whole save: the field's change too
	bad := map[string]any{
		"set":    map[string]any{"prio": map[string]any{"single": true, "options": []string{"p0", "p1", "p2"}}},
		"remove": []string{}, "order": []string{},
		"forges": map[string]any{"gh": map[string]any{"set": map[string]any{"prio": map[string]any{"to": "weight"}}, "remove": []string{}}},
	}
	if code := call(t, "PUT", ts.URL+"/api/config/section/fields?base="+saved.Model.Hash, bad, nil); code != http.StatusBadRequest {
		t.Errorf("weight on a select: %d, want 400", code)
	}
	if got, _ := os.ReadFile(s.cfgPath); string(got) != string(text) {
		t.Errorf("a refused save changed the file:\n%s", got)
	}
	if code := call(t, "PUT", ts.URL+"/api/config/section/fields?base="+saved.Model.Hash, map[string]any{"set": map[string]any{}, "remove": []string{}, "order": []string{}, "forges": map[string]any{"my forge": map[string]any{}}}, nil); code != http.StatusBadRequest {
		t.Errorf("a spaced forge name: %d, want 400", code)
	}

	// removing the field removes its places, on both forges
	if code, msg := callError(t, "PUT", ts.URL+"/api/config/section/fields?base="+saved.Model.Hash, map[string]any{"set": map[string]any{}, "remove": []string{"prio"}, "order": []string{}}); code != http.StatusOK {
		t.Fatalf("remove: %d %s", code, msg)
	}
	got, _ := os.ReadFile(s.cfgPath)
	for _, gone := range []string{"[fields.prio]", "prio = {", "fields.prio.values", "Urgent"} {
		if strings.Contains(string(got), gone) {
			t.Errorf("%q left behind:\n%s", gone, got)
		}
	}
	for _, kept := range []string{"[forges.gh]", "[forges.lab]", "[fields.status]"} {
		if !strings.Contains(string(got), kept) {
			t.Errorf("%q lost:\n%s", kept, got)
		}
	}
}

// A place saved with the field on a forge only found in the remotes
// writes the found forges' tables first. Serial: it answers for GitHub.
func TestConfigSectionFieldsPlaceOnAFoundForge(t *testing.T) {
	storetest.GitHub(t, map[string]string{"o/r": "o/r"})
	ts, s := forgeServer(t)
	if err := os.WriteFile(s.cfgPath, []byte(sectionFile), 0o644); err != nil {
		t.Fatal(err)
	}
	storetest.ForgeOrigin(t, s.b.GitDir(), "git@github.com:o/r.git")
	var m struct {
		Hash string `json:"hash"`
	}
	call(t, "GET", ts.URL+"/api/config/model", nil, &m)
	body := map[string]any{"set": map[string]any{}, "remove": []string{}, "order": []string{},
		"forges": map[string]any{"github": map[string]any{"set": map[string]any{"status": map[string]any{"to": "label"}}, "remove": []string{}}}}
	if code, msg := callError(t, "PUT", ts.URL+"/api/config/section/fields?base="+m.Hash, body); code != http.StatusOK {
		t.Fatalf("save: %d %s", code, msg)
	}
	text, _ := os.ReadFile(s.cfgPath)
	for _, want := range []string{"[forges.github]\nkind = \"github\"\nproject = \"o/r\"\nsync = true\n", `status = { to = "label" }`} {
		if !strings.Contains(string(text), want) {
			t.Errorf("the file lacks %q:\n%s", want, text)
		}
	}
}
