package server

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/insights"
)

// handleReport answers a sprint's report as Markdown (insights.SprintReport):
// the sprint the Insights burn-up follows (@current, the default: today's,
// else the latest with issues) or the one named; JSON for the page, or a
// .md download with format=md.
func (s *apiServer) handleReport(w http.ResponseWriter, r *http.Request) {
	cfg := s.config()
	now := time.Now()
	values := s.labelValues()
	sp := burnupSprint(cfg, now, values)
	if which := r.URL.Query().Get("sprint"); which != "" && which != "@current" {
		sp = namedSprint(cfg, which)
	}
	if sp == nil {
		writeError(w, http.StatusNotFound, errors.New("no such sprint: [[sprints.list]] names the sprints, and an issue must be in one"))
		return
	}
	all, err := s.b.ListIssues()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	persons := []string{}
	for name, vs := range values {
		if cfg.TypeOf(name, vs) == config.TypePerson {
			persons = append(persons, name)
		}
	}
	label := sp.Field + ":" + sp.Slug
	ids := map[string]bool{}
	var issues []insights.ReportIssue
	for _, is := range all {
		in := false
		ri := insights.ReportIssue{ID: is.ID, Title: is.ShownTitle(), Open: is.Status == "open"}
		for _, l := range is.Labels {
			f, v, _ := strings.Cut(l, ":")
			switch {
			case l == label:
				in = true
			case f == "status":
				ri.Status = v
			default:
				for _, p := range persons {
					if f == p {
						ri.Assignees = append(ri.Assignees, v)
					}
				}
			}
		}
		if !in {
			continue
		}
		if len(is.Forges) > 0 {
			ri.Number, ri.URL = is.Forges[0].Number, is.Forges[0].URL
		}
		issues = append(issues, ri)
		ids[is.ID] = true
	}
	history, err := s.b.History(nil) // every issue: the findings are the project's
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var mine []insights.Issue
	for _, h := range history {
		if ids[h.ID] {
			mine = append(mine, h)
		}
	}
	rep := insights.Compute(history, insights.Options{Field: "status", Workflow: cfg.Fields["status"].Options, From: now.AddDate(0, 0, -90), To: now, Sprint: sp})
	// not the burn-up's: it counts closed issues only, the report also
	// those validated and still open, and says so itself
	var findings []string
	for _, k := range []string{"flow", "throughput", "cycle"} {
		if h := rep.Headlines[k]; h != "" {
			findings = append(findings, h)
		}
	}
	md := insights.SprintReport(*sp, issues, mine, findings, now)
	if r.URL.Query().Get("format") == "md" {
		w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="sprint-report-%s.md"`, sp.Slug))
		_, _ = w.Write([]byte(md))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"sprint": sp.Slug, "title": sp.Title, "markdown": md})
}

// namedSprint is the sprint of [[sprints.list]] with that slug.
func namedSprint(cfg *config.Config, slug string) *insights.Sprint {
	field := iterationField(cfg)
	if field == "" {
		return nil
	}
	for _, w := range config.SprintWindows(cfg.Sprints) {
		if w.Slug == slug {
			return &insights.Sprint{Field: field, Slug: w.Slug, Title: or(w.Title, w.Slug), Start: w.Start, End: w.End}
		}
	}
	return nil
}
