package cli

import (
	"io"
	"strings"
	"text/tabwriter"
	"unicode/utf8"
)

// writeColumns prints rows as text/tabwriter does with a padding of 2
// spaces (minwidth 0, tabwidth 2, no flags), the last cell of each row
// left as it is: the same bytes, without tabwriter's cost on 20,000 rows.
// Rows of different lengths, or a cell holding a byte tabwriter reads
// apart (a tab, a line break, its escape) or a byte that isn't UTF-8, go
// through tabwriter itself.
func writeColumns(out io.Writer, rows [][]string) {
	n := 0
	if len(rows) > 0 {
		n = len(rows[0])
	}
	widths := make([]int, max(n-1, 0))
	for _, r := range rows {
		if len(r) != n {
			tabs(out, rows)
			return
		}
		for i, c := range r {
			if strings.ContainsAny(c, "\t\n\v\f") || !utf8.ValidString(c) {
				tabs(out, rows)
				return
			}
			if i < len(widths) {
				widths[i] = max(widths[i], utf8.RuneCountInString(c))
			}
		}
	}
	line := make([]byte, 0, 256)
	for _, r := range rows {
		line = line[:0]
		for i, c := range r {
			line = append(line, c...)
			if i < len(widths) {
				for pad := widths[i] - utf8.RuneCountInString(c) + 2; pad > 0; pad-- {
					line = append(line, ' ')
				}
			}
		}
		out.Write(append(line, '\n'))
	}
}

func tabs(out io.Writer, rows [][]string) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	for _, r := range rows {
		io.WriteString(tw, strings.Join(r, "\t")+"\n")
	}
	tw.Flush()
}
