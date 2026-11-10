package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/guilbep/git-board/store"
	"github.com/guilbep/git-board/tools/boardgen"
)

// The routes' complexity guards (docs/guide/performance.md, v2): on a
// generated board of guardIssues issues, each request counts its work
// (store.Counts) and is held to a limit far below the board's size: no
// route reads an entity once its view is fresh, and none makes a query
// per issue (an N+1 is thousands over). Counts, not time: CI's noise
// can't fail them.

const guardIssues = 2000

var guard struct {
	once sync.Once
	dir  string
	b    *boardgen.Board
	err  error
}

// guardServer serves a copy of the guards' board, made once for the
// package's run; its handler, the board answered once (its view built).
func guardServer(t *testing.T) (http.Handler, *boardgen.Board) {
	t.Helper()
	guard.once.Do(func() {
		tmp, err := os.MkdirTemp("", "git-board-guards-")
		if err != nil {
			guard.err = err
			return
		}
		guard.dir = filepath.Join(tmp, "board")
		guard.b, guard.err = boardgen.Git(context.Background(), guard.dir, boardgen.Options{Issues: guardIssues, Seed: 1})
	})
	if guard.err != nil {
		t.Fatal(guard.err)
	}
	dir := t.TempDir()
	if err := os.CopyFS(dir, os.DirFS(guard.dir)); err != nil {
		t.Fatal(err)
	}
	log, _ := NewLogger(io.Discard, "error", "text")
	s, err := New(t.Context(), Options{Dir: dir, Log: log})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	h := s.Handler()
	countGet(t, h, "/api/issues?filter=&limit=0")
	return h, guard.b
}

// countGet answers what GET path did.
func countGet(t *testing.T, h http.Handler, path string) *store.Counts {
	t.Helper()
	c := &store.Counts{}
	req := httptest.NewRequest("GET", path, nil).WithContext(store.WithCounts(t.Context(), c))
	req.Host = "localhost"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Fatalf("GET %s: %d: %s", path, rec.Code, rec.Body.String()[:min(300, rec.Body.Len())])
	}
	return c
}

// warmRoute is a fresh view's request: no entity, no git object, no
// process, a few listings and view calls whatever the board's size.
var warmRoute = map[string]int64{"entities": 0, "objects": 0, "loads": 0, "processes": 0, "listings": 6, "view calls": 16, "rows put": 0}

func within(t *testing.T, what string, c *store.Counts, l map[string]int64) {
	t.Helper()
	got := c.Snapshot()
	for k, most := range l {
		if v, ok := got[k]; !ok {
			t.Fatalf("%s: no counter %q", what, k)
		} else if v > most {
			t.Errorf("%s: %s %d, at most %d on %d issues (%v)", what, k, v, most, guardIssues, got)
		}
	}
	if testing.Verbose() {
		t.Logf("%s: %v", what, got)
	}
}

// The list routes, the facets, the people and the board read the view
// alone, in a few calls: no N+1.
func TestGuardRoutes(t *testing.T) {
	t.Parallel()
	h, gb := guardServer(t)
	for _, path := range []string{
		"/api/issues",
		"/api/issues?filter=&limit=0",
		"/api/issues?filter=status:ready+is:open&sort=updated+desc",
		"/api/labels",
		"/api/fields",
		"/api/people",
		"/api/board",
		"/api/inbox",
	} {
		within(t, "GET "+path, countGet(t, h, path), warmRoute)
	}
	// one issue: its entity alone
	id := string(gb.Issues[5])
	within(t, "GET /api/issues/{id}", countGet(t, h, "/api/issues/"+id),
		map[string]int64{"entities": 1, "loads": 1, "objects": 40, "processes": 2, "listings": 6, "view calls": 16, "rows put": 0})
	// the Work page: every open thread in one load, then none
	within(t, "GET /api/cycle, first", countGet(t, h, "/api/cycle?who=team"),
		map[string]int64{"entities": guardIssues, "loads": 4, "processes": 2, "listings": 8, "view calls": 32})
	within(t, "GET /api/cycle", countGet(t, h, "/api/cycle?who=team&limit=100"), warmRoute)
}

// The inbox reads the threads of the issues updated in its window: the
// people, the forges' records and the logs once for all of them, not a
// listing and a read per issue.
func TestGuardInboxRecent(t *testing.T) {
	t.Parallel()
	h, gb := guardServer(t)
	const recent = 20
	for _, id := range gb.Issues[:recent] {
		req := httptest.NewRequest("POST", "/api/issues/"+string(id)+"/comments", strings.NewReader(`{"body":"recent"}`))
		req.Host = "localhost"
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code/100 != 2 {
			t.Fatalf("comment: %d: %s", rec.Code, rec.Body.String())
		}
	}
	countGet(t, h, "/api/inbox") // the view, fresh again after the writes
	within(t, "GET /api/inbox, 20 recent", countGet(t, h, "/api/inbox"),
		map[string]int64{"loads": 1, "listings": 8, "processes": 0})
}
