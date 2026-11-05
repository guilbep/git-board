package mdfiles

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/people"
	"github.com/guilbep/git-board/next/kernel"
)

// What a file says becomes operations (v1's mdimport.go):
//
//   - A file whose id is an issue here: what differs from the issue becomes
//     its operations, as who writes: the title, the description, the state,
//     the fields, labels and assignees, a comment's text, and new comments.
//   - When the issue changed since the file's updated, a field that differs
//     is a conflict: the board's value stays, and it is told. New comments
//     go in either way.
//   - A file with no id, or one this board doesn't have, is a new issue,
//     its author, comments and times as the file says; its new id is
//     written back into the file, so that a second import finds it.
//   - A file of a deleted issue stays deleted.

// importer reads files into the board, as one write's prelude.
type importer struct {
	f         *Folder
	names     map[string]kernel.ID // people by id, name and login; read at the first need
	conflicts []string             // "file: field", the board's value kept
}

// op is an operation by who, now; its data as v1 writes it, keys sorted.
func op(kind string, who kernel.ID, data map[string]any) kernel.Op {
	raw, _ := json.Marshal(data)
	return kernel.Op{Kind: kind, Author: string(who), Time: time.Now().Unix(), Data: raw}
}

// at is op at t, unless t is zero.
func at(o kernel.Op, t time.Time) kernel.Op {
	if !t.IsZero() {
		o.Time = t.Unix()
	}
	return o
}

// read imports one file, named name. It answers the id of the issue it
// names, now, and its file again when it made one (the id written in).
func (im *importer) read(ctx context.Context, name string, f file) (id kernel.ID, rewrite []byte, err error) {
	b := im.f.Issues.Board
	id = kernel.ID(f.front.ID)
	if f.front.ID != "" && isID(f.front.ID) {
		e, gerr := issues.Kind.Get(ctx, b, id)
		if gerr == nil && e.State.Deleted { // its file came back, from a branch: it stays deleted
			return id, nil, nil
		}
		if gerr == nil {
			conflicts, err := im.into(ctx, f)
			for _, c := range conflicts {
				im.conflicts = append(im.conflicts, name+": "+c)
			}
			return id, nil, err
		}
	}
	if id, err = im.create(ctx, f); err != nil {
		return "", nil, err
	}
	ths, err := im.f.Issues.Threads(ctx, []kernel.ID{id})
	if err != nil {
		return id, nil, err
	}
	data, err := Render(ths[0])
	return id, data, err
}

// people are the board's people by name and login, and by id: the
// lowest id keeps a name.
func (im *importer) people(ctx context.Context) (map[string]kernel.ID, error) {
	if im.names != nil {
		return im.names, nil
	}
	ls, err := people.Kind.List(ctx, im.f.Issues.Board, kernel.Query{})
	if err != nil {
		return nil, err
	}
	slices.SortFunc(ls, func(x, y kernel.Listed[people.Person]) int { return strings.Compare(string(x.ID), string(y.ID)) })
	out := map[string]kernel.ID{}
	put := func(k string, id kernel.ID) {
		if _, taken := out[k]; k != "" && !taken {
			out[k] = id
		}
	}
	for _, l := range ls {
		put(string(l.ID), l.ID)
		put(l.Row.Name, l.ID)
		for _, a := range l.Row.Accounts {
			put(a.Login, l.ID)
		}
	}
	im.names = out
	return out, nil
}

// person is who a file names, made when there is none; "" is who writes.
func (im *importer) person(ctx context.Context, who string) (kernel.ID, error) {
	if who == "" {
		if im.f.Issues.Me == "" {
			return "", people.ErrNoMe
		}
		return im.f.Issues.Me, nil
	}
	names, err := im.people(ctx)
	if err != nil {
		return "", err
	}
	if id, ok := names[who]; ok {
		return id, nil
	}
	e, err := people.Kind.Create(ctx, im.f.Issues.Board, "", op("create", "", map[string]any{"name": who, "email": ""}))
	if err != nil {
		return "", err
	}
	names[who] = e.ID
	return e.ID, nil
}

// create makes a new issue of a file, as it says: its author, its times,
// its comments, in one pack.
func (im *importer) create(ctx context.Context, f file) (kernel.ID, error) {
	author, err := im.person(ctx, f.front.People.Author)
	if err != nil {
		return "", err
	}
	ops := []kernel.Op{at(op("create", author, map[string]any{"title": f.front.Title, "body": f.body}), f.front.Created)}
	if f.front.State == "closed" {
		ops = append(ops, at(op("set-status", author, map[string]any{"status": issues.Closed}), f.front.Updated))
	}
	if add, _ := im.f.Issues.Stored(f.labels(), nil, nil); len(add) > 0 {
		ops = append(ops, at(op("change-labels", author, map[string]any{"add": add, "remove": nil}), f.front.Created))
	}
	for _, c := range f.comments {
		who, err := im.person(ctx, c.who)
		if err != nil {
			return "", err
		}
		ops = append(ops, at(op("comment", who, map[string]any{"body": c.body}), c.at))
	}
	e, err := issues.Kind.Create(ctx, im.f.Issues.Board, "", ops...)
	if err != nil {
		return "", err
	}
	return e.ID, nil
}

// into applies a file's changes to its issue. A field the file changes,
// on an issue that changed since the file's updated, is a conflict: left
// as the board has it.
func (im *importer) into(ctx context.Context, f file) (conflicts []string, err error) {
	svc, b, id := im.f.Issues, im.f.Issues.Board, kernel.ID(f.front.ID)
	me := svc.Me
	if me == "" {
		return nil, people.ErrNoMe
	}
	cur, err := svc.Get(ctx, string(id))
	if err != nil {
		return nil, err
	}
	moved := cur.UpdatedAt.UTC().Truncate(time.Second).After(f.front.Updated.UTC())
	may := func(field string) bool {
		if moved {
			conflicts = append(conflicts, field+": changed here since the file's updated, the board's kept")
		}
		return !moved
	}
	_, err = issues.Kind.Do(ctx, b, id, func(e kernel.Entity[issues.State]) ([]kernel.Op, error) {
		is := &e.State
		conflicts = conflicts[:0]
		var ops []kernel.Op
		if f.front.Title != is.Title && may("title") {
			ops = append(ops, op("set-title", me, map[string]any{"title": f.front.Title}))
		}
		if f.body != strings.TrimSpace(is.Body) && may("description") {
			ops = append(ops, op("edit-text", me, map[string]any{"target": string(is.BodyOp), "body": f.body}))
		}
		if (f.front.State == "open" || f.front.State == "closed") && f.front.State != string(is.Status) && may("state") {
			ops = append(ops, op("set-status", me, map[string]any{"status": f.front.State}))
		}
		for _, c := range f.comments {
			if c.id == "" {
				who, err := im.person(ctx, c.who)
				if err != nil {
					return nil, err
				}
				ops = append(ops, at(op("comment", who, map[string]any{"body": c.body}), c.at))
				continue
			}
			i := slices.IndexFunc(is.Comments, func(x issues.Comment) bool { return string(x.ID) == c.id })
			if i >= 0 && strings.TrimSpace(is.Comments[i].Body) != c.body && may("comment "+c.id) {
				ops = append(ops, op("edit-text", me, map[string]any{"target": c.id, "body": c.body}))
			}
		}
		return ops, nil
	})
	if err != nil {
		return conflicts, err
	}
	want, have := f.labels(), slices.Sorted(slices.Values(cur.Labels))
	var add, remove []string
	for _, l := range want {
		if !slices.Contains(have, l) {
			add = append(add, l)
		}
	}
	for _, l := range have {
		if !slices.Contains(want, l) {
			remove = append(remove, l)
		}
	}
	if (len(add) > 0 || len(remove) > 0) && may("labels") {
		_, err = issues.Kind.Do(ctx, b, id, func(e kernel.Entity[issues.State]) ([]kernel.Op, error) {
			raw := e.State.Labels
			a, r := svc.Stored(add, remove, raw)
			a = slices.DeleteFunc(slices.Clone(a), func(l string) bool { return slices.Contains(raw, l) })
			r = slices.DeleteFunc(slices.Clone(r), func(l string) bool { return !slices.Contains(raw, l) })
			if len(a) == 0 && len(r) == 0 {
				return nil, nil
			}
			return []kernel.Op{op("change-labels", me, map[string]any{"add": a, "remove": r})}, nil
		})
		if err != nil {
			return conflicts, fmt.Errorf("changing labels on %s: %w", id, err)
		}
	}
	return conflicts, nil
}

// isID says s is 40 lowercase hex digits.
func isID(s string) bool {
	if len(s) != 40 {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
