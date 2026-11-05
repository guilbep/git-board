package httpapi

import (
	"path/filepath"
	"testing"
	"time"
)

// The git remotes at once, a whole sync in the background, its steps as
// progress events, one run at a time; the project's numbers.
func TestSyncAndRemotes(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	origin := filepath.Join(t.TempDir(), "origin.git")
	gitRun(t, "", "init", "-q", "--bare", origin)
	gitRun(t, dir, "remote", "add", "origin", origin)
	a := newAPI(t, dir)

	st := want[syncRes](t, a.do("GET", "/api/sync", nil), 200)
	if len(st.Remotes) != 1 || st.Remotes[0] != "origin" || st.Run != nil {
		t.Fatalf("GET /api/sync = %+v", st)
	}
	r := want[remoteRes](t, a.do("POST", "/api/remotes/origin/push", nil), 200)
	if r.Action != "push" {
		t.Fatalf("push = %+v", r)
	}
	r = want[remoteRes](t, a.do("POST", "/api/remotes/origin/fetch", nil), 200)
	if r.Issues == nil || r.Issues.Receive != 0 || r.Issues.Send != 0 {
		t.Fatalf("fetch after a push = %+v", r)
	}
	if e := want[apiError](t, a.do("POST", "/api/remotes/nope/pull", nil), 500); e.Message == "" {
		t.Fatalf("a remote that isn't = %+v", e)
	}

	ch := stream(t, a)
	run := want[Run](t, a.do("POST", "/api/sync", map[string]any{"scope": "all"}), 202)
	if run.State != "running" || run.ID == "" {
		t.Fatalf("POST /api/sync = %+v", run)
	}
	if e := next(t, ch, "progress"); e.ID != run.ID || e.Step == nil {
		t.Fatalf("a progress event = %+v", e)
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		st = want[syncRes](t, a.do("GET", "/api/sync", nil), 200)
		if st.Run != nil && st.Run.State != "running" || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if st.Run == nil || st.Run.State != "done" || len(st.Run.Steps) == 0 {
		t.Fatalf("the run = %+v", st.Run)
	}
	if e := want[apiError](t, a.do("POST", "/api/sync", map[string]any{"scope": "everything"}), 400); e.Field != "scope" {
		t.Fatalf("a scope that isn't = %+v", e)
	}
	if e := want[apiError](t, a.do("POST", "/api/sync", map[string]any{"scope": "forges"}), 202); e.Code != "" {
		t.Fatalf("a forge run = %+v", e)
	}

	n := want[numbersRes](t, a.do("POST", "/api/numbers", map[string]any{"prefix": "T", "start": 1}), 200)
	if n.Where != "on origin" {
		t.Fatalf("numbers init = %+v", n)
	}
	n = want[numbersRes](t, a.do("POST", "/api/numbers", map[string]any{}), 200)
	if n.Numbered != 2 {
		t.Fatalf("numbering = %+v, want the 2 issues", n)
	}
	page := want[issuePage](t, a.do("GET", "/api/issues?filter=T-1", nil), 200)
	if page.Total != 1 {
		t.Fatalf("T-1 = %+v", page)
	}
}

// One run at a time: a second while one runs is busy.
func TestSyncBusy(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	a.s.runMu.Lock()
	a.s.run = &Run{ID: "1", State: "running"}
	a.s.runMu.Unlock()
	if e := want[apiError](t, a.do("POST", "/api/sync", map[string]any{}), 409); e.Code != "busy" {
		t.Fatalf("a second run = %+v", e)
	}
}
