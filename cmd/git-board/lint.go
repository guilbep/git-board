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

// existingGithubLabels/applyGithubLabel are seams over ghlabels.Existing/
// ghlabels.Apply so tests can swap in a fixture instead of shelling out to
// the real `gh` binary — same pattern as bridge.go's fetchGithubItems.
var (
	existingGithubLabels = ghlabels.Existing
	applyGithubLabel     = ghlabels.Apply
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
and colored in the GitHub Issues UI, not just inside git-bug's own
label store. A missing label is created with its color and description;
an existing one is updated (--force) only where .git-board.toml sets its
color or description. Preview only by default; --apply carries it out. It
also reports the labels an issue template (.github/ISSUE_TEMPLATE) applies
that this checkout's GitHub repository doesn't have: GitHub drops them
silently. --apply doesn't create those: the fix is in the template.`,
		Example: `  git-board lint
  git-board lint --strict            # exit 1 if anything is found, e.g. in CI
  git-board lint --create-labels     # preview missing and differing GitHub labels
  git-board lint --create-labels --apply   # create and update them`,
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
	cmd.Flags().BoolVar(&apply, "apply", false, "with --create-labels, create and update the labels (default is preview only)")
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
// .git-board.toml's (internal/ghlabels.Plan) and, with apply, creates the
// missing ones and updates those the file colors or describes otherwise.
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
		changes := ghlabels.Plan(repo, desired, existing)
		if len(changes) == 0 {
			fmt.Fprintf(out, "%s: all labels present\n", repo)
			continue
		}
		for _, c := range changes {
			if !apply {
				fmt.Fprintf(out, "%s: %s\n", repo, describeChange(c))
				continue
			}
			if err := applyGithubLabel(c); err != nil {
				fmt.Fprintf(out, "%s: %s: %v\n", repo, describeChange(c), err)
				failed = true
				continue
			}
			verb, rest, _ := strings.Cut(describeChange(c), " ")
			fmt.Fprintf(out, "%s: %s %s\n", repo, map[string]string{"missing": "created", "update": "updated"}[verb], rest)
		}
	}
	checkTemplates(out)
	if !apply {
		fmt.Fprintln(out, "\ndry run — nothing written. Re-run with --apply to create and update these labels.")
	}
	if failed {
		return fmt.Errorf("one or more repos failed")
	}
	return nil
}

// describeChange is c in a line: missing feat #0e8a16 "A new feature", or
// update bug: color d73a4a → b60205.
func describeChange(c ghlabels.Change) string {
	if c.Was == nil {
		s := "missing " + c.Name
		if c.Color != "" {
			s += " #" + c.Color
		}
		if c.Description != "" {
			s += fmt.Sprintf(" %q", c.Description)
		}
		return s
	}
	var what []string
	if c.Color != c.Was.Color {
		what = append(what, fmt.Sprintf("color %s → %s", c.Was.Color, c.Color))
	}
	if c.Description != c.Was.Description {
		what = append(what, fmt.Sprintf("description %q → %q", c.Was.Description, c.Description))
	}
	return fmt.Sprintf("update %s: %s", c.Name, strings.Join(what, ", "))
}

// checkTemplates reports each issue template's labels that the checkout's
// GitHub repository doesn't have (names ignore case, as GitHub's do). It
// warns, never fails: when it can't tell (gh doesn't know the repository, a
// template doesn't parse), the labels may all exist, and CI must not fail.
func checkTemplates(out io.Writer) {
	templates, err := ghlabels.Templates(filepath.Join(filepath.Dir(config.Path()), ".github", "ISSUE_TEMPLATE"))
	if err == nil && len(templates) == 0 {
		return
	}
	var repo string
	var existing []ghlabels.Label
	if err == nil {
		repo, err = currentGithubRepo()
	}
	if err == nil {
		existing, err = existingGithubLabels(repo)
	}
	if err != nil {
		fmt.Fprintf(out, "issue templates: not checked: %v\n", err)
		return
	}
	for _, t := range templates {
		missing := slices.DeleteFunc(slices.Clone(t.Labels), func(l string) bool {
			return slices.ContainsFunc(existing, func(e ghlabels.Label) bool { return strings.EqualFold(e.Name, l) })
		})
		if len(missing) > 0 {
			fmt.Fprintf(out, "issue templates: %s applies %s, which %s doesn't have: GitHub drops them\n", t.File, strings.Join(missing, ", "), repo)
		}
	}
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
