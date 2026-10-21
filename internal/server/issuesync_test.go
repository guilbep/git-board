package server

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/git-bug/git-bug/bridge/core"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// One issue syncs on its own: a dry run says what would go where, a real
// run needs that dry run first, and a full sync running refuses it.
func TestIssueSync(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	id := gitbugiotest.NewIssue(t, s.b, "one issue")
	addBridge(t, s, "gh", map[string]string{"target": "github", "owner": "o", "project": "p", "default-login": "bot", "sync": "true"})
	addBridge(t, s, "gl", map[string]string{"target": "gitlab", "project-id": "1", "base-url": "https://gitlab.example.com/", "default-login": "bot", "sync": "true"})
	var got []string
	s.runIssueSync = func(_ context.Context, dryRun bool, bug string, _ ...core.Option) (<-chan core.SyncResult, error) {
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

// One issue syncs with the toml's assignees mode, before any forge sync
// wrote it, and leaves writing it to the next forge sync.
func TestIssueSyncTakesTheAssigneesMode(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	s.cfg = &config.Config{Assignees: "labels"}
	id := gitbugiotest.NewIssue(t, s.b, "one issue")
	addBridge(t, s, "gh", map[string]string{"target": "github", "owner": "o", "project": "p", "default-login": "bot", "sync": "true"})
	var got core.SyncOptions
	s.runIssueSync = func(_ context.Context, _ bool, _ string, opts ...core.Option) (<-chan core.SyncResult, error) {
		for _, o := range opts {
			o(&got)
		}
		out := make(chan core.SyncResult)
		close(out)
		return out, nil
	}
	if code := call(t, "POST", ts.URL+"/api/issues/"+id+"/sync", map[string]bool{"dry_run": true}, nil); code != http.StatusOK {
		t.Fatalf("dry run: %d", code)
	}
	if got.Assignees != "labels" {
		t.Errorf("synced with assignees %q, want labels", got.Assignees)
	}
	cfg, err := s.b.TestCache().Config().Read()
	if err != nil {
		t.Fatal(err)
	}
	if v := cfg.Value("git-bug.bridge.gh.assignees", ""); v != "" {
		t.Errorf("one issue's sync wrote gh assignees %q", v)
	}
}

// Link, Take and Unlink, one issue on one forge: nothing is written to the
// forge; the engine's refusals answer 409 with what it says.
func TestIssueLinkTakeUnlink(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	f := gitbugiotest.NewForge(t, s.b, "fk")
	ref := f.UserAct("alice").CreateIssue("forge title", "forge body")
	id := gitbugiotest.NewIssue(t, s.b, "local title")
	other := gitbugiotest.NewIssue(t, s.b, "another")
	live, cancel := s.events.subscribe()
	defer cancel()
	path := func(id, action string) string { return ts.URL + "/api/issues/" + id + "/" + action }

	var linked struct{ ID, URL, Number string }
	if code := call(t, "POST", path(id, "link"), map[string]string{"bridge": "fk", "issue": "#" + ref.Number}, &linked); code != http.StatusOK {
		t.Fatalf("link: %d", code)
	}
	if linked.URL != ref.URL || linked.Number != ref.Number || linked.ID == "" {
		t.Errorf("linked %+v, want %+v", linked, ref)
	}
	select {
	case e := <-live:
		if !strings.Contains(string(e), `"issues"`) || !strings.Contains(string(e), id) {
			t.Errorf("live event %s, want issues %s", e, id)
		}
	case <-time.After(time.Second):
		t.Error("no live event for the link")
	}
	for _, c := range []struct {
		what, id, action string
		body             map[string]string
		code             int
		says             string
	}{
		{"the same link again", id, "link", map[string]string{"bridge": "fk", "issue": ref.Number}, http.StatusConflict, "already"},
		{"an issue another bug has", other, "link", map[string]string{"bridge": "fk", "issue": ref.URL}, http.StatusConflict, "already"},
		{"no such forge issue", other, "link", map[string]string{"bridge": "fk", "issue": "99"}, http.StatusConflict, "no issue 99"},
		{"an unknown bridge", id, "link", map[string]string{"bridge": "nope", "issue": "1"}, http.StatusNotFound, ""},
		{"an unknown issue", "0000000", "link", map[string]string{"bridge": "fk", "issue": "1"}, http.StatusNotFound, ""},
		{"take on a bug not linked", other, "take", map[string]string{"bridge": "fk"}, http.StatusConflict, "not synced"},
		{"take on an unknown bridge", id, "take", map[string]string{"bridge": "nope"}, http.StatusNotFound, ""},
	} {
		if code, msg := callError(t, "POST", path(c.id, c.action), c.body); code != c.code || !strings.Contains(msg, c.says) {
			t.Errorf("%s: %d %q, want %d saying %q", c.what, code, msg, c.code, c.says)
		}
	}

	var took issueSyncResult
	if code := call(t, "POST", path(id, "take"), map[string]string{"bridge": "fk"}, &took); code != http.StatusOK {
		t.Fatalf("take: %d", code)
	}
	th, _ := s.b.Thread(id)
	if th.Title != "forge title" || th.Events[0].Body != "forge body" {
		t.Errorf("after take: %q, %q; want the forge's", th.Title, th.Events[0].Body)
	}
	if took.Errors != 0 || took.Bridges["fk"] == nil || took.Bridges["fk"].From.Changes+took.Bridges["fk"].From.Edits == 0 {
		t.Errorf("take answered %+v, want what came from fk", took)
	}

	if code := call(t, "POST", path(id, "unlink"), map[string]string{"bridge": "fk"}, nil); code != http.StatusOK {
		t.Fatalf("unlink: %d", code)
	}
	if code, msg := callError(t, "POST", path(id, "unlink"), map[string]string{"bridge": "fk"}); code != http.StatusConflict || !strings.Contains(msg, "not synced") {
		t.Errorf("unlink again: %d %q, want 409", code, msg)
	}
	if code, msg := callError(t, "POST", path(id, "link"), map[string]string{"bridge": "fk", "issue": ref.Number}); code != http.StatusConflict || !strings.Contains(msg, "can't change") {
		t.Errorf("link once unlinked: %d %q, want 409, final", code, msg)
	}
	if code := call(t, "POST", path(other, "link"), map[string]string{"bridge": "fk", "issue": ref.Number}, nil); code != http.StatusOK {
		t.Errorf("the unlinked issue to another bug: %d", code)
	}
	if f.Writes() != 0 {
		t.Errorf("%d writes to the forge, want none", f.Writes())
	}

	s.syncing.Store(true)
	if code := call(t, "POST", path(id, "take"), map[string]string{"bridge": "fk"}, nil); code != http.StatusConflict {
		t.Errorf("during a full sync: %d, want 409", code)
	}
}
