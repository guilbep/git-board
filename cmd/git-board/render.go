package main

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// render prints issues as a table, markdown, or JSON, optionally sorted and
// grouped. JSON output ignores grouping and prints the flat sorted list.
func render(out io.Writer, issues []gitbugio.Issue, group, sortSpec string, md, asJSON bool, cfg *config.Config) error {
	board.SortByField(issues, sortSpec, cfg)

	if asJSON {
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		return enc.Encode(issues)
	}

	for _, bucket := range board.BucketBy(issues, group) {
		if group != "" {
			if md {
				fmt.Fprintf(out, "### %s\n", bucket.Group)
			} else {
				fmt.Fprintf(out, "== %s ==\n", bucket.Group)
			}
		}
		if md {
			writeMarkdownTable(out, bucket.Issues)
		} else {
			writeTable(out, bucket.Issues)
		}
	}
	return nil
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
