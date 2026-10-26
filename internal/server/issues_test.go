package server

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

func newIssueServer(t *testing.T) (*httptest.Server, *apiServer) {
	t.Helper()
	b := storetest.New(t)
	// the settings file is the test's own: a save never reaches the checkout's
	s := &apiServer{cfg: &config.Config{}, b: b, me: "guilbep", cfgPath: filepath.Join(t.TempDir(), config.FileName)}
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
	var th store.Thread
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
	var again store.Thread
	if code := call(t, "GET", base+"/thread", nil, &again); code != http.StatusOK || len(again.Events) != len(th.Events) {
		t.Errorf("thread: %d, %d events, want %d", code, len(again.Events), len(th.Events))
	}
}

func TestIssueWritesAreChecked(t *testing.T) {
	t.Parallel()
	ts, _ := newIssueServer(t)
	var one, two store.Thread
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

// fakeSync answers a forge sync with given events, holding the run until
// release is closed.
func fakeSync(events []store.SyncEvent, release chan struct{}) func(context.Context, string, []config.Forge, bool, func(store.SyncEvent)) error {
	return func(_ context.Context, _ string, _ []config.Forge, _ bool, see func(store.SyncEvent)) error {
		<-release
		for _, e := range events {
			see(e)
		}
		return nil
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
	ts, s := forgeServer(t, ghForge, glForge)
	release := make(chan struct{})
	ev := func(forge, what string, ops int) store.SyncEvent {
		return store.SyncEvent{Forge: forge, What: what, Issue: "0123456789", Ops: ops}
	}
	events := []store.SyncEvent{ev("gh", "posted", 0), ev("gh", "pushed", 0), ev("gh", "linked", 0), ev("gh", "edit", 0), ev("gh", "change", 0), ev("gh", "change", 0),
		ev("gl", "created", 3), ev("gl", "updated", 4), {Forge: "gl", What: "warning", Line: "gitlab.epfl.ch/42400 refused labels"}}
	for range 5 {
		events = append(events, ev("gh", "comment", 0))
	}
	s.runSync = fakeSync(events, release)

	if code := call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": false}, nil); code != http.StatusConflict {
		t.Errorf("a real run before a dry run: %d, want 409", code)
	}
	if code := call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": true}, nil); code != http.StatusAccepted {
		t.Fatalf("dry run: %d", code)
	}
	// edits wait for the sync
	var th store.Thread
	if code := call(t, "POST", ts.URL+"/api/issues", map[string]any{"title": "during"}, &th); code != http.StatusConflict {
		t.Errorf("a write during a sync: %d, want 409", code)
	}
	close(release)
	st := waitJob(t, ts)
	gh, gl := st.Job.Bridges["gh"], st.Job.Bridges["gl"]
	if st.Job.State != "done" || !st.Job.DryRun || gh == nil || gl == nil {
		t.Fatalf("dry run job %+v", st.Job)
	}
	if want := (direction{Issues: 1, Changed: 1, Comments: 5, Edits: 1, Changes: 2}); gh.To != want || gh.Linked != 1 {
		t.Errorf("gh %+v, want to %+v, 1 linked", gh, want)
	}
	if want := (direction{Issues: 1, Changed: 1, Changes: 7}); gl.From != want || gl.Warnings < 1 || !slices.Contains(gl.Lines, "gitlab.epfl.ch/42400 refused labels") {
		t.Errorf("gl %+v, want from %+v and a warning", gl, want) // and maybe who can be assigned's, asked after the run
	}
	if !slices.Contains(gh.Lines, "would create 0123456") {
		t.Errorf("gh lines %q", gh.Lines)
	}

	release = make(chan struct{})
	close(release)
	s.runSync = fakeSync([]store.SyncEvent{ev("gh", "posted", 0)}, release)
	if code := call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": false}, nil); code != http.StatusAccepted {
		t.Fatalf("real run after a dry run: %d", code)
	}
	if st = waitJob(t, ts); st.Job.DryRun || st.Job.Bridges["gh"].To.Issues != 1 {
		t.Errorf("real run job %+v", st.Job)
	}
	if code := call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": false}, nil); code != http.StatusConflict {
		t.Errorf("a second real run without a new dry run: %d, want 409", code)
	}

	// a dry run that fails doesn't open the way to a real run
	s.runSync = func(context.Context, string, []config.Forge, bool, func(store.SyncEvent)) error {
		return context.Canceled
	}
	call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": true}, nil)
	if st = waitJob(t, ts); st.Job.State != "failed" || st.Job.Errors != 1 {
		t.Fatalf("dry run with an error: %+v", st.Job)
	}
	if code := call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": false}, nil); code != http.StatusConflict {
		t.Errorf("a real run after a failed dry run: %d, want 409", code)
	}
}

// Send is the push alone (the Work page's Send all), after a clean dry
// run. A send after a dry run that found issues on a forge by their marker
// runs: the core links them before it creates anything.
func TestForgeSend(t *testing.T) {
	t.Parallel()
	ts, s := forgeServer(t, ghForge)
	done := make(chan struct{})
	close(done)
	ran := make(chan string, 10) // the mode each run was given
	runs := func(events ...store.SyncEvent) {
		s.runSync = func(ctx context.Context, mode string, forges []config.Forge, dryRun bool, see func(store.SyncEvent)) error {
			ran <- mode
			return fakeSync(events, done)(ctx, mode, forges, dryRun, see)
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

	runs(store.SyncEvent{Forge: "gh", Way: "pull", What: "linked", Issue: "0123456789"})
	if code, _ := start(map[string]any{"mode": "send"}); code != http.StatusConflict {
		t.Errorf("a send before a dry run: %d, want 409", code)
	}
	for _, mode := range []string{"push", "get"} {
		if code, _ := start(map[string]any{"mode": mode}); code != http.StatusBadRequest {
			t.Errorf("mode %s: %d, want 400", mode, code)
		}
	}
	if _, mode := start(map[string]any{"dry_run": true, "mode": "send"}); mode != "sync" {
		t.Errorf("a dry run ran as %q, want both ways", mode)
	}
	if code, mode := start(map[string]any{"mode": "send"}); code != http.StatusAccepted || mode != "send" {
		t.Errorf("a send after a dry run found an issue by its marker: %d %s", code, mode)
	}
	runs()
	start(map[string]any{"dry_run": true})
	if code, mode := start(map[string]any{"mode": "send", "bridges": []string{"gh"}}); code != http.StatusAccepted || mode != "send" {
		t.Errorf("a send after a clean dry run: %d %s", code, mode)
	}
}

// The default run opens the settings' forges fresh, and a check through
// it writes nothing: the run after it does what it said.
func TestForgeSyncOverTheCore(t *testing.T) {
	t.Parallel()
	ts, s := forgeServer(t, ghForge)
	gh := fake.New("github.com/guilbep/git-bug-sync-test")
	s.openForges = func(_ context.Context, forges []config.Forge) ([]store.NamedForge, error) {
		if len(forges) != 1 || forges[0].Name != "gh" {
			t.Errorf("opened %+v, want gh", forges)
		}
		return []store.NamedForge{{Name: "gh", Forge: gh}}, nil
	}
	storetest.NewIssue(t, s.b, "to GitHub")
	call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": true}, nil)
	if st := waitJob(t, ts); st.Job.State != "done" || st.Job.Bridges["gh"].To.Issues != 1 || len(gh.Writes()) != 0 {
		t.Fatalf("check %+v, writes %v", st.Job, gh.Writes())
	}
	call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": false}, nil)
	if st := waitJob(t, ts); st.Job.State != "done" || st.Job.Bridges["gh"].To.Issues != 1 || len(gh.All()) != 1 {
		t.Fatalf("run %+v, forge %v", st.Job, gh.All())
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

// A comment posted on a step carries it; one without stays step 1's.
func TestCommentOnAStep(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	id, err := s.b.NewIssue("Export CSV", "", nil)
	if err != nil {
		t.Fatal(err)
	}
	base := ts.URL + "/api/issues/" + id[:8] + "/comments"
	var th store.Thread
	if code := call(t, "POST", base, map[string]any{"body": "plan looks fine", "step": 3}, &th); code != http.StatusOK {
		t.Fatalf("comment on step 3: %d", code)
	}
	if e := th.Events[len(th.Events)-1]; e.Meta["cycle:step"] != "3" || e.Body != "plan looks fine" {
		t.Errorf("step 3's comment %+v", e)
	}
	if code := call(t, "POST", base, map[string]any{"body": "plain"}, &th); code != http.StatusOK || th.Events[len(th.Events)-1].Meta["cycle:step"] != "" {
		t.Errorf("a plain comment: %d %+v", code, th.Events[len(th.Events)-1])
	}
	if code := call(t, "POST", base, map[string]any{"body": "x", "step": 8}, nil); code != http.StatusBadRequest {
		t.Errorf("step 8: %d", code)
	}
}

// With no [forges.<name>] table, /api/forges lists the forges of the git
// remotes as found there, and a run opens them.
func TestForgeSyncFindsTheForges(t *testing.T) {
	t.Parallel()
	dir := storetest.Repo(t, false)
	storetest.Git(t, dir, "remote", "add", "gh", "git@github.com:o/r.git")
	storetest.Init(t, dir)
	s := &apiServer{cfg: &config.Config{Discover: true}, b: storetest.Open(t, dir), me: "guilbep", cfgPath: filepath.Join(t.TempDir(), config.FileName)}
	ts := httptest.NewServer(withCORS(newMux(s, http.NotFoundHandler())))
	t.Cleanup(ts.Close)
	var st forgeStatus
	call(t, "GET", ts.URL+"/api/forges", nil, &st)
	if want := []syncBridge{{Name: "github", Target: "github", Discovered: true}}; !slices.Equal(st.Bridges, want) {
		t.Fatalf("bridges %+v", st.Bridges)
	}
	gh := fake.New("github.com/o/r")
	s.openForges = func(_ context.Context, forges []config.Forge) ([]store.NamedForge, error) {
		if len(forges) != 1 || forges[0].Project != "o/r" || !forges[0].Discovered {
			t.Errorf("opened %+v", forges)
		}
		return []store.NamedForge{{Name: "github", Forge: gh}}, nil
	}
	call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": true}, nil)
	if st := waitJob(t, ts); st.Job.State != "done" || st.Job.Bridges["github"] == nil {
		t.Fatalf("check %+v", st.Job)
	}
}
