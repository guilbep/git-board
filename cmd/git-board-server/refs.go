package main

import (
	"net/http"
	"strings"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// issueRef is an issue an issue field names: its value as written (#12,
// a git-bug id), and the issue when one in the repository matches.
type issueRef struct {
	Value  string `json:"value"`
	ID     string `json:"id,omitempty"`
	Title  string `json:"title,omitempty"`
	Status string `json:"status,omitempty"`
}

// refersTo: #12 names an issue with that number on one of its forges; a
// hex prefix of 7 or more, its git-bug id.
func refersTo(value string, issue gitbugio.Issue) bool {
	if n, ok := strings.CutPrefix(value, "#"); ok {
		for _, f := range issue.Forges {
			if f.Number == n {
				return true
			}
		}
		return false
	}
	return len(value) >= 7 && strings.HasPrefix(issue.ID, value)
}

// handleRefs answers an issue's links through its issue fields (parent,
// blocked-by…): out, the issues it names; in, the issues naming it (its
// sub-issues, what it blocks).
func (s *apiServer) handleRefs(w http.ResponseWriter, r *http.Request) {
	me, err := s.b.GetIssue(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	issues, err := s.b.ListIssues()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	cfg := s.config()
	isRef := map[string]bool{}
	for name, vs := range s.labelValues() {
		isRef[name] = cfg.TypeOf(name, vs) == config.TypeIssue
	}
	out, in := map[string][]issueRef{}, map[string][]issueRef{}
	for _, l := range me.Labels {
		field, v, _ := strings.Cut(l, ":")
		if !isRef[field] {
			continue
		}
		ref := issueRef{Value: v}
		for _, o := range issues {
			if o.ID != me.ID && refersTo(v, o) {
				ref.ID, ref.Title, ref.Status = o.ID, o.Title, o.Status
				break
			}
		}
		out[field] = append(out[field], ref)
	}
	for _, o := range issues {
		if o.ID == me.ID {
			continue
		}
		for _, l := range o.Labels {
			if field, v, _ := strings.Cut(l, ":"); isRef[field] && refersTo(v, me) {
				in[field] = append(in[field], issueRef{Value: v, ID: o.ID, Title: o.Title, Status: o.Status})
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"out": out, "in": in})
}
