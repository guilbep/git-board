package main

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/server"
	"github.com/guilbep/git-board/internal/store"
)

func newResetCmd() *cobra.Command {
	var hard bool
	cmd := &cobra.Command{
		Use:   "reset --hard [remote]",
		Short: "Start this clone's board over: empty, or the remote's",
		Long: `Start this clone's board over, on the git side alone: no forge and no
GitHub Project is touched.

git board reset --hard empties it: every issue, person, sync link and
release record goes, with the fetched copies and board.me. The switch's
mark and the numbers' counter stay, so no number is handed out twice.
A new reset's mark is written: until origin has it, a pull or a push
refuses, and git board push --prune gives it to origin.

git board reset --hard <remote> makes this clone's board the remote's,
its marks included, and drops what only this clone had. Every other
clone runs it after one clone started over and ran git board push
--prune.

Either first writes every ref it deletes or moves to a bundle in the git
directory, and names it: git fetch <bundle> 'refs/*:refs/*' brings them
back. It refuses while git board serve runs on the repository: Settings ›
Sync's Start over runs the same inside the server.

Then git board forge pull refills the board from the forges. It brings
back every issue and comment that came from a forge, not yet what
git-board itself posted to one.`,
		Example: `  git board reset --hard           # empty this clone's board
  git board push --prune           # make origin's board this clone's
  git board reset --hard origin    # on every other clone`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !hard {
				return errors.New("only reset --hard exists: it empties this clone's board, or with a remote makes it the remote's")
			}
			b, err := store.OpenCwd()
			if err != nil {
				return err
			}
			defer b.Close()
			if err := refuseWhileServed(b); err != nil {
				return err
			}
			remote := ""
			if len(args) == 1 {
				remote = args[0]
			}
			rep, err := b.ResetHard(remote)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "reset: %d refs deleted", rep.Deleted)
			if rep.Backup != "" {
				fmt.Fprintf(out, ", backed up in %s\n  git fetch %s 'refs/*:refs/*' brings them back", rep.Backup, rep.Backup)
			}
			fmt.Fprintln(out)
			if remote == "" {
				fmt.Fprintln(out, "this clone's board is empty. git board push --prune empties origin's; every other clone then runs git board reset --hard origin")
			}
			if rep.Me {
				fmt.Fprintln(out, "board.me is unset: its person is gone")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&hard, "hard", false, "empty this clone's board, or with a remote make it the remote's")
	return cmd
}

// refuseWhileServed refuses while git board serve runs on the repository,
// from any of its worktrees: they share refs/board.
func refuseWhileServed(b *store.Backend) error {
	common := b.Store().CommonDir()
	dirs, _ := filepath.Glob(filepath.Join(common, "worktrees", "*"))
	for _, dir := range append([]string{common}, dirs...) {
		if inst, ok := server.Running(dir); ok {
			return fmt.Errorf("git board serve runs on this repository (%s): stop it first, or use Settings › Sync, Start over", inst.URL)
		}
	}
	return nil
}
