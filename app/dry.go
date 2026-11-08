package app

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

	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/domain/people"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/kernel"
	"github.com/guilbep/git-board/store/git"
	"github.com/guilbep/git-board/view"
)

// A forge run's writes, heard; and its dry run (--check): the run on a
// copy of the repository's refs, every write to a forge counted and
// answered without being made. As v1's internal/store/forges.go.

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

// replying is a counted forge whose comments form threads: it forwards
// forge.Replier, which counted alone would hide, so a reply lands as a
// reply (ADR 0021: v1 posted it as a top-level comment).
type replying struct{ *counted }

var _ forge.Replier = replying{}

// wrap is c as the run sees it: with the optional capabilities of the
// forge it wraps, and no more.
func (c *counted) wrap() forge.Forge {
	if _, ok := c.f.(forge.Replier); ok {
		return replying{c}
	}
	return c
}

// Reply posts body in the thread holding toCommentID, heard as a comment.
func (r replying) Reply(ctx context.Context, issue forge.Ref, toCommentID, body string) (forge.Comment, error) {
	c := r.counted
	out := forge.Comment{Body: body}
	err := c.write("comment", "create-comment", issue.ID, func() (err error) {
		out, err = c.f.(forge.Replier).Reply(ctx, issue, toCommentID, body)
		return err
	})
	if c.dry && err == nil {
		out.ID, out.Time = c.madeUp(), time.Now().Unix()
		out.Author, err = c.f.Me(ctx)
	}
	return out, err
}

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
	return view.wrap(), nil
}

func (c *counted) Issues(ctx context.Context, since int64) ([]forge.Issue, error) {
	return c.f.Issues(ctx, since)
}

func (c *counted) Complete(ctx context.Context, is forge.Issue) (forge.Issue, error) {
	if strings.HasPrefix(is.ID, "dry-") {
		return is, nil // a dry run's made-up issue: nothing on the forge to read
	}
	return c.f.Complete(ctx, is)
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

// scratch is a copy of the repository's board, for a dry run to write
// to, opened as a board; done removes it. It is a mirror in the board's
// folder: on the same disk, git links the objects rather than copying
// them. The sync's journal goes with it, for the check to skip what a cut
// run read.
func (b *Board) scratch(ctx context.Context, g *git.Store) (*kernel.Board, func(), error) {
	parent := g.Folder()
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
	cmd := exec.CommandContext(ctx, "git", "clone", "--mirror", "--quiet", filepath.Dir(parent), dir)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if out, err := cmd.CombinedOutput(); err != nil {
		done()
		return nil, nil, fmt.Errorf("copying the refs for a check: %w: %s", err, strings.TrimSpace(string(out)))
	}
	s, err := git.OpenGitDir(dir)
	if err != nil {
		done()
		return nil, nil, err
	}
	if data, err := os.ReadFile(filepath.Join(parent, "sync.lock")); err == nil {
		if err := os.MkdirAll(s.Folder(), 0o755); err == nil {
			_ = os.WriteFile(filepath.Join(s.Folder(), "sync.lock"), data, 0o644)
		}
	}
	v, err := view.Open(ctx, filepath.Join(s.Folder(), "v2", "view.db"))
	if err != nil {
		s.Close()
		done()
		return nil, nil, err
	}
	kb, err := kernel.Open(ctx, s, v, issues.Kind, people.Kind)
	if err != nil {
		v.Close()
		s.Close()
		done()
		return nil, nil, err
	}
	return kb, func() { kb.Close(); done() }, nil
}
