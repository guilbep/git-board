package server

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghbridge"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// wholeServer serves mine, a clone of origin beside theirs, with the forge
// gh, whose runs forges answers, and a GitHub Project that reads empty;
// projectRead counts its reads.
func wholeServer(t *testing.T, forges func(ctx context.Context, mode string, fs []config.Forge, dryRun bool, see func(store.SyncEvent)) error) (ts *httptest.Server, s *apiServer, theirs *store.Backend, projectRead *atomic.Int32) {
	t.Helper()
	mine, theirs := storetest.NewPair(t)
	cfg := &config.Config{Forges: []config.Forge{ghForge}, GithubProject: config.GithubProject{Owner: "o", Number: 1}}
	projectRead = new(atomic.Int32)
	project := ghbridge.Project{
		Items: func(string, int, int) ([]ghbridge.Item, error) { projectRead.Add(1); return nil, nil },
		Meta:  func(string, int) (ghbridge.ProjectMeta, error) { return ghbridge.ProjectMeta{ID: "P"}, nil },
	}
	s = &apiServer{cfg: cfg, b: mine, me: "guilbep", cfgPath: filepath.Join(t.TempDir(), config.FileName), project: project, runSync: forges}
	ts = httptest.NewServer(withCORS(newMux(s, http.NotFoundHandler())))
	t.Cleanup(ts.Close)
	return ts, s, theirs, projectRead
}

// waitWhole waits for the whole sync's job to end.
func waitWhole(t *testing.T, ts *httptest.Server) wholeStatus {
	t.Helper()
	waitJob(t, ts)
	var st wholeStatus
	if code := call(t, "GET", ts.URL+"/api/sync/all", nil, &st); code != http.StatusOK || st.Job == nil {
		t.Fatalf("GET: %d %+v", code, st)
	}
	return st
}

func stepWords(job *forgeJob) string {
	var w []string
	for _, st := range job.Steps {
		w = append(w, st.Name+" "+st.State)
	}
	return strings.Join(w, ", ")
}

// The whole sync: a check (a fetch and compare, the forges' dry run, the
// Project's check) writes nothing and opens the way to the run, which
// pulls, syncs the forges and the Project, then pushes. A forge check
// alone doesn't open it.
func TestWholeSync(t *testing.T) {
	t.Parallel()
	var runs atomic.Int32
	ts, s, theirs, projectRead := wholeServer(t, func(_ context.Context, mode string, fs []config.Forge, dry bool, see func(store.SyncEvent)) error {
		if runs.Add(1); mode != "sync" {
			t.Errorf("a forge run as %s, want both ways", mode)
		}
		see(store.SyncEvent{Forge: "gh", Way: "push", What: "posted", Issue: "0123456789"})
		return nil
	})
	storetest.NewIssue(t, s.b, "Export CSV")
	before := s.b.RefsFingerprint()

	if code := call(t, "POST", ts.URL+"/api/sync/all", map[string]any{"dry_run": false}, nil); code != http.StatusConflict {
		t.Errorf("a run before a check: %d, want 409", code)
	}
	call(t, "POST", ts.URL+"/api/forges/sync", map[string]any{"dry_run": true}, nil)
	waitJob(t, ts)
	if code := call(t, "POST", ts.URL+"/api/sync/all", map[string]any{"dry_run": false}, nil); code != http.StatusConflict {
		t.Errorf("a run after a check of the forges alone: %d, want 409", code)
	}

	if code := call(t, "POST", ts.URL+"/api/sync/all", map[string]any{"dry_run": true}, nil); code != http.StatusAccepted {
		t.Fatalf("check: %d", code)
	}
	st := waitWhole(t, ts)
	if got := stepWords(st.Job); st.Job.State != "done" || got != "pull done, forges done, project done, push skipped" {
		t.Fatalf("check %s: %s %s", st.Job.State, got, st.Job.Error)
	}
	if st.Job.Bridges["gh"].To.Issues != 1 || st.Remote == nil || st.Remote.Checked == nil || st.Remote.Issues.Send != 1 || st.Project.Last == nil || projectRead.Load() != 1 {
		t.Errorf("check %+v, remote %+v, project %+v (%d reads)", st.Job, st.Remote, st.Project, projectRead.Load())
	}
	if s.b.RefsFingerprint() != before {
		t.Error("the check wrote here")
	}

	if code := call(t, "POST", ts.URL+"/api/sync/all", map[string]any{"dry_run": false}, nil); code != http.StatusAccepted {
		t.Fatalf("run after a check: %d", code)
	}
	st = waitWhole(t, ts)
	if got := stepWords(st.Job); st.Job.State != "done" || got != "pull done, forges done, project done, push done" {
		t.Fatalf("run %s: %s %s", st.Job.State, got, st.Job.Error)
	}
	if runs.Load() != 3 || projectRead.Load() != 3 { // the run checks the Project after its sync
		t.Errorf("%d forge runs, Project read %d times", runs.Load(), projectRead.Load())
	}
	if err := theirs.Pull("origin"); err != nil {
		t.Fatal(err)
	}
	if is, _ := theirs.ListIssues(); len(is) != 1 {
		t.Errorf("origin got %d issues, want 1", len(is))
	}
	if code := call(t, "POST", ts.URL+"/api/sync/all", map[string]any{"dry_run": false}, nil); code != http.StatusConflict {
		t.Errorf("a second run without a new check: %d, want 409", code)
	}
}

// A step that fails stops the steps after it, and says which: a forge
// sync that fails leaves the Project alone and pushes nothing.
func TestWholeSyncStopsAtAFailure(t *testing.T) {
	t.Parallel()
	var fail atomic.Bool
	ts, s, theirs, projectRead := wholeServer(t, func(context.Context, string, []config.Forge, bool, func(store.SyncEvent)) error {
		if fail.Load() {
			return errors.New("GitHub is down")
		}
		return nil
	})
	storetest.NewIssue(t, s.b, "Export CSV")
	call(t, "POST", ts.URL+"/api/sync/all", map[string]any{"dry_run": true}, nil)
	waitWhole(t, ts)
	fail.Store(true)
	call(t, "POST", ts.URL+"/api/sync/all", map[string]any{"dry_run": false}, nil)
	st := waitWhole(t, ts)
	if got := stepWords(st.Job); st.Job.State != "failed" || got != "pull done, forges failed, project skipped, push skipped" || st.Job.Error != "forges: GitHub is down" {
		t.Fatalf("run %s: %s, %q", st.Job.State, got, st.Job.Error)
	}
	if projectRead.Load() != 1 {
		t.Errorf("the Project was read %d times, want the check's alone", projectRead.Load())
	}
	if err := theirs.Pull("origin"); err != nil {
		t.Fatal(err)
	}
	if is, _ := theirs.ListIssues(); len(is) != 0 {
		t.Errorf("origin got %d issues after the failure", len(is))
	}
	// a check of the last 15 minutes, no older
	s.forgeMu.Lock()
	s.job = &forgeJob{DryRun: true, Mode: "all", State: "done", Ended: new(time.Now().Add(-dryRunFresh - time.Minute))}
	s.forgeMu.Unlock()
	if code := call(t, "POST", ts.URL+"/api/sync/all", map[string]any{"dry_run": false}, nil); code != http.StatusConflict {
		t.Errorf("a run after a stale check: %d, want 409", code)
	}
}
