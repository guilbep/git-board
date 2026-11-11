package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/domain/issues/issuesync"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/github"
	"github.com/guilbep/git-board/forge/gitlab"
	"github.com/guilbep/git-board/kernel"
	"github.com/guilbep/git-board/store/forgecopy"
	"github.com/guilbep/git-board/store/git"
	bsync "github.com/guilbep/git-board/sync"
)

// The forges' sync (docs/spec/sync.md): issuesync's run, with the forges
// of .git-board.toml, or those found in origin's URLs, whose sync is on, each opened fresh for its run, and
// the settings the run reads from the board's config, as v1's
// internal/store gave them.
// ponytail: v1's Quote and Published, which read the body's sections
// (issuesync/meanings.go), and a check of a board kept outside git, are
// v1's still.

// NamedForge is a forge project opened for a run, by its name in the
// settings. Way is its table's way (forge.Table's Direction): the run
// pulls from those that pull, and pushes to those that push.
type NamedForge struct {
	Name string
	Way  string
	forge.Forge
}

// GitHubAPI is GitHub's REST API for github.com: "" is api.github.com;
// tests, a fake.
var GitHubAPI = ""

// baseURL is a GitLab forge's: http on a loopback host, https elsewhere.
func baseURL(f forge.Table) string {
	host := f.Host
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	if host == "localhost" || host == "127.0.0.1" || host == "::1" {
		return "http://" + f.Host
	}
	return "https://" + f.Host
}

// Forges are the forges a sync runs with: the settings' [forges.<name>]
// tables, or with none, those found in origin's URLs and push URLs, as
// written and as git resolves them (insteadOf). A forge origin fetches
// from syncs; one only a push URL names is listed with its sync off, as
// the file alone says what syncs (ADR 0008), and a push mirror is no
// place this team keeps its issues until it says so. Origin alone: a fork's
// upstream isn't where this team's issues go. Without origin, none. A
// local path is no forge, and isn't told; say hears each other URL that
// names none, each forge found, and each left out, as v1 says them.
func (b *Board) Forges(ctx context.Context, say func(string)) ([]forge.Table, error) {
	found, err := b.FoundForges(ctx, say)
	if err != nil || !b.Config.Forges.Discover {
		return found, err
	}
	var out []forge.Table
	seen := map[string]string{} // key -> forge name
	for _, f := range found {
		project, err := b.resolveProject(ctx, f)
		if err != nil {
			say(fmt.Sprintf("forge %s, found in the remotes, is left out: %s %s: %v", f.Name, f.Host, cmp.Or(f.Path, f.Project), err))
			continue
		}
		f.Project = project
		key := strings.ToLower(f.Host + "/" + f.Project)
		if other, ok := seen[key]; ok { // two paths of one project: a rename's old one
			say(fmt.Sprintf("forge %s, found in the remotes, is left out: %s %s: the same project as %s", f.Name, f.Host, cmp.Or(f.Path, f.Project), other))
			continue
		}
		seen[key] = f.Name
		where := f.Project
		if f.Path != "" {
			where = f.Path + " (id " + f.Project + ")"
		}
		say(fmt.Sprintf("forge %s, found in the remotes: %s %s", f.Name, f.Host, where))
		out = append(out, f)
	}
	return out, nil
}

// FoundForges are Forges' forges as the settings and the remotes name
// them, with nothing asked of a forge: a found one's project is its path,
// as written in the URL.
func (b *Board) FoundForges(ctx context.Context, say func(string)) ([]forge.Table, error) {
	if !b.Config.Forges.Discover {
		return b.Config.Forges.List, nil
	}
	c := &b.forges
	fresh := false
	f, err := c.found.get(c.stamp(ctx, b.Dir), func() (foundIn, error) {
		fresh = true
		var f foundIn
		var err error
		f.forges, err = b.foundForges(ctx, func(s string) { f.skips = append(f.skips, s); say("skipped " + s) })
		if err == nil {
			// a request gone mid-read read no URL: kept, its empty list
			// hid the forges until git's config changed
			err = ctx.Err()
		}
		return f, err
	})
	if !fresh { // kept: its skips said again, as a read would say them
		for _, s := range f.skips {
			say("skipped " + s)
		}
	}
	return slices.Clone(f.forges), err
}

// foundForges is FoundForges, read from git: skipped hears each URL
// that names no forge.
func (b *Board) foundForges(ctx context.Context, skipped func(string)) ([]forge.Table, error) {
	if has, err := b.hasRemote(ctx, NumbersRemote); err != nil || !has {
		return nil, err
	}
	urls := func(args ...string) []string {
		out, _ := gitRun(ctx, b.Dir, args...) // no pushurl is no error of the run's
		var us []string
		for _, u := range strings.Split(strings.TrimSpace(out), "\n") {
			if u = NoCredentials(u); u != "" && !localPath(u) {
				us = append(us, u)
			}
		}
		return us
	}
	fetch := append(urls("remote", "get-url", "--all", NumbersRemote), urls("config", "--get-all", "remote."+NumbersRemote+".url")...)
	resolved := append(urls("remote", "get-url", "--all", NumbersRemote), urls("remote", "get-url", "--push", "--all", NumbersRemote)...)
	written := append(urls("config", "--get-all", "remote."+NumbersRemote+".url"), urls("config", "--get-all", "remote."+NumbersRemote+".pushurl")...)
	_, skips := forge.Discover(resolved) // an alias as written isn't told: its resolved URL is
	found, _ := forge.Discover(append(resolved, written...))
	for _, s := range skips {
		skipped(s)
	}
	fetched, _ := forge.Discover(fetch)
	for i, f := range found {
		found[i].Sync = slices.ContainsFunc(fetched, func(g forge.Table) bool { return sameProject(f, g) })
	}
	return found, nil
}

// sameProject says two found forges name one project, a URL's case aside.
func sameProject(a, b forge.Table) bool {
	return a.Kind == b.Kind && strings.EqualFold(a.Host, b.Host) && strings.EqualFold(a.Project, b.Project)
}

// forgeCache keeps, for a board's life, what finding the forges asked
// git and the forges: a server asks it on each GET /api/sync and
// /api/state. The remotes and the forges found in them are kept while
// git's config files are unchanged (stamp); a project as its forge names
// it (resolveProject: a call to the forge) once found.
// ponytail: a file a config includes (include.path) isn't watched; a
// project renamed on its forge shows on the board opened next (a config
// change, a restart).
type forgeCache struct {
	mu       sync.Mutex
	common   string // the repository's git directory, its worktrees' common one
	remotes  kept[[]string]
	found    kept[foundIn]
	projects map[string]string // kind host path -> project: kept for the board's life, a forge's answer
	// kind host url login -> what tokenFound found: kept by time, not a
	// stamp: a credential helper's store says nothing of when it changed
	tokens map[string]tokenSeen
	// the GitLab forges pictures may come from (gitlabForges), and each
	// host's token for its pictures (mediaToken), its own lock held
	// across a lookup
	gitlab  kept[[]forge.Table]
	mediaMu sync.Mutex
	media   map[string]mediaToken
}

// foundIn is what finding the forges in the remotes found, and skipped.
type foundIn struct {
	forges []forge.Table
	skips  []string
}

// stamp names the state of git's config files, the repository's and
// yours: each one's size and time; "" when it can't be told.
func (c *forgeCache) stamp(ctx context.Context, dir string) string {
	common := c.commonDir(ctx, dir)
	if common == "" {
		return ""
	}
	files := []string{filepath.Join(common, "config")}
	if home, err := os.UserHomeDir(); err == nil {
		files = append(files, filepath.Join(home, ".gitconfig"))
	}
	files = append(files, filepath.Join(cmp.Or(os.Getenv("XDG_CONFIG_HOME"), filepath.Join(os.Getenv("HOME"), ".config")), "git", "config"))
	var b strings.Builder
	for _, f := range files {
		if st, err := os.Stat(f); err == nil {
			fmt.Fprintf(&b, "%s %d %d\n", f, st.Size(), st.ModTime().UnixNano())
		} else {
			fmt.Fprintf(&b, "%s -\n", f)
		}
	}
	return b.String()
}

// commonDir is the repository's git directory, shared by its worktrees:
// where its config and refs are; "" when git can't tell.
func (c *forgeCache) commonDir(ctx context.Context, dir string) string {
	c.mu.Lock()
	common := c.common
	c.mu.Unlock()
	if common == "" {
		out, err := gitRun(ctx, dir, "rev-parse", "--path-format=absolute", "--git-common-dir")
		if err != nil {
			return ""
		}
		common = strings.TrimSpace(out)
		c.mu.Lock()
		c.common = common
		c.mu.Unlock()
	}
	return common
}

// resolveProject is a found forge's project as a table names it, read
// through the adapter: GitLab's numeric id, as links are keyed on it;
// GitHub's owner/repo as GitHub spells it, as a URL's case is the user's.
func (b *Board) resolveProject(ctx context.Context, f forge.Table) (string, error) {
	c := &b.forges
	key := f.Kind + " " + f.Host + " " + f.Project
	c.mu.Lock()
	p, ok := c.projects[key]
	c.mu.Unlock()
	if ok {
		return p, nil
	}
	p, err := resolveProject(ctx, f)
	if err == nil {
		c.mu.Lock()
		if c.projects == nil {
			c.projects = map[string]string{}
		}
		c.projects[key] = p
		c.mu.Unlock()
	}
	return p, err
}

func resolveProject(ctx context.Context, f forge.Table) (string, error) {
	switch f.Kind {
	case "gitlab":
		g, err := gitlab.New(ctx, baseURL(f), f.Project, f.Login)
		if err != nil {
			return "", err
		}
		return g.ProjectID(ctx)
	case "github":
		api := ""
		if f.Host == "github.com" {
			api = GitHubAPI
		}
		g, err := github.New(ctx, github.Options{Project: f.Project, Host: f.Host, Login: f.Login, API: api})
		if err != nil {
			return "", err
		}
		return g.FullName(ctx)
	}
	return f.Project, nil
}

// localPath says a remote URL is a directory here, never a forge.
func localPath(u string) bool {
	return strings.HasPrefix(u, "/") || strings.HasPrefix(u, ".") || strings.HasPrefix(u, "file://") ||
		(len(u) > 2 && u[1] == ':' && (u[2] == '\\' || u[2] == '/')) // C:\ on Windows
}

// OpenForges opens the adapters of the forges (Forges) whose sync is on,
// each with its posting account's token.
func (b *Board) OpenForges(ctx context.Context, tables []forge.Table) ([]NamedForge, error) {
	var out []NamedForge
	for _, f := range tables {
		if !f.Sync {
			continue
		}
		x, err := OpenForge(ctx, f)
		if err != nil {
			return nil, fmt.Errorf("forge %s: %w", f.Name, err)
		}
		out = append(out, NamedForge{Name: f.Name, Way: f.Direction(), Forge: x})
	}
	if len(out) == 0 {
		return nil, errors.New("no forge has its sync on")
	}
	return out, nil
}

// OpenForge opens a forge project's issues through its adapter, with the
// table's posting account's token, as OpenLabeler does. Tests replace it
// with a fake.
var OpenForge = func(ctx context.Context, f forge.Table) (forge.Forge, error) {
	switch f.Kind {
	case "github":
		api := ""
		if f.Host == "github.com" {
			api = GitHubAPI
		}
		return github.New(ctx, github.Options{Project: f.Project, Host: f.Host, Login: f.Login, API: api})
	case "gitlab":
		return gitlab.New(ctx, baseURL(f), f.Project, f.Login)
	}
	return nil, fmt.Errorf("kind %q: github or gitlab", f.Kind)
}

// SyncsAssignees says whether the assignees live natively on the forge of
// key: the assignee field's home there is the forge's own assignees.
func (c *Config) SyncsAssignees(key string) bool {
	gp, _ := c.Tools.GithubProject.Get()
	inProject := func(field string) bool {
		return len(c.Schema.Fields[field].Project) > 0 || gp.Holds(field)
	}
	return forge.Target(c.Schema, inProject, c.Forges.ForgeOf(key), "assignee") == forge.TargetAssignees
}

// Way is a forge run's way: pull, push, or both (sync).
type Way string

// The ways.
const (
	WayPull Way = "pull"
	WayPush Way = "push"
	WaySync Way = "sync"
)

// forgeWayOf is a forge table's way (forge.Table's Direction) as the
// sync engine spells it: the table's both is the engine's sync.
func forgeWayOf(way string) bsync.Way {
	switch way {
	case forge.WayPush:
		return bsync.Push
	case forge.WayPull:
		return bsync.Pull
	}
	return bsync.Both
}

// ForgeRun is one forge sync: its forges, and how it goes.
type ForgeRun struct {
	Forges []NamedForge
	// AllowCreate is the consent to create issues on a forge the board
	// never synced with, and MaxCreate or more on one in a run. Without
	// it, the run creates none there, and tells it (held).
	AllowCreate bool
	// DryRun is a check: the run on a copy of the repository's refs,
	// every write to a forge counted and answered without being made.
	// Nothing changes, here or on a forge.
	DryRun bool
	// Only are the parts the run writes (Parts), nil every one: a part
	// left out is left for the next run.
	Only []Part
	See  func(SyncEvent) // nil hears nothing
}

// Part is one part of what a forge run writes: pull-changes, pull-new,
// create or send (docs/prd/config-fit.md, Part 2).
type Part = issuesync.Part

// Parts are the parts, in a run's order.
var Parts = issuesync.Parts

// ParseParts reads a list of parts, as sync --only takes it: "pull-new,send".
func ParseParts(list []string) ([]Part, error) {
	var out []Part
	for _, s := range list {
		p := Part(strings.TrimSpace(s))
		if !slices.Contains(Parts, p) {
			return nil, usage(fmt.Errorf("part %q: pull-changes, pull-new, create or send", s))
		}
		out = append(out, p)
	}
	return out, nil
}

// MaxCreate is how many creations on one forge, in one run, need consent:
// a forge may email its watchers once per issue.
const MaxCreate = 50

// SyncEvent is one thing a run did on one forge. Way is pull or push.
// What is created, updated, linked, posted or pushed, with Issue the
// issue's id and Name its forge number and title; warning, waiting or left, with Line; held: the Ops
// creations waiting for consent, Line saying why; progress: Done of
// Total.
type SyncEvent struct {
	Forge, Way, What, Issue, Name string
	Ops                           int
	Line                          string
	Done, Total                   int
}

// Text says the event in words, its issue by name ("would create CO2-3
// Export CSV" in a dry run); "" for a write, which a run counts.
func (e SyncEvent) Text(dry bool, name string) string {
	var l string
	switch e.What {
	case "created":
		l = fmt.Sprintf("pull a new issue, %s (%d changes)", name, e.Ops)
	case "updated":
		l = fmt.Sprintf("pull %d changes into %s", e.Ops, name)
	case "linked":
		l = "link " + name + ", found by its marker"
	case "posted":
		l = "create " + name
	case "pushed":
		l = "send the changes of " + name
	case "restored":
		l = "put back the board's values on " + name + ", changed on the backup"
	case "warning", "waiting":
		return e.Line
	case "left":
		return "left out of the run: " + e.Line
	case "release":
		l = map[string]string{"created": "publish the release ", "edited": "update the release ", "attached": "attach a file to the release "}[e.Line] + e.Issue
	case "held":
		if dry {
			return fmt.Sprintf("%d issues to create need consent: %s", e.Ops, e.Line)
		}
		return fmt.Sprintf("%d issues not created: they need consent, %s", e.Ops, e.Line)
	default:
		return ""
	}
	if dry {
		return "would " + l
	}
	return l
}

// Part is the part an event counts in: an issue pulled that changed
// (pull-changes), a new one pulled (pull-new), one created (create), one
// sent its changes (send); "" for any other.
func (e SyncEvent) Part() Part { return issuesync.PartOf(e.What) }

// ErrNotSwitched: the repository's issues are still the git-bug fork's.
var ErrNotSwitched = errors.New("this repository still holds git-bug data, in refs/bugs: migrate it with git-board v0.11 (`git board switch`), then use v0.12")

// switched refuses a repository whose issues are in refs/bugs and that
// wasn't switched: a forge sync would pull the fork's mirrors as issues
// of their own, and push them back as duplicates.
func (b *Board) switched(ctx context.Context) error {
	if !b.inGit() {
		return nil
	}
	first := func(prefix string) (bool, error) {
		out, err := exec.CommandContext(ctx, "git", "-C", b.Dir, "for-each-ref", "--count=1", "--format=%(refname)", prefix).Output()
		if err != nil {
			return false, fmt.Errorf("reading %s: %w", prefix, err)
		}
		return strings.TrimSpace(string(out)) != "", nil
	}
	bugs, err := first("refs/bugs/")
	if err != nil || !bugs {
		return err
	}
	mark, err := first(switchedRef)
	if err != nil || mark {
		return err
	}
	return ErrNotSwitched
}

// SyncForges runs one way of the forges' sync.
func (b *Board) SyncForges(ctx context.Context, way Way, run ForgeRun) (issuesync.Report, error) {
	if err := b.switched(ctx); err != nil {
		return issuesync.Report{}, err
	}
	kb := b.Kernel
	adopt := func() {}
	switch g, ok := kb.Store().(*git.Store); {
	case run.DryRun && !ok:
		return issuesync.Report{}, missing("a check needs a board kept in git; this board is kept in " + b.DatastoreName())
	case run.DryRun:
		scratch, done, was, err := b.scratch(ctx, g)
		if err != nil {
			return issuesync.Report{}, err
		}
		defer done()
		kb = scratch
		// what the check found the board holding, the sync after it
		// doesn't read again; a journal is a shortcut: failing, it costs
		// reads, nothing more
		adopt = func() { _ = bsync.Adopt(g, scratch.Store(), was) }
	case b.Files != nil:
		// a Markdown folder's hand edits are read in first, and its files
		// follow what the sync wrote, each under the folder's lock; not
		// across the run, which waits on the forges and would hold every
		// other write back meanwhile
		_, end, err := b.Files.Writing(ctx)
		if err != nil {
			return issuesync.Report{}, err
		}
		end()
		defer func() {
			if err := b.Files.Render(ctx); err != nil && b.Issues.Notice != nil {
				b.Issues.Notice("markdown: %v", err)
			}
		}()
	}
	see := run.See
	if see == nil {
		see = func(SyncEvent) {}
	}
	names := map[string]string{}   // forge key -> name
	ways := map[string]bsync.Way{} // forge key -> its table's way
	ids := new(int)
	var fs []forge.Forge
	for _, f := range run.Forges {
		names[f.Key()], ways[f.Key()] = f.Name, forgeWayOf(f.Way)
		fs = append(fs, (&counted{f: f.Forge, name: f.Name, dry: run.DryRun, ids: ids, see: see}).wrap())
	}
	_, forgeOnly := b.Kernel.Store().(*forgecopy.Store)
	cfg := b.Config
	opt := issuesync.Options{
		Ways:           ways,
		Check:          run.DryRun,
		ForgeOnly:      forgeOnly,
		PublishPrivate: cfg.Forges.PublishPrivate,
		Local: func(meta map[string]string) bool {
			for k := range meta {
				if strings.HasPrefix(k, "review:") {
					return true
				}
			}
			return false
		},
		Held:      cfg.Held,
		Assignees: cfg.SyncsAssignees,
		Carry:     issuesync.Carry,
		Restore:   issuesync.Restore,
		ReplyTo:   issuesync.ReplyTo,
		Warn: func(msg string) {
			ev := SyncEvent{What: "warning", Line: msg}
			for key, name := range names {
				if strings.HasPrefix(msg, key+" ") {
					ev.Forge = name
				}
			}
			see(ev)
		},
		Only: run.Only,
		Did: func(key, what, id, name string, ops int) {
			w := "pull"
			if what == "posted" || what == "pushed" || what == "restored" {
				w = "push"
			}
			see(SyncEvent{Forge: names[key], Way: w, What: what, Issue: id, Name: name, Ops: ops})
		},
		Progress: func(key, way string, done, total int, name string) {
			see(SyncEvent{Forge: names[key], Way: way, What: "progress", Done: done, Total: total, Line: name})
		},
		Left: func(key string, why error) {
			see(SyncEvent{Forge: names[key], What: "left", Line: why.Error()})
		},
		Create: func(key string, n int, first bool) bool {
			why := "the first sync with this forge"
			switch {
			case run.AllowCreate:
				return true
			case !first && n < MaxCreate:
				return true
			case !first:
				why = fmt.Sprintf("%d or more in one run", MaxCreate)
			}
			see(SyncEvent{Forge: names[key], Way: "push", What: "held", Ops: n, Line: why})
			return false // a check holds them as a sync would: it says what Sync does
		},
	}
	ctx = forge.WithWaits(ctx, func(w forge.Wait) {
		see(SyncEvent{Forge: names[w.Forge], What: "waiting", Line: waitWords(w.Why, w.For)})
	})
	im := &issuesync.Mapper{Options: opt}
	ms := []bsync.Mapper{im}
	if len(cfg.Release.Binaries) > 0 && !forgeOnly { // v1 publishes the releases of binaries alone
		var raw []forge.Forge
		for _, f := range run.Forges {
			if forgeWayOf(f.Way) != bsync.Pull { // a release is published: a read-only mirror takes none
				raw = append(raw, f.Forge)
			}
		}
		rm := b.releaseMapper(run.DryRun, func(key, what, tag string) {
			see(SyncEvent{Forge: names[key], Way: "push", What: "release", Issue: tag, Line: what})
		})
		ms = append(ms, onForges{rm, raw})
	}
	err := bsync.Run(ctx, kb, bsync.Way(way), fs, ms...)
	if err == nil {
		adopt()
	}
	return im.Report, err
}

// onForges is a mapper run over forges of its own: the release mapper
// over the forges as opened, as counted ones carry no releases.
type onForges struct {
	bsync.Mapper
	fs []forge.Forge
}

func (o onForges) Carries(forge.Forge) bool { return true }
func (o onForges) Run(ctx context.Context, b *kernel.Board, way bsync.Way, _ []forge.Forge) error {
	var fs []forge.Forge
	for _, f := range o.fs {
		if o.Mapper.Carries(f) {
			fs = append(fs, f)
		}
	}
	if len(fs) == 0 {
		return nil
	}
	return o.Mapper.Run(ctx, b, way, fs)
}

// waitWords says a forge's wait: "GitHub doesn't answer: trying again in
// 30 s".
func waitWords(why string, d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%s: trying again in %d s", why, int(d.Round(time.Second)/time.Second))
	}
	return fmt.Sprintf("%s: trying again in %d min", why, int(d.Round(time.Minute)/time.Minute))
}

// EventIssue is an event's issue by name: its number, else its id's
// start, and its title.
func (b *Board) EventIssue(ctx context.Context, id string) string {
	short := id[:min(7, len(id))]
	th, err := b.Issues.Show(ctx, id)
	if err != nil {
		return short
	}
	if th.Number != "" {
		return th.Number + " " + th.Title
	}
	return short + " " + th.Title
}
