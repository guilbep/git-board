package server

import (
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store/storetest"
)

func TestInsightsReplayTheRepository(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	s.cfg.Fields = map[string]config.Field{"status": {Single: true, Options: []string{"backlog", "in-progress", "done"}}}
	start := time.Now().AddDate(0, 0, -3)
	s.cfg.Sprints = config.SprintsConfig{LengthDays: 14, List: []config.Sprint{{Slug: "s1", Title: "Sprint 1", Start: start}}}
	a := storetest.NewIssue(t, s.b, "a")
	b := storetest.NewIssue(t, s.b, "b")
	for id, labels := range map[string][]string{a: {"status:in-progress", "sprint:s1"}, b: {"status:backlog", "sprint:s1"}} {
		if _, err := s.b.ApplyLabelChange(id, labels, nil); err != nil {
			t.Fatal(err)
		}
	}
	storetest.CloseIssue(t, s.b, a)

	var r struct {
		Values    []string
		Days      []string
		Open      map[string][]int
		Burnup    *struct{ Scope, Done []int }
		Headlines map[string]string
	}
	call(t, "GET", ts.URL+"/api/insights?field=status&days=7", nil, &r)
	if len(r.Days) != 8 || len(r.Values) != 2 || r.Open["backlog"][7] != 1 || r.Open["in-progress"][7] != 0 {
		t.Errorf("report %+v", r)
	}
	if r.Burnup == nil || r.Burnup.Scope[len(r.Burnup.Scope)-1] != 2 || r.Burnup.Done[len(r.Burnup.Done)-1] != 1 {
		t.Errorf("burnup %+v", r.Burnup)
	}
	if r.Headlines["flow"] == "" {
		t.Errorf("headlines %v", r.Headlines)
	}

	// a view's filter without its is: terms, a list of them too
	s.cfg.Views = map[string]config.View{"both": {Filter: "is:open,closed sprint:s1 -status:done"}}
	r.Open = nil
	call(t, "GET", ts.URL+"/api/insights?field=status&days=7&view=both", nil, &r)
	if len(r.Open["backlog"]) != 8 || r.Open["backlog"][7] != 1 {
		t.Errorf("the view's report %+v", r)
	}

	var sprints []struct{ Slug, Start, End string }
	call(t, "GET", ts.URL+"/api/sprints", nil, &sprints)
	if len(sprints) != 1 || sprints[0].Slug != "s1" || sprints[0].End == "" {
		t.Errorf("sprints %+v", sprints)
	}
}
