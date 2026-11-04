package main

import "github.com/spf13/cobra"

// addBeyond adds what goes beyond gh and glab (#88 P2): deleting, undo,
// labels, config, forge status and login, insights, and -C.
func addBeyond(root *cobra.Command) {
	root.AddCommand(newDeleteCmd(), newUndoCmd(), newLabelsCmd())
}
