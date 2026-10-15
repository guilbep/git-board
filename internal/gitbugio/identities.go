package gitbugio

import (
	"context"
	"fmt"
	"sort"

	"github.com/git-bug/git-bug/bridge" // registers every forge, for the sync
	"github.com/git-bug/git-bug/bridge/core"
)

var _ = bridge.Targets

// Identities lists the repository's git-bug identities, by name.
func (b *Backend) Identities() ([]Person, error) {
	var res []Person
	for _, id := range b.c.Identities().AllIds() {
		i, err := b.c.Identities().Resolve(id)
		if err != nil {
			return nil, err
		}
		res = append(res, person(i))
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Name < res[j].Name })
	return res, nil
}

// Adopt makes an identity the one git-board writes as.
func (b *Backend) Adopt(idPrefix string) error {
	i, err := b.c.Identities().ResolvePrefix(idPrefix)
	if err != nil {
		return fmt.Errorf("resolving identity %s: %w", idPrefix, err)
	}
	return b.c.SetUserIdentity(i)
}

// NewIdentity creates an identity and adopts it.
func (b *Backend) NewIdentity(name, email string) (Person, error) {
	i, err := b.c.Identities().New(name, email)
	if err != nil {
		return Person{}, err
	}
	if err := b.c.SetUserIdentity(i); err != nil {
		return Person{}, err
	}
	return person(i), nil
}

// SyncBridge is a bridge taking part in the forge sync.
type SyncBridge struct {
	Name   string `json:"name"`
	Target string `json:"target"` // github, gitlab, gitea
}

// SyncBridges lists the bridges with sync enabled.
func (b *Backend) SyncBridges() ([]SyncBridge, error) {
	names, err := core.ConfiguredBridges(b.c)
	if err != nil {
		return nil, err
	}
	cfg, err := b.c.Config().Read()
	if err != nil {
		return nil, err
	}
	var res []SyncBridge
	for _, name := range names {
		br, err := core.LoadBridge(b.c, name)
		if err != nil || !br.IsSync() {
			continue
		}
		res = append(res, SyncBridge{Name: name, Target: cfg.Value("git-bug.bridge."+name+".target", "")})
	}
	return res, nil
}

// SyncForges runs git-bug's forge sync (git bug bridge sync) on the named
// bridges, or every bridge with sync enabled, in this process: a running
// git-board-server holds the repository, so the git-bug command can't.
func (b *Backend) SyncForges(ctx context.Context, dryRun bool, names []string) (<-chan core.SyncResult, error) {
	return core.SyncAll(ctx, b.c, names, dryRun)
}
