// Package datastore is where a board's entities are kept: the interface the
// core writes through, and the errors it answers (ADR 0002).
//
// A datastore keeps packs, the JSON blobs an entity's operations are written
// in, byte for byte. It never reads them: core/oplog encodes, hashes and
// orders them. So an entity's id, its operations' ids and every reference to
// them are the same in every datastore.
//
// git (core/gitstore) and PostgreSQL (core/pgstore) are the datastores.
package datastore

import (
	"context"
	"errors"
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

// Counter is a datastore that keeps a shared counter, for project numbers.
type Counter interface {
	// Count reads the counter: its value (nil when none was started) and
	// its version.
	Count(ctx context.Context) (value []byte, version string, err error)
	// SetCount writes the counter if its version is still prev ("" for one
	// that must not exist yet): ErrMoved otherwise.
	SetCount(ctx context.Context, value []byte, prev string) (version string, err error)
}

// Missing is ErrMissing with the sentence a user reads.
func Missing(sentence string) error { return &missing{sentence} }

type missing struct{ sentence string }

func (m *missing) Error() string { return m.sentence }
func (m *missing) Unwrap() error { return ErrMissing }
