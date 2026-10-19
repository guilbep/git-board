package server

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/branches"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/cycle"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/inbox"
	"github.com/guilbep/git-board/internal/work"
	"github.com/guilbep/git-board/internal/wtx"
)

// The Work page (internal/work): every open issue's cycle, grouped.
// GET /api/work?who=me|team (default me).
type workResponse struct {
	Me      inbox.Me     `json:"me"`
	Needs   []work.Row   `json:"needs"`
	Waiting []work.Row   `json:"waiting"`
	Agents  []workAgent  `json:"agents"`
	ToSend  []sendLine   `json:"to_send"` // newest first, at most 20
	Pending int          `json:"pending"` // every change the forges wait for
	Retro   retroSummary `json:"retro"`
	// CanStart tells wtx may start agents from this server; Reason, why
	// not (worktrees.go)
	CanStart bool   `json:"can_start"`
	Reason   string `json:"reason,omitempty"`
}

// workAgent is a linked worktree, the issue its branch names, and wtx's
// session in it.
type workAgent struct {
	Path    string          `json:"path"`
	Branch  string          `json:"branch"` // the head's short sha when detached
	Issue   *gitbugio.Issue `json:"issue,omitempty"`
	Step    int             `json:"step,omitempty"`
	Name    string          `json:"name,omitempty"`
	State   string          `json:"state"` // "Waits for you", "Waits for bob to review the PR", "No issue"
	At      time.Time       `json:"at"`    // the last commit
	Session *wtx.Session    `json:"session,omitempty"`
}

// sendLine is a change the forges wait for (Event.Pending).
type sendLine struct {
	Issue gitbugio.Issue `json:"issue"`
	What  string         `json:"what"` // "a comment", "the status", "the risk", "the title", "closed", "reopened", "a new issue"
	At    time.Time      `json:"at"`
}

// retroSummary counts the corrections flagged since the current sprint's
// start ([sprints] is the retro's clock).
type retroSummary struct {
	Sprint      string    `json:"sprint,omitempty"` // "" when none is current
	Since       time.Time `json:"since,omitempty"`
	Corrections int       `json:"corrections"`
}

func (s *apiServer) handleWork(w http.ResponseWriter, r *http.Request) {
	me, ok := s.inboxMe(w)
	if !ok {
		return
	}
	res, err := s.work(r.Context(), me, r.URL.Query().Get("who") == "team", time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	s.withSessions(r, &res)
	writeJSON(w, http.StatusOK, res)
}

func (s *apiServer) work(ctx context.Context, me inbox.Me, team bool, now time.Time) (workResponse, error) {
	cfg := s.config()
	stages := cfg.Stages()
	all, err := s.b.ListIssues()
	if err != nil {
		return workResponse{}, err
	}
	var open []string
	for _, i := range all {
		if i.Status == "open" {
			open = append(open, i.ID)
		}
	}
	threads, err := s.b.Threads(open)
	if err != nil {
		return workResponse{}, err
	}
	// one refresh for the page: the branches cycleOf would ask about
	var names []string
	for i := range threads {
		threads[i].Inherited = cfg.RolesFor(threads[i].Labels)
		n, err := s.pullNames(threads[i], stages)
		if err != nil {
			return workResponse{}, err
		}
		names = append(names, n...)
	}
	s.refreshPulls(ctx, names)
	// ponytail: Gather lists the branches once per issue; cache Branches()
	// per request when a board with hundreds of open issues shows it.
	rows := make([]work.Row, 0, len(threads))
	for _, th := range threads {
		in, err := cycle.Gather(s.b, cfg, th, me.Login, &s.pulls)
		if err != nil {
			return workResponse{}, err
		}
		rows = append(rows, work.RowOf(th, cycle.Compute(in), me))
	}
	g := work.Group(rows, me.Login, team)
	res := workResponse{Me: me, Needs: g.Needs, Waiting: g.Waiting, Retro: retroOf(cfg, threads, now)}
	if res.Agents, err = s.agents(rows); err != nil {
		return workResponse{}, err
	}
	res.ToSend, res.Pending = pendingLines(threads)
	return res, nil
}

// agents is one row per linked worktree, with the open issue its branch
// names (the evidence branch first, then the branch naming convention).
func (s *apiServer) agents(rows []work.Row) ([]workAgent, error) {
	wts, err := s.b.Worktrees()
	if err != nil {
		return nil, err
	}
	out := []workAgent{}
	for _, wt := range wts {
		a := workAgent{Path: wt.Path, Branch: wt.Branch, State: "No issue"}
		if a.Branch == "" && len(wt.Head) >= 7 {
			a.Branch = wt.Head[:7]
		}
		if c, err := s.b.ReadCommit(wt.Head); err == nil {
			a.At = c.Date
		}
		i := -1
		if wt.Branch != "" {
			i = slices.IndexFunc(rows, func(r work.Row) bool { return r.Branch == wt.Branch })
			if i < 0 {
				i = slices.IndexFunc(rows, func(r work.Row) bool { return branches.Names(wt.Branch, r.Issue) })
			}
		}
		if i >= 0 {
			r := rows[i]
			a.Issue, a.Step, a.Name, a.State = &r.Issue, r.Step, r.Name, agentState(r)
		}
		out = append(out, a)
	}
	return out, nil
}

func agentState(r work.Row) string {
	switch {
	case r.Done:
		return "Done"
	case r.Waits.Anyone:
		return "Waits for anyone to " + r.Waits.What
	case r.Mine:
		return "Waits for you"
	}
	return "Waits for " + r.Waits.Login + " to " + r.Waits.What
}

// pendingLines lists the changes the forges wait for, newest first, and counts
// them all.
func pendingLines(threads []gitbugio.Thread) ([]sendLine, int) {
	all := []sendLine{}
	for _, th := range threads {
		for _, e := range th.Events {
			if len(e.Pending) == 0 && len(e.EditPending) == 0 {
				continue
			}
			all = append(all, sendLine{Issue: th.Issue, What: sendWhat(e), At: e.At})
		}
	}
	slices.SortFunc(all, func(a, b sendLine) int { return b.At.Compare(a.At) })
	n := len(all)
	if n > 20 {
		all = all[:20]
	}
	return all, n
}

func sendWhat(e gitbugio.Event) string {
	edit := len(e.Pending) == 0
	switch e.Kind {
	case "created":
		if edit {
			return "the description"
		}
		return "a new issue"
	case "comment":
		if edit {
			return "an edited comment"
		}
		return "a comment"
	case "title":
		return "the title"
	case "status":
		if e.Status == "closed" {
			return "closed"
		}
		return "reopened"
	case "labels":
		for _, l := range append(slices.Clone(e.Added), e.Removed...) {
			if f, _, ok := strings.Cut(l, ":"); ok {
				return "the " + f
			}
		}
		return "the labels"
	}
	return "a change"
}

// retroOf counts the correction comments since the current sprint's start.
// ponytail: open issues only; M7 reads closed ones too.
func retroOf(cfg *config.Config, threads []gitbugio.Thread, now time.Time) retroSummary {
	sp, err := config.ResolveSprint(cfg.Sprints, "current", now)
	if err != nil || sp == nil {
		return retroSummary{}
	}
	r := retroSummary{Sprint: sp.Slug, Since: sp.Start}
	for _, th := range threads {
		for _, e := range th.Events {
			if e.Kind == "comment" && e.Meta["correction"] != "" && !e.At.Before(sp.Start) {
				r.Corrections++
			}
		}
	}
	return r
}

// withSessions reads wtx status once for the page: each agent row takes
// the session on its branch. When wtx can't run from here, or its status
// fails (no wtx.toml yet), the rows stay as they are and Reason says why.
func (s *apiServer) withSessions(r *http.Request, res *workResponse) {
	if _, res.Reason = s.wtxAllowed(r, false); res.Reason != "" {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
	defer cancel()
	out, err := wtx.Run(ctx, s.root, "status", "--json")
	var rows []wtx.Session
	if err == nil {
		rows, err = wtx.ParseStatus(out)
	}
	if err != nil {
		res.Reason = err.Error()
		return
	}
	res.CanStart = true
	for i, a := range res.Agents {
		if j := slices.IndexFunc(rows, func(row wtx.Session) bool { return row.Branch == a.Branch }); j >= 0 {
			res.Agents[i].Session = &rows[j]
		}
	}
}
