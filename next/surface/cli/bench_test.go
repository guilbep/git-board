package cli

import (
	"fmt"
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

// anIssue is an issue of the bench board, its first 8 characters.
func anIssue(b *testing.B, dir string) string {
	out, err := exec.Command("git", "-C", dir, "for-each-ref", "--count=1", "--format=%(refname:lstrip=3)", "refs/board/issue/").Output()
	if err != nil || len(out) < 8 {
		b.Fatalf("no issue: %v", err)
	}
	return strings.TrimSpace(string(out))[:8]
}

// show: budget 30 ms.
func BenchmarkShow(b *testing.B) { bench(b, "show", anIssue(b, benchRepo(b))) }

// The writes: budget 50 ms. Each adds to the bench board.
func BenchmarkComment(b *testing.B) {
	bench(b, "comment", anIssue(b, benchRepo(b)), "-b", "a comment from the benchmark")
}

func BenchmarkSet(b *testing.B) {
	dir := benchRepo(b)
	id := anIssue(b, dir)
	i := 0
	for b.Loop() { // a value that changes, each time
		i++
		if code := Main(ctx, []string{"-C", dir, "set", id, fmt.Sprintf("est=%d", i)}, io.Discard, os.Stderr); code != 0 {
			b.Fatalf("exit %d", code)
		}
	}
}

func BenchmarkNew(b *testing.B) { bench(b, "new", "From the benchmark", "-b", "body") }

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

// ls with free text, first on a board whose view has no search index
// yet: it builds it (budget 5 s at 20,000 issues).
func BenchmarkLsTextFirst(b *testing.B) {
	dir := benchRepo(b)
	common, err := exec.Command("git", "-C", dir, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		b.Fatal(err)
	}
	view := filepath.Join(strings.TrimSpace(string(common)), "board", "v2")
	for b.Loop() {
		b.StopTimer()
		os.RemoveAll(view)
		Main(ctx, []string{"-C", dir, "ls"}, io.Discard, io.Discard) // the rows, not the index
		b.StartTimer()
		if code := Main(ctx, []string{"-C", dir, "ls", "export crash"}, io.Discard, os.Stderr); code != 0 {
			b.Fatalf("exit %d", code)
		}
	}
}
