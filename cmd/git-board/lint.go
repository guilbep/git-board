package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/lint"
)

func newLintCmd() *cobra.Command {
	var strict bool

	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Report label/config drift, read-only",
		Long: `Report drift between issue labels and .git-board.toml, read-only —
writes nothing, unlike everything else in this CLI. Checks every open
issue (is:open) in the current git-bug repo: double singles (more than
one label for a single-cardinality field), unknown option values,
missing values on a configured single-cardinality field, orphan sprint
labels (no matching [[sprints.list]] entry), and configured options no
issue in the repo actually carries.

Exits 0 even with findings — this is a report, not a build gate — unless
--strict is given.`,
		Example: `  git-board lint
  git-board lint --strict   # exit 1 if anything is found, e.g. in CI`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLint(cmd.OutOrStdout(), strict)
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "exit 1 if any finding is reported")
	return cmd
}

func runLint(out io.Writer, strict bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	b, err := gitbugio.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()

	issues, err := b.ListIssues()
	if err != nil {
		return err
	}

	findings := lint.Run(issues, cfg)
	printLintFindings(out, findings)

	if strict && len(findings) > 0 {
		return fmt.Errorf("%d lint finding(s)", len(findings))
	}
	return nil
}

// printLintFindings groups findings by check (in lint.CheckOrder, so the
// rarer/higher-signal checks print first rather than being buried under a
// potentially large missing-value count) and prints a per-check summary
// line.
func printLintFindings(out io.Writer, findings []lint.Finding) {
	byCheck := make(map[string][]lint.Finding, len(lint.CheckOrder))
	for _, f := range findings {
		byCheck[f.Check] = append(byCheck[f.Check], f)
	}

	counts := make([]string, 0, len(lint.CheckOrder))
	for _, check := range lint.CheckOrder {
		group := byCheck[check]
		if len(group) == 0 {
			continue
		}
		counts = append(counts, fmt.Sprintf("%s: %d", check, len(group)))
		fmt.Fprintf(out, "%s:\n", check)
		for _, f := range group {
			if f.IssueID == "" {
				fmt.Fprintf(out, "  %s\n", f.Message)
				continue
			}
			fmt.Fprintf(out, "  %s %s: %s\n", shortID(f.IssueID), f.Title, f.Message)
		}
	}

	fmt.Fprintf(out, "\nsummary: %d finding(s)", len(findings))
	if len(counts) > 0 {
		fmt.Fprintf(out, " (%s)", strings.Join(counts, ", "))
	}
	fmt.Fprintln(out)
}
