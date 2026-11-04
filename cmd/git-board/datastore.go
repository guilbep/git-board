package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/store"
)

func newDatastoreCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "datastore",
		Short: "Say where the board is kept: git, or PostgreSQL",
		Long: `Say where this clone keeps its board, and what that datastore can do.

The board is kept in git (refs/board/*) unless this clone names another:

  git config board.datastore postgres://user@host/db

or GIT_BOARD_DATASTORE, which wins. The URL stays in this clone's git
config, never in .git-board.toml: it may hold a password. On PostgreSQL the
board is shared at once, so pull, push and start over don't apply; the
index is kept there too, for every machine.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDatastore(cmd.OutOrStdout())
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "copy",
		Short: "Copy the board in git into the datastore this clone names",
		Long: `Copy every issue, person, sync record and release record kept in git
(refs/board/*) into the datastore board.datastore names, with the project's
numbers. Their packs copy as they are, so every id stays the same, and so
do the markers sync left on the forges. What the datastore has already is
left alone, so a second run copies nothing. The git refs aren't touched.`,
		Example: `  git config board.datastore postgres://me@db.example.org/board
  git board datastore copy`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDatastoreCopy(cmd.Context(), cmd.OutOrStdout())
		},
	})
	return cmd
}

func runDatastore(out io.Writer) error {
	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()
	c := b.Capabilities()
	yes := map[bool]string{true: "yes", false: "no"}
	fmt.Fprintf(out, "kept in   %s\n", b.Where())
	fmt.Fprintf(out, "replica   %s (pull, push, start over)\n", yes[c.Replica])
	fmt.Fprintf(out, "counter   %s (project numbers)\n", yes[c.Counter])
	return nil
}

func runDatastoreCopy(ctx context.Context, out io.Writer) error {
	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()
	n, err := b.CopyFromGit(ctx)
	if err != nil {
		return err
	}
	fmt.Fprintf(out, "%d copied into %s\n", n, b.Where())
	return nil
}
