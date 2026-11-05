package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/app"
)

func newDatastore() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "datastore",
		Short: "Say where the board is kept: git, PostgreSQL or Markdown files",
		Long: `Say where this clone keeps its board, and what that datastore can do.

The board is kept in git (refs/board/*) unless this clone names another:

  git config board.datastore postgres://user@host/db
  git config board.datastore markdown:issues

or GIT_BOARD_DATASTORE, which wins.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			k, err := app.Datastore(cmd.Context(), cwd)
			if err != nil {
				return err
			}
			yes := map[bool]string{true: "yes", false: "no"}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "kept in   %s\n", k.Where)
			fmt.Fprintf(out, "replica   %s (pull, push, start over)\n", yes[k.Replica])
			fmt.Fprintf(out, "counter   %s (project numbers)\n", yes[k.Counter])
			return nil
		},
	}
	var from, to string
	cp := &cobra.Command{
		Use:   "copy",
		Short: "Copy the board from one datastore into another: by default, git into the one this clone names",
		Long: `Copy every issue, person, sync record and release record from one datastore
into another, with the project's numbers where the other keeps none yet.
The packs copy as they are, so every id stays the same. What the other has
already is left alone, so a second run copies nothing; the first datastore
isn't touched.

--from and --to take what board.datastore takes: git, markdown:<dir>, or
a postgres:// URL. By default the copy is from git into the datastore
this clone names.`,
		Example: `  git config board.datastore postgres://me@db.example.org/board
  git board datastore copy
  git board datastore copy --from postgres://me@db.example.org/board --to git`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			c, err := app.CopyDatastore(cmd.Context(), cwd, from, to)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d copied into %s\n", c.Entities, c.To)
			return nil
		},
	}
	cp.Flags().StringVar(&from, "from", "", "the datastore to copy from (default git)")
	cp.Flags().StringVar(&to, "to", "", "the datastore to copy into (default the one this clone names)")
	cmd.AddCommand(cp)
	return cmd
}
