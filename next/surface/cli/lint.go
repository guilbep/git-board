package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/app"
	"github.com/guilbep/git-board/next/domain/issues"
)

// git board lint, as v1's.

func newLint() *cobra.Command {
	var strict, createLabels, apply bool
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Report label/config drift, read-only",
		Long: `Report drift between issue labels and .git-board.toml, read-only —
writes nothing, unlike everything else in this CLI. Names the keys of
.git-board.toml git-board doesn't read (a misspelled one is otherwise
its zero value; every command warns of them on stderr too), then checks
every open issue (is:open) in the current repository: double singles (more than
one label for a single-cardinality field), unknown option values,
missing values on a configured single-cardinality field, orphan sprint
labels (no matching [[sprints.list]] entry), and configured options no
issue in the repo actually carries. With [branches] kinds: an issue with
two kinds, a title whose [PREFIX] names another kind than its label (the
label wins), and how many have no kind. With the cycle on, it first says
how [cycle]'s keys were read.

Exits 0 even with findings — this is a report, not a build gate — unless
--strict is given.

--create-labels checks whether every ns:value label git-board's config
implies exists as a real GitHub label on each configured repository: not
yet in this version.`,
		Example: `  git-board lint
  git-board lint --strict            # exit 1 if anything is found, e.g. in CI`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if apply && !createLabels {
				return fmt.Errorf("--apply only applies together with --create-labels")
			}
			if createLabels {
				return errors.New("lint --create-labels isn't in this version of git-board yet: use git-board v0.11")
			}
			b, err := open(cmd, false)
			if err != nil {
				return err
			}
			defer b.Close()
			all, err := b.issues.List(cmd.Context(), issues.ListReq{})
			if err != nil {
				return err
			}
			if reads := app.LintReads(b.Config); len(reads) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "read:\n  %s\n", strings.Join(reads, "\n  "))
			}
			findings := app.Lint(all, b.Config)
			printLint(cmd.OutOrStdout(), findings)
			if strict && len(findings) > 0 {
				return fmt.Errorf("%d lint finding(s)", len(findings))
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "exit 1 if any finding is reported")
	cmd.Flags().BoolVar(&createLabels, "create-labels", false, "check .git-board.toml's implied labels against each configured repo's real GitHub labels")
	cmd.Flags().BoolVar(&apply, "apply", false, "with --create-labels, create and update the labels (default is preview only)")
	return cmd
}

// printLint groups findings by check, in app.LintOrder, each group then
// a summary line.
func printLint(out io.Writer, findings []app.LintFinding) {
	byCheck := make(map[string][]app.LintFinding, len(app.LintOrder))
	for _, f := range findings {
		byCheck[f.Check] = append(byCheck[f.Check], f)
	}
	counts := make([]string, 0, len(app.LintOrder))
	for _, check := range app.LintOrder {
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
