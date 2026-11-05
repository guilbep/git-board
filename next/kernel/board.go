package kernel

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"

	"github.com/guilbep/git-board/next/kernel/pack"
	"github.com/guilbep/git-board/next/store"
)

// Board is an open board: a datastore and its view.
type Board struct {
	ds    store.Datastore
	view  View
	kinds map[string]kindSpec
}

// Open opens a board on ds, its view v, with its kinds.
func Open(ctx context.Context, ds store.Datastore, v View, kinds ...Kind) (*Board, error) {
	b := &Board{ds: ds, view: v, kinds: map[string]kindSpec{}}
	for _, k := range kinds {
		s := k.spec()
		if _, twice := b.kinds[s.name]; twice {
			return nil, fmt.Errorf("kernel: kind %q registered twice", s.name)
		}
		b.kinds[s.name] = s
	}
	return b, nil
}

// Store is the board's datastore.
func (b *Board) Store() store.Datastore { return b.ds }

// Close closes the view and the datastore.
func (b *Board) Close() error { return errors.Join(b.view.Close(), b.ds.Close()) }

// snapshotKey marks a context whose reads check freshness once per kind.
type snapshotKey struct{ b *Board }

// snapshot is the kinds already fresh in a Snapshot's context.
type snapshot struct {
	mu    sync.Mutex
	fresh map[string]bool
}

// Snapshot is ctx for one request or command: every read made with it
// brings each kind it reads to the datastore's heads once, at its first
// read, and answers from there. Without one, every read does.
func (b *Board) Snapshot(ctx context.Context) context.Context {
	return context.WithValue(ctx, snapshotKey{b}, &snapshot{fresh: map[string]bool{}})
}

// Fresh brings the view to the datastore's heads, for the kinds given or
// every kind: one listing of the heads when nothing moved
// (design.md, Freshness). An entity that fails to load is kept out and
// recorded (Broken); it never fails the call.
func (b *Board) Fresh(ctx context.Context, kinds ...Kind) error {
	names := make([]string, 0, len(kinds))
	for _, k := range kinds {
		names = append(names, k.Name())
	}
	if len(names) == 0 {
		for n := range b.kinds {
			names = append(names, n)
		}
		slices.Sort(names)
	}
	return b.fresh(ctx, names...)
}

func (b *Board) fresh(ctx context.Context, names ...string) error {
	snap, _ := ctx.Value(snapshotKey{b}).(*snapshot)
	if snap != nil {
		snap.mu.Lock()
		defer snap.mu.Unlock()
		names = slices.DeleteFunc(slices.Clone(names), func(n string) bool { return snap.fresh[n] })
		if len(names) == 0 {
			return nil
		}
	}
	for _, n := range names {
		if _, ok := b.kinds[n]; !ok {
			return fmt.Errorf("kernel: no kind %q on this board", n)
		}
	}
	heads, err := b.ds.Heads(ctx, names...)
	if err != nil {
		return fmt.Errorf("listing the board: %w", err)
	}
	for _, n := range names {
		var listing []Head
		for _, h := range heads {
			if h.Kind == n {
				listing = append(listing, Head{ID(h.ID), Version(h.Version)})
			}
		}
		if err := b.freshKind(ctx, b.kinds[n], listing); err != nil {
			return err
		}
		if snap != nil {
			snap.fresh[n] = true
		}
	}
	return nil
}

// freshKind brings one kind's rows to listing: equal to the mark, nothing
// to do; else the entities new or moved are loaded and folded, and one
// missing from the listing is dropped only once a point read says it is
// gone (a listing that missed it costs a read, never a lost issue).
func (b *Board) freshKind(ctx context.Context, k kindSpec, listing []Head) error {
	mark, err := b.view.Mark(ctx, k.name)
	if err != nil {
		return err
	}
	reset := mark.Rev != k.rev
	if !reset && mark.Sum == Sum(listing) {
		return nil
	}
	var base []Head
	if !reset {
		if base, err = b.view.Heads(ctx, k.name); err != nil {
			return err
		}
	}
	bases := make(map[ID]Version, len(base))
	for _, h := range base {
		bases[h.ID] = h.Version
	}
	var refs []store.Ref
	listed := make(map[ID]bool, len(listing))
	for _, h := range listing {
		listed[h.ID] = true
		if v, ok := bases[h.ID]; !ok || v != h.Version {
			refs = append(refs, store.Ref{Kind: k.name, ID: string(h.ID), Version: string(h.Version)})
		}
	}
	for _, h := range base {
		if !listed[h.ID] {
			refs = append(refs, store.Ref{Kind: k.name, ID: string(h.ID)}) // gone, or missed: read it
		}
	}
	w := Write{Rev: k.rev, Reset: reset, Texts: mark.Texts && !reset}
	var mu sync.Mutex
	err = b.ds.Load(ctx, refs, func(r store.Ref, packs [][]byte, err error) error {
		id, base := ID(r.ID), bases[ID(r.ID)]
		if errors.Is(err, store.ErrNotFound) && !listed[id] {
			mu.Lock()
			w.Drop = append(w.Drop, Head{id, base})
			mu.Unlock()
			return nil
		}
		p := b.put(k, id, base, Version(r.Version), packs, err, w.Texts)
		mu.Lock()
		w.Put = append(w.Put, p)
		mu.Unlock()
		return nil
	})
	if err != nil {
		return fmt.Errorf("loading the %s entities: %w", k.name, err)
	}
	return b.view.Write(ctx, k.name, w)
}

// put folds one entity's packs into its row, or records its error.
func (b *Board) put(k kindSpec, id ID, base, v Version, packs [][]byte, err error, texts bool) Put {
	p := Put{ID: id, Base: base, Version: v, Texted: texts}
	if err == nil {
		var es []Entry
		if es, _, err = order(packs); err == nil {
			p.Doc, err = k.fold(id, es)
		}
	}
	if err != nil {
		p.Err, p.Doc, p.Texted = err.Error(), Indexed{}, false
	} else if !texts {
		p.Doc.Texts = nil
	}
	return p
}

// texts brings a kind's texts in: from the first query that asks for
// text, every row keeps them (design.md: the free-text index is built by
// the first query that needs it, not by a first build).
func (b *Board) texts(ctx context.Context, k kindSpec) error {
	heads, err := b.view.Untexted(ctx, k.name)
	if err != nil || len(heads) == 0 {
		return err
	}
	refs := make([]store.Ref, len(heads))
	for i, h := range heads {
		refs[i] = store.Ref{Kind: k.name, ID: string(h.ID), Version: string(h.Version)}
	}
	w := Write{Rev: k.rev, Texts: true}
	var mu sync.Mutex
	err = b.ds.Load(ctx, refs, func(r store.Ref, packs [][]byte, err error) error {
		v := Version(r.Version)
		p := b.put(k, ID(r.ID), v, v, packs, err, true)
		mu.Lock()
		w.Put = append(w.Put, p)
		mu.Unlock()
		return nil
	})
	if err != nil {
		return fmt.Errorf("loading the %s entities' texts: %w", k.name, err)
	}
	return b.view.Write(ctx, k.name, w)
}

// Broken are the entities that fail to load, after bringing every kind
// to the heads.
func (b *Board) Broken(ctx context.Context) ([]Broken, error) {
	if err := b.Fresh(ctx); err != nil {
		return nil, err
	}
	return b.view.Broken(ctx)
}

// load reads one entity at its current version, and orders it.
func (b *Board) load(ctx context.Context, kind string, id ID) ([]Entry, Version, uint64, error) {
	if !store.IsID(string(id)) {
		return nil, "", 0, fmt.Errorf("%w: %s %s", ErrNotFound, kind, id)
	}
	var es []Entry
	var v Version
	var clock uint64
	err := b.ds.Load(ctx, []store.Ref{{Kind: kind, ID: string(id)}}, func(r store.Ref, packs [][]byte, err error) error {
		if errors.Is(err, store.ErrNotFound) {
			return fmt.Errorf("%w: %s %s", ErrNotFound, kind, id)
		}
		if err != nil {
			return err
		}
		v = Version(r.Version)
		es, clock, err = order(packs)
		return err
	})
	return es, v, clock, err
}

// tries is how many times a write is tried on an entity that moves.
const tries = 5

// write adds one pack of ops on the entity at prev, at clock, and
// answers the new version; ErrMoved when it moved meanwhile.
func (b *Board) write(ctx context.Context, kind string, id ID, prev Version, clock uint64, ops []Op) (Version, []Entry, error) {
	data, err := pack.Encode(pack.Pack{Clock: clock, Ops: ops})
	if err != nil {
		return "", nil, err
	}
	if id == "" {
		id = ID(pack.Hash(data))
	}
	v, err := b.ds.Add(ctx, kind, string(id), data, string(prev))
	if err != nil {
		return "", nil, err
	}
	es, _, err := order([][]byte{data})
	return Version(v), es, err
}
