package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newGet is v1's get: an issue's own labels, one a line.
func newGet() *cobra.Command {
	return &cobra.Command{
		Use:     "get <id>",
		Short:   "Show fields (labels) for an issue",
		Example: `  git-board get a1b2c3`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := open(cmd, false)
			if err != nil {
				return err
			}
			defer b.Close()
			is, err := b.issues.Get(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			for _, l := range is.Labels {
				fmt.Fprintln(cmd.OutOrStdout(), l)
			}
			return nil
		},
	}
}
