package main

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghbridge"
	"github.com/guilbep/git-board/internal/store"
)

// Seams over the gh calls, for tests.
var (
	fetchProjectMeta                        = ghbridge.FetchProjectMeta
	projectWriter    ghbridge.ProjectWriter = ghbridge.GhWriter{}
	fetchAssignable                         = ghbridge.FetchAssignable
)

func newBridgeSyncCmd() *cobra.Command {
	var apply bool
	var limit int
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync fields both ways between the GitHub Project and labels",
		Long: `Sync fields both ways between the GitHub Project of .git-board.toml and
the labels: status, prio, mod, est and sprint.

Each field is compared with the value both sides last agreed on, which
git-board records on the issue: only the side that changed since is
taken. When both changed differently the field is a conflict, left alone
on both sides until "git-board bridge resolve". Assignees merge person
by person: changes on GitHub reach the labels, changes in git-board reach
the GitHub issue (with write = true; someone GitHub can't assign stays in
git-board, with a warning).

A dry run by default. Writing to the Project needs, in .git-board.toml:

  [github-project]
  write = true

and a gh token with the project scope.`,
		Example: `  git-board bridge sync            # what would change, on each side
  git-board bridge sync --apply`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBridgeSync(cmd.OutOrStdout(), limit, apply)
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "write the changes (the default is a dry run)")
	cmd.Flags().IntVar(&limit, "limit", defaultBridgePullLimit, "max Project items to read")
	return cmd
}

func newBridgeResolveCmd() *cobra.Command {
	var ours, theirs bool
	cmd := &cobra.Command{
		Use:   "resolve <issue number> <field>",
		Short: "Resolve a conflict by keeping one side",
		Long: `Resolve a conflict "git-board bridge sync" held, by keeping git-board's
value (--ours, written to the Project) or the Project's (--theirs, set on
the label). Both then agree on it.`,
		Example: `  git-board bridge resolve 2605 status --theirs`,
		Args:    cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if ours == theirs {
				return fmt.Errorf("pass one of --ours or --theirs")
			}
			number, err := strconv.Atoi(args[0])
			if err != nil {
				return fmt.Errorf("issue number %q: %w", args[0], err)
			}
			return runBridgeResolve(cmd.OutOrStdout(), number, args[1], ours)
		},
	}
	cmd.Flags().BoolVar(&ours, "ours", false, "keep git-board's value, written to the Project")
	cmd.Flags().BoolVar(&theirs, "theirs", false, "keep the Project's value, set on the label")
	return cmd
}

// project is the GitHub Project through the seams above.
func project() ghbridge.Project {
	return ghbridge.Project{Items: fetchGithubItems, Meta: fetchProjectMeta, Writer: projectWriter, Assignable: fetchAssignable}
}

// openSync loads the config, opens the repository and reads both sides.
func openSync(limit int) (*config.Config, *store.Backend, ghbridge.State, error) {
	cfg, err := loadConfig()
	if err != nil {
		return nil, nil, ghbridge.State{}, err
	}
	b, err := store.OpenCwd()
	if err != nil {
		return nil, nil, ghbridge.State{}, err
	}
	s, err := ghbridge.LoadState(b, project(), cfg, limit)
	if err != nil {
		_ = b.Close()
		return nil, nil, ghbridge.State{}, err
	}
	return cfg, b, s, nil
}

func runBridgeSync(out io.Writer, limit int, apply bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()
	if err := projectRun(out, cfg, b, limit, "both", apply); err != nil || apply {
		return err
	}
	fmt.Fprintln(out, "\ndry run — nothing written. Re-run with --apply to write these changes.")
	return nil
}

// projectRun reads both sides and prints the plan, restricted to one side
// ("labels", "project") or not ("both"); with apply, writes it, to the
// Project with write = true alone. git-board sync's Project step is its
// "both".
func projectRun(out io.Writer, cfg *config.Config, b *store.Backend, limit int, side string, apply bool) error {
	s, err := ghbridge.LoadState(b, project(), cfg, limit)
	if err != nil {
		return err
	}
	plan := s.Plan(cfg)
	switch side {
	case "labels":
		plan = plan.LabelsOnly()
	case "project":
		plan = plan.ProjectOnly()
	}
	printSyncPlan(out, plan)

	if !apply {
		if len(plan.Edits) > 0 && !cfg.GithubProject.Write {
			fmt.Fprintln(out, "Writing to the Project also needs write = true under [github-project] in .git-board.toml.")
		}
		return nil
	}
	skipped, err := ghbridge.ApplySync(b, projectWriter, b, plan, cfg, s.Meta)
	if err != nil {
		return err
	}
	fmt.Fprintln(out, "\napplied.")
	if skipped > 0 {
		fmt.Fprintf(out, "%d change(s) not written to the Project: set write = true under [github-project] in .git-board.toml.\n", skipped)
	}
	if len(plan.Conflicts) > 0 {
		fmt.Fprintf(out, "%d conflict(s) held: git-board bridge resolve <issue number> <field> --ours | --theirs\n", len(plan.Conflicts))
	}
	return nil
}

func runBridgeResolve(out io.Writer, number int, field string, ours bool) error {
	cfg, b, s, err := openSync(ghbridge.DefaultItemLimit)
	if err != nil {
		return err
	}
	defer b.Close()
	kept, err := ghbridge.ResolveConflict(b, project(), cfg, s, number, field, ours)
	if errors.Is(err, ghbridge.ErrNotConflict) {
		return fmt.Errorf("%w: run `git-board bridge sync` to see the conflicts", err)
	}
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "#%d %s: kept %s on both sides\n", number, field, show(kept))
	return nil
}

// printSyncPlan renders a sync plan: per issue, label changes, Project
// writes, conflicts and warnings, then a summary.
func printSyncPlan(out io.Writer, plan ghbridge.SyncPlan) {
	edits := map[string][]ghbridge.ProjectEdit{}
	for _, e := range plan.Edits {
		edits[e.BugID] = append(edits[e.BugID], e)
	}
	conflicts := map[string][]ghbridge.Conflict{}
	for _, c := range plan.Conflicts {
		conflicts[c.BugID] = append(conflicts[c.BugID], c)
	}

	labelChanges, warnings := 0, 0
	for _, c := range plan.Changes {
		es, cs := edits[c.BugID], conflicts[c.BugID]
		if len(c.Added)+len(c.Removed)+len(c.Warnings)+len(es)+len(cs) == 0 {
			continue
		}
		fmt.Fprintf(out, "#%d %s\n", c.Number, c.Title)
		for _, l := range c.Added {
			fmt.Fprintf(out, "  + %s\n", l)
		}
		for _, l := range c.Removed {
			fmt.Fprintf(out, "  - %s\n", l)
		}
		for _, e := range es {
			if e.Kind == ghbridge.EditAssignees {
				fmt.Fprintf(out, "  -> GitHub issue: assignee%s%s\n", signed("+", e.Add), signed("-", e.Remove))
				continue
			}
			fmt.Fprintf(out, "  -> Project: %s = %s\n", e.Field, show(e.Value))
		}
		for _, k := range cs {
			agreed := "never agreed"
			if k.HasBase {
				agreed = "last agreed: " + show(k.Base)
			}
			fmt.Fprintf(out, "  ! conflict on %s: git-board %s, Project %s (%s)\n", k.Field, show(k.Local), show(k.Project), agreed)
		}
		for _, w := range c.Warnings {
			fmt.Fprintf(out, "  ! %s\n", w)
		}
		if len(c.Added)+len(c.Removed) > 0 {
			labelChanges++
		}
		warnings += len(c.Warnings)
	}
	for _, u := range plan.Unmatched {
		fmt.Fprintf(out, "unmatched #%d %s: %s\n", u.Number, u.Title, u.Reason)
	}
	fmt.Fprintf(out, "\nsummary: %d issue(s) matched, %d with label changes, %d Project write(s), %d conflict(s), %d warning(s), %d unmatched\n",
		len(plan.Changes), labelChanges, len(plan.Edits), len(plan.Conflicts), warnings, len(plan.Unmatched))
}

// signed is " +alice +bob" for sign "+".
func signed(sign string, logins []string) string {
	var b strings.Builder
	for _, l := range logins {
		b.WriteString(" " + sign + l)
	}
	return b.String()
}

func show(value string) string {
	if value == "" {
		return "(empty)"
	}
	return value
}
