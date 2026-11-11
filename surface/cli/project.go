package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/surface/tui"
	"github.com/guilbep/git-board/tools/ghproject"
)

// The GitHub Project alone: its fields, matched to the labels, which git
// board sync moves after the forges (tools/ghproject), as v1's git
// board project.

func newProject() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "The GitHub Project alone: pull, push or sync its fields with the labels",
		Long: `The GitHub Project of .git-board.toml alone: its fields (status, prio,
mod, est, sprint, moscow, milestone, the assignees, and any field a
mapping sends to the Project) matched to the labels.

Each field is compared with the value both sides last agreed on, which
git-board records on the issue: only the side that changed since is
taken. A field changed on both sides is a conflict, left alone on both
until git board project resolve, and listed on the Conflicts page.
Writing to the Project needs write = true under [github-project], and a
token with the project scope: the GitHub forge's, GITHUB_TOKEN or git's
credential helper's.`,
		Example: `  git board project configure --owner EPFL-ENAC --project 40
  git board project pull --check
  git board project sync`,
	}
	cmd.AddCommand(
		newProjectWay("pull", ghproject.Labels, "Take the Project's changes into the labels, and write nothing there",
			`Take the Project's changes into the labels: the fields changed there since
both sides agreed. Nothing is written to the Project: a field changed in
git-board waits for git board project push.`),
		newProjectWay("push", ghproject.OnGH, "Write git-board's changes to the Project, and take nothing",
			`Write git-board's changes to the Project: the fields changed in git-board
since both sides agreed, with write = true under [github-project]. No label
changes: a field changed on the Project waits for git board project pull.`),
		newProjectWay("sync", ghproject.Both, "Take the Project's changes, and write git-board's",
			`Both ways, as git board sync's Project step: the Project's changes reach
the labels, git-board's reach the Project.`),
		newProjectResolve(), newProjectConfigure(), newProjectFit(),
	)
	return cmd
}

func newProjectWay(use string, side ghproject.Side, short, long string) *cobra.Command {
	var check bool
	var limit int
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			r, err := b.ProjectRun(cmd.Context(), side, !check, limit)
			if err != nil {
				return err
			}
			printReport(out, r)
			if check {
				fmt.Fprintln(out, "checked: nothing written")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "the dry run: say what it would change, write nothing")
	cmd.Flags().IntVar(&limit, "limit", ghproject.DefaultLimit, "max Project items to read")
	return cmd
}

func newProjectResolve() *cobra.Command {
	var ours, theirs bool
	cmd := &cobra.Command{
		Use:   "resolve <issue> <field>",
		Short: "Resolve a conflict by keeping one side",
		Long: `Resolve a conflict "git board project sync" held, by keeping git-board's
value (--ours, written to the Project) or the Project's (--theirs, set on
the label). Both then agree on it. The issue is its number on GitHub, or
owner/repo#number (or its URL) when two repositories of the Project have
that number in conflict.`,
		Example: `  git board project resolve 2605 status --theirs
  git board project resolve EPFL-ENAC/co2-calculator#2605 status --ours`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if ours == theirs {
				return usageError{errors.New("pass one of --ours or --theirs")}
			}
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			kept, err := b.ProjectResolve(cmd.Context(), args[0], args[1], ours)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s %s: kept %s on both sides\n", args[0], args[1], show(kept))
			return nil
		},
	}
	cmd.Flags().BoolVar(&ours, "ours", false, "keep git-board's value, written to the Project")
	cmd.Flags().BoolVar(&theirs, "theirs", false, "keep the Project's value, set on the label")
	return cmd
}

func newProjectConfigure() *cobra.Command {
	var owner string
	var number int
	var remove bool
	cmd := &cobra.Command{
		Use:   "configure",
		Short: "Name the GitHub Project in .git-board.toml",
		Long: `Write [github-project] in .git-board.toml: the Project's owner and number.
write = true, which lets a sync write to the Project, is kept for the same
Project, and is set by hand. --remove takes the table out.`,
		Example: `  git board project configure --owner EPFL-ENAC --project 40
  git board project configure --remove`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if remove == (owner != "" || number != 0) {
				return usageError{errors.New("pass --owner and --project, or --remove")}
			}
			b, err := open(cmd, false)
			if err != nil {
				return err
			}
			gp, _ := b.Config.Tools.GithubProject.Get()
			dir := b.Dir
			b.Close()
			path, err := app.ConfigureProject(dir, gp, owner, number)
			if err != nil {
				return err
			}
			if remove {
				fmt.Fprintf(cmd.OutOrStdout(), "removed [github-project] from %s\n", path)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "configured [github-project] owner=%q number=%d in %s\n", owner, number, path)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&owner, "owner", "", "the GitHub organization or user that owns the Project")
	cmd.Flags().IntVar(&number, "project", 0, "the Project's number")
	cmd.Flags().BoolVar(&remove, "remove", false, "take [github-project] out")
	return cmd
}

// printReport says a run's plan: per issue, label changes, Project
// writes, conflicts and warnings, then a summary, as v1 does.
func printReport(out io.Writer, r ghproject.Report) {
	plan := r.Plan
	edits := map[string][]ghproject.Edit{}
	for _, e := range plan.Edits {
		edits[e.IssueID] = append(edits[e.IssueID], e)
	}
	conflicts := map[string][]ghproject.Conflict{}
	for _, c := range plan.Conflicts {
		conflicts[c.IssueID] = append(conflicts[c.IssueID], c)
	}
	for _, c := range plan.Changes {
		es, cs := edits[c.IssueID], conflicts[c.IssueID]
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
			if e.Kind == ghproject.EditAssignees {
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
			fmt.Fprintf(out, "  ! %s\n", w.Text)
		}
	}
	for _, u := range plan.Unmatched {
		fmt.Fprintf(out, "unmatched #%d %s: %s\n", u.Number, u.Title, u.Reason)
	}
	for _, l := range plan.UnmappedLines() {
		fmt.Fprintln(out, l)
	}
	n := plan.Counts()
	fmt.Fprintf(out, "\nsummary: %d issue(s) matched, %d with label changes, %d Project write(s), %d conflict(s), %d warning(s), %d unmatched\n",
		n.Matched, n.LabelChanges, n.ProjectWrites, n.Conflicts, n.Warnings, n.Unmatched)
	if r.Truncated {
		fmt.Fprintf(out, "warning: read %d items, the --limit: the Project has more, which this run leaves out; raise --limit\n", r.Items)
	}
	if !r.Applied && len(plan.Edits) > 0 && !r.Write {
		fmt.Fprintln(out, "Writing to the Project also needs write = true under [github-project] in .git-board.toml.")
	}
	if !r.Applied {
		if l := r.FitLine(); l != "" {
			fmt.Fprintln(out, l)
		}
		return
	}
	fmt.Fprintln(out, "\napplied.")
	if r.Skipped > 0 {
		fmt.Fprintf(out, "%d change(s) not written to the Project: set write = true under [github-project] in .git-board.toml.\n", r.Skipped)
	}
	if n.Conflicts > 0 {
		fmt.Fprintf(out, "%d conflict(s) held: git board project resolve <issue number> <field> --ours | --theirs\n", n.Conflicts)
	}
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

// projectBridge is the TUI's Bridge: the Project's pull, "i" its check
// and "o" its run.
type projectBridge struct {
	ctx context.Context
	b   *app.Board
}

var _ tui.Bridge = projectBridge{}

func (p projectBridge) Read() (string, error) {
	r, err := p.b.ProjectRun(p.ctx, ghproject.Labels, false, 0)
	if err != nil {
		return "", err
	}
	n := r.Plan.Counts()
	return fmt.Sprintf("GitHub Project %s: %d matched (%d to change), %d conflict(s), %d unmatched", r.Project, n.Matched, n.LabelChanges, n.Conflicts, n.Unmatched), nil
}

func (p projectBridge) Apply() ([]string, error) {
	r, err := p.b.ProjectRun(p.ctx, ghproject.Labels, true, 0)
	if err != nil {
		return nil, err
	}
	var changed []string
	for _, c := range r.Plan.Changes {
		if len(c.Added)+len(c.Removed) > 0 {
			changed = append(changed, c.IssueID)
		}
	}
	return changed, nil
}
