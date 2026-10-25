package server

import (
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// addBridge configures a bridge: M5's second phase, where the forges come
// from .git-board.toml.
func addBridge(t *testing.T, s *apiServer, name string, kv map[string]string) {
	t.Helper()
	t.Skip("M5 phase 2: bridges")
}

type settingsBody struct {
	Bridges []gitbugio.Bridge `json:"bridges"`
	Uploads struct {
		Enabled bool `json:"enabled"`
	} `json:"uploads"`
	GithubProject config.GithubProject `json:"github_project"`
	ConfigPath    string               `json:"config_path"`
	Cycle         config.Stages        `json:"cycle"`
}

func TestSettingsListAndToggleBridgeSync(t *testing.T) {
	t.Skip("M5 phase 2: bridges")
}
