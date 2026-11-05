package httpapi

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/guilbep/git-board/next/app"
)

// The development cycle (experimental; docs/rewrite/api.md, routes 49
// and 50): an issue's step and its evidence, its actions, and the Work
// page's rows. Off, each answers 404 experimental.

type cycleReq struct {
	ID string `path:"id"`
}

func cycleReply(v app.CycleView) Reply {
	return Reply{ETag: v.Version, Body: v}
}

func issueCycle(ctx context.Context, r cycleReq) (Reply, error) {
	v, err := board(ctx).Cycle(ctx, r.ID, "")
	return cycleReply(v), err
}

// actReq is one action; each reads the fields its action names (api.md).
type actReq struct {
	ID       string `path:"id"`
	Action   string `path:"action"`
	IfMatch  string `header:"If-Match"`
	Risk     string `json:"risk"`
	Verdict  string `json:"verdict"`
	On       string `json:"on"`
	Body     string `json:"body"`
	Output   string `json:"output"`
	Commit   string `json:"commit"`
	Index    *int   `json:"index"`
	Why      string `json:"why"`
	State    string `json:"state"`
	Path     string `json:"path"`
	Line     int    `json:"line"`
	Side     string `json:"side"`
	ReplyTo  string `json:"reply_to"`
	Comment  string `json:"comment"`
	Resolved bool   `json:"resolved"`
	Branch   string `json:"branch"`
	Number   int    `json:"number"`
	Title    string `json:"title"`
	SHA      string `json:"sha"`
}

// cycleAct is route 50: the issue's writes compare-and-swap from the
// version If-Match names; the forge's (a pull request's) don't read it.
func cycleAct(ctx context.Context, r actReq) (Reply, error) {
	if !board(ctx).CycleOn() {
		return Reply{}, app.ErrCycleOff
	}
	ctx, id, err := expect(ctx, r.ID, r.IfMatch)
	if err != nil {
		return Reply{}, err
	}
	v, err := board(ctx).Act(ctx, app.CycleAct{Name: id, Action: r.Action, Risk: r.Risk, Verdict: r.Verdict, On: r.On, Body: r.Body,
		Output: r.Output, Commit: r.Commit, Index: r.Index, Why: r.Why, State: r.State, Path: r.Path, Line: r.Line, Side: r.Side,
		ReplyTo: r.ReplyTo, Comment: r.Comment, Resolved: r.Resolved, Branch: r.Branch, Number: r.Number, Title: r.Title, SHA: r.SHA})
	if err != nil {
		return Reply{}, err
	}
	return cycleReply(v), nil
}

type workReq struct {
	Who string `query:"who"` // me (default) or team
	// Limit pages the groups, as the list routes: each group's first
	// rows, its total and the cursor after them; Group and Cursor, one
	// group's next page. None: every row, unpaged.
	Limit  *int   `query:"limit"`
	Group  string `query:"group"` // needs or waiting
	Cursor string `query:"cursor"`
}

// workPage is the Work page's rows; paged, each group's total and the
// cursor of its next page.
type workPage struct {
	app.WorkGroups
	NeedsTotal   *int   `json:"needs_total,omitempty"`
	WaitingTotal *int   `json:"waiting_total,omitempty"`
	NeedsNext    string `json:"needs_next,omitempty"`
	WaitingNext  string `json:"waiting_next,omitempty"`
}

// workRows is the Work page: what needs you, what waits on others; team,
// what needs anyone and every other open issue.
func workRows(ctx context.Context, r workReq) (workPage, error) {
	if r.Who != "" && r.Who != "me" && r.Who != "team" {
		return workPage{}, invalid("who", fmt.Errorf("who %q: me or team", r.Who))
	}
	if r.Group != "" && r.Group != "needs" && r.Group != "waiting" {
		return workPage{}, invalid("group", fmt.Errorf("group %q: needs or waiting", r.Group))
	}
	if r.Limit == nil && (r.Group != "" || r.Cursor != "") {
		return workPage{}, invalid("limit", errors.New("group and cursor page the rows: give a limit"))
	}
	if r.Cursor != "" && r.Group == "" {
		return workPage{}, invalid("group", errors.New("a cursor is a group's: give the group"))
	}
	if r.Limit != nil && (*r.Limit < 1 || *r.Limit > maxLimit) {
		return workPage{}, invalid("limit", fmt.Errorf("limit %d: from 1 to %d", *r.Limit, maxLimit))
	}
	g, err := board(ctx).Work(ctx, "", r.Who == "team")
	if err != nil || r.Limit == nil {
		return workPage{WorkGroups: g}, err
	}
	p := workPage{}
	pageOf := func(rows []app.WorkRow, total **int, next *string) ([]app.WorkRow, error) {
		n := len(rows)
		*total = &n
		c, err := parseCursor(r.Cursor)
		if err != nil {
			return nil, err
		}
		// the rows are oldest wait first, then by id (work.Group)
		start := resume(rows, c, func(row app.WorkRow) bool {
			return cmp.Or(row.Since.Compare(c.Since), strings.Compare(row.Issue.ID, c.ID)) > 0
		})
		end := min(len(rows), start+*r.Limit)
		if end < len(rows) {
			last := rows[end-1]
			*next = cursorAt{Since: last.Since, ID: last.Issue.ID}.String()
		}
		return rows[start:end], nil
	}
	p.Needs, p.Waiting = []app.WorkRow{}, []app.WorkRow{}
	if r.Group != "waiting" {
		if p.Needs, err = pageOf(g.Needs, &p.NeedsTotal, &p.NeedsNext); err != nil {
			return workPage{}, err
		}
	}
	if r.Group != "needs" {
		if p.Waiting, err = pageOf(g.Waiting, &p.WaitingTotal, &p.WaitingNext); err != nil {
			return workPage{}, err
		}
	}
	return p, nil
}
