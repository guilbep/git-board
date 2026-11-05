package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/app"
	"github.com/guilbep/git-board/next/forge"
)

func newSync() *cobra.Command {
	var remote string
	var gitOnly, allowCreate bool
	cmd := &cobra.Command{
		Use:   "sync [path...]",
		Short: "Sync the issues: git, the forges, then git again",
		Long: `Sync the issues of one or more repositories: a git pull from the remote,
the forge sync of every forge whose sync is on, then a git push to the
remote, which numbers the issues origin has. A failed pull stops the
steps after it; a failed forge step is told, and the push still runs; a
forge that can't be read is left out, told, and the others go on.
Without the remote, the git steps are skipped. --git syncs with git
alone: git board pull, then git board push.

A forge this repository never synced with, or 50 creations or more on
one, gets no issue created without --allow-create.

Each path is a directory of a git repository (the current directory if
none given), synced in turn: a failure on one doesn't stop the others.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := args
			if len(paths) == 0 {
				paths = []string{"."}
			}
			out := cmd.OutOrStdout()
			var failed []string
			for _, path := range paths {
				if err := syncOne(cmd, out, path, remote, allowCreate, gitOnly); err != nil {
					fmt.Fprintf(out, "%s: %s\n", path, app.NoCredentials(err.Error()))
					failed = append(failed, path)
				}
			}
			if len(failed) > 0 {
				return fmt.Errorf("sync failed for %d of %d repo(s): %v", len(failed), len(paths), failed)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&remote, "remote", "origin", "git remote to pull from / push to")
	cmd.Flags().BoolVar(&gitOnly, "git", false, "sync with git alone: pull, then push")
	cmd.Flags().BoolVar(&allowCreate, "allow-create", false, "create issues on a forge never synced with, or 50 or more on one")
	return cmd
}

// syncOne is the whole sync of the repository at path, each step told.
func syncOne(cmd *cobra.Command, out io.Writer, path, remote string, allowCreate, gitOnly bool) error {
	ctx := cmd.Context()
	b, err := app.Open(ctx, path, app.Options{Who: true,
		Notice: func(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) }})
	if err != nil {
		return fmt.Errorf("opening: %w", err)
	}
	defer b.Close()
	say := func(format string, a ...any) { fmt.Fprintf(out, path+": "+format+"\n", a...) }
	labels := map[string]string{app.StepPull: "git pull from " + remote, app.StepForges: "the forges",
		app.StepProject: "the GitHub Project", app.StepPush: "git push to " + remote}
	w := app.Whole{Remote: remote, NoProject: gitOnly, Step: func(name, state, note string) {
		switch {
		case state == "running":
			say("%s...", labels[name])
		case state == "failed":
			say("%s failed: %s", labels[name], note)
		case state == "skipped" && gitOnly && (name == app.StepForges || name == app.StepProject):
		case state == "skipped":
			say("%s: skipped, %s", labels[name], note)
		case note != "":
			say("%s: %s", labels[name], note)
		}
	}}
	var tables []forge.Table
	if !gitOnly {
		if tables, err = b.Forges(ctx, func(s string) { fmt.Fprintln(out, s) }); err != nil {
			return err
		}
	}
	if hasSyncOn(tables) {
		w.Forges = func(ctx context.Context) error {
			fs, err := b.OpenForges(ctx, tables)
			if err != nil {
				return err
			}
			return forgeWay(cmd, b, app.WaySync, app.ForgeRun{Forges: fs, AllowCreate: allowCreate}, say)
		}
	}
	if err := b.SyncWhole(ctx, w); err != nil {
		return err
	}
	say("synced")
	return nil
}

func hasSyncOn(tables []forge.Table) bool {
	for _, f := range tables {
		if f.Sync {
			return true
		}
	}
	return false
}
