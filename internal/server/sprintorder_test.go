package server

import (
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/fields"
)

// Sprints follow their dates wherever a field's order counts: the server
// gives the sprint field its [[sprints.list]] slugs by start date, and
// says which one is current.
func TestSprintFieldTakesItsDates(t *testing.T) {
	t.Parallel()
	ts, s := newIssueServer(t)
	day := func(d int) time.Time { return time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, d) }
	s.cfg.Sprints = fields.Sprints{LengthDays: 14, List: []fields.Sprint{
		{Slug: "hypercare", Start: day(-3)},
		{Slug: "delivery", Start: day(-30)},
		{Slug: "post-hypercare", Start: day(10)},
	}}
	var got map[string]struct {
		Options []string `json:"options"`
		Current string   `json:"current"`
	}
	call(t, "GET", ts.URL+"/api/config/fields", nil, &got)
	if order := strings.Join(got["sprint"].Options, ","); order != "delivery,hypercare,post-hypercare" || got["sprint"].Current != "hypercare" {
		t.Errorf("sprint field %+v", got["sprint"])
	}

	// an order of your own stays
	s.cfg.Fields = map[string]fields.Field{"sprint": {Options: []string{"post-hypercare", "delivery"}}}
	call(t, "GET", ts.URL+"/api/config/fields", nil, &got)
	if order := strings.Join(got["sprint"].Options, ","); order != "post-hypercare,delivery" {
		t.Errorf("configured order replaced: %s", order)
	}
}
