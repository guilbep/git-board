// Package dstest holds every datastore to the contract of
// core/datastore.Datastore: the tests each datastore's own package runs.
package dstest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/guilbep/git-board/core/datastore"
)

// id is an entity id: 40 hex digits.
func id(n int) string { return fmt.Sprintf("%040x", n) }

// pack is a pack the git datastore takes: a clock, one operation.
func pack(clock int, kind string) []byte {
	return fmt.Appendf(nil, `{"clock":%d,"ops":[{"kind":%q,"time":1}]}`, clock, kind)
}

// Run checks the contract on fresh datastores from open.
func Run(t *testing.T, open func(t *testing.T) datastore.Datastore) {
	t.Run("empty", func(t *testing.T) {
		ds := open(t)
		ctx := t.Context()
		for _, kind := range datastore.Kinds {
			heads, err := ds.Heads(ctx, kind)
			if err != nil || len(heads) != 0 {
				t.Fatalf("heads %s = %v, %v; want none", kind, heads, err)
			}
		}
		if _, _, err := ds.Packs(ctx, "issue", id(1)); !errors.Is(err, datastore.ErrNotFound) {
			t.Fatalf("packs of none: %v, want ErrNotFound", err)
		}
		if ds.Folder() == "" {
			t.Fatal("no folder")
		}
	})

	t.Run("add and read", func(t *testing.T) {
		ds := open(t)
		ctx := t.Context()
		v1, err := ds.Add(ctx, "issue", id(1), pack(1, "create"), "")
		if err != nil || v1 == "" {
			t.Fatalf("create: %q, %v", v1, err)
		}
		if _, err := ds.Add(ctx, "issue", id(1), pack(1, "again"), ""); !errors.Is(err, datastore.ErrMoved) {
			t.Fatalf("create twice: %v, want ErrMoved", err)
		}
		v2, err := ds.Add(ctx, "issue", id(1), pack(2, "comment"), v1)
		if err != nil || v2 == v1 || v2 == "" {
			t.Fatalf("append: %q, %v (was %q)", v2, err, v1)
		}
		if _, err := ds.Add(ctx, "issue", id(1), pack(2, "stale"), v1); !errors.Is(err, datastore.ErrMoved) {
			t.Fatalf("append on a stale version: %v, want ErrMoved", err)
		}
		if _, err := ds.Add(ctx, "issue", id(2), pack(2, "orphan"), v1); err == nil {
			t.Fatal("append to an entity that doesn't exist: no error")
		}
		packs, v, err := ds.Packs(ctx, "issue", id(1))
		if err != nil || v != v2 {
			t.Fatalf("packs: version %q, %v; want %q", v, err, v2)
		}
		want := [][]byte{pack(1, "create"), pack(2, "comment")}
		slices.SortFunc(packs, bytes.Compare)
		slices.SortFunc(want, bytes.Compare)
		if !slices.EqualFunc(packs, want, bytes.Equal) {
			t.Fatalf("packs = %q, want %q", packs, want)
		}
		heads, err := ds.Heads(ctx, "issue")
		if err != nil || len(heads) != 1 || heads[id(1)] != v2 {
			t.Fatalf("heads = %v, %v; want %s at %s", heads, err, id(1), v2)
		}
		if other, err := ds.Heads(ctx, "person"); err != nil || len(other) != 0 {
			t.Fatalf("person heads = %v, %v; want none", other, err)
		}
	})

	t.Run("one writer wins", func(t *testing.T) {
		ds := open(t)
		ctx := t.Context()
		v1, err := ds.Add(ctx, "sync", id(3), pack(1, "create"), "")
		if err != nil {
			t.Fatal(err)
		}
		var wg sync.WaitGroup
		errs := make([]error, 8)
		for i := range errs {
			wg.Go(func() {
				_, errs[i] = ds.Add(context.WithoutCancel(ctx), "sync", id(3), pack(2, fmt.Sprint("w", i)), v1)
			})
		}
		wg.Wait()
		won := 0
		for _, err := range errs {
			switch {
			case err == nil:
				won++
			case !errors.Is(err, datastore.ErrMoved):
				t.Fatalf("a losing write: %v, want ErrMoved", err)
			}
		}
		packs, _, err := ds.Packs(ctx, "sync", id(3))
		if won != 1 || err != nil || len(packs) != 2 {
			t.Fatalf("%d writes won, %d packs, %v; want 1 and 2", won, len(packs), err)
		}
	})

	// A reader never sees an entity that exists gone, nor a pack half
	// written, while another writes it: the git datastore's listing once
	// lost a whole kind to the lock file of a ref being moved.
	t.Run("read while written", func(t *testing.T) {
		ds := open(t)
		ctx := t.Context()
		const writes = 20
		written := map[string]bool{}
		for c := 1; c <= writes+1; c++ {
			written[string(pack(c, "w"))] = true
		}
		v, err := ds.Add(ctx, "issue", id(4), pack(1, "w"), "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := ds.Add(ctx, "issue", id(5), pack(1, "w"), ""); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() {
			var err error
			for c := 2; c <= writes+1 && err == nil; c++ {
				v, err = ds.Add(ctx, "issue", id(4), pack(c, "w"), v)
			}
			done <- err
		}()
		var wg sync.WaitGroup
		stop := make(chan struct{})
		errs := make(chan error, 4)
		for range 4 {
			wg.Go(func() {
				for {
					select {
					case <-stop:
						return
					default:
					}
					if err := readBoth(ctx, ds, written); err != nil {
						errs <- err
						return
					}
				}
			})
		}
		werr := <-done
		close(stop)
		wg.Wait()
		close(errs)
		if werr != nil {
			t.Fatalf("write: %v", werr)
		}
		for err := range errs {
			t.Fatal(err)
		}
	})

	if _, ok := open(t).(datastore.Counter); !ok {
		return
	}
	t.Run("counter", func(t *testing.T) {
		c := open(t).(datastore.Counter)
		ctx := t.Context()
		if v, ver, err := c.Count(ctx); v != nil || ver != "" || err != nil {
			t.Fatalf("count of none: %q, %q, %v", v, ver, err)
		}
		v1, err := c.SetCount(ctx, []byte("a"), "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.SetCount(ctx, []byte("b"), ""); !errors.Is(err, datastore.ErrMoved) {
			t.Fatalf("start twice: %v, want ErrMoved", err)
		}
		v2, err := c.SetCount(ctx, []byte("c"), v1)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := c.SetCount(ctx, []byte("d"), v1); !errors.Is(err, datastore.ErrMoved) {
			t.Fatalf("stale: %v, want ErrMoved", err)
		}
		if v, ver, err := c.Count(ctx); string(v) != "c" || ver != v2 || err != nil {
			t.Fatalf("count = %q, %q, %v; want c at %q", v, ver, err, v2)
		}
	})
}

// readBoth lists the issues and reads id(4) and id(5): both must be there,
// with packs that are all whole ones of written.
func readBoth(ctx context.Context, ds datastore.Datastore, written map[string]bool) error {
	heads, err := ds.Heads(ctx, "issue")
	if err != nil {
		return fmt.Errorf("heads: %w", err)
	}
	for _, n := range []int{4, 5} {
		if _, ok := heads[id(n)]; !ok {
			return fmt.Errorf("heads lack %s, which exists: %v", id(n), heads)
		}
		packs, _, err := ds.Packs(ctx, "issue", id(n))
		if err != nil {
			return fmt.Errorf("packs of %s: %w", id(n), err)
		}
		for _, p := range packs {
			if !written[string(p)] {
				return fmt.Errorf("packs of %s: %q was never written", id(n), p)
			}
		}
	}
	return nil
}
