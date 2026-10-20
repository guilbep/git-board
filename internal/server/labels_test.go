package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghlabels"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
	"github.com/guilbep/git-board/internal/lint"
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

// Serial: it answers for gh (ghlabels.Gh).
func TestLabelsTab(t *testing.T) {
	path := filepath.Join(t.TempDir(), config.FileName)
	if err := os.WriteFile(path, []byte(labelsTestConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	b := gitbugiotest.New(t)
	ts := newTestServer(t, b, cfg)
	both := gitbugiotest.NewIssue(t, b, "two kinds")
	if _, err := b.ApplyLabelChange(both, []string{"kind:feat", "kind:bug"}, nil); err != nil {
		t.Fatal(err)
	}

	var gh []string
	orig := ghlabels.Gh
	t.Cleanup(func() { ghlabels.Gh = orig })
	ghlabels.Gh = func(args ...string) ([]byte, error) {
		gh = append(gh, strings.Join(args, " "))
		switch args[0] + " " + args[1] {
		case "label list":
			return json.Marshal([]ghlabels.Label{{Name: "bug", Color: "d73a4a", Description: "Something isn't working"},
				{Name: "feat", Color: "a2eeef", Description: "New feature"}, {Name: "wontfix", Color: "ffffff"}})
		case "api search/issues?per_page=1&q=repo%3Ao%2Fr+label%3A%22wontfix%22":
			return []byte("2\n"), nil
		}
		return nil, nil
	}

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
	if slices.ContainsFunc(gh, func(c string) bool { return strings.HasPrefix(c, "label create") }) {
		t.Errorf("a dry run ran gh label create: %q", gh)
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
	if last := gh[len(gh)-1]; last != "label delete wontfix --repo o/r --yes" {
		t.Errorf("last gh call = %q", last)
	}
}
