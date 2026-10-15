package gitbugio

import (
	"context"
	"fmt"
	"sort"
	"strings"

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

// PushURLs are where a push to remote goes, as git's does: its pushurls,
// else its urls. Empty without such a remote.
func (b *Backend) PushURLs(remote string) []string {
	cfg, err := b.c.Config().Read()
	if err != nil {
		return nil
	}
	var res []string
	for _, key := range []string{"remote." + remote + ".pushurl", "remote." + remote + ".url"} {
		for _, e := range cfg.GetAll(key) {
			res = append(res, e.Value)
		}
		if len(res) > 0 {
			return res
		}
	}
	return nil
}

// MyLogin is the forge login of the identity git-board writes as: what
// @me means (assignee:@me), since assignee labels hold forge logins. Its own
// login, else the one a bridge tagged it with (github-login, gitlab-login,
// gitea-login), else a sync link (git-bug.bridge.<name>.login-<identity>).
// "" when none says.
func (b *Backend) MyLogin() string {
	set, err := b.c.IsUserIdentitySet()
	if err != nil || !set {
		return ""
	}
	me, err := b.c.GetUserIdentity()
	if err != nil {
		return ""
	}
	if l := me.Login(); l != "" {
		return l
	}
	meta := me.ImmutableMetadata()
	for _, key := range []string{"github-login", "gitlab-login", "gitea-login"} {
		if l := meta[key]; l != "" {
			return l
		}
	}
	cfg, err := b.c.Config().Read()
	if err != nil {
		return ""
	}
	suffix := ".login-" + me.Id().String()
	for e := range cfg.All() {
		if k := e.Key.String(); strings.HasPrefix(k, "git-bug.bridge.") && strings.HasSuffix(k, suffix) && e.Value != "" {
			return e.Value
		}
	}
	return ""
}
