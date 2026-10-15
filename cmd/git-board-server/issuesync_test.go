package main

import (
	"context"
	"net/http"
	"testing"

	"github.com/git-bug/git-bug/bridge/core"

	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// One issue syncs on its own: a dry run says what would go where, a real
// run needs that dry run first, and a full sync running refuses it.
func TestIssueSync(t *testing.T) {
	ts, s := newIssueServer(t)
	id := gitbugiotest.NewIssue(t, s.b, "one issue")
	addBridge(t, s, "gh", map[string]string{"target": "github", "owner": "o", "project": "p", "default-login": "bot", "sync": "true"})
	addBridge(t, s, "gl", map[string]string{"target": "gitlab", "project-id": "1", "base-url": "https://gitlab.example.com/", "default-login": "bot", "sync": "true"})
	var got []string
	s.runIssueSync = func(_ context.Context, dryRun bool, bug string) (<-chan core.SyncResult, error) {
		got = append(got, bug)
		out := make(chan core.SyncResult, 2)
		if dryRun {
			r := core.NewExportNothing("", "would send: 1 comments")
			out <- core.SyncResult{Bridge: "gh", Export: &r}
		} else {
			r := core.NewExportComment("")
			out <- core.SyncResult{Bridge: "gh", Export: &r}
		}
		close(out)
		return out, nil
	}
	type result struct {
		DryRun  bool                    `json:"dry_run"`
		Writes  int                     `json:"writes"`
		Errors  int                     `json:"errors"`
		Bridges map[string]*bridgeTally `json:"bridges"`
	}
	var res result

	if code := call(t, "POST", ts.URL+"/api/issues/"+id+"/sync", map[string]bool{"dry_run": false}, nil); code != http.StatusConflict {
		t.Errorf("a real run before its dry run: %d, want 409", code)
	}
	if code := call(t, "POST", ts.URL+"/api/issues/"+id+"/sync", map[string]bool{"dry_run": true}, &res); code != http.StatusOK {
		t.Fatalf("dry run: %d", code)
	}
	if !res.DryRun || res.Writes != 1 || res.Bridges["gh"].To.Comments != 1 {
		t.Errorf("dry run %+v", res)
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
