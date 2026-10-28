package main

import (
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/store"
)

// The git remote alone: the board's own data, as git moves it. git board
// sync runs these as its first and last steps, around the forges and the
// GitHub Project.

func newFetchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "fetch [remote]",
		Short: "Fetch the remote's board and say what waits, changing nothing here",
		Long: `Fetch the remote's issues and people (origin by default) into their
copies here, refs/board-remotes/<remote>/, and say what a pull would
receive and a push would send. Nothing of this clone's board changes:
git board pull merges them.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return onRemote(args, func(b *store.Backend, remote string) error {
				if err := b.Fetch(remote); err != nil {
					return err
				}
				if err := b.ResetGuard(remote); err != nil {
					return err
				}
				rs, err := b.Remotes()
				if err != nil {
					return err
				}
				for _, r := range rs {
					if r.Name == remote {
						fmt.Fprintf(cmd.OutOrStdout(), "%s: issues %d to receive, %d to send; people %d to receive, %d to send\n",
							remote, r.Issues.Receive, r.Issues.Send, r.People.Receive, r.People.Send)
					}
				}
				return nil
			})
		},
	}
}

func newPullCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "pull [remote]",
		Short: "Pull the remote's board: fetch, then merge",
		Long: `Pull the remote's issues and people (origin by default): fetch them,
then merge them into this clone's, every clone's operations kept. From
origin it also takes the project's numbers. It is git board sync's first
step, alone: no forge, no GitHub Project.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return onRemote(args, func(b *store.Backend, remote string) error {
				note, err := b.PullStep(remote, false)
				if err == nil {
					said(cmd.OutOrStdout(), "pulled from "+remote, note)
				}
				return err
			})
		},
	}
}

func newPushCmd() *cobra.Command {
	var prune bool
	cmd := &cobra.Command{
		Use:   "push [remote]",
		Short: "Push this clone's board to the remote",
		Long: `Push this clone's issues and people to the remote (origin by default),
then, on origin, number the issues it now has. The push isn't forced: when
the remote holds changes this clone doesn't have, it refuses, and git
board pull merges them first. It is git board sync's last step, alone.

--prune makes the remote's board this clone's: what the remote has and
this clone doesn't is deleted there, and a ref the remote moved is
overwritten. The remote's board is first written to a bundle in the git
directory, which it names. It is how a board started over with git board
reset --hard reaches origin; every other clone then runs git board reset
--hard origin.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			return onRemote(args, func(b *store.Backend, remote string) error {
				if prune {
					rep, err := b.PushPrune(remote)
					if err != nil {
						return err
					}
					fmt.Fprintf(out, "pushed to %s, and deleted there %d refs this clone doesn't have\n", remote, rep.Deleted)
					if rep.Backup != "" {
						fmt.Fprintf(out, "  %s's board as it was: %s; git fetch %s 'refs/board-pruned/*:refs/board/*' brings it here\n", remote, rep.Backup, rep.Backup)
					}
					return nil
				}
				note, err := b.PushStep(remote)
				if err == nil {
					said(out, "pushed to "+remote, note)
				}
				return err
			})
		},
	}
	cmd.Flags().BoolVar(&prune, "prune", false, "make the remote's board this clone's, deleting there what isn't here")
	return cmd
}

// onRemote opens the repository here and runs do on the remote args name,
// origin when none.
func onRemote(args []string, do func(b *store.Backend, remote string) error) error {
	remote := "origin"
	if len(args) == 1 {
		remote = args[0]
	}
	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()
	return do(b, remote)
}

// said prints what was done, then its note on a line of its own.
func said(out io.Writer, done, note string) {
	fmt.Fprintln(out, done)
	if note != "" {
		fmt.Fprintln(out, note)
	}
}
