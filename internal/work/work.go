// Package work is the Work page's rows: every open issue's cycle, grouped
// into what needs the viewer and what waits on others (the Inbox's shape,
// internal/inbox, with the cycle as its source).
package work

import (
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/cycle"
	"github.com/guilbep/git-board/internal/inbox"
	"github.com/guilbep/git-board/internal/store"
)

// Row is one open issue's cycle, slimmed for a list.
type Row struct {
	Issue    store.Issue `json:"issue"`
	Step     int         `json:"step"`
	Of       int         `json:"of"`
	Name     string      `json:"name"`
	Risk     string      `json:"risk,omitempty"`
	Done     bool        `json:"done"`
	Waits    cycle.Waits `json:"waits"`
	Mine     bool        `json:"mine"`
	Actions  []string    `json:"actions,omitempty"`
	Since    time.Time   `json:"since"`    // when the gate started waiting
	Involved bool        `json:"involved"` // the viewer's issue
	Question *Question   `json:"question,omitempty"`
	Branch   string      `json:"branch,omitempty"` // the evidence branch (agents match it)
}

// Question is an agent's question to the viewer: a comment with ask
// metadata, else the newest comment ending in ? the viewer hasn't
// answered.
type Question struct {
	ID   string       `json:"id"`
	Body string       `json:"body"`
	By   store.Person `json:"by"`
	At   time.Time    `json:"at"`
}

// Groups is Needs you and Waiting on others, oldest wait first.
type Groups struct {
	Needs   []Row `json:"needs"`
	Waiting []Row `json:"waiting"`
}

// RowOf is the thread's row, from its computed cycle.
func RowOf(th store.Thread, c cycle.Cycle, me inbox.Me) Row {
	r := Row{Issue: th.Issue, Step: c.Position(), Of: c.Of, Name: c.Name, Risk: c.Risk, Done: c.Done, Waits: c.Waits, Mine: c.Mine,
		Actions: c.Actions, Involved: Involved(th, me), Question: QuestionOf(th, me), Branch: c.Evidence.Branch}
	r.Since = Since(th)
	if r.Question != nil {
		r.Since = r.Question.At
	}
	return r
}

// Group sorts the rows into the two groups: a row needs the viewer when
// it carries a question, or its gate waits for the viewer, or (team) for
// anyone; otherwise it waits on others when team, or when the viewer's.
func Group(rows []Row, me string, team bool) Groups {
	g := Groups{Needs: []Row{}, Waiting: []Row{}}
	for _, r := range rows {
		switch {
		case r.Done:
		case r.Question != nil || (me != "" && !r.Waits.Anyone && strings.EqualFold(r.Waits.Login, me)) || (team && r.Waits.Anyone):
			g.Needs = append(g.Needs, r)
		case team || r.Involved:
			g.Waiting = append(g.Waiting, r)
		}
	}
	oldest := func(a, b Row) int {
		if c := a.Since.Compare(b.Since); c != 0 {
			return c
		}
		return strings.Compare(a.Issue.ID, b.Issue.ID)
	}
	slices.SortFunc(g.Needs, oldest)
	slices.SortFunc(g.Waiting, oldest)
	return g
}

// Since is when the thread's last gate change happened: its last event
// that isn't a plain comment (a comment with metadata is a sign-off, an
// approval, a run or a check), the creation otherwise.
// ponytail: the last state change, not the gate's own start; the step
// history (replaying the events through Compute) would give that.
func Since(th store.Thread) time.Time {
	var at time.Time
	for _, e := range th.Events {
		if e.Kind != "comment" || len(e.Meta) > 0 {
			at = e.At
		}
	}
	return at
}

// Involved tells whether the issue is the viewer's: its author, or a
// field naming the login (assignee, pm, reviewer…).
func Involved(th store.Thread, me inbox.Me) bool {
	if me.Is(th.Author) {
		return true
	}
	if me.Login == "" {
		return false
	}
	return slices.ContainsFunc(th.AllLabels(), func(l string) bool {
		_, v, ok := strings.Cut(l, ":")
		return ok && strings.EqualFold(v, me.Login)
	})
}

// QuestionOf is the newest question by someone else: a comment with ask
// metadata, else one ending in ?; nil once the viewer commented after it.
// ponytail: nothing marks an identity as an agent yet, so any other
// person's question counts; M5 names the agent on the worktree.
func QuestionOf(th store.Thread, me inbox.Me) *Question {
	var ask, mark *Question
	for _, e := range th.Events {
		if e.Kind != "comment" {
			continue
		}
		if me.Is(e.Author) {
			ask, mark = nil, nil // answered
			continue
		}
		q := &Question{ID: e.ID, Body: e.Body, By: e.Author, At: e.At}
		switch {
		case e.Meta["ask"] != "":
			ask = q
		case strings.HasSuffix(strings.TrimSpace(e.Body), "?"):
			mark = q
		}
	}
	if ask != nil {
		return ask
	}
	return mark
}
