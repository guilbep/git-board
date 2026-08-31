package main

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"

	"github.com/guilbep/git-board/internal/gitbugio"
)

func fieldValue(issue gitbugio.Issue, fieldName string) string {
	prefix := fieldName + ":"
	for _, l := range issue.Labels {
		if strings.HasPrefix(l, prefix) {
			return strings.TrimPrefix(l, prefix)
		}
	}
	return ""
}

// render prints issues as a table, markdown, or JSON, optionally sorted and
// grouped. JSON output ignores grouping and prints the flat sorted list.
func render(out io.Writer, issues []gitbugio.Issue, group, sortSpec string, md, asJSON bool) error {
	if sortSpec != "" {
		key, direction, _ := strings.Cut(sortSpec, " ")
		desc := strings.TrimSpace(direction) == "desc"
		// ponytail: string sort — fine for label fields like "prio" (the
		// only sort a shipped view uses); wrong for numeric fields like
		// "est" ("16" < "2"). Add a numeric-field lookup from config if
		// that's ever sorted.
		sort.SliceStable(issues, func(i, j int) bool {
			vi, vj := fieldValue(issues[i], key), fieldValue(issues[j], key)
			if desc {
				return vi > vj
			}
			return vi < vj
		})
	}

	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(issues)
	}

	buckets, order := bucketBy(issues, group)
	for _, name := range order {
		if group != "" {
			if md {
				fmt.Fprintf(out, "### %s\n", name)
			} else {
				fmt.Fprintf(out, "== %s ==\n", name)
			}
		}
		if md {
			writeMarkdownTable(out, buckets[name])
		} else {
			writeTable(out, buckets[name])
		}
	}
	return nil
}

func bucketBy(issues []gitbugio.Issue, group string) (map[string][]gitbugio.Issue, []string) {
	buckets := map[string][]gitbugio.Issue{}
	var order []string
	for _, issue := range issues {
		key := ""
		if group != "" {
			key = fieldValue(issue, group)
			if key == "" {
				key = "(none)"
			}
		}
		if _, seen := buckets[key]; !seen {
			order = append(order, key)
		}
		buckets[key] = append(buckets[key], issue)
	}
	return buckets, order
}

func writeTable(out io.Writer, issues []gitbugio.Issue) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "id\ttitle\tstatus\tlabels")
	for _, issue := range issues {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", shortID(issue.ID), issue.Title, issue.Status, strings.Join(issue.Labels, ", "))
	}
	tw.Flush()
}

func writeMarkdownTable(out io.Writer, issues []gitbugio.Issue) {
	fmt.Fprintln(out, "| id | title | status | labels |")
	fmt.Fprintln(out, "|---|---|---|---|")
	for _, issue := range issues {
		fmt.Fprintf(out, "| %s | %s | %s | %s |\n", shortID(issue.ID), issue.Title, issue.Status, strings.Join(issue.Labels, ", "))
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
