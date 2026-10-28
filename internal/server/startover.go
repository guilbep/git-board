package server

import (
	"errors"
	"fmt"
	"net/http"
	"slices"

	"github.com/guilbep/git-board/internal/store"
)

// Settings › Sync, Start over: git board reset --hard, push --prune,
// reset --hard origin and prune, run inside the server, which holds the
// repository. Each refuses while a forge or whole sync runs: it would
// empty the board under the run.

type startOverStatus struct {
	// Switched: git-bug's data, if any, can go; unswitched it is the
	// repository's only copy of its issues, and prune refuses
	Switched bool `json:"switched"`
	GitBug   struct {
		Refs  int  `json:"refs"`
		Cache bool `json:"cache"`
		Keys  int  `json:"keys"`
	} `json:"git_bug"`
	Remote bool `json:"remote"` // origin exists
}

type startOverResult struct {
	Deleted int    `json:"deleted"`
	Backup  string `json:"backup"`
	Me      bool   `json:"me"` // board.me was unset
}

// handleStartOverStatus says what Start over would find: git-bug's data,
// as a dry prune counts it, and whether there is an origin.
func (s *apiServer) handleStartOverStatus(w http.ResponseWriter, r *http.Request) {
	st := startOverStatus{Switched: true}
	rep, err := s.b.Prune("", true)
	switch {
	case errors.Is(err, store.ErrNotSwitched):
		st.Switched = false
		bugs, err := s.b.Store().Refs("refs/bugs/")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		st.GitBug.Refs = len(bugs)
	case err != nil:
		writeError(w, http.StatusInternalServerError, err)
		return
	default:
		for _, n := range rep.Refs {
			st.GitBug.Refs += n
		}
		st.GitBug.Cache, st.GitBug.Keys = rep.Cache != "", len(rep.Keys)
	}
	rs, err := s.b.Remotes()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	st.Remote = slices.ContainsFunc(rs, func(r store.Remote) bool { return r.Name == store.NumbersRemote })
	writeJSON(w, http.StatusOK, st)
}

// handleStartOver runs one of Start over's actions on origin's side or
// this clone's: reset (git board reset --hard), take (reset --hard
// origin), push-prune (push --prune) or prune (prune, on origin too with
// on_remote).
func (s *apiServer) handleStartOver(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Action   string `json:"action"`
		OnRemote bool   `json:"on_remote"`
	}
	if !decode(w, r, &req) {
		return
	}
	if !slices.Contains([]string{"reset", "take", "push-prune", "prune"}, req.Action) {
		writeError(w, http.StatusBadRequest, fmt.Errorf("action %q: want reset, take, push-prune or prune", req.Action))
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	s.forgeMu.Lock()
	defer s.forgeMu.Unlock()
	if s.syncing.Load() {
		writeCoded(w, http.StatusConflict, "syncing", errSyncing)
		return
	}
	origin := store.NumbersRemote
	var res startOverResult
	var rep store.ResetReport
	var err error
	switch req.Action {
	case "reset":
		rep, err = s.b.ResetHard("")
	case "take":
		rep, err = s.b.ResetHard(origin)
	case "push-prune":
		rep, err = s.b.PushPrune(origin)
	case "prune":
		remote := ""
		if req.OnRemote {
			remote = origin
		}
		var p store.PruneReport
		p, err = s.b.Prune(remote, false)
		for _, n := range p.Refs {
			rep.Deleted += n
		}
		rep.Deleted += p.Remote
		rep.Backup = p.Backup
	}
	if errors.Is(err, store.ErrNotSwitched) {
		writeError(w, http.StatusConflict, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, errors.New(store.NoCredentials(err.Error())))
		return
	}
	if req.Action != "prune" {
		s.job = nil // its check was of the board gone
	}
	res.Deleted, res.Backup, res.Me = rep.Deleted, rep.Backup, rep.Me
	writeJSON(w, http.StatusOK, res)
}
