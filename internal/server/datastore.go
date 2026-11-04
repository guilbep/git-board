package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/guilbep/git-board/internal/store"
)

// Settings › Datastore: where the board is kept (ADR 0002). The setting is
// board.datastore in this clone's git config; it takes effect when git
// board serve starts again, which the page says.

type datastoreResponse struct {
	store.Capabilities
	// Setting is board.datastore as the page shows it, its password hidden.
	Setting string `json:"setting"`
	// Env: GIT_BOARD_DATASTORE sets it, and the page can't change it.
	Env bool `json:"env"`
	// Restart: the setting isn't what the server opened with.
	Restart bool `json:"restart"`
}

// handleDatastore answers where the board is kept and what that datastore
// can do: the page shows a control that needs a capability the datastore
// lacks as disabled, saying why.
func (s *apiServer) handleDatastore(w http.ResponseWriter, r *http.Request) {
	setting, env, restart, err := s.b.DatastoreSetting()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, datastoreResponse{Capabilities: s.b.Capabilities(), Setting: setting, Env: env, Restart: restart})
}

type datastoreRequest struct {
	URL string `json:"url"` // git, forge, or a postgres:// URL
}

// handleSetDatastore writes board.datastore.
func (s *apiServer) handleSetDatastore(w http.ResponseWriter, r *http.Request) {
	var req datastoreRequest
	if !decode(w, r, &req) {
		return
	}
	if _, env, _, _ := s.b.DatastoreSetting(); env {
		writeError(w, http.StatusConflict, errEnvDatastore)
		return
	}
	if err := s.b.SetDatastore(s.b.Unredact(req.URL)); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	s.handleDatastore(w, r)
}

// handleTestDatastore connects to the datastore a URL names, writing
// nothing, and says what is there.
func (s *apiServer) handleTestDatastore(w http.ResponseWriter, r *http.Request) {
	var req datastoreRequest
	if !decode(w, r, &req) {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, s.b.TestDatastore(ctx, s.b.Unredact(req.URL)))
}

// handleCopyDatastore copies the board in git into the PostgreSQL database
// a URL names, every id kept; what is there already is left alone.
func (s *apiServer) handleCopyDatastore(w http.ResponseWriter, r *http.Request) {
	var req datastoreRequest
	if !decode(w, r, &req) {
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	url := s.b.Unredact(req.URL)
	n, err := s.b.CopyGitTo(r.Context(), url)
	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	res := s.b.TestDatastore(r.Context(), url)
	res.Copied = n
	res.Line = fmt.Sprintf("%d copied, every id kept. %s", n, res.Line)
	writeJSON(w, http.StatusOK, res)
}

var errEnvDatastore = errDatastoreEnv{}

type errDatastoreEnv struct{}

func (errDatastoreEnv) Error() string {
	return "GIT_BOARD_DATASTORE sets where the board is kept, over git config: change it where git board serve starts"
}
