package httpapi

import (
	"context"
	"fmt"
	"time"

	"github.com/guilbep/git-board/app"
)

type insightsReq struct {
	Field  string  `query:"field"`
	Days   int     `query:"days"`
	View   string  `query:"view"`
	Sprint *string `query:"sprint"`
}

// insights is route 51: the issues' history replayed (flow, throughput,
// cycle time, the current sprint's burn-up), as insights --json; with
// sprint, that sprint's report as Markdown (@current or "" for the
// burn-up's).
func insights(ctx context.Context, r insightsReq) (any, error) {
	b := board(ctx)
	if r.Sprint != nil {
		return b.SprintReport(ctx, *r.Sprint, time.Time{})
	}
	if r.Days < 0 || r.Days > 730 {
		return nil, invalid("days", fmt.Errorf("days %d: 1 to 730", r.Days))
	}
	if _, ok := b.Views[r.View]; r.View != "" && !ok {
		return nil, invalid("view", fmt.Errorf("unknown view %q", r.View))
	}
	return b.Insights(ctx, app.InsightsReq{Field: r.Field, Days: r.Days, View: r.View})
}
