package ghlabels

import (
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

	"github.com/guilbep/git-board/internal/config"
)

func names(labels []Label) []string {
	var res []string
	for _, l := range labels {
		res = append(res, l.Name)
	}
	return res
}

func TestDesiredCoversFieldOptionsAndSprints(t *testing.T) {
	cfg := &config.Config{
		Fields: map[string]config.Field{
			"status":   {Single: true, Options: []string{"ready", "backlog"}, Colors: map[string]string{"ready": "#0E8A16"}},
			"prio":     {Single: true, Options: []string{"p0"}, Descriptions: map[string]string{"p0": "Now"}},
			"assignee": {Single: false}, // no Options -- free-form, no label to create
		},
		Sprints: config.SprintsConfig{
			List: []config.Sprint{{Slug: "delivery"}, {Slug: "post-delivery"}},
		},
		Branches: config.Branches{Kinds: map[string]string{"bug": "fix"}},
	}

	got := Desired(cfg)
	want := []string{
		"prio:p0",
		"sprint:delivery",
		"sprint:post-delivery",
		"status:ready", // a field's values in their order
		"status:backlog",
	}
	if !reflect.DeepEqual(names(got), want) {
		t.Errorf("Desired = %v, want %v", names(got), want)
	}
	if p0 := got[0]; p0.Color != "e00000" || p0.OwnColor || p0.Description != "Now" || !p0.OwnDescription {
		t.Errorf("p0 = %+v, want the board's red, not its own, and its description", p0)
	}
	if ready := got[3]; ready.Color != "0e8a16" || !ready.OwnColor {
		t.Errorf("ready = %+v, want its own hex", ready)
	}

	// flat kinds are the kind field, stored as the plain label
	cfg.Fields["kind"] = config.Field{Single: true, Options: []string{"bug"}, Flat: true}
	if got := names(Desired(cfg)); !slices.Contains(got, "bug") || slices.Contains(got, "kind:bug") {
		t.Errorf("flat kinds: Desired = %v, want bug", got)
	}
}

func TestDesiredEmptyConfigIsEmpty(t *testing.T) {
	if got := Desired(&config.Config{}); len(got) != 0 {
		t.Errorf("Desired(empty config) = %v, want empty", got)
	}
}

// The org's labels keep their colors: a label is changed only where the
// file sets its color or description, never for git-board's own colors.
func TestPlanChangesOnlyWhatTheFileSets(t *testing.T) {
	t.Parallel()
	desired := []Label{
		{Name: "bug", Color: "e00000"},                                                   // the board's color: GitHub's stays
		{Name: "feat", Color: "0e8a16", OwnColor: true},                                  // the file's color
		{Name: "docs", Color: "0075CA", Description: "Docs", OwnDescription: true},       // the file's description
		{Name: "perf", Color: "a2eeef", OwnColor: true},                                  // already so
		{Name: "status:ready", Color: "00897f", Description: "Go", OwnDescription: true}, // missing
	}
	existing := []Label{
		{Name: "Bug", Color: "d73a4a", Description: "Something isn't working"},
		{Name: "feat", Color: "A2EEEF", Description: "New feature"},
		{Name: "docs", Color: "0075ca"},
		{Name: "perf", Color: "A2EEEF"},
		{Name: "wontfix", Color: "ffffff"},
	}
	got := Plan("o/r", desired, existing, func(_, _ string) bool { return false })
	want := []Change{
		{Repo: "o/r", Name: "feat", Color: "0e8a16", Description: "New feature", Update: true, Was: &existing[1]},
		{Repo: "o/r", Name: "docs", Color: "0075ca", Description: "Docs", Update: true, Was: &existing[2]},
		{Repo: "o/r", Name: "status:ready", Color: "00897f", Description: "Go"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Plan =\n%+v\nwant\n%+v", got, want)
	}
	for i, want := range []string{"color", "color", "description", "same", "missing"} {
		if got := State(desired[i], existing); got != want {
			t.Errorf("State(%s) = %s, want %s", desired[i].Name, got, want)
		}
	}
	cfg := &config.Config{Fields: map[string]config.Field{"assignee": {}}}
	existing = append(existing, Label{Name: "assignee:alice"}, Label{Name: "kind:bug"})
	if got := names(Stray(cfg, desired, existing)); !slices.Equal(got, []string{"wontfix", "kind:bug"}) {
		t.Errorf("Stray = %v, want wontfix, kind:bug", got)
	}
}

// The labels go through the GitHub forge adapter, with its token rule:
// GITHUB_TOKEN, never gh's. Serial: it sets the environment and API.
func TestThroughTheAdapter(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "forge-token")
	t.Setenv("GH_TOKEN", "gh-token")
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		calls = append(calls, r.Header.Get("Authorization")+" "+r.Method+" "+r.URL.Path+" "+strings.TrimSpace(string(body)))
		mu.Unlock()
		switch r.URL.Path {
		case "/repos/o/r/labels":
			if r.Method == http.MethodGet {
				_, _ = io.WriteString(w, `[{"id":1,"name":"bug","color":"d73a4a","description":"Broken"}]`)
			}
		case "/graphql":
			_, _ = io.WriteString(w, `{"data":{"repository":{"label":{"issues":{"totalCount":2},"pullRequests":{"totalCount":1}}}}}`)
		}
	}))
	t.Cleanup(srv.Close)
	saved := API
	t.Cleanup(func() { API = saved })
	API = srv.URL

	if got, err := Existing("o/r"); err != nil || !slices.Equal(got, []Label{{Name: "bug", Color: "d73a4a", Description: "Broken"}}) {
		t.Fatalf("Existing = %+v, %v", got, err)
	}
	if err := Apply(Change{Repo: "o/r", Name: "status:ready", Color: "00897f"}); err != nil {
		t.Fatal(err)
	}
	if err := Apply(Change{Repo: "o/r", Name: "feat", Color: "0e8a16", Description: "New feature", Update: true}); err != nil {
		t.Fatal(err)
	}
	if n, err := Count("o/r", "good first issue"); err != nil || n != 3 {
		t.Fatalf("Count = %d, %v", n, err)
	}
	if err := Delete("o/r", "wontfix"); err != nil {
		t.Fatal(err)
	}
	mu.Lock()
	defer mu.Unlock()
	for _, c := range calls {
		if !strings.HasPrefix(c, "Bearer forge-token ") {
			t.Errorf("not the forge's token: %q", c)
		}
	}
	want := []string{
		`POST /repos/o/r/labels {"color":"00897f","description":"","name":"status:ready"}`,
		`PATCH /repos/o/r/labels/feat {"color":"0e8a16","description":"New feature"}`,
	}
	if got := []string{strings.TrimPrefix(calls[1], "Bearer forge-token "), strings.TrimPrefix(calls[2], "Bearer forge-token ")}; !slices.Equal(got, want) {
		t.Errorf("writes\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if last := calls[len(calls)-1]; last != "Bearer forge-token DELETE /repos/o/r/labels/wontfix " {
		t.Errorf("last call %q", last)
	}
}

func TestTemplatesReadsEveryLabelsShape(t *testing.T) {
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
	got, err := Templates(dir)
	if err != nil {
		t.Fatalf("Templates: %v", err)
	}
	want := []Template{
		{File: "a.yaml", Labels: []string{"enhancement", "triage"}},
		{File: "b.md", Labels: []string{"bug", "needs-triage"}},
		{File: "c.md", Labels: []string{"feat"}},
		{File: "e.md", Labels: []string{"specs"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Templates = %+v, want %+v", got, want)
	}
}
