package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newNumbers() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "numbers",
		Short: "Number the issues origin has that have no project number yet",
		Long: `Number the issues origin has that have no project number yet,
through the counter on origin (refs/board/seq). An issue only in this
clone waits for its push: git board sync, or Settings › Sync, push it
and number it. A project that doesn't number its issues: nothing.

A board kept in PostgreSQL numbers every issue through its database's
counter. One kept in Markdown files numbers them through
<folder>/.board/numbers, on the default branch alone (origin's HEAD, else
main, else master): on another branch this says why it waits, and the
branch's issues take their numbers once merged.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := open(cmd, false)
			if err != nil {
				return err
			}
			defer b.Close()
			n, err := b.Number(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "numbered %d issue(s)\n", n)
			return nil
		},
	}
	var prefix string
	var start int
	initCmd := &cobra.Command{
		Use:   "init --prefix CO2 --start 3052",
		Short: "Start the project's own numbers: the switch, for everyone",
		Long: `Start the project's own numbers, CO2-3052 and on, by pushing the
counter's first commit to origin. That push is the switch, for every clone.

GitHub numbers below start stay the issues' numbers (#2518 is CO2-2518);
the counter numbers the rest. start is one more than GitHub's highest
issue or pull request number:

  gh api 'repos/OWNER/REPO/issues?state=all&per_page=1' -q '.[0].number'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			b, err := open(cmd, false)
			if err != nil {
				return err
			}
			defer b.Close()
			where, err := b.InitNumbers(cmd.Context(), prefix, start)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "numbering as %s-N from %d, %s\n", prefix, start, where)
			return nil
		},
	}
	initCmd.Flags().StringVar(&prefix, "prefix", "", "the numbers' prefix, like CO2")
	initCmd.Flags().IntVar(&start, "start", 0, "the first number the counter hands out")
	_ = initCmd.MarkFlagRequired("prefix")
	_ = initCmd.MarkFlagRequired("start")
	cmd.AddCommand(initCmd)
	return cmd
}
