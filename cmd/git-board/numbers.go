package main

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/store"
)

func newNumbersCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "numbers",
		Short: "Number the issues origin has that have no project number yet",
		Long: `Number the issues origin has that have no project number yet,
through the counter on origin (refs/board/seq). An issue only in this
clone waits for its push: git board sync, or Push on the Sync page, push
it and number it. A project that doesn't number its issues: nothing.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := store.OpenCwd()
			if err != nil {
				return err
			}
			defer b.Close()
			if err := b.FetchNumbers(store.NumbersRemote); err != nil {
				return err
			}
			n, err := b.NumberIssues(store.NumbersRemote)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "numbered %d issue(s)\n", n)
			return nil
		},
	}
	cmd.AddCommand(newNumbersInitCmd())
	return cmd
}

func newNumbersInitCmd() *cobra.Command {
	var prefix string
	var start int
	cmd := &cobra.Command{
		Use:   "init --prefix CO2 --start 3052",
		Short: "Start the project's own numbers: the switch, for everyone",
		Long: `Start the project's own numbers, CO2-3052 and on, by pushing the
counter's first commit to origin. That push is the switch, for every clone.

GitHub numbers below start stay the issues' numbers (#2518 is CO2-2518);
the counter numbers the rest. start is one more than GitHub's highest
issue or pull request number:

  gh api 'repos/OWNER/REPO/issues?state=all&per_page=1' -q '.[0].number'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := store.OpenCwd()
			if err != nil {
				return err
			}
			defer b.Close()
			if err := b.InitNumbers(store.NumbersRemote, prefix, start); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "numbering as %s-N from %d, on %s\n", prefix, start, store.NumbersRemote)
			return nil
		},
	}
	cmd.Flags().StringVar(&prefix, "prefix", "", "the numbers' prefix, like CO2")
	cmd.Flags().IntVar(&start, "start", 0, "the first number the counter hands out")
	_ = cmd.MarkFlagRequired("prefix")
	_ = cmd.MarkFlagRequired("start")
	return cmd
}
