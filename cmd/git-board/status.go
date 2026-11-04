package main

import (
	"errors"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/core/inbox"
	"github.com/guilbep/git-board/core/model"
	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/store"
)

func newStatusCmd() *cobra.Command {
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
			return runNeedsYou(cmd.OutOrStdout(), resolveMe(me), js, markRead, time.Now())
		},
	}
	js = addJSON(cmd, "render as JSON", jsonFields(statusReport{}), true)
	cmd.Flags().BoolVar(&markRead, "mark-read", false, "mark what it shows from your inbox read")
	meFlag(cmd, &me)
	return cmd
}

// statusReport is what status prints.
type statusReport struct {
	Me       inbox.Me      `json:"me"`
	Assigned []model.Issue `json:"assigned"`
	Opened   []inbox.Row   `json:"opened"`
	Follows  []inbox.Row   `json:"follows"`
}

func runNeedsYou(out io.Writer, me string, js *jsonOut, markRead bool, now time.Time) error {
	cfg, err := loadConfig()
	if err != nil {
		return err
	}
	b, err := store.OpenCwd()
	if err != nil {
		return err
	}
	defer b.Close()
	m, err := inboxMe(b, me)
	if err != nil {
		return err
	}
	if m.Login == "" {
		return errors.New("your person has neither a name nor a forge login, so no field and no mention can name you: give them a name, or pass --me")
	}
	st, err := inbox.Load(b.Storage(), m.ID)
	if err != nil {
		return err
	}

	all, err := b.ListIssues()
	if err != nil {
		return err
	}
	rep := statusReport{Me: m, Assigned: []model.Issue{}, Opened: []inbox.Row{}, Follows: []inbox.Row{}}
	assigned, err := board.Match(b, cfg, "assignee:@me is:open", m.Login, slices.Clone(all), now)
	if err != nil {
		return err
	}
	sort.SliceStable(assigned, func(i, j int) bool { return assigned[i].UpdatedAt.After(assigned[j].UpdatedAt) })
	rep.Assigned = append(rep.Assigned, assigned...)

	// the inbox, as the server gathers it (internal/server/inbox.go)
	since := now.Add(-inbox.Window)
	var window []string
	for _, is := range all {
		if !is.UpdatedAt.Before(since) {
			window = append(window, is.ID)
		}
	}
	threads, err := b.Threads(window)
	if err != nil {
		return err
	}
	for i := range threads {
		threads[i].Inherited = cfg.RolesFor(threads[i].Labels)
	}
	inViews, _, err := board.FollowedViews(b, cfg, st.Views, m.Login, all, now)
	if err != nil {
		return err
	}
	for _, r := range inbox.Rows(m, st, threads, inViews, cfg.IssueSchema(), since) {
		if slices.Contains(r.Reasons, "author") {
			rep.Opened = append(rep.Opened, r)
		} else {
			rep.Follows = append(rep.Follows, r)
		}
	}

	if js.on() {
		err = js.write(out, rep)
	} else {
		writeStatus(out, rep)
	}
	if err != nil || !markRead || len(rep.Opened)+len(rep.Follows) == 0 {
		return err
	}
	return markSeen(b, m.ID, threads, append(slices.Clone(rep.Opened), rep.Follows...), since)
}

// markSeen marks every event of the rows' issues seen, as the Inbox's
// Mark all read, and forgets the issues out of the window.
func markSeen(b *store.Backend, meID string, threads []model.Thread, rows []inbox.Row, since time.Time) error {
	st, err := inbox.Load(b.Storage(), meID)
	if err != nil {
		return err
	}
	keep := map[string]bool{}
	for _, th := range threads {
		keep[th.ID] = true
		if slices.ContainsFunc(rows, func(r inbox.Row) bool { return r.Issue.ID == th.ID }) {
			st.MarkSeen(th)
		}
	}
	st.Prune(keep)
	return inbox.Save(b.Storage(), meID, st)
}

func writeStatus(out io.Writer, rep statusReport) {
	fmt.Fprintln(out, "Assigned to you")
	if len(rep.Assigned) == 0 {
		fmt.Fprintln(out, "  nothing open")
	}
	for _, is := range rep.Assigned {
		fmt.Fprintf(out, "  %s\n", statusIssue(is))
	}
	section := func(title string, rows []inbox.Row) {
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
func statusIssue(is model.Issue) string {
	parts := []string{shortID(is.ID)}
	if is.Number != "" {
		parts = append(parts, is.Number)
	}
	parts = append(parts, is.ShownTitle())
	if s := currentStatusValue(is.Labels); s != "" {
		parts = append(parts, "["+s+"]")
	}
	if is.Status == "closed" {
		parts = append(parts, "(closed)")
	}
	return strings.Join(parts, "  ")
}
