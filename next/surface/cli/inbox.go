package cli

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/next/domain/issues"
)

func newFollow() *cobra.Command {
	var me string
	var auto bool
	var js *jsonOut
	cmd := &cobra.Command{
		Use:   "follow <id>",
		Short: "Follow an issue: every change to it reaches your inbox",
		Long: `Follow an issue: every change to it reaches your inbox ("git board
status", the web UI's Inbox). "git board unfollow" undoes it: only a
mention of you, or being added or removed, reaches you then.

--auto goes back to the automatic choice: you follow the issues you are
in (you opened it, are assigned, commented, are mentioned, hold a role).

Your follows are yours, not the board's: they stay in this clone, as the
web UI's do. It prints whether you follow the issue now, and why.`,
		Example: `  git board follow a1b2c3d
  git board follow "#2528" --auto
  git board ls "follows:@me is:open"     # what you follow`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			yes := true
			set := &yes
			if auto {
				set = nil
			}
			return runFollow(cmd, args[0], set, resolveMe(me), js)
		},
	}
	cmd.Flags().BoolVar(&auto, "auto", false, "back to automatic: follow it while you are in it")
	js = addJSON(cmd, `print {"following", "reasons", "set"}`, jsonFields(issues.FollowState{}), true)
	meFlag(cmd, &me)
	return cmd
}

func newUnfollow() *cobra.Command {
	var me string
	var js *jsonOut
	cmd := &cobra.Command{
		Use:   "unfollow <id>",
		Short: "Stop following an issue: only what is addressed to you reaches your inbox",
		Long: `Stop following an issue, even one you are in: only a mention of you,
or being added or removed, reaches your inbox then. "git board follow"
follows it again, "follow --auto" goes back to the automatic choice.`,
		Example: `  git board unfollow a1b2c3d`,
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			no := false
			return runFollow(cmd, args[0], &no, resolveMe(me), js)
		},
	}
	js = addJSON(cmd, `print {"following", "reasons", "set"}`, jsonFields(issues.FollowState{}), true)
	meFlag(cmd, &me)
	return cmd
}

func runFollow(cmd *cobra.Command, name string, set *bool, me string, js *jsonOut) error {
	b, err := open(cmd, true)
	if err != nil {
		return err
	}
	defer b.Close()
	id, fs, err := b.issues.Follow(cmd.Context(), name, set, me)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if js.on() {
		return js.write(out, fs)
	}
	fmt.Fprintf(out, "%s: %s\n", shortID(id), fs.String())
	return nil
}

func newStatus() *cobra.Command {
	var me string
	var markRead bool
	var js *jsonOut
	cmd := &cobra.Command{
		Use:   "status",
		Short: "What needs you: your issues, and what changed in what you follow",
		Long: `What needs you, as gh status: the open issues assigned to you, then
your inbox, as the web UI's Inbox has it: what changed since you last
looked, in the last 14 days, by someone else. It is in two parts: the
issues you opened, and the others you follow or are mentioned in.

You are the person you write as (git board me); @me is --me,
GIT_BOARD_ME, else their forge login, else their name.

Nothing is marked read: the web UI's Inbox still shows the same.
--mark-read marks what it printed read, as the Inbox's Mark all read.

--json prints {"me", "assigned", "opened", "follows"}: "assigned" are
issues as ls --json prints them; "opened" and "follows" are inbox rows,
each {"issue", "reasons", "news", "who", "latest", "first"}.`,
		Example: `  git board status
  git board status --mark-read
  git board status --json | jq '.follows[].issue.title'`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := open(cmd, true)
			if err != nil {
				return err
			}
			defer b.Close()
			// what it prints is read before it is marked: the marking comes
			// after, as v1 prints first
			rep, err := b.issues.Status(cmd.Context(), resolveMe(me), markRead, time.Now())
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if js.on() {
				return js.write(out, rep)
			}
			writeStatus(out, rep)
			return nil
		},
	}
	js = addJSON(cmd, "render as JSON", jsonFields(issues.StatusReport{}), true)
	cmd.Flags().BoolVar(&markRead, "mark-read", false, "mark what it shows from your inbox read")
	meFlag(cmd, &me)
	return cmd
}

func writeStatus(out io.Writer, rep issues.StatusReport) {
	fmt.Fprintln(out, "Assigned to you")
	if len(rep.Assigned) == 0 {
		fmt.Fprintln(out, "  nothing open")
	}
	for _, is := range rep.Assigned {
		fmt.Fprintf(out, "  %s\n", statusIssue(is))
	}
	section := func(title string, rows []issues.InboxRow) {
		fmt.Fprintf(out, "\n%s\n", title)
		if len(rows) == 0 {
			fmt.Fprintln(out, "  nothing new")
		}
		for _, r := range rows {
			var who []string
			for _, p := range r.Who {
				who = append(who, p.Name)
			}
			fmt.Fprintf(out, "  %s\n    %s, by %s, %s\n", statusIssue(r.Issue), strings.Join(r.News, ", "), strings.Join(who, ", "), when(r.Latest))
		}
	}
	section("New in what you opened", rep.Opened)
	section("New in what you follow", rep.Follows)
}

// statusIssue is an issue on its line: id, number, title, status field.
func statusIssue(is issues.Issue) string {
	parts := []string{shortID(is.ID)}
	if is.Number != "" {
		parts = append(parts, is.Number)
	}
	parts = append(parts, is.ShownTitle())
	for _, l := range is.Labels {
		if v, ok := strings.CutPrefix(l, "status:"); ok {
			parts = append(parts, "["+v+"]")
			break
		}
	}
	if is.Status == "closed" {
		parts = append(parts, "(closed)")
	}
	return strings.Join(parts, "  ")
}
