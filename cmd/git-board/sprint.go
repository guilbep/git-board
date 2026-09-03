package main

import (
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

func newSprintCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sprint",
		Short: "List, resolve, create and report on configured sprints",
		Example: `  git-board sprint ls
  git-board sprint current
  git-board sprint new q1-hardening --title "Q1 Hardening" --start 2026-10-05
  git-board sprint report delivery`,
	}
	cmd.AddCommand(
		newSprintLsCmd(),
		newSprintResolveCmd("current"),
		newSprintResolveCmd("next"),
		newSprintResolveCmd("previous"),
		newSprintNewCmd(),
		newSprintReportCmd(),
	)
	return cmd
}

func newSprintLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "ls",
		Short:   "List every configured sprint with its computed date range",
		Example: `  git-board sprint ls`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			return runSprintLs(cmd.OutOrStdout(), cfg)
		},
	}
}

func runSprintLs(out io.Writer, cfg *config.Config) error {
	windows := config.SprintWindows(cfg.Sprints)
	if len(windows) == 0 {
		fmt.Fprintln(out, "no sprints configured")
		return nil
	}
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "slug\ttitle\tstart\tend")
	for _, w := range windows {
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", w.Slug, w.Title, dateStr(w.Start), dateStr(w.End))
	}
	return tw.Flush()
}

// noSprintMessage is what each resolve alias prints when config.ResolveSprint
// returns no match — not an error, a normal, expected outcome (e.g. no
// sprint has started yet, or there's nothing configured after the last one).
var noSprintMessage = map[string]string{
	"current":  "no sprint currently active",
	"next":     "no next sprint configured",
	"previous": "no previous sprint",
}

func newSprintResolveCmd(which string) *cobra.Command {
	return &cobra.Command{
		Use:     which,
		Short:   fmt.Sprintf("Show the sprint @%s resolves to", which),
		Example: fmt.Sprintf("  git-board sprint %s", which),
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			return runSprintResolve(cmd.OutOrStdout(), cfg, which, time.Now())
		},
	}
}

func runSprintResolve(out io.Writer, cfg *config.Config, which string, today time.Time) error {
	sprint, err := config.ResolveSprint(cfg.Sprints, which, today)
	if err != nil {
		return err
	}
	if sprint == nil {
		fmt.Fprintln(out, noSprintMessage[which])
		return nil
	}
	for _, w := range config.SprintWindows(cfg.Sprints) {
		if w.Slug == sprint.Slug {
			fmt.Fprintf(out, "%s: %s (%s .. %s)\n", w.Slug, w.Title, dateStr(w.Start), dateStr(w.End))
			return nil
		}
	}
	// ponytail: unreachable under normal operation — SprintWindows and
	// ResolveSprint both order the same cfg.Sprints.List the same way
	// (sortedSprints), so a resolved slug always has a matching window.
	// Only a hand-edited config with a duplicate slug could land here;
	// degrade to the bare slug/title rather than a scary "internal error".
	fmt.Fprintf(out, "%s: %s\n", sprint.Slug, sprint.Title)
	return nil
}

func newSprintNewCmd() *cobra.Command {
	var title, start string

	cmd := &cobra.Command{
		Use:     "new <slug>",
		Short:   "Append a new sprint to .git-board.toml",
		Example: `  git-board sprint new q1-hardening --title "Q1 Hardening" --start 2026-10-05`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSprintNew(cmd.OutOrStdout(), args[0], title, start)
		},
	}
	cmd.Flags().StringVar(&title, "title", "", "sprint title (required)")
	cmd.Flags().StringVar(&start, "start", "", "sprint start date, YYYY-MM-DD (required)")
	_ = cmd.MarkFlagRequired("title")
	_ = cmd.MarkFlagRequired("start")
	return cmd
}

func runSprintNew(out io.Writer, slug, title, start string) error {
	startDate, err := time.Parse("2006-01-02", start)
	if err != nil {
		return fmt.Errorf("invalid --start %q, want YYYY-MM-DD: %w", start, err)
	}
	if err := config.AppendSprint(configPath, slug, title, startDate); err != nil {
		return err
	}
	fmt.Fprintf(out, "added sprint %q %q starting %s to %s\n", slug, title, dateStr(startDate), configPath)
	return nil
}

func newSprintReportCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "report <slug>",
		Short:   "Aggregate a sprint's issues by status",
		Example: `  git-board sprint report delivery`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadConfig()
			if err != nil {
				return err
			}
			if !hasSprintSlug(cfg, args[0]) {
				return fmt.Errorf("unknown sprint %q (run `git-board sprint ls` to see configured slugs)", args[0])
			}
			issues, err := loadMatchingIssues(cfg, "sprint:"+args[0], "")
			if err != nil {
				return err
			}
			return runSprintReport(cmd.OutOrStdout(), issues)
		},
	}
}

func hasSprintSlug(cfg *config.Config, slug string) bool {
	for _, s := range cfg.Sprints.List {
		if s.Slug == slug {
			return true
		}
	}
	return false
}

func runSprintReport(out io.Writer, issues []gitbugio.Issue) error {
	tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
	fmt.Fprintln(tw, "status\tcount")
	for _, b := range board.BucketBy(issues, "status") {
		fmt.Fprintf(tw, "%s\t%d\n", b.Group, len(b.Issues))
	}
	if err := tw.Flush(); err != nil {
		return err
	}
	fmt.Fprintf(out, "total: %d\n", len(issues))
	return nil
}

func dateStr(t time.Time) string {
	return t.Format("2006-01-02")
}
