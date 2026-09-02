package main

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// apiServer holds what every handler needs: the parsed config, an open
// git-bug repo cache, and the server-wide "current user" for @me values (no
// per-request auth in v1 — see README).
type apiServer struct {
	cfg *config.Config
	b   *gitbugio.Backend
	me  string
}

// newMux wires the API routes plus a static handler (the built frontend) at
// "/". Go 1.22 method-pattern routing needs no router framework.
func newMux(s *apiServer, static http.Handler) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/views", s.handleViews)
	mux.HandleFunc("GET /api/config/fields", s.handleFields)
	mux.HandleFunc("GET /api/issues", s.handleListIssues)
	mux.HandleFunc("GET /api/issues/{id}", s.handleGetIssue)
	mux.HandleFunc("PATCH /api/issues/{id}", s.handlePatchIssue)
	mux.Handle("/", static)
	return mux
}

func (s *apiServer) handleViews(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.Views)
}

func (s *apiServer) handleFields(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.cfg.Fields)
}

// handleListIssues applies filter, then sort, then group — same semantics as
// `ls`/`view`. filter defaults to board.DefaultFilter, same as the CLI.
func (s *apiServer) handleListIssues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filterStr := q.Get("filter")
	if filterStr == "" {
		filterStr = board.DefaultFilter
	}

	issues, err := board.LoadMatching(s.b, s.cfg, filterStr, s.me)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	board.SortByField(issues, q.Get("sort"))
	writeJSON(w, http.StatusOK, board.BucketBy(issues, q.Get("group")))
}

func (s *apiServer) handleGetIssue(w http.ResponseWriter, r *http.Request) {
	issue, err := s.b.GetIssue(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, err)
		return
	}
	writeJSON(w, http.StatusOK, issue)
}

// handlePatchIssue applies a field=value assignment per body key, via the
// same fields.Apply the CLI's `set` command uses, then returns the updated
// issue so the frontend can reconcile from one source of truth.
func (s *apiServer) handlePatchIssue(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var assignments map[string]string
	if err := json.NewDecoder(r.Body).Decode(&assignments); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	names := make([]string, 0, len(assignments))
	for name := range assignments {
		names = append(names, name)
	}
	sort.Strings(names) // deterministic application order

	today := time.Now()
	for _, name := range names {
		if _, err := fields.Apply(s.b, s.cfg, id, name, assignments[name], s.me, today); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}
	}

	s.handleGetIssue(w, r)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
