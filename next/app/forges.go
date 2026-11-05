package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/next/domain/issues/issuesync"
	"github.com/guilbep/git-board/next/forge"
	"github.com/guilbep/git-board/next/forge/github"
	"github.com/guilbep/git-board/next/forge/gitlab"
	"github.com/guilbep/git-board/next/store/forgecopy"
)

// The forges' sync (docs/spec/sync.md): issuesync's run, with the forges
// of .git-board.toml whose sync is on, each opened fresh for its run, and
// the settings the run reads from the board's config, as v1's
// internal/store gives them.
// ponytail: v1's dry run (--check), the forges found in the git remotes,
// a Markdown folder's files around the run and the cycle's comment keys
// (Quote, Carry, Restore, Published, ReplyTo) are v1's still.

// NamedForge is a forge project opened for a run, by its name in the
// settings.
type NamedForge struct {
	Name string
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

// OpenForges opens the adapters of the forges whose sync is on, each with
// its posting account's token.
func (b *Board) OpenForges(ctx context.Context) ([]NamedForge, error) {
	var out []NamedForge
	for _, f := range b.Config.Forges.List {
		if !f.Sync {
			continue
		}
		var x forge.Forge
		var err error
		switch f.Kind {
		case "github":
			api := ""
			if f.Host == "github.com" {
				api = GitHubAPI
			}
			x, err = github.New(ctx, github.Options{Project: f.Project, Host: f.Host, Login: f.Login, API: api})
		case "gitlab":
			x, err = gitlab.New(ctx, baseURL(f), f.Project, f.Login)
		default:
			err = fmt.Errorf("kind %q: github or gitlab", f.Kind)
		}
		if err != nil {
			return nil, fmt.Errorf("forge %s: %w", f.Name, err)
		}
		out = append(out, NamedForge{f.Name, x})
	}
	if len(out) == 0 {
		return nil, errors.New("no forge has its sync on")
	}
	return out, nil
}

// SyncsAssignees says whether the assignees live natively on the forge of
// key: the assignee field's home there is the forge's own assignees.
func (c *Config) SyncsAssignees(key string) bool {
	gp, _ := c.Tools.GithubProject.Get()
	inProject := func(field string) bool {
		return len(c.Schema.Fields[field].Project) > 0 || gp.Number != 0 && slices.Contains(projectFields, field)
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

// ForgeRun is one forge sync: its forges, and how it goes.
type ForgeRun struct {
	Forges []NamedForge
	// AllowCreate is the consent to create issues on a forge the board
	// never synced with, and MaxCreate or more on one in a run. Without
	// it, the run creates none there, and tells it (held).
	AllowCreate bool
	See         func(SyncEvent) // nil hears nothing
}

// MaxCreate is how many creations on one forge, in one run, need consent:
// a forge may email its watchers once per issue.
const MaxCreate = 50

// SyncEvent is one thing a run did on one forge. Way is pull or push.
// What is created, updated, linked, posted or pushed, with Issue the
// issue's id; warning, waiting or left, with Line; held: the Ops
// creations waiting for consent, Line saying why; progress: Done of
// Total.
type SyncEvent struct {
	Forge, Way, What, Issue string
	Ops                     int
	Line                    string
	Done, Total             int
}

// Text says the event in words, its issue by name; "" for progress.
func (e SyncEvent) Text(name string) string {
	switch e.What {
	case "created":
		return fmt.Sprintf("pull a new issue, %s (%d changes)", name, e.Ops)
	case "updated":
		return fmt.Sprintf("pull %d changes into %s", e.Ops, name)
	case "linked":
		return "link " + name + ", found by its marker"
	case "posted":
		return "create " + name
	case "pushed":
		return "send the changes of " + name
	case "warning", "waiting":
		return e.Line
	case "left":
		return "left out of the run: " + e.Line
	case "held":
		return fmt.Sprintf("%d issues not created: they need consent, %s", e.Ops, e.Line)
	}
	return ""
}

// ErrNotSwitched: the repository's issues are still the git-bug fork's.
var ErrNotSwitched = errors.New("this repository's issues are still git-bug's, in refs/bugs: run git-board switch first")

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
	mark, err := first("refs/board/switched")
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
	see := run.See
	if see == nil {
		see = func(SyncEvent) {}
	}
	names := map[string]string{} // forge key -> name
	var fs []forge.Forge
	for _, f := range run.Forges {
		names[f.Key()] = f.Name
		fs = append(fs, f.Forge)
	}
	_, forgeOnly := b.Kernel.Store().(*forgecopy.Store)
	cfg := b.Config
	opt := issuesync.Options{
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
		Warn: func(msg string) {
			ev := SyncEvent{What: "warning", Line: msg}
			for key, name := range names {
				if strings.HasPrefix(msg, key+" ") {
					ev.Forge = name
				}
			}
			see(ev)
		},
		Did: func(key, what, id string, ops int) {
			w := "pull"
			if what == "posted" || what == "pushed" {
				w = "push"
			}
			see(SyncEvent{Forge: names[key], Way: w, What: what, Issue: id, Ops: ops})
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
			return false
		},
	}
	ctx = forge.WithWaits(ctx, func(w forge.Wait) {
		see(SyncEvent{Forge: names[w.Forge], What: "waiting", Line: waitWords(w.Why, w.For)})
	})
	switch way {
	case WayPull:
		return issuesync.Pull(ctx, b.Kernel, opt, fs...)
	case WayPush:
		return issuesync.Push(ctx, b.Kernel, opt, fs...)
	}
	return issuesync.Sync(ctx, b.Kernel, opt, fs...)
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
