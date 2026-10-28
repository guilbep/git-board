package main

import (
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/store"
)

func newPruneCmd() *cobra.Command {
	var dryRun bool
	var remote string
	cmd := &cobra.Command{
		Use:   "prune",
		Short: "Remove git-bug's data from a repository switched to git-board",
		Long: `Remove git-bug's data from a repository switched to git-board
(git board switch --apply), once no clone or automation uses git-bug any
more:

- its refs: refs/bugs, refs/identities and the old release records in
  refs/releases, here and as fetched from a remote
  (refs/remotes/<remote>/bugs/<id>…; a branch named releases/1.2 stays);
- its cache, .git/git-bug;
- its git-bug.* keys in .git/config, the bridge's among them.

The refs go to a bundle in the git directory first, which it names:
git fetch <bundle> 'refs/*:refs/*' brings them back. git-board's own data,
refs/board, stays.

--remote also deletes git-bug's refs on that remote, where nothing backs
them up: a clone still on git-bug loses its shared issues. Run it last,
once every clone has synced with git-board, and each clone runs prune
without it to clean itself.

A repository whose issues are still git-bug's is refused: run git board
switch first.`,
		Example: `  git board prune -n               # what it would remove, nothing removed
  git board prune
  git board prune --remote origin  # and on origin`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := store.OpenCwd()
			if err != nil {
				return err
			}
			defer b.Close()
			rep, err := b.Prune(remote, dryRun)
			if err != nil && rep.Backup == "" { // nothing removed here
				return err
			}
			printPrune(cmd.OutOrStdout(), rep, remote, dryRun)
			return err
		},
	}
	cmd.Flags().BoolVarP(&dryRun, "dry-run", "n", false, "say what it would remove, and remove nothing")
	cmd.Flags().StringVar(&remote, "remote", "", "also delete git-bug's refs on this remote")
	return cmd
}

func printPrune(out io.Writer, rep store.PruneReport, remote string, dryRun bool) {
	if len(rep.Refs) == 0 && rep.Cache == "" && len(rep.Keys) == 0 && rep.Remote == 0 {
		fmt.Fprintln(out, "No git-bug data here.")
		return
	}
	for _, ns := range slices.Sorted(maps.Keys(rep.Refs)) {
		fmt.Fprintf(out, "%s*: %d refs\n", ns, rep.Refs[ns])
	}
	if rep.Cache != "" {
		fmt.Fprintf(out, "git-bug's cache: %s\n", rep.Cache)
	}
	if len(rep.Keys) > 0 {
		fmt.Fprintf(out, "config keys: %s\n", strings.Join(rep.Keys, ", "))
	}
	if remote != "" {
		fmt.Fprintf(out, "on %s: %d refs\n", remote, rep.Remote)
	}
	switch {
	case dryRun:
		fmt.Fprintln(out, "A dry run: nothing removed.")
	case rep.Backup != "":
		fmt.Fprintf(out, "Removed. The refs are backed up in %s:\n  git fetch %s 'refs/*:refs/*'\nbrings them back.\n", rep.Backup, rep.Backup)
	default:
		fmt.Fprintln(out, "Removed.")
	}
}
