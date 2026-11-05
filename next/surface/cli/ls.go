package cli

import (
	"fmt"

	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/domain/issues"
)

func newLs() *cobra.Command {
	var group, sort, me string
	var md bool
	var limit int
	var js *jsonOut
	cmd := &cobra.Command{
		Use:   "ls [filter]",
		Short: "List issues matching a filter",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			req := issues.ListReq{Filter: issues.DefaultFilter, Me: me}
			if len(args) > 0 {
				req.Filter = args[0]
			}
			if req.Me == "" {
				req.Me = os.Getenv("GIT_BOARD_ME")
			}
			if limit < 0 {
				return usageError{fmt.Errorf("--limit %d: a count, 0 for all", limit)}
			}
			ctx := cmd.Context()
			b, err := open(ctx, needsMe(req.Filter))
			if err != nil {
				return err
			}
			defer b.Close()
			list, err := b.issues.List(ctx, req)
			if err != nil {
				return err
			}
			return render(cmd.OutOrStdout(), list, group, sort, limit, md, js, b.issues.Schema)
		},
	}
	cmd.Flags().StringVar(&group, "group", "", "group results by field")
	cmd.Flags().StringVar(&sort, "sort", "", `sort by created, updated, title or a field, then asc or desc: "prio asc", "updated desc"`)
	cmd.Flags().IntVarP(&limit, "limit", "L", 0, "list this many at most, after sorting (0: all)")
	cmd.Flags().BoolVar(&md, "md", false, "render as markdown")
	cmd.Flags().StringVar(&me, "me", "", "current user, for @me filters/values (env GIT_BOARD_ME)")
	js = addJSON(cmd, "render as JSON", jsonFields(issues.Issue{}), true)
	return cmd
}

// needsMe says a filter reads who writes: @me, involves:, follows:.
func needsMe(filter string) bool {
	return strings.Contains(filter, "@me") || strings.Contains(filter, "involves:") || strings.Contains(filter, "follows:")
}

// render prints issues as a table, Markdown or JSON, sorted, cut and
// grouped. JSON is the flat sorted list.
func render(out io.Writer, list []issues.Issue, group, sortSpec string, limit int, md bool, js *jsonOut, s *issues.Schema) error {
	if err := s.Sort(list, sortSpec); err != nil {
		return err
	}
	if limit > 0 && len(list) > limit {
		list = list[:limit]
	}
	if js.on() {
		return js.write(out, list)
	}
	// a column per field, the configured ones and every namespace on the
	// issues, then "other" for the plain labels: never one dropped
	names := s.FieldNames(list)
	for _, bucket := range issues.BucketBy(list, group) {
		if group != "" {
			if md {
				fmt.Fprintf(out, "### %s\n", bucket.Group)
			} else {
				fmt.Fprintf(out, "== %s ==\n", bucket.Group)
			}
		}
		if md {
			writeMarkdownTable(out, bucket.Issues, s, names)
		} else {
			writeTable(out, bucket.Issues, s, names)
		}
	}
	return nil
}

// columns are an issue's fields in names' order, blank where unset, each
// field's values joined in its labels' order, and its plain labels
// joined: what v1's table shows, read in one pass over the labels.
func columns(is issues.Issue, s *issues.Schema, names []string, at map[string]int) (cells []string, other string) {
	cells = make([]string, len(names))
	set := make([]bool, len(names))
	var plain []string
	for _, l := range is.Labels {
		ns, val, ok := strings.Cut(l, ":")
		if !ok {
			plain = append(plain, l)
			continue
		}
		i, known := at[ns]
		if !known {
			continue
		}
		v := s.Format(ns, []string{val})
		if set[i] {
			v = cells[i] + ", " + v
		}
		cells[i], set[i] = v, true
	}
	return cells, strings.Join(plain, ", ")
}

// indexOf is each name's place.
func indexOf(names []string) map[string]int {
	at := make(map[string]int, len(names))
	for i, n := range names {
		at[n] = i
	}
	return at
}

func header(names []string) []string {
	h := []string{"id", "title", "state"}
	for _, n := range names {
		h = append(h, issues.DisplayName(n))
	}
	return append(h, "other")
}

func writeTable(out io.Writer, list []issues.Issue, s *issues.Schema, names []string) {
	rows := make([][]string, 0, len(list)+1)
	rows = append(rows, header(names))
	at := indexOf(names)
	for _, is := range list {
		cells, other := columns(is, s, names, at)
		row := make([]string, 0, len(cells)+4)
		row = append(append(row, shortID(is.ID), is.Headline(), is.Status), cells...)
		rows = append(rows, append(row, other))
	}
	writeColumns(out, rows)
}

func writeMarkdownTable(out io.Writer, list []issues.Issue, s *issues.Schema, names []string) {
	h := header(names)
	fmt.Fprintf(out, "| %s |\n", strings.Join(h, " | "))
	fmt.Fprintf(out, "|%s\n", strings.Repeat("---|", len(h)))
	at := indexOf(names)
	for _, is := range list {
		cells, other := columns(is, s, names, at)
		row := append([]string{shortID(is.ID), is.Headline(), is.Status}, cells...)
		fmt.Fprintf(out, "| %s |\n", strings.Join(append(row, other), " | "))
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
