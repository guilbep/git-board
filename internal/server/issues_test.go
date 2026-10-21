package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/git-bug/git-bug/bridge/core"
	"github.com/git-bug/git-bug/entity"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

func newIssueServer(t *testing.T) (*httptest.Server, *apiServer) {
	t.Helper()
	b := gitbugiotest.New(t)
	s := &apiServer{cfg: &config.Config{}, b: b, me: "guilbep"}
	ts := httptest.NewServer(withCORS(newMux(s, http.NotFoundHandler())))
	t.Cleanup(ts.Close)
	return ts, s
}

// call sends a JSON request and decodes a 2xx answer into out.
func call(t *testing.T, method, url string, body, out any) int {
	t.Helper()
	data, _ := json.Marshal(body)
	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil && resp.StatusCode < 300 {
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			t.Fatalf("%s %s: %v", method, url, err)
		}
	}
	return resp.StatusCode
}

func TestIssueLifecycle(t *testing.T) {
	t.Parallel()
	ts, _ := newIssueServer(t)
	var th gitbugio.Thread
	if code := call(t, "POST", ts.URL+"/api/issues", map[string]any{
		"title": "Export CSV", "body": "see ![shot](https://github.com/user-attachments/assets/a)", "labels": []string{"status:ready"},
	}, &th); code != http.StatusCreated {
		t.Fatalf("create: %d", code)
	}
	if th.Title != "Export CSV" || len(th.Events) != 2 || !strings.Contains(th.Events[0].HTML, "/api/media?url=https%3A%2F%2Fgithub.com") {
		t.Fatalf("created %+v", th)
	}
	base := ts.URL + "/api/issues/" + th.ID

	if code := call(t, "POST", base+"/comments", map[string]any{"body": "first"}, &th); code != http.StatusOK {
		t.Fatalf("comment: %d", code)
	}
	c := th.Events[len(th.Events)-1]
	if code := call(t, "PUT", base+"/comments/"+c.ID, map[string]any{"body": "*first*", "base": "first"}, &th); code != http.StatusOK {
		t.Fatalf("edit: %d", code)
	}
	if code := call(t, "PUT", base+"/comments/"+c.ID, map[string]any{"body": "mine", "base": "first"}, nil); code != http.StatusConflict {
		t.Errorf("an edit of a comment changed meanwhile: %d, want 409", code)
	}
	if last := th.Events[len(th.Events)-1]; !last.Edited || !strings.Contains(last.HTML, "<em>first</em>") {
		t.Errorf("edited %+v", last)
	}
	if code := call(t, "PUT", base+"/title", map[string]any{"title": "Export CSV and XLSX"}, &th); code != http.StatusOK || th.Title != "Export CSV and XLSX" {
		t.Errorf("title: %d %q", code, th.Title)
	}
	if code := call(t, "POST", base+"/labels", map[string]any{"add": []string{"bug"}, "remove": []string{"status:ready"}}, &th); code != http.StatusOK {
		t.Errorf("labels: %d", code)
	}
	if strings.Join(th.Labels, ",") != "bug" {
		t.Errorf("labels %v, want bug", th.Labels)
	}
	if code := call(t, "POST", base+"/comments", map[string]any{"body": "done", "status": "closed"}, &th); code != http.StatusOK || th.Status != "closed" {
		t.Errorf("close with comment: %d %s", code, th.Status)
	}
	var again gitbugio.Thread
	if code := call(t, "GET", base+"/thread", nil, &again); code != http.StatusOK || len(again.Events) != len(th.Events) {
		t.Errorf("thread: %d, %d events, want %d", code, len(again.Events), len(th.Events))
	}
}

func TestIssueWritesAreChecked(t *testing.T) {
	t.Parallel()
	ts, _ := newIssueServer(t)
	var one, two gitbugio.Thread
	call(t, "POST", ts.URL+"/api/issues", map[string]any{"title": "one"}, &one)
	call(t, "POST", ts.URL+"/api/issues", map[string]any{"title": "two"}, &two)
	call(t, "POST", ts.URL+"/api/issues/"+one.ID+"/comments", map[string]any{"body": "hi"}, &one)
	comment := one.Events[len(one.Events)-1].ID

	for name, req := range map[string]struct {
		method, path string
		body         any
		want         int
	}{
		"another issue's comment": {"PUT", "/api/issues/" + two.ID + "/comments/" + comment, map[string]any{"body": "x"}, http.StatusBadRequest},
		"no title":                {"POST", "/api/issues", map[string]any{"title": "  "}, http.StatusBadRequest},
		"empty comment":           {"POST", "/api/issues/" + one.ID + "/comments", map[string]any{"body": ""}, http.StatusBadRequest},
		"bad status":              {"PUT", "/api/issues/" + one.ID + "/status", map[string]any{"status": "done"}, http.StatusBadRequest},
		"unknown issue":           {"GET", "/api/issues/ffffffff/thread", nil, http.StatusNotFound},
	} {
		if got := call(t, req.method, ts.URL+req.path, req.body, nil); got != req.want {
			t.Errorf("%s: %d, want %d", name, got, req.want)
		}
	}
}

func TestMeAndIdentities(t *testing.T) {
	t.Parallel()
	ts, _ := newIssueServer(t)
	var me meResponse
	if call(t, "GET", ts.URL+"/api/me", nil, &me); me.Me == nil || len(me.Identities) != 1 {
		t.Fatalf("me %+v", me)
	}
	if code := call(t, "POST", ts.URL+"/api/me", map[string]any{"name": "Bob", "email": "bob@example.com"}, &me); code != http.StatusOK || me.Me.Name != "Bob" {
		t.Errorf("create: %d %+v", code, me.Me)
	}
	first := me.Identities[0].ID
	if me.Identities[0].Name == "Bob" {
		first = me.Identities[1].ID
	}
	if code := call(t, "POST", ts.URL+"/api/me", map[string]any{"id": first}, &me); code != http.StatusOK || me.Me.ID != first {
		t.Errorf("adopt: %d %+v", code, me.Me)
	}
}

// fakeSync answers a forge sync with given results, holding the run until
// release is closed.
func fakeSync(results []core.SyncResult, release chan struct{}) func(context.Context, string, bool, []string, ...core.Option) (<-chan core.SyncResult, error) {
	return func(context.Context, string, bool, []string, ...core.Option) (<-chan core.SyncResult, error) {
		out := make(chan core.SyncResult)
		go func() {
			defer close(out)
			<-release
			for _, r := range results {
				out <- r
			}
		}()
		return out, nil
	}
}

func waitJob(t *testing.T, ts *httptest.Server) forgeStatus {
	t.Helper()
	var st forgeStatus
	for range 100 {
		call(t, "GET", ts.URL+"/api/forges", nil, &st)
		if st.Job != nil && st.Job.State != "running" {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("the sync didn't end: %+v", st.Job)
	return st
}

func TestForgeSyncRunsADryRunFirst(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	release := make(chan struct{})
	s.runSync = fakeSync([]core.SyncResult{
		{Bridge: "gh", Export: &core.ExportResult{Event: core.ExportEventNothing, Reason: "would create issue: Export CSV, with 2 comments"}},
		{Bridge: "gh", Export: &core.ExportResult{Event: core.ExportEventNothing, Reason: "would send: 3 comments, 1 edits, labels, status"}},
		{Bridge: "gh", Export: &core.ExportResult{Event: core.ExportEventNothing, Reason: "missing author token"}},
		{Bridge: "gh", Export: &core.ExportResult{Event: core.ExportEventNothing, Reason: "would sync with https://github.com/o/r/issues/9, found by its git-bug marker"}},
		{Bridge: "gl", Import: &core.ImportResult{Event: core.ImportEventNothing, Reason: "would import issue https://gitlab.example.com/g/r/-/work_items/3"}},
		{Bridge: "gl", Import: &core.ImportResult{Event: core.ImportEventNothing, Reason: "would pull up to 4 changes from https://gitlab.example.com/g/r/-/work_items/5"}},
		{Bridge: "gl", Import: &core.ImportResult{Event: core.ImportEventWarning, Err: context.Canceled}},
	}, release)

	if code := call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": false}, nil); code != http.StatusConflict {
		t.Errorf("a real run before a dry run: %d, want 409", code)
	}
	if code := call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": true}, nil); code != http.StatusAccepted {
		t.Fatalf("dry run: %d", code)
	}
	// edits wait for the sync
	var th gitbugio.Thread
	if code := call(t, "POST", ts.URL+"/api/issues", map[string]any{"title": "during"}, &th); code != http.StatusConflict {
		t.Errorf("a write during a sync: %d, want 409", code)
	}
	close(release)
	st := waitJob(t, ts)
	gh, gl := st.Job.Bridges["gh"], st.Job.Bridges["gl"]
	if st.Job.State != "done" || !st.Job.DryRun || gh == nil || gl == nil {
		t.Fatalf("dry run job %+v", st.Job)
	}
	if want := (direction{Issues: 1, Changed: 1, Comments: 5, Edits: 1, Changes: 2}); gh.To != want || gh.StaysLocal != 1 || gh.Linked != 1 {
		t.Errorf("gh %+v, want to %+v, 1 local, 1 linked", gh, want)
	}
	if want := (direction{Issues: 1, Changed: 1, Changes: 4}); gl.From != want || gl.Warnings != 1 {
		t.Errorf("gl %+v, want from %+v and a warning", gl, want)
	}

	release = make(chan struct{})
	close(release)
	s.runSync = fakeSync([]core.SyncResult{{Bridge: "gh", Export: &core.ExportResult{Event: core.ExportEventBug}}}, release)
	if code := call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": false}, nil); code != http.StatusAccepted {
		t.Fatalf("real run after a dry run: %d", code)
	}
	if st = waitJob(t, ts); st.Job.DryRun || st.Job.Bridges["gh"].To.Issues != 1 {
		t.Errorf("real run job %+v", st.Job)
	}
	if code := call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": false}, nil); code != http.StatusConflict {
		t.Errorf("a second real run without a new dry run: %d, want 409", code)
	}

	// a dry run with errors doesn't open the way to a real run
	release = make(chan struct{})
	close(release)
	s.runSync = fakeSync([]core.SyncResult{{Bridge: "gl", Import: &core.ImportResult{Event: core.ImportEventError, Err: context.Canceled}}}, release)
	call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": true}, nil)
	if st = waitJob(t, ts); st.Job.Errors != 1 {
		t.Fatalf("dry run with an error: %+v", st.Job)
	}
	if code := call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": false}, nil); code != http.StatusConflict {
		t.Errorf("a real run after a dry run with errors: %d, want 409", code)
	}
}

// Get and Send are one way of a sync, each after a clean dry run. Send
// alone would create again the issues the dry run found on a forge by
// their git-bug marker: those wait for a Get or a Sync.
func TestForgeGetAndSend(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	done := make(chan struct{})
	close(done)
	ran := make(chan string, 10) // the mode each run was given
	runs := func(results ...core.SyncResult) {
		s.runSync = func(ctx context.Context, mode string, dryRun bool, names []string, _ ...core.Option) (<-chan core.SyncResult, error) {
			ran <- mode
			return fakeSync(results, done)(ctx, mode, dryRun, names)
		}
	}
	start := func(body map[string]any) (int, string) {
		t.Helper()
		code := call(t, "POST", ts.URL+"/api/forges/sync", body, nil)
		if code != http.StatusAccepted {
			return code, ""
		}
		mode := <-ran
		if st := waitJob(t, ts); st.Job.Mode != mode {
			t.Errorf("job %+v, run as %s", st.Job, mode)
		}
		return code, mode
	}

	runs(core.SyncResult{Bridge: "gh", Export: &core.ExportResult{Event: core.ExportEventNothing, Reason: "would sync with https://github.com/o/r/issues/9, found by its git-bug marker"}})
	if code, _ := start(map[string]any{"mode": "send"}); code != http.StatusConflict {
		t.Errorf("a send before a dry run: %d, want 409", code)
	}
	if code, _ := start(map[string]any{"mode": "push"}); code != http.StatusBadRequest {
		t.Errorf("an unknown mode: %d, want 400", code)
	}
	if _, mode := start(map[string]any{"dry_run": true, "mode": "get"}); mode != "sync" {
		t.Errorf("a dry run ran as %q, want both ways", mode)
	}
	if code, _ := start(map[string]any{"mode": "send"}); code != http.StatusConflict {
		t.Errorf("a send after a dry run found an issue by its marker: %d, want 409", code)
	}
	runs()
	if code, mode := start(map[string]any{"mode": "get"}); code != http.StatusAccepted || mode != "get" {
		t.Errorf("a get: %d %s", code, mode)
	}
	start(map[string]any{"dry_run": true})
	if code, mode := start(map[string]any{"mode": "send"}); code != http.StatusAccepted || mode != "send" {
		t.Errorf("a send after a clean dry run: %d %s", code, mode)
	}
}

// Send doesn't get the forges' changes first, and sends an issue's labels
// whole: an issue the check found changed on both sides would lose the
// labels added on the forge, and the comment texts edited there. Send
// waits for a Get or a Sync on that forge; one issue changed on each side
// loses nothing.
func TestForgeSendRefusesIssuesChangedBothWays(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	done := make(chan struct{})
	close(done)
	check := func(results ...core.SyncResult) *bridgeTally {
		t.Helper()
		s.runSync = fakeSync(results, done)
		if code := call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": true}, nil); code != http.StatusAccepted {
			t.Fatalf("check: %d", code)
		}
		return waitJob(t, ts).Job.Bridges["gh"]
	}
	send := func(bridges ...string) int {
		t.Helper()
		s.runSync = fakeSync(nil, done)
		code := call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"mode": "send", "bridges": bridges}, nil)
		if code == http.StatusAccepted {
			waitJob(t, ts)
		}
		return code
	}
	pulls := func(bridge, id string) core.SyncResult {
		return core.SyncResult{Bridge: bridge, Import: &core.ImportResult{Event: core.ImportEventNothing, EntityId: entity.Id(id), Reason: "would pull up to 2 changes from https://github.com/o/r/issues/1"}}
	}
	sends := func(bridge, id string) core.SyncResult {
		return core.SyncResult{Bridge: bridge, Export: &core.ExportResult{Event: core.ExportEventNothing, EntityId: entity.Id(id), Reason: "would send: labels"}}
	}
	imports := core.SyncResult{Bridge: "gh", Import: &core.ImportResult{Event: core.ImportEventNothing, Reason: "would import issue https://github.com/o/r/issues/3"}}

	if gh := check(pulls("gh", "x"), sends("gh", "y"), imports, pulls("gl", "z")); gh.Conflicts != 0 {
		t.Errorf("conflicts %d, want none: x changed on GitHub, y here", gh.Conflicts)
	}
	if code := send(); code != http.StatusAccepted {
		t.Errorf("a send where no issue changed both ways: %d", code)
	}

	// the order the check reports them in doesn't matter
	if gh := check(sends("gh", "x"), pulls("gh", "x"), pulls("gl", "z")); gh.Conflicts != 1 {
		t.Errorf("conflicts %d, want x", gh.Conflicts)
	}
	if code := send(); code != http.StatusConflict {
		t.Errorf("a send to a forge with an issue changed both ways: %d, want 409", code)
	}
	if code := send("gl"); code != http.StatusAccepted {
		t.Errorf("a send to the other forge alone: %d", code)
	}
}

func TestMarkdownPreview(t *testing.T) {
	t.Parallel()
	ts, _ := newIssueServer(t)
	var res struct {
		HTML string `json:"html"`
	}
	if code := call(t, "POST", ts.URL+"/api/markdown", map[string]any{"body": "**hi** <script>x</script>"}, &res); code != http.StatusOK {
		t.Fatalf("preview: %d", code)
	}
	if !strings.Contains(res.HTML, "<strong>hi</strong>") || strings.Contains(res.HTML, "<script") {
		t.Errorf("html %q", res.HTML)
	}
}
