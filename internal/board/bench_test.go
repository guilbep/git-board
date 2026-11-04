package board

import (
	"testing"

	"github.com/guilbep/git-board/core/issue/boardgen"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// The benchmarks run on boardgen.BenchRepo: $GIT_BOARD_BENCH_REPO, or a
// board of 1,000 issues (docs/guide/performance.md).

func benchRepo(b *testing.B) (string, *config.Config) {
	b.Helper()
	dir, err := boardgen.BenchRepo()
	if err != nil {
		b.Fatal(err)
	}
	cfg, err := config.Load(config.PathIn(dir))
	if err != nil {
		b.Fatal(err)
	}
	return dir, cfg
}

func open(b *testing.B, dir string) *store.Backend {
	b.Helper()
	be, err := store.Open(dir)
	if err != nil {
		b.Fatal(err)
	}
	return be
}

// filters are ls's common ones.
var filters = []struct{ name, filter string }{
	{"open", "is:open"},
	{"text", "invoice crashes"},
	{"fields", "status:ready assignee:dev3"},
}

// A command's ls: open the board, list, match, close, each time.
func BenchmarkCLILs(b *testing.B) {
	dir, cfg := benchRepo(b)
	for _, f := range filters {
		b.Run(f.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				be := open(b, dir)
				if _, err := LoadMatching(be, cfg, f.filter, "dev3"); err != nil {
					b.Fatal(err)
				}
				be.Close()
			}
		})
	}
}

// A server's ls: the board open already, listed again.
func BenchmarkWarmLs(b *testing.B) {
	dir, cfg := benchRepo(b)
	be := open(b, dir)
	defer be.Close()
	if _, err := be.ListIssues(); err != nil { // the server's first list, before
		b.Fatal(err)
	}
	for _, f := range filters {
		b.Run(f.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if _, err := LoadMatching(be, cfg, f.filter, "dev3"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Opening the board and bringing its index up to date, as every command
// does.
func BenchmarkOpen(b *testing.B) {
	dir, _ := benchRepo(b)
	b.ReportAllocs()
	for b.Loop() {
		be := open(b, dir)
		be.Labels() // refreshes the index
		be.Close()
	}
}

// A command's show: open the board, its thread and issue, close.
func BenchmarkCLIShow(b *testing.B) {
	dir, _ := benchRepo(b)
	be := open(b, dir)
	issues, err := be.ListIssues()
	be.Close()
	if err != nil || len(issues) == 0 {
		b.Fatal("no issue", err)
	}
	id := issues[len(issues)/2].ID[:8]
	b.ReportAllocs()
	for b.Loop() {
		be := open(b, dir)
		if _, err := be.Thread(id); err != nil {
			b.Fatal(err)
		}
		if _, err := be.GetIssue(id); err != nil {
			b.Fatal(err)
		}
		be.Close()
	}
}

// A command's new, then set: each opens the board, writes, reads the
// issue back. They add issues to the board they run on.
func BenchmarkCLINewSet(b *testing.B) {
	dir, _ := benchRepo(b)
	b.ReportAllocs()
	for b.Loop() {
		be := open(b, dir)
		id, err := be.NewIssue("A benchmark's issue", "made by BenchmarkCLINewSet", []string{"status:backlog"})
		if err != nil {
			b.Fatal(err)
		}
		if _, err := be.GetIssue(id); err != nil {
			b.Fatal(err)
		}
		be.Close()
		be = open(b, dir)
		if _, err := be.ApplyLabelChange(id[:10], []string{"prio:p1"}, nil); err != nil {
			b.Fatal(err)
		}
		if _, err := be.GetIssue(id[:10]); err != nil {
			b.Fatal(err)
		}
		be.Close()
	}
}
