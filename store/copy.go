package store

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync/atomic"
)

// Copied is what a Copy wrote.
type Copied struct {
	Entities int  // written to: new, or with packs they lacked
	Counter  bool // the counter, copied
}

// Copy copies the kinds' entities of from into to: for each, the packs to
// lacks, in one Put, so a second copy writes nothing (datastores.md,
// Copying a board in). Ids stay: they are the packs' hashes, the same
// bytes. The counter is copied when to keeps one and has none yet. to
// must take packs as they are (Caps.Put).
func Copy(ctx context.Context, from, to Datastore, kinds ...string) (Copied, error) {
	var c Copied
	put := to.Caps().Put
	if put == nil {
		return c, fmt.Errorf("%w: this datastore can't take another's packs as they are", ErrMissing)
	}
	theirs, err := from.Heads(ctx, kinds...)
	if err != nil {
		return c, err
	}
	ours, err := to.Heads(ctx, kinds...)
	if err != nil {
		return c, err
	}
	have := make(map[Ref]bool, len(ours))
	for _, h := range ours {
		have[Ref{Kind: h.Kind, ID: h.ID}] = true
	}
	refs := make([]Ref, len(theirs))
	for i, h := range theirs {
		refs[i] = Ref(h)
	}
	var n atomic.Int64 // from's Load may call on several goroutines
	err = from.Load(ctx, refs, func(r Ref, packs [][]byte, err error) error {
		if errors.Is(err, ErrNotFound) {
			return nil // gone meanwhile
		}
		if err != nil {
			return fmt.Errorf("copying %s %s: %w", r.Kind, r.ID, err)
		}
		prev := ""
		if have[Ref{Kind: r.Kind, ID: r.ID}] {
			var mine [][]byte
			err := to.Load(ctx, []Ref{{Kind: r.Kind, ID: r.ID}}, func(m Ref, p [][]byte, err error) error {
				mine, prev = p, m.Version
				return err
			})
			if err != nil {
				return fmt.Errorf("copying %s %s: %w", r.Kind, r.ID, err)
			}
			packs = slices.DeleteFunc(slices.Clone(packs), func(p []byte) bool {
				return slices.ContainsFunc(mine, func(m []byte) bool { return bytes.Equal(bytes.TrimSpace(m), bytes.TrimSpace(p)) })
			})
			if len(packs) == 0 {
				return nil
			}
		}
		if _, err := put.Put(ctx, r.Kind, r.ID, packs, prev); err != nil {
			return fmt.Errorf("copying %s %s: %w", r.Kind, r.ID, err)
		}
		n.Add(1)
		return nil
	})
	c.Entities = int(n.Load())
	if err != nil {
		return c, err
	}
	src, dst := from.Caps().Counter, to.Caps().Counter
	if src == nil || dst == nil {
		return c, nil
	}
	value, _, err := src.Count(ctx)
	if err != nil || value == nil {
		return c, err
	}
	mine, _, err := dst.Count(ctx)
	if err != nil || mine != nil {
		return c, err
	}
	if _, err := dst.SetCount(ctx, value, ""); err != nil {
		return c, fmt.Errorf("copying the counter: %w", err)
	}
	c.Counter = true
	return c, nil
}
