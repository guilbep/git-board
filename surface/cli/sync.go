package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/app"
	"github.com/guilbep/git-board/forge"
)

func newSync() *cobra.Command {
	var remote, migrate string
	var only []string
	var check, gitOnly, allowCreate bool
	cmd := &cobra.Command{
		Use:   "sync [path...]",
		Short: "Sync the issues: git, the forges, then git again",
		Long: `Sync the issues of one or more repositories: a git pull from the remote,
the forge sync of every forge whose sync is on, then a git push to the
remote, which numbers the issues origin has. A failed pull stops the
steps after it; a failed forge step is told, and the push still runs; a
forge that can't be read is left out, told, and the others go on.
Without the remote, the git steps are skipped. --git syncs with git
alone: git board pull, then git board push. --check runs the same steps
and writes nothing: a fetch and compare, the forges' dry run, the
Project's check.

--migrate <field> moves a field's values to the home its forge mappings
now name, once a mapping changed and a sync ran: the old labels off the
forge's issues, and the assignees set when they are the new home.

A forge this repository never synced with, or 50 creations or more on
one, gets no issue created without --allow-create.

--only <parts> runs some parts of the forges' sync, and leaves the others
for the next sync: pull-changes (the forge issues that changed, pulled
into their issues), pull-new (the forge issues new to the board), create
(the issues created on a forge), send (the changes sent to the linked
forge issues). --only pull-changes,pull-new pushes nothing to the forges.
The Project pulls with a pull part, and is written to with create or
send. --check counts each part, and says the issues of each.

Each path is a directory of a git repository (the current directory if
none given), synced in turn: a failure on one doesn't stop the others.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := args
			if len(paths) == 0 {
				paths = []string{"."}
			}
			out := cmd.OutOrStdout()
			parts, err := app.ParseParts(only)
			if err != nil {
				return err
			}
			var failed []string
			for _, path := range paths {
				one := func() error {
					return syncOne(cmd, out, path, remote, app.ForgeRun{AllowCreate: allowCreate, DryRun: check, Only: parts}, gitOnly)
				}
				if migrate != "" {
					one = func() error { return migrateOne(cmd, out, path, migrate) }
				}
				if err := one(); err != nil {
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
	cmd.Flags().BoolVar(&check, "check", false, "the dry run: say what a sync would do, write nothing")
	cmd.Flags().BoolVar(&gitOnly, "git", false, "sync with git alone: pull, then push")
	cmd.Flags().StringVar(&migrate, "migrate", "", "move one field's values to the home its forge mappings name: the old labels off the forge, the assignees set")
	cmd.Flags().BoolVar(&allowCreate, "allow-create", false, "create issues on a forge never synced with, or 50 or more on one")
	cmd.Flags().StringSliceVar(&only, "only", nil, "the parts the sync runs, the others left for the next: pull-changes, pull-new, create, send")
	return cmd
}

// syncOne is the whole sync of the repository at path, each step told.
// run says the forges' run: its consent, its check and its parts.
func syncOne(cmd *cobra.Command, out io.Writer, path, remote string, run app.ForgeRun, gitOnly bool) error {
	check := run.DryRun
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
	if check {
		labels[app.StepPull] = "git fetch from " + remote
	}
	w := app.Whole{Remote: remote, DryRun: check, NoProject: gitOnly, Only: run.Only, Step: func(name, state, note string) {
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
	parts := map[app.Part]int{}
	if hasSyncOn(tables) {
		w.Forges = func(ctx context.Context) error {
			fs, err := b.OpenForges(ctx, tables)
			if err != nil {
				return err
			}
			run.Forges = fs
			return forgeWay(cmd, b, app.WaySync, run, say, func(e app.SyncEvent) { parts[e.Part()]++ })
		}
	}
	if err := b.SyncWhole(ctx, w); err != nil {
		return err
	}
	if check {
		if w.Forges != nil {
			say("parts: %d pull-changes, %d pull-new, %d create, %d send: run some with --only",
				parts[app.Parts[0]], parts[app.Parts[1]], parts[app.Parts[2]], parts[app.Parts[3]])
		}
		say("checked: nothing written")
	} else {
		say("synced")
	}
	return nil
}

// migrateOne is sync --migrate: the migration a forge mapping's change
// needs after its first sync run (app.MigrateField), each forge told.
func migrateOne(cmd *cobra.Command, out io.Writer, path, field string) error {
	b, err := app.Open(cmd.Context(), path, app.Options{Who: true,
		Notice: func(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) }})
	if err != nil {
		return fmt.Errorf("opening: %w", err)
	}
	defer b.Close()
	_, err = b.MigrateField(cmd.Context(), app.MigrateReq{Field: field, Say: func(line string) { fmt.Fprintln(out, path+": "+line) }})
	return err
}

func hasSyncOn(tables []forge.Table) bool {
	for _, f := range tables {
		if f.Sync {
			return true
		}
	}
	return false
}
