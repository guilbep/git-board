package kernel

import (
	"context"
	"crypto/sha256"
	"encoding/json"
)

// View is where the board's read model lives: every kind's rows, terms
// and texts, each keyed by the version it was folded at, and per kind
// the listing they were folded from (design.md, The read path). It is
// disposable: deleting it loses nothing. view keeps it in sqlite.
//
// The kernel decides what goes in it; a View stores, and answers queries.
type View interface {
	// Mark is what the view holds of a kind.
	Mark(ctx context.Context, kind string) (Mark, error)
	// Heads is the listing a kind's rows are at, broken entities
	// included, sorted by id.
	Heads(ctx context.Context, kind string) ([]Head, error)
	// Write changes a kind's rows in one transaction. Each put and drop
	// lands only where the stored version is still its Base: a row
	// another process wrote meanwhile is left to it.
	Write(ctx context.Context, kind string, w Write) error
	// Rows answers the visible rows matching q, in its order.
	Rows(ctx context.Context, kind string, q Query) ([]Row, error)
	// Resolve answers up to two visible ids starting with prefix, a
	// valid one (1 to 40 lowercase hex digits).
	Resolve(ctx context.Context, kind, prefix string) ([]ID, error)
	// Unsearchable are the rows whose terms and texts aren't kept yet.
	Unsearchable(ctx context.Context, kind string) ([]Head, error)
	// Broken are the entities that failed to load, every kind's.
	Broken(ctx context.Context) ([]Broken, error)
	Close() error
}

// Mark is what a view holds of a kind: the rev its rows were folded by,
// the sum of the listing they are at (Sum), and whether it keeps the
// search index, terms and texts: from the first query that searches on.
// A first build writes rows alone. Comparing a listing's sum with Sum is
// the whole of a fresh query's check.
type Mark struct {
	Rev    int
	Sum    [32]byte
	Search bool
}

// Head is an entity at a version.
type Head struct {
	ID      ID
	Version Version
}

// Write is one transaction's changes to a kind.
type Write struct {
	Rev    int  // the rev the puts were folded by
	Reset  bool // drop the kind's rows first, if they are still of another rev
	Search bool // keep terms and texts from now on
	Put    []Put
	Drop   []Head // each at its Version, the base
}

// Put is one entity's new row, or its error, at Version, replacing the
// one at Base ("" for none).
type Put struct {
	ID            ID
	Base, Version Version
	Doc           Indexed
	Err           string // broken: no row, the error kept
	Search        bool   // Doc.Terms and Doc.Texts are given, to keep
}

// Row is a row as the view answers it.
type Row struct {
	ID      ID
	Version Version
	Row     []byte
}

// Sum is a listing's sum, as a Mark keeps it: each head's HeadSum added,
// mod 2^256. The order of the heads doesn't count, and a write of one
// head changes the sum by its own (AddSum, SubSum), without reading the
// 20,000 others.
func Sum(heads []Head) [32]byte {
	var s [32]byte
	for _, h := range heads {
		s = AddSum(s, HeadSum(h))
	}
	return s
}

// HeadSum is one head's part of a Sum: the sha256 of "<id> <version>".
func HeadSum(h Head) [32]byte {
	var buf [160]byte
	b := append(buf[:0], h.ID...)
	b = append(b, ' ')
	return sha256.Sum256(append(b, h.Version...))
}

// AddSum is a + b, mod 2^256, as big-endian numbers.
func AddSum(a, b [32]byte) [32]byte {
	var carry uint16
	for i := 31; i >= 0; i-- {
		t := uint16(a[i]) + uint16(b[i]) + carry
		a[i], carry = byte(t), t>>8
	}
	return a
}

// SubSum is a - b, mod 2^256: AddSum undone.
func SubSum(a, b [32]byte) [32]byte {
	var borrow int16
	for i := 31; i >= 0; i-- {
		t := int16(a[i]) - int16(b[i]) - borrow
		borrow = 0
		if t < 0 {
			t += 256
			borrow = 1
		}
		a[i] = byte(t)
	}
	return a
}

// jsonCodec is a row as JSON, for a kind that brings no codec.
type jsonCodec[R any] struct{}

func (jsonCodec[R]) Append(dst []byte, r *R) []byte {
	b, err := json.Marshal(r)
	if err != nil {
		panic(err) // a row is plain data
	}
	return append(dst, b...)
}

func (jsonCodec[R]) Decode(src []byte, r *R) error { return json.Unmarshal(src, r) }
