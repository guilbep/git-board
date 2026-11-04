package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
)

// ErrCantUndo: the change asked to undo has no inverse, or there is none.
var ErrCantUndo = errors.New("can't undo")

// ErrWroteAfter: someone else wrote to the issue after the change to undo.
var ErrWroteAfter = errors.New("someone else wrote after you")

// Undone is what an undo wrote: on which issue, and what it put back, a
// line each.
type Undone struct {
	Issue string   `json:"issue"`
	Title string   `json:"title"`
	Ops   []string `json:"undone"` // the ids of the operations undone
	What  []string `json:"what"`
}

// Undo writes the inverse of the last change the person you write as made
// to an issue, by id prefix; to any issue when idPrefix is "", the change
// with the latest time. A change is what one write wrote: one pack, as
// edit -e writes a title and a description at once. A change by you that
// came from a forge isn't yours to undo here. It is refused when someone
// else wrote to the issue after it, unless force. Undoing twice redoes.
func (b *Backend) Undo(idPrefix string, force bool) (Undone, error) {
	done, err := b.markdownWrite()
	if err != nil {
		return Undone{}, err
	}
	defer done()
	me, err := b.me()
	if err != nil {
		return Undone{}, err
	}
	id := ""
	if idPrefix != "" {
		if id, err = b.resolve("issue", idPrefix); err != nil {
			return Undone{}, err
		}
	} else if id, err = b.lastWritten(me); err != nil {
		return Undone{}, err
	}
	if b.isDeleted(id) {
		return Undone{}, fmt.Errorf("%w: issue %s is deleted, and a delete is never undone (docs/adr/0019)", ErrCantUndo, id[:8])
	}
	persons, err := b.allPersons()
	if err != nil {
		return Undone{}, err
	}
	name := func(id string) string {
		if p := persons[id]; p != nil {
			return p.Name
		}
		return id[:min(8, len(id))]
	}
	var res Undone
	is, err := b.write(id, func(_ string, is *issue.Issue, l *oplog.Log) ([]oplog.Op, error) {
		ops, u, err := inverse(me, l, force, name)
		res = u
		return ops, err
	})
	if err != nil {
		return Undone{}, err
	}
	res.Issue, res.Title = is.ID, is.Title
	return res, nil
}

// lastWritten is the issue of your latest change, deleted issues too: a
// delete is a change, and undo says it can't undo it rather than undo an
// older one elsewhere.
func (b *Backend) lastWritten(me string) (string, error) {
	issues, err := all(b, "issue", b.issues, func(id string) (*issue.Issue, error) {
		is, _, err := b.loadAny(id)
		return is, err
	})
	if err != nil {
		return "", err
	}
	best, at := "", int64(-1)
	for _, id := range slices.Sorted(maps.Keys(issues)) {
		_, l, err := b.loadAny(id)
		if err != nil {
			return "", err
		}
		if k := lastMine(l, me); k >= 0 && l.Entries[k].Time > at {
			best, at = id, l.Entries[k].Time
		}
	}
	if best == "" {
		return "", fmt.Errorf("%w: you haven't changed an issue yet", ErrCantUndo)
	}
	return best, nil
}

// lastMine is the index of the last operation me wrote here, not pulled
// from a forge; -1 for none.
func lastMine(l *oplog.Log, me string) int {
	for k := len(l.Entries) - 1; k >= 0; k-- {
		if e := l.Entries[k]; e.Author == me && e.Origin == nil {
			return k
		}
	}
	return -1
}

func pack(id string) string { p, _, _ := strings.Cut(id, ":"); return p }

// inverse is the operations that undo me's last change on the log: its
// pack's operations, each against the state before the pack.
func inverse(me string, l *oplog.Log, force bool, name func(string) string) ([]oplog.Op, Undone, error) {
	var u Undone
	last := lastMine(l, me)
	if last < 0 {
		return nil, u, fmt.Errorf("%w: you haven't changed this issue", ErrCantUndo)
	}
	p := pack(l.Entries[last].ID)
	first := last
	for first > 0 && pack(l.Entries[first-1].ID) == p {
		first--
	}
	change := l.Entries[first : last+1]
	if change[0].Kind == "create" {
		return nil, u, fmt.Errorf("%w: your last change here created the issue; git board delete %s deletes it", ErrCantUndo, l.ID[:8])
	}
	var others []string
	for _, e := range l.Entries[last+1:] {
		who := name(e.Author)
		if e.Origin != nil {
			who = "a pull from " + e.Origin.Forge
		}
		if !slices.Contains(others, who) {
			others = append(others, who)
		}
	}
	if len(others) > 0 && !force {
		return nil, u, fmt.Errorf("%w (%d change%s since, by %s): --force undoes yours anyway", ErrWroteAfter,
			len(l.Entries)-last-1, map[bool]string{true: "", false: "s"}[len(l.Entries)-last-1 == 1], strings.Join(others, ", "))
	}
	before, err := issue.Apply(&oplog.Log{Kind: l.Kind, ID: l.ID, Entries: l.Entries[:first]})
	if err != nil {
		return nil, u, err
	}
	now, err := issue.Apply(l)
	if err != nil {
		return nil, u, err
	}
	var ops []oplog.Op
	labelsAdd, labelsRemove := []string{}, []string{}
	for k := len(change) - 1; k >= 0; k-- {
		e := change[k]
		u.Ops = append(u.Ops, e.ID)
		var d struct {
			Title, Body, Target, Status string
			Add, Remove                 []string
		}
		_ = json.Unmarshal(e.Data, &d)
		switch e.Kind {
		case "delete":
			return nil, u, fmt.Errorf("%w: a delete is never undone (docs/adr/0019)", ErrCantUndo)
		case "set-meta":
			return nil, u, fmt.Errorf("%w: a change of metadata (a review thread resolved, a Project field) has no undo", ErrCantUndo)
		case "set-title":
			if now.Title != before.Title {
				ops = append(ops, issue.SetTitle(me, before.Title))
				u.What = append(u.What, fmt.Sprintf("title back to %q", before.Title))
			}
		case "set-status":
			if now.Status != before.Status {
				ops = append(ops, issue.SetStatus(me, before.Status))
				u.What = append(u.What, "status back to "+string(before.Status))
			}
		case "change-labels":
			// what it changed, against the labels before it
			for _, a := range d.Add {
				if !slices.Contains(before.Labels, a) && slices.Contains(now.Labels, a) && !slices.Contains(labelsRemove, a) {
					labelsRemove = append(labelsRemove, a)
				}
			}
			for _, r := range d.Remove {
				if slices.Contains(before.Labels, r) && !slices.Contains(now.Labels, r) && !slices.Contains(labelsAdd, r) {
					labelsAdd = append(labelsAdd, r)
				}
			}
		case "edit-text":
			was, ok := undoText(before, d.Target)
			cur, still := undoText(now, d.Target)
			if ok && still && cur != was {
				ops = append(ops, issue.EditText(me, d.Target, was))
				what := "the description"
				if d.Target != now.BodyOp {
					what = "comment " + shortRef(d.Target)
				}
				u.What = append(u.What, what+" back to its previous version")
			}
		case "comment":
			if slices.ContainsFunc(now.Comments, func(c issue.Comment) bool { return c.ID == e.ID }) {
				ops = append(ops, issue.Delete(me, e.ID))
				u.What = append(u.What, "comment "+shortRef(e.ID)+" deleted")
			}
		default:
			return nil, u, fmt.Errorf("%w: a %s operation has no undo", ErrCantUndo, e.Kind)
		}
	}
	if len(labelsAdd)+len(labelsRemove) > 0 {
		slices.Sort(labelsAdd)
		slices.Sort(labelsRemove)
		ops = append(ops, issue.ChangeLabels(me, labelsAdd, labelsRemove))
		var parts []string
		for _, a := range labelsAdd {
			parts = append(parts, "+"+a)
		}
		for _, r := range labelsRemove {
			parts = append(parts, "-"+r)
		}
		u.What = append(u.What, "labels "+strings.Join(parts, " "))
	}
	if len(ops) == 0 {
		return nil, u, fmt.Errorf("%w: your last change here is undone already", ErrCantUndo)
	}
	slices.Reverse(u.Ops)
	return ops, u, nil
}

// undoText is the text an edit-text's target names in is: its body, or a
// comment's; false when it isn't there (deleted).
func undoText(is *issue.Issue, target string) (string, bool) {
	if target == is.BodyOp {
		return is.Body, true
	}
	i := slices.IndexFunc(is.Comments, func(c issue.Comment) bool { return c.ID == target })
	if i < 0 {
		return "", false
	}
	return is.Comments[i].Body, true
}

// shortRef is a text's id as show prints it: abc1234:1.
func shortRef(id string) string {
	h, n, _ := strings.Cut(id, ":")
	return h[:min(7, len(h))] + ":" + n
}
