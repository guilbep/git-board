package store

import (
	"context"
	"sync/atomic"
)

// Counts is the work a board's reads and writes did, counted for the
// tests that guard their complexity (docs/guide/performance.md, v2): a
// warm ls reads no entity, a write loads none, a refresh after one move
// loads one. Each layer adds to the Counts its context carries, and does
// nothing more than a context lookup when it carries none.
type Counts struct {
	Listings  atomic.Int64 // Heads: listings of the datastore
	Loads     atomic.Int64 // Load: calls
	Entities  atomic.Int64 // entities loaded
	Objects   atomic.Int64 // git objects read (store/git)
	Processes atomic.Int64 // git processes started (store/git)
	Adds      atomic.Int64 // packs added
	ViewCalls atomic.Int64 // calls to the view (kernel): its queries
	RowsRead  atomic.Int64 // rows the view answered
	RowsPut   atomic.Int64 // rows the view wrote or dropped
}

type countsKey struct{}

// WithCounts is ctx counting into c.
func WithCounts(ctx context.Context, c *Counts) context.Context {
	return context.WithValue(ctx, countsKey{}, c)
}

// CountsOf is the Counts ctx counts into, nil when none: each method
// of a nil Counts does nothing.
func CountsOf(ctx context.Context) *Counts {
	c, _ := ctx.Value(countsKey{}).(*Counts)
	return c
}

// Snapshot is the counters' values, by name.
func (c *Counts) Snapshot() map[string]int64 {
	return map[string]int64{
		"listings": c.Listings.Load(), "loads": c.Loads.Load(), "entities": c.Entities.Load(),
		"objects": c.Objects.Load(), "processes": c.Processes.Load(), "adds": c.Adds.Load(),
		"view calls": c.ViewCalls.Load(), "rows read": c.RowsRead.Load(), "rows put": c.RowsPut.Load(),
	}
}
