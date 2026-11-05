package kernel

import (
	"context"

	"github.com/guilbep/git-board/store"
)

// counted is a View that counts its calls and rows into the Counts of
// the call's context (store.Counts): what the complexity guards read.
type counted struct{ View }

func call(ctx context.Context) *store.Counts {
	c := store.CountsOf(ctx)
	if c != nil {
		c.ViewCalls.Add(1)
	}
	return c
}

func (v counted) Mark(ctx context.Context, kind string) (Mark, error) {
	call(ctx)
	return v.View.Mark(ctx, kind)
}

func (v counted) Heads(ctx context.Context, kind string) ([]Head, error) {
	hs, err := v.View.Heads(ctx, kind)
	if c := call(ctx); c != nil {
		c.RowsRead.Add(int64(len(hs)))
	}
	return hs, err
}

func (v counted) Write(ctx context.Context, kind string, w Write) error {
	if c := call(ctx); c != nil {
		c.RowsPut.Add(int64(len(w.Put) + len(w.Drop)))
	}
	return v.View.Write(ctx, kind, w)
}

func (v counted) Rows(ctx context.Context, kind string, q Query) ([]Row, error) {
	rs, err := v.View.Rows(ctx, kind, q)
	if c := call(ctx); c != nil {
		c.RowsRead.Add(int64(len(rs)))
	}
	return rs, err
}

func (v counted) Resolve(ctx context.Context, kind, prefix string) ([]ID, error) {
	call(ctx)
	return v.View.Resolve(ctx, kind, prefix)
}

func (v counted) Unsearchable(ctx context.Context, kind string) ([]Head, error) {
	hs, err := v.View.Unsearchable(ctx, kind)
	if c := call(ctx); c != nil {
		c.RowsRead.Add(int64(len(hs)))
	}
	return hs, err
}

func (v counted) Broken(ctx context.Context) ([]Broken, error) {
	call(ctx)
	return v.View.Broken(ctx)
}
