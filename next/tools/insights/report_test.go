package insights

import (
	"strings"
	"testing"
	"time"
)

func TestSprintReportReadsLikeAnUpdate(t *testing.T) {
	day := func(d int) time.Time { return time.Date(2026, 10, d, 9, 0, 0, 0, time.UTC) }
	sp := Sprint{Field: "sprint", Slug: "s12", Title: "Sprint 12", Start: day(5), End: day(19)}
	issues := []ReportIssue{
		{ID: "a1", Title: "Login [SSO] *fast*", Number: "12", URL: "https://github.com/o/r/issues/12", Assignees: []string{"guilbep"}, Status: "in-progress", Open: true},
		{ID: "b2", Title: "Export CSV", Number: "13", URL: "https://github.com/o/r/issues/13", Status: "validated", Open: false},
		{ID: "c3", Title: "Planner years", Status: "in-review", Open: true, Assignees: []string{"ana", "ben"}},
		{ID: "d4", Title: "Taxonomy gaps", Status: "blocked", Open: true},
		{ID: "e5", Title: "Docs", Open: true},
	}
	history := []Issue{
		{ID: "a1", Created: day(1), Changes: []Change{{At: day(4), Added: []string{"sprint:s12"}}}},
		{ID: "e5", Created: day(9), Changes: []Change{{At: day(9), Added: []string{"sprint:s12"}}}}, // added during the sprint
	}
	md := SprintReport(sp, issues, history, []string{"No issue closed in 4 weeks."}, day(12))
	for _, want := range []string{
		"# Sprint 12 (5 Oct – 18 Oct 2026)",
		"**1 of 5 done (20%)** · 7 days left · 1 added after the start",
		"- No issue closed in 4 weeks.",
		"## Done (1)\n\n- [#13](https://github.com/o/r/issues/13) Export CSV",
		"## In progress (1)\n\n- [#12](https://github.com/o/r/issues/12) Login \\[SSO\\] \\*fast\\* (@guilbep)",
		"## In review (1)\n\n- `c3` Planner years (@ana, @ben)",
		"## Blocked or waiting (1)",
		"## Not started (1)\n\n- `e5` Docs",
		"## Added after the start (1)\n\n- `e5` Docs, on 9 Oct",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("report misses %q\n%s", want, md)
		}
	}
	if over := SprintReport(sp, issues, history, nil, day(25)); !strings.Contains(over, "ended 18 Oct") {
		t.Errorf("a finished sprint:\n%s", over)
	}
}
