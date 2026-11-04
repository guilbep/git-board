package main

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
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
	var noOpen bool
	root := &cobra.Command{
		Use:   "git-board",
		Short: "Fields, sprints and views over issues kept in git",
		Long: `Fields, sprints and views over issues kept in git — a config-as-code
replacement for GitHub Projects. Field values (status, priority, sprint,
assignees, ...) live as namespaced labels; "git board project --help"
says how to keep them in step with an actual GitHub Project.

A sync moves the issues three places, each with its own commands: the
git remote ("git board fetch", "pull", "push"), the forges ("git board
forge pull|push|sync") and the GitHub Project ("git board project
pull|push|sync"). "git board sync" runs all three.

With no command, "git-board" opens this repository's web UI: in the
browser where its server already runs, else it starts one here on
localhost, port 8080 or the next free one, until Ctrl-C. Each repository
gets its own. "git-board serve" runs the server alone.

FILTER GRAMMAR (used by ls/view/tui):
  field:value             issue has the label field:value
  -field:value            issue does NOT have that label
  field:@current/@next/@previous   resolves against [sprints] in .git-board.toml
  field:@me               resolves to --me / GIT_BOARD_ME
  is:open / is:closed     issue status
  title:*glob*            glob match on title
  free text               substring match on title
  Conditions are AND'd together — combine as many as you like.`,
		Example: `  git-board                                    # this repository's web UI
  git-board ls                                 # open issues
  git-board ls "assignee:guilbep -status:validated"
  git-board ls "sprint:@current is:open" --group status
  git-board view backlog --md
  git-board set a1b2c3 status=ready prio=p1
  git-board tui "assignee:@me is:open" --me guilbep`,
		Version:       buildVersion(),
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runBoard(cmd.Context(), cmd.OutOrStdout(), !noOpen)
		},
	}
	root.Flags().BoolVar(&noOpen, "no-open", false, "don't open the browser")
	root.AddCommand(newInitCmd(), newViewCmd(), newNumbersCmd(), newLsCmd(), newSetCmd(), newGetCmd(), newTuiCmd(), newBridgeCmd(), newLintCmd(), newMoveCmd(), newCloseCmd(), newSyncCmd(), newSprintCmd(), newHtmlCmd(), newExportCmd(), newImportCmd(), newDatastoreCmd(), newBranchCmd(), newServeCmd(), newReleaseCmd(), newDeployCmd(), newVerifyCmd(), newSwitchCmd(), newPruneCmd(), newFetchCmd(), newPullCmd(), newPushCmd(), newForgeCmd(), newProjectCmd(), newResetCmd(),
		newMeCmd(), newNewCmd(), newCommentCmd())
	usageErrors(root)
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
	var force bool

	cmd := &cobra.Command{
		Use:   "set <id> field=value [field=value...]",
		Short: "Set one or more fields on an issue",
		Long: `Set one or more fields on an issue, in one change.

  field=value     a single field (status, prio) swaps to value; a multi
                  field (assignee) adds it
  field=+value    a multi field adds value
  field=-value    removes value, from a single field or a multi one
  field=          clears the field: every value it holds goes
  label=+name     adds the plain label name (bug, not kind:bug)
  label=-name     removes it

Values resolve first: @me is --me, GIT_BOARD_ME, else the forge login of
the person you write as (git board me); @current, @next and @previous are
sprints; a configured alias is its current name.

Each value is held to .git-board.toml before anything is written: one of
the field's options, a number for a number field, a YYYY-MM-DD date for a
date field, a sprint for a sprint field, a [labels] label when the project
lists them, and a field that is configured or already on some issue. One
that isn't is refused, and nothing is written; --force writes it anyway.
Removing and clearing are never refused: they are how a wrong value goes.

Setting status to its last option closes the issue, as "git board close"
does; setting it to another option on a closed issue reopens it.`,
		Example: `  git-board set a1b2c3 status=ready prio=p1
  git-board set a1b2c3 sprint=@next --me guilbep
  git-board set a1b2c3 assignee=+@me         # multi-cardinality: add yourself
  git-board set a1b2c3 assignee=-guilbep     # multi-cardinality: remove
  git-board set a1b2c3 prio=                 # clear the field
  git-board set a1b2c3 label=+regression     # a plain label
  git-board set a1b2c3 milestone=v1 --force  # a field nothing configures yet`,
		Args: cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSet(cmd.OutOrStdout(), args[0], args[1:], resolveMe(me), force)
		},
	}
	meFlag(cmd, &me)
	cmd.Flags().BoolVar(&force, "force", false, "write values .git-board.toml doesn't allow (an unknown field, a value outside the options)")
	return cmd
}

func newTuiCmd() *cobra.Command {
	var me string

	cmd := &cobra.Command{
		Use:   "tui [filter]",
		Short: "Interactive scrollable, searchable issue browser",
		Long: `Interactive scrollable, searchable issue browser.

Keys: enter open detail, e edit a field (from detail), c quick-filter
by category, f structured filter (same grammar as "ls" — see below —
empty input clears it), s sort by field e.g. "prio" or "prio desc"
(same as --sort, persists across a later f/c filter, empty input
clears it), i/o preview/apply the GitHub Project's fields as labels
(same pull/push mnemonic as git-bug's own termui; requires "git board
project configure" first),
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

	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	firstPull(os.Stderr, b)

	matched, err := board.LoadMatching(b, cfg, filterStr, me)
	if err != nil {
		b.Close()
		return err
	}
	// tui.Run owns b from here, and closes it when it returns.
	return tui.Run(matched, filterStr, b, cfg, me, time.Now())
}

// loadMatchingIssues opens the git-bug repo cache in cwd and returns issues
// matching filterStr — the shared core behind ls/view's table render and the
// tui's interactive list. The actual matching logic lives in internal/board
// so the server (internal/server) can reuse it too.
func loadMatchingIssues(cfg *config.Config, filterStr, me string) ([]store.Issue, error) {
	b, err := store.OpenCwd()
	if err != nil {
		return nil, err
	}
	defer b.Close()
	firstPull(os.Stderr, b)

	return board.LoadMatching(b, cfg, filterStr, me)
}

// firstPull brings a fresh clone its remote's board (store.PullIfEmpty)
// and says so; a remote it can't read is said, not an error: the board
// opens empty, with the reason
func firstPull(out io.Writer, b *store.Backend) {
	n, err := b.PullIfEmpty(store.BoardRemote)
	switch {
	case err != nil:
		fmt.Fprintf(out, "git-board: %v\n", err)
	case n > 0:
		fmt.Fprintf(out, "git-board: a fresh clone: pulled %d issue%s from %s\n", n, map[bool]string{true: "", false: "s"}[n == 1], store.BoardRemote)
	}
}

func runSet(out io.Writer, idPrefix string, args []string, me string, force bool) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}

	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()

	assignments, err := parseAssignments(b, cfg, args, me, force)
	if err != nil {
		return err
	}
	current, err := b.GetLabels(idPrefix)
	if err != nil {
		return err
	}
	labels, results, err := foldAssignments(cfg, current, assignments)
	if err != nil {
		return err
	}
	added, removed := labelDelta(current, labels)
	if len(added) > 0 || len(removed) > 0 {
		if _, err := b.ApplyLabelChange(idPrefix, added, removed); err != nil {
			return err
		}
	}
	state, err := statusInStep(b, cfg, idPrefix, assignments, labels)
	if err != nil {
		return err
	}
	for i, a := range assignments {
		line := idPrefix + ": " + describe(a, results[i])
		if i == len(assignments)-1 {
			line += state
		}
		fmt.Fprintln(out, line)
	}
	return nil
}

func runGet(out io.Writer, idPrefix string) error {
	b, err := store.OpenCwd()
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
