package main

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/store"
)

func newUndoCmd() *cobra.Command {
	var force, asJSON bool
	cmd := &cobra.Command{
		Use:   "undo [<id>]",
		Short: "Undo your last change to an issue",
		Long: `Undo the last change you made to an issue, by writing its inverse: a
label added is removed and one removed added back, a status, a title or
a text goes back to what it was, a new comment is deleted. With no id,
your last change to any issue, by its time.

A change is what one command wrote: git board edit -e changes the title
and the description at once, and undo puts both back. Undoing twice
redoes. Changes a sync pulled from a forge aren't yours to undo here.

It refuses, exit code 2, what it can't undo, and says why: creating an
issue (git board delete deletes it), a delete, a change of metadata. It
refuses too when someone else wrote to the issue after you, or a sync
pulled into it: --force undoes yours anyway, over theirs.`,
		Example: `  git board undo a1b2c3d
  git board undo                     # your last change, on any issue
  git board undo a1b2c3d --force     # though someone wrote after you`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := store.OpenCwd()
			if err != nil {
				return err
			}
			defer b.Close()
			id := ""
			if len(args) == 1 {
				id = args[0]
			}
			u, err := b.Undo(id, force)
			if errors.Is(err, store.ErrCantUndo) || errors.Is(err, store.ErrWroteAfter) {
				return usageError{err}
			}
			if err != nil {
				return err
			}
			if asJSON {
				return writeJSON(cmd.OutOrStdout(), u)
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "undone on %s %s:\n", shortID(u.Issue), u.Title)
			for _, w := range u.What {
				fmt.Fprintln(out, "  "+w)
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "undo though someone else wrote to the issue after you")
	cmd.Flags().BoolVar(&asJSON, "json", false, `print {"issue", "title", "undone", "what"}`)
	return cmd
}
