package conf

import (
	"fmt"
	"strings"
)

// Diff is b against a as diff -u writes it, three lines of context, a
// and b named in its header: what an upgrade or a downgrade --dry-run
// would write. "" when they are the same.
func Diff(nameA, nameB, a, b string) string {
	if a == b {
		return ""
	}
	x, y := diffLines(a), diffLines(b)
	// lcs[i][j]: the longest common subsequence of x[i:] and y[j:]
	lcs := make([][]int, len(x)+1)
	for i := range lcs {
		lcs[i] = make([]int, len(y)+1)
	}
	for i := len(x) - 1; i >= 0; i-- {
		for j := len(y) - 1; j >= 0; j-- {
			if x[i] == y[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else {
				lcs[i][j] = max(lcs[i+1][j], lcs[i][j+1])
			}
		}
	}
	type op struct {
		kind byte // ' ', '-', '+'
		line string
		a, b int // the lines of x and y before it
	}
	var ops []op
	i, j := 0, 0
	for i < len(x) || j < len(y) {
		switch {
		case i < len(x) && j < len(y) && x[i] == y[j]:
			ops = append(ops, op{' ', x[i], i, j})
			i, j = i+1, j+1
		case i < len(x) && (j == len(y) || lcs[i+1][j] >= lcs[i][j+1]):
			ops = append(ops, op{'-', x[i], i, j}) // diff -u's order: what goes, then what comes
			i++
		default:
			ops = append(ops, op{'+', y[j], i, j})
			j++
		}
	}
	const context = 3
	var out strings.Builder
	fmt.Fprintf(&out, "--- %s\n+++ %s\n", nameA, nameB)
	for k := 0; k < len(ops); {
		for k < len(ops) && ops[k].kind == ' ' {
			k++
		}
		if k == len(ops) {
			break
		}
		last := k // the hunk's last change
		for n := k; n < len(ops) && n-last <= 2*context+1; n++ {
			if ops[n].kind != ' ' {
				last = n
			}
		}
		from, to := max(0, k-context), min(len(ops), last+context+1)
		var na, nb int
		for _, o := range ops[from:to] {
			if o.kind != '+' {
				na++
			}
			if o.kind != '-' {
				nb++
			}
		}
		fmt.Fprintf(&out, "@@ -%s +%s @@\n", hunkRange(ops[from].a, na), hunkRange(ops[from].b, nb))
		for _, o := range ops[from:to] {
			out.WriteByte(o.kind)
			out.WriteString(o.line)
			out.WriteByte('\n')
		}
		k = to
	}
	return out.String()
}

// hunkRange is a hunk's start and length as diff -u writes them: an
// empty one starts at the line before.
func hunkRange(before, n int) string {
	start := before + 1
	if n == 0 {
		start = before
	}
	if n == 1 {
		return fmt.Sprint(start)
	}
	return fmt.Sprintf("%d,%d", start, n)
}

func diffLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
