package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/tui"
)

const (
	configPath    = config.FileName // at the root: where the tests write it
	defaultFilter = board.DefaultFilter
)

// meFlag is shared by ls/view/set: --me, falling back to GIT_BOARD_ME.
func meFlag(cmd *cobra.Command, target *string) {
	cmd.Flags().StringVar(target, "me", "", "current user, for @me filters/values (env GIT_BOARD_ME)")
}

func resolveMe(flag string) string {
	if flag != "" {
		return flag
	}
	return os.Getenv("GIT_BOARD_ME")
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "git-board",
		Short: "Fields, sprints and views over git-bug issues",
		Long: `Fields, sprints and views over git-bug issues — a config-as-code
replacement for GitHub Projects. Field values (status, priority, sprint,
assignees, ...) live as namespaced git-bug labels; run "git-board bridge
--help" to pull them from an actual GitHub Project.

FILTER GRAMMAR (used by ls/view/tui):
  field:value             issue has the label field:value
  -field:value            issue does NOT have that label
  field:@current/@next/@previous   resolves against [sprints] in .git-board.toml
  field:@me               resolves to --me / GIT_BOARD_ME
  is:open / is:closed     issue status
  title:*glob*            glob match on title
  free text               substring match on title
  Conditions are AND'd together — combine as many as you like.`,
		Example: `  git-board                                    # default: open issues
  git-board ls "assignee:guilbep -status:validated"
  git-board ls "sprint:@current is:open" --group status
  git-board view backlog --md
  git-board set a1b2c3 status=ready prio=p1
  git-board tui "assignee:@me is:open" --me guilbep`,
		Version:       buildVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLs(cmd.OutOrStdout(), defaultFilter, "", "", false, false, "")
		},
	}
	root.AddCommand(newViewCmd(), newLsCmd(), newSetCmd(), newGetCmd(), newTuiCmd(), newBridgeCmd(), newLintCmd(), newMoveCmd(), newCloseCmd(), newSyncCmd(), newSprintCmd(), newHtmlCmd(), newBranchCmd())
	return root
}

func newLsCmd() *cobra.Command {
	var group, sort, me string
	var md, asJSON bool

	cmd := &cobra.Command{
		Use:   "ls [filter]",
		Short: "List issues matching a filter",
		Example: `  git-board ls                                          # is:open (default)
  git-board ls "assignee:guilbep -status:validated"
  git-board ls "sprint:delivery is:open" --group status --sort "prio asc"
  git-board ls "assignee:@me is:open" --me guilbep
  git-board ls "title:*login*" --json
  git-board ls "#2528"                                  # quoted: # starts a shell comment`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filterStr := defaultFilter
			if len(args) > 0 {
				filterStr = args[0]
			}
			return runLs(cmd.OutOrStdout(), filterStr, group, sort, md, asJSON, resolveMe(me))
		},
	}
	cmd.Flags().StringVar(&group, "group", "", "group results by field")
	cmd.Flags().StringVar(&sort, "sort", "", `sort results, e.g. "prio asc"`)
	cmd.Flags().BoolVar(&md, "md", false, "render as markdown")
	cmd.Flags().BoolVar(&asJSON, "json", false, "render as JSON")
	meFlag(cmd, &me)
	return cmd
}

func newViewCmd() *cobra.Command {
	var me string
	var md, asJSON bool

	cmd := &cobra.Command{
		Use:   "view <name>",
		Short: "Render a named view from .git-board.toml",
		Example: `  git-board view backlog
  git-board view current-sprint --md
  git-board view mine --me guilbep --json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runView(cmd.OutOrStdout(), args[0], md, asJSON, resolveMe(me))
		},
	}
	cmd.Flags().BoolVar(&md, "md", false, "render as markdown")
	cmd.Flags().BoolVar(&asJSON, "json", false, "render as JSON")
	meFlag(cmd, &me)
	return cmd
}

func newSetCmd() *cobra.Command {
	var me string

	cmd := &cobra.Command{
		Use:   "set <id> field=value [field=value...]",
		Short: "Set one or more fields on an issue",
		Example: `  git-board set a1b2c3 status=ready prio=p1
  git-board set a1b2c3 sprint=@next --me guilbep
  git-board set a1b2c3 assignee=+guilbep     # multi-cardinality: add
  git-board set a1b2c3 assignee=-guilbep     # multi-cardinality: remove`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSet(cmd.OutOrStdout(), args[0], args[1:], resolveMe(me))
		},
	}
	meFlag(cmd, &me)
	return cmd
}

func newTuiCmd() *cobra.Command {
	var me string

	cmd := &cobra.Command{
		Use:   "tui [filter]",
		Short: "Interactive scrollable, searchable issue browser",
		Long: `Interactive scrollable, searchable issue browser.

Keys: enter open detail, e edit a field (from detail), g view body/
comments via "git bug bug show" (from detail; requires git-bug on PATH
as a git subcommand, not needed for anything else here), c quick-filter
by category, f structured filter (same grammar as "ls" — see below —
empty input clears it), s sort by field e.g. "prio" or "prio desc"
(same as --sort, persists across a later f/c filter, empty input
clears it), i/o preview/apply the GitHub Project bridge (same
pull/push mnemonic as git-bug's own termui, bound to
"bridge pull"/"...--apply" instead — requires "bridge configure" first),
/ bubbles/list's own fuzzy text search, esc/q back or quit.

The optional [filter] argument uses the same grammar as "ls" (see
"git-board --help") and narrows what's loaded before you even start
browsing. "f" re-applies that same structured grammar live from inside
the TUI; "/" is a separate, fuzzy text search on top of whatever's
currently shown — the two compose, they don't replace each other.`,
		Example: `  git-board tui
  git-board tui "assignee:guilbep -status:validated"
  git-board tui "sprint:@current is:open" --me guilbep`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filterStr := defaultFilter
			if len(args) > 0 {
				filterStr = args[0]
			}
			return runTui(filterStr, resolveMe(me))
		},
	}
	meFlag(cmd, &me)
	return cmd
}

func newGetCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "get <id>",
		Short:   "Show fields (labels) for an issue",
		Example: `  git-board get a1b2c3`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd.OutOrStdout(), args[0])
		},
	}
}

// loadConfig reads .git-board.toml: an empty config without one, an error
// ("parsing .git-board.toml: …") when it's invalid.
func loadConfig() (*config.Config, error) {
	return config.Load(config.Path())
}

func runLs(out io.Writer, filterStr, group, sort string, md, asJSON bool, me string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	return listAndRender(out, cfg, filterStr, group, sort, md, asJSON, me)
}

func runView(out io.Writer, name string, md, asJSON bool, me string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	view, ok := cfg.Views[name]
	if !ok {
		return fmt.Errorf("unknown view %q", name)
	}
	return listAndRender(out, cfg, view.Filter, view.Group, view.Sort, md, asJSON, me)
}

func listAndRender(out io.Writer, cfg *config.Config, filterStr, group, sort string, md, asJSON bool, me string) error {
	matched, err := loadMatchingIssues(cfg, filterStr, me)
	if err != nil {
		return err
	}
	return render(out, matched, group, sort, md, asJSON, cfg)
}

func runTui(filterStr, me string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	b, err := gitbugio.OpenCwd()
	if err != nil {
		return err
	}

	matched, err := board.LoadMatching(b, cfg, filterStr, me)
	if err != nil {
		b.Close()
		return err
	}
	// tui.Run owns b from here — including across a "g" close/reopen
	// cycle — and closes whichever backend is live when it returns.
	reopen := func() (tui.Backend, error) { return gitbugio.OpenCwd() }
	return tui.Run(matched, filterStr, b, cfg, me, time.Now(), reopen)
}

// loadMatchingIssues opens the git-bug repo cache in cwd and returns issues
// matching filterStr — the shared core behind ls/view's table render and the
// tui's interactive list. The actual matching logic lives in internal/board
// so cmd/git-board-server can reuse it too.
func loadMatchingIssues(cfg *config.Config, filterStr, me string) ([]gitbugio.Issue, error) {
	b, err := gitbugio.OpenCwd()
	if err != nil {
		return nil, err
	}
	defer b.Close()

	return board.LoadMatching(b, cfg, filterStr, me)
}

func runSet(out io.Writer, idPrefix string, assignments []string, me string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	b, err := gitbugio.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()

	today := time.Now()
	for _, assignment := range assignments {
		fieldName, value, ok := strings.Cut(assignment, "=")
		if !ok {
			return fmt.Errorf("invalid assignment %q, expected field=value", assignment)
		}
		label, err := fields.Apply(b, cfg, idPrefix, fieldName, value, me, today)
		if err != nil {
			return err
		}
		fmt.Fprintf(out, "%s: %s\n", idPrefix, label)
	}
	return nil
}

func runGet(out io.Writer, idPrefix string) error {
	b, err := gitbugio.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()

	labels, err := b.GetLabels(idPrefix)
	if err != nil {
		return err
	}
	for _, l := range labels {
		fmt.Fprintln(out, l)
	}
	return nil
}
