package server

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// One issue syncs on its own: a dry run says what would go where, a real
// run needs that dry run first, and a full sync running refuses it.
func TestIssueSync(t *testing.T) {
	t.Parallel()
	ts, s := forgeServer(t, ghForge, glForge)
	id := storetest.NewIssue(t, s.b, "one issue")
	var got []string
	s.runIssueSync = func(_ context.Context, forges []config.Forge, dryRun bool, issue string, see func(store.SyncEvent)) error {
		got = append(got, issue)
		if len(forges) != 2 {
			t.Errorf("forges %+v, want both", forges)
		}
		see(store.SyncEvent{Forge: "gh", Way: "push", What: "comment", Issue: "1000"})
		return nil
	}
	var res issueSyncResult

	if code := call(t, "POST", ts.URL+"/api/issues/"+id+"/sync", map[string]bool{"dry_run": false}, nil); code != http.StatusConflict {
		t.Errorf("a real run before its dry run: %d, want 409", code)
	}
	before := time.Now()
	if code := call(t, "POST", ts.URL+"/api/issues/"+id+"/sync", map[string]bool{"dry_run": true}, &res); code != http.StatusOK {
		t.Fatalf("dry run: %d", code)
	}
	if !res.DryRun || res.Writes != 1 || res.Bridges["gh"].To.Comments != 1 {
		t.Errorf("dry run %+v", res)
	}
	// when it came back, by the server's clock: the page compares it with job.ended
	if res.Ended.Before(before) || res.Ended.After(time.Now()) {
		t.Errorf("ended %v, want between %v and now", res.Ended, before)
	}
	if _, listed := res.Bridges["gl"]; !listed {
		t.Errorf("every sync bridge is listed, up to date or not: %v", res.Bridges)
	}
	if code := call(t, "POST", ts.URL+"/api/issues/"+id+"/sync", map[string]bool{"dry_run": false}, &res); code != http.StatusOK {
		t.Fatalf("real run: %d", code)
	}
	if res.DryRun || res.Bridges["gh"].To.Comments != 1 || len(got) != 2 || got[1] != id {
		t.Errorf("real run %+v, synced %v", res, got)
	}

	s.syncing.Store(true)
	if code := call(t, "POST", ts.URL+"/api/issues/"+id+"/sync", map[string]bool{"dry_run": true}, nil); code != http.StatusConflict {
		t.Errorf("during a full sync: %d, want 409", code)
	}
}
