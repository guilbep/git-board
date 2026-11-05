// Package storetest is the conformance suite every datastore runs: add
// and read, one writer wins, what a datastore refuses, and what a listing
// and a load guarantee while others write (docs/rewrite/design.md, 2.6:
// a listing that missed refs git moved once dropped every issue).
package storetest

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guilbep/git-board/kernel/pack"
	"github.com/guilbep/git-board/store"
)

// Options say how to run the suite on a datastore.
type Options struct {
	// Open opens a new, empty datastore.
	Open func(t *testing.T) store.Datastore
	// Churn, when set, does what other processes do to the datastore
	// until ctx ends (git: real `git update-ref` of other refs, and `git
	// pack-refs`), while the suite reads and writes.
	Churn func(ctx context.Context, ds store.Datastore) error
	// Junk, when set, puts names in the datastore that aren't entities
	// (git: refs/board/issue/notes). Every listing skips them.
	Junk func(t *testing.T, ds store.Datastore)
}

// ID is the n-th test id.
func ID(n int) string { return fmt.Sprintf("%040x", n) }

// Pack is a pack at clock, with one operation; a first one has a nonce.
func Pack(clock uint64) []byte {
	b, err := pack.Encode(pack.Pack{Clock: clock, Ops: []pack.Op{{Kind: "comment", Author: ID(0), Time: int64(clock)}}})
	if err != nil {
		panic(err)
	}
	return b
}

var ctx = context.Background()

// Run runs the suite.
func Run(t *testing.T, o Options) {
	t.Run("empty", func(t *testing.T) {
		t.Parallel()
		ds := o.Open(t)
		heads, err := ds.Heads(ctx, "issue", "person")
		if err != nil || len(heads) != 0 {
			t.Fatalf("Heads = %v, %v; want none", heads, err)
		}
		var got []error
		err = ds.Load(ctx, []store.Ref{{Kind: "issue", ID: ID(1)}, {Kind: "issue", ID: ID(2), Version: "x"}},
			func(_ store.Ref, _ [][]byte, err error) error { got = append(got, err); return nil })
		if err != nil || len(got) != 2 || !errors.Is(got[0], store.ErrNotFound) || !errors.Is(got[1], store.ErrNotFound) {
			t.Fatalf("Load of nothing = %v, %v; want ErrNotFound for each, and no error", got, err)
		}
	})

	t.Run("add and read", func(t *testing.T) {
		t.Parallel()
		ds := o.Open(t)
		v1 := add(t, ds, "issue", ID(2), Pack(1), "")
		add(t, ds, "issue", ID(1), Pack(1), "")
		add(t, ds, "person", ID(3), Pack(1), "")
		v2 := add(t, ds, "issue", ID(2), Pack(2), v1)
		heads, err := ds.Heads(ctx, "person", "issue")
		if err != nil || len(heads) != 3 {
			t.Fatalf("Heads = %v, %v; want 3", heads, err)
		}
		want := []store.Head{{Kind: "issue", ID: ID(1), Version: heads[0].Version}, {Kind: "issue", ID: ID(2), Version: v2},
			{Kind: "person", ID: ID(3), Version: heads[2].Version}}
		if !slices.Equal(heads, want) {
			t.Fatalf("Heads = %v, want %v: sorted by kind, then id", heads, want)
		}
		if only, _ := ds.Heads(ctx, "person"); len(only) != 1 {
			t.Fatalf("Heads(person) = %v, want the person alone", only)
		}
		refs := []store.Ref{{Kind: "issue", ID: ID(2)}}
		if ds.Caps().Pinned {
			refs = append(refs, store.Ref{Kind: "issue", ID: ID(2), Version: v1})
		}
		got := map[string]int{}
		err = ds.Load(ctx, refs, func(r store.Ref, packs [][]byte, err error) error {
			if err != nil {
				return err
			}
			got[r.Version] = len(packs)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if got[v2] != 2 || ds.Caps().Pinned && got[v1] != 1 {
			t.Fatalf("Load = %v packs by version, want 2 at %s (the current one, named) and 1 at %s", got, v2, v1)
		}
	})

	t.Run("one writer wins", func(t *testing.T) {
		t.Parallel()
		ds := o.Open(t)
		v := add(t, ds, "issue", ID(1), Pack(1), "")
		var won, moved atomic.Int32
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				_, err := ds.Add(ctx, "issue", ID(1), Pack(2), v)
				switch {
				case err == nil:
					won.Add(1)
				case errors.Is(err, store.ErrMoved):
					moved.Add(1)
				default:
					t.Error(err)
				}
			})
		}
		wg.Wait()
		if won.Load() != 1 || moved.Load() != 7 {
			t.Fatalf("%d writers won, %d were told it moved; want 1 and 7", won.Load(), moved.Load())
		}
	})

	t.Run("refuses", func(t *testing.T) {
		t.Parallel()
		ds := o.Open(t)
		old := add(t, ds, "issue", ID(1), Pack(1), "")
		v := add(t, ds, "issue", ID(1), Pack(2), old)
		empty, _ := pack.Encode(pack.Pack{Clock: 3, Ops: []pack.Op{}})
		for _, c := range []struct {
			name, id, prev string
			pack           []byte
			moved          bool
		}{
			{"a new entity that exists", ID(1), "", Pack(1), true},
			{"a version it moved from", ID(1), old, Pack(2), true},
			{"an entity that doesn't exist", ID(2), v, Pack(3), true},
			{"a bad id", "notes", "", Pack(1), false},
			{"a clock that isn't the next", ID(1), v, Pack(5), false},
			{"a pack with no operation", ID(1), v, empty, false},
		} {
			_, err := ds.Add(ctx, "issue", c.id, c.pack, c.prev)
			if err == nil || errors.Is(err, store.ErrMoved) != c.moved {
				t.Errorf("%s: Add = %v, want an error (ErrMoved: %t)", c.name, err, c.moved)
			}
		}
	})

	if o.Junk != nil {
		t.Run("junk is no entity", func(t *testing.T) {
			t.Parallel()
			ds := o.Open(t)
			add(t, ds, "issue", ID(1), Pack(1), "")
			o.Junk(t, ds)
			heads, err := ds.Heads(ctx, "issue")
			if err != nil || len(heads) != 1 || heads[0].ID != ID(1) {
				t.Fatalf("Heads = %v, %v; want %s alone", heads, err, ID(1))
			}
		})
	}

	t.Run("listed while written", func(t *testing.T) {
		t.Parallel()
		ds := o.Open(t)
		const n = 6
		versions := make([]string, n)
		for i := range n {
			versions[i] = add(t, ds, "issue", ID(i+1), Pack(1), "")
		}
		while(t, o, ds, func(ctx context.Context) error {
			// each entity appended to, again and again, as a writer does
			for c := uint64(2); ctx.Err() == nil; c++ {
				for i := range n {
					v, err := ds.Add(ctx, "issue", ID(i+1), Pack(c), versions[i])
					if err != nil {
						return stopped(ctx, err)
					}
					versions[i] = v
				}
			}
			return nil
		}, func() error {
			heads, err := ds.Heads(ctx, "issue")
			if err != nil {
				return err
			}
			for i := range n {
				if !slices.ContainsFunc(heads, func(h store.Head) bool { return h.ID == ID(i+1) }) {
					return fmt.Errorf("a listing missed %s, which existed throughout: %v", ID(i+1), heads)
				}
			}
			return nil
		})
	})

	t.Run("loaded while written", func(t *testing.T) {
		t.Parallel()
		ds := o.Open(t)
		v := add(t, ds, "issue", ID(1), Pack(1), "")
		while(t, o, ds, func(ctx context.Context) error {
			for c := uint64(2); ctx.Err() == nil; c++ {
				var err error
				if v, err = ds.Add(ctx, "issue", ID(1), Pack(c), v); err != nil {
					return stopped(ctx, err)
				}
			}
			return nil
		}, func() error {
			return ds.Load(ctx, []store.Ref{{Kind: "issue", ID: ID(1)}}, func(r store.Ref, packs [][]byte, err error) error {
				if err != nil {
					return err
				}
				// a load is whole: the packs of clocks 1 to n, each once
				clocks := make([]uint64, 0, len(packs))
				for _, p := range packs {
					c, _, err := pack.Header(p)
					if err != nil {
						return err
					}
					clocks = append(clocks, c)
				}
				slices.Sort(clocks)
				for i, c := range clocks {
					if c != uint64(i+1) {
						return fmt.Errorf("a load at %s read clocks %v: not whole", r.Version, clocks)
					}
				}
				return nil
			})
		})
	})
}

// while runs write and the datastore's churn, and read again and again
// meanwhile, for a second at most; read's first error fails the test.
func while(t *testing.T, o Options, ds store.Datastore, write func(context.Context) error, read func() error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	wg.Go(func() { errs <- write(ctx) })
	if o.Churn != nil {
		wg.Go(func() { errs <- o.Churn(ctx, ds) })
	}
	reads := 0
	for ctx.Err() == nil {
		if err := read(); err != nil {
			cancel()
			wg.Wait()
			t.Fatalf("after %d reads: %v", reads, err)
		}
		reads++
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil && !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	}
	if reads < 3 {
		t.Fatalf("only %d reads while written", reads)
	}
}

// stopped is err, unless ctx ended: a write cut short by the end of the
// test isn't a failure.
func stopped(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return nil
	}
	return err
}

func add(t *testing.T, ds store.Datastore, kind, id string, p []byte, prev string) string {
	t.Helper()
	v, err := ds.Add(ctx, kind, id, p, prev)
	if err != nil {
		t.Fatalf("Add %s %s: %v", kind, id, err)
	}
	return v
}
