package git

import (
	"context"
	"os"
	"sync/atomic"
	"testing"

	"github.com/guilbep/git-board/store"
)

// The benchmarks read the board at $GIT_BOARD_BENCH_REPO (a boardgen
// board, docs/guide/performance.md), never writing to it.
func benchStore(b *testing.B) *Store {
	dir := os.Getenv("GIT_BOARD_BENCH_REPO")
	if dir == "" {
		b.Skip("GIT_BOARD_BENCH_REPO: a board to read")
	}
	s, err := Open(context.Background(), dir)
	if err != nil {
		b.Fatal(err)
	}
	return s
}

func BenchmarkHeads(b *testing.B) {
	s := benchStore(b)
	for b.Loop() {
		if _, err := s.Heads(context.Background(), "issue", "person"); err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkLoadAll is a first build's reading: every issue, every pack.
func BenchmarkLoadAll(b *testing.B) {
	s := benchStore(b)
	ctx := context.Background()
	heads, err := s.Heads(ctx, "issue")
	if err != nil {
		b.Fatal(err)
	}
	refs := make([]store.Ref, len(heads))
	for i, h := range heads {
		refs[i] = store.Ref(h)
	}
	for b.Loop() {
		var packs atomic.Int64
		err := s.Load(ctx, refs, func(_ store.Ref, p [][]byte, err error) error {
			packs.Add(int64(len(p)))
			return err
		})
		if err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(float64(packs.Load()), "packs")
	}
}

// BenchmarkLoadOne is show's reading: one issue.
func BenchmarkLoadOne(b *testing.B) {
	s := benchStore(b)
	ctx := context.Background()
	heads, err := s.Heads(ctx, "issue")
	if err != nil {
		b.Fatal(err)
	}
	ref := store.Ref(heads[len(heads)/2])
	for b.Loop() {
		if err := s.Load(ctx, []store.Ref{ref}, func(_ store.Ref, _ [][]byte, err error) error { return err }); err != nil {
			b.Fatal(err)
		}
	}
}
