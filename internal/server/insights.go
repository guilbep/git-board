package server

import (
	"net/http"
	"strconv"
	"time"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/filter"
	"github.com/guilbep/git-board/internal/insights"
)

// handleInsights replays the issues' history (internal/insights) over the
// last days (90 by default) for field (status by default), on the issues
// of view when given (its filter, open or closed), with a burn-up of the
// current sprint.
func (s *apiServer) handleInsights(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	cfg := s.config()
	field := q.Get("field")
	if field == "" {
		field = "status"
	}
	days, _ := strconv.Atoi(q.Get("days"))
	if days <= 0 || days > 730 {
		days = 90
	}
	var ids map[string]bool
	if v, ok := cfg.Views[q.Get("view")]; ok && q.Get("view") != "" {
		// without its is: terms: insights look at the closed issues too
		// (throughput, cycle time)
		noIs := filter.Without(v.Filter, func(c filter.Condition) bool { return c.Kind == filter.KindIs })
		issues, err := board.LoadMatching(s.b, cfg, noIs, s.me)
		if err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		ids = map[string]bool{}
		for _, is := range issues {
			ids[is.ID] = true
		}
	}
	history, err := s.b.History(ids)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	now := time.Now()
	o := insights.Options{Field: field, Workflow: cfg.Fields[field].Options, From: now.AddDate(0, 0, -days), To: now}
	o.Sprint = burnupSprint(cfg, now, s.labelValues())
	writeJSON(w, http.StatusOK, insights.Compute(history, o))
}

// burnupSprint is the sprint the burn-up follows: today's, when an issue
// is in it, else the latest started one that has issues.
func burnupSprint(cfg *config.Config, now time.Time, values map[string][]string) *insights.Sprint {
	field := iterationField(cfg)
	if field == "" {
		return nil
	}
	used := map[string]bool{}
	for _, v := range values[field] {
		used[v] = true
	}
	windows := config.SprintWindows(cfg.Sprints)
	for i := len(windows) - 1; i >= 0; i-- {
		if w := windows[i]; !w.Start.After(now) && used[w.Slug] {
			return &insights.Sprint{Field: field, Slug: w.Slug, Title: or(w.Title, w.Slug), Start: w.Start, End: w.End}
		}
	}
	return nil
}

// iterationField is the field holding sprints: sprint, iteration, else a
// field of type iteration.
func iterationField(cfg *config.Config) string {
	for _, name := range []string{"sprint", "iteration"} {
		if cfg.TypeOf(name, nil) == config.TypeIteration {
			return name
		}
	}
	for name := range cfg.Fields {
		if cfg.TypeOf(name, nil) == config.TypeIteration {
			return name
		}
	}
	return ""
}

func or(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

// handleSprints answers [[sprints.list]] with each sprint's end: the
// roadmap layout draws them.
func (s *apiServer) handleSprints(w http.ResponseWriter, r *http.Request) {
	type sprint struct {
		Slug  string `json:"slug"`
		Title string `json:"title"`
		Start string `json:"start"`
		End   string `json:"end"`
	}
	res := []sprint{}
	for _, w := range config.SprintWindows(s.config().Sprints) {
		res = append(res, sprint{Slug: w.Slug, Title: or(w.Title, w.Slug), Start: w.Start.Format("2006-01-02"), End: w.End.Format("2006-01-02")})
	}
	writeJSON(w, http.StatusOK, res)
}
