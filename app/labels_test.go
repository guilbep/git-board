package app

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/guilbep/git-board/conf"
	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/forge"
)

// lint --create-labels' parts, from v1's internal/ghlabels tests.

func labelNames(labels []DesiredLabel) []string {
	var res []string
	for _, l := range labels {
		res = append(res, l.Name)
	}
	return res
}

func TestDesiredCoversFieldOptionsAndSprints(t *testing.T) {
	t.Parallel()
	s := &issues.Schema{
		Kinds: map[string]string{"bug": "fix"},
		Fields: map[string]issues.Field{
			"status":   {Single: true, Options: []string{"ready", "backlog"}, Colors: map[string]string{"ready": "#0e8a16"}},
			"prio":     {Single: true, Options: []string{"p0"}, Descriptions: map[string]string{"p0": "Now"}},
			"assignee": {}, // no options: free-form, no label to make
		},
		Sprints: issues.Sprints{List: []issues.Sprint{{Slug: "delivery"}, {Slug: "post-delivery"}}},
	}
	got := DesiredLabels(s)
	want := []string{"prio:p0", "sprint:delivery", "sprint:post-delivery", "status:ready", "status:backlog"} // a field's values in their order
	if !reflect.DeepEqual(labelNames(got), want) {
		t.Errorf("DesiredLabels = %v, want %v", labelNames(got), want)
	}
	if p0 := got[0]; p0.Color != "e00000" || p0.OwnColor || p0.Description != "Now" || !p0.OwnDescription {
		t.Errorf("p0 = %+v, want the board's red, not its own, and its description", p0)
	}
	if ready := got[3]; ready.Color != "0e8a16" || !ready.OwnColor {
		t.Errorf("ready = %+v, want its own hex", ready)
	}
	// flat kinds are the kind field, stored as the plain label
	s.Fields["kind"] = issues.Field{Single: true, Options: []string{"bug"}, Flat: true}
	if got := labelNames(DesiredLabels(s)); !slices.Contains(got, "bug") || slices.Contains(got, "kind:bug") {
		t.Errorf("flat kinds: DesiredLabels = %v, want bug", got)
	}
	if got := DesiredLabels(&issues.Schema{}); len(got) != 0 {
		t.Errorf("DesiredLabels(empty) = %v, want none", got)
	}
}

// [labels] list's labels are the project's: plain, with no field, colored
// and described by the file.
func TestDesiredHoldsTheLabelsList(t *testing.T) {
	t.Parallel()
	s := &issues.Schema{Labels: issues.Labels{List: []string{"needs-design", "regression"}, Colors: map[string]string{"regression": "red"},
		Descriptions: map[string]string{"needs-design": "waits for a design decision"}, Defined: true}}
	got := DesiredLabels(s)
	if !slices.Equal(labelNames(got), []string{"needs-design", "regression"}) {
		t.Fatalf("DesiredLabels %v", labelNames(got))
	}
	if got[0].Field != "" || got[0].Value != "needs-design" || got[0].Description == "" || !got[0].OwnDescription || got[0].OwnColor {
		t.Errorf("needs-design %+v", got[0])
	}
	if got[1].Color != issues.PaletteHex["red"] || !got[1].OwnColor {
		t.Errorf("regression %+v", got[1])
	}
}

// The org's labels keep their colors: a label is changed only where the
// file sets its color or description, never for git-board's own colors.
// A label held off the forge is never made.
func TestPlanChangesOnlyWhatTheFileSets(t *testing.T) {
	t.Parallel()
	desired := []DesiredLabel{
		{Label: forge.Label{Name: "bug", Color: "e00000"}},                                                   // the board's color: the forge's stays
		{Label: forge.Label{Name: "feat", Color: "0e8a16"}, OwnColor: true},                                  // the file's color
		{Label: forge.Label{Name: "docs", Color: "0075CA", Description: "Docs"}, OwnDescription: true},       // the file's description
		{Label: forge.Label{Name: "perf", Color: "a2eeef"}, OwnColor: true},                                  // already so
		{Label: forge.Label{Name: "status:ready", Color: "00897f", Description: "Go"}, OwnDescription: true}, // missing
		{Label: forge.Label{Name: "sprint:s1", Color: "3a7bc8"}},                                             // missing, held
	}
	existing := []forge.Label{
		{Name: "Bug", Color: "d73a4a", Description: "Something isn't working"},
		{Name: "feat", Color: "A2EEEF", Description: "New feature"},
		{Name: "docs", Color: "0075ca"},
		{Name: "perf", Color: "A2EEEF"},
		{Name: "wontfix", Color: "ffffff"},
	}
	var asked []string
	held := func(key, label string) bool {
		asked = append(asked, key+" "+label)
		return strings.HasPrefix(label, "sprint:")
	}
	got := PlanLabels("github.com/o/r", desired, existing, held)
	want := []LabelChange{
		{Label: forge.Label{Name: "feat", Color: "0e8a16", Description: "New feature"}, Update: true, Was: &existing[1]},
		{Label: forge.Label{Name: "docs", Color: "0075ca", Description: "Docs"}, Update: true, Was: &existing[2]},
		{Label: forge.Label{Name: "status:ready", Color: "00897f", Description: "Go"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("PlanLabels =\n%+v\nwant\n%+v", got, want)
	}
	if !slices.Equal(asked, []string{"github.com/o/r status:ready", "github.com/o/r sprint:s1"}) {
		t.Errorf("held asked %q", asked)
	}
}

func TestIssueTemplatesReadsEveryLabelsShape(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	for name, body := range map[string]string{
		"a.yaml":     "labels: [\"enhancement\", \"triage\"]\n",
		"b.md":       "---\nname: B\nlabels: bug, needs-triage\n---\nBody\n",
		"c.md":       "---\nlabels:\n  - feat\n---\n",
		"d.md":       "no front matter, no labels\n",
		"e.md":       "--- \nlabels:\n  - specs\n---\n", // co2's specs.md opens with "--- "
		"config.yml": "blank_issues_enabled: false\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := IssueTemplates(dir)
	want := []IssueTemplate{
		{File: "a.yaml", Labels: []string{"enhancement", "triage"}},
		{File: "b.md", Labels: []string{"bug", "needs-triage"}},
		{File: "c.md", Labels: []string{"feat"}},
		{File: "e.md", Labels: []string{"specs"}},
	}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("IssueTemplates = %+v, %v, want %+v", got, err, want)
	}
	if got, err := IssueTemplates(filepath.Join(dir, "none")); got != nil || err != nil {
		t.Errorf("no folder: %v, %v", got, err)
	}
}

// ENAC's template wants every EPFL-ENAC label as GitHub has it: nothing
// for lint --create-labels to update.
func TestENACTemplateAgreesWithItsLabels(t *testing.T) {
	t.Parallel()
	var existing []forge.Label
	data, err := os.ReadFile("testdata/labels/enac-labels.json")
	if err == nil {
		err = json.Unmarshal(data, &existing)
	}
	if err != nil {
		t.Fatal(err)
	}
	text, err := starter("enac")
	if err != nil {
		t.Fatal(err)
	}
	f, err := conf.Parse(conf.FileName, []byte(text))
	if err != nil {
		t.Fatal(err)
	}
	c, err := ReadConfig(f, nil)
	if err != nil {
		t.Fatal(err)
	}
	desired := DesiredLabels(c.Schema)
	for _, l := range existing {
		if !slices.ContainsFunc(desired, func(d DesiredLabel) bool { return strings.EqualFold(d.Name, l.Name) }) {
			t.Errorf("%s: not in the template", l.Name)
		}
	}
	never := func(string, string) bool { return false }
	for _, c := range PlanLabels("github.com/EPFL-ENAC/x", desired, existing, never) {
		if c.Update {
			t.Errorf("%s would be updated: %+v from %+v", c.Name, c.Label, *c.Was)
		}
	}
}

// The labels go through the GitHub forge adapter, with its token rule:
// GITHUB_TOKEN for github.com, never GH_TOKEN. Serial: it sets the
// environment and GitHubAPI.
func TestLabelsThroughTheAdapter(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "forge-token")
	t.Setenv("GH_TOKEN", "gh-token")
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, r.Header.Get("Authorization")+" "+r.Method+" "+r.URL.Path+" "+strings.TrimSpace(string(body)))
		mu.Unlock()
		if r.URL.Path == "/repos/o/r/labels" && r.Method == http.MethodGet {
			_, _ = io.WriteString(w, `[{"id":1,"name":"bug","color":"d73a4a","description":"Broken"}]`)
		}
	}))
	t.Cleanup(srv.Close)
	saved := GitHubAPI
	t.Cleanup(func() { GitHubAPI = saved })
	GitHubAPI = srv.URL

	l, err := OpenLabeler(t.Context(), forge.Table{Kind: "github", Host: "github.com", Project: "o/r"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := l.Labels(t.Context()); err != nil || !slices.Equal(got, []forge.Label{{Name: "bug", Color: "d73a4a", Description: "Broken"}}) {
		t.Fatalf("Labels = %+v, %v", got, err)
	}
	if err := l.SetLabel(t.Context(), forge.Label{Name: "status:ready", Color: "00897f"}, false); err != nil {
		t.Fatal(err)
	}
	if err := l.SetLabel(t.Context(), forge.Label{Name: "feat", Color: "0e8a16", Description: "New feature"}, true); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, c := range calls {
		if !strings.HasPrefix(c, "Bearer forge-token ") && !strings.HasPrefix(c, "token forge-token ") {
			t.Errorf("not the forge's token: %q", c)
		}
	}
	var writes []string
	for _, c := range calls {
		if _, rest, _ := strings.Cut(c, "forge-token "); !strings.HasPrefix(rest, "GET ") {
			writes = append(writes, rest)
		}
	}
	want := []string{
		`POST /repos/o/r/labels {"color":"00897f","description":"","name":"status:ready"}`,
		`PATCH /repos/o/r/labels/feat {"color":"0e8a16","description":"New feature"}`,
	}
	if !slices.Equal(writes, want) {
		t.Errorf("writes\n%s\nwant\n%s", strings.Join(writes, "\n"), strings.Join(want, "\n"))
	}
}
