package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/go-git/go-git/v5/plumbing/transport"
	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

func newSyncCmd() *cobra.Command {
	var remote string
	var check, gitOnly bool

	cmd := &cobra.Command{
		Use:   "sync [path...]",
		Short: "Sync the issues: git, the forges, the GitHub Project, then git again",
		Long: `Sync the issues of one or more repositories, as Settings › Sync does: a
git pull from the remote, the forge sync of every forge whose sync is on,
the GitHub Project's fields when [github-project] is set (written to the
Project with write = true alone), then a git push to the remote, which
numbers the issues origin has. A step that fails stops the steps after it,
and says which; a forge that can't be read is told, and the others go on.
Without the remote, the git steps are skipped.

The forges are those of .git-board.toml, or with no [forges.<name>]
table, those found in the git remotes, each told in a line.

--check runs the same steps and writes nothing: a fetch and compare, the
forges' dry run, the Project's check. --git syncs with git alone: a pull,
then a push.

Each path is a directory of a git repository (the current directory if
none given), synced in turn: a failure on one doesn't stop the others.
This is deliberately explicit paths, not [repos] from .git-board.toml:
that table holds "owner/repo" display names, not local clone locations,
and there's no established convention yet for mapping one to the other —
see docs/implementation-plan.md.`,
		Example: `  git-board sync --check                  # what a sync would do, nothing written
  git-board sync
  git-board sync --git ../co2-calculator ../git-time
  git-board sync --remote upstream`,
		RunE: func(cmd *cobra.Command, args []string) error {
			paths := args
			if len(paths) == 0 {
				paths = []string{"."}
			}
			if gitOnly {
				if check {
					return errors.New("--check checks the whole sync: leave --git out")
				}
				return runSync(cmd.OutOrStdout(), paths, remote)
			}
			return eachPath(cmd.OutOrStdout(), paths, func(path string) error {
				return wholeOne(cmd.Context(), cmd.OutOrStdout(), path, remote, check)
			})
		},
	}
	cmd.Flags().StringVar(&remote, "remote", "origin", "git remote to pull from / push to")
	cmd.Flags().BoolVar(&check, "check", false, "the dry run: say what a sync would do, write nothing")
	cmd.Flags().BoolVar(&gitOnly, "git", false, "sync with git alone: pull, then push")
	return cmd
}

// eachPath syncs each path in turn, keeping going after a failure on one so
// one broken repo doesn't block the rest; every failure is still surfaced,
// as a combined error at the end.
func eachPath(out io.Writer, paths []string, sync func(path string) error) error {
	var failed []string
	for _, path := range paths {
		if err := sync(path); err != nil {
			fmt.Fprintf(out, "%s: %v\n", path, err)
			failed = append(failed, path)
		}
	}
	if len(failed) > 0 {
		return fmt.Errorf("sync failed for %d of %d repo(s): %v", len(failed), len(paths), failed)
	}
	return nil
}

// wholeOne is the whole sync of one repository (store.SyncWhole), as the
// server's Settings › Sync runs it, each step told.
func wholeOne(ctx context.Context, out io.Writer, path, remote string, check bool) error {
	b, err := store.Open(path)
	if err != nil {
		return fmt.Errorf("opening: %w", err)
	}
	defer b.Close()
	cfg, err := config.Load(config.PathIn(path))
	if err != nil {
		return err
	}
	say := func(format string, a ...any) { fmt.Fprintf(out, path+": "+format+"\n", a...) }
	forges, err := runForges(ctx, out, b, cfg)
	if err != nil {
		return err
	}
	labels := map[string]string{store.StepPull: "git pull from " + remote, store.StepForges: "the forges",
		store.StepProject: "the GitHub Project", store.StepPush: "git push to " + remote}
	if check {
		labels[store.StepPull] = "git fetch from " + remote
	}
	run := store.Whole{Remote: remote, DryRun: check, Step: func(name, state, note string) {
		switch {
		case state == "running":
			say("%s...", labels[name])
		case state == "failed":
			say("%s failed: %s", labels[name], note)
		case state == "skipped":
			say("%s: skipped, %s", labels[name], note)
		case note != "":
			say("%s: %s", labels[name], note)
		}
	}}
	var on []config.Forge
	for _, f := range forges {
		if f.Sync {
			on = append(on, f)
		}
	}
	if len(on) > 0 {
		run.Forges = func(ctx context.Context) error {
			fs, err := openForges(ctx, on)
			if err != nil {
				return err
			}
			_, err = b.SyncForges(ctx, store.ForgeRun{Forges: fs, PublishPrivate: cfg.PublishPrivate, DryRun: check, See: func(e store.SyncEvent) {
				if line := e.Text(check, eventIssue(b, e.Issue)); line != "" {
					say("  %s: %s", e.Forge, line)
				}
			}})
			return err
		}
	}
	if gp := cfg.GithubProject; gp.Owner != "" && gp.Number != 0 {
		run.Project = func(context.Context) error { return bridgeSync(out, cfg, b, defaultBridgePullLimit, !check) }
	}
	if err := b.SyncWhole(ctx, run); err != nil {
		return err
	}
	if check {
		say("checked: nothing written")
	} else {
		say("synced")
	}
	return nil
}

// eventIssue names an issue in a forge event's line: its number, else its
// short id, and its title. One a dry run would pull is only in its copy.
func eventIssue(b *store.Backend, id string) string {
	short := id[:min(7, len(id))]
	i, err := b.GetIssue(id)
	if err != nil {
		return short
	}
	return cmp.Or(i.Number, short) + " " + i.Title
}

// runSync is the git sync alone (--git): it pulls then pushes each path in
// turn, in-process (store's Pull and Push, over the core's git store).
func runSync(out io.Writer, paths []string, remote string) error {
	return eachPath(out, paths, func(path string) error {
		fmt.Fprintf(out, "%s: pulling from %s...\n", path, remote)
		return syncOne(out, path, remote)
	})
}

func syncOne(out io.Writer, path, remote string) error {
	b, err := store.Open(path)
	if err != nil {
		return fmt.Errorf("opening: %w", err)
	}
	defer b.Close()

	if err := b.Pull(remote); err != nil {
		// An empty remote (nothing pushed there yet — the normal state
		// of a brand-new shared repo before anyone's first sync) is not
		// a failure: there's nothing to pull, not an error pulling it.
		// Confirmed this is a real case, not a hypothetical: found via
		// a real two-repo test using a genuinely empty bare remote.
		if errors.Is(err, transport.ErrEmptyRemoteRepository) {
			fmt.Fprintf(out, "%s: remote has nothing to pull yet\n", path)
		} else {
			return fmt.Errorf("pull: %w", err)
		}
	}
	if err := b.Switched(); err != nil {
		return err
	}
	if remote == store.NumbersRemote {
		if err := b.FetchNumbers(remote); err != nil {
			return fmt.Errorf("numbers: %w", err)
		}
	}

	fmt.Fprintf(out, "%s: pushing to %s...\n", path, remote)
	pushOut, err := b.Push(remote)
	if err != nil {
		return fmt.Errorf("push: %w", err)
	}
	if pushOut != "" {
		fmt.Fprintf(out, "%s: %s\n", path, pushOut)
	}
	if remote == store.NumbersRemote {
		n, err := b.NumberIssues(remote)
		if err != nil {
			return fmt.Errorf("numbering: %w", err)
		}
		if n > 0 {
			fmt.Fprintf(out, "%s: numbered %d issue(s)\n", path, n)
		}
	}
	fmt.Fprintf(out, "%s: synced\n", path)
	return nil
}
