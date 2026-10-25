package server

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"
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
	t.Skip("M5 phase 2: bridges and tokens")
}

// A forge that refuses the list is a warning, not an error: a check with
// one still allows the run.
func TestForgeSyncWarnsWhenTheForgeWontSay(t *testing.T) {
	t.Skip("M5 phase 2: bridges and tokens")
}

// Who can be assigned is asked once the run is done: a forge that hangs
// keeps neither the run running nor the edits waiting.
func TestForgeSyncDoesntWaitForWhoCanBeAssigned(t *testing.T) {
	t.Skip("M5 phase 2: bridges and tokens")
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
