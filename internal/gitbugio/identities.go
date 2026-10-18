package gitbugio

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

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

// Bridge is a configured bridge, as the settings page shows it.
type Bridge struct {
	Name    string `json:"name"`
	Target  string `json:"target"` // github, gitlab, gitea…
	Where   string `json:"where"`  // the project it syncs with
	Sync    bool   `json:"sync"`
	CanSync bool   `json:"can_sync"` // the target supports sync (not jira, launchpad)
}

// ErrNoBridge: no bridge has that name.
var ErrNoBridge = errors.New("no bridge has that name")

// Bridges lists the configured bridges, sync on or off.
func (b *Backend) Bridges() ([]Bridge, error) {
	names, err := core.ConfiguredBridges(b.c)
	if err != nil {
		return nil, err
	}
	cfg, err := b.c.Config().Read()
	if err != nil {
		return nil, err
	}
	res := []Bridge{}
	for _, name := range names {
		get := func(k string) string { return cfg.Value("git-bug.bridge."+name+"."+k, "") }
		br := Bridge{Name: name, Target: get("target")}
		br.Sync, _ = strconv.ParseBool(get("sync"))
		if nb, err := core.NewBridge(b.c, br.Target, name); err == nil {
			br.CanSync = nb.SupportsSync()
		}
		switch br.Target {
		case "github":
			br.Where = "https://github.com/" + get("owner") + "/" + get("project")
		case "gitlab":
			br.Where = get("base-url") + " project " + get("project-id")
		case "gitea":
			br.Where = strings.TrimSuffix(get("base-url"), "/") + "/" + get("owner") + "/" + get("project")
		}
		res = append(res, br)
	}
	return res, nil
}

// SetBridgeSync turns a bridge's sync on or off (git-bug's sync key).
func (b *Backend) SetBridgeSync(name string, on bool) error {
	if !core.BridgeExist(b.c, name) {
		return ErrNoBridge
	}
	br, err := core.LoadBridge(b.c, name)
	if err != nil {
		return err
	}
	if !br.SupportsSync() {
		return fmt.Errorf("bridge %s can't sync: its forge isn't supported", name)
	}
	return core.SetBridgeConfig(b.c, name, core.ConfigKeySync, strconv.FormatBool(on))
}

// the name a bridge of a forge gets by default, as in the README's setup
var shortName = map[string]string{"github": "gh", "gitlab": "gl", "gitea": "gitea"}

var bridgeName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// AddBridge configures a bridge in this process, as `git bug bridge new`
// does, with --sync when sync: the forge checks the credential (a pasted
// token, then kept in git-bug's credential store, or a stored one by id)
// and the project. An empty name takes the forge's short one, gh, gl or
// gitea, made unique. It answers the name.
func (b *Backend) AddBridge(target, name, projectURL, credID, token string, sync bool) (string, error) {
	switch {
	case name == "":
		name = shortName[target]
		for n := 2; name != "" && core.BridgeExist(b.c, name); n++ {
			name = shortName[target] + strconv.Itoa(n)
		}
	case !bridgeName.MatchString(name):
		return "", fmt.Errorf("a bridge's name is letters, digits, - and _, not %q", name)
	case core.BridgeExist(b.c, name):
		return "", fmt.Errorf("a bridge named %s exists already", name)
	}
	br, err := core.NewBridge(b.c, target, name)
	if err != nil || !br.SupportsSync() {
		return "", fmt.Errorf("git-board syncs with GitHub, GitLab and Gitea, not %q", target)
	}
	if credID == "" && token == "" {
		return "", errors.New("pick a stored credential or paste a token")
	}
	if err := br.Configure(core.BridgeParams{URL: projectURL, CredPrefix: credID, TokenRaw: token}, false); err != nil {
		return "", err
	}
	return name, core.SetBridgeConfig(b.c, name, core.ConfigKeySync, strconv.FormatBool(sync))
}

// RemoveBridge removes a bridge's configuration. The issues it imported
// stay, with their links to the forge.
func (b *Backend) RemoveBridge(name string) error {
	if !core.BridgeExist(b.c, name) {
		return ErrNoBridge
	}
	return core.RemoveBridge(b.c, name)
}

// SyncForges runs git-bug's forge sync (git bug bridge sync) on the named
// bridges, or every bridge with sync enabled, in this process: a running
// git-board's server holds the repository, so the git-bug command can't.
func (b *Backend) SyncForges(ctx context.Context, dryRun bool, names []string) (<-chan core.SyncResult, error) {
	return core.SyncAll(ctx, b.c, names, dryRun)
}

// GetForges brings the forges' changes in and sends nothing: git bug
// bridge pull on the named bridges, or every bridge with sync enabled,
// from where the last sync's pull ended, which it moves on as a sync does.
func (b *Backend) GetForges(ctx context.Context, names []string) (<-chan core.SyncResult, error) {
	return b.oneWay(names, func(br *core.Bridge, out chan<- core.SyncResult) {
		start := time.Now().Add(-5 * time.Second) // as the sync does
		results, err := br.ImportAllSince(ctx, b.lastSync(br.Name))
		ok := err == nil
		if err != nil {
			r := core.NewImportError(err, "")
			out <- core.SyncResult{Bridge: br.Name, Import: &r}
		}
		for r := range results {
			ok = ok && r.Event != core.ImportEventError
			out <- core.SyncResult{Bridge: br.Name, Import: &r}
		}
		if ok {
			_ = core.SetBridgeConfig(b.c, br.Name, lastSyncKey, strconv.FormatInt(start.Unix(), 10))
		}
	})
}

// SendForges sends the local changes to the forges and gets nothing: git
// bug bridge push on the named bridges, or every bridge with sync enabled.
func (b *Backend) SendForges(ctx context.Context, names []string) (<-chan core.SyncResult, error) {
	return b.oneWay(names, func(br *core.Bridge, out chan<- core.SyncResult) {
		results, err := br.ExportAll(ctx, time.Time{})
		if err != nil {
			r := core.NewExportError(err, "")
			out <- core.SyncResult{Bridge: br.Name, Export: &r}
		}
		for r := range results {
			out <- core.SyncResult{Bridge: br.Name, Export: &r}
		}
	})
}

// lastSyncKey is where git-bug keeps when a bridge's last sync pulled from
// (bridge/core configKeyLastSyncTime, not exported).
const lastSyncKey = "lastSyncTime"

func (b *Backend) lastSync(name string) time.Time {
	cfg, err := b.c.Config().Read()
	if err != nil {
		return time.Time{}
	}
	if t, err := cfg.Int("git-bug.bridge."+name+"."+lastSyncKey, 0); err == nil && t != 0 {
		return time.Unix(t, 0)
	}
	return time.Time{}
}

// oneWay runs each of the named bridges in turn, or every one with sync
// enabled, after checking they all have it.
func (b *Backend) oneWay(names []string, run func(*core.Bridge, chan<- core.SyncResult)) (<-chan core.SyncResult, error) {
	if len(names) == 0 {
		all, err := b.SyncBridges()
		if err != nil {
			return nil, err
		}
		for _, br := range all {
			names = append(names, br.Name)
		}
		if len(names) == 0 {
			return nil, errors.New("no bridge has sync enabled")
		}
	}
	var bridges []*core.Bridge
	for _, name := range names {
		br, err := core.LoadBridge(b.c, name)
		if err != nil {
			return nil, err
		}
		if !br.IsSync() {
			return nil, fmt.Errorf("bridge %s doesn't have sync enabled", name)
		}
		bridges = append(bridges, br)
	}
	out := make(chan core.SyncResult)
	go func() {
		defer close(out)
		for _, br := range bridges {
			run(br, out)
		}
	}()
	return out, nil
}

// SyncIssue syncs one issue (git bug bridge sync --issue) with every
// bridge with sync enabled, in this process.
func (b *Backend) SyncIssue(ctx context.Context, dryRun bool, id string) (<-chan core.SyncResult, error) {
	return core.SyncIssue(ctx, b.c, id, nil, dryRun)
}

// LinkIssue makes an issue one with an issue already on a bridge's forge,
// by its number ("12", "#12") or URL (git bug bridge link). Nothing is
// written to the forge: the next sync sends what differs, unless
// TakeIssue keeps the forge's version.
func (b *Backend) LinkIssue(ctx context.Context, id, bridge, issue string) (core.Ref, error) {
	if !core.BridgeExist(b.c, bridge) {
		return core.Ref{}, ErrNoBridge
	}
	return core.Link(ctx, b.c, id, bridge, issue)
}

// UnlinkIssue stops syncing an issue with a bridge's forge, for good (git
// bug bridge unlink). The forge issue stays as it is.
func (b *Backend) UnlinkIssue(ctx context.Context, id, bridge string) error {
	if !core.BridgeExist(b.c, bridge) {
		return ErrNoBridge
	}
	return core.Unlink(ctx, b.c, id, bridge)
}

// TakeIssue replaces what an issue hasn't sent to a bridge's forge yet
// with the forge's version (git bug bridge take); comments the forge
// doesn't have stay. The channel must be drained: it holds the bridge lock.
func (b *Backend) TakeIssue(ctx context.Context, id, bridge string) (<-chan core.SyncResult, error) {
	if !core.BridgeExist(b.c, bridge) {
		return nil, ErrNoBridge
	}
	return core.Take(ctx, b.c, id, bridge)
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
	return b.linkedLogin(me.Id().String(), "")
}

// myGitHubLogin is the GitHub login of the identity git-board writes as,
// found the way the bridge maps an author to an account: its
// github-login, else a GitHub bridge's sync link, else its own login.
func (b *Backend) myGitHubLogin() string {
	set, err := b.c.IsUserIdentitySet()
	if err != nil || !set {
		return ""
	}
	me, err := b.c.GetUserIdentity()
	if err != nil {
		return ""
	}
	if l := me.ImmutableMetadata()["github-login"]; l != "" {
		return l
	}
	if l := b.linkedLogin(me.Id().String(), "github"); l != "" {
		return l
	}
	return me.Login()
}

// linkedLogin is the login a sync bridge links an identity to
// (git-bug.bridge.<name>.login-<identity>), by a bridge of the target, or
// of any target when "".
func (b *Backend) linkedLogin(identity, target string) string {
	cfg, err := b.c.Config().Read()
	if err != nil {
		return ""
	}
	suffix := ".login-" + identity
	for e := range cfg.All() {
		k := e.Key.String()
		name, ok := strings.CutPrefix(strings.TrimSuffix(k, suffix), "git-bug.bridge.")
		if !ok || !strings.HasSuffix(k, suffix) || e.Value == "" {
			continue
		}
		if target == "" {
			return e.Value
		}
		for _, t := range cfg.GetAll("git-bug.bridge." + name + ".target") {
			if t.Value == target {
				return e.Value
			}
		}
	}
	return ""
}
