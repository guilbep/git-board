package people

import (
	"context"
	"errors"
	"testing"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/datastore/memstore"
)

// ctxStore answers a read with its context's error.
type ctxStore struct{ datastore.Datastore }

func (s ctxStore) Heads(ctx context.Context, kind string) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return s.Datastore.Heads(ctx, kind)
}

func (s ctxStore) Packs(ctx context.Context, kind, id string) ([][]byte, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	return s.Datastore.Packs(ctx, kind, id)
}

func TestReadsTakeTheCallersContext(t *testing.T) {
	t.Parallel()
	ds := ctxStore{memstore.New(t.TempDir())}
	p, err := Create(context.Background(), ds, NewCreate("", "Ada", ""))
	if err != nil {
		t.Fatal(err)
	}
	gone, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := LoadContext(gone, ds, p.ID); !errors.Is(err, context.Canceled) {
		t.Errorf("LoadContext: %v, want context.Canceled", err)
	}
	if _, err := ListContext(gone, ds); !errors.Is(err, context.Canceled) {
		t.Errorf("ListContext: %v, want context.Canceled", err)
	}
	if _, err := Do(gone, ds, p.ID, SetName("", "Ada L.")); !errors.Is(err, context.Canceled) {
		t.Errorf("Do: %v, want context.Canceled", err)
	}
	if got, err := Load(ds, p.ID); err != nil || got.Name != "Ada" {
		t.Errorf("Load: %v, %v", got, err)
	}
}
