package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/store"
)

// keptInHistory is said after each delete: it hides, it doesn't erase
// (docs/adr/0019).
const keptInHistory = "its text stays in the board's history, in every clone that has it: revoke a leaked secret"

func newDeleteCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "delete <id>",
		Short: "Delete an issue: hidden from every view, kept in history",
		Long: `Delete an issue, as the person you write as. No list, search, page,
export or sync shows it again, and its id answers not found (exit 3).

A delete hides; it doesn't erase. The issue's text stays in the board's
history, in every clone that fetched it, and on the forges it was synced
to: a forge keeps its copy. A secret pasted into an issue is leaked:
revoke it. A pull never brings a deleted issue back.

On a terminal it asks first; --yes doesn't ask, and is needed when no
one is at a terminal (a script, CI). git board undo doesn't undo a
delete.`,
		Example: `  git board delete a1b2c3d
  git board delete a1b2c3d --yes
  git board comment a1b2c3d --delete 9c8b7a6:1   # a comment`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := store.OpenCwd()
			if err != nil {
				return err
			}
			defer b.Close()
			if err := needMe(b); err != nil {
				return err
			}
			is, err := b.GetIssue(args[0])
			if err != nil {
				return err
			}
			if err := confirm(cmd, yes, fmt.Sprintf("delete %s %q?", shortID(is.ID), is.Headline())); err != nil {
				return err
			}
			if _, err := b.DeleteIssue(is.ID); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "deleted %s %s\n", shortID(is.ID), is.Headline())
			fmt.Fprintln(cmd.ErrOrStderr(), "note: "+keptInHistory)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "don't ask: delete it")
	return cmd
}
