package server

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

func TestSprintReportOfTheCurrentSprint(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	today := time.Now().UTC().Truncate(24 * time.Hour)
	s.cfg.Sprints = config.SprintsConfig{LengthDays: 14, List: []config.Sprint{
		{Slug: "s1", Title: "Sprint 1", Start: today.AddDate(0, 0, -3)},
	}}
	a := gitbugiotest.NewImportedIssue(t, s.b, "Login", "https://github.com/o/r/issues/7")
	b := gitbugiotest.NewIssue(t, s.b, "Export")
	c := gitbugiotest.NewIssue(t, s.b, "Elsewhere")
	for id, labels := range map[string][]string{
		a: {"sprint:s1", "status:in-progress", "assignee:guilbep"},
		b: {"sprint:s1", "status:validated"},
		c: {"status:ready"},
	} {
		if _, err := s.b.ApplyLabelChange(id, labels, nil); err != nil {
			t.Fatal(err)
		}
	}
	gitbugiotest.CloseIssue(t, s.b, b)

	var rep struct{ Sprint, Title, Markdown string }
	if code := call(t, "GET", ts.URL+"/api/report", nil, &rep); code != http.StatusOK {
		t.Fatalf("report: %d", code)
	}
	for _, want := range []string{"# Sprint 1 (", "**1 of 2 done (50%)**", "## Done (1)", "## In progress (1)\n\n- [#7](https://github.com/o/r/issues/7) Login (@guilbep)"} {
		if !strings.Contains(rep.Markdown, want) {
			t.Errorf("missing %q in\n%s", want, rep.Markdown)
		}
	}
	if rep.Sprint != "s1" || strings.Contains(rep.Markdown, "Elsewhere") {
		t.Errorf("sprint %q, or an issue of no sprint listed:\n%s", rep.Sprint, rep.Markdown)
	}

	resp, err := http.Get(ts.URL + "/api/report?sprint=s1&format=md")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if !strings.HasPrefix(resp.Header.Get("Content-Type"), "text/markdown") || !strings.Contains(resp.Header.Get("Content-Disposition"), "sprint-report-s1.md") || !strings.HasPrefix(string(body), "# Sprint 1") {
		t.Errorf("download: %s %s %.40q", resp.Header.Get("Content-Type"), resp.Header.Get("Content-Disposition"), body)
	}
	if code := call(t, "GET", ts.URL+"/api/report?sprint=nope", nil, nil); code != http.StatusNotFound {
		t.Errorf("an unknown sprint: %d, want 404", code)
	}
}
