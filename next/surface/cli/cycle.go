package cli

import (
	"cmp"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/app"
	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/tools/branches"
)

// The cycle's commands and their neighbours (v1's cmd/git-board
// verify.go, branch.go and sprint.go): verify, the experimental cycle's
// gate; branch, an issue's branch; sprint, the calendar.

func newVerify() *cobra.Command {
	var base string
	cmd := &cobra.Command{
		Use:   "verify [commit]",
		Short: "Tell whether a commit's checks are proven, as Mark ready for review does",
		Long: `Reads the [cycle.checks.*] and the signers file of the base branch's
.git-board.toml, then each check's signed results in refs/notes/ci/*
(docs/adr/0001), and prints its verdict at the commit (default HEAD).
Exits 1 when the commit isn't proven: the server's gate, without a server.
The development cycle's: experimental, on with [experimental] features =
["cycle"] in .git-board.toml.`,
		Example: `  git board verify
  git board verify fix/2518-retry --base origin/main`,
		Args:   cobra.MaximumNArgs(1),
		Hidden: true, // the cycle is experimental
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := open(cmd, false)
			if err != nil {
				return err
			}
			defer b.Close()
			commit := ""
			if len(args) == 1 {
				commit = args[0]
			}
			sha, base, g, err := b.Verify(cmd.Context(), base, commit)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "%s against the checks on %s\n", sha[:7], base)
			for _, c := range g.Checks {
				line := "not run"
				if c.Result != nil {
					line = fmt.Sprintf("%s, signed by %s on %s", c.Verdict, c.Signer, c.Result.At.Local().Format(time.DateTime))
				}
				fmt.Fprintf(out, "  %-12s %s\n", c.Name, line)
			}
			if !g.Proven {
				return fmt.Errorf("not proven: %s", g.Reason)
			}
			fmt.Fprintln(out, cmp.Or(g.Reason, "proven"))
			return nil
		},
	}
	cmd.Flags().StringVar(&base, "base", "", "the branch whose .git-board.toml and signers file rule (default: [branches] base, else origin's default branch, else main; origin/<name> when it exists)")
	return cmd
}

func newBranch() *cobra.Command {
	var create bool
	cmd := &cobra.Command{
		Use:   "branch <id>",
		Short: "Print the branch to create for an issue, and the command creating it; --create runs it",
		Long: `Print the branch to create for an issue, named from [branches] in
.git-board.toml, and the git command creating it from the base. It
creates nothing.

--create runs that command: it creates the branch from the base and
switches to it, as gh issue develop --checkout. Each branch goes in the
issue's folder (2518/1-..., 2518/2-...), so a second run creates the
issue's next branch.`,
		Example: `  git-board branch a1b2c3
  git-board branch a1b2c3 --create`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := open(cmd, false)
			if err != nil {
				return err
			}
			defer b.Close()
			th, err := b.issues.Show(cmd.Context(), args[0])
			if err != nil {
				return err
			}
			s, err := branches.Suggest(cmd.Context(), b.Code(), b.Config.Branches, th.Issue)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if !create {
				fmt.Fprintf(out, "%s\n%s\n", s.Suggested, s.Command)
				return nil
			}
			// in the working tree, as the user would type it: git switch
			// checks what the change would lose
			sw := exec.CommandContext(cmd.Context(), "git", "switch", "-c", s.Suggested, s.Start)
			var stderr strings.Builder
			sw.Stdout, sw.Stderr = cmd.ErrOrStderr(), &stderr
			if err := sw.Run(); err != nil {
				return fmt.Errorf("%s: %s", s.Command, strings.TrimSpace(stderr.String()))
			}
			fmt.Fprintln(out, s.Suggested)
			return nil
		},
	}
	cmd.Flags().BoolVar(&create, "create", false, "create the branch from the base and switch to it")
	return cmd
}

func newSprint() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sprint",
		Short: "List, resolve, create and report on configured sprints",
		Example: `  git-board sprint ls
  git-board sprint current
  git-board sprint new q1-hardening --title "Q1 Hardening" --start 2026-10-05
  git-board sprint report delivery`,
	}
	sprints := func(cmd *cobra.Command) (issues.Sprints, error) {
		b, err := open(cmd, false)
		if err != nil {
			return issues.Sprints{}, err
		}
		defer b.Close()
		return b.Config.Schema.Sprints, nil
	}
	ls := &cobra.Command{
		Use:     "ls",
		Short:   "List every configured sprint with its computed date range",
		Example: `  git-board sprint ls`,
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			sp, err := sprints(cmd)
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			windows := sp.Windows()
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
		},
	}
	cmd.AddCommand(ls)
	for which, none := range map[string]string{"current": "no sprint currently active", "next": "no next sprint configured", "previous": "no previous sprint"} {
		cmd.AddCommand(&cobra.Command{
			Use:     which,
			Short:   fmt.Sprintf("Show the sprint @%s resolves to", which),
			Example: fmt.Sprintf("  git-board sprint %s", which),
			Args:    cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				sp, err := sprints(cmd)
				if err != nil {
					return err
				}
				out := cmd.OutOrStdout()
				s := sp.Resolve(which, time.Now())
				if s == nil {
					fmt.Fprintln(out, none)
					return nil
				}
				for _, w := range sp.Windows() {
					if w.Slug == s.Slug {
						fmt.Fprintf(out, "%s: %s (%s .. %s)\n", w.Slug, w.Title, dateStr(w.Start), dateStr(w.End))
						return nil
					}
				}
				fmt.Fprintf(out, "%s: %s\n", s.Slug, s.Title)
				return nil
			},
		})
	}
	var title, start string
	add := &cobra.Command{
		Use:     "new <slug>",
		Short:   "Append a new sprint to .git-board.toml",
		Example: `  git-board sprint new q1-hardening --title "Q1 Hardening" --start 2026-10-05`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			at, err := time.Parse("2006-01-02", start)
			if err != nil {
				return fmt.Errorf("invalid --start %q, want YYYY-MM-DD: %w", start, err)
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			path, err := app.AppendSprint(cwd, args[0], title, at)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "added sprint %q %q starting %s to %s\n", args[0], title, dateStr(at), path)
			return nil
		},
	}
	add.Flags().StringVar(&title, "title", "", "sprint title (required)")
	add.Flags().StringVar(&start, "start", "", "sprint start date, YYYY-MM-DD (required)")
	_ = add.MarkFlagRequired("title")
	_ = add.MarkFlagRequired("start")
	report := &cobra.Command{
		Use:     "report <slug>",
		Short:   "Aggregate a sprint's issues by status",
		Example: `  git-board sprint report delivery`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := open(cmd, false)
			if err != nil {
				return err
			}
			defer b.Close()
			known := false
			for _, s := range b.Config.Schema.Sprints.List {
				known = known || s.Slug == args[0]
			}
			if !known {
				return fmt.Errorf("unknown sprint %q (run `git-board sprint ls` to see configured slugs)", args[0])
			}
			list, err := b.issues.List(cmd.Context(), issues.ListReq{Filter: "sprint:" + args[0]})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			tw := tabwriter.NewWriter(out, 0, 2, 2, ' ', 0)
			fmt.Fprintln(tw, "status\tcount")
			var order []string
			count := map[string]int{}
			for _, is := range list {
				key := "(none)"
				for _, l := range is.AllLabels() {
					if v, ok := strings.CutPrefix(l, "status:"); ok {
						key = v
						break
					}
				}
				if count[key] == 0 {
					order = append(order, key)
				}
				count[key]++
			}
			for _, k := range order {
				fmt.Fprintf(tw, "%s\t%d\n", k, count[k])
			}
			if err := tw.Flush(); err != nil {
				return err
			}
			fmt.Fprintf(out, "total: %d\n", len(list))
			return nil
		},
	}
	cmd.AddCommand(add, report)
	return cmd
}

func dateStr(t time.Time) string { return t.Format("2006-01-02") }
