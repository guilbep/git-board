package httpapi

import (
	"context"
	"fmt"

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
}

// workRows is the Work page: what needs you, what waits on others; team,
// what needs anyone and every other open issue.
func workRows(ctx context.Context, r workReq) (app.WorkGroups, error) {
	if r.Who != "" && r.Who != "me" && r.Who != "team" {
		return app.WorkGroups{}, invalid("who", fmt.Errorf("who %q: me or team", r.Who))
	}
	return board(ctx).Work(ctx, "", r.Who == "team")
}
