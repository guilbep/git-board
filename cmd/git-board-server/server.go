package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"
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
	cfg *config.Config // read with config(): the settings page replaces it
	b   *gitbugio.Backend
	me  string

	cfgMu   sync.RWMutex
	cfgPath string // .git-board.toml; empty: config.Path()

	// The GitHub Project sync (sync.go): one at a time, and the last check.
	project ghbridge.Project
	syncMu  sync.Mutex
	last    *syncReport

	// Every change to the repository holds writeMu (issues.go,
	// beginWrite); the sync with the forges (forges.go) holds it for
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
	mux.HandleFunc("PUT /api/views/{name}", s.handleSaveView)
	mux.HandleFunc("GET /api/config/fields", s.handleFields)
	mux.HandleFunc("GET /api/colors", s.handleColors)
	mux.HandleFunc("GET /api/issues", s.handleListIssues)
	mux.HandleFunc("GET /api/issues/{id}", s.handleGetIssue)
	mux.HandleFunc("PATCH /api/issues/{id}", s.handlePatchIssue)
	mux.HandleFunc("POST /api/issues", s.handleNewIssue)
	mux.HandleFunc("GET /api/issues/{id}/thread", s.handleThread)
	mux.HandleFunc("GET /api/issues/{id}/refs", s.handleRefs)
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
	mux.HandleFunc("GET /api/insights", s.handleInsights)
	mux.HandleFunc("GET /api/sprints", s.handleSprints)
	mux.HandleFunc("GET /api/settings", s.handleSettings)
	mux.HandleFunc("GET /api/config", s.handleConfigFile)
	mux.HandleFunc("POST /api/config/check", s.handleConfigCheck)
	mux.HandleFunc("GET /api/config/docs", s.handleConfigDocs)
	mux.HandleFunc("PUT /api/config", s.handleConfigSave)
	mux.HandleFunc("PUT /api/bridges/{name}/sync", s.handleBridgeSync)
	mux.HandleFunc("GET /api/credentials", s.handleCredentials)
	mux.HandleFunc("POST /api/bridges", s.handleAddBridge)
	mux.HandleFunc("DELETE /api/bridges/{name}", s.handleRemoveBridge)
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
	views := s.config().Views
	if views == nil {
		views = map[string]config.View{}
	}
	writeJSON(w, http.StatusOK, views)
}

// fieldResponse is a field as the page gets it: the config's, its type
// and unit resolved (config.TypeOf), and for an iteration the current one.
type fieldResponse struct {
	config.Field
	Current string `json:"current,omitempty"`
}

// labelValues is every field's values on the issues.
func (s *apiServer) labelValues() map[string][]string {
	values := map[string][]string{}
	for _, l := range s.b.Labels() {
		if field, value, ok := strings.Cut(l, ":"); ok {
			values[field] = append(values[field], value)
		}
	}
	return values
}

// one value at a time, unless the type holds several: people, the issues
// one blocks or depends on
var multiByName = map[string]bool{"blocks": true, "blocked-by": true, "depends-on": true}

// handleFields answers the fields with their types: the configured ones,
// and the undeclared ones whose type isn't a plain select (a date, a
// person, an issue…), which the page can't guess from values alone. An
// iteration takes [[sprints.list]]'s slugs by start date as options,
// unless it has its own, and names the current one.
func (s *apiServer) handleFields(w http.ResponseWriter, r *http.Request) {
	cfg := s.config()
	values := s.labelValues()
	res := map[string]fieldResponse{}
	for name, f := range cfg.Fields {
		f.Type, f.Unit = cfg.TypeOf(name, values[name]), cfg.UnitOf(name)
		res[name] = fieldResponse{Field: f}
	}
	for name, vs := range values {
		if _, declared := cfg.Fields[name]; declared {
			continue
		}
		t := cfg.TypeOf(name, vs)
		if t == config.TypeSelect {
			continue
		}
		single := t != config.TypePerson && !(t == config.TypeIssue && multiByName[name])
		res[name] = fieldResponse{Field: config.Field{Type: t, Unit: cfg.UnitOf(name), Single: single, Numeric: t == config.TypeNumber}}
	}
	if windows := config.SprintWindows(cfg.Sprints); len(windows) > 0 {
		current := ""
		if cur, err := config.ResolveSprint(cfg.Sprints, "current", time.Now()); err == nil && cur != nil {
			current = cur.Slug
		}
		for _, name := range iterationFields(cfg, res) {
			f := res[name]
			if f.Type == "" {
				f.Type, f.Single = config.TypeIteration, true // the sprint field, no label yet
			}
			if len(f.Options) == 0 {
				for _, w := range windows {
					f.Options = append(f.Options, w.Slug)
				}
			}
			f.Current = current
			res[name] = f
		}
	}
	writeJSON(w, http.StatusOK, res)
}

// iterationFields are the fields taking [[sprints.list]]: of type
// iteration, and sprint even before any issue has one.
func iterationFields(cfg *config.Config, res map[string]fieldResponse) []string {
	names := []string{}
	for name, f := range res {
		if f.Type == config.TypeIteration {
			names = append(names, name)
		}
	}
	if _, ok := res["sprint"]; !ok && cfg.TypeOf("sprint", nil) == config.TypeIteration {
		names = append(names, "sprint")
	}
	return names
}

// handleColors answers each field value's color (config.Colors): of the
// labels in use and of the configured options, so a value shows the same
// color everywhere. A date is red once past, amber within a week.
func (s *apiServer) handleColors(w http.ResponseWriter, r *http.Request) {
	cfg := s.config()
	values := s.labelValues()
	for name, f := range cfg.Fields {
		values[name] = append(values[name], f.Options...)
		for v := range f.Colors {
			values[name] = append(values[name], v)
		}
	}
	today := time.Now().Format("2006-01-02")
	soon := time.Now().AddDate(0, 0, 7).Format("2006-01-02")
	res := map[string]map[string]string{}
	for field, vs := range values {
		colors := cfg.Colors(field, vs)
		if cfg.TypeOf(field, vs) == config.TypeDate {
			for _, v := range vs {
				switch {
				case colors[v] != "":
				case v < today: // ISO dates sort as dates
					colors[v] = "red"
				case v <= soon:
					colors[v] = "amber"
				}
			}
		}
		if len(colors) > 0 {
			res[field] = colors
		}
	}
	writeJSON(w, http.StatusOK, res)
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
	issues, err := board.LoadMatching(s.b, s.config(), filterStr+" "+q.Get("q"), s.me)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	board.SortByField(issues, q.Get("sort"), s.config())
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
		if _, err := fields.Apply(s.b, s.config(), id, name, assignments[name], s.me, today); err != nil {
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
