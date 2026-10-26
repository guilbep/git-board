// Package fake is a forge kept in memory, for tests. Its control methods
// (Add, Edit, Delete, ...) act on the forge's side, as its users would;
// the forge.Forge methods are what sync sees.
package fake

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/core/forge"
)

// ErrCrash is what a write returns after it landed, when After says so.
var ErrCrash = errors.New("fake: the write landed, then the connection broke")

// Write is one write the forge was asked for, by the account As. Issue is
// the issue's id, empty for create-issue.
type Write struct {
	Op    string // create-issue, create-comment, edit-comment, set-title, set-body, set-status, change-labels, set-assignees
	As    forge.Account
	Issue string
	Args  []string
}

type entry struct {
	forge.Issue
	confidential, deleted bool
}

// project is the forge's state, shared by every account's view of it.
type project struct {
	mu            sync.Mutex
	key           string
	me            forge.Account
	vis           forge.Visibility
	issues        []*entry
	tokens        map[string]bool
	next          int
	maxAssignees  int
	refuseLabels  []string
	before, after func(Write) error
	writes        []Write
}

// Forge is a forge in memory, seen through one account. Its issues stay in
// the order they were added.
type Forge struct {
	p  *project
	as forge.Account
}

// New makes an empty, private forge with the given key. Its posting
// account is {1, board}.
func New(key string) *Forge {
	me := forge.Account{ID: "1", Login: "board"}
	return &Forge{p: &project{key: key, me: me, vis: forge.Private, tokens: map[string]bool{}}, as: me}
}

func now() int64 { return time.Now().Unix() }

func (f *Forge) Key() string { return f.p.key }

// Server is the key up to its first /.
func (f *Forge) Server() string {
	host, _, _ := strings.Cut(f.p.key, "/")
	return host
}

func (f *Forge) Me(context.Context) (forge.Account, error) { return f.as, nil }

func (f *Forge) Visibility(context.Context) (forge.Visibility, error) {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	return f.p.vis, nil
}

func (f *Forge) As(_ context.Context, a forge.Account) (forge.Forge, error) {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	if a.ID != f.p.me.ID && !f.p.tokens[a.ID] {
		return nil, forge.ErrNoToken
	}
	return &Forge{p: f.p, as: a}, nil
}

// The forge's side.

// SetVisibility says who may read the project.
func (f *Forge) SetVisibility(v forge.Visibility) {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	f.p.vis = v
}

// Grant gives the account a token here.
func (f *Forge) Grant(a forge.Account) {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	f.p.tokens[a.ID] = true
}

// LimitAssignees keeps at most n assignees per issue, as GitLab's free
// edition does. 0 is no limit.
func (f *Forge) LimitAssignees(n int) {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	f.p.maxAssignees = n
}

// RefuseLabels refuses these labels, added or removed, for good: a labels
// write sends the others, and names these in a *forge.Refused.
func (f *Forge) RefuseLabels(labels ...string) {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	f.p.refuseLabels = labels
}

// Before is called before each write, without the forge's lock: an error
// refuses the write, which doesn't land.
func (f *Forge) Before(hook func(Write) error) {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	f.p.before = hook
}

// After is called once a write landed: an error is returned instead of the
// write's result, as when the connection breaks after the forge answered.
func (f *Forge) After(hook func(Write) error) {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	f.p.after = hook
}

// Writes lists the writes that landed, in order.
func (f *Forge) Writes() []Write {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	return slices.Clone(f.p.writes)
}

// Add appends an issue, as its author opened it. An empty ID, number, URL
// or status, and a zero time, get one.
func (f *Forge) Add(is forge.Issue) forge.Issue {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	t := now()
	if is.ID == "" {
		is.ID = f.p.id(1_000_000)
	}
	if is.Number == "" {
		is.Number = strconv.Itoa(len(f.p.issues) + 1)
	}
	if is.URL == "" {
		is.URL = "https://" + f.p.key + "/issues/" + is.Number
	}
	if is.Status == "" {
		is.Status = "open"
	}
	if is.Created == 0 {
		is.Created = t
	}
	is.Updated = max(is.Updated, is.Created)
	for i := range is.Comments {
		f.p.fill(&is.Comments[i], t)
	}
	e := &entry{Issue: clone(is)}
	f.p.issues = append(f.p.issues, e)
	return clone(e.Issue)
}

// Edit changes the first issue with this number, as its users would. The
// issue's update time becomes now, and so does the change time of each
// field and comment whose text changed, unless change set one. It is false
// when there is no such issue.
func (f *Forge) Edit(number string, change func(*forge.Issue)) bool {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	e := f.p.byNumber(number)
	if e == nil {
		return false
	}
	before := clone(e.Issue)
	if e.Changed == nil {
		e.Changed = map[string]int64{}
	}
	change(&e.Issue)
	e.Issue = clone(e.Issue) // the change may have put the caller's slices in
	t := now()
	e.Updated = t
	for field, v := range map[string][2]string{
		"title": {before.Title, e.Title}, "body": {before.Body, e.Body}, "status": {before.Status, e.Status},
	} {
		if v[0] != v[1] && e.Changed[field] == before.Changed[field] {
			e.Changed[field] = t
		}
	}
	for i := range e.Comments {
		c := &e.Comments[i]
		j := slices.IndexFunc(before.Comments, func(o forge.Comment) bool { return o.ID == c.ID })
		if j >= 0 && before.Comments[j].Body != c.Body && before.Comments[j].Edited == c.Edited {
			c.Edited = t
		}
		f.p.fill(c, t)
	}
	return true
}

// Delete removes the issue with this number, as if deleted on the forge.
func (f *Forge) Delete(number string) bool {
	return f.mark(number, func(e *entry) { e.deleted = true })
}

// SetConfidential makes the issue confidential, or not.
func (f *Forge) SetConfidential(number string, on bool) bool {
	return f.mark(number, func(e *entry) { e.confidential = on })
}

func (f *Forge) mark(number string, do func(*entry)) bool {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	e := f.p.byNumber(number)
	if e != nil {
		do(e)
	}
	return e != nil
}

// Get returns the issue with this number, confidential or not. It is false
// when there is none, or it was deleted.
func (f *Forge) Get(number string) (forge.Issue, bool) {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	if e := f.p.byNumber(number); e != nil {
		return clone(e.Issue), true
	}
	return forge.Issue{}, false
}

// All returns every issue not deleted, confidential ones included.
func (f *Forge) All() []forge.Issue {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	var out []forge.Issue
	for _, e := range f.p.issues {
		if !e.deleted {
			out = append(out, clone(e.Issue))
		}
	}
	return out
}

// What sync reads.

func (f *Forge) Issues(_ context.Context, since int64) ([]forge.Issue, error) {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	var out []forge.Issue
	for _, e := range f.p.issues {
		if !e.deleted && !e.confidential && e.Updated >= since {
			out = append(out, clone(e.Issue))
		}
	}
	return out, nil
}

func (f *Forge) Issue(_ context.Context, id string) (forge.Issue, error) {
	f.p.mu.Lock()
	defer f.p.mu.Unlock()
	e, err := f.p.live(id)
	if err != nil {
		return forge.Issue{}, err
	}
	return clone(e.Issue), nil
}

// What sync writes.

func (f *Forge) CreateIssue(_ context.Context, title, body string) (forge.Issue, error) {
	var out forge.Issue
	err := f.write(Write{Op: "create-issue", Args: []string{title, body}}, func(p *project, t int64) error {
		n := strconv.Itoa(len(p.issues) + 1)
		e := &entry{Issue: forge.Issue{ID: p.id(1_000_000), Number: n, URL: "https://" + p.key + "/issues/" + n,
			Author: f.as, Created: t, Updated: t, Title: title, Body: body, Status: "open", Changed: map[string]int64{}}}
		p.issues = append(p.issues, e)
		out = clone(e.Issue)
		return nil
	})
	return out, err
}

func (f *Forge) CreateComment(_ context.Context, issueID, body string) (forge.Comment, error) {
	var out forge.Comment
	err := f.edit("create-comment", issueID, []string{body}, func(e *entry, p *project, t int64) error {
		out = forge.Comment{ID: p.id(2_000_000), Body: body, Author: f.as, Time: t}
		e.Comments = append(e.Comments, out)
		return nil
	})
	return out, err
}

func (f *Forge) EditComment(_ context.Context, issueID, commentID, body string) error {
	return f.edit("edit-comment", issueID, []string{commentID, body}, func(e *entry, _ *project, t int64) error {
		i := slices.IndexFunc(e.Comments, func(c forge.Comment) bool { return c.ID == commentID })
		if i < 0 {
			return forge.ErrNotFound
		}
		e.Comments[i].Body, e.Comments[i].Edited = body, t
		return nil
	})
}

func (f *Forge) SetTitle(_ context.Context, issueID, title string) error {
	return f.set("title", issueID, title, func(e *entry) { e.Title = title })
}

func (f *Forge) SetBody(_ context.Context, issueID, body string) error {
	return f.set("body", issueID, body, func(e *entry) { e.Body = body })
}

func (f *Forge) SetStatus(_ context.Context, issueID, status string) error {
	if status != "open" && status != "closed" {
		return fmt.Errorf("fake: bad status %q", status)
	}
	return f.set("status", issueID, status, func(e *entry) { e.Status = status })
}

func (f *Forge) ChangeLabels(_ context.Context, issueID string, add, remove []string) error {
	args := slices.Concat(add, []string{"--"}, remove)
	var refused []string
	err := f.edit("change-labels", issueID, args, func(e *entry, p *project, _ int64) error {
		for _, l := range slices.Concat(add, remove) {
			if slices.Contains(p.refuseLabels, l) {
				refused = append(refused, l)
			}
		}
		for _, l := range add {
			if !slices.Contains(e.Labels, l) && !slices.Contains(refused, l) {
				e.Labels = append(e.Labels, l)
			}
		}
		e.Labels = slices.DeleteFunc(e.Labels, func(l string) bool { return slices.Contains(remove, l) && !slices.Contains(refused, l) })
		return nil
	})
	if err == nil && len(refused) > 0 {
		err = &forge.Refused{Values: refused, Why: "the fake refuses them"}
	}
	return err
}

func (f *Forge) SetAssignees(_ context.Context, issueID string, users []forge.Account) ([]forge.Account, error) {
	var ids []string
	for _, u := range users {
		ids = append(ids, u.ID)
	}
	var kept []forge.Account
	err := f.edit("set-assignees", issueID, ids, func(e *entry, p *project, _ int64) error {
		e.Assignees = nil
		for _, u := range users {
			if !slices.ContainsFunc(e.Assignees, func(a forge.Account) bool { return a.ID == u.ID }) &&
				(p.maxAssignees == 0 || len(e.Assignees) < p.maxAssignees) {
				e.Assignees = append(e.Assignees, u)
			}
		}
		kept = slices.Clone(e.Assignees)
		return nil
	})
	return kept, err
}

func (f *Forge) set(field, issueID, value string, do func(*entry)) error {
	return f.edit("set-"+field, issueID, []string{value}, func(e *entry, _ *project, t int64) error {
		do(e)
		e.Changed[field] = t
		return nil
	})
}

// edit writes to a live issue, and moves its update time.
func (f *Forge) edit(op, issueID string, args []string, do func(*entry, *project, int64) error) error {
	return f.write(Write{Op: op, Issue: issueID, Args: args}, func(p *project, t int64) error {
		e, err := p.live(issueID)
		if err != nil {
			return err
		}
		if e.Changed == nil {
			e.Changed = map[string]int64{}
		}
		if err := do(e, p, t); err != nil {
			return err
		}
		e.Updated = t
		return nil
	})
}

// write runs the Before hook, lands the write, then runs the After hook.
func (f *Forge) write(w Write, do func(*project, int64) error) error {
	w.As = f.as
	f.p.mu.Lock()
	before, after := f.p.before, f.p.after
	f.p.mu.Unlock()
	if before != nil {
		if err := before(w); err != nil {
			return err
		}
	}
	f.p.mu.Lock()
	err := do(f.p, now())
	if err == nil {
		f.p.writes = append(f.p.writes, w)
	}
	f.p.mu.Unlock()
	if err != nil || after == nil {
		return err
	}
	return after(w)
}

func (p *project) id(base int) string {
	p.next++
	return strconv.Itoa(base + p.next)
}

// fill gives a comment added by the forge's side an id and a time.
func (p *project) fill(c *forge.Comment, t int64) {
	if c.ID == "" {
		c.ID = p.id(2_000_000)
	}
	if c.Time == 0 {
		c.Time = t
	}
}

func (p *project) byNumber(number string) *entry {
	for _, e := range p.issues {
		if e.Number == number && !e.deleted {
			return e
		}
	}
	return nil
}

func (p *project) live(id string) (*entry, error) {
	for _, e := range p.issues {
		if e.ID == id && !e.deleted {
			if e.confidential {
				return nil, forge.ErrConfidential
			}
			return e, nil
		}
	}
	return nil, forge.ErrNotFound
}

func clone(is forge.Issue) forge.Issue {
	is.Labels = slices.Clone(is.Labels)
	is.Assignees = slices.Clone(is.Assignees)
	is.Comments = slices.Clone(is.Comments)
	is.Changed = maps.Clone(is.Changed)
	return is
}
