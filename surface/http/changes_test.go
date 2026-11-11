package httpapi

import (
	"net/http"
	"slices"
	"testing"
)

// The board as a page keeps it: the snapshot, every issue, its stamp as
// its ETag and a 304 for a page that holds it; the changes since its
// cursor, a write's issue as it is now; a cursor of another boot, gone; a
// list's 304; a filter's ids.
func TestSnapshotAndChanges(t *testing.T) {
	t.Parallel()
	dir, _ := newBoard(t)
	a := newAPI(t, dir)
	res := a.do("GET", "/api/snapshot", nil)
	snap := want[snapshotRes](t, res, 200)
	etag := res.Header.Get("ETag")
	if etag == "" || len(snap.Issues) < 2 || snap.Cursor == "" || snap.Stamp == "" {
		t.Fatalf("snapshot: ETag %q, %d issues, cursor %q", etag, len(snap.Issues), snap.Cursor)
	}
	if res := a.do("GET", "/api/snapshot", nil, "If-None-Match", etag); res.StatusCode != http.StatusNotModified {
		t.Fatalf("snapshot held: %d, want 304", res.StatusCode)
	}

	made := want[threadBody](t, a.do("POST", "/api/issues", map[string]any{"title": "Made since"}), 201)
	ch := want[changesRes](t, a.do("GET", "/api/changes?since="+snap.Cursor, nil), 200)
	if !slices.ContainsFunc(ch.Put, func(is issueJSON) bool { return is.ID == made.ID && is.Title == "Made since" }) || ch.Cursor == snap.Cursor {
		t.Errorf("changes since the snapshot: %+v", ch)
	}
	if again := want[changesRes](t, a.do("GET", "/api/changes?since="+ch.Cursor, nil), 200); len(again.Put)+len(again.Dropped) != 0 {
		t.Errorf("changes since the last: %+v, want none", again)
	}
	if res := a.do("GET", "/api/snapshot", nil, "If-None-Match", etag); res.StatusCode != 200 {
		t.Errorf("snapshot after a write, the old ETag: %d, want 200", res.StatusCode)
	}
	if res := a.do("GET", "/api/changes?since=another-boot:3", nil); res.StatusCode != http.StatusGone {
		t.Errorf("another boot's cursor: %d, want 410", res.StatusCode)
	}

	res = a.do("GET", "/api/issues?filter=is:open", nil)
	if res.StatusCode != 200 || res.Header.Get("ETag") == "" {
		t.Fatalf("list: %d, ETag %q", res.StatusCode, res.Header.Get("ETag"))
	}
	if again := a.do("GET", "/api/issues?filter=is:open", nil, "If-None-Match", res.Header.Get("ETag")); again.StatusCode != http.StatusNotModified {
		t.Errorf("list held: %d, want 304", again.StatusCode)
	}

	ids := want[idsRes](t, a.do("GET", "/api/issues/ids?f=Made&f=is:open&sort=title+asc", nil), 200)
	if len(ids.Lists) != 2 || !slices.Equal(ids.Lists[0], []string{made.ID}) || len(ids.Lists[1]) < 2 {
		t.Errorf("ids of free text, then of every open issue: %v", ids.Lists)
	}
	page := want[idsRes](t, a.do("GET", "/api/issues/ids?f=is:open&limit=1&offset=1&sort=title+asc", nil), 200)
	if len(page.Lists[0]) != 1 || page.Totals[0] != len(ids.Lists[1]) || page.Lists[0][0] != ids.Lists[1][1] {
		t.Errorf("the second id of the open issues, and their total: %+v, all %v", page, ids.Lists[1])
	}
}
