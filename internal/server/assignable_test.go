package server

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/git-bug/git-bug/bridge/core"
	"github.com/git-bug/git-bug/bridge/core/auth"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// waitAssignable waits for the run to end and its fetch of who can be
// assigned to answer for bridge, in the bridge's lines.
func waitAssignable(t *testing.T, ts *httptest.Server, bridge string) forgeStatus {
	t.Helper()
	for range 300 {
		st := waitJob(t, ts)
		if tally := st.Job.Bridges[bridge]; tally != nil && slices.ContainsFunc(tally.Lines, func(l string) bool {
			return strings.Contains(l, "assignable people on ") || strings.HasPrefix(l, "who can be assigned: ")
		}) {
			return st
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("%s never said who can be assigned", bridge)
	return forgeStatus{}
}

// A forge sync asks each forge who can be assigned, says how many, and
// the person fields offer them apart from their options.
func TestForgeSyncFetchesWhoCanBeAssigned(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	s.cfg = &config.Config{Fields: map[string]config.Field{"assignee": {}, "status": {Single: true, Options: []string{"ready"}}}}
	gl := fakeGitLab(t)
	if code := call(t, "POST", ts.URL+"/api/bridges", map[string]string{"target": "gitlab", "url": gl.URL + "/scratchpad/sync-trial", "token": "good-token"}, nil); code != http.StatusOK {
		t.Fatalf("add a bridge: %d", code)
	}
	done := make(chan struct{})
	close(done)
	s.runSync = fakeSync(nil, done)

	if code := call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": true}, nil); code != http.StatusAccepted {
		t.Fatalf("dry run: %d", code)
	}
	st := waitAssignable(t, ts, "gl")
	tally := st.Job.Bridges["gl"]
	if st.Job.State != "done" || tally == nil || tally.Assignable != 2 || tally.Warnings != 0 {
		t.Fatalf("job %+v, gl %+v: want done, 2 assignable", st.Job, tally)
	}
	if !slices.ContainsFunc(tally.Lines, func(l string) bool { return strings.HasPrefix(l, "2 assignable people on ") }) {
		t.Errorf("lines %q: want the count", tally.Lines)
	}

	var fields map[string]fieldResponse
	getJSON(t, ts.URL+"/api/config/fields", &fields)
	a := fields["assignee"]
	if !slices.Contains(a.People, "alice") || !slices.Contains(a.People, "bob") || len(a.Options) != 0 {
		t.Errorf("assignee %+v: want alice and bob as people, no options", a)
	}
	if !slices.Contains(a.People, "guilbert") { // the identity the bridge's token tagged
		t.Errorf("people %q: want the git-bug identities too", a.People)
	}
	if !slices.IsSortedFunc(a.People, func(x, y string) int { return strings.Compare(strings.ToLower(x), strings.ToLower(y)) }) {
		t.Errorf("people %q: want sorted", a.People)
	}
	if len(fields["status"].People) != 0 {
		t.Errorf("status %+v: people only on person fields", fields["status"])
	}
	if saved, err := loadAssignable(s.b.Storage()); err != nil || len(saved["gl"]) != 2 {
		t.Errorf("kept %v (%v): want gl's two", saved, err)
	}
}

// A forge that refuses the list is a warning, not an error: a check with
// one still allows the run.
func TestForgeSyncWarnsWhenTheForgeWontSay(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	githubForge(t, s.b)
	for k, v := range map[string]string{core.ConfigKeySync: "true", "default-login": "guilbep"} {
		if err := core.SetBridgeConfig(s.b.TestCache(), "gh", k, v); err != nil {
			t.Fatal(err)
		}
	}
	tok := auth.NewToken("github", "a-token")
	tok.SetMetadata(auth.MetaKeyLogin, "guilbep")
	if err := auth.Store(s.b.TestCache(), tok); err != nil {
		t.Fatal(err)
	}
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/assignees" || r.Header.Get("Authorization") != "Bearer a-token" {
			t.Errorf("asked %s with %q", r.URL.Path, r.Header.Get("Authorization"))
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(gh.Close)
	s.githubAPI = gh.URL
	done := make(chan struct{})
	close(done)
	s.runSync = fakeSync(nil, done)

	call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": true}, nil)
	st := waitAssignable(t, ts, "gh")
	tally := st.Job.Bridges["gh"]
	if st.Job.State != "done" || st.Job.Errors != 0 || tally == nil || tally.Warnings != 1 || tally.Assignable != 0 {
		t.Fatalf("job %+v, gh %+v: want done, one warning, no error", st.Job, tally)
	}
}

// Who can be assigned is asked once the run is done: a forge that hangs
// keeps neither the run running nor the edits waiting.
func TestForgeSyncDoesntWaitForWhoCanBeAssigned(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	id := gitbugiotest.NewIssue(t, s.b, "Edit me")
	githubForge(t, s.b)
	for k, v := range map[string]string{core.ConfigKeySync: "true", "default-login": "guilbep"} {
		if err := core.SetBridgeConfig(s.b.TestCache(), "gh", k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := auth.Store(s.b.TestCache(), auth.NewToken("github", "a-token")); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	gh := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-release
		_, _ = w.Write([]byte(`[{"login":"carol"}]`))
	}))
	free := sync.OnceFunc(func() { close(release) })
	t.Cleanup(gh.Close)
	t.Cleanup(free) // before the close, which waits for the handler
	s.githubAPI = gh.URL
	done := make(chan struct{})
	close(done)
	s.runSync = fakeSync(nil, done)

	call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": true}, nil)
	if st := waitJob(t, ts); st.Job.State != "done" {
		t.Fatalf("job %+v: want done while the forge hangs", st.Job)
	}
	if code := call(t, "POST", ts.URL+"/api/issues/"+id+"/labels", map[string]any{"add": []string{"status:ready"}}, nil); code != http.StatusOK {
		t.Errorf("an edit while the forge hangs: %d, want 200", code)
	}
	free()
	if tally := waitAssignable(t, ts, "gh").Job.Bridges["gh"]; tally.Assignable != 1 {
		t.Errorf("gh %+v: want carol, once the forge answers", tally)
	}
}

// Asking who can be assigned follows no redirect: GitLab's PRIVATE-TOKEN,
// unlike Authorization, would go along to wherever it points.
func TestAssignableFollowsNoRedirect(t *testing.T) {
	t.Parallel()
	elsewhere := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("the redirect was followed, with token %q", r.Header.Get("PRIVATE-TOKEN"))
	}))
	t.Cleanup(elsewhere.Close)
	forge := httptest.NewServer(http.RedirectHandler(elsewhere.URL+"/users", http.StatusFound))
	t.Cleanup(forge.Close)
	req, _ := http.NewRequest(http.MethodGet, forge.URL+"/api/v4/projects/1/users", nil)
	req.Header.Set("PRIVATE-TOKEN", "a-token")
	resp, err := assignableHTTP.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Errorf("answered %s, want the redirect itself", resp.Status)
	}
}
