package server

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/guilbep/git-board/core/issue/boardgen"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// The server's lists on boardgen.BenchRepo: $GIT_BOARD_BENCH_REPO, or a
// board of 1,000 issues (docs/guide/performance.md). The board stays
// open, as the server keeps it.
func BenchmarkAPI(b *testing.B) {
	dir, err := boardgen.BenchRepo()
	if err != nil {
		b.Fatal(err)
	}
	cfg, err := config.Load(config.PathIn(dir))
	if err != nil {
		b.Fatal(err)
	}
	be, err := store.Open(dir)
	if err != nil {
		b.Fatal(err)
	}
	defer be.Close()
	mux := newMux(&apiServer{cfg: cfg, b: be, me: "dev3"}, http.NotFoundHandler())
	if _, err := be.ListIssues(); err != nil { // the server's first list, before
		b.Fatal(err)
	}
	for _, path := range []string{"/api/issues", "/api/issues?filter=invoice+crashes", "/api/facets"} {
		b.Run(path, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				w := httptest.NewRecorder()
				mux.ServeHTTP(w, httptest.NewRequest("GET", "http://localhost"+path, nil))
				if w.Code != http.StatusOK {
					b.Fatalf("%s: %d %s", path, w.Code, w.Body)
				}
			}
		})
	}
}
