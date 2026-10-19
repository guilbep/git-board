package server

import (
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
)

// Sprints follow their dates wherever a field's order counts: the server
// gives the sprint field its [[sprints.list]] slugs by start date, and
// says which one is current.
func TestSprintFieldTakesItsDates(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	day := func(d int) time.Time { return time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, d) }
	s.cfg.Sprints = config.SprintsConfig{LengthDays: 14, List: []config.Sprint{
		{Slug: "hypercare", Start: day(-3)},
		{Slug: "delivery", Start: day(-30)},
		{Slug: "post-hypercare", Start: day(10)},
	}}
	var fields map[string]struct {
		Options []string `json:"options"`
		Current string   `json:"current"`
	}
	call(t, "GET", ts.URL+"/api/config/fields", nil, &fields)
	if got := strings.Join(fields["sprint"].Options, ","); got != "delivery,hypercare,post-hypercare" || fields["sprint"].Current != "hypercare" {
		t.Errorf("sprint field %+v", fields["sprint"])
	}

	// an order of your own stays
	s.cfg.Fields = map[string]config.Field{"sprint": {Options: []string{"post-hypercare", "delivery"}}}
	call(t, "GET", ts.URL+"/api/config/fields", nil, &fields)
	if got := strings.Join(fields["sprint"].Options, ","); got != "post-hypercare,delivery" {
		t.Errorf("configured order replaced: %s", got)
	}
}
