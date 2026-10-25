package gitbugio

import (
	"context"
	"errors"

	"github.com/git-bug/git-bug/bridge/core"
)

// Forge sync and its settings are M5's second phase: the core's syncer,
// with the forges of .git-board.toml and tokens from the environment or
// git's credential helper. Until then each answers ErrNotWired. These are
// the only uses of git-bug left in gitbugio: the server's forge sync still
// reads git-bug's result types, until it moves to the core too.

// ErrNotWired: forge sync isn't wired to the core yet.
var ErrNotWired = errors.New("forge sync is not wired yet (M5 phase 2)")

// SyncBridge is a bridge taking part in the forge sync.
type SyncBridge struct {
	Name   string `json:"name"`
	Target string `json:"target"` // github, gitlab
}

// SyncBridges lists the bridges with sync enabled.
func (b *Backend) SyncBridges() ([]SyncBridge, error) { return nil, ErrNotWired }

// Bridge is a configured bridge, as the settings page shows it.
type Bridge struct {
	Name    string `json:"name"`
	Target  string `json:"target"` // github, gitlab
	Where   string `json:"where"`  // the project it syncs with
	Sync    bool   `json:"sync"`
	CanSync bool   `json:"can_sync"`

	// where the forge's API finds the project: owner and project (GitHub;
	// GitLab's project-id), and an instance's base URL
	Owner   string `json:"-"`
	Project string `json:"-"`
	BaseURL string `json:"-"`
}

// ErrNoBridge: no bridge has that name.
var ErrNoBridge = errors.New("no bridge has that name")

// Bridges lists the configured bridges, sync on or off.
func (b *Backend) Bridges() ([]Bridge, error) { return nil, ErrNotWired }

// SetBridgeSync turns a bridge's sync on or off.
func (b *Backend) SetBridgeSync(name string, on bool) error { return ErrNotWired }

// AddBridge configures a bridge and answers its name.
func (b *Backend) AddBridge(target, name, projectURL, credID, token string, sync bool) (string, error) {
	return "", ErrNotWired
}

// RemoveBridge removes a bridge's configuration.
func (b *Backend) RemoveBridge(name string) error { return ErrNotWired }

// SyncForges syncs with the named bridges' forges, or every one's.
func (b *Backend) SyncForges(ctx context.Context, dryRun bool, names []string, opts ...core.Option) (<-chan core.SyncResult, error) {
	return nil, ErrNotWired
}

// GetForges brings the forges' changes in and sends nothing.
func (b *Backend) GetForges(ctx context.Context, names []string, opts ...core.Option) (<-chan core.SyncResult, error) {
	return nil, ErrNotWired
}

// SendForges sends the local changes to the forges and gets nothing.
func (b *Backend) SendForges(ctx context.Context, names []string) (<-chan core.SyncResult, error) {
	return nil, ErrNotWired
}

// SyncIssue syncs one issue with every forge.
func (b *Backend) SyncIssue(ctx context.Context, dryRun bool, id string, opts ...core.Option) (<-chan core.SyncResult, error) {
	return nil, ErrNotWired
}

// ForgeToken is the token for a forge host, "" when there's none.
func (b *Backend) ForgeToken(host string) string { return "" }

// GitHubToken is the GitHub login of the person git-board writes as, and
// that login's token: who opens and merges pull requests from git-board.
func (b *Backend) GitHubToken() (login, token string, err error) { return "", "", ErrNotWired }

// ForgeHosts are the hosts a token is known for.
func (b *Backend) ForgeHosts() []string { return nil }

// Credential is a stored token, as the settings page lists it: never its
// value.
type Credential struct {
	ID     string `json:"id"`
	Target string `json:"target"`
	Host   string `json:"host"`
	Login  string `json:"login"`
}

// Credentials lists the stored tokens, without their values.
func (b *Backend) Credentials() ([]Credential, error) { return nil, ErrNotWired }
