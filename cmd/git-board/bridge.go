package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghbridge"
	"github.com/guilbep/git-board/internal/store"
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

Pull is read-only against GitHub: it only reads the Project with gh,
never writes back to the Project or any GitHub issue. It matches project
items to the issues the forge sync linked to their GitHub issue; an item
with no matching issue is reported unmatched, not an error. It is git
board project pull: a field changed in git-board since both sides last
agreed stays. Sync goes both ways.

Meant to run repeatedly, not once: a Project's fields keep changing as
work progresses, and every run recomputes the diff, so a second run
with nothing changed is a no-op.`,
		Example: `  git-board bridge configure --owner EPFL-ENAC --project 40
  git-board bridge pull                    # dry-run preview (default)
  git-board bridge pull --apply            # actually write the labels
  git-board bridge pull --owner EPFL-ENAC --project 40 --dry-run`,
	}
	cmd.AddCommand(newBridgeConfigureCmd(), newBridgePullCmd(), newBridgeSyncCmd(), newBridgeResolveCmd())
	// git board project took its place: cobra tells the command run only,
	// so every one of them says so
	cmd.Hidden = true
	for _, c := range append(cmd.Commands(), cmd) {
		c.Deprecated = bridgeDeprecated
	}
	return cmd
}

const bridgeDeprecated = "use git board project: pull, push, sync, resolve, configure"

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
		Short: "Take the GitHub Project's changes into the labels (git board project pull)",
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
	// the flags name the Project for this run as the table would: the
	// fields it syncs live there for the run either way
	cfg.GithubProject.Owner, cfg.GithubProject.Number = owner, project

	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()
	// git board project pull: only the side that changed since both
	// agreed is taken, so a change made here isn't overwritten
	if err := projectRun(out, cfg, b, limit, "labels", apply); err != nil || apply {
		return err
	}
	fmt.Fprintln(out, "\ndry run — nothing written. Re-run with --apply to write these changes.")
	return nil
}
