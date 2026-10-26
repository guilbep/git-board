package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghbridge"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// fetchGithubItems is a seam over ghbridge.FetchItems so tests can swap in
// a fixture instead of shelling out to the real `gh` binary.
var fetchGithubItems = ghbridge.FetchItems

const defaultBridgePullLimit = ghbridge.DefaultItemLimit

func newBridgeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "bridge",
		Short: "Bridge GitHub Project (v2) field values into labels",
		Long: `Bridge GitHub Project (v2) field values into labels.

Pull is read-only against GitHub: it only calls "gh project item-list",
never writes back to the Project or any GitHub issue. It matches project
items to the issues the forge sync linked to their GitHub issue; an item
with no matching issue is reported unmatched, not an error. Sync goes
both ways.

Meant to run repeatedly, not once: a Project's fields keep changing as
work progresses, and every run recomputes the diff, so a second run
with nothing changed is a no-op.`,
		Example: `  git-board bridge configure --owner EPFL-ENAC --project 40
  git-board bridge pull                    # dry-run preview (default)
  git-board bridge pull --apply            # actually write the labels
  git-board bridge pull --owner EPFL-ENAC --project 40 --dry-run`,
	}
	cmd.AddCommand(newBridgeConfigureCmd(), newBridgePullCmd(), newBridgeSyncCmd(), newBridgeResolveCmd())
	return cmd
}

func newBridgeConfigureCmd() *cobra.Command {
	var owner string
	var project int

	cmd := &cobra.Command{
		Use:     "configure",
		Short:   "Store the GitHub Project owner/number in .git-board.toml",
		Example: `  git-board bridge configure --owner EPFL-ENAC --project 40`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			path := config.Path()
			if err := config.SetGithubProject(path, owner, project); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "configured [github-project] owner=%q number=%d in %s\n", owner, project, path)
			return nil
		},
	}
	cmd.Flags().StringVar(&owner, "owner", "", "GitHub org/user that owns the project (required)")
	cmd.Flags().IntVar(&project, "project", 0, "GitHub Project (v2) number (required)")
	_ = cmd.MarkFlagRequired("owner")
	_ = cmd.MarkFlagRequired("project")
	return cmd
}

func newBridgePullCmd() *cobra.Command {
	var owner string
	var project int
	var apply, dryRun bool
	var limit int

	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Sync GitHub Project field values into labels on matching issues",
		Example: `  git-board bridge pull                              # dry-run preview
  git-board bridge pull --apply                       # write the labels
  git-board bridge pull --owner EPFL-ENAC --project 40 --apply
  git-board bridge pull --limit 500 --apply           # smaller batch, avoids rate limits`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if apply && dryRun {
				return fmt.Errorf("--apply and --dry-run are mutually exclusive")
			}
			return runBridgePull(cmd.OutOrStdout(), owner, project, limit, apply)
		},
	}
	cmd.Flags().StringVar(&owner, "owner", "", "GitHub org/user (overrides .git-board.toml [github-project])")
	cmd.Flags().IntVar(&project, "project", 0, "GitHub Project number (overrides .git-board.toml [github-project])")
	cmd.Flags().BoolVar(&apply, "apply", false, "write the label changes (default is dry-run preview only)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview only, write nothing (the default with neither flag)")
	cmd.Flags().IntVar(&limit, "limit", defaultBridgePullLimit, "max items to fetch from gh project item-list")
	return cmd
}

func runBridgePull(out io.Writer, ownerFlag string, projectFlag, limit int, apply bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	owner := ownerFlag
	if owner == "" {
		owner = cfg.GithubProject.Owner
	}
	project := projectFlag
	if project == 0 {
		project = cfg.GithubProject.Number
	}
	if owner == "" || project == 0 {
		return fmt.Errorf("no GitHub project configured: pass --owner/--project, or run `git-board bridge configure` first")
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

	items, err := fetchGithubItems(owner, project, limit)
	if err != nil {
		return err
	}

	plan := ghbridge.BuildPlan(items, issues, cfg)
	printBridgePlan(out, plan)

	if !apply {
		fmt.Fprintln(out, "\ndry run — nothing written. Re-run with --apply to write these changes.")
		return nil
	}
	if err := ghbridge.ApplyPlan(b, plan); err != nil {
		return err
	}
	fmt.Fprintln(out, "\napplied.")
	return nil
}

// printBridgePlan renders what BuildPlan found: per-issue label changes,
// then a summary — matched/not-matched, fields set, and any values that
// didn't map to a configured option (skipped, not silently dropped).
func printBridgePlan(out io.Writer, plan ghbridge.Plan) {
	fieldsSet := 0
	changed := 0
	warnings := 0
	for _, c := range plan.Changes {
		if len(c.Added) == 0 && len(c.Removed) == 0 && len(c.Warnings) == 0 {
			continue
		}
		changed++
		for _, f := range c.Fields {
			if len(f.Added) > 0 || len(f.Removed) > 0 {
				fieldsSet++
			}
		}
		warnings += len(c.Warnings)
		fmt.Fprintf(out, "#%d %s\n", c.Number, c.Title)
		for _, l := range c.Added {
			fmt.Fprintf(out, "  + %s\n", l)
		}
		for _, l := range c.Removed {
			fmt.Fprintf(out, "  - %s\n", l)
		}
		for _, w := range c.Warnings {
			fmt.Fprintf(out, "  ! %s\n", w)
		}
	}

	notAnIssue, notBridged := 0, 0
	for _, u := range plan.Unmatched {
		fmt.Fprintf(out, "unmatched #%d %s: %s\n", u.Number, u.Title, u.Reason)
		if u.NotAnIssue {
			notAnIssue++
		} else {
			notBridged++
		}
	}

	fmt.Fprintf(out, "\nsummary: %d issue(s) matched (%d changed), %d unmatched (%d not an issue, %d not bridged), %d field(s) set, %d value warning(s)\n",
		len(plan.Changes), changed, len(plan.Unmatched), notAnIssue, notBridged, fieldsSet, warnings)
}
