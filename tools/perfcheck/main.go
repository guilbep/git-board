// Command perfcheck compares benchmarks' medians with their budgets
// (make perf; docs/guide/performance.md, v2): each argument names a
// board and the `go test -bench` output measured on it, as
// 20k=bench-20k.txt. A benchmark is named <board>/<package>/<name>, its
// package the last element of the pkg: line above it (cli, http, git).
// It fails when a median exceeds its budget by more than the tolerance,
// or when a budget names a benchmark that didn't run.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"
)

type budget struct {
	max time.Duration
	why string
}

func main() {
	budgets := flag.String("budgets", "testdata/perf/budgets.txt", "the budgets file: name, budget, why")
	tolerance := flag.Float64("tolerance", 0.15, "how far over its budget a median may be")
	report := flag.String("report", "", "a Markdown table of the results, written there too")
	flag.Parse()
	bs, order, err := readBudgets(*budgets)
	if err != nil {
		fatal(err)
	}
	runs := map[string][]time.Duration{}
	for _, arg := range flag.Args() {
		board, file, ok := strings.Cut(arg, "=")
		if !ok {
			fatal(fmt.Errorf("%q: board=file", arg))
		}
		if err := readBench(file, board, runs); err != nil {
			fatal(err)
		}
	}
	var out strings.Builder
	out.WriteString("| benchmark | median | runs | budget | |\n|---|---|---|---|---|\n")
	failed := 0
	names := slices.Sorted(func(yield func(string) bool) {
		for n := range runs {
			if !yield(n) {
				return
			}
		}
	})
	for _, n := range order {
		if _, ok := runs[n]; !ok {
			fmt.Fprintf(&out, "| %s | | 0 | %v | FAIL: didn't run |\n", n, bs[n].max)
			failed++
		}
	}
	for _, n := range names {
		m := median(runs[n])
		b, ok := bs[n]
		verdict := "no budget"
		switch {
		case !ok:
		case float64(m) > float64(b.max)*(1+*tolerance):
			verdict = fmt.Sprintf("FAIL: %.0f%% over", 100*(float64(m)/float64(b.max)-1))
			failed++
		default:
			verdict = fmt.Sprintf("ok, %.0f%%", 100*float64(m)/float64(b.max))
		}
		limit := ""
		if ok {
			limit = b.max.String()
		}
		fmt.Fprintf(&out, "| %s | %v | %d | %s | %s |\n", n, round(m), len(runs[n]), limit, verdict)
	}
	fmt.Print(out.String())
	if *report != "" {
		if err := os.WriteFile(*report, []byte(out.String()), 0o644); err != nil {
			fatal(err)
		}
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "perfcheck: %d over budget (tolerance %.0f%%), or not run\n", failed, 100**tolerance)
		os.Exit(1)
	}
}

// readBudgets reads lines of name, budget (a Go duration) and why; #
// starts a comment.
func readBudgets(file string) (map[string]budget, []string, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()
	bs := map[string]budget{}
	var order []string
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fs := strings.Fields(line)
		if len(fs) < 3 {
			return nil, nil, fmt.Errorf("%s:%d: name, budget, why", file, n)
		}
		d, err := time.ParseDuration(fs[1])
		if err != nil {
			return nil, nil, fmt.Errorf("%s:%d: %w", file, n, err)
		}
		if _, twice := bs[fs[0]]; twice {
			return nil, nil, fmt.Errorf("%s:%d: %s twice", file, n, fs[0])
		}
		bs[fs[0]] = budget{d, strings.Join(fs[2:], " ")}
		order = append(order, fs[0])
	}
	return bs, order, sc.Err()
}

// readBench adds each benchmark line's ns/op to runs.
func readBench(file, board string, runs map[string][]time.Duration) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	pkg := ""
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if p, ok := strings.CutPrefix(line, "pkg: "); ok {
			pkg = path.Base(strings.TrimSpace(p))
			continue
		}
		fs := strings.Fields(line)
		if len(fs) < 4 || !strings.HasPrefix(fs[0], "Benchmark") || fs[3] != "ns/op" {
			continue
		}
		name := strings.TrimPrefix(fs[0], "Benchmark")
		if i := strings.LastIndexByte(name, '-'); i > 0 {
			if _, err := strconv.Atoi(name[i+1:]); err == nil {
				name = name[:i] // the -<GOMAXPROCS>
			}
		}
		ns, err := strconv.ParseFloat(fs[2], 64)
		if err != nil {
			return fmt.Errorf("%s: %q: %w", file, line, err)
		}
		key := board + "/" + pkg + "/" + name
		runs[key] = append(runs[key], time.Duration(ns))
	}
	return sc.Err()
}

func median(ds []time.Duration) time.Duration {
	s := slices.Sorted(slices.Values(ds))
	if len(s)%2 == 1 {
		return s[len(s)/2]
	}
	return (s[len(s)/2-1] + s[len(s)/2]) / 2
}

func round(d time.Duration) time.Duration {
	switch {
	case d > time.Second:
		return d.Round(10 * time.Millisecond)
	case d > time.Millisecond:
		return d.Round(10 * time.Microsecond)
	}
	return d
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "perfcheck:", err)
	os.Exit(2)
}
