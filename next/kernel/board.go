package kernel

import (
	"context"
	"errors"
	"fmt"
	"runtime"
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

	mu        sync.Mutex
	observers []func(ctx context.Context, c []Change)
	// freshening is held while a kind's rows are brought to the heads,
	// and indexing while its search index is built: a read meanwhile
	// waits for the one running rather than doing its work twice (a
	// first build, a server's background index)
	freshening, indexing sync.Mutex
}

// Open opens a board on ds, its view v, with its kinds.
func Open(ctx context.Context, ds store.Datastore, v View, kinds ...Kind) (*Board, error) {
	b := &Board{ds: ds, view: counted{v}, kinds: map[string]kindSpec{}}
	for _, k := range kinds {
		s := k.spec()
		if _, twice := b.kinds[s.name]; twice {
			return nil, fmt.Errorf("kernel: kind %q registered twice", s.name)
		}
		b.kinds[s.name] = s
	}
	return b, nil
}

// Observe registers f, told of each change this process wrote, once it
// landed: the Markdown render, the forge-only sender (design.md, The
// write path, 6). f runs on the writer's goroutine, after the view took
// the change.
func (b *Board) Observe(f func(ctx context.Context, c []Change)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.observers = append(b.observers, f)
}

// landed tells the observers of a change written.
func (b *Board) landed(ctx context.Context, c ...Change) {
	b.mu.Lock()
	fs := slices.Clip(b.observers)
	b.mu.Unlock()
	for _, f := range fs {
		f(ctx, c)
	}
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
// read, and answers from there. Without one, every read does. A context
// that is one already stays as it is: a command's reads and writes share
// it.
func (b *Board) Snapshot(ctx context.Context) context.Context {
	if _, ok := ctx.Value(snapshotKey{b}).(*snapshot); ok {
		return ctx
	}
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
	return b.fresh(ctx, false, names...)
}

// fresh brings the named kinds to the heads; search: the query asks for
// the search index, so entities read now are indexed as they are read.
func (b *Board) fresh(ctx context.Context, search bool, names ...string) error {
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
	// Each kind's mark is read before the listing: a view read after it
	// may hold a write made since, which the listing would take back
	// (another process's write-through). A kind whose view moved meanwhile
	// is listed again.
	for try := 1; len(names) > 0; try++ {
		marks := make(map[string]Mark, len(names))
		for _, n := range names {
			m, err := b.view.Mark(ctx, n)
			if err != nil {
				return err
			}
			marks[n] = m
		}
		heads, err := b.ds.Heads(ctx, names...)
		if err != nil {
			return fmt.Errorf("listing the board: %w", err)
		}
		var again []string
		for _, n := range names {
			var listing []Head
			for _, h := range heads {
				if h.Kind == n {
					listing = append(listing, Head{ID(h.ID), Version(h.Version)})
				}
			}
			moved, err := b.freshKind(ctx, b.kinds[n], listing, marks[n], search)
			if err != nil {
				return err
			}
			if moved {
				again = append(again, n)
			} else if snap != nil {
				snap.fresh[n] = true
			}
		}
		if try == tries {
			// writers keep the view moving, and their writes through it
			// with it: the next query lists again
			return nil
		}
		names = again
	}
	return nil
}

// freshKind brings one kind's rows to listing: equal to the mark, nothing
// to do; else the entities new or moved are loaded and folded, and one
// missing from the listing is dropped only once a point read says it is
// gone (a listing that missed it costs a read, never a lost issue). before
// is the kind's mark as read before the listing: when the view moved
// since, nothing is written and moved says to list again.
func (b *Board) freshKind(ctx context.Context, k kindSpec, listing []Head, before Mark, search bool) (moved bool, err error) {
	b.freshening.Lock()
	defer b.freshening.Unlock()
	mark, err := b.view.Mark(ctx, k.name)
	if err != nil {
		return false, err
	}
	reset := mark.Rev != k.rev
	if !reset && mark.Sum == Sum(listing) {
		return false, nil
	}
	if mark != before {
		return true, nil
	}
	var base []Head
	if !reset {
		if base, err = b.view.Heads(ctx, k.name); err != nil {
			return false, err
		}
		if mark, err = b.view.Mark(ctx, k.name); err != nil {
			return false, err
		} else if mark != before {
			return true, nil // a write between the two reads: base may be newer than the listing
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
	search = search || mark.Search && !reset
	return false, b.stream(ctx, k, Write{Rev: k.rev, Reset: reset, Search: search}, refs, func(r store.Ref, packs [][]byte, err error) (Put, *Head) {
		id, base := ID(r.ID), bases[ID(r.ID)]
		if errors.Is(err, store.ErrNotFound) && !listed[id] {
			return Put{}, &Head{id, base}
		}
		return b.put(k, id, base, Version(r.Version), packs, err, search), nil
	})
}

// batch is how many entities a transaction of a refresh writes: a first
// build writes while it reads.
const batch = 2048

// stream loads refs, makes each a put or a drop, and writes them in
// batches of w's kind as they come, the first batch with w's reset: the
// view is written while the datastore is read. Each batch is a
// transaction, each row conditional on its base: a refresh cut short
// leaves rows another refresh completes.
func (b *Board) stream(ctx context.Context, k kindSpec, w Write, refs []store.Ref, each func(store.Ref, [][]byte, error) (Put, *Head)) error {
	type change struct {
		put  Put
		drop *Head
	}
	changes := make(chan change, batch)
	written := make(chan error, 1)
	go func() {
		var err error
		cur := w
		flush := func() {
			if err == nil {
				err = b.view.Write(ctx, k.name, cur)
			}
			cur = Write{Rev: w.Rev, Search: w.Search}
		}
		for c := range changes {
			if c.drop != nil {
				cur.Drop = append(cur.Drop, *c.drop)
			} else {
				cur.Put = append(cur.Put, c.put)
			}
			if len(cur.Put)+len(cur.Drop) == batch {
				flush()
			}
		}
		if len(cur.Put)+len(cur.Drop) > 0 || cur.Reset || len(refs) == 0 {
			flush()
		}
		written <- err
	}()
	err := b.load(ctx, refs, func(r store.Ref, packs [][]byte, err error) {
		p, d := each(r, packs, err)
		changes <- change{p, d}
	})
	close(changes)
	if werr := <-written; err == nil && werr != nil {
		return werr
	}
	if err != nil {
		return fmt.Errorf("loading the %s entities: %w", k.name, err)
	}
	return nil
}

// load reads refs and hands each to fold on a pool of its own: the
// datastore reads on while entities fold (git's cat-file waits for no
// fold), on every CPU.
func (b *Board) load(ctx context.Context, refs []store.Ref, fold func(store.Ref, [][]byte, error)) error {
	type loaded struct {
		r     store.Ref
		packs [][]byte
		err   error
	}
	ch := make(chan loaded, 256)
	var wg sync.WaitGroup
	for range runtime.GOMAXPROCS(0) {
		wg.Go(func() {
			for l := range ch {
				fold(l.r, l.packs, l.err)
			}
		})
	}
	err := b.ds.Load(ctx, refs, func(r store.Ref, packs [][]byte, err error) error {
		ch <- loaded{r, packs, err}
		return nil
	})
	close(ch)
	wg.Wait()
	return err
}

// put folds one entity's packs into its row, or records its error; with
// search, its terms and texts too.
func (b *Board) put(k kindSpec, id ID, base, v Version, packs [][]byte, err error, search bool) Put {
	p := Put{ID: id, Base: base, Version: v, Search: search}
	if err == nil {
		var es []Entry
		if es, _, err = order(packs); err == nil {
			p.Doc, err = k.fold(id, es)
		} else {
			err = named(k.name, id, err)
		}
	}
	if err != nil {
		p.Err, p.Doc, p.Search = err.Error(), Indexed{}, false
	} else if !search {
		p.Doc.Terms, p.Doc.Texts = nil, nil
	}
	return p
}

// searchable brings in a kind's search index, its terms and texts: the
// first query that searches builds it, not a first build (design.md:
// the free-text index is indexed by the first query that asks for it).
func (b *Board) searchable(ctx context.Context, k kindSpec) error {
	b.indexing.Lock()
	defer b.indexing.Unlock()
	heads, err := b.view.Unsearchable(ctx, k.name)
	if err != nil || len(heads) == 0 {
		return err
	}
	refs := make([]store.Ref, len(heads))
	bases := make(map[ID]Version, len(heads))
	for i, h := range heads {
		refs[i] = store.Ref{Kind: k.name, ID: string(h.ID), Version: string(h.Version)}
		bases[h.ID] = h.Version
	}
	// a datastore that isn't Pinned loads the current version, which may
	// have moved from the row's: the row is replaced where it is still at
	// its base
	return b.stream(ctx, k, Write{Rev: k.rev, Search: true}, refs, func(r store.Ref, packs [][]byte, err error) (Put, *Head) {
		return b.put(k, ID(r.ID), bases[ID(r.ID)], Version(r.Version), packs, err, true), nil
	})
}

// Index builds the kinds' search index now, rather than on the first
// query that searches: a server does it in the background once it
// serves (design.md: the free-text index is indexed after the rows, in
// the server's background, or by the first query that asks for it). A
// query that searches meanwhile waits for it.
func (b *Board) Index(ctx context.Context, kinds ...Kind) error {
	for _, k := range kinds {
		// the rows first, as a list builds them: its first page waits for
		// them alone
		if err := b.fresh(ctx, false, k.Name()); err != nil {
			return err
		}
		if err := b.searchable(ctx, b.kinds[k.Name()]); err != nil {
			return err
		}
	}
	return nil
}

// Broken are the entities that fail to load, after bringing every kind
// to the heads.
func (b *Board) Broken(ctx context.Context) ([]Broken, error) {
	if err := b.Fresh(ctx); err != nil {
		return nil, err
	}
	return b.view.Broken(ctx)
}

// entity reads one entity at its current version, and orders it.
func (b *Board) entity(ctx context.Context, kind string, id ID) ([]Entry, Version, uint64, error) {
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
		return named(kind, id, err)
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

// named is an error reading an entity's packs, as v1 words it: "oplog:
// <its ref>: …". Broken entities' lines are a contract (the golden
// boards' stderr).
func named(kind string, id ID, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("oplog: refs/board/%s/%s: %w", kind, id, err)
}
