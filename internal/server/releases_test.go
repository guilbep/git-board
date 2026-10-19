package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
	"github.com/guilbep/git-board/internal/release"
	"github.com/guilbep/git-board/internal/release/releasetest"
)

// releaseServer serves a releasetest fixture's app repo, the working
// directory, with its [release] table, its fake tools and its backend.
func releaseServer(t *testing.T) (*apiServer, *httptest.Server, *releasetest.Fixture) {
	t.Helper()
	r, tools, fx := releasetest.NewRunner(t)
	t.Chdir(fx.App)
	// the trunk named: the fixture's clone of an empty remote has no origin/HEAD
	cfg := &config.Config{Release: r.Cfg, Branches: config.Branches{Base: "dev"}}
	s := &apiServer{cfg: cfg, b: r.B, me: "dev", releaseTools: tools, releaseCache: t.TempDir()}
	ts := httptest.NewServer(withCORS(newMux(s, http.NotFoundHandler())))
	t.Cleanup(ts.Close)
	return s, ts, fx
}

// runs makes both targets' overlays of env run commit as version.
func runs(t *testing.T, fx *releasetest.Fixture, env, commit, version string) {
	t.Helper()
	for i := range fx.ConfigBare {
		fx.SetOverlay(t, i, env, release.Deployed{Commit: commit, Version: version})
	}
}

func firstCommit(t *testing.T, fx *releasetest.Fixture) string {
	return releasetest.Git(t, fx.App, "rev-list", "--max-parents=0", "HEAD")
}

// otherClone is another clone of the app, pushing tag on commit to the
// authority remote: this one hasn't fetched it.
func otherClone(t *testing.T, fx *releasetest.Fixture, commit, tag string) {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "other")
	releasetest.Git(t, filepath.Dir(dir), "clone", "-q", fx.AppBare, dir)
	releasetest.Git(t, dir, "tag", "-a", tag, "-m", tag, commit)
	releasetest.Git(t, dir, "push", "-q", "origin", "refs/tags/"+tag)
}

// coded sends a JSON request and returns the status and the error's code
// and text.
func coded(t *testing.T, method, url string, body any) (status int, code, msg string) {
	t.Helper()
	data, _ := json.Marshal(body)
	req, _ := http.NewRequest(method, url, bytes.NewReader(data))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var out struct{ Code, Error string }
	_ = json.NewDecoder(resp.Body).Decode(&out)
	return resp.StatusCode, out.Code, out.Error
}

// waitRelease polls the release job until it ends.
func waitRelease(t *testing.T, ts *httptest.Server) releaseJob {
	t.Helper()
	for deadline := time.Now().Add(30 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		var job releaseJob
		if call(t, "GET", ts.URL+"/api/releases/job", nil, &job) != http.StatusOK {
			t.Fatal("GET /api/releases/job failed")
		}
		if job.State != "running" {
			return job
		}
		if time.Now().After(deadline) {
			t.Fatalf("job still running: %+v", job)
		}
	}
}

func envNames(es []release.EnvState) []string {
	var names []string
	for _, e := range es {
		names = append(names, e.Name)
	}
	return names
}

func TestReleasesDisabled(t *testing.T) {
	ts, _ := newIssueServer(t)
	resp, err := http.Get(ts.URL + "/api/releases")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if got := strings.TrimSpace(string(body)); resp.StatusCode != http.StatusOK || got != `{"enabled":false}` {
		t.Errorf("GET /api/releases = %d %s", resp.StatusCode, got)
	}
	for _, w := range [][2]string{
		{"POST", "/api/releases"}, {"POST", "/api/releases/v1.0.0/promote"}, {"PUT", "/api/releases/v1.0.0/notes"},
		{"POST", "/api/releases/v1.0.0/comments"}, {"POST", "/api/releases/v1.0.0/yank"}, {"POST", "/api/releases/adopt"},
		{"POST", "/api/environments/dev/deploy"}, {"POST", "/api/environments/dev/rollback"},
	} {
		if status, _, msg := coded(t, w[0], ts.URL+w[1], map[string]string{}); status != http.StatusNotFound || msg != "no [release] in .git-board.toml" {
			t.Errorf("%s %s = %d %q", w[0], w[1], status, msg)
		}
	}
}

func TestReleasesStatus(t *testing.T) {
	_, ts, fx := releaseServer(t)
	c := firstCommit(t, fx)
	fx.Tag(t, c, "v1.0.0")
	runs(t, fx, "stage", c, "v1.0.0")

	resp, err := http.Get(ts.URL + "/api/releases")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	var got releasesResponse
	if err := json.Unmarshal(body, &got); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("%d %s: %v", resp.StatusCode, body, err)
	}
	if !got.Enabled || !slices.Equal(envNames(got.Environments), []string{"dev", "stage", "prod"}) || got.Job != nil {
		t.Errorf("got %+v", got)
	}
	if len(got.Releases) != 1 || got.Releases[0].Tag.Name != "v1.0.0" || !slices.Equal(got.Releases[0].RunsIn, []string{"stage"}) {
		t.Errorf("releases %+v", got.Releases)
	}
	if !strings.Contains(string(body), `"runsIn":["stage"]`) {
		t.Errorf("fields aren't lower camel case: %s", body)
	}
}

func TestDeployJobRunsAndPublishes(t *testing.T) {
	s, ts, fx := releaseServer(t)
	c := firstCommit(t, fx)
	fx.Tag(t, c, "v1.0.0")
	runs(t, fx, "dev", c, "v1.0.0")
	head := fx.Commit(t, "feat")
	events, cancel := s.events.subscribe()
	defer cancel()
	published := make(chan struct{}, 100)
	go func() {
		for data := range events {
			var e event
			if json.Unmarshal(data, &e) == nil && e.Kind == "releases" {
				published <- struct{}{}
			}
		}
	}()

	// {tag?}: an empty body is the trunk's head
	req, _ := http.NewRequest("POST", ts.URL+"/api/environments/dev/deploy", nil)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var job releaseJob
	_ = json.NewDecoder(resp.Body).Decode(&job)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusAccepted || job.Kind != "deploy" || job.State != "running" {
		t.Fatalf("POST deploy = %d %+v", resp.StatusCode, job)
	}
	job = waitRelease(t, ts)
	if job.State != "done" || len(job.Steps) == 0 || job.Ended == nil || job.Error != "" {
		t.Fatalf("job %+v", job)
	}
	for _, st := range job.Steps {
		if st.State != "done" {
			t.Errorf("step %+v, want each step once, done", st)
		}
	}
	// the POST's own, each step's running and done, and one after the
	// final state, which may come after the state was seen
	want := 2*len(job.Steps) + 2
	for n := 0; n < want; n++ {
		select {
		case <-published:
		case <-time.After(5 * time.Second):
			t.Fatalf("%d releases events, want %d: the POST's, two per step and the end's", n, want)
		}
	}
	var got releasesResponse
	getJSON(t, ts.URL+"/api/releases", &got)
	if got.Environments[0].Version != "1.0.1-dev.1.g"+head[:7] || got.Job == nil || got.Job.State != "done" {
		t.Errorf("after the deploy: dev %+v, job %+v", got.Environments[0], got.Job)
	}
}

func TestSecondJobConflicts(t *testing.T) {
	s, ts, fx := releaseServer(t)
	fx.Tag(t, firstCommit(t, fx), "v1.0.0")
	s.releaseRun = &releaseJob{Kind: "deploy", Subject: "dev", State: "running", Steps: []release.Step{}, Started: time.Now()}
	for _, w := range []struct {
		method, path string
		body         any
	}{
		{"POST", "/api/environments/dev/deploy", map[string]string{}},
		{"POST", "/api/releases", map[string]any{"deploy": true}},
		{"POST", "/api/releases/v1.0.0-rc.1/promote", map[string]string{}},
		{"POST", "/api/environments/prod/rollback", map[string]string{"tag": "v1.0.0", "reason": "bad"}},
		{"POST", "/api/releases/v1.0.0/yank", map[string]string{"reason": "bad"}},
		{"PUT", "/api/releases/v1.0.0/notes", map[string]string{"notes": "x"}},
		{"POST", "/api/releases/v1.0.0/comments", map[string]string{"message": "x"}},
		{"POST", "/api/releases/adopt", map[string]any{"proposals": []any{}}},
	} {
		if status, code, _ := coded(t, w.method, ts.URL+w.path, w.body); status != http.StatusConflict || code != "releasing" {
			t.Errorf("%s %s = %d %q, want 409 releasing", w.method, w.path, status, code)
		}
	}
}

func TestCutTakenIsCoded(t *testing.T) {
	_, ts, fx := releaseServer(t)
	c := firstCommit(t, fx)
	fx.Tag(t, c, "v1.0.0")
	runs(t, fx, "stage", c, "v1.0.0")
	runs(t, fx, "prod", c, "v1.0.0")
	m := fx.Merge(t, "fix/12-a", "")
	runs(t, fx, "dev", m, "1.0.1-dev.1.g"+m[:7])
	otherClone(t, fx, m, "v1.0.1") // meanwhile

	var job releaseJob
	if status := call(t, "POST", ts.URL+"/api/releases", map[string]any{"version": "1.0.1", "final": true}, &job); status != http.StatusAccepted || job.Kind != "cut" {
		t.Fatalf("POST cut = %d %+v", status, job)
	}
	job = waitRelease(t, ts)
	if job.State != "failed" || job.Code != "taken" || !strings.Contains(job.Error, "v1.0.1") {
		t.Errorf("job %+v, want failed, taken", job)
	}
}

func TestIssueEnvironments(t *testing.T) {
	s, ts, fx := releaseServer(t)
	c := firstCommit(t, fx)
	runs(t, fx, "stage", c, "v1.0.0")
	id := gitbugiotest.NewImportedIssue(t, s.b, "a", "https://github.com/org/app/issues/12")
	fx.Merge(t, "fix/12-a", "")
	later := fx.Commit(t, "later")
	runs(t, fx, "dev", later, "1.0.1-dev.2.g"+later[:7])

	var got []issueEnvironment
	getJSON(t, ts.URL+"/api/issues/"+id+"/environments", &got)
	want := []issueEnvironment{{Name: "dev", Version: "1.0.1-dev.2.g" + later[:7], In: true}, {Name: "stage", Version: "v1.0.0"}, {Name: "prod"}}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	other := gitbugiotest.NewImportedIssue(t, s.b, "b", "https://github.com/org/app/issues/13")
	getJSON(t, ts.URL+"/api/issues/"+other+"/environments", &got)
	if len(got) != 3 || slices.ContainsFunc(got, func(e issueEnvironment) bool { return e.In }) {
		t.Errorf("an issue no branch names: %+v", got)
	}
}

// R23: while a job holds the runner, a page's read doesn't wait for it: it
// reads offline, what was last fetched; once free, it fetches again.
func TestReleasesReadOfflineBesideAJob(t *testing.T) {
	s, ts, fx := releaseServer(t)
	otherClone(t, fx, firstCommit(t, fx), "v1.0.0")
	client := &http.Client{Timeout: 10 * time.Second}
	read := func() releasesResponse {
		t.Helper()
		resp, err := client.Get(ts.URL + "/api/releases")
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var got releasesResponse
		if err := json.NewDecoder(resp.Body).Decode(&got); err != nil || resp.StatusCode != http.StatusOK {
			t.Fatalf("%d: %v", resp.StatusCode, err)
		}
		return got
	}

	s.runnerMu.Lock() // a job runs
	jobEnds := sync.OnceFunc(s.runnerMu.Unlock)
	t.Cleanup(jobEnds) // before the server's close, which waits for a blocked read
	if got := read(); len(got.Environments) != 3 || len(got.Releases) != 0 {
		t.Errorf("beside a job: %d environments, releases %+v; want 3, none fetched", len(got.Environments), got.Releases)
	}
	jobEnds()
	if got := read(); len(got.Releases) != 1 {
		t.Errorf("after it: releases %+v, want v1.0.0 fetched", got.Releases)
	}
}

func TestReleasesNotesAndComments(t *testing.T) {
	s, ts, fx := releaseServer(t)
	c := firstCommit(t, fx)
	fx.Tag(t, c, "v1.0.0")
	runs(t, fx, "prod", c, "v1.0.0")
	if _, err := s.b.CreateRelease(gitbugio.ReleaseRecord{Commit: c}); err != nil {
		t.Fatal(err)
	}
	if status := call(t, "PUT", ts.URL+"/api/releases/v1.0.0/notes", map[string]string{"notes": "the first"}, nil); status != http.StatusOK {
		t.Fatalf("PUT notes = %d", status)
	}
	if status := call(t, "POST", ts.URL+"/api/releases/v1.0.0/comments", map[string]string{"message": "fine in prod"}, nil); status != http.StatusOK {
		t.Fatalf("POST comment = %d", status)
	}

	var d releaseDetail
	getJSON(t, ts.URL+"/api/releases/v1.0.0", &d)
	if rec := d.Row.Record; rec == nil || rec.Notes != "the first" || len(rec.Comments) != 1 || rec.Comments[0].Message != "fine in prod" {
		t.Errorf("record %+v", d.Row.Record)
	}
	if h := d.History["prod"]; len(h) != 2 || h[0].Version != "v1.0.0" || d.Issues == nil || d.Candidates == nil {
		t.Errorf("detail %+v", d)
	} else if h[0].Target != "config (ghcr.test)" || h[1].Target != "config (quay.test)" {
		t.Errorf("history's targets %q, %q; want each target's name", h[0].Target, h[1].Target)
	}
	if status, _, msg := coded(t, "GET", ts.URL+"/api/releases/v9.9.9", nil); status != http.StatusNotFound || msg != "no release tag v9.9.9" {
		t.Errorf("unknown tag: %d %q", status, msg)
	}
}

// panicky is the fake tools, but a registry lookup panics.
type panicky struct{ release.Tools }

func (panicky) Revision(context.Context, string) (string, string, error) { panic("registry exploded") }

// A read or a quick write that panics leaves the runner free: the next job
// or write isn't wedged behind it.
func TestReleasesRunnerFreedAfterAPanic(t *testing.T) {
	s, _, _ := releaseServer(t)
	s.releaseTools = panicky{s.releaseTools}
	ts := httptest.NewUnstartedServer(newMux(s, http.NotFoundHandler()))
	ts.Config.ErrorLog = log.New(io.Discard, "", 0) // net/http recovers the panic and logs its stack
	ts.Start()
	defer ts.Close()

	if resp, err := http.Get(ts.URL + "/api/releases/draft"); err == nil { // the draft's image check panics
		_ = resp.Body.Close()
	}
	if !s.runnerMu.TryLock() {
		t.Fatal("a read that panicked left the runner held")
	}
	s.runnerMu.Unlock()

	func() {
		defer func() { _ = recover() }()
		s.quickWrite(httptest.NewRecorder(), func(context.Context) error { panic("engine exploded") })
	}()
	if !s.runnerMu.TryLock() {
		t.Fatal("a quick write that panicked left the runner held")
	}
	s.runnerMu.Unlock()
}

// R25: an issue's environments are read as the last fetch left them; the
// Releases page's read is the one that fetches.
func TestIssueEnvironmentsReadOffline(t *testing.T) {
	s, ts, fx := releaseServer(t)
	c := firstCommit(t, fx)
	runs(t, fx, "dev", c, "1.0.0-dev.0.g"+c[:7])
	id := gitbugiotest.NewImportedIssue(t, s.b, "a", "https://github.com/org/app/issues/12")
	m := fx.Merge(t, "fix/12-a", "")
	getJSON(t, ts.URL+"/api/releases", nil) // fetched: dev runs c

	runs(t, fx, "dev", m, "1.0.1-dev.1.g"+m[:7]) // pushed to the config repos
	var got []issueEnvironment
	getJSON(t, ts.URL+"/api/issues/"+id+"/environments", &got)
	if len(got) != 3 || got[0].In || got[0].Version != "1.0.0-dev.0.g"+c[:7] {
		t.Errorf("before a fetch: %+v, want dev as last fetched, without it", got)
	}
	getJSON(t, ts.URL+"/api/releases", nil)
	getJSON(t, ts.URL+"/api/issues/"+id+"/environments", &got)
	if len(got) != 3 || !got[0].In {
		t.Errorf("after a fetch: %+v, want dev with it", got)
	}
}

// R26: a record written outside the server (a CLI cut, a pull) reaches the
// Releases page; an issue edited outside it, only the issues' pages.
func TestReleasesFromOutsideArePublished(t *testing.T) {
	_, s := newIssueServer(t)
	events, cancel := s.events.subscribe()
	defer cancel()
	stop := make(chan struct{})
	defer close(stop)
	go s.watchRefs(10*time.Millisecond, stop)
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) { // the watcher has seen the refs
		s.refsMu.Lock()
		seen := s.refs != ""
		s.refsMu.Unlock()
		if seen {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the watcher didn't start")
		}
	}

	// next is the kind of the next event, "" when none comes within wait.
	next := func(wait time.Duration) string {
		select {
		case data := <-events:
			var e event
			_ = json.Unmarshal(data, &e)
			return e.Kind
		case <-time.After(wait):
			return ""
		}
	}

	gitbugiotest.NewIssue(t, s.b, "edited elsewhere")
	if kind := next(5 * time.Second); kind != "issues" {
		t.Fatalf("an issue edit: %q, want issues", kind)
	}
	if kind := next(200 * time.Millisecond); kind != "" { // a tick publishes all its events at once
		t.Errorf("an issue edit also published %q", kind)
	}

	if _, err := s.b.CreateRelease(gitbugio.ReleaseRecord{Commit: "0123456789abcdef0123456789abcdef01234567"}); err != nil {
		t.Fatal(err)
	}
	for timeout := time.After(5 * time.Second); ; {
		select {
		case data := <-events:
			var e event
			if json.Unmarshal(data, &e) == nil && e.Kind == "releases" {
				return
			}
		case <-timeout:
			t.Fatal("no releases event for a record written outside the server")
		}
	}
}

// What a request gets wrong is refused before anything runs: 400 for its
// body, 404 for a tag or record that isn't there.
func TestReleasesRequestChecks(t *testing.T) {
	s, ts, fx := releaseServer(t)
	c := firstCommit(t, fx)
	fx.Tag(t, c, "v1.0.0", "v1.0.1")
	runs(t, fx, "prod", c, "v1.0.1")
	if _, err := s.b.CreateRelease(gitbugio.ReleaseRecord{Commit: c}); err != nil { // v1.0.0's and v1.0.1's
		t.Fatal(err)
	}
	fx.Tag(t, fx.Commit(t, "later"), "v1.0.2") // no record
	for _, w := range []struct {
		method, path string
		body         any
		status       int
		msg          string
	}{
		{"POST", "/api/releases/v1.0.0/comments", map[string]string{"message": ""}, 400, "message is required"},
		{"POST", "/api/environments/prod/rollback", map[string]string{"tag": "latest", "reason": "bad"}, 400, `"latest" isn't a release tag`},
		{"POST", "/api/environments/prod/rollback", map[string]string{"tag": "v1.0.0"}, 400, "a rollback takes a tag and a reason"},
		{"POST", "/api/environments/prod/rollback", map[string]string{"tag": "v9.9.9", "reason": "bad"}, 404, `no release tag "v9.9.9"`},
		{"POST", "/api/environments/stage/deploy", map[string]string{"tag": "v9.9.9"}, 404, `no release tag "v9.9.9"`},
		{"POST", "/api/releases/v9.9.9/yank", map[string]string{"reason": "bad"}, 404, "no release tag v9.9.9"},
		{"POST", "/api/releases/v1.0.2/yank", map[string]string{"reason": "bad"}, 404, "v1.0.2 has no release record"},
	} {
		if status, _, msg := coded(t, w.method, ts.URL+w.path, w.body); status != w.status || msg != w.msg {
			t.Errorf("%s %s %v = %d %q, want %d %q", w.method, w.path, w.body, status, msg, w.status, w.msg)
		}
	}
	if job := s.jobSnapshot(); job != nil {
		t.Errorf("a refused request started %+v", job)
	}
	if status := call(t, "PUT", ts.URL+"/api/releases/v1.0.0/notes", map[string]string{"notes": ""}, nil); status != http.StatusOK {
		t.Errorf("clearing the notes = %d", status)
	}
}
