package server

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/git-bug/git-bug/bridge/core"
	"github.com/git-bug/git-bug/bridge/core/auth"

	"github.com/guilbep/git-board/internal/config"
)

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
	st := waitJob(t, ts)
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
	st := waitJob(t, ts)
	tally := st.Job.Bridges["gh"]
	if st.Job.State != "done" || st.Job.Errors != 0 || tally == nil || tally.Warnings != 1 || tally.Assignable != 0 {
		t.Fatalf("job %+v, gh %+v: want done, one warning, no error", st.Job, tally)
	}
}
