package app

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/guilbep/git-board/conf"
	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/kernel"
	bsync "github.com/guilbep/git-board/sync"
)

// The board's state, named (docs/design/first-run/proposal.md): every
// lifecycle the code otherwise infers from files, refs and git config,
// computed once. git board status, GET /api/status and the web app's
// Start page read it, and nothing else derives these states. Each part
// names its state in a word, and the next actions say what moves it on:
// a sentence for a person, a command for the CLI, a route for the web
// app and an agent.

// State is the board's state, as one value.
type State struct {
	// Phase is "new" (no identity, or no issue and no forge run yet), or
	// "ready".
	Phase     string         `json:"phase"`
	File      FileState      `json:"file"`
	Me        MeState        `json:"me"`
	Datastore DatastoreState `json:"datastore"`
	Forges    []ForgeState   `json:"forges"`
	Project   ProjectState   `json:"project"`
	Run       RunState       `json:"run"`
	Issues    int            `json:"issues"` // how many the board shows
	Cycle     bool           `json:"cycle"`  // the dev cycle is on ([cycle], [experimental])
	Next      []Action       `json:"next"`   // in order: the first is the one to do first
}

// FileState is .git-board.toml: "none" (the defaults hold), "current",
// "older" (a schema migration is pending), or "newer" (read-only to this
// git-board).
type FileState struct {
	State   string `json:"state"`
	Path    string `json:"path"`
	Version int    `json:"version,omitempty"`
}

// MeState is who writes: "none" (the board is read-only), or "chosen".
type MeState struct {
	State string `json:"state"`
	ID    string `json:"id,omitempty"`
}

// DatastoreState is where the board is kept: "git", "markdown",
// "postgres", or "forges" (the forges alone).
type DatastoreState struct {
	State   string `json:"state"`
	Where   string `json:"where"`              // as a sentence says it
	FromEnv bool   `json:"from_env,omitempty"` // GIT_BOARD_DATASTORE names it, over board.datastore
}

// ForgeState is a forge's state: "off" (its sync is off), "no-token",
// "first" (on, never synced: a push waits for consent before it creates
// issues), or "synced".
type ForgeState struct {
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Project    string `json:"project"`
	Way        string `json:"way"` // push, pull or both
	State      string `json:"state"`
	Discovered bool   `json:"discovered,omitempty"` // found in the git remotes, not in the file
	Token      string `json:"token,omitempty"`      // where the token is: an environment variable, or git's credential helper
	LastRun    string `json:"last_run,omitempty"`   // RFC 3339
}

// ProjectState is the GitHub Project bridge: "none", "broken" (the table
// doesn't read: Problem says why), or "configured".
type ProjectState struct {
	State   string   `json:"state"`
	Project string   `json:"project,omitempty"` // owner/number
	Fields  []string `json:"fields,omitempty"`  // the fields its columns hold
	Stated  bool     `json:"stated,omitempty"`  // the file states them; else the default seven
	Write   bool     `json:"write,omitempty"`
	Problem string   `json:"problem,omitempty"`
}

// RunState is the sync run: "idle", "running" (in this process or
// another), or "stopped" (a run stopped before it went through: the next
// one resumes it).
type RunState struct {
	State   string        `json:"state"`
	Stopped *bsync.CutRun `json:"stopped,omitempty"`
	Problem string        `json:"problem,omitempty"`
}

// Action is a next step: what it does, the command that does it, and the
// route that does it. Either can be "": the web app shows a button, and
// never a command.
type Action struct {
	ID  string `json:"id"`
	Say string `json:"say"`
	CLI string `json:"cli,omitempty"`
	API string `json:"api,omitempty"`
}

// State computes the board's state. It asks no forge anything: tokens
// are only looked up (git board forge status --check asks).
func (b *Board) State(ctx context.Context) (State, error) {
	s := State{Forges: []ForgeState{}, Next: []Action{}}
	s.File = fileState(b.Dir)
	if b.Issues.Me != "" {
		s.Me = MeState{State: "chosen", ID: string(b.Issues.Me)}
	} else {
		s.Me = MeState{State: "none"}
	}
	s.Datastore = datastoreState(b)
	ids, err := issues.Kind.Match(ctx, b.Kernel, kernel.Query{})
	if err != nil {
		return s, err
	}
	s.Issues = len(ids)
	s.Cycle = b.CycleOn()

	statuses, err := b.ForgeStatuses(ctx, false)
	if err != nil {
		return s, err
	}
	ran := false
	for _, f := range statuses {
		fs := ForgeState{Name: f.Name, Kind: f.Kind, Project: f.Project, Way: f.Way, Discovered: f.Discovered}
		for _, t := range f.Tokens {
			if t.Found {
				fs.Token = t.Source
				break
			}
		}
		seen, last, known, err := forgeLastRun(ctx, b, f)
		if err != nil {
			return s, err
		}
		if last > 0 {
			fs.LastRun = time.Unix(last, 0).UTC().Format(time.RFC3339)
		}
		ran = ran || seen
		switch {
		case !f.Sync:
			fs.State = "off"
		case fs.Token == "":
			fs.State = "no-token"
		case !seen && !known:
			fs.State = "on" // its sync entity's key is the forge's own: the first sync says
		case !seen:
			fs.State = "first"
		default:
			fs.State = "synced"
		}
		s.Forges = append(s.Forges, fs)
	}

	s.Project = projectState(b)
	s.Run = runState(b)

	if s.Me.State == "none" || (s.Issues == 0 && !ran) {
		s.Phase = "new"
	} else {
		s.Phase = "ready"
	}
	s.Next = next(s)
	return s, nil
}

func fileState(dir string) FileState {
	path := configPath(dir)
	fs := FileState{State: "none", Path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		return fs // none: the defaults hold
	}
	v, err := conf.Version(string(data))
	if err != nil {
		return FileState{State: "current", Path: path} // it doesn't parse: lint says why
	}
	fs.Version = v
	switch {
	case v < conf.Current:
		fs.State = "older"
	case v > conf.Current:
		fs.State = "newer"
	default:
		fs.State = "current"
	}
	return fs
}

func datastoreState(b *Board) DatastoreState {
	ds := b.Kernel.Store()
	d := DatastoreState{Where: Where(ds), FromEnv: os.Getenv(DatastoreEnv) != ""}
	switch Where(ds) {
	case "git":
		d.State = "git"
	case "the forges alone":
		d.State = "forges"
	default:
		d.State = map[bool]string{true: "markdown", false: "postgres"}[b.Files != nil]
	}
	return d
}

// forgeLastRun says whether a forge has its sync entity, keyed as the
// adapter keys it (host/project): none before the first sync, or since a
// reset or a drop forgot it, which is what the consent to create reads.
// last is when its last two-way run started, 0 for none (a pull or a push
// alone records none). known is false where the key is
// only the forge's to say: a GitLab project the remotes name by its path,
// whose key is its numeric id.
// ponytail: GitHub's key is tried as written and in lower case; the
// adapter's FullName can differ in case only.
func forgeLastRun(ctx context.Context, b *Board, f ForgeStatus) (seen bool, last int64, known bool, err error) {
	if f.Kind == "gitlab" && strings.Trim(f.Project, "0123456789") != "" {
		return false, 0, false, nil
	}
	host := cmp.Or(f.Host, "github.com")
	for _, key := range []string{host + "/" + f.Project, strings.ToLower(host + "/" + f.Project)} {
		e, err := bsync.Kind.Get(ctx, b.Kernel, bsync.StateID(key))
		if errors.Is(err, kernel.ErrNotFound) {
			continue
		}
		if err != nil {
			return false, 0, true, err
		}
		return true, e.State.LastRun, true, nil
	}
	return false, 0, true, nil
}

func projectState(b *Board) ProjectState {
	c, err := b.Config.Project()
	if err != nil {
		return ProjectState{State: "broken", Problem: err.Error()}
	}
	if !c.Configured() {
		return ProjectState{State: "none"}
	}
	gp, _ := b.Config.Tools.GithubProject.Get()
	return ProjectState{State: "configured", Project: c.Name(), Fields: c.Holds, Stated: gp.Fields != nil, Write: c.Write}
}

// Syncing says a sync run holds the board's lock now, in this process or
// another.
func (b *Board) Syncing() bool { return bsync.Running(b.Kernel.Store()) }

func runState(b *Board) RunState {
	ds := b.Kernel.Store()
	if b.Syncing() {
		return RunState{State: "running"}
	}
	cut, err := bsync.Cut(ds)
	if err != nil {
		return RunState{State: "idle", Problem: err.Error()}
	}
	if cut != nil {
		return RunState{State: "stopped", Stopped: cut}
	}
	return RunState{State: "idle"}
}

// next is what moves the board on, the first first.
func next(s State) []Action {
	out := []Action{}
	add := func(id, say, cli, api string) { out = append(out, Action{ID: id, Say: say, CLI: cli, API: api}) }
	if s.Me.State == "none" {
		add("me", "Choose who you are: until then the board is read-only.", "git board me --new", "PUT /api/me")
	}
	if s.File.State == "older" {
		add("schema", fmt.Sprintf("Move %s to schema %d.", conf.FileName, conf.Current), "git board config upgrade", "PUT /api/config")
	}
	if s.Run.State == "stopped" {
		add("resume", "A sync stopped before it went through: sync again, and it resumes where it stopped.", "git board sync", "POST /api/sync")
	}
	syncs := false
	for _, f := range s.Forges {
		switch f.State {
		case "no-token":
			add("login:"+f.Name, fmt.Sprintf("Log in to %s (%s): no token is found for it.", f.Name, f.Project), "git board forge login "+f.Name, "POST /api/forges/auth")
		case "first":
			say := fmt.Sprintf("Run the first sync with %s (%s).", f.Name, f.Project)
			if f.Way != "pull" {
				say += " It creates issues there only with your consent."
			}
			add("sync:"+f.Name, say, "git board sync --check", "POST /api/sync")
		case "synced":
			syncs = true
		}
	}
	if len(s.Forges) == 0 {
		add("forge", "Add a forge: the board then mirrors its issues.", "git remote add origin <url>", "POST /api/forges")
	}
	if s.Project.State == "broken" {
		add("project", "Fix the [github-project] table: "+s.Project.Problem, "git board lint", "PUT /api/github-project")
	}
	if s.Project.State == "configured" && !s.Project.Stated {
		add("fields", "Write the Project's fields in the file: it holds the default seven until then.", "git board project configure", "PUT /api/github-project")
	}
	if s.Issues == 0 && !syncs && s.Me.State == "chosen" {
		add("new", "Write the first issue.", "git board new", "POST /api/issues")
	}
	return out
}
