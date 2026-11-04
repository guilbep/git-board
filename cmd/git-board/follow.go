package main

import (
	"cmp"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/core/model"
	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/inbox"
	"github.com/guilbep/git-board/internal/store"
)

// The inbox's three choices for an issue, as the web UI's bell offers
// them (internal/inbox): Following, Not following, Automatic.

func newFollowCmd() *cobra.Command {
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
			return runFollow(cmd.OutOrStdout(), args[0], set, resolveMe(me), js)
		},
	}
	cmd.Flags().BoolVar(&auto, "auto", false, "back to automatic: follow it while you are in it")
	js = addJSON(cmd, `print {"following", "reasons", "set"}`, jsonFields(followState{}), true)
	meFlag(cmd, &me)
	return cmd
}

func newUnfollowCmd() *cobra.Command {
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
			return runFollow(cmd.OutOrStdout(), args[0], &no, resolveMe(me), js)
		},
	}
	js = addJSON(cmd, `print {"following", "reasons", "set"}`, jsonFields(followState{}), true)
	meFlag(cmd, &me)
	return cmd
}

// followState is whether you follow an issue, and why: the web UI's
// GET /api/inbox/follow/{id}.
type followState struct {
	Following bool     `json:"following"`
	Reasons   []string `json:"reasons"`
	Set       *bool    `json:"set"` // what you chose; null: automatic
}

func runFollow(out io.Writer, idPrefix string, set *bool, me string, js *jsonOut) error {
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
	th, err := b.Thread(idPrefix)
	if err != nil {
		return err
	}
	st, err := inbox.Load(b.Storage(), m.ID)
	if err != nil {
		return err
	}
	st.SetFollow(th.ID, set)
	if err := inbox.Save(b.Storage(), m.ID, st); err != nil {
		return err
	}
	fs, err := followOf(b, cfg, m, st, th)
	if err != nil {
		return err
	}
	if js.on() {
		return js.write(out, fs)
	}
	fmt.Fprintf(out, "%s: %s\n", shortID(th.ID), fs.String())
	return nil
}

// String is the state as follow prints it: "following (author, assignee)",
// "not following, by choice".
func (f followState) String() string {
	s := "not following"
	if f.Following {
		s = "following"
	}
	if len(f.Reasons) > 0 {
		s += " (" + strings.Join(f.Reasons, ", ") + ")"
	}
	switch {
	case f.Set == nil:
		s += ", automatic"
	case !*f.Set:
		s += ", by choice"
	}
	return s
}

// inboxMe is who your inbox is for: the person you write as, and @me's
// login (--me, GIT_BOARD_ME, else their forge login or name).
func inboxMe(b *store.Backend, me string) (inbox.Me, error) {
	p, ok, err := b.Me()
	if err != nil {
		return inbox.Me{}, err
	}
	if !ok {
		return inbox.Me{}, store.ErrNoMe
	}
	return inbox.Me{ID: p.ID, Login: cmp.Or(me, b.MyName())}, nil
}

// followOf is whether m follows th with st, and why, as the server's
// follow answers it.
func followOf(b *store.Backend, cfg *config.Config, m inbox.Me, st inbox.State, th model.Thread) (followState, error) {
	th.Inherited = cfg.RolesFor(th.Labels)
	inViews, _, err := board.FollowedViews(b, cfg, st.Views, m.Login, []model.Issue{th.Issue}, time.Now())
	if err != nil {
		return followState{}, err
	}
	following, reasons, set := inbox.Follows(m, st, th, inViews[th.ID], cfg)
	if reasons == nil {
		reasons = []string{}
	}
	return followState{Following: following, Reasons: reasons, Set: set}, nil
}
