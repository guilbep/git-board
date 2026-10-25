package server

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
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
	"github.com/guilbep/git-board/internal/release"
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
	forgeMu sync.Mutex // guards job, issueDry and checked
	job     *forgeJob
	// the forge sync, or one way of it (mode get, send): nil, git-bug's
	runSync func(ctx context.Context, mode string, dryRun bool, names []string, opts ...core.Option) (<-chan core.SyncResult, error)
	// one issue's sync (issuesync.go): nil, git-bug's; issueDry is when
	// each issue's last clean dry run ended
	runIssueSync func(ctx context.Context, dryRun bool, id string, opts ...core.Option) (<-chan core.SyncResult, error)
	issueDry     map[string]time.Time
	// checked is when each git remote was last fetched, pulled or pushed
	// (remotes.go)
	checked map[string]time.Time
	// githubHighest is GitHub's highest issue or pull request number of a
	// repository (numbers.go): nil, through gh
	githubHighest func(ctx context.Context, repo string) (int, error)
	// inboxMu guards the inbox files (inbox.go): one read-change-write at
	// a time
	inboxMu sync.Mutex
	// assignMu guards assignable.json: a run's fetch of who can be
	// assigned may outlast it, into the next run's
	assignMu sync.Mutex

	// exposed are names --addr exposes beyond the loopback ones (withGuard)
	exposed []string
	// gitDir is the repository's git directory (instance.go)
	gitDir string
	// store is where attached files go (uploads.go); nil: from the
	// environment, at the first upload
	store *uploads.Store

	// live updates (events.go): the pages listening, and the refs last
	// seen: all of them, and the releases' alone
	events      broker
	refsMu      sync.Mutex
	refs        string
	releaseRefs string

	// githubAPI is GitHub's API for the PR gate (pulls.go): "" is
	// https://api.github.com; tests, a fake
	githubAPI string
	// githubToken is who opens and merges pull requests, and their token:
	// nil, the backend's (Backend.GitHubToken)
	githubToken func() (login, token string, err error)
	// pulls is each branch's PR as the cycle last read it (cycle.go)
	pulls pullCache

	// releases (releases.go): releaseMu guards releaseRun, the current or
	// last job; runnerMu serializes the Runner's fetches and writes, a job
	// holding it for its whole run. releaseTools: nil, release.Exec();
	// releaseCache: "", the Runner's own (tests: a temp dir)
	releaseTools release.Tools
	releaseCache string
	releaseMu    sync.Mutex
	releaseRun   *releaseJob
	runnerMu     sync.Mutex

	// root is the checkout the server runs in: wtx runs there
	// (worktrees.go); wtxMu, one wtx write at a time
	root  string
	wtxMu sync.Mutex

	// step 4's runs (checks.go): proofMu guards proofJob, the run in
	// progress or the last one
	proofMu  sync.Mutex
	proofJob *proofJob

	lazyOnce
}

// newMux wires the API routes plus a static handler (the built frontend) at
// "/", behind withGuard. Go 1.22 method-pattern routing needs no router
// framework.
func newMux(s *apiServer, static http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/events", s.handleEvents)
	mux.HandleFunc("GET /api/repository", s.handleRepository)
	mux.HandleFunc("GET /api/views", s.handleViews)
	mux.HandleFunc("PUT /api/views/{name}", s.handleSaveView)
	mux.HandleFunc("GET /api/config/fields", s.handleFields)
	mux.HandleFunc("GET /api/colors", s.handleColors)
	mux.HandleFunc("GET /api/issues", s.handleListIssues)
	mux.HandleFunc("GET /api/facets", s.handleFacets)
	mux.HandleFunc("GET /api/issues/{id}", s.handleGetIssue)
	mux.HandleFunc("PATCH /api/issues/{id}", s.handlePatchIssue)
	mux.HandleFunc("POST /api/issues", s.handleNewIssue)
	mux.HandleFunc("GET /api/issues/{id}/thread", s.handleThread)
	mux.HandleFunc("POST /api/issues/{id}/sync", s.handleIssueSync)
	mux.HandleFunc("GET /api/issues/{id}/refs", s.handleRefs)
	mux.HandleFunc("GET /api/issues/{id}/branches", s.handleBranches)
	mux.HandleFunc("GET /api/issues/{id}/environments", s.handleIssueEnvironments)
	mux.HandleFunc("GET /api/issues/{id}/pulls", s.handlePulls)
	mux.HandleFunc("POST /api/issues/{id}/pulls", s.handleOpenPull)
	mux.HandleFunc("POST /api/issues/{id}/pulls/{number}/merge", s.handleMergePull)
	mux.HandleFunc("PATCH /api/issues/{id}/pulls/{number}", s.handleEditPull)
	mux.HandleFunc("GET /api/issues/{id}/cycle", s.handleCycle)
	mux.HandleFunc("POST /api/issues/{id}/cycle/{action}", s.handleCycleAction)
	mux.HandleFunc("GET /api/issues/{id}/tests", s.handleStepTests)
	mux.HandleFunc("POST /api/issues/{id}/tests/run", s.handleRunTests)
	mux.HandleFunc("GET /api/issues/{id}/tests/log/{hash}", s.handleRunLog)
	mux.HandleFunc("GET /api/checks/setup", s.handleChecksSetup)
	mux.HandleFunc("POST /api/checks/setup", s.handleWriteChecks)
	mux.HandleFunc("GET /api/issues/{id}/changes", s.handleChanges)
	mux.HandleFunc("GET /api/issues/{id}/changes/file", s.handleChangesFile)
	mux.HandleFunc("POST /api/issues/{id}/review", s.handleReview)
	mux.HandleFunc("POST /api/issues/{id}/review/{comment}/resolve", s.handleResolve)
	mux.HandleFunc("GET /api/issues/{id}/agent", s.handleIssueAgent)
	mux.HandleFunc("POST /api/issues/{id}/agent/{action}", s.handleIssueAgentAction)
	mux.HandleFunc("POST /api/issues/{id}/comments", s.handleComment)
	mux.HandleFunc("PUT /api/issues/{id}/comments/{comment}", s.handleEditComment)
	mux.HandleFunc("PUT /api/issues/{id}/title", s.handleTitle)
	mux.HandleFunc("PUT /api/issues/{id}/status", s.handleStatus)
	mux.HandleFunc("POST /api/issues/{id}/labels", s.handleLabels)
	mux.HandleFunc("GET /api/me", s.handleMe)
	mux.HandleFunc("POST /api/me", s.handleSetMe)
	mux.HandleFunc("GET /api/inbox", s.handleInbox)
	mux.HandleFunc("GET /api/work", s.handleWork)
	mux.HandleFunc("POST /api/inbox/seen", s.handleInboxSeen)
	mux.HandleFunc("GET /api/inbox/follow/{id}", s.handleFollow)
	mux.HandleFunc("PUT /api/inbox/follow/{id}", s.handleSetFollow)
	mux.HandleFunc("PUT /api/inbox/settings", s.handleInboxSettings)
	mux.HandleFunc("GET /api/media", s.handleMedia)
	mux.HandleFunc("POST /api/markdown", s.handleMarkdown)
	mux.HandleFunc("GET /api/uploads", s.handleUploadStatus)
	mux.HandleFunc("POST /api/uploads", s.handleUpload)
	mux.HandleFunc("GET /api/insights", s.handleInsights)
	mux.HandleFunc("GET /api/report", s.handleReport)
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
	mux.HandleFunc("GET /api/forges/docs", s.handleForgeDocs)
	mux.HandleFunc("POST /api/forges/sync", s.handleForgeSync)
	mux.HandleFunc("GET /api/remotes", s.handleRemotes)
	mux.HandleFunc("POST /api/remotes/{name}/{action}", s.handleRemote)
	mux.HandleFunc("GET /api/numbers", s.handleNumbers)
	mux.HandleFunc("POST /api/numbers/init", s.handleNumbersInit)
	mux.HandleFunc("POST /api/numbers/run", s.handleNumbersRun)
	mux.HandleFunc("GET /api/labels", s.handleLabelList)
	mux.HandleFunc("POST /api/labels/send", s.handleLabelSend)
	mux.HandleFunc("POST /api/labels/adopt", s.handleLabelAdopt)
	mux.HandleFunc("POST /api/labels/delete", s.handleLabelDelete)
	mux.HandleFunc("GET /api/lint", s.handleLint)
	mux.HandleFunc("GET /api/worktrees", s.handleWorktrees)
	mux.HandleFunc("POST /api/worktrees/preview", s.handleWtxPreview)
	mux.HandleFunc("POST /api/worktrees/init", s.handleWtxInit)
	mux.HandleFunc("GET /api/sync", s.handleSyncStatus)
	mux.HandleFunc("POST /api/sync/check", s.handleSyncCheck)
	mux.HandleFunc("POST /api/sync/apply", s.handleSyncApply)
	mux.HandleFunc("POST /api/sync/resolve", s.handleSyncResolve)
	mux.HandleFunc("GET /api/releases", s.handleReleases)
	mux.HandleFunc("GET /api/releases/draft", s.handleDraft)
	mux.HandleFunc("POST /api/releases", s.handleCut)
	mux.HandleFunc("GET /api/releases/job", s.handleReleaseJob)
	mux.HandleFunc("GET /api/releases/setup", s.handleSetup)
	mux.HandleFunc("POST /api/releases/setup", s.handleEnable)
	mux.HandleFunc("GET /api/releases/adopt", s.handleProposals)
	mux.HandleFunc("POST /api/releases/adopt", s.handleAdopt)
	mux.HandleFunc("GET /api/releases/{tag}", s.handleRelease)
	mux.HandleFunc("POST /api/releases/{tag}/promote", s.handlePromote)
	mux.HandleFunc("PUT /api/releases/{tag}/notes", s.handleReleaseNotes)
	mux.HandleFunc("POST /api/releases/{tag}/comments", s.handleReleaseComment)
	mux.HandleFunc("POST /api/releases/{tag}/yank", s.handleYank)
	mux.HandleFunc("POST /api/environments/{env}/deploy", s.handleDeploy)
	mux.HandleFunc("POST /api/environments/{env}/rollback", s.handleRollback)
	mux.Handle("/", static)
	return withGuard(s.withEvents(mux), s.exposed)
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
	Current  string `json:"current,omitempty"`
	Inherits string `json:"inherits,omitempty"` // a role's project value ([roles])
	// People are who a person field offers to pick (assignable.go), apart
	// from options: those make board columns, charts and filter order
	People []string `json:"people,omitempty"`
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
	// a role is a person field with the project's value, even unused yet
	for role, who := range cfg.Roles {
		f := res[role]
		f.Type, f.Single, f.Inherits = config.TypePerson, false, who
		res[role] = f
	}
	if people := s.people(); len(people) > 0 {
		for name, f := range res {
			if f.Type == config.TypePerson {
				f.People = people
				res[name] = f
			}
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
// `ls`/`view`. No filter is board.DefaultFilter, as the CLI's; an empty
// one, every issue, as typed.
func (s *apiServer) handleListIssues(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	issues, err := board.LoadMatching(s.b, s.config(), filterOf(q), s.me)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	board.SortByField(issues, q.Get("sort"), s.config())
	writeJSON(w, http.StatusOK, board.BucketBy(issues, q.Get("group")))
}

// handleFacets is the filters panel for a filter (board.Facets): the page
// counts and matches nothing itself.
func (s *apiServer) handleFacets(w http.ResponseWriter, r *http.Request) {
	facets, err := board.Facets(s.b, s.config(), filterOf(r.URL.Query()), s.me)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, facets)
}

func filterOf(q url.Values) string {
	if !q.Has("filter") {
		return board.DefaultFilter
	}
	return q.Get("filter")
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
