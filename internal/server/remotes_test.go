package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

func serverOf(t *testing.T, b *store.Backend) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(withCORS(newMux(&apiServer{cfg: &config.Config{}, b: b}, http.NotFoundHandler())))
	t.Cleanup(ts.Close)
	return ts
}

// origin is the remote of a remotes answer.
func origin(t *testing.T, ts *httptest.Server, method, path string) remoteStatus {
	t.Helper()
	var st remotesStatus
	if code := call(t, method, ts.URL+path, nil, &st); code != http.StatusOK || len(st.Remotes) != 1 || st.Remotes[0].Name != "origin" {
		t.Fatalf("%s %s: %d %+v", method, path, code, st)
	}
	return st.Remotes[0]
}

// Two clones of one bare remote: issues and people go through it with
// Push and Pull, and each says what is left to move, after a fetch.
func TestRemotesPushAndPull(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	me, them := serverOf(t, mine), serverOf(t, theirs)

	if o := origin(t, me, "GET", "/api/remotes"); o.Tracked || o.Checked != nil {
		t.Errorf("never fetched: %+v, want not checked", o)
	}
	id := storetest.NewIssue(t, mine, "Export CSV")
	if o := origin(t, me, "POST", "/api/remotes/origin/fetch"); o.Checked == nil || o.Issues != (store.Ahead{Send: 1}) || o.People != (store.Ahead{Send: 1}) {
		t.Errorf("after a fetch of an empty remote: %+v, want 1 issue and 1 person to send", o)
	}
	if _, err := mine.Push("origin"); err != nil {
		t.Fatal(err)
	}
	if o := origin(t, me, "GET", "/api/remotes"); o.Issues != (store.Ahead{}) || o.People != (store.Ahead{}) {
		t.Errorf("after the push: %+v, want nothing left", o)
	}

	if o := origin(t, them, "POST", "/api/remotes/origin/fetch"); o.Issues != (store.Ahead{Receive: 1}) || o.People != (store.Ahead{Send: 1, Receive: 1}) {
		t.Errorf("the colleague's fetch: %+v, want 1 issue to receive, their own person to send", o)
	}
	if o := origin(t, them, "POST", "/api/remotes/origin/pull"); o.Issues != (store.Ahead{}) {
		t.Errorf("after the pull: %+v", o)
	}
	if _, err := theirs.GetIssue(id); err != nil {
		t.Fatalf("the colleague's pull didn't bring the issue: %v", err)
	}

	// both change it: a fetch shows both ways, a pull merges, a push sends the merge
	if _, err := theirs.ApplyLabelChange(id, []string{"prio:p1"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := theirs.Push("origin"); err != nil {
		t.Fatal(err)
	}
	if _, err := mine.ApplyLabelChange(id, []string{"status:ready"}, nil); err != nil {
		t.Fatal(err)
	}
	if o := origin(t, me, "POST", "/api/remotes/origin/fetch"); o.Issues != (store.Ahead{Send: 1, Receive: 1}) {
		t.Errorf("both changed it: %+v, want 1 to send and 1 to receive", o)
	}
	if code := call(t, "POST", me.URL+"/api/remotes/origin/push", nil, nil); code != http.StatusNotFound {
		t.Errorf("a push, the whole sync's alone: %d, want 404", code)
	}
	if o := origin(t, me, "POST", "/api/remotes/origin/pull"); o.Issues != (store.Ahead{Send: 1}) {
		t.Errorf("after merging: %+v, want the merge to send", o)
	}
	if labels, _ := mine.GetLabels(id); strings.Join(labels, " ") != "prio:p1 status:ready" {
		t.Errorf("merged labels %v", labels)
	}

	if code := call(t, "POST", me.URL+"/api/remotes/nope/fetch", nil, nil); code != http.StatusNotFound {
		t.Errorf("an unknown remote: %d, want 404", code)
	}
}

// A remote's URL may carry a token: the page never gets it, nor in
// git's errors, which quote the URL.
func TestRemoteURLsLoseTheirCredentials(t *testing.T) {
	t.Parallel()
	// pairs, not a map: gofmt 1.26 and 1.27 align a map's long keys differently
	for _, c := range [][2]string{
		{"https://oauth2:glpat-secret@gitlab.example.com/g/r.git", "https://gitlab.example.com/g/r.git"},
		{"https://ghp_secret@github.com/o/r.git: non-fast-forward update", "https://github.com/o/r.git: non-fast-forward update"},
		{"git@github.com:o/r.git", "git@github.com:o/r.git"},
		{"/srv/git/r.git", "/srv/git/r.git"},
	} {
		in, want := c[0], c[1]
		if got := store.NoCredentials(in); got != want {
			t.Errorf("NoCredentials(%q) = %q, want %q", in, got, want)
		}
	}
}

// A push goes to every pushurl of a remote, as git's does: the page names
// them all, before pushing.
func TestRemotePushURLs(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	for _, kv := range [][2]string{
		{"remote.mirror.url", "https://github.com/o/r.git"},
		{"remote.mirror.pushurl", "https://github.com/o/r.git"},
		{"remote.mirror.pushurl", "https://oauth2:glpat-secret@gitlab.example.com/g/r.git"},
		{"remote.plain.url", "/srv/git/r.git"},
	} {
		storetest.Git(t, b.GitDir(), "config", "--add", kv[0], kv[1])
	}
	rs, err := b.Remotes()
	if err != nil || len(rs) != 2 {
		t.Fatalf("Remotes: %v %+v", err, rs)
	}
	if got := strings.Join(rs[0].PushURLs, " "); got != "https://github.com/o/r.git https://gitlab.example.com/g/r.git" {
		t.Errorf("mirror pushes to %s", got)
	}
	if got := strings.Join(rs[1].PushURLs, " "); got != "/srv/git/r.git" {
		t.Errorf("plain pushes to %s, want its url", got)
	}
	if code := send(t, "POST", serverOf(t, b).URL+"/api/remotes/plain/pull", "https://evil.example"); code != http.StatusForbidden {
		t.Errorf("a pull asked by another site: %d, want 403", code)
	}
}

// send is a write with an Origin, as a browser sends.
func send(t *testing.T, method, url, origin string) int {
	t.Helper()
	req, _ := http.NewRequest(method, url, strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", origin)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	return resp.StatusCode
}

func TestRemoteEvents(t *testing.T) {
	t.Parallel()
	for path, want := range map[string]string{
		"/api/remotes/origin/pull":  "issues", // the boards reload
		"/api/remotes/origin/fetch": "sync",
	} {
		if got := eventFor(httptest.NewRequest("POST", path, nil)).Kind; got != want {
			t.Errorf("%s: %s, want %s", path, got, want)
		}
	}
}
