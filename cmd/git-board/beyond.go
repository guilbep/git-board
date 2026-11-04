package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// addBeyond adds what goes beyond gh and glab (#88 P2): deleting, undo,
// labels, config, forge status and login, insights, and -C.
func addBeyond(root *cobra.Command) {
	root.AddCommand(newDeleteCmd(), newUndoCmd(), newLabelsCmd(), newConfigCmd(), newInsightsCmd())
	forgeAuth(root)
	var dir string
	root.PersistentFlags().StringVarP(&dir, "directory", "C", "", "run as if started in this directory, as git -C does")
	root.PersistentPreRunE = func(*cobra.Command, []string) error {
		if dir == "" {
			return nil
		}
		if err := os.Chdir(dir); err != nil {
			return usageError{fmt.Errorf("-C %s: %w", dir, err)}
		}
		return nil
	}
}
