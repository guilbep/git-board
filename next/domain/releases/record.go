package releases

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sync"
	"time"

	"github.com/guilbep/git-board/next/domain/people"
	"github.com/guilbep/git-board/next/kernel"
)

// A release's page, kept as an entity of the release kind, in
// refs/board/release/<id>: one record per released commit, frozen at
// creation but for its notes, comments and yank. Its names are the tags
// on its commit, read from git, never stored (data-model.md, Releases).

// Artifact is an image a release shipped, by digest.
type Artifact struct {
	Image    string `json:"image"`
	Registry string `json:"registry"`
	Digest   string `json:"digest"`
}

// ChartArtifact is the Helm chart a release shipped.
type ChartArtifact struct {
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

// Comment is a comment on a release.
type Comment struct {
	Author  string    `json:"author"`
	Message string    `json:"message"`
	Date    time.Time `json:"date"`
}

// Asset is an archive published with a release: its name and sha256, as
// checksums.txt has them.
type Asset struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

// Record is a release's page, as surfaces show it: its people by name.
type Record struct {
	ID        string        `json:"id"`
	Commit    string        `json:"commit"`
	Previous  string        `json:"previous"` // the previous final's tag
	Artifacts []Artifact    `json:"artifacts"`
	Chart     ChartArtifact `json:"chart"`
	Assets    []Asset       `json:"assets"` // the archives published with its tag
	Notes     string        `json:"notes"`
	Comments  []Comment     `json:"comments"`
	Yanked    string        `json:"yanked"`  // the reason, "" when not
	Author    string        `json:"author"`  // the creating person's name
	Created   time.Time     `json:"created"` // as its author's clock had it
}

// State is a release's state, and its row: few, small, read whole from
// the datastore (Records), not through the view. Its
// people are ids, named when read (Record).
type State struct {
	ID        kernel.ID     `json:"id"`
	Commit    string        `json:"commit"`
	Previous  string        `json:"previous,omitempty"`
	Artifacts []Artifact    `json:"artifacts,omitempty"`
	Chart     ChartArtifact `json:"chart,omitzero"`
	Assets    []Asset       `json:"assets,omitempty"`
	Notes     string        `json:"notes,omitempty"`
	Comments  []Said        `json:"comments,omitempty"`
	Yanked    string        `json:"yanked,omitempty"`
	Author    kernel.ID     `json:"author,omitempty"`
	Created   int64         `json:"created"`
	made      bool
}

// Said is a comment as the state keeps it.
type Said struct {
	Author  kernel.ID `json:"author"`
	Message string    `json:"message"`
	Time    int64     `json:"time"`
}

// Data is the union of the record operations' fields: an operation's
// payload, as the spec's data model writes it.
type Data struct {
	Commit    string        `json:"commit,omitempty"`
	Previous  string        `json:"previous,omitempty"`
	Artifacts []Artifact    `json:"artifacts,omitempty"`
	Chart     ChartArtifact `json:"chart,omitzero"`
	Assets    []Asset       `json:"assets,omitempty"`
	Notes     string        `json:"notes,omitempty"`
	Message   string        `json:"message,omitempty"`
	Reason    string        `json:"reason,omitempty"`
}

// Kind is the release kind.
var Kind = kernel.Define(kernel.Spec[State, State]{
	Name:  "release",
	Rev:   1,
	New:   func(id kernel.ID) State { return State{ID: id} },
	Apply: apply,
	Index: func(s *State) kernel.Doc[State] {
		return kernel.Doc[State]{Row: *s, Terms: []string{"commit:" + s.Commit}, Created: s.Created, Updated: s.Created}
	},
})

// ErrBad is an operation no release folds.
var ErrBad = errors.New("releases: bad operation")

// apply folds one operation (data-model.md, Releases). An operation it
// doesn't know is an error, as for every kind.
func apply(s *State, e kernel.Entry) error {
	var d Data
	if len(e.Data) > 0 {
		if err := json.Unmarshal(e.Data, &d); err != nil {
			return fmt.Errorf("release %s, operation %s: %w", s.ID, e.ID, err)
		}
	}
	if !s.made != (e.Kind == "create") {
		return fmt.Errorf("%w: release %s: operation %s is %q, and only the first is a create", ErrBad, s.ID, e.ID, e.Kind)
	}
	switch e.Kind {
	case "create":
		if d.Commit == "" {
			return fmt.Errorf("%w: release %s: a record needs its commit", ErrBad, s.ID)
		}
		s.made = true
		s.Commit, s.Previous, s.Artifacts, s.Chart, s.Assets = d.Commit, d.Previous, d.Artifacts, d.Chart, d.Assets
		s.Author, s.Created = kernel.ID(e.Author), e.Time
	case "set-notes":
		s.Notes = d.Notes
	case "comment":
		if d.Message == "" {
			return fmt.Errorf("%w: release %s, operation %s: an empty comment", ErrBad, s.ID, e.ID)
		}
		s.Comments = append(s.Comments, Said{Author: kernel.ID(e.Author), Message: d.Message, Time: e.Time})
	case "yank":
		if d.Reason == "" {
			return fmt.Errorf("%w: release %s, operation %s: a yank needs its reason", ErrBad, s.ID, e.ID)
		}
		s.Yanked = d.Reason
	default:
		return fmt.Errorf("%w: release %s: unknown operation %q, operation %s", ErrBad, s.ID, e.Kind, e.ID)
	}
	return nil
}

// Service reads and writes the release records.
type Service struct {
	Board *kernel.Board
	Me    kernel.ID // who writes; "" refuses every write (people.ErrNoMe)
}

// Records is the records, one per commit, newest first. A second record
// for a commit, made on another clone before the two met, never shows: the
// one whose id sorts first is the commit's.
func (s *Service) Records(ctx context.Context) ([]Record, error) {
	type folded struct {
		id  kernel.ID
		st  State
		err error
	}
	var mu sync.Mutex
	var all []folded
	err := Kind.Logs(ctx, s.Board, func(id kernel.ID, log []kernel.Entry, err error) {
		var st State
		if err == nil {
			st, err = Kind.Fold(id, log)
		}
		mu.Lock()
		all = append(all, folded{id, st, err})
		mu.Unlock()
	})
	if err != nil {
		return nil, fmt.Errorf("reading the releases: %w", err)
	}
	slices.SortFunc(all, func(x, y folded) int { return cmp.Compare(x.id, y.id) })
	for _, f := range all {
		if f.err != nil {
			return nil, fmt.Errorf("reading the releases: %w", f.err)
		}
	}
	dir, err := people.Read(ctx, s.Board, people.None)
	if err != nil {
		return nil, fmt.Errorf("reading the releases: %w", err)
	}
	byCommit := map[string]Record{}
	for _, f := range all { // sorted: the first of a commit counts
		if _, ok := byCommit[f.st.Commit]; !ok {
			byCommit[f.st.Commit] = record(f.st, dir)
		}
	}
	return slices.SortedFunc(maps.Values(byCommit), func(x, y Record) int {
		return cmp.Or(y.Created.Compare(x.Created), cmp.Compare(x.ID, y.ID))
	}), nil
}

// record is a state as surfaces show it.
func record(s State, dir people.Directory) Record {
	r := Record{ID: string(s.ID), Commit: s.Commit, Previous: s.Previous, Artifacts: s.Artifacts, Chart: s.Chart,
		Assets: s.Assets, Notes: s.Notes, Yanked: s.Yanked, Created: time.Unix(s.Created, 0)}
	if s.Author != "" {
		r.Author = dir.Who(s.Author).Name
	}
	for _, c := range s.Comments {
		r.Comments = append(r.Comments, Comment{Author: dir.Who(c.Author).Name, Message: c.Message, Date: time.Unix(c.Time, 0)})
	}
	return r
}

// For is commit's record; ok is false while it has none.
func (s *Service) For(ctx context.Context, commit string) (r Record, ok bool, err error) {
	rs, err := s.Records(ctx)
	if i := slices.IndexFunc(rs, func(x Record) bool { return x.Commit == commit }); i >= 0 {
		return rs[i], true, nil
	}
	return Record{}, false, err
}

// Create records r's commit, previous, artifacts, chart and assets, by
// Me; the rest is the other operations'. A commit that has a record
// already keeps it, and it is returned.
func (s *Service) Create(ctx context.Context, r Record) (Record, error) {
	if r.Commit == "" {
		return Record{}, errors.New("a release record needs its commit")
	}
	if got, ok, err := s.For(ctx, r.Commit); ok || err != nil {
		return got, err
	}
	if s.Me == "" {
		return Record{}, people.ErrNoMe
	}
	if _, err := Kind.Create(ctx, s.Board, "", s.op("create", Data{Commit: r.Commit, Previous: r.Previous, Artifacts: r.Artifacts, Chart: r.Chart, Assets: r.Assets})); err != nil {
		return Record{}, fmt.Errorf("recording the release of %s: %w", r.Commit, err)
	}
	got, _, err := s.For(ctx, r.Commit)
	return got, err
}

// SetNotes replaces the notes of the record with this id.
func (s *Service) SetNotes(ctx context.Context, id, notes string) error {
	return s.edit(ctx, id, s.op("set-notes", Data{Notes: notes}))
}

// Comment comments on the record with this id.
func (s *Service) Comment(ctx context.Context, id, message string) error {
	if message == "" {
		return errors.New("an empty comment")
	}
	return s.edit(ctx, id, s.op("comment", Data{Message: message}))
}

// Yank marks the record with this id yanked, for reason.
func (s *Service) Yank(ctx context.Context, id, reason string) error {
	if reason == "" {
		return errors.New("a yank needs its reason")
	}
	return s.edit(ctx, id, s.op("yank", Data{Reason: reason}))
}

func (s *Service) op(kind string, d Data) kernel.Op {
	raw, _ := json.Marshal(d)
	return kernel.Op{Kind: kind, Author: string(s.Me), Time: time.Now().Unix(), Data: raw}
}

// edit appends op to the record with this id, as Me.
func (s *Service) edit(ctx context.Context, id string, op kernel.Op) error {
	if s.Me == "" {
		return people.ErrNoMe
	}
	_, err := Kind.Do(ctx, s.Board, kernel.ID(id), func(kernel.Entity[State]) ([]kernel.Op, error) { return []kernel.Op{op}, nil })
	if err != nil {
		return fmt.Errorf("editing release %s: %w", id, err)
	}
	return nil
}
