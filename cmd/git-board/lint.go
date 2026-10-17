package main

import (
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghlabels"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/lint"
)

// existingGithubLabels/createGithubLabel are seams over ghlabels.Existing/
// ghlabels.Create so tests can swap in a fixture instead of shelling out to
// the real `gh` binary — same pattern as bridge.go's fetchGithubItems.
var (
	existingGithubLabels = ghlabels.Existing
	createGithubLabel    = ghlabels.Create
	currentGithubRepo    = ghlabels.Current
)

func newLintCmd() *cobra.Command {
	var strict, createLabels, apply bool

	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Report label/config drift, read-only",
		Long: `Report drift between issue labels and .git-board.toml, read-only —
writes nothing, unlike everything else in this CLI. Checks every open
issue (is:open) in the current git-bug repo: double singles (more than
one label for a single-cardinality field), unknown option values,
missing values on a configured single-cardinality field, orphan sprint
labels (no matching [[sprints.list]] entry), and configured options no
issue in the repo actually carries. With [branches] kinds: an issue with
two kinds, a title whose [PREFIX] names another kind than its label (the
label wins), and how many have no kind.

Exits 0 even with findings — this is a report, not a build gate — unless
--strict is given.

--create-labels checks a different thing: whether every ns:value label
git-board's config implies (one per [fields.<name>] option, plus one per
[[sprints.list]] entry) exists as a real GitHub label on each configured
[repos] entry, via "gh label list"/"gh label create" — so they're visible
and colorable in the GitHub Issues UI, not just inside git-bug's own
label store. Preview only by default; --apply actually creates them. It
also reports the labels an issue template (.github/ISSUE_TEMPLATE) applies
that this checkout's GitHub repository doesn't have: GitHub drops them
silently. --apply doesn't create those: the fix is in the template.`,
		Example: `  git-board lint
  git-board lint --strict            # exit 1 if anything is found, e.g. in CI
  git-board lint --create-labels     # preview missing GitHub labels
  git-board lint --create-labels --apply   # actually create them`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if apply && !createLabels {
				return fmt.Errorf("--apply only applies together with --create-labels")
			}
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if createLabels {
				return runCreateLabels(cmd.OutOrStdout(), cfg, apply)
			}
			return runLint(cmd.OutOrStdout(), cfg, strict)
		},
	}
	cmd.Flags().BoolVar(&strict, "strict", false, "exit 1 if any finding is reported")
	cmd.Flags().BoolVar(&createLabels, "create-labels", false, "check .git-board.toml's implied labels against each configured repo's real GitHub labels")
	cmd.Flags().BoolVar(&apply, "apply", false, "with --create-labels, actually create the missing labels (default is preview only)")
	return cmd
}

func runLint(out io.Writer, cfg *config.Config, strict bool) error {
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

// runCreateLabels checks each [repos] entry's real GitHub labels against
// .git-board.toml's implied set (internal/ghlabels.Desired) and, with
// apply, creates whatever's missing via "gh label create".
func runCreateLabels(out io.Writer, cfg *config.Config, apply bool) error {
	if len(cfg.Repos) == 0 {
		return fmt.Errorf("no [repos] configured in .git-board.toml")
	}
	desired := ghlabels.Desired(cfg)
	var failed bool
	for _, repo := range cfg.Repos {
		existing, err := existingGithubLabels(repo)
		if err != nil {
			fmt.Fprintf(out, "%s: %v\n", repo, err)
			failed = true
			continue
		}
		missing := ghlabels.MissingFrom(desired, existing)
		if len(missing) == 0 {
			fmt.Fprintf(out, "%s: all labels present\n", repo)
			continue
		}
		for _, name := range missing {
			if !apply {
				fmt.Fprintf(out, "%s: missing %s\n", repo, name)
				continue
			}
			if err := createGithubLabel(repo, name); err != nil {
				fmt.Fprintf(out, "%s: create %s: %v\n", repo, name, err)
				failed = true
				continue
			}
			fmt.Fprintf(out, "%s: created %s\n", repo, name)
		}
	}
	if !checkTemplates(out) {
		failed = true
	}
	if !apply {
		fmt.Fprintln(out, "\ndry run — nothing written. Re-run with --apply to create these labels.")
	}
	if failed {
		return fmt.Errorf("one or more repos failed")
	}
	return nil
}

// checkTemplates reports each issue template's labels that the checkout's
// GitHub repository doesn't have (names ignore case, as GitHub's do); false
// when it couldn't tell.
func checkTemplates(out io.Writer) bool {
	templates, err := ghlabels.Templates(filepath.Join(filepath.Dir(config.Path()), ".github", "ISSUE_TEMPLATE"))
	if err == nil && len(templates) == 0 {
		return true
	}
	var repo string
	var existing []string
	if err == nil {
		repo, err = currentGithubRepo()
	}
	if err == nil {
		existing, err = existingGithubLabels(repo)
	}
	if err != nil {
		fmt.Fprintf(out, "issue templates: %v\n", err)
		return false
	}
	for _, t := range templates {
		missing := slices.DeleteFunc(slices.Clone(t.Labels), func(l string) bool {
			return slices.ContainsFunc(existing, func(e string) bool { return strings.EqualFold(e, l) })
		})
		if len(missing) > 0 {
			fmt.Fprintf(out, "issue templates: %s applies %s, which %s doesn't have: GitHub drops them\n", t.File, strings.Join(missing, ", "), repo)
		}
	}
	return true
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
