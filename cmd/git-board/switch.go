package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

func newSwitchCmd() *cobra.Command {
	var apply, force, allowCreate bool
	var gitBug string
	cmd := &cobra.Command{
		Use:   "switch",
		Short: "Move this repository's issues from git-bug to git-board, pulled from its forges",
		Long: `Move this repository's issues from git-bug (refs/bugs) to git-board's own
core (refs/board), by pulling them from the forges of .git-board.toml
whose sync is on, or with no [forges.<name>] table, from those the git
remotes are on, each printed. Nothing is converted: what only git-bug
holds is lost, and the report lists it.

By default it is a dry run, on a copy of the repository: it pulls (never
pushes), joins the mirrors git-bug posted to their originals, keys the
project's numbers again, carries the release records over, then counts
what the next sync would write to the forges, every write answered
without being made. It writes nothing here but origin's numbers, fetched,
and nothing on any forge.

--apply does the same, then writes the copy's refs/board/* here, and,
unless board.me is set, makes you the person of your git-bug identity
(git config git-bug.identity). It never deletes refs/bugs and never
pushes. It refuses when the report has
a blocker. --allow-create accepts the issues and comments the next sync
would create on the forges, that blocker alone; --force applies despite
every blocker.

The numbers are keyed again on origin's refs/board/seq, which --apply
makes this clone's. The next git-board sync sends it first, a
fast-forward; if origin's numbers moved on meanwhile, git refuses it and
the sync says so: run --apply again, which keys origin's numbers.

git-bug's binary reads its own data, on the copy: --git-bug names it.`,
		Example: `  git-board switch                     # the dry run, and its report
  git-board switch --apply
  git-board switch --apply --allow-create   # the next sync's creations are expected
  git-board switch --git-bug ~/.local/bin/git-bug`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSwitch(cmd.Context(), cmd.OutOrStdout(), store.SwitchRun{GitBug: gitBug, Apply: apply, Force: force, AllowCreate: allowCreate})
		},
	}
	cmd.Flags().BoolVar(&apply, "apply", false, "write the switch here, not just report it")
	cmd.Flags().BoolVar(&allowCreate, "allow-create", false, "with --apply, accept what the next sync would create on the forges")
	cmd.Flags().BoolVar(&force, "force", false, "with --apply, apply despite every blocker of the report")
	cmd.Flags().StringVar(&gitBug, "git-bug", "git-bug", "git-bug's binary, which reads the issues it holds")
	return cmd
}

// openForges opens the forges a switch pulls from, and a sync syncs
// with. Tests swap it.
var openForges = store.OpenForges

func runSwitch(ctx context.Context, out io.Writer, run store.SwitchRun) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()
	forges, left, err := runForges(ctx, out, b, cfg)
	if err != nil {
		return err
	}
	run.LeftOut = left
	var on []config.Forge
	for _, f := range forges {
		if f.Sync {
			on = append(on, f)
		} else {
			fmt.Fprintf(out, "%s: its sync is off, so it isn't pulled\n", f.Name)
		}
	}
	if len(on) == 0 && len(left) > 0 {
		return store.ErrForgesLeftOut // not "add a remote": the remotes have them
	}
	if run.Forges, err = openForges(ctx, on); err != nil {
		return err
	}
	run.PublishPrivate = cfg.PublishPrivate
	rep, err := b.Switch(ctx, run)
	if err != nil {
		return err
	}
	printSwitch(out, rep, run.Apply)
	if run.Apply && !rep.Applied {
		return errors.New("not applied: the report has blockers (--allow-create accepts the creations alone, --force applies despite every blocker)")
	}
	return nil
}

// runForges are the forges of a run: the settings' tables, or those found
// in the git remotes, each told in a line, its project read (GitLab's id,
// GitHub's spelling), and each URL skipped or forge left out with why;
// left are those left out, each "name: why".
func runForges(ctx context.Context, out io.Writer, b *store.Backend, cfg *config.Config) (forges []config.Forge, left []string, err error) {
	say := func(s string) { fmt.Fprintln(out, s) }
	if forges, err = b.Forges(cfg, say); err != nil {
		return nil, nil, err
	}
	forges = store.ResolveForges(ctx, forges, func(f config.Forge, why error) {
		left = append(left, fmt.Sprintf("%s: %v", f.Name, why))
		fmt.Fprintf(out, "forge %s, found in the remotes, is left out: %s %s: %v\n", f.Name, f.Host, cmp.Or(f.Path, f.Project), why)
	})
	for _, f := range forges {
		if !f.Discovered {
			continue
		}
		project := f.Project
		if f.Path != "" {
			project = f.Path + " (id " + f.Project + ")"
		}
		fmt.Fprintf(out, "forge %s, found in the remotes: %s %s\n", f.Name, f.Host, project)
	}
	return forges, left, nil
}

func printSwitch(out io.Writer, rep store.SwitchReport, apply bool) {
	list := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		fmt.Fprintf(out, "  %s (%d):\n", title, len(items))
		for i, it := range items {
			if i == 20 {
				fmt.Fprintf(out, "    … and %d more\n", len(items)-i)
				break
			}
			fmt.Fprintf(out, "    %s\n", it)
		}
	}
	if !apply {
		fmt.Fprintln(out, "A dry run, on a copy: nothing is written here (--apply writes it), and nothing on a forge.")
	}
	p := rep.Pulled
	fmt.Fprintf(out, "Pulled: %d issues made, %d mirrors joined to their original's; %d issues on two forges or more.\n",
		p.Created, rep.Joined, rep.OnSeveral)
	for _, name := range slices.Sorted(maps.Keys(rep.Forges)) {
		fmt.Fprintf(out, "  %s: %d issues\n", name, rep.Forges[name])
	}
	fmt.Fprintf(out, "Numbers: %d keyed again by the new issues.\n", rep.Numbers)
	list("left without an issue, kept so that none is handed out again", rep.NoIssue)
	fmt.Fprintln(out, "Lost at the switch:")
	list("git-bug's issues not on the forges pulled", rep.Lost)
	list("git-bug's issues pulled as two", rep.Split)
	list("comments the fork never posted", rep.LostComments)
	fmt.Fprintln(out, "  comment metadata (cycle, review): not readable from git-bug's JSON, none carried")
	fmt.Fprintf(out, "Releases: %d records in refs/releases, %d carried into git-board's.\n", rep.Releases, rep.Carried)
	list("not carried", rep.ReleaseNotes)
	fmt.Fprintf(out, "The next sync: %d creations on the forges, %d other writes.\n", len(rep.WouldCreate), len(rep.Writes))
	list("it would create", rep.WouldCreate)
	list("its other writes", rep.Writes)
	if rep.You != "" && !rep.Applied {
		fmt.Fprintf(out, "You: %s, your git-bug identity, who you write as once applied (board.me).\n", rep.You)
	}
	if len(rep.Blockers) > 0 {
		fmt.Fprintln(out, "Blockers:")
		for _, b := range rep.Blockers {
			fmt.Fprintf(out, "  - %s\n", b)
		}
	}
	if rep.Applied {
		fmt.Fprintln(out, "Applied: refs/board/* written; refs/bugs/* kept; nothing pushed.")
		if rep.You != "" {
			fmt.Fprintf(out, "You: %s\n", rep.You)
		}
		fmt.Fprintln(out, "Next: git-board sync --check, then git-board sync, once every clone runs this git-board.")
		if rep.Numbers > 0 {
			fmt.Fprintf(out, "  The sync sends the %d numbers keyed again first; if %s's numbers moved on meanwhile, it refuses: run git-board switch --apply again.\n", rep.Numbers, store.NumbersRemote)
		}
	}
}
