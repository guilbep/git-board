package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/app"
)

// The forges alone: GitHub's and GitLab's issues, a translation of the
// board made both ways, as v1's git board forge. No git step.

func newForge() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "forge",
		Short: "The forges alone: pull, push or sync their issues, with no git step",
		Long: `The forges alone: the issues on GitHub and GitLab, which git board sync
moves between its git pull and its git push. These run that step on its
own, one way or both, with the forges of .git-board.toml, or those found
in the git remotes, whose sync is on.`,
	}
	cmd.AddCommand(
		newForgeWay(app.WayPull, "Take the forges' changes, and send nothing",
			`Take the forges' changes: new issues and comments, edits, closings. Nothing
goes to a forge.`),
		newForgeWay(app.WayPush, "Send this clone's changes to the forges, and take nothing",
			`Send this clone's changes to the forges. Nothing comes back from them. A
forge this repository never synced with, or 50 creations or more on one,
gets no issue created without --allow-create.`),
		newForgeWay(app.WaySync, "Take the forges' changes, then send this clone's",
			`Take the forges' changes, then send this clone's: the forge step of git
board sync, alone. Creations wait for consent as push says.`),
	)
	return cmd
}

func newForgeWay(way app.Way, short, long string) *cobra.Command {
	var allowCreate bool
	cmd := &cobra.Command{
		Use:   string(way),
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
			tables, err := b.Forges(cmd.Context(), func(s string) { fmt.Fprintln(out, s) })
			if err != nil {
				return err
			}
			fs, err := b.OpenForges(cmd.Context(), tables)
			if err != nil {
				return err
			}
			say := func(format string, a ...any) { fmt.Fprintf(out, format+"\n", a...) }
			if err := forgeWay(cmd, b.Board, way, app.ForgeRun{Forges: fs, AllowCreate: allowCreate}, say); err != nil {
				return err
			}
			say("done")
			return nil
		},
	}
	cmd.Flags().BoolVar(&allowCreate, "allow-create", false, "create issues on a forge never synced with, or 50 or more on one")
	return cmd
}

// forgeWay runs one way of the forge step, each event told in a line.
func forgeWay(cmd *cobra.Command, b *app.Board, way app.Way, run app.ForgeRun, say func(format string, a ...any)) error {
	run.See = func(e app.SyncEvent) {
		if e.What == "progress" {
			return
		}
		name := ""
		if e.Issue != "" {
			name = b.EventIssue(cmd.Context(), e.Issue)
		}
		line := e.Text(name)
		if e.What == "held" {
			line += " (--allow-create)"
		}
		if line != "" {
			say("  %s: %s", e.Forge, line)
		}
	}
	_, err := b.SyncForges(cmd.Context(), way, run)
	return err
}
