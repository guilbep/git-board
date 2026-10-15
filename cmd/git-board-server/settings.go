package main

import (
	"errors"
	"net/http"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// settingsResponse is the settings page: the bridges, and how attachments
// and the GitHub Project are set up. The identity is GET /api/me.
type settingsResponse struct {
	Bridges []gitbugio.Bridge `json:"bridges"`
	Uploads struct {
		Enabled   bool   `json:"enabled"`
		Endpoint  string `json:"endpoint,omitempty"`
		PublicURL string `json:"public_url,omitempty"`
		Prefix    string `json:"prefix,omitempty"`
	} `json:"uploads"`
	GithubProject config.GithubProject `json:"github_project"`
	ConfigPath    string               `json:"config_path"`
}

func (s *apiServer) handleSettings(w http.ResponseWriter, r *http.Request) {
	bridges, err := s.b.Bridges()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	res := settingsResponse{Bridges: bridges, GithubProject: s.config().GithubProject, ConfigPath: s.configPath()}
	if st := s.uploadStore(); st != nil {
		res.Uploads.Enabled, res.Uploads.Endpoint, res.Uploads.PublicURL, res.Uploads.Prefix = true, st.Endpoint, st.PublicURL, st.Prefix
	}
	writeJSON(w, http.StatusOK, res)
}

// handleBridgeSync turns a bridge's sync on or off.
func (s *apiServer) handleBridgeSync(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Sync bool `json:"sync"`
	}
	if !decode(w, r, &req) {
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	if err := s.b.SetBridgeSync(r.PathValue("name"), req.Sync); err != nil {
		code := http.StatusBadRequest
		if errors.Is(err, gitbugio.ErrNoBridge) {
			code = http.StatusNotFound
		}
		writeError(w, code, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"sync": req.Sync})
}
