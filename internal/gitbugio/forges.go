package gitbugio

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/github"
	"github.com/guilbep/git-board/core/forge/gitlab"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/syncer"
	"github.com/guilbep/git-board/internal/config"
)

// Forge sync is the core's syncer, with the forges of .git-board.toml
// (config.Forge), each opened fresh for its run, and their tokens from the
// environment or git's credential helper (tokens.go).

// NamedForge is a forge project opened for a run, by its name in the
// settings.
type NamedForge struct {
	Name string
	forge.Forge
}

// OpenForges opens the adapters of forges, each with its posting
// account's token.
func OpenForges(ctx context.Context, forges []config.Forge) ([]NamedForge, error) {
	var out []NamedForge
	for _, f := range forges {
		var x forge.Forge
		var err error
		switch f.Kind {
		case "github":
			x, err = github.New(ctx, github.Options{Project: f.Project, Host: f.Host, Login: f.Login})
		case "gitlab":
			x, err = gitlab.New(ctx, f.BaseURL(), f.Project, f.Login)
		default:
			err = fmt.Errorf("kind %q: github or gitlab", f.Kind)
		}
		if err != nil {
			return nil, fmt.Errorf("forge %s: %w", f.Name, err)
		}
		out = append(out, NamedForge{f.Name, x})
	}
	return out, nil
}

// ForgeRun is one forge sync: its forges, and how it goes.
type ForgeRun struct {
	Forges         []NamedForge
	PublishPrivate bool
	// DryRun is a check: the run on a copy of the repository's refs, every
	// write to a forge counted and answered without being made. Nothing
	// changes, here or on a forge.
	DryRun bool
	See    func(SyncEvent) // nil hears nothing
}

// SyncEvent is one thing a run did, or a dry run would do, on one forge.
// Way is pull or push. What is, as syncer.Options.Did says, created,
// updated, linked, posted or pushed, with Issue the issue's id; or a
// write to the forge: comment, edit or change, Issue then the forge
// issue's id; or warning, with Line.
type SyncEvent struct {
	Forge, Way, What, Issue string
	Ops                     int // created, updated: the operations pulled
	Line                    string
}

// SyncForges pulls from every forge of the run, then pushes to each.
func (b *Backend) SyncForges(ctx context.Context, run ForgeRun) (syncer.Report, error) {
	return b.forgeSync(ctx, run, syncer.Sync, "")
}

// GetForges brings the forges' changes in and sends nothing.
func (b *Backend) GetForges(ctx context.Context, run ForgeRun) (syncer.Report, error) {
	return b.forgeSync(ctx, run, syncer.Pull, "")
}

// SendForges sends the local changes to the forges and takes nothing.
func (b *Backend) SendForges(ctx context.Context, run ForgeRun) (syncer.Report, error) {
	return b.forgeSync(ctx, run, syncer.Push, "")
}

// SyncIssue syncs one issue with every forge of the run.
func (b *Backend) SyncIssue(ctx context.Context, run ForgeRun, idPrefix string) (syncer.Report, error) {
	id, err := b.resolve("issue", idPrefix)
	if err != nil {
		return syncer.Report{}, err
	}
	return b.forgeSync(ctx, run, syncer.Sync, id)
}

type syncWay func(context.Context, *gitstore.Store, syncer.Options, ...forge.Forge) (syncer.Report, error)

func (b *Backend) forgeSync(ctx context.Context, run ForgeRun, way syncWay, issueID string) (syncer.Report, error) {
	if err := b.switched(); err != nil {
		return syncer.Report{}, err
	}
	see := run.See
	if see == nil {
		see = func(SyncEvent) {}
	}
	s := b.s
	if run.DryRun {
		scratch, done, err := b.scratch(ctx)
		if err != nil {
			return syncer.Report{}, err
		}
		defer done()
		s = scratch
	}
	names := map[string]string{} // forge key -> name
	ids := new(int)
	var fs []forge.Forge
	for _, f := range run.Forges {
		names[f.Key()] = f.Name
		fs = append(fs, &counted{f: f.Forge, name: f.Name, dry: run.DryRun, ids: ids, see: see})
	}
	opt := syncer.Options{
		PublishPrivate: run.PublishPrivate,
		Issue:          issueID,
		Local:          isLocal,
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
			way := "pull"
			if what == "posted" || what == "pushed" {
				way = "push"
			}
			see(SyncEvent{Forge: names[key], Way: way, What: what, Issue: id, Ops: ops})
		},
	}
	return way(ctx, s, opt, fs...)
}

// ErrNotSwitched: the repository's issues are still the git-bug fork's.
var ErrNotSwitched = errors.New("this repository's issues are still git-bug's, in refs/bugs: run git-board switch first")

// switched refuses a forge sync on a repository whose issues are in
// refs/bugs and that the core never pulled: it would pull the fork's
// mirrors without the fork's links, as issues of their own, and push them
// back as duplicates.
func (b *Backend) switched() error {
	bugs, err := b.s.Refs("refs/bugs/")
	if err != nil || len(bugs) == 0 {
		return err
	}
	if synced, err := b.s.Refs("refs/board/sync/"); err != nil || len(synced) > 0 {
		return err
	}
	return ErrNotSwitched
}

// isLocal says a comment stays in git-board: a line comment, a reply or a
// review (review:* metadata) never goes to a forge.
func isLocal(meta map[string]string) bool {
	for k := range meta {
		if strings.HasPrefix(k, "review:") {
			return true
		}
	}
	return false
}

// scratch is a copy of the repository, for a dry run to write to; done
// removes it. It is a mirror in git-board's folder: on the same disk, git
// links the objects rather than copying them. (A clone sharing them
// through objects/info/alternates is quicker, but go-git doesn't read
// those.)
func (b *Backend) scratch(ctx context.Context) (*gitstore.Store, func(), error) {
	return scratchOf(ctx, b.s.CommonDir(), b.s.CommonDir())
}

// scratchOf is a mirror of the repository at from, in home's board folder.
func scratchOf(ctx context.Context, from, home string) (*gitstore.Store, func(), error) {
	parent := filepath.Join(home, "board")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, nil, err
	}
	dir, err := os.MkdirTemp(parent, "check-")
	if err != nil {
		return nil, nil, err
	}
	done := func() { _ = os.RemoveAll(dir) }
	cmd := exec.CommandContext(ctx, "git", "clone", "--mirror", "--quiet", from, dir)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		done()
		return nil, nil, fmt.Errorf("copying the refs for a check: %w: %s", err, strings.TrimSpace(string(out)))
	}
	s, err := gitstore.Open(dir)
	if err != nil {
		done()
		return nil, nil, err
	}
	return s, done, nil
}

// counted is a forge whose writes the run hears: made, then heard; or, in
// a dry run, heard and answered as if made. It implements every method
// itself, so a write the interface gains can't pass by unheard.
type counted struct {
	f    forge.Forge
	name string
	dry  bool
	ids  *int // a dry run's made-up ids, shared by every account's view
	see  func(SyncEvent)
	made func(id string) // nil, or hears a dry run's made-up id for each issue it would create
}

var _ forge.Forge = (*counted)(nil)

func (c *counted) Key() string                                   { return c.f.Key() }
func (c *counted) Server() string                                { return c.f.Server() }
func (c *counted) Me(ctx context.Context) (forge.Account, error) { return c.f.Me(ctx) }
func (c *counted) Public(ctx context.Context) (bool, error)      { return c.f.Public(ctx) }

func (c *counted) As(ctx context.Context, a forge.Account) (forge.Forge, error) {
	g, err := c.f.As(ctx, a)
	if err != nil {
		return nil, err
	}
	view := *c
	view.f = g
	return &view, nil
}

func (c *counted) Issues(ctx context.Context, since int64) ([]forge.Issue, error) {
	return c.f.Issues(ctx, since)
}

func (c *counted) Issue(ctx context.Context, id string) (forge.Issue, error) {
	if strings.HasPrefix(id, "dry-") {
		return forge.Issue{}, forge.ErrNotFound
	}
	return c.f.Issue(ctx, id)
}

// write makes a write, unless dry, then tells it, with op as its Line.
func (c *counted) write(what, op, issueID string, do func() error) error {
	if !c.dry {
		if err := do(); err != nil {
			return err
		}
	}
	c.see(SyncEvent{Forge: c.name, Way: "push", What: what, Issue: issueID, Line: op})
	return nil
}

// madeUp is a dry run's id for what it would create: never one of a
// forge's own.
func (c *counted) madeUp() string {
	*c.ids++
	return "dry-" + strconv.Itoa(*c.ids)
}

func (c *counted) CreateIssue(ctx context.Context, title, body string) (forge.Issue, error) {
	if c.dry {
		me, err := c.f.Me(ctx)
		id, now := c.madeUp(), time.Now().Unix()
		if c.made != nil {
			c.made(id)
		}
		return forge.Issue{ID: id, Number: id, URL: id, Author: me, Created: now, Updated: now, Title: title, Body: body, Status: "open"}, err
	}
	return c.f.CreateIssue(ctx, title, body)
}

func (c *counted) CreateComment(ctx context.Context, issueID, body string) (forge.Comment, error) {
	out := forge.Comment{Body: body}
	err := c.write("comment", "create-comment", issueID, func() (err error) {
		out, err = c.f.CreateComment(ctx, issueID, body)
		return err
	})
	if c.dry && err == nil {
		out.ID, out.Time = c.madeUp(), time.Now().Unix()
		out.Author, err = c.f.Me(ctx)
	}
	return out, err
}

func (c *counted) EditComment(ctx context.Context, issueID, commentID, body string) error {
	return c.write("edit", "edit-comment", issueID, func() error { return c.f.EditComment(ctx, issueID, commentID, body) })
}

func (c *counted) SetTitle(ctx context.Context, issueID, title string) error {
	return c.write("change", "set-title", issueID, func() error { return c.f.SetTitle(ctx, issueID, title) })
}

func (c *counted) SetBody(ctx context.Context, issueID, body string) error {
	return c.write("edit", "set-body", issueID, func() error { return c.f.SetBody(ctx, issueID, body) })
}

func (c *counted) SetStatus(ctx context.Context, issueID, status string) error {
	return c.write("change", "set-status", issueID, func() error { return c.f.SetStatus(ctx, issueID, status) })
}

func (c *counted) ChangeLabels(ctx context.Context, issueID string, add, remove []string) error {
	return c.write("change", "change-labels", issueID, func() error { return c.f.ChangeLabels(ctx, issueID, add, remove) })
}

func (c *counted) SetAssignees(ctx context.Context, issueID string, users []forge.Account) ([]forge.Account, error) {
	kept := users // a dry run: the forge keeps them all
	err := c.write("change", "set-assignees", issueID, func() (err error) {
		kept, err = c.f.SetAssignees(ctx, issueID, users)
		return err
	})
	return kept, err
}
