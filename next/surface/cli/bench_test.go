package cli

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The budgets of docs/rewrite/design.md, section 4, as benchmarks of the
// commands on $GIT_BOARD_BENCH_REPO, a board boardgen made
// (docs/guide/performance.md):
//
//	go run ./cmd/boardgen -n 20000 /tmp/b20k
//	GIT_BOARD_BENCH_REPO=/tmp/b20k go test ./next/surface/cli -run - -bench . -benchtime 10x
//
// A process's start (5 ms) is not in them. BenchmarkLsFirst deletes the
// board's view (.git/board/v2) each time: it is disposable.
func benchRepo(b *testing.B) string {
	dir := os.Getenv("GIT_BOARD_BENCH_REPO")
	if dir == "" {
		b.Skip("GIT_BOARD_BENCH_REPO: a board to measure on")
	}
	return dir
}

func bench(b *testing.B, args ...string) {
	dir := benchRepo(b)
	if code := Main(ctx, append([]string{"-C", dir}, args...), io.Discard, os.Stderr); code != 0 {
		b.Fatalf("%v: exit %d", args, code)
	}
	for b.Loop() {
		Main(ctx, append([]string{"-C", dir}, args...), io.Discard, io.Discard)
	}
}

// ls, view fresh: budget 100 ms at 20,000 issues.
func BenchmarkLs(b *testing.B) { bench(b, "ls") }

// ls with fields: fewer rows to print.
func BenchmarkLsFields(b *testing.B) { bench(b, "ls", "status:ready assignee:dev3") }

// ls with free text, its index built.
func BenchmarkLsText(b *testing.B) { bench(b, "ls", "export crash") }

// show: budget 30 ms.
func BenchmarkShow(b *testing.B) {
	dir := benchRepo(b)
	out, err := exec.Command("git", "-C", dir, "for-each-ref", "--count=1", "--format=%(refname:lstrip=3)", "refs/board/issue/").Output()
	if err != nil || len(out) < 8 {
		b.Fatalf("no issue: %v", err)
	}
	bench(b, "show", strings.TrimSpace(string(out))[:8])
}

// ls, first on a clone (no view): budget 3 s.
func BenchmarkLsFirst(b *testing.B) {
	dir := benchRepo(b)
	common, err := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		b.Fatal(err)
	}
	view := filepath.Join(strings.TrimSpace(string(common)), "board", "v2")
	for b.Loop() {
		b.StopTimer()
		os.RemoveAll(view)
		b.StartTimer()
		if code := Main(ctx, []string{"-C", dir, "ls"}, io.Discard, os.Stderr); code != 0 {
			b.Fatalf("exit %d", code)
		}
	}
}
