package main

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/store"
)

// The GitHub Project alone: its fields, matched to the labels, which git
// board sync moves after the forges. It was git board bridge.

func newProjectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "The GitHub Project alone: pull, push or sync its fields with the labels",
		Long: `The GitHub Project of .git-board.toml alone: its fields (status, prio,
mod, est, sprint, moscow, milestone, the assignees) matched to the labels.

Each field is compared with the value both sides last agreed on, which
git-board records on the issue: only the side that changed since is
taken. A field changed on both sides is a conflict, left alone on both
until git board project resolve, and listed on the Conflicts page.
Writing to the Project needs write = true under [github-project], and a
gh token with the project scope.`,
		Example: `  git board project configure --owner EPFL-ENAC --project 40
  git board project pull --check
  git board project sync`,
	}
	cmd.AddCommand(
		newProjectWayCmd("pull", "labels", "Take the Project's changes into the labels, and write nothing there",
			`Take the Project's changes into the labels: the fields changed there since
both sides agreed. Nothing is written to the Project: a field changed in
git-board waits for git board project push.`),
		newProjectWayCmd("push", "project", "Write git-board's changes to the Project, and take nothing",
			`Write git-board's changes to the Project: the fields changed in git-board
since both sides agreed, with write = true under [github-project]. No label
changes: a field changed on the Project waits for git board project pull.`),
		newProjectWayCmd("sync", "both", "Take the Project's changes, and write git-board's",
			`Both ways, as git board sync's Project step: the Project's changes reach
the labels, git-board's reach the Project.`),
	)
	for _, c := range []*cobra.Command{newBridgeResolveCmd(), newBridgeConfigureCmd()} {
		c.Long = strings.ReplaceAll(c.Long, "git-board bridge", "git board project")
		c.Example = strings.ReplaceAll(c.Example, "git-board bridge", "git board project")
		cmd.AddCommand(c)
	}
	return cmd
}

func newProjectWayCmd(use, side, short, long string) *cobra.Command {
	var check bool
	var limit int
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		Long:  long,
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			b, err := store.OpenCwd()
			if err != nil {
				return err
			}
			defer b.Close()
			if err := projectRun(out, cfg, b, limit, side, !check); err != nil {
				return err
			}
			if check {
				fmt.Fprintln(out, "checked: nothing written")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "the dry run: say what it would change, write nothing")
	cmd.Flags().IntVar(&limit, "limit", defaultBridgePullLimit, "max Project items to read")
	return cmd
}
