package cli

import (
	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/surface/tui"
)

func newTui() *cobra.Command {
	var me string
	cmd := &cobra.Command{
		Use:   "tui [filter]",
		Short: "Interactive scrollable, searchable issue browser",
		Long: `Interactive scrollable, searchable issue browser.

Keys: enter open detail, e edit a field (from detail), c quick-filter
by category, f structured filter (same grammar as "ls"; empty input
clears it), s sort by field e.g. "prio" or "prio desc" (same as --sort,
persists across a later f/c filter, empty input clears it), i/o
preview/apply the GitHub Project's fields as labels, as git board
project pull does, / bubbles/list's own fuzzy text search, esc/q back or
quit.

The optional [filter] argument uses the same grammar as "ls" and
narrows what's loaded before you start browsing.`,
		Example: `  git-board tui
  git-board tui "assignee:guilbep -status:validated"
  git-board tui "sprint:@current is:open" --me guilbep`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filter := issues.DefaultFilter
			if len(args) > 0 {
				filter = args[0]
			}
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			me := resolveMe(me)
			list, err := b.issues.List(cmd.Context(), issues.ListReq{Filter: filter, Me: me})
			if err != nil {
				b.Close()
				return err
			}
			// tui.Run owns the board from here, and closes it
			return tui.Run(list, filter, tui.Over(cmd.Context(), b.Board, me), b.Config.Schema, bridge(cmd, b))
		},
	}
	meFlag(cmd, &me)
	return cmd
}

// bridge is the TUI's GitHub Project bridge: none yet.
func bridge(*cobra.Command, *board) tui.Bridge { return nil }
