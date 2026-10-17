package main

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

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

// handleCredentials lists git-bug's stored tokens: forge, host and login,
// never a value.
func (s *apiServer) handleCredentials(w http.ResponseWriter, r *http.Request) {
	creds, err := s.b.Credentials()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, creds)
}

// handleAddBridge configures a bridge, sync on unless asked off, checking
// the token and the project with the forge (gitbugio.AddBridge). The
// token, when pasted, goes to git-bug's credential store and is never
// sent back.
func (s *apiServer) handleAddBridge(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Target     string `json:"target"`
		Name       string `json:"name"`
		URL        string `json:"url"`
		Credential string `json:"credential"`
		Token      string `json:"token"`
		Sync       *bool  `json:"sync"` // absent: on
	}
	if !decode(w, r, &req) {
		return
	}
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	name, err := s.b.AddBridge(req.Target, req.Name, strings.TrimSpace(req.URL), req.Credential, strings.TrimSpace(req.Token), req.Sync == nil || *req.Sync)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Errorf("the bridge isn't added: %w", plainBridgeError(err)))
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"name": name})
}

// handleRemoveBridge removes a bridge's configuration; its issues stay.
func (s *apiServer) handleRemoveBridge(w http.ResponseWriter, r *http.Request) {
	done, ok := s.beginWrite(w)
	if !ok {
		return
	}
	defer done()
	if err := s.b.RemoveBridge(r.PathValue("name")); err != nil {
		code := http.StatusInternalServerError
		if errors.Is(err, gitbugio.ErrNoBridge) {
			code = http.StatusNotFound
		}
		writeError(w, code, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{})
}

// plainBridgeError says in words what a forge answered a bridge's
// configuration, the forge's own message kept at the end.
func plainBridgeError(err error) error {
	msg := err.Error()
	low := strings.ToLower(msg)
	var plain string
	switch {
	case strings.Contains(low, "401") || strings.Contains(low, "bad credentials") || strings.Contains(low, "unauthorized"):
		plain = "the forge refused the token: wrong, expired, or without the scope it needs (api, or repo)"
	case strings.Contains(low, "404") || strings.Contains(low, "doesn't exist") || strings.Contains(low, "non-existent"):
		plain = "the forge doesn't know this project, or the token can't see it"
	case strings.Contains(low, "no such host") || strings.Contains(low, "connection refused") || strings.Contains(low, "timeout"):
		plain = "git-board can't reach the forge at that address"
	default:
		return err
	}
	return fmt.Errorf("%s (%s)", plain, msg)
}
