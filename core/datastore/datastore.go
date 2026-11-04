// Package datastore is where a board's entities are kept: the interface the
// core writes through, and the errors it answers (ADR 0002).
//
// A datastore keeps packs, the JSON blobs an entity's operations are written
// in, byte for byte. It never reads them, but for git, which chains them by
// their clock: core/oplog encodes, hashes and orders them. So an entity's id, its operations' ids and every reference to
// them are the same in every datastore.
//
// git (core/gitstore) and PostgreSQL (core/pgstore) are the datastores.
package datastore

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
)

// Kinds are the entity kinds a datastore keeps.
var Kinds = []string{"issue", "person", "sync", "release"}

// Errors a datastore answers.
var (
	// ErrNotFound: the entity has no pack.
	ErrNotFound = errors.New("datastore: no such entity")
	// ErrMoved: the entity's version is no longer the one the write read.
	ErrMoved = errors.New("datastore: entity moved since it was read")
	// ErrMissing: the datastore lacks a capability a feature needs. It is
	// wrapped with the sentence the user reads.
	ErrMissing = errors.New("datastore: capability missing")
	// ErrHeld: another holds the lock a Locker was asked for.
	ErrHeld = errors.New("datastore: lock held")
)

// Datastore keeps entities as packs.
type Datastore interface {
	// Heads are a kind's entities, by id, each with its version: an opaque
	// string that changes whenever the entity does.
	Heads(ctx context.Context, kind string) (map[string]string, error)
	// Packs are every pack of an entity, in any order, and its version:
	// ErrNotFound when there is none.
	Packs(ctx context.Context, kind, id string) (packs [][]byte, version string, err error)
	// Add stores one pack on an entity if its version is still prev ("" for
	// an entity that must not exist yet), and gives the new version: ErrMoved
	// otherwise.
	Add(ctx context.Context, kind, id string, pack []byte, prev string) (version string, err error)
	// Folder is this machine's own folder for the board: the index, the sync
	// lock and its journal, the caches. It is never shared, and is not the
	// board.
	Folder() string
}

// Versioned is a datastore whose versions name what they hold: an entity's
// packs at a version Heads gave stay what they were, whatever the entity
// became since. A reader that has the version reads them without asking
// for the entity's version again. git is one: a version is a commit.
type Versioned interface {
	// PacksAt are every pack of the entity at version, in any order.
	PacksAt(ctx context.Context, kind, id, version string) ([][]byte, error)
}

// Counter is a datastore that keeps a shared counter, for project numbers.
type Counter interface {
	// Count reads the counter: its value (nil when none was started) and
	// its version.
	Count(ctx context.Context) (value []byte, version string, err error)
	// SetCount writes the counter if its version is still prev ("" for one
	// that must not exist yet): ErrMoved otherwise.
	SetCount(ctx context.Context, value []byte, prev string) (version string, err error)
}

// Locker is a datastore that every machine on the board shares, and that
// holds locks for them all: a forge sync, one at a time per board, takes
// its lock there. A datastore each machine keeps a copy of (git, the
// Markdown files) is no Locker: a machine's own file lock is the board's.
type Locker interface {
	// Lock takes the lock named name without waiting: ErrHeld when
	// another holds it, on this machine or another. unlock lets it go. A
	// holder that ends without unlocking, however it ends, keeps no one
	// out.
	Lock(ctx context.Context, name string) (unlock func(), err error)
}

// Missing is ErrMissing with the sentence a user reads.
func Missing(sentence string) error { return &missing{sentence} }

type missing struct{ sentence string }

func (m *missing) Error() string { return m.sentence }
func (m *missing) Unwrap() error { return ErrMissing }

// Copy copies every entity of from into to: the packs to hasn't, a whole
// entity or the rest of one a copy cut short, oldest first, by (clock,
// bytes), as git chains them in that order. It copies from's counter
// when to keeps one and has none. A board moving between datastores keeps
// its ids, as its packs copy as they are. It answers how many entities it
// wrote to.
func Copy(ctx context.Context, from, to Datastore) (int, error) {
	n := 0
	for _, kind := range Kinds {
		theirs, err := from.Heads(ctx, kind)
		if err != nil {
			return n, err
		}
		for id := range theirs {
			packs, _, err := from.Packs(ctx, kind, id)
			if err != nil {
				return n, err
			}
			byClock(packs)
			mine, v, err := to.Packs(ctx, kind, id)
			if err != nil && !errors.Is(err, ErrNotFound) {
				return n, err
			}
			have := map[string]bool{}
			for _, p := range mine {
				have[string(p)] = true
			}
			wrote := false
			for _, p := range packs {
				if have[string(p)] {
					continue
				}
				if v, err = to.Add(ctx, kind, id, p, v); err != nil {
					return n, err
				}
				have[string(p)], wrote = true, true
			}
			if wrote {
				n++
			}
		}
	}
	fc, ok1 := from.(Counter)
	tc, ok2 := to.(Counter)
	if !ok1 || !ok2 {
		return n, nil
	}
	value, _, err := fc.Count(ctx)
	if err != nil || value == nil {
		return n, err
	}
	if mine, _, err := tc.Count(ctx); err != nil || mine != nil {
		return n, err
	}
	_, err = tc.SetCount(ctx, value, "")
	return n, err
}

// byClock sorts packs by their clock, then their bytes. This is all Copy
// reads of a pack: its "clock". One it can't read sorts first.
func byClock(packs [][]byte) {
	clock := func(p []byte) uint64 {
		var hd struct {
			Clock uint64 `json:"clock"`
		}
		_ = json.Unmarshal(p, &hd)
		return hd.Clock
	}
	slices.SortStableFunc(packs, func(a, b []byte) int {
		return cmp.Or(cmp.Compare(clock(a), clock(b)), bytes.Compare(a, b))
	})
}
