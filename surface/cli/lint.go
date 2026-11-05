package cli

import (
	"context"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/forge"
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

--create-labels checks a different thing: whether every ns:value label
git-board's config implies (one per [fields.<name>] option, plus one per
[[sprints.list]] entry, and [labels] list's) exists as a real label on
each configured [repos] entry and [forges] table (else on the forges
origin names, GitHub or GitLab), through the forge's API with the forge
sync's token (GITHUB_TOKEN for github.com, GITLAB_TOKEN for GITLAB_HOST,
else git's credential helper for the host) — so they're visible and
colored in the forge's issues, not just inside git-board. A label whose
field lives elsewhere on that forge (a GitHub Project, the assignees, a
milestone) isn't made there. A missing label is created with its color
and description; an existing one is updated only where .git-board.toml
sets its color or description. Preview only by default; --apply carries
it out. It also reports the labels an issue template
(.github/ISSUE_TEMPLATE) applies that origin's GitHub repository doesn't
have: GitHub drops them silently. --apply doesn't create those: the fix
is in the template.`,
		Example: `  git-board lint
  git-board lint --strict            # exit 1 if anything is found, e.g. in CI
  git-board lint --create-labels     # preview missing and differing forge labels
  git-board lint --create-labels --apply   # create and update them`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if apply && !createLabels {
				return fmt.Errorf("--apply only applies together with --create-labels")
			}
			b, err := open(cmd, false)
			if err != nil {
				return err
			}
			defer b.Close()
			if createLabels {
				return runCreateLabels(cmd, b, apply)
			}
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
	cmd.Flags().BoolVar(&createLabels, "create-labels", false, "check .git-board.toml's implied labels against each configured repo's real forge labels")
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

// runCreateLabels checks each forge's real labels (app.LabelForges)
// against .git-board.toml's (app.PlanLabels) and, with apply, creates the
// missing ones and updates those the file colors or describes otherwise.
// v1's output and exit codes.
func runCreateLabels(cmd *cobra.Command, b *board, apply bool) error {
	ctx, out := cmd.Context(), cmd.OutOrStdout()
	targets, err := b.LabelForges(ctx, func(s string) { fmt.Fprintln(cmd.ErrOrStderr(), s) })
	if err != nil {
		return err
	}
	desired := app.DesiredLabels(b.Config.Schema)
	have := map[string][]forge.Label{} // by key, for the templates' check
	var failed bool
	for _, t := range targets {
		l, err := app.OpenLabeler(ctx, t.Table)
		var existing []forge.Label
		if err == nil {
			existing, err = l.Labels(ctx)
		}
		if err != nil {
			fmt.Fprintf(out, "%s: %v\n", t.Repo, err)
			failed = true
			continue
		}
		have[strings.ToLower(t.Key)] = existing
		changes := app.PlanLabels(t.Key, desired, existing, b.Config.Held)
		if len(changes) == 0 {
			fmt.Fprintf(out, "%s: all labels present\n", t.Repo)
			continue
		}
		for _, c := range changes {
			if !apply {
				fmt.Fprintf(out, "%s: %s\n", t.Repo, describeChange(c))
				continue
			}
			if err := l.SetLabel(ctx, c.Label, c.Update); err != nil {
				fmt.Fprintf(out, "%s: %s: %v\n", t.Repo, describeChange(c), err)
				failed = true
				continue
			}
			verb, rest, _ := strings.Cut(describeChange(c), " ")
			fmt.Fprintf(out, "%s: %s %s\n", t.Repo, map[string]string{"missing": "created", "update": "updated"}[verb], rest)
		}
	}
	checkTemplates(ctx, out, b, have)
	if !apply {
		fmt.Fprintln(out, "\ndry run — nothing written. Re-run with --apply to create and update these labels.")
	}
	if failed {
		return fmt.Errorf("one or more repos failed")
	}
	return nil
}

// forgeLabels are the forge project's labels, read-only.
func forgeLabels(ctx context.Context, t forge.Table) ([]forge.Label, error) {
	l, err := app.OpenLabeler(ctx, t)
	if err != nil {
		return nil, err
	}
	return l.Labels(ctx)
}

// describeChange is c in a line: missing feat #0e8a16 "A new feature", or
// update bug: color d73a4a → b60205.
func describeChange(c app.LabelChange) string {
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

// checkTemplates reports each issue template's labels that origin's
// GitHub repository doesn't have (names ignore case, as GitHub's do),
// its labels read already in have when it is one of lint's forges. It
// warns, never fails: when it can't tell (origin isn't on GitHub, a
// template doesn't parse), the labels may all exist, and CI must not
// fail.
func checkTemplates(ctx context.Context, out io.Writer, b *board, have map[string][]forge.Label) {
	templates, err := app.IssueTemplates(b.IssueTemplatesDir())
	if err == nil && len(templates) == 0 {
		return
	}
	var repo forge.Table
	var existing []forge.Label
	if err == nil {
		repo, err = b.OriginGitHub(ctx)
	}
	if err == nil {
		var ok bool
		if existing, ok = have[strings.ToLower(repo.Host+"/"+repo.Project)]; !ok {
			existing, err = forgeLabels(ctx, repo)
		}
	}
	if err != nil {
		fmt.Fprintf(out, "issue templates: not checked: %v\n", err)
		return
	}
	for _, t := range templates {
		missing := slices.DeleteFunc(slices.Clone(t.Labels), func(l string) bool {
			return slices.ContainsFunc(existing, func(e forge.Label) bool { return strings.EqualFold(e.Name, l) })
		})
		if len(missing) > 0 {
			fmt.Fprintf(out, "issue templates: %s applies %s, which %s doesn't have: GitHub drops them\n", t.File, strings.Join(missing, ", "), repo.Project)
		}
	}
}
