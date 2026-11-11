package httpapi

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// The server's budgets (docs/rewrite/design.md, section 4) on
// $GIT_BOARD_BENCH_REPO, a board boardgen made (a copy: the writes add
// to it):
//
//	cp -r /tmp/b20k.pristine /tmp/b20k
//	GIT_BOARD_BENCH_REPO=/tmp/b20k go test ./surface/http -run - -bench . -benchtime 20x
//
// Each request goes through the whole handler: the checks, the log, the
// snapshot, the JSON. The listings are the budget's: 100 ms a list.
func benchAPI(b *testing.B) (*Server, http.Handler, string) {
	dir := os.Getenv("GIT_BOARD_BENCH_REPO")
	if dir == "" {
		b.Skip("GIT_BOARD_BENCH_REPO: a board to measure on")
	}
	log, _ := NewLogger(io.Discard, "info", "text")
	s, err := New(ctx, Options{Dir: dir, Log: log})
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(func() { s.Close() })
	return s, s.Handler(), dir
}

func get(b *testing.B, h http.Handler, path string) int {
	req := httptest.NewRequest("GET", path, nil)
	req.Host = "localhost"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		b.Fatalf("GET %s: %d: %s", path, rec.Code, rec.Body.String()[:min(300, rec.Body.Len())])
	}
	return rec.Body.Len()
}

func benchGet(b *testing.B, path string) {
	_, h, _ := benchAPI(b)
	n := get(b, h, path) // the view fresh, as a running server's is
	b.SetBytes(int64(n))
	for b.Loop() {
		get(b, h, path)
	}
}

// The open issues, a first page: budget 100 ms at 20,000 issues.
func BenchmarkListOpen(b *testing.B) { benchGet(b, "/api/issues") }

// Every issue, every page at once (limit=0): the most a list sends.
func BenchmarkListAll(b *testing.B) { benchGet(b, "/api/issues?filter=&limit=0") }

// A board's column: a filter on fields, sorted.
func BenchmarkListColumn(b *testing.B) {
	benchGet(b, "/api/issues?filter=status:ready+is:open&sort=updated+desc")
}

// The board as a page keeps it: every issue, open and closed, its ETag
// (the page's first open, and each after a change of the board).
func BenchmarkSnapshot(b *testing.B) { benchGet(b, "/api/snapshot") }

// One issue's thread: budget 10 ms.
func BenchmarkThread(b *testing.B) {
	_, _, dir := benchAPI(b)
	out, err := exec.Command("git", "-C", dir, "for-each-ref", "--count=1", "--format=%(refname:lstrip=3)", "refs/board/issue/").Output()
	if err != nil {
		b.Fatal(err)
	}
	benchGet(b, "/api/issues/"+strings.TrimSpace(string(out)))
}

// The Work page, every open issue's step, unpaged (the cycle on in the
// board's .git-board.toml), on a server that answered it once.
func BenchmarkWorkTeam(b *testing.B) { benchGet(b, "/api/cycle?who=team") }

// The Work page's first page, each group's first 100 rows: budget 300 ms
// at 20,000 issues.
func BenchmarkWorkTeamPage(b *testing.B) { benchGet(b, "/api/cycle?who=team&limit=100") }

// The Work page's first request on a server just opened, its rows
// fresh: every open issue's thread read once.
func BenchmarkWorkTeamFirst(b *testing.B) {
	for b.Loop() {
		b.StopTimer()
		s, h, _ := benchAPI(b)
		get(b, h, "/api/issues")
		b.StartTimer()
		get(b, h, "/api/cycle?who=team")
		b.StopTimer()
		s.Close()
		b.StartTimer()
	}
}

// The filters panel's counts, every facet: what a board asks with its
// list.
func BenchmarkFacets(b *testing.B) { benchGet(b, "/api/labels") }

// fanOut is how many pages listen to the change events in
// BenchmarkEventsFanOut.
const fanOut = 50

// A comment through the API, told to fanOut pages: from the request to
// the last page's event.
func BenchmarkEventsFanOut(b *testing.B) {
	_, h, dir := benchAPI(b)
	srv := httptest.NewServer(h)
	defer srv.Close()
	got := make(chan struct{}, fanOut*4)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	for range fanOut {
		req, _ := http.NewRequestWithContext(ctx, "GET", srv.URL+"/api/events", nil)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		go func() {
			defer res.Body.Close()
			sc := bufio.NewScanner(res.Body)
			for sc.Scan() {
				if strings.HasPrefix(sc.Text(), "data: ") && strings.Contains(sc.Text(), `"kind":"issue"`) {
					got <- struct{}{}
				}
			}
		}()
	}
	out, err := exec.Command("git", "-C", dir, "for-each-ref", "--count=1", "--format=%(refname:lstrip=3)", "refs/board/issue/").Output()
	if err != nil {
		b.Fatal(err)
	}
	path := srv.URL + "/api/issues/" + strings.TrimSpace(string(out)) + "/comments"
	time.Sleep(100 * time.Millisecond) // every page subscribed
	for b.Loop() {
		res, err := http.Post(path, "application/json", strings.NewReader(`{"body":"from the fan-out benchmark"}`))
		if err != nil || res.StatusCode != 201 {
			b.Fatalf("POST: %v %v", res, err)
		}
		res.Body.Close()
		for range fanOut {
			select {
			case <-got:
			case <-time.After(10 * time.Second):
				b.Fatal("a page had no event")
			}
		}
	}
}
