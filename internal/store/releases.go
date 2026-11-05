package store

import (
	"fmt"

	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/core/release"

	"github.com/guilbep/git-board/internal/avatar"
)

// The release records are the core's (core/release); the Backend writes
// them as you and names their authors.

// releaseNames is a record's people by name, from the persons read once.
func (b *Backend) releaseNames() (func(id string) string, error) {
	persons, err := b.allPersons()
	if err != nil {
		return nil, err
	}
	return func(id string) string { return personOf(id, persons[id], avatar.None).Name }, nil
}

// Releases is the records, one per commit, newest first (release.Records).
func (b *Backend) Releases() ([]release.Record, error) {
	name, err := b.releaseNames()
	if err != nil {
		return nil, err
	}
	return release.Records(b.ds, name)
}

// ReleaseFor is commit's record; ok is false while it has none.
func (b *Backend) ReleaseFor(commit string) (r release.Record, ok bool, err error) {
	name, err := b.releaseNames()
	if err != nil {
		return release.Record{}, false, err
	}
	return release.For(b.ds, commit, name)
}

// CreateRelease records r's commit, previous, artifacts, chart and assets,
// by you; the rest is the other operations'. A commit that has a record
// already keeps it, and it is returned.
func (b *Backend) CreateRelease(r release.Record) (release.Record, error) {
	me, err := b.me()
	if err != nil {
		return release.Record{}, err
	}
	return release.Create(ctx, b.ds, me, r, b.personName)
}

// personName is a person's name, by id.
func (b *Backend) personName(id string) string {
	p, _ := people.Load(b.ds, id)
	return personOf(id, p, avatar.None).Name
}

// SetReleaseNotes replaces the notes of the record with this id.
func (b *Backend) SetReleaseNotes(id, notes string) error {
	return b.editRelease(func(me string) error { return release.SetNotes(ctx, b.ds, id, me, notes) })
}

// CommentRelease comments on the record with this id.
func (b *Backend) CommentRelease(id, message string) error {
	return b.editRelease(func(me string) error { return release.AddComment(ctx, b.ds, id, me, message) })
}

// YankRelease marks the record with this id yanked, for reason.
func (b *Backend) YankRelease(id, reason string) error {
	return b.editRelease(func(me string) error { return release.Yank(ctx, b.ds, id, me, reason) })
}

// editRelease runs edit as you.
func (b *Backend) editRelease(edit func(me string) error) error {
	me, err := b.me()
	if err != nil {
		return err
	}
	return edit(me)
}

// PushReleases pushes the records to remote, and the persons who wrote
// them, as Push does with the issues.
func (b *Backend) PushReleases(remote string) error {
	if !b.inGit() {
		return nil // the datastore is shared: the records are there
	}
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
	if !b.inGit() {
		return nil
	}
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
	if err := b.guarded(remote); err != nil {
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
