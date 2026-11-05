package issues

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/guilbep/git-board/domain/people"
	"github.com/guilbep/git-board/kernel"
	bsync "github.com/guilbep/git-board/sync"
)

// Show is an issue with its whole history, by name; not found once a
// delete hid it.
func (s *Service) Show(ctx context.Context, name string) (Thread, error) {
	th, _, err := s.read(ctx, name)
	return th, err
}

// ThreadsOf is Show for many issues of a list, each at the version listed:
// the listing, the people and the forges' records are read once for all,
// and the logs in one load, where Show reads them for each. each is
// called once per issue, i its place in list, from several goroutines at
// once.
func (s *Service) ThreadsOf(ctx context.Context, list []Issue, each func(i int, th Thread, err error)) error {
	ctx = s.Board.Snapshot(ctx)
	if err := s.Board.Fresh(ctx, Kind, people.Kind); err != nil {
		return err
	}
	dir, err := people.Read(ctx, s.Board, s.Avatars)
	if err != nil {
		return err
	}
	rd := s.reading(ctx, s.notice, true)
	heads := make([]kernel.Head, len(list))
	place := make(map[kernel.ID]int, len(list))
	for i, is := range list {
		heads[i] = kernel.Head{ID: kernel.ID(is.ID), Version: is.Version}
		place[kernel.ID(is.ID)] = i
	}
	return Kind.LogsOf(ctx, s.Board, heads, func(id kernel.ID, v kernel.Version, log []kernel.Entry, err error) {
		var th Thread
		if err == nil {
			th, err = s.threadOf(id, v, log, dir, rd)
		}
		each(place[id], th, err)
	})
}

// ThreadsMark is what every thread shows besides its own log, as
// versions: the people's (names, logins), the forges' records (where an
// issue is, what is pending) and the counter's (its number). A thread
// kept with its issue's version and this mark is the thread Show reads
// while both stay.
func (s *Service) ThreadsMark(ctx context.Context) (string, error) {
	heads, err := s.Board.Store().Heads(ctx, people.Kind.Name(), bsync.Kind.Name())
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for _, x := range heads {
		fmt.Fprintf(h, "%s %s %s\n", x.Kind, x.ID, x.Version)
	}
	if c := s.Board.Store().Caps().Counter; c != nil {
		_, v, err := c.Count(ctx)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "counter %s\n", v)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// threadOf is an issue's thread from its log, read at v.
func (s *Service) threadOf(id kernel.ID, v kernel.Version, log []kernel.Entry, dir people.Directory, rd reading) (Thread, error) {
	st, err := Kind.Fold(id, log)
	if err != nil {
		return Thread{}, err
	}
	if st.Deleted {
		return Thread{}, fmt.Errorf("issue %s: %w", id, ErrDeleted)
	}
	d := index(&st)
	th := s.thread(&st, &d.Row, log, dir, rd)
	th.Version = v
	return th, nil
}

// read is an issue's thread and its log.
func (s *Service) read(ctx context.Context, name string) (Thread, []kernel.Entry, error) {
	ctx = s.Board.Snapshot(ctx)
	if err := s.Board.Fresh(ctx, Kind, people.Kind); err != nil { // one listing for both
		return Thread{}, nil, err
	}
	id, err := s.Resolve(ctx, name)
	if err != nil {
		return Thread{}, nil, err
	}
	log, v, err := Kind.Log(ctx, s.Board, id)
	if err != nil {
		return Thread{}, nil, err
	}
	dir, err := people.Read(ctx, s.Board, s.Avatars)
	if err != nil {
		return Thread{}, nil, err
	}
	th, err := s.threadOf(id, v, log, dir, s.reading(ctx, s.notice, true))
	if err != nil {
		return Thread{}, nil, err
	}
	return th, log, nil
}

// thread replays an issue's log into its history, as the fold applies it.
// Marked pending for a forge is what hasn't reached it yet, when rd
// reads now.
func (s *Service) thread(st *State, r *Row, log []kernel.Entry, dir people.Directory, rd reading) Thread {
	th := Thread{Issue: s.issue(st.ID, r, dir, rd), Author: dir.Who(st.Author), EditedAt: time.Unix(st.Updated, 0)}
	th.Participants = append(th.Participants, th.Author)
	for _, c := range st.Comments {
		if !slices.ContainsFunc(th.Participants, func(p people.Who) bool { return p.ID == c.Author }) {
			th.Participants = append(th.Participants, dir.Who(c.Author))
		}
	}
	texts := map[kernel.OpID]int{}     // a text's operation id -> its event
	origins := map[kernel.Origin]int{} // a comment's origin -> its event
	from := map[int]*kernel.Origin{}   // an event -> where it was pulled from
	changed := map[int][2][]string{}   // a labels event -> the labels it added and removed
	labels := map[string]bool{}
	title := ""
	for _, e := range log {
		d, _ := decode(e.Data) // the fold read it already
		ev := Event{ID: string(e.ID), Author: dir.Who(kernel.ID(e.Author)), At: time.Unix(e.Time, 0)}
		switch e.Kind {
		case "create", "comment":
			if e.Origin != nil {
				if i, dup := origins[*e.Origin]; dup { // the same comment, pulled again
					texts[e.ID] = i
					continue
				}
				origins[*e.Origin] = len(th.Events)
				from[len(th.Events)] = e.Origin
			}
			ev.Kind, ev.Body, ev.Meta = "comment", d.Body, maps.Clone(d.Meta)
			if e.Kind == "create" {
				ev.Kind, title = "created", d.Title
			}
			ev.History = []Version{{ID: ev.ID, Author: ev.Author, At: ev.At, Body: d.Body}}
			texts[e.ID] = len(th.Events)
		case "edit-text":
			if i, ok := texts[d.Target]; ok {
				t := &th.Events[i]
				t.Body = d.Body
				t.History = append(t.History, Version{ID: ev.ID, Author: ev.Author, At: ev.At, Body: d.Body})
			}
			continue
		case "set-meta":
			if i, ok := texts[d.Target]; ok {
				t := &th.Events[i]
				if t.Meta == nil {
					t.Meta = map[string]string{}
				}
				maps.Copy(t.Meta, d.Meta)
				t.MetaHistory = append(t.MetaHistory, MetaSet{ID: ev.ID, Author: ev.Author, At: ev.At, Meta: maps.Clone(d.Meta)})
			}
			continue
		case "set-title":
			ev.Kind, ev.Title, ev.Was, title = "title", d.Title, title, d.Title
		case "set-status":
			ev.Kind, ev.Status = "status", string(d.Status)
		case "change-labels":
			ev.Added, ev.Removed = applyLabels(labels, d.Add, d.Remove)
			if len(ev.Added) == 0 && len(ev.Removed) == 0 {
				continue
			}
			changed[len(th.Events)] = [2][]string{ev.Added, ev.Removed}
			ev.Kind, ev.Added, ev.Removed = "labels", s.Schema.Shown(slices.Clone(ev.Added)), s.Schema.Shown(slices.Clone(ev.Removed))
		default:
			continue
		}
		th.Events = append(th.Events, ev)
	}
	for i := range th.Events {
		e := &th.Events[i]
		if e.Kind != "created" && e.Kind != "comment" {
			continue
		}
		e.Edited = len(e.History) > 1
		if !e.Edited {
			e.History = nil
		}
		e.Review = reviewOf(e.Meta)
		if len(e.Meta) == 0 {
			e.Meta = nil
		}
	}
	if rd.now {
		for _, r := range rd.recs[string(st.ID)] {
			s.pending(&th, st, r, from, changed)
		}
	}
	if len(st.Removed) > 0 { // deleted comments: no event, no text
		removed := map[string]bool{}
		for _, c := range st.Removed {
			removed[string(c.ID)] = true
		}
		th.Events = slices.DeleteFunc(th.Events, func(e Event) bool { return e.Kind == "comment" && removed[e.ID] })
	}
	return th
}

// applyLabels applies a change of labels to a set, and answers what it
// changed: added labels that weren't there, removed ones that were.
func applyLabels(labels map[string]bool, add, remove []string) (added, removed []string) {
	before := maps.Clone(labels)
	for _, a := range add {
		labels[a] = true
	}
	for _, r := range remove {
		delete(labels, r)
	}
	for _, a := range add {
		if labels[a] && !before[a] && !slices.Contains(added, a) {
			added = append(added, a)
		}
	}
	for _, r := range remove {
		if before[r] && !labels[r] && !slices.Contains(removed, r) {
			removed = append(removed, r)
		}
	}
	return added, removed
}

// reviewOf is a comment's review:* metadata, nil for a plain comment.
func reviewOf(meta map[string]string) *Review {
	r := Review{
		Path:     meta["review:path"],
		Side:     meta["review:side"],
		Commit:   meta["review:commit"],
		PR:       meta["review:pr"],
		ReplyTo:  meta["review:reply-to"],
		State:    meta["review:state"],
		Resolved: meta["review:resolved"] == "true",
	}
	r.Line, _ = strconv.Atoi(meta["review:line"])
	if r == (Review{}) {
		return nil
	}
	return &r
}

// LogEntry is one change, as log prints it.
type LogEntry struct {
	At      time.Time  `json:"at"`
	Author  people.Who `json:"author"`
	Kind    string     `json:"kind"` // created, comment, edit, title, state, labels
	ID      string     `json:"id"`
	Target  string     `json:"target,omitempty"`
	Title   string     `json:"title,omitempty"`
	Was     string     `json:"was,omitempty"`
	State   string     `json:"state,omitempty"`
	Added   []string   `json:"added,omitempty"`
	Removed []string   `json:"removed,omitempty"`
	Body    string     `json:"body,omitempty"`
}

// LogReq asks for an issue's changes.
type LogReq struct {
	Name  string    // the issue
	Field string    // only that field's label changes
	Since time.Time // only the changes from then
}

// Log is every change made to an issue, in the order its log applied
// them, and the thread they come from.
func (s *Service) Log(ctx context.Context, r LogReq) ([]LogEntry, Thread, error) {
	th, log, err := s.read(ctx, r.Name)
	if err != nil {
		return nil, Thread{}, err
	}
	order := make(map[string]int, len(log))
	for i, e := range log {
		order[string(e.ID)] = i
	}
	entries := historyOf(th, order)
	if r.Field != "" {
		entries = keepField(entries, r.Field)
	}
	entries = slices.DeleteFunc(entries, func(e LogEntry) bool { return e.At.Before(r.Since) })
	if entries == nil {
		entries = []LogEntry{}
	}
	return entries, th, nil
}

// historyOf is a thread's changes, in the log's order.
func historyOf(th Thread, order map[string]int) []LogEntry {
	var res []LogEntry
	for _, e := range th.Events {
		le := LogEntry{At: e.At, Author: e.Author, Kind: e.Kind, ID: e.ID}
		switch e.Kind {
		case "created":
			le.Title, le.Body = firstTitle(th), firstBody(e)
		case "comment":
			le.Body = firstBody(e)
		case "title":
			le.Title, le.Was = e.Title, e.Was
		case "status":
			le.Kind, le.State = "state", e.Status
		case "labels":
			le.Added, le.Removed = e.Added, e.Removed
		}
		res = append(res, le)
		for _, v := range slices.Clip(e.History[min(1, len(e.History)):]) {
			res = append(res, LogEntry{At: v.At, Author: v.Author, Kind: "edit", ID: v.ID, Target: e.ID, Body: v.Body})
		}
	}
	slices.SortStableFunc(res, func(x, y LogEntry) int {
		i, okX := order[x.ID]
		j, okY := order[y.ID]
		if okX && okY {
			return i - j
		}
		return x.At.Compare(y.At)
	})
	return res
}

// firstBody is a text as it was written, before any edit.
func firstBody(e Event) string {
	if len(e.History) > 0 {
		return e.History[0].Body
	}
	return e.Body
}

// firstTitle is the title the issue was created with.
func firstTitle(th Thread) string {
	for _, e := range th.Events {
		if e.Kind == "title" {
			return e.Was
		}
	}
	return th.Title
}

// keepField keeps the label changes of field, each with that field's
// labels only.
func keepField(entries []LogEntry, field string) []LogEntry {
	in := func(ls []string) []string {
		return slices.DeleteFunc(slices.Clone(ls), func(l string) bool { return !strings.HasPrefix(l, field+":") })
	}
	var res []LogEntry
	for _, e := range entries {
		if e.Kind != "labels" {
			continue
		}
		e.Added, e.Removed = in(e.Added), in(e.Removed)
		if len(e.Added)+len(e.Removed) > 0 {
			res = append(res, e)
		}
	}
	return res
}
