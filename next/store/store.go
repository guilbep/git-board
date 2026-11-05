// Package store is where a board's entities are kept: the Datastore
// interface every datastore implements, its capabilities and its errors
// (ADR 0002). A datastore keeps packs, the bytes an entity's operations
// are written in, and never reads them but for what its own format needs
// (git chains them by their clock). docs/spec/storage.md is git's format.
package store

import (
	"context"
	"errors"
)

// Datastore keeps entities as packs.
type Datastore interface {
	// Heads lists every entity of the kinds, each with its version,
	// sorted by kind then id. It is snapshot-safe: an entity that exists
	// from the call's start to its end is listed, whatever writers do
	// meanwhile. A ref whose name isn't an id is skipped (storage.md,
	// Refs).
	Heads(ctx context.Context, kinds ...string) ([]Head, error)
	// Load streams the packs of each ref at its version ("" for the
	// current one), in any order, as each is read: each gets the version
	// it was read at and every pack reachable from it. A ref with no
	// entity gets ErrNotFound through each, and the load goes on; an
	// error each returns stops it, and each may be called on several
	// goroutines at once. Load of one ref is a point read; of 20,000, a
	// first build. A datastore that isn't Pinned answers each ref at its
	// current version, whatever version it names.
	Load(ctx context.Context, refs []Ref, each func(r Ref, packs [][]byte, err error) error) error
	// Add stores one pack if the entity is at prev ("" for a new one),
	// and gives the new version; ErrMoved otherwise.
	Add(ctx context.Context, kind, id string, pack []byte, prev string) (string, error)
	Caps() Caps
	// Folder is this machine's own folder for the board: the view, the
	// sync lock and its journal. Never shared, never the board.
	Folder() string
	Close() error
}

// Head is an entity as a listing gives it.
type Head struct{ Kind, ID, Version string }

// Ref names an entity at a version; "" is its current one.
type Ref struct{ Kind, ID, Version string }

// Caps says what a datastore can do beyond the interface. A nil field is
// a capability it lacks; a feature that needs it fails with ErrMissing
// and a sentence the user reads (ADR 0002 §3). A wrapper forwards them by
// copying the value.
type Caps struct {
	Pinned  bool    // a version names its packs for good (git): no recheck
	Shared  bool    // every machine reads the same store (PostgreSQL)
	Counter Counter // project numbers (ADR 0016)
	Locker  Locker  // a lock every machine shares (PostgreSQL)
	Remote  Remote  // fetch, push, start over (git)
	SQL     SQL     // a database the view may live in, beside the board (PostgreSQL)
	Put     Putter  // takes another datastore's packs as they are: datastore copy
}

// SQL is the database a datastore keeps the board in, where the view may
// keep its tables too, in a schema of its own.
type SQL interface {
	// DSN is the database's connection string, as libpq takes it.
	DSN() string
}

// Putter stores another datastore's packs as they are, clocks and all,
// in one write: datastore copy (datastores.md, Copying a board in).
type Putter interface {
	// Put stores the packs the entity doesn't hold yet if it is at prev
	// ("" for a new one), and gives its version: prev when it held them
	// all. ErrMoved when it moved.
	Put(ctx context.Context, kind, id string, packs [][]byte, prev string) (string, error)
}

// Counter keeps the project numbers' counter (ADR 0016).
type Counter interface {
	Count(ctx context.Context) (value []byte, version string, err error)
	SetCount(ctx context.Context, value []byte, prev string) (version string, err error)
}

// Locker holds a lock every machine on the board shares.
type Locker interface {
	Lock(ctx context.Context, name string) (unlock func(), err error)
}

// Remote fetches, merges and pushes the kinds' entities (storage.md,
// Fetch and push).
type Remote interface {
	Fetch(ctx context.Context, remote string, kinds ...string) ([]Head, error)
	Push(ctx context.Context, remote string, kinds ...string) error
	Drop(ctx context.Context, kinds ...string) error
}

var (
	ErrNotFound = errors.New("store: no such entity")
	ErrMoved    = errors.New("store: entity moved since it was read")
	ErrMissing  = errors.New("store: capability missing")
	ErrHeld     = errors.New("store: lock held")
)

// IsID says id is an entity id: 40 lowercase hex digits. A listing asks
// it of every ref, twice (its name, its version): a table, no branch a
// byte.
func IsID(id string) bool {
	if len(id) != 40 {
		return false
	}
	var bad byte
	for i := 0; i < 40; i++ {
		bad |= notHex[id[i]]
	}
	return bad == 0
}

// notHex is 1 for every byte that isn't a lowercase hex digit.
var notHex = func() (t [256]byte) {
	for i := range t {
		if c := byte(i); (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			t[i] = 1
		}
	}
	return t
}()
