package issues

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/next/domain/people"
	"github.com/guilbep/git-board/next/kernel"
)

// The issue domain's writes. Each reads the issue, decides on the head
// it will write on (kernel.Do, 5 tries when another writer moved it), and
// validates what it writes before writing it.

var (
	// ErrNoSuchComment is a comment id that isn't one of the issue's.
	ErrNoSuchComment = errors.New("no such comment on this issue")
	// ErrChanged is a text someone changed since the edit started: saving
	// would replace their text silently.
	ErrChanged = errors.New("someone changed this comment meanwhile")
)

// op is an operation by who, now. Its data is written as v1 writes it,
// keys sorted.
func op(kind string, who kernel.ID, data map[string]any) kernel.Op {
	raw, _ := json.Marshal(data)
	return kernel.Op{Kind: kind, Author: string(who), Time: time.Now().Unix(), Data: raw}
}

// writer is who writes, or people.ErrNoMe.
func (s *Service) writer() (kernel.ID, error) {
	if s.Me == "" {
		return "", people.ErrNoMe
	}
	return s.Me, nil
}

// do writes on the issue a name gives what decide answers; a deleted one
// is not found.
func (s *Service) do(ctx context.Context, name string, decide func(me kernel.ID, st *State) ([]kernel.Op, error)) (State, error) {
	me, err := s.writer()
	if err != nil {
		return State{}, err
	}
	id, err := s.Resolve(ctx, name)
	if err != nil {
		return State{}, err
	}
	e, err := Kind.Do(ctx, s.Board, id, func(cur kernel.Entity[State]) ([]kernel.Op, error) {
		if cur.State.Deleted {
			return nil, fmt.Errorf("issue %s: %w", id, ErrDeleted)
		}
		return decide(me, &cur.State)
	})
	return e.State, err
}

// NewReq asks for a new issue.
type NewReq struct {
	Title, Body string
	Assign      []string // field=value, as set's
	Force       bool     // write values the config doesn't allow
	MeName      string   // what @me means: --me; "" for who writes
}

// New creates an issue as who writes, with its fields in the same pack,
// and keeps its state in step with its status.
func (s *Service) New(ctx context.Context, r NewReq) (Issue, error) {
	ctx, done, werr := s.writing(ctx)
	if werr != nil {
		return Issue{}, werr
	}
	defer done()
	me, err := s.writer()
	if err != nil {
		return Issue{}, err
	}
	as, err := s.assignments(ctx, r.Assign, r.MeName, r.Force)
	if err != nil {
		return Issue{}, err
	}
	labels, _, err := s.fold(nil, as)
	if err != nil {
		return Issue{}, err
	}
	ops := []kernel.Op{op("create", me, map[string]any{"title": r.Title, "body": r.Body})}
	if add, _ := s.stored(labels, nil, nil); len(add) > 0 {
		ops = append(ops, op("change-labels", me, map[string]any{"add": add, "remove": nil}))
	}
	e, err := Kind.Create(ctx, s.Board, "", ops...)
	if err != nil {
		return Issue{}, fmt.Errorf("creating the issue: %w", err)
	}
	if _, err := s.inStep(ctx, string(e.ID), as, labels); err != nil {
		return Issue{}, err
	}
	return s.Get(ctx, string(e.ID))
}

// Get is an issue as a list shows it, by name.
func (s *Service) Get(ctx context.Context, name string) (Issue, error) {
	ctx = s.Board.Snapshot(ctx)
	th, err := s.Show(ctx, name)
	return th.Issue, err
}

// current is an issue's state as a write decides on it, by name: its
// labels, status and forges, from its log, without the thread and the
// people a Get reads.
func (s *Service) current(ctx context.Context, name string) (Issue, error) {
	id, err := s.Resolve(ctx, name)
	if err != nil {
		return Issue{}, err
	}
	e, err := Kind.Get(ctx, s.Board, id)
	if err != nil {
		return Issue{}, err
	}
	if e.State.Deleted {
		return Issue{}, fmt.Errorf("issue %s: %w", id, ErrDeleted)
	}
	d := index(&e.State)
	return s.issue(id, &d.Row, people.Directory{}, nil), nil
}

// Comment adds a comment as who writes, and answers its id and the
// issue's.
func (s *Service) Comment(ctx context.Context, name, body string) (kernel.OpID, kernel.ID, error) {
	ctx, done, werr := s.writing(ctx)
	if werr != nil {
		return "", "", werr
	}
	defer done()
	st, err := s.do(ctx, name, func(me kernel.ID, _ *State) ([]kernel.Op, error) {
		return []kernel.Op{op("comment", me, map[string]any{"body": body})}, nil
	})
	if err != nil {
		return "", "", err
	}
	return st.Comments[len(st.Comments)-1].ID, st.ID, nil
}

// text finds one of an issue's texts by id: its body (the create's id)
// or a comment, by any of its ids.
func text(st *State, id kernel.OpID) (target kernel.OpID, body string, err error) {
	if id == st.BodyOp {
		return id, st.Body, nil
	}
	if i, ok := st.comments[id]; ok && !st.removed[i] {
		c := st.all[i]
		return c.ID, c.Body, nil
	}
	return "", "", ErrNoSuchComment
}

// EditText replaces a text, the body or a comment's, by its id. base is
// the text the edit started from: another one there now refuses it
// (ErrChanged); "" skips the check.
func (s *Service) EditText(ctx context.Context, name string, id kernel.OpID, body, base string) error {
	ctx, done, werr := s.writing(ctx)
	if werr != nil {
		return werr
	}
	defer done()
	_, err := s.do(ctx, name, func(me kernel.ID, st *State) ([]kernel.Op, error) {
		target, now, err := text(st, id)
		switch {
		case err != nil:
			return nil, err
		case base != "" && now != base:
			return nil, ErrChanged
		case now == body:
			return nil, nil
		}
		return []kernel.Op{op("edit-text", me, map[string]any{"target": string(target), "body": body})}, nil
	})
	return err
}

// DeleteComment hides a comment (ADR 0019); the description isn't one.
func (s *Service) DeleteComment(ctx context.Context, name string, id kernel.OpID) error {
	ctx, done, werr := s.writing(ctx)
	if werr != nil {
		return werr
	}
	defer done()
	_, err := s.do(ctx, name, func(me kernel.ID, st *State) ([]kernel.Op, error) {
		target, _, err := text(st, id)
		if err != nil {
			return nil, err
		}
		if target == st.BodyOp {
			return nil, fmt.Errorf("%w: that is the description, not a comment", ErrNoSuchComment)
		}
		return []kernel.Op{op("delete", me, map[string]any{"target": string(target)})}, nil
	})
	return err
}

// Delete hides an issue (ADR 0019): no view shows it again.
func (s *Service) Delete(ctx context.Context, name string) (kernel.ID, error) {
	ctx, done, werr := s.writing(ctx)
	if werr != nil {
		return "", werr
	}
	defer done()
	st, err := s.do(ctx, name, func(me kernel.ID, st *State) ([]kernel.Op, error) {
		return []kernel.Op{op("delete", me, map[string]any{"target": string(st.BodyOp)})}, nil
	})
	return st.ID, err
}

// Edit changes the title, the body, or both, each when given and other
// than now; base is what they were when the edit started (ErrChanged
// when another writer changed one since).
func (s *Service) Edit(ctx context.Context, name string, title, body *string, base Thread) ([]string, error) {
	ctx, done, werr := s.writing(ctx)
	if werr != nil {
		return nil, werr
	}
	defer done()
	var changed []string
	_, err := s.do(ctx, name, func(me kernel.ID, st *State) ([]kernel.Op, error) {
		changed = nil
		var ops []kernel.Op
		if title != nil && *title != base.Title {
			if st.Title != base.Title {
				return nil, ErrChanged
			}
			ops = append(ops, op("set-title", me, map[string]any{"title": *title}))
			changed = append(changed, "title")
		}
		if body != nil && len(base.Events) > 0 && *body != base.Events[0].Body {
			if st.Body != base.Events[0].Body {
				return nil, ErrChanged
			}
			ops = append(ops, op("edit-text", me, map[string]any{"target": string(st.BodyOp), "body": *body}))
			changed = append(changed, "description")
		}
		return ops, nil
	})
	return changed, err
}

// SetStatus opens or closes an issue.
func (s *Service) SetStatus(ctx context.Context, name string, status Status) error {
	ctx, done, werr := s.writing(ctx)
	if werr != nil {
		return werr
	}
	defer done()
	if status != Open && status != Closed {
		return fmt.Errorf("status %q: want open or closed", status)
	}
	_, err := s.do(ctx, name, func(me kernel.ID, st *State) ([]kernel.Op, error) {
		if st.Status == status {
			return nil, nil
		}
		return []kernel.Op{op("set-status", me, map[string]any{"status": status})}, nil
	})
	return err
}

// ChangeLabels adds and removes labels, as git-board reads them: a flat
// kind is written as its plain label, an assignee in any case once. It
// answers the labels after.
func (s *Service) ChangeLabels(ctx context.Context, name string, added, removed []string) ([]string, error) {
	ctx, done, werr := s.writing(ctx)
	if werr != nil {
		return nil, werr
	}
	defer done()
	st, err := s.do(ctx, name, func(me kernel.ID, st *State) ([]kernel.Op, error) {
		add, rm := s.stored(added, removed, st.Labels)
		add = slices.DeleteFunc(slices.Clone(add), func(l string) bool { return slices.Contains(st.Labels, l) })
		rm = slices.DeleteFunc(slices.Clone(rm), func(l string) bool { return !slices.Contains(st.Labels, l) })
		if len(add) == 0 && len(rm) == 0 {
			return nil, nil
		}
		return []kernel.Op{op("change-labels", me, map[string]any{"add": add, "remove": rm})}, nil
	})
	if err != nil {
		return nil, fmt.Errorf("changing labels on %s: %w", name, err)
	}
	return s.Schema.Shown(slices.Clone(st.Labels)), nil
}

// stored is a change of labels as the issue keeps them, raw its labels
// now: a flat kind added as bug, and removed in each spelling raw has; an
// assignee there in another case not added again, removed in each case.
func (s *Service) stored(added, removed, raw []string) (a, r []string) {
	flat := func(l string) (string, bool) {
		if !s.Schema.FlatKinds() {
			return "", false
		}
		k := strings.TrimPrefix(l, "kind:")
		_, ok := s.Schema.Kinds[k]
		return k, ok
	}
	assignee := func(l string) (string, bool) {
		const p = "assignee:"
		if len(l) > len(p) && strings.EqualFold(l[:len(p)], p) {
			return p + l[len(p):], true
		}
		return l, false
	}
	for _, l := range added {
		if as, ok := assignee(l); ok && slices.ContainsFunc(raw, func(x string) bool { y, _ := assignee(x); return y == as }) {
			continue // there, in some case
		}
		if k, ok := flat(l); ok {
			if slices.Contains(raw, k) || slices.Contains(a, k) {
				continue
			}
			l = k
		}
		a = append(a, l)
	}
	for _, l := range removed {
		if as, ok := assignee(l); ok {
			for _, x := range raw {
				if y, _ := assignee(x); y == as && !slices.Contains(r, x) {
					r = append(r, x)
				}
			}
			continue
		}
		k, ok := flat(l)
		if !ok {
			r = append(r, l)
			continue
		}
		for _, sp := range []string{k, "kind:" + k} {
			if slices.Contains(raw, sp) && !slices.Contains(r, sp) {
				r = append(r, sp)
			}
		}
	}
	return a, r
}

// assignments reads field=value arguments: parsed, resolved (@me,
// sprints, aliases) and, unless force, held to the config.
func (s *Service) assignments(ctx context.Context, args []string, meName string, force bool) ([]Assignment, error) {
	var inUse []string // the labels in use, read when a field isn't configured: a list
	read := false
	if meName == "" && slices.ContainsFunc(args, func(a string) bool { return strings.HasSuffix(a, "@me") }) {
		meName = s.myName(ctx)
	}
	today := time.Now()
	var res []Assignment
	var errs []error
	for _, arg := range args {
		a, err := ParseAssignment(arg)
		if err == nil {
			a, err = a.resolve(s.Schema, meName, today)
		}
		if err == nil && !force && !read && a.Field != PlainLabel && !s.Schema.known(a.Field, nil) {
			counts, err := s.Labels(ctx)
			if err != nil {
				return nil, err
			}
			for _, c := range counts {
				inUse = append(inUse, c.Label)
			}
			read = true
		}
		if err == nil && !force {
			err = s.Schema.Check(a, inUse)
		}
		if err != nil {
			errs = append(errs, err)
			continue
		}
		res = append(res, a)
	}
	if err := errors.Join(errs...); err != nil {
		if !force && slices.ContainsFunc(errs, func(e error) bool { return errors.Is(e, ErrUnchecked) }) {
			return nil, errors.Join(err, errors.New("nothing written; --force writes it anyway"))
		}
		return nil, err
	}
	return res, nil
}

// myName is what @me means in a value: who writes's forge login, else
// their name.
func (s *Service) myName(ctx context.Context) string {
	dir, err := people.Read(ctx, s.Board, s.Avatars)
	if err != nil || s.Me == "" {
		return ""
	}
	if _, ok := dir.ByID[s.Me]; !ok {
		return ""
	}
	w := dir.Who(s.Me)
	if w.Login != "" {
		return w.Login
	}
	return w.Name
}

// fold applies assignments to labels, each on the last's result.
func (s *Service) fold(current []string, as []Assignment) ([]string, []Result, error) {
	labels := slices.Clone(current)
	results := make([]Result, 0, len(as))
	for _, a := range as {
		r, err := s.Schema.Change(labels, a)
		if err != nil {
			return nil, nil, err
		}
		labels = Fold(labels, r)
		results = append(results, r)
	}
	return labels, results, nil
}

// SetReq asks set to assign fields on one issue.
type SetReq struct {
	Name   string
	Assign []string
	Force  bool
	MeName string
}

// Set assigns fields on an issue in one change, and keeps its state in
// step with its status. It answers a line per assignment, as set prints
// them: what it did, the last one with what the state did.
func (s *Service) Set(ctx context.Context, r SetReq) ([]string, error) {
	ctx, done, werr := s.writing(ctx)
	if werr != nil {
		return nil, werr
	}
	defer done()
	as, err := s.assignments(ctx, r.Assign, r.MeName, r.Force)
	if err != nil {
		return nil, err
	}
	is, err := s.current(ctx, r.Name)
	if err != nil {
		return nil, err
	}
	labels, results, err := s.fold(is.Labels, as)
	if err != nil {
		return nil, err
	}
	added, removed := delta(is.Labels, labels)
	if len(added)+len(removed) > 0 {
		if _, err := s.ChangeLabels(ctx, is.ID, added, removed); err != nil {
			return nil, err
		}
	}
	state, err := s.inStep(ctx, is.ID, as, labels)
	if err != nil {
		return nil, err
	}
	lines := make([]string, len(as))
	for i, a := range as {
		lines[i] = r.Name + ": " + Describe(a, results[i])
		if i == len(as)-1 {
			lines[i] += state
		}
	}
	return lines, nil
}

// delta is what turns from into to.
func delta(from, to []string) (added, removed []string) {
	for _, l := range to {
		if !slices.Contains(from, l) {
			added = append(added, l)
		}
	}
	for _, l := range from {
		if !slices.Contains(to, l) {
			removed = append(removed, l)
		}
	}
	return added, removed
}

// inStep keeps an issue's state with its status when the assignments
// set one and [fields.status] has options: the last option closes it,
// another reopens it. It answers what it did: ", closed", ", reopened"
// or "".
func (s *Service) inStep(ctx context.Context, id string, as []Assignment, labels []string) (string, error) {
	if !slices.ContainsFunc(as, func(a Assignment) bool { return a.Field == "status" && !a.Clears() && a.Op != '-' }) {
		return "", nil
	}
	options := s.Schema.Fields["status"].Options
	status := ""
	for _, l := range labels {
		if v, ok := strings.CutPrefix(l, "status:"); ok {
			status = v
		}
	}
	if !slices.Contains(options, status) {
		return "", nil
	}
	want := Open
	if status == options[len(options)-1] {
		want = Closed
	}
	is, err := s.current(ctx, id)
	if err != nil || Status(is.Status) == want {
		return "", err
	}
	if err := s.SetStatus(ctx, id, want); err != nil {
		return "", err
	}
	return map[Status]string{Open: ", reopened", Closed: ", closed"}[want], nil
}

// LabelCount is a label in use, with how many open and closed issues
// have it.
type LabelCount struct {
	Label  string `json:"label"`
	Field  string `json:"field,omitempty"` // status for status:ready; "" for a plain label
	Value  string `json:"value"`
	Open   int    `json:"open"`
	Closed int    `json:"closed"`
}

// Labels are the labels in use: the plain ones first, then each field's
// values, by field. A deleted issue doesn't count.
func (s *Service) Labels(ctx context.Context) ([]LabelCount, error) {
	quiet := *s // counting tells nothing aside: the list it serves does
	quiet.Notice = nil
	all, err := quiet.List(ctx, ListReq{})
	if err != nil {
		return nil, err
	}
	by := map[string]*LabelCount{}
	for _, is := range all {
		for _, l := range is.Labels {
			c := by[l]
			if c == nil {
				c = &LabelCount{Label: l, Value: l}
				if f, v, ok := strings.Cut(l, ":"); ok {
					c.Field, c.Value = f, v
				}
				by[l] = c
			}
			if is.Status == string(Closed) {
				c.Closed++
			} else {
				c.Open++
			}
		}
	}
	out := make([]LabelCount, 0, len(by))
	for _, c := range by {
		out = append(out, *c)
	}
	slices.SortFunc(out, func(a, b LabelCount) int {
		if c := strings.Compare(a.Field, b.Field); c != 0 {
			return c
		}
		return strings.Compare(a.Value, b.Value)
	})
	return out, nil
}

// Step is the status option after current, or before it (back), in
// [fields.status]' order: no status yet goes to the first; no wrapping
// round.
func (s *Schema) Step(current string, back bool) (string, error) {
	options := s.Fields["status"].Options
	if len(options) == 0 {
		return "", fmt.Errorf("no [fields.status] options configured in .git-board.toml")
	}
	i := slices.Index(options, current)
	switch {
	case !back && current == "":
		return options[0], nil
	case back && current == "":
		return "", fmt.Errorf("no status set: nothing to go back from")
	case i < 0:
		return "", fmt.Errorf("current status %q doesn't match any configured [fields.status] option", current)
	case !back && i+1 >= len(options):
		return "", fmt.Errorf("status %q is already the last option (%s) — nothing to advance to", current, options[i])
	case back && i == 0:
		return "", fmt.Errorf("status %q is already the first option: nothing to go back to", current)
	case back:
		return options[i-1], nil
	}
	return options[i+1], nil
}

// closedAs is the label close --reason writes.
const closedAs = "closed-as"

// CloseReq asks close to close an issue.
type CloseReq struct {
	Name        string
	Comment     string // a comment first, "" for none
	Reason      string // completed, not-planned or duplicate
	DuplicateOf string // the issue it duplicates, a name
}

// Close closes an issue: a comment first, its status the terminal option,
// why (closed-as, duplicate-of, and a "Duplicate of" comment), then its
// state. It answers what close prints.
func (s *Service) Close(ctx context.Context, r CloseReq) (string, error) {
	ctx, done, werr := s.writing(ctx)
	if werr != nil {
		return "", werr
	}
	defer done()
	field := s.Schema.Fields["status"]
	if len(field.Options) == 0 {
		return "", fmt.Errorf("no [fields.status] options configured in .git-board.toml")
	}
	terminal := field.Options[len(field.Options)-1]
	is, err := s.current(ctx, r.Name)
	if err != nil {
		return "", err
	}
	var added, removed []string
	for _, l := range is.Labels {
		if strings.HasPrefix(l, closedAs+":") {
			removed = append(removed, l)
		}
	}
	if r.Reason != "completed" {
		added = append(added, closedAs+":"+r.Reason)
	}
	note := ""
	if r.DuplicateOf != "" {
		other, err := s.current(ctx, r.DuplicateOf)
		if err != nil {
			return "", fmt.Errorf("--duplicate-of %s: %w", r.DuplicateOf, err)
		}
		if other.ID == is.ID {
			return "", usage{fmt.Errorf("--duplicate-of %s: that is the issue itself", r.DuplicateOf)}
		}
		ref := refFrom(is, other)
		added = append(added, "duplicate-of:"+ref)
		note = "Duplicate of " + ref
	}
	added = slices.DeleteFunc(added, func(l string) bool { return slices.Contains(is.Labels, l) })
	removed = slices.DeleteFunc(removed, func(l string) bool { return slices.Contains(added, l) || l == closedAs+":"+r.Reason })
	for _, c := range []string{r.Comment, note} {
		if c != "" {
			if _, _, err := s.Comment(ctx, is.ID, c); err != nil {
				return "", err
			}
		}
	}
	label, err := s.apply(ctx, is.ID, is.Labels, terminal)
	if err != nil {
		return "", err
	}
	if len(added)+len(removed) > 0 {
		if _, err := s.ChangeLabels(ctx, is.ID, added, removed); err != nil {
			return "", err
		}
	}
	if err := s.SetStatus(ctx, is.ID, Closed); err != nil {
		return "", err
	}
	said := label
	for _, l := range added {
		said += ", " + l
	}
	return r.Name + ": " + said + ", closed", nil
}

// apply sets an issue's status, its labels now labels, in a change of
// its own (v1's fields.Apply), and answers the label it set.
func (s *Service) apply(ctx context.Context, id string, labels []string, status string) (string, error) {
	r, err := s.Schema.Change(labels, Assignment{Field: "status", Value: status})
	if err != nil {
		return "", err
	}
	if _, err := s.ChangeLabels(ctx, id, r.Added, r.Removed); err != nil {
		return "", err
	}
	return r.Label, nil
}

// refFrom is how is names other: other's number on a forge both are on
// (#12), else its id's first 8 characters.
func refFrom(is, other Issue) string {
	for _, f := range other.Forges {
		if f.Number != "" && slices.ContainsFunc(is.Forges, func(g Forge) bool { return g.Name == f.Name }) {
			return "#" + f.Number
		}
	}
	return other.ID[:min(8, len(other.ID))]
}

// Reopen opens an issue: a comment first; its status the first option
// when it was the terminal one or none; its closed-as goes (a
// duplicate-of stays). It answers what reopen prints.
func (s *Service) Reopen(ctx context.Context, name, comment string) (string, error) {
	ctx, done, werr := s.writing(ctx)
	if werr != nil {
		return "", werr
	}
	defer done()
	is, err := s.current(ctx, name)
	if err != nil {
		return "", err
	}
	if comment != "" {
		if _, _, err := s.Comment(ctx, is.ID, comment); err != nil {
			return "", err
		}
	}
	said := ""
	if options := s.Schema.Fields["status"].Options; len(options) > 0 {
		status := ""
		for _, l := range is.Labels {
			if v, ok := strings.CutPrefix(l, "status:"); ok {
				status = v
			}
		}
		if status == "" || status == options[len(options)-1] {
			label, err := s.apply(ctx, is.ID, is.Labels, options[0])
			if err != nil {
				return "", err
			}
			said = label + ", "
		}
	}
	if err := s.SetStatus(ctx, is.ID, Open); err != nil {
		return "", err
	}
	var reasons []string
	for _, l := range is.Labels {
		if strings.HasPrefix(l, closedAs+":") {
			reasons = append(reasons, l)
		}
	}
	if len(reasons) > 0 {
		if _, err := s.ChangeLabels(ctx, is.ID, nil, reasons); err != nil {
			return "", err
		}
	}
	return name + ": " + said + "open", nil
}

// Writing brackets a write: a Markdown folder's hand edits are read in
// before it, its files rendered after (next/domain/issues/mdfiles). It
// answers the context the write goes on, and what ends the write. A
// write within one (Reopen's comment) is within it.
type Writing func(ctx context.Context) (context.Context, func(), error)

// writing begins a write: ctx made a snapshot, within s.Writing if set.
func (s *Service) writing(ctx context.Context) (context.Context, func(), error) {
	ctx = s.Board.Snapshot(ctx)
	if s.Writing == nil {
		return ctx, func() {}, nil
	}
	return s.Writing(ctx)
}
