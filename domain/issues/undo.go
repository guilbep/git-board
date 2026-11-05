package issues

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/guilbep/git-board/domain/people"
	"github.com/guilbep/git-board/kernel"
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

// Undo writes the inverse of the last change who writes made to an
// issue, by name; to any issue when name is "", the change with the
// latest time. A change is what one write wrote: one pack, as edit -e
// writes a title and a description at once. A change that came from a
// forge isn't theirs to undo here. It is refused when someone else wrote
// to the issue after it, unless force. Undoing twice redoes, but for a
// new comment: its undo is a delete, and a delete is final (ADR 0019),
// so the second undo refuses and says to post the text again.
func (s *Service) Undo(ctx context.Context, name string, force bool) (Undone, error) {
	ctx, done, werr := s.writing(ctx)
	if werr != nil {
		return Undone{}, werr
	}
	defer done()
	me, err := s.writer()
	if err != nil {
		return Undone{}, err
	}
	var id kernel.ID
	if name != "" {
		if id, err = s.Resolve(ctx, name); err != nil {
			return Undone{}, err
		}
	} else if id, err = s.lastWritten(ctx, me); err != nil {
		return Undone{}, err
	}
	dir, err := people.Read(ctx, s.Board, s.Avatars)
	if err != nil {
		return Undone{}, err
	}
	whom := func(id kernel.ID) string {
		if names := dir.Names(id); len(names) > 0 {
			return names[0]
		}
		return string(id[:min(8, len(id))])
	}
	var res Undone
	e, err := Kind.Do(ctx, s.Board, id, func(cur kernel.Entity[State]) ([]kernel.Op, error) {
		if cur.State.Deleted {
			return nil, fmt.Errorf("%w: issue %s is deleted, and a delete is never undone (docs/adr/0019)", ErrCantUndo, id[:8])
		}
		log, v, err := Kind.Log(ctx, s.Board, id)
		if err != nil {
			return nil, err
		}
		if v != cur.Version {
			return nil, fmt.Errorf("%w: issue %s moved while undo read it: run it again", ErrChanged, id[:8])
		}
		ops, u, err := inverse(me, id, log, force, whom)
		res = u
		return ops, err
	})
	if err != nil {
		return Undone{}, err
	}
	res.Issue, res.Title = string(e.ID), e.State.Title
	return res, nil
}

// lastWritten is the issue of me's latest change, deleted issues too: a
// delete is a change, and undo says it can't undo it rather than undo an
// older one elsewhere.
func (s *Service) lastWritten(ctx context.Context, me kernel.ID) (kernel.ID, error) {
	var mu sync.Mutex
	var best kernel.ID
	at := int64(-1)
	var first error
	err := Kind.Logs(ctx, s.Board, func(id kernel.ID, log []kernel.Entry, err error) {
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			if first == nil {
				first = err
			}
			return
		}
		if k := lastMine(log, me); k >= 0 {
			if t := log[k].Time; t > at || t == at && id < best { // ties: the first id, as v1 walks them
				best, at = id, t
			}
		}
	})
	if err == nil {
		err = first
	}
	if err != nil {
		return "", err
	}
	if best == "" {
		return "", fmt.Errorf("%w: you haven't changed an issue yet", ErrCantUndo)
	}
	return best, nil
}

// lastMine is the index of the last operation me wrote here, not pulled
// from a forge; -1 for none.
func lastMine(log []kernel.Entry, me kernel.ID) int {
	for k := len(log) - 1; k >= 0; k-- {
		if e := log[k]; kernel.ID(e.Author) == me && e.Origin == nil {
			return k
		}
	}
	return -1
}

// inverse is the operations that undo me's last change on the log: its
// pack's operations, each against the state before the pack.
func inverse(me, id kernel.ID, log []kernel.Entry, force bool, whom func(kernel.ID) string) ([]kernel.Op, Undone, error) {
	var u Undone
	last := lastMine(log, me)
	if last < 0 {
		return nil, u, fmt.Errorf("%w: you haven't changed this issue", ErrCantUndo)
	}
	p := log[last].ID.Pack()
	first := last
	for first > 0 && log[first-1].ID.Pack() == p {
		first--
	}
	change := log[first : last+1]
	if change[0].Kind == "create" {
		return nil, u, fmt.Errorf("%w: your last change here created the issue; git board delete %s deletes it", ErrCantUndo, id[:8])
	}
	var others []string
	for _, e := range log[last+1:] {
		who := whom(kernel.ID(e.Author))
		if e.Origin != nil {
			who = "a pull from " + e.Origin.Forge
		}
		if !slices.Contains(others, who) {
			others = append(others, who)
		}
	}
	if n := len(log) - last - 1; len(others) > 0 && !force {
		return nil, u, fmt.Errorf("%w (%d change%s since, by %s): --force undoes yours anyway", ErrWroteAfter,
			n, map[bool]string{true: "", false: "s"}[n == 1], strings.Join(others, ", "))
	}
	before, err := Kind.Fold(id, log[:first])
	if err != nil {
		return nil, u, err
	}
	now, err := Kind.Fold(id, log)
	if err != nil {
		return nil, u, err
	}
	var ops []kernel.Op
	labelsAdd, labelsRemove := []string{}, []string{}
	for k := len(change) - 1; k >= 0; k-- {
		e := change[k]
		u.Ops = append(u.Ops, string(e.ID))
		var d struct {
			Title, Body, Target, Status string
			Add, Remove                 []string
		}
		_ = json.Unmarshal(e.Data, &d)
		switch e.Kind {
		case "delete":
			// a tombstone is final (ADR 0019), also the one undoing a
			// comment wrote: it doesn't restore, nor post a copy that
			// a sync would post to the forge beside the first
			if t := kernel.OpID(d.Target); t != before.BodyOp {
				return nil, u, fmt.Errorf("%w: your last change here deleted comment %s, and a delete is never undone (docs/adr/0019): post it again with git board comment %s",
					ErrCantUndo, shortRef(d.Target), id[:8])
			}
			return nil, u, fmt.Errorf("%w: a delete is never undone (docs/adr/0019)", ErrCantUndo)
		case "set-meta":
			return nil, u, fmt.Errorf("%w: a change of metadata (a review thread resolved, a Project field) has no undo", ErrCantUndo)
		case "set-title":
			if now.Title != before.Title {
				ops = append(ops, op("set-title", me, map[string]any{"title": before.Title}))
				u.What = append(u.What, fmt.Sprintf("title back to %q", before.Title))
			}
		case "set-status":
			if now.Status != before.Status {
				ops = append(ops, op("set-status", me, map[string]any{"status": before.Status}))
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
			target := kernel.OpID(d.Target)
			was, ok := textOf(&before, target)
			cur, still := textOf(&now, target)
			if ok && still && cur != was {
				ops = append(ops, op("edit-text", me, map[string]any{"target": d.Target, "body": was}))
				what := "the description"
				if target != now.BodyOp {
					what = "comment " + shortRef(d.Target)
				}
				u.What = append(u.What, what+" back to its previous version")
			}
		case "comment":
			if slices.ContainsFunc(now.Comments, func(c Comment) bool { return c.ID == e.ID }) {
				ops = append(ops, op("delete", me, map[string]any{"target": string(e.ID)}))
				u.What = append(u.What, "comment "+shortRef(string(e.ID))+" deleted")
			}
		default:
			return nil, u, fmt.Errorf("%w: a %s operation has no undo", ErrCantUndo, e.Kind)
		}
	}
	if len(labelsAdd)+len(labelsRemove) > 0 {
		slices.Sort(labelsAdd)
		slices.Sort(labelsRemove)
		ops = append(ops, op("change-labels", me, map[string]any{"add": labelsAdd, "remove": labelsRemove}))
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

// textOf is the text an edit-text's target names in st: its body, or a
// comment's; false when it isn't there (deleted).
func textOf(st *State, target kernel.OpID) (string, bool) {
	if target == st.BodyOp {
		return st.Body, true
	}
	i := slices.IndexFunc(st.Comments, func(c Comment) bool { return c.ID == target })
	if i < 0 {
		return "", false
	}
	return st.Comments[i].Body, true
}

// shortRef is a text's id as show prints it: abc1234:1.
func shortRef(id string) string {
	h, n, _ := strings.Cut(id, ":")
	return h[:min(7, len(h))] + ":" + n
}
