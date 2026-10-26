package store

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

// GitHubAPI is GitHub's REST API for github.com: "" is api.github.com;
// tests, a fake.
var GitHubAPI = ""

// OpenForges opens the adapters of forges, each with its posting
// account's token.
func OpenForges(ctx context.Context, forges []config.Forge) ([]NamedForge, error) {
	var out []NamedForge
	for _, f := range forges {
		var x forge.Forge
		var err error
		switch f.Kind {
		case "github":
			x, err = github.New(ctx, github.Options{Project: f.Project, Host: f.Host, Login: f.Login, API: githubAPI(f)})
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

// githubAPI is GitHubAPI for a forge on github.com, "" (the host's own)
// for an Enterprise one.
func githubAPI(f config.Forge) string {
	if f.Host == "github.com" {
		return GitHubAPI
	}
	return ""
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
	case "warning":
		return e.Line
	default:
		return ""
	}
	if dry {
		return "would " + l
	}
	return l
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
	if err := b.Switched(); err != nil {
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

// switchedRef is the switch's mark: written last by switch --apply, a
// commit of nothing at the switch's time. It goes to the remotes with the
// entities, and a clone without one takes a remote's.
const switchedRef = "refs/board/switched"

// Switched refuses a repository whose issues are in refs/bugs and that
// wasn't switched: a forge sync would pull the fork's mirrors without the
// fork's links, as issues of their own, and push them back as duplicates.
func (b *Backend) Switched() error {
	bugs, err := b.s.Refs("refs/bugs/")
	if err != nil || len(bugs) == 0 {
		return err
	}
	if h, err := b.s.Ref(switchedRef); err != nil || h != gitstore.ZeroHash {
		return err
	}
	return ErrNotSwitched
}

// markSwitched writes the switch's mark, unless there.
func (b *Backend) markSwitched() error {
	if h, err := b.s.Ref(switchedRef); err != nil || h != gitstore.ZeroHash {
		return err
	}
	tree, err := b.s.WriteTree(nil)
	if err != nil {
		return err
	}
	c, err := b.s.WriteCommit(gitstore.Commit{Tree: tree})
	if err != nil {
		return err
	}
	return b.s.UpdateRef(ctx, switchedRef, c, gitstore.ZeroHash)
}

// markCopy is where a remote's mark is fetched to.
func markCopy(remote string) string { return "refs/board-remotes/" + remote + "/switched" }

// fetchMark takes the remote's mark, for a clone of the fork's issues that
// has none: a switch on another clone switched this one too.
func (b *Backend) fetchMark(remote string) error {
	if err := b.Switched(); !errors.Is(err, ErrNotSwitched) {
		return err
	}
	if _, err := b.run(nil, "fetch", "--no-write-fetch-head", "--quiet", remote, "+"+switchedRef+":"+markCopy(remote)); err != nil {
		if strings.Contains(err.Error(), "couldn't find remote ref") {
			return nil
		}
		return fmt.Errorf("fetching the switch's mark from %s: %w", remote, err)
	}
	theirs, err := b.s.Ref(markCopy(remote))
	if err != nil || theirs == gitstore.ZeroHash {
		return err
	}
	return b.s.UpdateRef(ctx, switchedRef, theirs, gitstore.ZeroHash)
}

// pushMark sends the mark to a remote whose copy here isn't it.
func (b *Backend) pushMark(remote string) error {
	ours, err := b.s.Ref(switchedRef)
	if err != nil || ours == gitstore.ZeroHash {
		return err
	}
	theirs, err := b.s.Ref(markCopy(remote))
	if err != nil || theirs == ours {
		return err
	}
	if _, err := b.run(nil, "push", "--quiet", remote, switchedRef+":"+switchedRef); err != nil {
		return fmt.Errorf("pushing the switch's mark to %s: %w", remote, err)
	}
	return b.s.UpdateRef(ctx, markCopy(remote), ours, theirs)
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
// It first removes the copies a day old: a run killed mid-way leaves its
// own.
func scratchOf(ctx context.Context, from, home string) (*gitstore.Store, func(), error) {
	parent := filepath.Join(home, "board")
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, nil, err
	}
	left, _ := filepath.Glob(filepath.Join(parent, "check-*"))
	for _, d := range left {
		if fi, err := os.Stat(d); err == nil && time.Since(fi.ModTime()) > 24*time.Hour {
			_ = os.RemoveAll(d)
		}
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
func (c *counted) Visibility(ctx context.Context) (forge.Visibility, error) {
	return c.f.Visibility(ctx)
}

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

func (c *counted) Issue(ctx context.Context, issue forge.Ref) (forge.Issue, error) {
	if strings.HasPrefix(issue.ID, "dry-") {
		return forge.Issue{}, forge.ErrNotFound
	}
	return c.f.Issue(ctx, issue)
}

// write makes a write, unless dry, then tells it, with op as its Line: a
// write some values of were refused landed the others.
func (c *counted) write(what, op, issueID string, do func() error) error {
	var err error
	if !c.dry {
		err = do()
	}
	if err == nil || errors.As(err, new(*forge.Refused)) {
		c.see(SyncEvent{Forge: c.name, Way: "push", What: what, Issue: issueID, Line: op})
	}
	return err
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

func (c *counted) CreateComment(ctx context.Context, issue forge.Ref, body string) (forge.Comment, error) {
	out := forge.Comment{Body: body}
	err := c.write("comment", "create-comment", issue.ID, func() (err error) {
		out, err = c.f.CreateComment(ctx, issue, body)
		return err
	})
	if c.dry && err == nil {
		out.ID, out.Time = c.madeUp(), time.Now().Unix()
		out.Author, err = c.f.Me(ctx)
	}
	return out, err
}

func (c *counted) EditComment(ctx context.Context, issue forge.Ref, commentID, body string) error {
	return c.write("edit", "edit-comment", issue.ID, func() error { return c.f.EditComment(ctx, issue, commentID, body) })
}

func (c *counted) SetTitle(ctx context.Context, issue forge.Ref, title string) error {
	return c.write("change", "set-title", issue.ID, func() error { return c.f.SetTitle(ctx, issue, title) })
}

func (c *counted) SetBody(ctx context.Context, issue forge.Ref, body string) error {
	return c.write("edit", "set-body", issue.ID, func() error { return c.f.SetBody(ctx, issue, body) })
}

func (c *counted) SetStatus(ctx context.Context, issue forge.Ref, status string) error {
	return c.write("change", "set-status", issue.ID, func() error { return c.f.SetStatus(ctx, issue, status) })
}

func (c *counted) ChangeLabels(ctx context.Context, issue forge.Ref, add, remove []string) error {
	return c.write("change", "change-labels", issue.ID, func() error { return c.f.ChangeLabels(ctx, issue, add, remove) })
}

func (c *counted) SetAssignees(ctx context.Context, issue forge.Ref, users []forge.Account) ([]forge.Account, error) {
	kept := users // a dry run: the forge keeps them all
	err := c.write("change", "set-assignees", issue.ID, func() (err error) {
		kept, err = c.f.SetAssignees(ctx, issue, users)
		return err
	})
	return kept, err
}
