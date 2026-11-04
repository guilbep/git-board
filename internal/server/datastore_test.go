package server

import (
	"net/http"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/pgstore/pgtest"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// Settings › Datastore: test a database, copy the board into it, name it;
// the server says it needs a restart, and never shows the password.
func TestDatastoreSettings(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	storetest.NewIssue(t, b, "kept in git")
	ts := newTestServer(t, b, nil)

	var got datastoreResponse
	getJSON(t, ts.URL+"/api/datastore", &got)
	if got.Setting != "git" || got.Datastore != "git" || !got.Replica || got.Restart {
		t.Fatalf("datastore %+v", got)
	}
	var res store.TestResult
	if code := call(t, "POST", ts.URL+"/api/datastore/test", datastoreRequest{URL: "mysql://x"}, &res); code != http.StatusOK || res.OK {
		t.Fatalf("test of a bad URL: %d %+v", code, res)
	}
	if code := call(t, "PUT", ts.URL+"/api/datastore", datastoreRequest{URL: "mysql://x"}, nil); code != http.StatusBadRequest {
		t.Fatalf("a bad URL saved: %d", code)
	}

	url := pgtest.URL(t)
	withPassword := strings.Replace(url, "postgres@", "postgres:secret@", 1) // trust auth: any password goes
	if code := call(t, "POST", ts.URL+"/api/datastore/test", datastoreRequest{URL: withPassword}, &res); code != http.StatusOK || !res.OK || res.Probe.Tables {
		t.Fatalf("test: %d %+v", code, res)
	}
	if code := call(t, "POST", ts.URL+"/api/datastore/copy", datastoreRequest{URL: withPassword}, &res); code != http.StatusOK || res.Copied != 2 || res.Probe.Entities != 2 {
		t.Fatalf("copy: %d %+v; want the issue and its author", code, res)
	}
	if code := call(t, "PUT", ts.URL+"/api/datastore", datastoreRequest{URL: withPassword}, &got); code != http.StatusOK || !got.Restart || strings.Contains(got.Setting, "secret") {
		t.Fatalf("saved: %d %+v; want a restart, no password", code, got)
	}
	// the page sends back what it was shown: the password stays
	if code := call(t, "PUT", ts.URL+"/api/datastore", datastoreRequest{URL: got.Setting}, nil); code != http.StatusOK {
		t.Fatalf("saved again: %d", code)
	}
	if v := strings.TrimSpace(storetest.Git(t, b.GitDir(), "config", store.DatastoreKey)); v != withPassword {
		t.Fatalf("board.datastore = %q, want %q", v, withPassword)
	}
	if code := call(t, "PUT", ts.URL+"/api/datastore", datastoreRequest{URL: "git"}, &got); code != http.StatusOK || got.Restart {
		t.Fatalf("back to git: %d %+v", code, got)
	}
}
