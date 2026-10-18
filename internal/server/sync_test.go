package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghbridge"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

const issueURL = "https://github.com/o/r/issues/7"

type recordedEdits struct{ edits []ghbridge.ProjectEdit }

func (r *recordedEdits) Edit(_ string, e ghbridge.ProjectEdit) error {
	r.edits = append(r.edits, e)
	return nil
}

// syncFixture is issue #7, "in-review" in git-board and "QA Validation" on
// the Project, last agreed at "ready": a conflict on status.
func syncFixture(t *testing.T, write bool) (*httptest.Server, *gitbugio.Backend, string, *recordedEdits) {
	t.Helper()
	cfg := &config.Config{
		Fields: map[string]config.Field{"status": {Single: true,
			Options: []string{"ready", "in-review", "qa"}, Project: map[string]string{"qa": "QA Validation"}}},
		GithubProject: config.GithubProject{Owner: "o", Number: 1, Write: write},
	}
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewImportedIssue(t, b, "Results page", issueURL)
	if _, err := b.ApplyLabelChange(id, []string{"status:in-review"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := b.SetProjectBases(id, ghbridge.BasePrefix(cfg.GithubProject), map[string]string{"status": "ready"}); err != nil {
		t.Fatal(err)
	}
	rec := &recordedEdits{}
	project := ghbridge.Project{
		Items: func(string, int, int) ([]ghbridge.Item, error) {
			return []ghbridge.Item{{ID: "item-7", Title: "Results page", Status: "QA Validation",
				Content: ghbridge.Content{Number: 7, Type: "Issue", URL: issueURL}}}, nil
		},
		Meta: func(string, int) (ghbridge.ProjectMeta, error) {
			return ghbridge.ProjectMeta{ID: "P", Fields: map[string]ghbridge.ProjectField{"Status": {ID: "F-status",
				Options: map[string]string{"Ready": "o-ready", "In review": "o-review", "QA Validation": "o-qa"}}}}, nil
		},
		Writer: rec,
	}
	s := &apiServer{cfg: cfg, b: b, me: "guilbep", project: project}
	ts := httptest.NewServer(withCORS(newMux(s, http.NotFoundHandler())))
	t.Cleanup(ts.Close)
	return ts, b, id, rec
}

func postJSON(t *testing.T, url string, body, out any) *http.Response {
	t.Helper()
	resp, err := http.Post(url, "application/json", jsonBody(t, body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	if out != nil && resp.StatusCode == http.StatusOK {
		decodeJSON(t, resp, out)
	}
	return resp
}

func TestSyncStatusReadsNothingUntilAsked(t *testing.T) {
	ts, _, _, _ := syncFixture(t, false)
	var st syncStatus
	getJSON(t, ts.URL+"/api/sync", &st)
	if st.Project != "o/1" || st.Last != nil {
		t.Errorf("status = %+v, want the project and no check yet", st)
	}
}

func TestSyncCheckListsConflicts(t *testing.T) {
	ts, _, _, rec := syncFixture(t, true)
	var st syncStatus
	if resp := postJSON(t, ts.URL+"/api/sync/check", nil, &st); resp.StatusCode != http.StatusOK {
		t.Fatalf("check: %d", resp.StatusCode)
	}
	if st.Last == nil || len(st.Last.Conflicts) != 1 {
		t.Fatalf("last = %+v, want one conflict", st.Last)
	}
	c := st.Last.Conflicts[0]
	if c.Number != 7 || c.Field != "status" || c.Local != "in-review" || c.Project != "qa" || c.Base != "ready" {
		t.Errorf("conflict = %+v", c)
	}
	if len(rec.edits) != 0 {
		t.Errorf("a check wrote to the Project: %+v", rec.edits)
	}
	var again syncStatus
	getJSON(t, ts.URL+"/api/sync", &again)
	if again.Last == nil || len(again.Last.Conflicts) != 1 {
		t.Errorf("the check wasn't kept: %+v", again.Last)
	}
}

func TestSyncResolveKeepsTheProjectsValue(t *testing.T) {
	ts, b, id, rec := syncFixture(t, false)
	var st syncStatus
	resp := postJSON(t, ts.URL+"/api/sync/resolve", map[string]any{"number": 7, "field": "status", "keep": "project"}, &st)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("resolve: %d", resp.StatusCode)
	}
	labels, err := b.GetLabels(id)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(labels, "status:qa") || len(rec.edits) != 0 {
		t.Errorf("labels %v edits %v, want status:qa and no Project write", labels, rec.edits)
	}
	if st.Last == nil || len(st.Last.Conflicts) != 0 {
		t.Errorf("after resolving: %+v, want no conflict", st.Last)
	}
}

func TestSyncResolveKeepingGitBoardNeedsWrite(t *testing.T) {
	ts, _, _, rec := syncFixture(t, false)
	resp := postJSON(t, ts.URL+"/api/sync/resolve", map[string]any{"number": 7, "field": "status", "keep": "git-board"}, nil)
	if resp.StatusCode != http.StatusBadRequest || len(rec.edits) != 0 {
		t.Errorf("status %d edits %v, want 400 and nothing written", resp.StatusCode, rec.edits)
	}

	ts, _, _, rec = syncFixture(t, true)
	if resp := postJSON(t, ts.URL+"/api/sync/resolve", map[string]any{"number": 7, "field": "status", "keep": "git-board"}, nil); resp.StatusCode != http.StatusOK {
		t.Fatalf("resolve with write: %d", resp.StatusCode)
	}
	if len(rec.edits) != 1 || rec.edits[0].Arg != "o-review" {
		t.Errorf("edits %+v, want status set to In review", rec.edits)
	}
}

func TestSyncWithoutAProjectIsABadRequest(t *testing.T) {
	b := gitbugiotest.New(t)
	ts := newTestServer(t, b, &config.Config{})
	var st syncStatus
	getJSON(t, ts.URL+"/api/sync", &st)
	if st.Project != "" {
		t.Errorf("project = %q, want none", st.Project)
	}
	if resp := postJSON(t, ts.URL+"/api/sync/check", nil, nil); resp.StatusCode != http.StatusBadRequest {
		t.Errorf("check without a project: %d, want 400", resp.StatusCode)
	}
}

func decodeJSON(t *testing.T, resp *http.Response, out any) {
	t.Helper()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		t.Fatalf("decode: %v", err)
	}
}
