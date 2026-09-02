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
	configPath    = ".git-board.toml"
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
		Use:           "git-board",
		Short:         "Fields, sprints and views over git-bug issues",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runLs(cmd.OutOrStdout(), defaultFilter, "", "", false, false, "")
		},
	}
	root.AddCommand(newViewCmd(), newLsCmd(), newSetCmd(), newGetCmd(), newTuiCmd())
	return root
}

func newLsCmd() *cobra.Command {
	var group, sort, me string
	var md, asJSON bool

	cmd := &cobra.Command{
		Use:   "ls [filter]",
		Short: "List issues matching a filter expression",
		Args:  cobra.MaximumNArgs(1),
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
		Args:  cobra.ExactArgs(1),
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
		Args:  cobra.MinimumNArgs(2),
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
		Args:  cobra.MaximumNArgs(1),
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
		Use:   "get <id>",
		Short: "Show fields (labels) for an issue",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runGet(cmd.OutOrStdout(), args[0])
		},
	}
}

func loadConfig() (*config.Config, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("%s not found or invalid: %w", configPath, err)
	}
	return cfg, nil
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
	return render(out, matched, group, sort, md, asJSON)
}

func runTui(filterStr, me string) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	matched, err := loadMatchingIssues(cfg, filterStr, me)
	if err != nil {
		return err
	}
	return tui.Run(matched, filterStr)
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
