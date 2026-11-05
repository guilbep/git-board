package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A benchmark's runs are named board/package/name, their median is
// what a budget holds.
func TestReadBenchMedian(t *testing.T) {
	t.Parallel()
	f := filepath.Join(t.TempDir(), "bench.txt")
	out := "pkg: github.com/guilbep/git-board/surface/cli\n" +
		"BenchmarkLs-2   5   90000000 ns/op\n" +
		"BenchmarkLs-2   5  110000000 ns/op\n" +
		"BenchmarkLs-2   5  100000000 ns/op\n" +
		"pkg: github.com/guilbep/git-board/surface/http\n" +
		"BenchmarkListAll-2  5  1000 ns/op  67.36 MB/s\n"
	if err := os.WriteFile(f, []byte(out), 0o644); err != nil {
		t.Fatal(err)
	}
	runs := map[string][]time.Duration{}
	if err := readBench(f, "20k", runs); err != nil {
		t.Fatal(err)
	}
	if m := median(runs["20k/cli/Ls"]); m != 100*time.Millisecond {
		t.Fatalf("20k/cli/Ls median = %v, runs %v", m, runs)
	}
	if len(runs["20k/http/ListAll"]) != 1 {
		t.Fatalf("runs = %v", runs)
	}
	bs, order, err := readBudgets(filepath.Join("..", "..", "testdata", "perf", "budgets.txt"))
	if err != nil || len(order) == 0 || bs["20k/http/WorkTeamPage"].max != 300*time.Millisecond {
		t.Fatalf("budgets.txt: %v %v %v", bs["20k/http/WorkTeamPage"], len(order), err)
	}
}
