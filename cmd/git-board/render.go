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
	"github.com/guilbep/git-board/internal/tui"
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

	// One column per field configured in .git-board.toml (sorted, same
	// order tui.ParseLabels/FieldNames already use), plus a trailing
	// "other" column for any label that doesn't match a configured field
	// — never silently dropped. This is the convention: a known field
	// always gets its own column with its display name, driven by
	// whatever a project's own config defines, not a hardcoded field
	// list — so it stays correct as fields are added/renamed/removed.
	fieldNames := tui.FieldNames(cfg)

	for _, bucket := range board.BucketBy(issues, group) {
		if group != "" {
			if md {
				fmt.Fprintf(out, "### %s\n", bucket.Group)
			} else {
				fmt.Fprintf(out, "== %s ==\n", bucket.Group)
			}
		}
		if md {
			writeMarkdownTable(out, bucket.Issues, cfg, fieldNames)
		} else {
			writeTable(out, bucket.Issues, cfg, fieldNames)
		}
	}
	return nil
}

// fieldColumns renders issue's known fields as fieldNames-ordered cell
// values (one per configured field, blank if unset) plus a joined "other"
// cell for any label that isn't a configured field at all.
func fieldColumns(issue gitbugio.Issue, cfg *config.Config, fieldNames []string) (cells []string, other string) {
	known, otherLabels := tui.ParseLabels(issue.Labels, cfg.Fields)
	byName := make(map[string][]string, len(known))
	for _, f := range known {
		byName[f.Name] = f.Values
	}
	cells = make([]string, len(fieldNames))
	for i, name := range fieldNames {
		cells[i] = formatFieldValues(name, byName[name], cfg)
	}
	return cells, strings.Join(otherLabels, ", ")
}

// formatFieldValues joins a field's values for display. A numeric field
// named "est" gets an "h" (hours) suffix per value — the one field this
// convention is known to apply to; a general numeric-field-implies-hours
// rule would be presumptuous for a field this codebase has never seen.
func formatFieldValues(name string, values []string, cfg *config.Config) string {
	if name == "est" && cfg.Fields[name].Numeric {
		suffixed := make([]string, len(values))
		for i, v := range values {
			suffixed[i] = v + "h"
		}
		return strings.Join(suffixed, ", ")
	}
	return strings.Join(values, ", ")
}

func writeTable(out io.Writer, issues []gitbugio.Issue, cfg *config.Config, fieldNames []string) {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	header := []string{"id", "title", "state"}
	for _, name := range fieldNames {
		header = append(header, tui.DisplayFieldName(name))
	}
	header = append(header, "other")
	fmt.Fprintln(tw, strings.Join(header, "\t"))

	for _, issue := range issues {
		cells, other := fieldColumns(issue, cfg, fieldNames)
		row := append([]string{shortID(issue.ID), issue.Title, issue.Status}, cells...)
		row = append(row, other)
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}
	tw.Flush()
}

func writeMarkdownTable(out io.Writer, issues []gitbugio.Issue, cfg *config.Config, fieldNames []string) {
	header := []string{"id", "title", "state"}
	for _, name := range fieldNames {
		header = append(header, tui.DisplayFieldName(name))
	}
	header = append(header, "other")
	fmt.Fprintf(out, "| %s |\n", strings.Join(header, " | "))
	fmt.Fprintf(out, "|%s|\n", strings.Repeat("---|", len(header)))

	for _, issue := range issues {
		cells, other := fieldColumns(issue, cfg, fieldNames)
		row := append([]string{shortID(issue.ID), issue.Title, issue.Status}, cells...)
		row = append(row, other)
		fmt.Fprintf(out, "| %s |\n", strings.Join(row, " | "))
	}
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}
