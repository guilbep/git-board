// Package release is the release domain (ADR 0020, step 3): a release
// record, an entity of the core's own kind; versions and the tags that
// name them; the [release] table that says where an app's code goes; and
// the GitOps overlay that says what runs there. It drives no tool and
// reaches no registry: git-board's internal/ship cuts and deploys with it.
package release

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/guilbep/git-board/core/oplog"
)

// A release's page, kept in git as an entity of the core's release kind,
// in refs/board/release/<id>: one record per released commit, frozen at
// creation but for its notes, comments and yank. Its names are the tags
// on its commit, read from git, never stored. The spec's data model has
// its operations.

// Kind is the record's entity kind: refs/board/release/<id>.
const Kind = "release"

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

// Record is a release's page: what it shipped and what was said about it.
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

func newOp(kind, author string, d Data) oplog.Op {
	raw, _ := json.Marshal(d)
	return oplog.Op{Kind: kind, Author: author, Time: time.Now().Unix(), Data: raw}
}

// Apply folds a record's operations; name is a person's name, by id. An
// operation it doesn't know is an error, as for every kind of the core.
func Apply(l *oplog.Log, name func(id string) string) (Record, error) {
	r := Record{ID: l.ID}
	for i, e := range l.Entries {
		var d Data
		if err := json.Unmarshal(e.Data, &d); err != nil {
			return r, fmt.Errorf("release %s, operation %s: %w", l.ID, e.ID, err)
		}
		if (i == 0) != (e.Kind == "create") {
			return r, fmt.Errorf("release %s: operation %s is %q, and only the first is a create", l.ID, e.ID, e.Kind)
		}
		switch e.Kind {
		case "create":
			if d.Commit == "" {
				return r, fmt.Errorf("release %s: a record needs its commit", l.ID)
			}
			r.Commit, r.Previous, r.Artifacts, r.Chart, r.Assets = d.Commit, d.Previous, d.Artifacts, d.Chart, d.Assets
			r.Author, r.Created = name(e.Author), time.Unix(e.Time, 0)
		case "set-notes":
			r.Notes = d.Notes
		case "comment":
			if d.Message == "" {
				return r, fmt.Errorf("release %s, operation %s: an empty comment", l.ID, e.ID)
			}
			r.Comments = append(r.Comments, Comment{Author: name(e.Author), Message: d.Message, Date: time.Unix(e.Time, 0)})
		case "yank":
			if d.Reason == "" {
				return r, fmt.Errorf("release %s, operation %s: a yank needs its reason", l.ID, e.ID)
			}
			r.Yanked = d.Reason
		default:
			return r, fmt.Errorf("release %s: unknown operation %q, operation %s", l.ID, e.Kind, e.ID)
		}
	}
	return r, nil
}

// Records is the records, one per commit, newest first. A second record
// for a commit, made on another clone before the two met, never shows: the
// one whose id sorts first is the commit's.
func Records(s oplog.Datastore, name func(id string) string) ([]Record, error) {
	ids, err := oplog.List(s, Kind)
	if err != nil {
		return nil, fmt.Errorf("reading the releases: %w", err)
	}
	byCommit := map[string]Record{}
	for _, id := range ids { // sorted: the first of a commit counts
		l, err := oplog.Load(s, Kind, id)
		if err != nil {
			return nil, fmt.Errorf("reading the releases: %w", err)
		}
		r, err := Apply(l, name)
		if err != nil {
			return nil, fmt.Errorf("reading the releases: %w", err)
		}
		if _, ok := byCommit[r.Commit]; !ok {
			byCommit[r.Commit] = r
		}
	}
	return slices.SortedFunc(maps.Values(byCommit), func(x, y Record) int {
		return cmp.Or(y.Created.Compare(x.Created), cmp.Compare(x.ID, y.ID))
	}), nil
}

// For is commit's record; ok is false while it has none.
// ponytail: reads every record, an index by commit if there get to be many
func For(s oplog.Datastore, commit string, name func(id string) string) (r Record, ok bool, err error) {
	rs, err := Records(s, name)
	if i := slices.IndexFunc(rs, func(x Record) bool { return x.Commit == commit }); i >= 0 {
		return rs[i], true, nil
	}
	return Record{}, false, err
}

// Create records r's commit, previous, artifacts, chart and assets, by
// author; the rest is the other operations'. A commit that has a record
// already keeps it, and it is returned.
func Create(ctx context.Context, s oplog.Datastore, author string, r Record, name func(id string) string) (Record, error) {
	if r.Commit == "" {
		return Record{}, errors.New("a release record needs its commit")
	}
	if got, ok, err := For(s, r.Commit, name); ok || err != nil {
		return got, err
	}
	op := newOp("create", author, Data{Commit: r.Commit, Previous: r.Previous, Artifacts: r.Artifacts, Chart: r.Chart, Assets: r.Assets})
	l, err := oplog.Create(ctx, s, Kind, "", []oplog.Op{op})
	if err != nil {
		return Record{}, fmt.Errorf("recording the release of %s: %w", r.Commit, err)
	}
	return Apply(l, name)
}

// SetNotes replaces the notes of the record with this id, by author.
func SetNotes(ctx context.Context, s oplog.Datastore, id, author, notes string) error {
	return edit(ctx, s, id, newOp("set-notes", author, Data{Notes: notes}))
}

// AddComment comments on the record with this id, by author.
func AddComment(ctx context.Context, s oplog.Datastore, id, author, message string) error {
	if message == "" {
		return errors.New("an empty comment")
	}
	return edit(ctx, s, id, newOp("comment", author, Data{Message: message}))
}

// Yank marks the record with this id yanked, for reason, by author.
func Yank(ctx context.Context, s oplog.Datastore, id, author, reason string) error {
	if reason == "" {
		return errors.New("a yank needs its reason")
	}
	return edit(ctx, s, id, newOp("yank", author, Data{Reason: reason}))
}

// edit appends op to the record with this id.
func edit(ctx context.Context, s oplog.Datastore, id string, op oplog.Op) error {
	l, err := oplog.Load(s, Kind, id)
	if err != nil {
		return fmt.Errorf("reading release %s: %w", id, err)
	}
	if err := l.Append(ctx, s, []oplog.Op{op}); err != nil {
		return fmt.Errorf("editing release %s: %w", id, err)
	}
	return nil
}
