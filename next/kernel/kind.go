package kernel

import (
	"cmp"
	"context"
	"errors"
	"fmt"

	"github.com/guilbep/git-board/next/store"
)

// Get is the entity's state at its current version. A hidden one too:
// what a tombstone hides is its domain's to say.
func (k *K[S, R]) Get(ctx context.Context, b *Board, id ID) (Entity[S], error) {
	es, v, clock, err := b.entity(ctx, k.s.Name, id)
	if err != nil {
		return Entity[S]{}, err
	}
	st, err := k.fold(id, es)
	return Entity[S]{ID: id, Version: v, Clock: clock, State: st}, err
}

// Log is every operation in order, and the version they are at: log,
// show, undo, diff.
func (k *K[S, R]) Log(ctx context.Context, b *Board, id ID) ([]Entry, Version, error) {
	es, v, _, err := b.entity(ctx, k.s.Name, id)
	return es, v, err
}

// Logs reads every entity of the kind at the datastore's heads, and
// gives each log to each, from several goroutines at once: what an
// answer as the board stood then reads (ls --at), where the view holds
// now's.
func (k *K[S, R]) Logs(ctx context.Context, b *Board, each func(id ID, log []Entry, err error)) error {
	heads, err := b.ds.Heads(ctx, k.s.Name)
	if err != nil {
		return err
	}
	refs := make([]store.Ref, len(heads))
	for i, h := range heads {
		refs[i] = store.Ref(h)
	}
	return b.load(ctx, refs, func(r store.Ref, packs [][]byte, err error) {
		var es []Entry
		if err == nil {
			es, _, err = order(packs)
			err = named(k.s.Name, ID(r.ID), err)
		}
		each(ID(r.ID), es, err)
	})
}

// Fold is the state entries add up to: a log read with Log, or part of
// one (a domain's "as it was then").
func (k *K[S, R]) Fold(id ID, entries []Entry) (S, error) { return k.fold(id, entries) }

// List answers the visible rows matching q, fresh.
func (k *K[S, R]) List(ctx context.Context, b *Board, q Query) ([]Listed[R], error) {
	rows, err := k.rows(ctx, b, q)
	if err != nil {
		return nil, err
	}
	out := make([]Listed[R], len(rows))
	for i, r := range rows {
		out[i] = Listed[R]{ID: r.ID, Version: r.Version}
		if err := k.codec.Decode(r.Row, &out[i].Row); err != nil {
			return nil, fmt.Errorf("%s %s: its row: %w", k.s.Name, r.ID, err)
		}
	}
	return out, nil
}

// Match answers the ids of the visible rows matching q, fresh, without
// decoding their rows: a free text's hits.
func (k *K[S, R]) Match(ctx context.Context, b *Board, q Query) ([]ID, error) {
	rows, err := k.rows(ctx, b, q)
	if err != nil {
		return nil, err
	}
	out := make([]ID, len(rows))
	for i, r := range rows {
		out[i] = r.ID
	}
	return out, nil
}

func (k *K[S, R]) rows(ctx context.Context, b *Board, q Query) ([]Row, error) {
	if q.Limit < 0 {
		return nil, fmt.Errorf("a limit of %d: a count, 0 for all", q.Limit)
	}
	search := q.Text != "" || len(q.All)+len(q.Any)+len(q.Not) > 0
	if err := b.fresh(ctx, search, k.s.Name); err != nil {
		return nil, err
	}
	if search {
		if err := b.searchable(ctx, b.kinds[k.s.Name]); err != nil {
			return nil, err
		}
	}
	return b.view.Rows(ctx, k.s.Name, q)
}

// Resolve is the whole id of the visible entity a prefix names: 1 to 40
// lowercase hex digits, matching one entity (queries.md, Prefixes).
func (k *K[S, R]) Resolve(ctx context.Context, b *Board, prefix string) (ID, error) {
	if !isPrefix(prefix) {
		return "", fmt.Errorf("%w: %s %s: an id is 1 to 40 lowercase hex digits", ErrNotFound, k.s.Name, prefix)
	}
	if err := b.fresh(ctx, false, k.s.Name); err != nil {
		return "", err
	}
	ids, err := b.view.Resolve(ctx, k.s.Name, prefix)
	switch {
	case err != nil:
		return "", err
	case len(ids) == 0:
		return "", fmt.Errorf("%w: %s %s", ErrNotFound, k.s.Name, prefix)
	case len(ids) > 1 && string(ids[0]) != prefix:
		return "", fmt.Errorf("%w: %s %s", ErrAmbiguous, k.s.Name, prefix)
	}
	return ids[0], nil
}

func isPrefix(p string) bool {
	if len(p) == 0 || len(p) > 40 {
		return false
	}
	for i := 0; i < len(p); i++ {
		if c := p[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// Create writes a new entity's first pack. id is "" unless the kind is
// Given. The view takes the new row at once (write-through).
func (k *K[S, R]) Create(ctx context.Context, b *Board, id ID, ops ...Op) (Entity[S], error) {
	if (id != "") != k.s.Given {
		return Entity[S]{}, fmt.Errorf("kernel: a %s's id is given: %t", k.s.Name, k.s.Given)
	}
	if len(ops) == 0 {
		return Entity[S]{}, errors.New("kernel: an entity starts with an operation")
	}
	// the first pack's ids name its entity: "pending" stands for both
	if _, err := k.fold(cmp.Or(id, "pending"), pending(nil, ops)); err != nil {
		return Entity[S]{}, err // refused before anything is written
	}
	v, es, err := b.write(ctx, k.s.Name, id, "", 1, ops)
	if err != nil {
		return Entity[S]{}, fmt.Errorf("creating a %s: %w", k.s.Name, err)
	}
	if id == "" {
		id = es[0].ID.Pack()
	}
	st, err := k.fold(id, es)
	if err != nil {
		return Entity[S]{}, err
	}
	e := Entity[S]{ID: id, Version: v, Clock: 1, State: st}
	err = k.through(ctx, b, e, "")
	b.landed(ctx, Change{Kind: k.s.Name, ID: id, Version: v})
	return e, err
}

// Do reads the entity, asks decide for the operations, and writes them in
// one pack. When the entity moved meanwhile it reads again and asks again
// (5 tries): a decision always sees the head it is written on. No
// operation writes nothing.
func (k *K[S, R]) Do(ctx context.Context, b *Board, id ID, decide func(cur Entity[S]) ([]Op, error)) (Entity[S], error) {
	for try := 1; ; try++ {
		es, v, clock, err := b.entity(ctx, k.s.Name, id)
		if err != nil {
			return Entity[S]{}, err
		}
		st, err := k.fold(id, es)
		if err != nil {
			return Entity[S]{}, err
		}
		cur := Entity[S]{ID: id, Version: v, Clock: clock, State: st}
		if err := check(ctx, k.s.Name, id, v); err != nil {
			return cur, err
		}
		ops, err := decide(cur)
		if err != nil || len(ops) == 0 {
			return cur, err
		}
		if _, err := k.fold(id, pending(es, ops)); err != nil {
			return cur, err // refused before anything is written
		}
		nv, added, err := b.write(ctx, k.s.Name, id, v, clock+1, ops)
		if errors.Is(err, store.ErrMoved) && try < tries {
			continue
		}
		if err != nil {
			return cur, fmt.Errorf("writing %s %s: %w", k.s.Name, id, err)
		}
		// the new pack sorts last: its clock is the highest (storage.md,
		// Updating a ref)
		for _, e := range added {
			if err := k.s.Apply(&st, e); err != nil {
				return cur, err
			}
		}
		next := Entity[S]{ID: id, Version: nv, Clock: clock + 1, State: st}
		moved(ctx, id, nv)
		err = k.through(ctx, b, next, v)
		b.landed(ctx, Change{Kind: k.s.Name, ID: id, Version: nv})
		return next, err
	}
}

// pending is entries followed by ops not written yet, with ids that name
// no operation ("pending:<n>"): a fold of it says whether they apply. A
// fold that refused one once it was written would leave the entity
// unreadable on every clone (ADR 0004).
func pending(entries []Entry, ops []Op) []Entry {
	out := append(make([]Entry, 0, len(entries)+len(ops)), entries...)
	for i, op := range ops {
		out = append(out, Entry{ID: OpID(fmt.Sprintf("pending:%d", i)), Op: op})
	}
	return out
}

// through replaces the entity's row in the view, where it is still at
// base: a process reads its own writes without a listing.
func (k *K[S, R]) through(ctx context.Context, b *Board, e Entity[S], base Version) error {
	d := k.s.Index(&e.State)
	p := Put{ID: e.ID, Base: base, Version: e.Version, Search: true, Doc: Indexed{
		Row: k.codec.Append(nil, &d.Row), Terms: d.Terms, Texts: d.Texts,
		Created: d.Created, Updated: d.Updated, Hidden: d.Hidden}}
	return b.view.Write(ctx, k.s.Name, Write{Rev: k.s.Rev, Put: []Put{p}})
}
