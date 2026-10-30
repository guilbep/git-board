package server

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghlabels"
	"github.com/guilbep/git-board/internal/lint"
	"github.com/guilbep/git-board/internal/store/storetest"
)

const labelsTestConfig = `[repos]
list = ["o/r"]

[fields.status]
single = true
options = ["ready", "done"]

[fields.kind.colors]
feat = "#0e8a16"

[branches]
kinds = { bug = "fix", feat = "feat" }
`

// fakeLabels serves GitHub's labels API for o/r, with these labels, the
// count of wontfix's issues, and writes recorded in calls ("DELETE
// /repos/o/r/labels/wontfix"). It sets the environment: serial.
func fakeLabels(t *testing.T, labels string) *[]string {
	t.Helper()
	t.Setenv("GITHUB_TOKEN", "t")
	var mu sync.Mutex
	calls := &[]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		*calls = append(*calls, r.Method+" "+r.URL.Path)
		mu.Unlock()
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/labels":
			_, _ = io.WriteString(w, labels)
		case r.URL.Path == "/graphql":
			_, _ = io.WriteString(w, `{"data":{"repository":{"label":{"issues":{"totalCount":2},"pullRequests":{"totalCount":0}}}}}`)
		}
	}))
	t.Cleanup(srv.Close)
	saved := ghlabels.API
	t.Cleanup(func() { ghlabels.API = saved })
	ghlabels.API = srv.URL
	return calls
}

// Serial: it answers for GitHub (ghlabels.API) and sets its token.
func TestLabelsTab(t *testing.T) {
	path := filepath.Join(t.TempDir(), config.FileName)
	if err := os.WriteFile(path, []byte(labelsTestConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b := storetest.New(t)
	ts := newTestServer(t, b, cfg)
	both := storetest.NewIssue(t, b, "two kinds")
	if _, err := b.ApplyLabelChange(both, []string{"kind:feat", "kind:bug"}, nil); err != nil {
		t.Fatal(err)
	}

	gh := fakeLabels(t, `[{"id":1,"name":"bug","color":"d73a4a","description":"Something isn't working"},
		{"id":2,"name":"feat","color":"a2eeef","description":"New feature"},{"id":3,"name":"wontfix","color":"ffffff"}]`)

	var list struct {
		Repos []string   `json:"repos"`
		Rows  []labelRow `json:"rows"`
	}
	getJSON(t, ts.URL+"/api/labels", &list)
	var got []string
	for _, r := range list.Rows {
		got = append(got, fmt.Sprint(r.Name, " ", r.GitHub[0].State, " ", r.Count))
	}
	// the board's red for bug isn't GitHub's; the file's green for feat
	// neither; an issue carries both kinds
	if want := []string{"bug color 1", "feat color 1", "status:ready missing 0", "status:done missing 0"}; !slices.Equal(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}

	var sent struct {
		Changes []labelChange `json:"changes"`
	}
	postJSON(t, ts.URL+"/api/labels/send", map[string]bool{"dry_run": true}, &sent)
	got = nil
	for _, c := range sent.Changes {
		got = append(got, c.Name+" "+c.Color)
	}
	// bug keeps GitHub's color: the file sets none
	if want := []string{"feat 0e8a16", "status:ready 00897f", "status:done 4f9a1a"}; !slices.Equal(got, want) {
		t.Errorf("send plan = %q, want %q", got, want)
	}
	if slices.ContainsFunc(*gh, func(c string) bool { return !strings.HasPrefix(c, "GET ") }) {
		t.Errorf("a dry run wrote to GitHub: %q", *gh)
	}

	var adopted struct {
		Text string `json:"text"`
	}
	postJSON(t, ts.URL+"/api/labels/adopt", map[string]string{"text": labelsTestConfig, "field": "kind", "value": "bug",
		"color": "D73A4A", "description": "Something isn't working"}, &adopted)
	for _, want := range []string{"[fields.kind.colors]\nbug = \"#d73a4a\"\nfeat = \"#0e8a16\"\n", "[fields.kind.descriptions]\nbug = \"Something isn't working\"\n"} {
		if !strings.Contains(adopted.Text, want) {
			t.Errorf("adopted text lacks %q:\n%s", want, adopted.Text)
		}
	}

	var linted struct {
		Findings []lint.Finding `json:"findings"`
		Stray    []strayLabel   `json:"stray"`
	}
	getJSON(t, ts.URL+"/api/lint", &linted)
	if !slices.ContainsFunc(linted.Findings, func(f lint.Finding) bool { return f.Check == lint.CheckTwoKinds }) ||
		len(linted.Stray) != 1 || linted.Stray[0].Name != "wontfix" {
		t.Errorf("lint = %+v, want two-kinds and wontfix", linted)
	}

	var count struct {
		Count int `json:"count"`
	}
	postJSON(t, ts.URL+"/api/labels/delete", map[string]any{"repo": "o/r", "name": "wontfix", "dry_run": true}, &count)
	if count.Count != 2 {
		t.Errorf("count = %d, want 2", count.Count)
	}
	if resp := postJSON(t, ts.URL+"/api/labels/delete", map[string]any{"repo": "o/r", "name": "bug"}, nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("deleting the project's bug: %d, want 400", resp.StatusCode)
	}
	postJSON(t, ts.URL+"/api/labels/delete", map[string]any{"repo": "o/r", "name": "wontfix"}, nil)
	if last := (*gh)[len(*gh)-1]; last != "DELETE /repos/o/r/labels/wontfix" {
		t.Errorf("last call = %q", last)
	}
}

// Serial: it answers for GitHub (fakeLabels). With a Project, the fields
// it holds are in the Project, not labels on GitHub: none is sent.
func TestLabelsHeldByTheProject(t *testing.T) {
	path := filepath.Join(t.TempDir(), config.FileName)
	if err := os.WriteFile(path, []byte(labelsTestConfig+"\n[github-project]\nowner = \"o\"\nnumber = 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	ts := newTestServer(t, storetest.New(t), cfg)
	fakeLabels(t, `[{"id":1,"name":"bug","color":"d73a4a"}]`)

	var list struct {
		Rows []labelRow `json:"rows"`
	}
	getJSON(t, ts.URL+"/api/labels", &list)
	var got []string
	for _, r := range list.Rows {
		got = append(got, r.Name+" "+r.GitHub[0].State)
	}
	if want := []string{"bug color", "feat missing", "status:ready held", "status:done held"}; !slices.Equal(got, want) {
		t.Errorf("rows = %q, want %q", got, want)
	}
	var sent struct {
		Changes []labelChange `json:"changes"`
	}
	postJSON(t, ts.URL+"/api/labels/send", map[string]bool{"dry_run": true}, &sent)
	if slices.ContainsFunc(sent.Changes, func(c labelChange) bool { return strings.HasPrefix(c.Name, "status:") }) {
		t.Errorf("send plan %+v sends a status", sent.Changes)
	}
}

// Serial: it answers for GitHub (fakeLabels). A direct call deletes only
// what the lint lists as stray: not a free-form field's value, not the
// project's.
func TestLabelDeleteOnlyStray(t *testing.T) {
	path := filepath.Join(t.TempDir(), config.FileName)
	if err := os.WriteFile(path, []byte(labelsTestConfig+"\n[fields.assignee]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	ts := newTestServer(t, storetest.New(t), cfg)

	gh := fakeLabels(t, `[{"id":1,"name":"bug"},{"id":2,"name":"wontfix"},{"id":3,"name":"assignee:alice"}]`)

	for _, name := range []string{"assignee:alice", "bug"} {
		if resp := postJSON(t, ts.URL+"/api/labels/delete", map[string]any{"repo": "o/r", "name": name}, nil); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("deleting %s: %d, want 400", name, resp.StatusCode)
		}
	}
	if slices.ContainsFunc(*gh, func(c string) bool { return strings.HasPrefix(c, "DELETE") }) {
		t.Errorf("a refused delete reached GitHub: %q", *gh)
	}
	if resp := postJSON(t, ts.URL+"/api/labels/delete", map[string]any{"repo": "o/r", "name": "wontfix"}, nil); resp.StatusCode != http.StatusOK {
		t.Errorf("deleting wontfix: %d", resp.StatusCode)
	}
	if last := (*gh)[len(*gh)-1]; last != "DELETE /repos/o/r/labels/wontfix" {
		t.Errorf("last call = %q", last)
	}
}

// The issue's label picker reads [labels]: the list, its colors and
// descriptions; defined false without the table.
func TestLabelsListForThePicker(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	var res struct {
		Defined      bool              `json:"defined"`
		List         []string          `json:"list"`
		Colors       map[string]string `json:"colors"`
		Descriptions map[string]string `json:"descriptions"`
	}
	ts := newTestServer(t, b, &config.Config{Labels: config.Labels{Defined: true, List: []string{"needs-design", "regression"}, Colors: map[string]string{"regression": "red"}}})
	if resp := getJSON(t, ts.URL+"/api/config/labels", &res); resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if !res.Defined || len(res.List) != 2 || res.Colors["regression"] != config.PaletteHex["red"] || res.Colors["needs-design"] == "" || res.Descriptions == nil {
		t.Errorf("labels %+v", res)
	}
	none := newTestServer(t, b, &config.Config{})
	res.Defined, res.List = true, nil
	if getJSON(t, none.URL+"/api/config/labels", &res); res.Defined || res.List == nil || len(res.List) != 0 {
		t.Errorf("no table: %+v", res)
	}
}

// Adopt on a [labels] list label writes [labels.colors] and
// [labels.descriptions], not a field's tables.
func TestAdoptALabelOfTheList(t *testing.T) {
	t.Parallel()
	ts := newTestServer(t, storetest.New(t), &config.Config{})
	text := "[labels]\nlist = [\"needs-design\"]\n"
	var adopted struct {
		Text string `json:"text"`
	}
	postJSON(t, ts.URL+"/api/labels/adopt", map[string]string{"text": text, "field": "", "value": "needs-design",
		"color": "5319E7", "description": "Waits for a design decision"}, &adopted)
	for _, want := range []string{"[labels.colors]\nneeds-design = \"#5319e7\"", "[labels.descriptions]\nneeds-design = \"Waits for a design decision\""} {
		if !strings.Contains(adopted.Text, want) {
			t.Errorf("adopted text lacks %q:\n%s", want, adopted.Text)
		}
	}
	if strings.Contains(adopted.Text, "[fields.") {
		t.Errorf("a field's table written:\n%s", adopted.Text)
	}
}
