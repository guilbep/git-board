package httpapi

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The server's budgets (docs/rewrite/design.md, section 4) on
// $GIT_BOARD_BENCH_REPO, a board boardgen made (a copy: the writes add
// to it):
//
//	cp -r /tmp/b20k.pristine /tmp/b20k
//	GIT_BOARD_BENCH_REPO=/tmp/b20k go test ./next/surface/http -run - -bench . -benchtime 20x
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

// One issue's thread: budget 10 ms.
func BenchmarkThread(b *testing.B) {
	_, _, dir := benchAPI(b)
	out, err := exec.Command("git", "-C", dir, "for-each-ref", "--count=1", "--format=%(refname:lstrip=3)", "refs/board/issue/").Output()
	if err != nil {
		b.Fatal(err)
	}
	benchGet(b, "/api/issues/"+strings.TrimSpace(string(out)))
}
