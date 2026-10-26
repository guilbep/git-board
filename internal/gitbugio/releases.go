package gitbugio

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/people"
)

// A release's page, kept in git as an entity of the core's release kind,
// in refs/board/release/<id>: one record per released commit, frozen at
// creation but for its notes, comments and yank. Its names are the tags
// on its commit, read from git, never stored. The spec's data model has
// its operations.

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

// ReleaseComment is a comment on a release.
type ReleaseComment struct {
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

// ReleaseRecord is a release's page: what it shipped and what was said
// about it.
type ReleaseRecord struct {
	ID        string           `json:"id"`
	Commit    string           `json:"commit"`
	Previous  string           `json:"previous"` // the previous final's tag
	Artifacts []Artifact       `json:"artifacts"`
	Chart     ChartArtifact    `json:"chart"`
	Assets    []Asset          `json:"assets"` // the archives published with its tag
	Notes     string           `json:"notes"`
	Comments  []ReleaseComment `json:"comments"`
	Yanked    string           `json:"yanked"`  // the reason, "" when not
	Author    string           `json:"author"`  // the creating person's name
	Created   time.Time        `json:"created"` // as its author's clock had it
}

// releaseData is the union of the release operations' fields.
type releaseData struct {
	Commit    string        `json:"commit,omitempty"`
	Previous  string        `json:"previous,omitempty"`
	Artifacts []Artifact    `json:"artifacts,omitempty"`
	Chart     ChartArtifact `json:"chart,omitzero"`
	Assets    []Asset       `json:"assets,omitempty"`
	Notes     string        `json:"notes,omitempty"`
	Message   string        `json:"message,omitempty"`
	Reason    string        `json:"reason,omitempty"`
}

func releaseOp(kind, author string, d releaseData) oplog.Op {
	raw, _ := json.Marshal(d)
	return oplog.Op{Kind: kind, Author: author, Time: time.Now().Unix(), Data: raw}
}

// compileRelease folds a record's operations; name is a person's name.
func compileRelease(l *oplog.Log, name func(id string) string) (ReleaseRecord, error) {
	r := ReleaseRecord{ID: l.ID}
	for i, e := range l.Entries {
		var d releaseData
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
			r.Comments = append(r.Comments, ReleaseComment{Author: name(e.Author), Message: d.Message, Date: time.Unix(e.Time, 0)})
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

// Releases is the records, one per commit, newest first. A second record
// for a commit, made on another clone before the two met, never shows: the
// one whose id sorts first is the commit's.
func (b *Backend) Releases() ([]ReleaseRecord, error) {
	ids, err := oplog.List(b.s, "release")
	if err != nil {
		return nil, fmt.Errorf("reading the releases: %w", err)
	}
	persons, err := b.allPersons()
	if err != nil {
		return nil, err
	}
	name := func(id string) string { return personOf(id, persons[id]).Name }
	byCommit := map[string]ReleaseRecord{}
	for _, id := range ids { // sorted: the first of a commit counts
		l, err := oplog.Load(b.s, "release", id)
		if err != nil {
			return nil, fmt.Errorf("reading the releases: %w", err)
		}
		r, err := compileRelease(l, name)
		if err != nil {
			return nil, fmt.Errorf("reading the releases: %w", err)
		}
		if _, ok := byCommit[r.Commit]; !ok {
			byCommit[r.Commit] = r
		}
	}
	return slices.SortedFunc(maps.Values(byCommit), func(x, y ReleaseRecord) int {
		return cmp.Or(y.Created.Compare(x.Created), cmp.Compare(x.ID, y.ID))
	}), nil
}

// ReleaseFor is commit's record; ok is false while it has none.
// ponytail: reads every record, an index by commit if there get to be many
func (b *Backend) ReleaseFor(commit string) (r ReleaseRecord, ok bool, err error) {
	rs, err := b.Releases()
	if i := slices.IndexFunc(rs, func(x ReleaseRecord) bool { return x.Commit == commit }); i >= 0 {
		return rs[i], true, nil
	}
	return ReleaseRecord{}, false, err
}

// CreateRelease records r's commit, previous, artifacts, chart and assets,
// by you; the rest is the other operations'. A commit that has a record
// already keeps it, and it is returned.
func (b *Backend) CreateRelease(r ReleaseRecord) (ReleaseRecord, error) {
	if r.Commit == "" {
		return ReleaseRecord{}, errors.New("a release record needs its commit")
	}
	if got, ok, err := b.ReleaseFor(r.Commit); ok || err != nil {
		return got, err
	}
	me, err := b.me()
	if err != nil {
		return ReleaseRecord{}, err
	}
	op := releaseOp("create", me, releaseData{Commit: r.Commit, Previous: r.Previous, Artifacts: r.Artifacts, Chart: r.Chart, Assets: r.Assets})
	l, err := oplog.Create(ctx, b.s, "release", "", []oplog.Op{op})
	if err != nil {
		return ReleaseRecord{}, fmt.Errorf("recording the release of %s: %w", r.Commit, err)
	}
	return compileRelease(l, b.personName)
}

// personName is a person's name, by id.
func (b *Backend) personName(id string) string {
	p, _ := people.Load(b.s, id)
	return personOf(id, p).Name
}

// SetReleaseNotes replaces the notes of the record with this id.
func (b *Backend) SetReleaseNotes(id, notes string) error {
	return b.editRelease(id, "set-notes", releaseData{Notes: notes})
}

// CommentRelease comments on the record with this id.
func (b *Backend) CommentRelease(id, message string) error {
	if message == "" {
		return errors.New("an empty comment")
	}
	return b.editRelease(id, "comment", releaseData{Message: message})
}

// YankRelease marks the record with this id yanked, for reason.
func (b *Backend) YankRelease(id, reason string) error {
	if reason == "" {
		return errors.New("a yank needs its reason")
	}
	return b.editRelease(id, "yank", releaseData{Reason: reason})
}

// editRelease adds an operation by you to the record with this id.
func (b *Backend) editRelease(id, kind string, d releaseData) error {
	l, err := oplog.Load(b.s, "release", id)
	if err != nil {
		return fmt.Errorf("reading release %s: %w", id, err)
	}
	me, err := b.me()
	if err != nil {
		return err
	}
	if err := l.Append(ctx, b.s, []oplog.Op{releaseOp(kind, me, d)}); err != nil {
		return fmt.Errorf("editing release %s: %w", id, err)
	}
	return nil
}

// PushReleases pushes the records to remote, and the persons who wrote
// them, as Push does with the issues.
func (b *Backend) PushReleases(remote string) error {
	if err := b.pushKinds(remote, "person", "release"); err != nil {
		return fmt.Errorf("pushing the releases to %s: %w", remote, err)
	}
	return nil
}

// PullReleases fetches remote's entities and merges them, the records and
// the persons who wrote them with the rest.
// ponytail: merges the issues too, as Pull does; the release kind alone
// if that ever gets in the way
func (b *Backend) PullReleases(remote string) error {
	if err := b.Pull(remote); err != nil {
		return fmt.Errorf("pulling the releases from %s: %w", remote, err)
	}
	return nil
}

// pushKinds pushes the entities of these kinds, those with a ref at all.
func (b *Backend) pushKinds(remote string, kinds ...string) error {
	if err := b.checkRemote(remote); err != nil {
		return err
	}
	args := []string{"push", remote}
	for _, k := range kinds {
		refs, err := b.s.Refs(oplog.RefName(k, ""))
		if err != nil {
			return err
		}
		if len(refs) > 0 {
			args = append(args, fmt.Sprintf("refs/board/%s/*:refs/board/%s/*", k, k))
		}
	}
	if len(args) == 2 {
		return nil
	}
	_, err := b.run(nil, args...)
	return err
}
