package oplog

import (
	"context"
	"errors"
	"testing"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/datastore/memstore"
)

// ctxStore answers a read with its context's error, as a datastore over
// a network does once its caller has gone.
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

// LoadContext and ListContext hand their caller's context to the
// datastore; Load and List read with none.
func TestReadsTakeTheCallersContext(t *testing.T) {
	t.Parallel()
	ds := ctxStore{memstore.New(t.TempDir())}
	l, err := Create(context.Background(), ds, "issue", "", []Op{op("create")})
	if err != nil {
		t.Fatal(err)
	}
	gone, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := LoadContext(gone, ds, "issue", l.ID); !errors.Is(err, context.Canceled) {
		t.Errorf("LoadContext, its caller gone: %v, want context.Canceled", err)
	}
	if _, err := ListContext(gone, ds, "issue"); !errors.Is(err, context.Canceled) {
		t.Errorf("ListContext, its caller gone: %v, want context.Canceled", err)
	}
	if got, err := Load(ds, "issue", l.ID); err != nil || got.Version != l.Version {
		t.Errorf("Load: %v, %v", got, err)
	}
	if ids, err := List(ds, "issue"); err != nil || len(ids) != 1 {
		t.Errorf("List: %v, %v", ids, err)
	}
	// Append reads again a log it didn't read itself, with its own context
	if err := (&Log{Kind: "issue", ID: l.ID}).Append(gone, ds, []Op{op("comment")}); !errors.Is(err, context.Canceled) {
		t.Errorf("Append, its caller gone: %v, want context.Canceled", err)
	}
}
