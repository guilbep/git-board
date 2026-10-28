package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/guilbep/git-board/internal/ghbridge"
)

// syncStatus is what the page knows about the sync with the GitHub
// Project: which one, and the last check, if any. Reading the Project costs
// GitHub API points and takes seconds on a big one, so it's only read when
// asked (check, apply, resolve), never to render a page.
type syncStatus struct {
	Project string      `json:"project"` // "owner/number", "" when none is configured
	Write   bool        `json:"write"`   // [github-project] write: may write to the Project
	Last    *syncReport `json:"last"`
}

// syncReport is a dry run's outcome: what is held for a person, and what a
// sync would change.
type syncReport struct {
	CheckedAt     time.Time           `json:"checked_at"`
	Title         string              `json:"title"` // the Project's, as GitHub answered
	Conflicts     []ghbridge.Conflict `json:"conflicts"`
	LabelChanges  int                 `json:"label_changes"`  // issues whose labels a sync would change
	ProjectWrites int                 `json:"project_writes"` // fields a sync would write to the Project
	Warnings      int                 `json:"warnings"`
	Unmatched     int                 `json:"unmatched"`
}

func (s *apiServer) status() syncStatus {
	gp := s.config().GithubProject
	st := syncStatus{Write: gp.Write, Last: s.last}
	if gp.Owner != "" && gp.Number != 0 {
		st.Project = fmt.Sprintf("%s/%d", gp.Owner, gp.Number)
	}
	return st
}

func (s *apiServer) handleSyncStatus(w http.ResponseWriter, r *http.Request) {
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	writeJSON(w, http.StatusOK, s.status())
}

// check reads both sides and keeps the dry run's outcome. The caller holds
// syncMu.
func (s *apiServer) check() error {
	state, err := ghbridge.LoadState(s.b, s.project, s.config(), ghbridge.DefaultItemLimit)
	if err != nil {
		return err
	}
	plan := state.Plan(s.config())
	report := &syncReport{CheckedAt: time.Now(), Title: state.Meta.Title, Conflicts: plan.Conflicts, ProjectWrites: len(plan.Edits), Unmatched: len(plan.Unmatched)}
	if report.Conflicts == nil {
		report.Conflicts = []ghbridge.Conflict{}
	}
	for _, c := range plan.Changes {
		if len(c.Added)+len(c.Removed) > 0 {
			report.LabelChanges++
		}
		report.Warnings += len(c.Warnings)
	}
	s.last = report
	return nil
}

func (s *apiServer) handleSyncCheck(w http.ResponseWriter, r *http.Request) {
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if err := s.check(); err != nil {
		writeSyncError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.status())
}

// handleSyncApply is `git board project sync`: conflicts stay held,
// and the Project is only written to with write = true.
func (s *apiServer) handleSyncApply(w http.ResponseWriter, r *http.Request) {
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if err := s.apply(); err != nil {
		writeSyncError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.status())
}

// apply syncs the Project's fields, then checks what's left. The caller
// holds syncMu.
func (s *apiServer) apply() error {
	state, err := ghbridge.LoadState(s.b, s.project, s.config(), ghbridge.DefaultItemLimit)
	if err == nil {
		_, err = ghbridge.ApplySync(s.b, s.project.Writer, s.b, state.Plan(s.config()), s.config(), state.Meta)
	}
	if err == nil {
		err = s.check()
	}
	return err
}

// handleSyncResolve is `git board project resolve`: keep "git-board"'s value
// (written to the Project) or the "project"'s (set on the label).
func (s *apiServer) handleSyncResolve(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Number int    `json:"number"`
		Field  string `json:"field"`
		Keep   string `json:"keep"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Keep != "git-board" && req.Keep != "project" {
		writeError(w, http.StatusBadRequest, fmt.Errorf(`keep %q: want "git-board" or "project"`, req.Keep))
		return
	}

	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	state, err := ghbridge.LoadState(s.b, s.project, s.config(), ghbridge.DefaultItemLimit)
	if err == nil {
		_, err = ghbridge.ResolveConflict(s.b, s.project, s.config(), state, req.Number, req.Field, req.Keep == "git-board")
	}
	if err == nil {
		err = s.check()
	}
	if err != nil {
		writeSyncError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, s.status())
}

// writeSyncError answers 400 for what the request or the config got wrong,
// 502 for GitHub failing.
func writeSyncError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ghbridge.ErrNoProject), errors.Is(err, ghbridge.ErrNoWrite), errors.Is(err, ghbridge.ErrNotConflict):
		writeError(w, http.StatusBadRequest, err)
	default:
		writeError(w, http.StatusBadGateway, err)
	}
}
