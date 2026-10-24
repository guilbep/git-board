package syncer

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
)

// Report counts what a pull did, by forge issue.
type Report struct{ Created, Updated, Unchanged int }

// ponytail: pulls current state only, without events: a field changed on
// both sides keeps git-board's value. The later-change-wins rule needs the
// forge's event times, M3.

// Pull copies a forge's issues into git-board, as sync.md's "Pull, the
// trial's part" says. If a write fails it returns the error; the issues
// already pulled stay pulled.
func Pull(ctx context.Context, s *gitstore.Store, f forge.Forge) (Report, error) {
	var r Report
	st, err := loadState(s, f.Key())
	if err != nil {
		return r, err
	}
	issues, err := f.Issues(ctx)
	if err != nil {
		return r, err
	}
	now := time.Now().Unix()
	for _, fi := range issues {
		if err := ctx.Err(); err != nil {
			return r, err
		}
		if fi.Status != "open" && fi.Status != "closed" {
			return r, fmt.Errorf("syncer: forge issue %s: bad status %q", fi.ID, fi.Status)
		}
		var wrote, created bool
		if id, ok := st.links[fi.ID]; ok {
			wrote, err = pullLinked(s, f.Key(), st, id, fi, now)
		} else {
			created = true
			err = pullNew(s, f.Key(), st, fi, now)
		}
		if err != nil {
			return r, err
		}
		switch {
		case created:
			r.Created++
		case wrote:
			r.Updated++
		default:
			r.Unchanged++
		}
	}
	return r, nil
}

// isAssignee says whether a label is an assignee label: the prefix
// assignee:, in any case, and a name.
func isAssignee(l string) bool {
	return len(l) > len("assignee:") && strings.EqualFold(l[:len("assignee:")], "assignee:")
}

// labelSet is the unique, sorted labels without assignee labels.
func labelSet(labels []string) []string {
	var out []string
	for _, l := range labels {
		if !isAssignee(l) {
			out = append(out, l)
		}
	}
	slices.Sort(out)
	return slices.Compact(out)
}

func split(v string) []string {
	if v == "" {
		return nil
	}
	return strings.Split(v, "\n")
}

// from stamps an operation with the forge's origin.
func from(op oplog.Op, key, event string, time int64) oplog.Op {
	op.Origin = &oplog.Origin{Forge: key, Event: event}
	op.Time = time
	return op
}

func pullNew(s *gitstore.Store, key string, st *state, fi forge.Issue, now int64) error {
	a := "forge:" + fi.Author.Login
	ev := "issue:" + fi.ID
	labels := strings.Join(labelSet(fi.Labels), "\n")
	ops := []oplog.Op{from(issue.NewCreate(a, fi.Title, fi.Body), key, ev, fi.Created)}
	if fi.Status == "closed" {
		ops = append(ops, from(issue.SetStatus(a, issue.Closed), key, ev, now))
	}
	if labels != "" {
		ops = append(ops, from(issue.ChangeLabels(a, split(labels), nil), key, ev, now))
	}
	for _, c := range fi.Comments {
		ops = append(ops, from(issue.NewComment("forge:"+c.Author.Login, c.Body), key, "comment:"+c.ID, c.Time))
	}
	is, err := issue.Create(s, ops...)
	if err != nil {
		return err
	}
	sops := []oplog.Op{
		linkOp(a, now, is.ID, fi.ID, fi.Number, fi.URL),
		seenOp(a, now, is.ID, "title", fi.Title),
		seenOp(a, now, is.ID, "body", fi.Body),
		seenOp(a, now, is.ID, "status", fi.Status),
		seenOp(a, now, is.ID, "labels", labels),
	}
	for _, c := range fi.Comments {
		sops = append(sops, seenOp(a, now, is.ID, "comment:"+c.ID, c.Body))
	}
	return st.write(s, key, sops)
}

func pullLinked(s *gitstore.Store, key string, st *state, id string, fi forge.Issue, now int64) (bool, error) {
	is, err := issue.Load(s, id)
	if err != nil {
		if isMissing(err) {
			return false, nil // the issue may arrive with a fetch
		}
		return false, err
	}
	a := "forge:" + fi.Author.Login
	ev := "issue:" + fi.ID
	seen := st.seen[id]
	var ops, sops []oplog.Op

	// text compares one field. write adds the issue's operation for F.
	text := func(field, f, g string, write func() oplog.Op) {
		if f == seen[field] {
			return
		}
		if g == seen[field] {
			ops = append(ops, write())
		} else if g != f {
			return
		}
		sops = append(sops, seenOp(a, now, id, field, f))
	}
	text("title", fi.Title, is.Title, func() oplog.Op { return from(issue.SetTitle(a, fi.Title), key, ev, now) })
	text("body", fi.Body, is.Body, func() oplog.Op { return from(issue.EditText(a, is.BodyOp, fi.Body), key, ev, now) })
	text("status", fi.Status, string(is.Status), func() oplog.Op {
		return from(issue.SetStatus(a, issue.Status(fi.Status)), key, ev, now)
	})

	fl := strings.Join(labelSet(fi.Labels), "\n")
	if fl != seen["labels"] {
		F, S, G := split(fl), split(seen["labels"]), labelSet(is.Labels)
		var add, remove []string
		for _, l := range F {
			if !slices.Contains(S, l) && !slices.Contains(G, l) {
				add = append(add, l)
			}
		}
		for _, l := range S {
			if !slices.Contains(F, l) && slices.Contains(G, l) {
				remove = append(remove, l)
			}
		}
		if len(add) > 0 || len(remove) > 0 {
			ops = append(ops, from(issue.ChangeLabels(a, add, remove), key, ev, now))
		}
		sops = append(sops, seenOp(a, now, id, "labels", fl))
	}

	for _, c := range fi.Comments {
		field, ev := "comment:"+c.ID, "comment:"+c.ID
		ca := "forge:" + c.Author.Login
		i := slices.IndexFunc(is.Comments, func(ic issue.Comment) bool {
			return ic.Origin != nil && ic.Origin.Forge == key && ic.Origin.Event == ev
		})
		if i < 0 {
			ops = append(ops, from(issue.NewComment(ca, c.Body), key, ev, c.Time))
			sops = append(sops, seenOp(a, now, id, field, c.Body))
			continue
		}
		text(field, c.Body, is.Comments[i].Body, func() oplog.Op {
			return from(issue.EditText(ca, is.Comments[i].ID, c.Body), key, ev, now)
		})
	}

	if len(ops) > 0 {
		if _, err := issue.Do(s, id, ops...); err != nil {
			return false, err
		}
	}
	if err := st.write(s, key, sops); err != nil {
		return false, err
	}
	return len(ops)+len(sops) > 0, nil
}

func isMissing(err error) bool { return errors.Is(err, oplog.ErrNotFound) }
