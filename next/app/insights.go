package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/tools/insights"
)

// InsightsReport is what the Insights page draws and insights --json
// prints (next/tools/insights).
type InsightsReport = insights.Report

// InsightsReq asks for a replay: Field's flow (status) over the last
// Days (90), on View's issues (every issue when ""), open and closed.
type InsightsReq struct {
	Field string
	Days  int
	View  string
	Me    string    // what @me means in the view's filter
	Now   time.Time // zero: now
}

func usagef(format string, args ...any) error { return usage(fmt.Errorf(format, args...)) }

// Insights replays the issues' history, as v1's insights and its page.
func (b *Board) Insights(ctx context.Context, r InsightsReq) (InsightsReport, error) {
	if r.Field == "" {
		r.Field = "status"
	}
	if r.Days == 0 {
		r.Days = 90
	}
	if r.Days < 0 || r.Days > 730 {
		return InsightsReport{}, usagef("--days %d: 1 to 730", r.Days)
	}
	if r.Now.IsZero() {
		r.Now = time.Now()
	}
	var ids map[string]bool
	if r.View != "" {
		v, ok := b.Views[r.View]
		if !ok {
			return InsightsReport{}, usagef("unknown view %q", r.View)
		}
		// without its is: terms: the closed issues count (throughput,
		// cycle time)
		noIs := issues.Without(v.Filter, func(c issues.Condition) bool { return c.Kind == issues.QIs })
		list, err := b.Issues.List(ctx, issues.ListReq{Filter: noIs, Me: r.Me, Today: r.Now})
		if err != nil {
			return InsightsReport{}, err
		}
		ids = map[string]bool{}
		for _, is := range list {
			ids[is.ID] = true
		}
	}
	history, err := b.Issues.History(ctx, ids)
	if err != nil {
		return InsightsReport{}, err
	}
	sp, err := b.burnupSprint(ctx, r.Now)
	if err != nil {
		return InsightsReport{}, err
	}
	o := insights.Options{Field: r.Field, Workflow: b.Config.Schema.Fields[r.Field].Options, From: r.Now.AddDate(0, 0, -r.Days), To: r.Now, Sprint: sp}
	return insights.Compute(replayed(history), o), nil
}

func replayed(hs []issues.History) []insights.Issue {
	out := make([]insights.Issue, len(hs))
	for i, h := range hs {
		is := insights.Issue{ID: h.ID, Created: h.Created}
		for _, c := range h.Changes {
			is.Changes = append(is.Changes, insights.Change{At: c.At, Added: c.Added, Removed: c.Removed, Status: c.Status})
		}
		out[i] = is
	}
	return out
}

// iterationField is the field holding sprints: sprint, iteration, else a
// field of type iteration.
func (b *Board) iterationField() string {
	s := b.Config.Schema
	for _, name := range []string{"sprint", "iteration"} {
		if s.TypeOf(name, nil) == issues.TypeIteration {
			return name
		}
	}
	for name := range s.Fields {
		if s.TypeOf(name, nil) == issues.TypeIteration {
			return name
		}
	}
	return ""
}

func sprintOf(field string, w issues.SprintWindow) *insights.Sprint {
	return &insights.Sprint{Field: field, Slug: w.Slug, Title: cmpOr(w.Title, w.Slug), Start: w.Start, End: w.End}
}

func cmpOr(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// burnupSprint is the sprint the burn-up follows: the latest started one
// that has issues; nil for none.
func (b *Board) burnupSprint(ctx context.Context, now time.Time) (*insights.Sprint, error) {
	field := b.iterationField()
	if field == "" {
		return nil, nil
	}
	counts, err := b.Issues.LabelsIn(ctx, issues.ListReq{})
	if err != nil {
		return nil, err
	}
	used := map[string]bool{}
	for _, c := range counts {
		if v, ok := strings.CutPrefix(c.Label, field+":"); ok {
			used[v] = true
		}
	}
	windows := b.Config.Schema.Sprints.Windows()
	for i := len(windows) - 1; i >= 0; i-- {
		if w := windows[i]; !w.Start.After(now) && used[w.Slug] {
			return sprintOf(field, w), nil
		}
	}
	return nil, nil
}

// SprintReport is a sprint's report, Markdown to paste in Slack, an
// email or an issue.
type SprintReport struct {
	Sprint   string `json:"sprint"`
	Title    string `json:"title"`
	Markdown string `json:"markdown"`
}

// SprintReport writes the report of the sprint slug names; "" or
// "@current" for the burn-up's.
func (b *Board) SprintReport(ctx context.Context, slug string, now time.Time) (SprintReport, error) {
	if now.IsZero() {
		now = time.Now()
	}
	sp, err := b.burnupSprint(ctx, now)
	if err != nil {
		return SprintReport{}, err
	}
	if slug != "" && slug != "@current" {
		sp = nil
		if field := b.iterationField(); field != "" {
			for _, w := range b.Config.Schema.Sprints.Windows() {
				if w.Slug == slug {
					sp = sprintOf(field, w)
				}
			}
		}
	}
	if sp == nil {
		return SprintReport{}, &Error{Class: NotFound, Err: fmt.Errorf("no such sprint: [[sprints.list]] names the sprints, and an issue must be in one")}
	}
	list, err := b.Issues.List(ctx, issues.ListReq{Filter: "", Today: now})
	if err != nil {
		return SprintReport{}, err
	}
	s := b.Config.Schema
	label := sp.Field + ":" + sp.Slug
	ids := map[string]bool{}
	var mine []insights.ReportIssue
	for _, is := range list {
		in := false
		ri := insights.ReportIssue{ID: is.ID, Title: is.ShownTitle(), Open: is.Status == "open"}
		for _, l := range is.Labels {
			f, v, _ := strings.Cut(l, ":")
			switch {
			case l == label:
				in = true
			case f == "status":
				ri.Status = v
			case s.TypeOf(f, nil) == issues.TypePerson:
				ri.Assignees = append(ri.Assignees, v)
			}
		}
		if !in {
			continue
		}
		if len(is.Forges) > 0 {
			ri.Number, ri.URL = is.Forges[0].Number, is.Forges[0].URL
		}
		mine = append(mine, ri)
		ids[is.ID] = true
	}
	history, err := b.Issues.History(ctx, nil) // every issue: the findings are the project's
	if err != nil {
		return SprintReport{}, err
	}
	all := replayed(history)
	var theirs []insights.Issue
	for _, h := range all {
		if ids[h.ID] {
			theirs = append(theirs, h)
		}
	}
	rep := insights.Compute(all, insights.Options{Field: "status", Workflow: s.Fields["status"].Options, From: now.AddDate(0, 0, -90), To: now, Sprint: sp})
	// not the burn-up's: it counts closed issues only, the report also
	// those validated and still open, and says so itself
	var findings []string
	for _, k := range []string{"flow", "throughput", "cycle"} {
		if h := rep.Headlines[k]; h != "" {
			findings = append(findings, h)
		}
	}
	return SprintReport{Sprint: sp.Slug, Title: sp.Title, Markdown: insights.SprintReport(*sp, mine, theirs, findings, now)}, nil
}
