package server

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/internal/config"
)

type configFile struct {
	Path    string `json:"path"`
	Text    string `json:"text"`
	Exists  bool   `json:"exists"`
	Hash    string `json:"hash"`
	Default string `json:"default"`
}

type configCheck struct {
	OK    bool `json:"ok"`
	Error *struct {
		Line    int    `json:"line"`
		Message string `json:"message"`
	} `json:"error"`
	Warnings []string `json:"warnings"`
	Summary  struct {
		Fields  []string `json:"fields"`
		Views   []string `json:"views"`
		Sprints int      `json:"sprints"`
	} `json:"summary"`
}

func TestConfigFileEditedFromThePage(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	s.cfgPath = filepath.Join(t.TempDir(), ".git-board.toml")
	old := "# our board\n[views.backlog]\nfilter = \"status:backlog\"\n"
	if err := os.WriteFile(s.cfgPath, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}

	var f configFile
	call(t, "GET", ts.URL+"/api/config", nil, &f)
	if !f.Exists || f.Text != old || f.Hash == "" {
		t.Fatalf("GET %+v", f)
	}

	var c configCheck
	call(t, "POST", ts.URL+"/api/config/check", map[string]string{"text": "[views.x\n"}, &c)
	if c.OK || c.Error == nil || c.Error.Line != 1 {
		t.Errorf("a broken file: %+v", c)
	}
	newText := old + "\n[views.mine]\nfilter = \"assignee:@me\"\nlayout = \"board\"\n\n[fields.status]\noptions = [\"backlog\", \"done\"]\n"
	call(t, "POST", ts.URL+"/api/config/check", map[string]string{"text": newText}, &c)
	if !c.OK || strings.Join(c.Summary.Views, ",") != "backlog,mine" || strings.Join(c.Summary.Fields, ",") != "status" {
		t.Errorf("a good file: %+v", c)
	}

	if code := call(t, "PUT", ts.URL+"/api/config", map[string]string{"text": "[views.x\n", "base": f.Hash}, nil); code != http.StatusBadRequest {
		t.Errorf("saving a broken file: %d, want 400", code)
	}
	if code := call(t, "PUT", ts.URL+"/api/config", map[string]string{"text": newText, "base": f.Hash}, nil); code != http.StatusOK {
		t.Fatalf("save: %d", code)
	}
	got, _ := os.ReadFile(s.cfgPath)
	if string(got) != newText {
		t.Errorf("file now %q", got)
	}
	var views map[string]any
	call(t, "GET", ts.URL+"/api/views", nil, &views)
	if _, ok := views["mine"]; !ok {
		t.Errorf("views after the save, no restart: %v", views)
	}

	// someone edited the file since the page read it
	if code := call(t, "PUT", ts.URL+"/api/config", map[string]string{"text": old, "base": f.Hash}, nil); code != http.StatusConflict {
		t.Errorf("a stale base: %d, want 409", code)
	}
}

// A repository without the file starts from the default: offered, saved
// like any edit.
func TestConfigFileStartsFromTheDefault(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	s.cfgPath = filepath.Join(t.TempDir(), ".git-board.toml")

	var f configFile
	call(t, "GET", ts.URL+"/api/config", nil, &f)
	if f.Exists || f.Default != config.Default {
		t.Fatalf("GET without a file: exists %v, default %d bytes", f.Exists, len(f.Default))
	}
	if code := call(t, "PUT", ts.URL+"/api/config", map[string]string{"text": f.Default, "base": f.Hash}, nil); code != http.StatusOK {
		t.Fatalf("save: %d", code)
	}
	var after configFile
	call(t, "GET", ts.URL+"/api/config", nil, &after)
	if !after.Exists || after.Default != "" {
		t.Errorf("GET with the file: exists %v, default %d bytes", after.Exists, len(after.Default))
	}
}

// A save switches the kind form without a restart: flat kinds store bug,
// [fields.kind] options store kind:bug.
func TestSaveSwitchesTheKindForm(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	s.cfgPath = filepath.Join(t.TempDir(), ".git-board.toml")
	storedAfter := func(text string) string {
		t.Helper()
		var f configFile
		call(t, "GET", ts.URL+"/api/config", nil, &f)
		if code := call(t, "PUT", ts.URL+"/api/config", map[string]string{"text": text, "base": f.Hash}, nil); code != http.StatusOK {
			t.Fatalf("save: %d", code)
		}
		id, err := s.b.NewIssue("x", "", []string{"kind:bug"})
		if err != nil {
			t.Fatal(err)
		}
		is, err := issue.Load(s.b.Store(), id)
		if err != nil {
			t.Fatal(err)
		}
		return is.Labels[0]
	}
	flat := "[branches]\nkinds = { bug = \"fix\" }\n"
	if got := storedAfter(flat); got != "bug" {
		t.Errorf("flat form stored %q, want bug", got)
	}
	if got := storedAfter("[fields.kind]\noptions = [\"bug\"]\n\n" + flat); got != "kind:bug" {
		t.Errorf("kind:* form stored %q, want kind:bug", got)
	}
}

func TestConfigDocsComeInSections(t *testing.T) {
	t.Parallel()
	ts, _ := newIssueServer(t)
	var docs []struct {
		ID, Title, HTML string
	}
	call(t, "GET", ts.URL+"/api/config/docs", nil, &docs)
	var ids []string
	for _, d := range docs {
		ids = append(ids, d.ID)
	}
	if strings.Join(ids, ",") != "overview,fields,types,colors,views,roles,filters,sprints,github,forges,attachments,branches,cycle,worktrees,numbers,release,repos" {
		t.Errorf("sections %v", ids)
	}
	if len(docs) > 1 && (!strings.Contains(docs[1].HTML, "<table>") || docs[1].Title != "Fields" || strings.Contains(docs[1].HTML, "<h2")) {
		t.Errorf("fields section: %s %q", docs[1].Title, docs[1].HTML[:min(200, len(docs[1].HTML))])
	}
}

func TestForgeDocsComeInSections(t *testing.T) {
	t.Parallel()
	ts, _ := newIssueServer(t)
	var docs []struct {
		ID, Title, HTML string
	}
	call(t, "GET", ts.URL+"/api/forges/docs", nil, &docs)
	var ids []string
	for _, d := range docs {
		ids = append(ids, d.ID)
	}
	// the add-a-forge dialog shows three, sync, tokens and adding beside its steps
	if strings.Join(ids, ",") != "three,tokens,adding,sync,removing,one,on" {
		t.Errorf("sections %v", ids)
	}
}

func TestSaveViewWritesItsTable(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	s.cfgPath = filepath.Join(t.TempDir(), ".git-board.toml")
	if err := os.WriteFile(s.cfgPath, []byte("# team board\n[views.backlog]\nfilter = \"is:open\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := map[string]string{"layout": "table", "filter": "is:open -status:validated", "group": "sprint", "sort": "prio asc"}
	if code := call(t, "PUT", ts.URL+"/api/views/backlog", body, nil); code != http.StatusOK {
		t.Fatalf("save: %d", code)
	}
	var views map[string]struct{ Filter, Group, Sort string }
	call(t, "GET", ts.URL+"/api/views", nil, &views)
	if v := views["backlog"]; v.Filter != "is:open -status:validated" || v.Group != "sprint" || v.Sort != "prio asc" {
		t.Errorf("served at once: %+v", v)
	}
	got, _ := os.ReadFile(s.cfgPath)
	if !strings.HasPrefix(string(got), "# team board\n[views.backlog]\n") {
		t.Errorf("file %q", got)
	}
	if code := call(t, "PUT", ts.URL+"/api/views/in-review", map[string]string{"layout": "board", "filter": "status:in-review,qa", "group": "assignee"}, nil); code != http.StatusOK {
		t.Errorf("a new view: %d", code)
	}
	withCols := map[string]any{"layout": "table", "filter": "is:open", "columns": []string{"title", "status", "links"}}
	var saved map[string]struct{ Columns []string }
	if code := call(t, "PUT", ts.URL+"/api/views/cols", withCols, &saved); code != http.StatusOK || strings.Join(saved["cols"].Columns, ",") != "title,status,links" {
		t.Errorf("a view's columns: %d %+v", code, saved["cols"])
	}
	if code := call(t, "PUT", ts.URL+"/api/views/my%20view", body, nil); code != http.StatusBadRequest {
		t.Errorf("a name with a space: %d, want 400", code)
	}
	s.syncing.Store(true)
	if code := call(t, "PUT", ts.URL+"/api/views/backlog", body, nil); code != http.StatusConflict {
		t.Errorf("during a forge sync: %d, want 409", code)
	}
}
