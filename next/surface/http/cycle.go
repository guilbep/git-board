package httpapi

import (
	"context"
	"errors"
	"fmt"

	"github.com/guilbep/git-board/next/app"
	"github.com/guilbep/git-board/next/domain/issues"
)

// The development cycle (experimental; docs/rewrite/api.md, route 49):
// an issue's step and its evidence, and the Work page's rows. Off, both
// answer 404 experimental.

type cycleReq struct {
	ID string `path:"id"`
}

func issueCycle(ctx context.Context, r cycleReq) (app.IssueCycle, error) {
	return board(ctx).Cycle(ctx, r.ID, "")
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
		list := make([]issues.Issue, len(rows))
		for i, row := range rows {
			list[i] = row.Issue
		}
		start, err := after(list, r.Cursor)
		if err != nil {
			return nil, err
		}
		end := min(len(rows), start+*r.Limit)
		if end < len(rows) {
			*next = cursor(end, rows[end-1].Issue.ID)
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
