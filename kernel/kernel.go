// Package kernel is the board's core: entities, operations, ids, their
// order, kinds, and the board that opens a datastore and its view
// (docs/rewrite/design.md, section 3).
//
// A kind is a value a domain defines once (Define) and hands to Open: its
// fold (Apply), what the view keeps of a state (Index) and how a row is
// encoded (Row). The kernel, the view and the datastores know no kind by
// name: a fifth kind is a new Define and nothing else.
package kernel

import (
	"errors"
	"fmt"
	"strings"

	"github.com/guilbep/git-board/kernel/pack"
	"github.com/guilbep/git-board/store"
)

// ID names an entity: 40 lowercase hex digits.
type ID string

// OpID names an operation: "<pack hash>:<index>".
type OpID string

// Version is a datastore's opaque version of an entity: a commit in git.
type Version string

// Op is one operation, as docs/spec/data-model.md writes it.
type Op = pack.Op

// Origin says where on a forge an operation came from.
type Origin = pack.Origin

// Entry is an operation in its entity's order.
type Entry struct {
	ID    OpID
	Clock uint64
	Op
}

// Pack is the pack hash in an operation's id.
func (id OpID) Pack() ID {
	h, _, _ := strings.Cut(string(id), ":")
	return ID(h)
}

// Spec defines a kind. S is its state, R its row: what lists show and
// filter on.
type Spec[S, R any] struct {
	Name  string // the ref segment: refs/board/<Name>/<id>
	Given bool   // ids are given (sync: SHA-1 of the forge key), not the first pack's hash
	Rev   int    // bumped when Apply, Index or Row change: the view rebuilds this kind
	New   func(id ID) S
	// Apply applies one operation. An unknown kind is an error: the
	// kernel never drops an operation (data-model.md, Operations).
	Apply func(s *S, e Entry) error
	// Index is what the view keeps of a state.
	Index func(s *S) Doc[R]
	// Row encodes and decodes R; nil is JSON.
	Row Codec[R]
}

// Codec encodes a row as bytes and back.
type Codec[R any] interface {
	Append(dst []byte, r *R) []byte
	Decode(src []byte, r *R) error
}

// Doc is a state as the view keeps it.
type Doc[R any] struct {
	Row R
	// Terms are exact terms, ns:value (ADR 0007): is:open, bug,
	// author:<id>, account:github.com/42.
	Terms   []string
	Texts   []Text // free text by field: title, body, comments
	Created int64
	Updated int64
	Hidden  bool // a tombstone (ADR 0019): kept, never listed
}

// Text is one field's free text.
type Text struct{ Field, Body string }

// Entity is one entity's state at a version.
type Entity[S any] struct {
	ID      ID
	Version Version
	Clock   uint64 // its head's clock: the next pack's is Clock+1
	State   S
}

// Listed is a row as a list answers it.
type Listed[R any] struct {
	ID      ID
	Version Version
	Row     R
}

// Query is what the view answers; docs/spec/queries.md gives the
// semantics. An empty field filters nothing.
type Query struct {
	All  []string   // every term
	Any  [][]string // one term of each group
	Not  []string   // none of the terms
	Text string     // free text: each word's tokens in a row, the last a prefix
	In   []string   // the text fields Text is looked for in; none: all
	Sort string     // "created", "updated" or "" (by id)
	Desc bool
	// Limit is the most rows answered; 0 answers all.
	Limit int
}

// Errors.
var (
	ErrNotFound  = errors.New("not found")
	ErrAmbiguous = errors.New("ambiguous id prefix")
	ErrMoved     = store.ErrMoved // after 5 tries
)

// Broken is an entity that fails to load: a query never answers it, and
// never fails because of it (queries.md, Freshness).
type Broken struct {
	Kind string
	ID   ID
	Err  error
}

// Change is an entity that appeared, moved or went.
type Change struct {
	Kind    string
	ID      ID
	Version Version // "" when gone
}

// Kind is a K without its types, for Open.
type Kind interface {
	Name() string
	// spec is the kind as the board drives it: only K implements Kind.
	spec() kindSpec
}

// kindSpec is a kind untyped: its name, its rev, and its fold from the
// ordered operations to what the view keeps.
type kindSpec struct {
	name  string
	rev   int
	given bool
	fold  func(id ID, entries []Entry) (Indexed, error)
}

// Indexed is a Doc as the view stores it: the row encoded.
type Indexed struct {
	Row              []byte
	Terms            []string
	Texts            []Text
	Created, Updated int64
	Hidden           bool
}

// K is a registered kind, typed.
type K[S, R any] struct {
	s     Spec[S, R]
	codec Codec[R]
}

// Define makes a kind. A domain calls it once, in a package variable, and
// hands it to Open.
func Define[S, R any](s Spec[S, R]) *K[S, R] {
	if s.Name == "" || strings.ContainsAny(s.Name, "/. ") || s.New == nil || s.Apply == nil || s.Index == nil {
		panic(fmt.Sprintf("kernel: kind %q is missing a name, New, Apply or Index", s.Name))
	}
	k := &K[S, R]{s: s, codec: s.Row}
	if k.codec == nil {
		k.codec = jsonCodec[R]{}
	}
	return k
}

// Name is the kind's ref segment.
func (k *K[S, R]) Name() string { return k.s.Name }

func (k *K[S, R]) spec() kindSpec {
	return kindSpec{name: k.s.Name, rev: k.s.Rev, given: k.s.Given, fold: func(id ID, entries []Entry) (Indexed, error) {
		st, err := k.fold(id, entries)
		if err != nil {
			return Indexed{}, err
		}
		d := k.s.Index(&st)
		return Indexed{Row: k.codec.Append(nil, &d.Row), Terms: d.Terms, Texts: d.Texts,
			Created: d.Created, Updated: d.Updated, Hidden: d.Hidden}, nil
	}}
}

// fold is the state entries add up to.
func (k *K[S, R]) fold(id ID, entries []Entry) (S, error) {
	s := k.s.New(id)
	for _, e := range entries {
		if err := k.s.Apply(&s, e); err != nil {
			return s, err
		}
	}
	return s, nil
}

// order is an entity's ordered operations from its packs, and its clock.
func order(packs [][]byte) ([]Entry, uint64, error) {
	es, clock, err := pack.Order(packs)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Entry, len(es))
	for i, e := range es {
		out[i] = Entry{ID: OpID(e.ID), Clock: e.Clock, Op: e.Op}
	}
	return out, clock, nil
}
