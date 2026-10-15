package insights

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

var day0 = time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC) // a Thursday

func at(days int, hour int) time.Time {
	return day0.AddDate(0, 0, days).Add(time.Duration(hour) * time.Hour)
}

// three issues: a moves backlog → in-progress → done and closes, b stays
// in backlog, c is created later in in-progress
func sample() []Issue {
	return []Issue{
		{ID: "a", Created: at(0, 0), Changes: []Change{
			{At: at(0, 0), Added: []string{"status:backlog", "sprint:s1"}},
			{At: at(2, 1), Added: []string{"status:in-progress"}, Removed: []string{"status:backlog"}},
			{At: at(5, 2), Added: []string{"status:done"}, Removed: []string{"status:in-progress"}},
			{At: at(5, 3), Status: "closed"},
		}},
		{ID: "b", Created: at(0, 1), Changes: []Change{{At: at(0, 1), Added: []string{"status:backlog", "sprint:s1"}}}},
		{ID: "c", Created: at(3, 0), Changes: []Change{{At: at(3, 0), Added: []string{"status:in-progress"}}}},
	}
}

func TestReplayCountsEachDay(t *testing.T) {
	r := Compute(sample(), Options{Field: "status", Workflow: []string{"backlog", "in-progress", "done"}, From: at(0, 0), To: at(6, 0)})
	if !reflect.DeepEqual(r.Values, []string{"backlog", "in-progress", "done"}) {
		t.Fatalf("values %v", r.Values)
	}
	if len(r.Days) != 7 || r.Days[0] != "2026-10-01" || r.Days[6] != "2026-10-07" {
		t.Fatalf("days %v", r.Days)
	}
	// end of each day
	want := map[string][]int{
		"backlog":     {2, 2, 1, 1, 1, 1, 1},
		"in-progress": {0, 0, 1, 2, 2, 1, 1},
		"done":        {0, 0, 0, 0, 0, 1, 1},
	}
	if !reflect.DeepEqual(r.All, want) {
		t.Errorf("all %v\nwant %v", r.All, want)
	}
	// the board that day: open issues only, a closed on day 5
	if r.Open["done"][6] != 0 || r.Open["in-progress"][3] != 2 {
		t.Errorf("open %v", r.Open)
	}
}

func TestThroughputCycleAndBurnup(t *testing.T) {
	r := Compute(sample(), Options{
		Field: "status", Workflow: []string{"backlog", "in-progress", "done"},
		From: at(0, 0), To: at(6, 0),
		Sprint: &Sprint{Field: "sprint", Slug: "s1", Title: "Sprint 1", Start: at(0, -9), End: at(14, -9)},
	})
	closed := 0
	for _, w := range r.Weeks {
		closed += w.Closed
	}
	if closed != 1 {
		t.Errorf("weeks %+v", r.Weeks)
	}
	// a started (in-progress) on day 2 01:00 and closed day 5 03:00
	if len(r.Cycle.Days) != 1 || r.Cycle.Days[0] < 3.08 || r.Cycle.Days[0] > 3.09 || r.Cycle.Start != "in-progress" {
		t.Errorf("cycle %+v", r.Cycle)
	}
	b := r.Burnup
	if b == nil || b.Scope[0] != 2 || b.Done[len(b.Done)-1] != 1 || b.Done[0] != 0 {
		t.Fatalf("burnup %+v", b)
	}
	for _, k := range []string{"flow", "throughput", "cycle", "burnup"} {
		if strings.TrimSpace(r.Headlines[k]) == "" {
			t.Errorf("no %s headline: %v", k, r.Headlines)
		}
	}
	if !strings.Contains(r.Headlines["burnup"], "1 of 2 done") {
		t.Errorf("burnup headline %q", r.Headlines["burnup"])
	}
}

// Without any issue going through a started value (statuses imported in
// one go), the cycle falls back to opened → closed, and says so.
func TestCycleFallsBackToLeadTime(t *testing.T) {
	issues := []Issue{
		{ID: "x", Created: at(0, 0), Changes: []Change{{At: at(0, 0), Added: []string{"status:done"}}, {At: at(4, 0), Status: "closed"}}},
	}
	r := Compute(issues, Options{Field: "status", Workflow: []string{"backlog", "in-progress", "done"}, From: at(0, 0), To: at(6, 0)})
	if len(r.Cycle.Days) != 1 || r.Cycle.Days[0] != 4 || r.Cycle.From != "opened" {
		t.Errorf("cycle %+v", r.Cycle)
	}
	if !strings.Contains(r.Headlines["cycle"], "of being opened") {
		t.Errorf("headline %q", r.Headlines["cycle"])
	}
}
