package httpapi

import (
	"context"
	"fmt"

	"github.com/guilbep/git-board/next/app"
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
}

// workRows is the Work page: what needs you, what waits on others; team,
// what needs anyone and every other open issue.
func workRows(ctx context.Context, r workReq) (app.WorkGroups, error) {
	if r.Who != "" && r.Who != "me" && r.Who != "team" {
		return app.WorkGroups{}, invalid("who", fmt.Errorf("who %q: me or team", r.Who))
	}
	return board(ctx).Work(ctx, "", r.Who == "team")
}
