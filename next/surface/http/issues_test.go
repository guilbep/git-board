package httpapi

import (
	"net/http"
	"slices"
	"strings"
	"testing"
)

// The issue routes, end to end on a real board: a list, paged; a show
// with its version; every change through PATCH, compare-and-swap.
func TestIssues(t *testing.T) {
	t.Parallel()
	dir, ids := newBoard(t)
	a := newAPI(t, dir)

	open := want[issuePage](t, a.do("GET", "/api/issues", nil), 200)
	if open.Total != 1 || open.Items[0].Title != "Export crashes" || open.Items[0].Version == "" {
		t.Fatalf("GET /api/issues = %+v, want the open one, with its version", open)
	}
	all := want[issuePage](t, a.do("GET", "/api/issues?filter=&limit=1&sort=prio+desc", nil), 200)
	if all.Total != 2 || len(all.Items) != 1 || all.Next == "" || all.Items[0].Title != "Export crashes" {
		t.Fatalf("a first page = %+v", all)
	}
	rest := want[issuePage](t, a.do("GET", "/api/issues?filter=&limit=1&sort=prio+desc&cursor="+all.Next, nil), 200)
	if len(rest.Items) != 1 || rest.Next != "" || rest.Items[0].Title != "Login fails" {
		t.Fatalf("the last page = %+v", rest)
	}

	id := string(ids[1])
	res := a.do("GET", "/api/issues/"+id[:8], nil)
	th := want[threadBody](t, res, 200)
	if th.ID != id || etag(res) != `"`+th.Version+`"` || th.Events[0].HTML != "<p>Body 1</p>\n" {
		t.Fatalf("GET /api/issues/{id} = %+v, ETag %s", th, etag(res))
	}
	if res := a.do("GET", "/api/issues/"+id, nil, "If-None-Match", etag(res)); res.StatusCode != http.StatusNotModified {
		t.Fatalf("If-None-Match of the version: %d, want 304", res.StatusCode)
	}

	// a change from the version read; then one from it again is stale
	res = a.do("PATCH", "/api/issues/"+id, map[string]any{"title": "Export crashes on Safari", "labels": map[string]any{"add": []string{"ui"}}, "set": []string{"prio=p0"}}, "If-Match", etag(res))
	th2 := want[threadBody](t, res, 200)
	if th2.Title != "Export crashes on Safari" || !slices.Contains(th2.Labels, "ui") || !slices.Contains(th2.Labels, "prio:p0") || th2.Version == th.Version || etag(res) != `"`+th2.Version+`"` {
		t.Fatalf("PATCH = %+v", th2)
	}
	e := want[apiError](t, a.do("PATCH", "/api/issues/"+id, map[string]any{"title": "Lost"}, "If-Match", `"`+th.Version+`"`), http.StatusPreconditionFailed)
	if e.Code != "stale" {
		t.Fatalf("a stale PATCH = %+v, want code stale", e)
	}
	if got := want[threadBody](t, a.do("GET", "/api/issues/"+id, nil), 200); got.Title != th2.Title {
		t.Fatalf("a stale PATCH wrote: %q", got.Title)
	}

	// comments: add, edit from a base that moved, edit, delete
	res = a.do("POST", "/api/issues/"+id+"/comments", map[string]any{"body": "Repro on **Safari 17**"}, "If-Match", `"`+th2.Version+`"`)
	th3 := want[threadBody](t, res, http.StatusCreated)
	loc := res.Header.Get("Location")
	cid := loc[strings.LastIndexByte(loc, '/')+1:]
	last := th3.Events[len(th3.Events)-1]
	if last.ID != cid || last.HTML != "<p>Repro on <strong>Safari 17</strong></p>\n" {
		t.Fatalf("POST comments: Location %s, last event %+v", loc, last)
	}
	e = want[apiError](t, a.do("PATCH", "/api/issues/"+id+"/comments/"+cid, map[string]any{"body": "x", "base": "not what it was"}), http.StatusConflict)
	if e.Code != "changed" {
		t.Fatalf("an edit from another base = %+v, want changed", e)
	}
	th4 := want[threadBody](t, a.do("PATCH", "/api/issues/"+id+"/comments/"+cid, map[string]any{"body": "Repro on Safari 18", "base": "Repro on **Safari 17**"}), 200)
	if b := th4.Events[len(th4.Events)-1].Body; b != "Repro on Safari 18" {
		t.Fatalf("an edit = %q", b)
	}
	th5 := want[threadBody](t, a.do("DELETE", "/api/issues/"+id+"/comments/"+cid, nil), 200)
	if slices.ContainsFunc(th5.Events, func(e struct{ Kind, ID, Body, HTML string }) bool { return e.ID == cid && e.Body != "" }) {
		t.Fatalf("the comment is still there: %+v", th5.Events)
	}

	// close with a comment, as a reason; reopen
	th6 := want[threadBody](t, a.do("PATCH", "/api/issues/"+id, map[string]any{"status": "closed", "comment": "Fixed in 2.1", "reason": "completed"}), 200)
	if th6.Status != "closed" || !slices.ContainsFunc(th6.Events, func(e struct{ Kind, ID, Body, HTML string }) bool { return e.Body == "Fixed in 2.1" }) {
		t.Fatalf("close = %+v", th6)
	}
	if th := want[threadBody](t, a.do("PATCH", "/api/issues/"+id, map[string]any{"status": "open"}), 200); th.Status != "open" {
		t.Fatalf("reopen = %s", th.Status)
	}
	if e := want[apiError](t, a.do("PATCH", "/api/issues/"+id, map[string]any{"status": "done"}), 400); e.Field != "status" || e.Code != "invalid" {
		t.Fatalf("a status that isn't = %+v", e)
	}

	// the log, undo, follow
	log := want[items[map[string]any]](t, a.do("GET", "/api/issues/"+id+"/log", nil), 200)
	if len(log.Items) < 5 || log.Items[0]["kind"] != "created" {
		t.Fatalf("log = %v", log.Items)
	}
	u := want[map[string]any](t, a.do("POST", "/api/issues/"+id+"/undo", map[string]any{}), 200)
	if u["issue"] == nil {
		t.Fatalf("undo = %v", u)
	}
	if f := want[map[string]any](t, a.do("GET", "/api/issues/"+id+"/follow", nil), 200); f["following"] != true {
		t.Fatalf("follow, as its author = %v", f)
	}
	if f := want[map[string]any](t, a.do("PUT", "/api/issues/"+id+"/follow", map[string]any{"follow": false}), 200); f["following"] != false || f["set"] != false {
		t.Fatalf("unfollow = %v", f)
	}

	// create, then delete
	res = a.do("POST", "/api/issues", map[string]any{"title": "New one", "body": "It is new", "set": []string{"prio=p2"}})
	nw := want[threadBody](t, res, http.StatusCreated)
	if res.Header.Get("Location") != "/api/issues/"+nw.ID || !slices.Contains(nw.Labels, "prio:p2") || etag(res) == "" {
		t.Fatalf("POST /api/issues = %+v, Location %s", nw, res.Header.Get("Location"))
	}
	if e := want[apiError](t, a.do("POST", "/api/issues", map[string]any{"title": " "}), 400); e.Field != "title" {
		t.Fatalf("no title = %+v", e)
	}
	if res := a.do("DELETE", "/api/issues/"+nw.ID, nil, "If-Match", `"`+nw.Version+`"`); res.StatusCode != http.StatusNoContent {
		t.Fatalf("DELETE: %d", res.StatusCode)
	}
	if e := want[apiError](t, a.do("GET", "/api/issues/"+nw.ID, nil), 404); e.Code != "not-found" {
		t.Fatalf("a deleted issue = %+v", e)
	}
}

// The same nouns as the CLI: the labels in use and the fields, the
// people, who writes, the inbox.
func TestBoardRoutes(t *testing.T) {
	t.Parallel()
	dir, ids := newBoard(t)
	a := newAPI(t, dir)
	ls := want[items[map[string]any]](t, a.do("GET", "/api/labels?filter=is:open", nil), 200)
	if len(ls.Items) != 2 { // bug, prio:p1
		t.Fatalf("labels of the open issues = %v", ls.Items)
	}
	fs := want[map[string]map[string]any](t, a.do("GET", "/api/fields", nil), 200)
	if _, ok := fs["prio"]; ok { // a select undeclared: the labels say it
		t.Fatalf("fields = %v: an undeclared select isn't one", fs)
	}
	ps := want[items[map[string]any]](t, a.do("GET", "/api/people", nil), 200)
	if len(ps.Items) != 1 || ps.Items[0]["login"] != "ada" {
		t.Fatalf("people = %v", ps.Items)
	}
	m := want[meRes](t, a.do("GET", "/api/me", nil), 200)
	if m.Me == nil || m.Me.Login != "ada" || m.Avatars != "forge" {
		t.Fatalf("me = %+v", m)
	}
	m = want[meRes](t, a.do("PUT", "/api/me", map[string]any{"avatars": "none"}), 200)
	if m.Avatars != "none" || m.Me.AvatarURL != "" {
		t.Fatalf("me, avatars none = %+v", m)
	}
	m = want[meRes](t, a.do("PUT", "/api/me", map[string]any{"name": "Bob", "email": "bob@example.com"}), 200)
	if m.Me == nil || m.Me.Name != "Bob" {
		t.Fatalf("a new me = %+v", m)
	}
	// Bob again: the request's mistake, not the server's
	if e := want[apiError](t, a.do("PUT", "/api/me", map[string]any{"name": "Bob", "email": "bob@example.com"}), 400); e.Code != "invalid" || e.Field != "name" {
		t.Errorf("Bob made twice: %+v", e)
	}
	// Bob writes now
	th := want[threadBody](t, a.do("POST", "/api/issues/"+string(ids[1])+"/comments", map[string]any{"body": "me too"}), 201)
	_ = th
	in := want[map[string]any](t, a.do("GET", "/api/inbox", nil), 200)
	if in["me"] == nil {
		t.Fatalf("inbox = %v", in)
	}
	want[map[string]any](t, a.do("POST", "/api/inbox/seen", map[string]any{"issues": []string{string(ids[1])}}), 200)
	if e := want[apiError](t, a.do("PUT", "/api/inbox/settings", map[string]any{"views": []string{"nope"}}), 400); e.Field != "views" {
		t.Fatalf("a view that isn't = %+v", e)
	}
	md := want[map[string]string](t, a.do("POST", "/api/markdown", map[string]any{"body": "*hi* <script>alert(1)</script>"}), 200)
	if !strings.Contains(md["html"], "<em>hi</em>") || strings.Contains(md["html"], "script") {
		t.Fatalf("markdown = %q", md["html"])
	}
	b := want[boardRes](t, a.do("GET", "/api/board", nil), 200)
	if b.Datastore.Name != "git" || b.GitDir == "" || b.Version != "test" {
		t.Fatalf("board = %+v", b)
	}
}
