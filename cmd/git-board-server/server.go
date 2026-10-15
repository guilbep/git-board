package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/git-bug/git-bug/bridge/core"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/ghbridge"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/uploads"
)

// apiServer holds what every handler needs: the parsed config, an open
// git-bug repo cache, and the server-wide "current user" for @me values (no
// per-request auth in v1 — see README).
//
// ponytail: one *gitbugio.Backend shared across every request goroutine, no
// serialization beyond git-bug's own per-entity locking (cache/*.go). Fine
// for the expected single-browser-tab usage this MVP targets; if concurrent
// PATCHes from multiple clients ever race, wrap handlePatchIssue's body in
// a sync.Mutex on apiServer first — cheapest fix before reaching for
// per-request Backend instances.
type apiServer struct {
	cfg *config.Config
	b   *gitbugio.Backend
	me  string

	// The GitHub Project sync (sync.go): one at a time, and the last check.
	project ghbridge.Project
	syncMu  sync.Mutex
	last    *syncReport

	// Every change to the repository holds writeMu (issues.go,
	// beginWrite); the sync with GitHub/GitLab (forges.go) holds it for
	// its whole run, and syncing tells writes to answer 409 meanwhile.
	writeMu sync.Mutex
	syncing atomic.Bool
	forgeMu sync.Mutex // guards job
	job     *forgeJob
	runSync func(context.Context, bool, []string) (<-chan core.SyncResult, error) // nil: git-bug's

	// exposed are names --addr exposes beyond the loopback ones (withGuard)
	exposed []string
	// store is where attached files go (uploads.go); nil: from the
	// environment, at the first upload
	store *uploads.Store

	lazyOnce
}

// newMux wires the API routes plus a static handler (the built frontend) at
// "/", behind withGuard. Go 1.22 method-pattern routing needs no router
// framework.
func newMux(s *apiServer, static http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/views", s.handleViews)
	mux.HandleFunc("GET /api/config/fields", s.handleFields)
	mux.HandleFunc("GET /api/issues", s.handleListIssues)
	mux.HandleFunc("GET /api/issues/{id}", s.handleGetIssue)
	mux.HandleFunc("PATCH /api/issues/{id}", s.handlePatchIssue)
	mux.HandleFunc("POST /api/issues", s.handleNewIssue)
	mux.HandleFunc("GET /api/issues/{id}/thread", s.handleThread)
	mux.HandleFunc("POST /api/issues/{id}/comments", s.handleComment)
	mux.HandleFunc("PUT /api/issues/{id}/comments/{comment}", s.handleEditComment)
	mux.HandleFunc("PUT /api/issues/{id}/title", s.handleTitle)
	mux.HandleFunc("PUT /api/issues/{id}/status", s.handleStatus)
	mux.HandleFunc("POST /api/issues/{id}/labels", s.handleLabels)
	mux.HandleFunc("GET /api/me", s.handleMe)
	mux.HandleFunc("POST /api/me", s.handleSetMe)
	mux.HandleFunc("GET /api/media", s.handleMedia)
	mux.HandleFunc("POST /api/markdown", s.handleMarkdown)
	mux.HandleFunc("GET /api/uploads", s.handleUploadStatus)
	mux.HandleFunc("POST /api/uploads", s.handleUpload)
	mux.HandleFunc("GET /api/settings", s.handleSettings)
	mux.HandleFunc("PUT /api/bridges/{name}/sync", s.handleBridgeSync)
	mux.HandleFunc("GET /api/forges", s.handleForges)
	mux.HandleFunc("POST /api/forges/sync", s.handleForgeSync)
	mux.HandleFunc("GET /api/sync", s.handleSyncStatus)
	mux.HandleFunc("POST /api/sync/check", s.handleSyncCheck)
	mux.HandleFunc("POST /api/sync/apply", s.handleSyncApply)
	mux.HandleFunc("POST /api/sync/resolve", s.handleSyncResolve)
	mux.Handle("/", static)
	return withGuard(mux, s.exposed)
}

// Both answer {} without a config: a nil map would encode as null.
func (s *apiServer) handleViews(w http.ResponseWriter, r *http.Request) {
	views := s.cfg.Views
	if views == nil {
		views = map[string]config.View{}
	}
	writeJSON(w, http.StatusOK, views)
}

func (s *apiServer) handleFields(w http.ResponseWriter, r *http.Request) {
	fields := s.cfg.Fields
	if fields == nil {
		fields = map[string]config.Field{}
	}
	writeJSON(w, http.StatusOK, fields)
}

// handleListIssues applies filter, then sort, then group — same semantics as
// `ls`/`view`. filter defaults to board.DefaultFilter, same as the CLI.
func (s *apiServer) handleListIssues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	filterStr := q.Get("filter")
	if filterStr == "" {
		filterStr = board.DefaultFilter
	}

	// q is the page's search box, GitHub's syntax: free text (titles,
	// descriptions, comments) and qualifiers, on top of the view's filter
	issues, err := board.LoadMatching(s.b, s.cfg, filterStr+" "+q.Get("q"), s.me)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	board.SortByField(issues, q.Get("sort"), s.cfg)
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

	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()

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

var (
	errForbiddenOrigin = errors.New("refused: a write from another site")
	errNotJSON         = errors.New("refused: a write must be JSON (Content-Type: application/json)")
	errForeignHost     = errors.New("refused: this server answers to localhost (see --addr)")
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}

// writeCoded is writeError with a code the page acts on: syncing (retry
// when the forge sync ends), changed (someone edited meanwhile).
func writeCoded(w http.ResponseWriter, status int, code string, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error(), "code": code})
}
