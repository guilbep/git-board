package kernel

import (
	"context"
	"crypto/sha256"
	"encoding/json"
)

// View is where the board's read model lives: every kind's rows, terms
// and texts, each keyed by the version it was folded at, and per kind
// the listing they were folded from (design.md, The read path). It is
// disposable: deleting it loses nothing. next/view keeps it in sqlite.
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
	// Untexted are the rows whose texts aren't kept yet.
	Untexted(ctx context.Context, kind string) ([]Head, error)
	// Broken are the entities that failed to load, every kind's.
	Broken(ctx context.Context) ([]Broken, error)
	Close() error
}

// Mark is what a view holds of a kind: the rev its rows were folded by,
// the sum of the listing they are at (Sum), and whether texts are kept,
// from the first query that asked for text on. Comparing a listing's sum
// with it is the whole of a fresh query's check.
type Mark struct {
	Rev   int
	Sum   [32]byte
	Texts bool
}

// Head is an entity at a version.
type Head struct {
	ID      ID
	Version Version
}

// Write is one transaction's changes to a kind.
type Write struct {
	Rev   int  // the rev the puts were folded by
	Reset bool // drop the kind's rows first, if they are still of another rev
	Texts bool // keep texts from now on
	Put   []Put
	Drop  []Head // each at its Version, the base
}

// Put is one entity's new row, or its error, at Version, replacing the
// one at Base ("" for none).
type Put struct {
	ID            ID
	Base, Version Version
	Doc           Indexed
	Err           string // broken: no row, the error kept
	Texted        bool   // Doc.Texts are given, every one of the entity's
}

// Row is a row as the view answers it.
type Row struct {
	ID      ID
	Version Version
	Row     []byte
}

// Listing encodes heads, sorted by id: a line "<id> <version>" each.
// Two listings are the same when their bytes are.
func Listing(heads []Head) []byte {
	b := make([]byte, 0, len(heads)*82)
	for _, h := range heads {
		b = append(b, h.ID...)
		b = append(b, ' ')
		b = append(b, h.Version...)
		b = append(b, '\n')
	}
	return b
}

// Sum is a listing's sum, as a Mark keeps it.
func Sum(heads []Head) [32]byte { return sha256.Sum256(Listing(heads)) }

// ParseListing is Listing read back.
func ParseListing(b []byte) []Head {
	out := make([]Head, 0, len(b)/82)
	for len(b) > 0 {
		i := 0
		for i < len(b) && b[i] != '\n' {
			i++
		}
		line := b[:i]
		if i < len(b) {
			i++
		}
		b = b[i:]
		for j := 0; j < len(line); j++ {
			if line[j] == ' ' {
				out = append(out, Head{ID: ID(line[:j]), Version: Version(line[j+1:])})
				break
			}
		}
	}
	return out
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
