package pgstore_test

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/datastore/dstest"
	"github.com/guilbep/git-board/core/datastore/memstore"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/pgstore"
	"github.com/guilbep/git-board/core/pgstore/pgtest"
)

func open(t *testing.T) *pgstore.Store {
	t.Helper()
	s, err := pgstore.Open(t.Context(), pgtest.DSN(t), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestDatastore(t *testing.T) {
	t.Parallel()
	dstest.Run(t, func(t *testing.T) datastore.Datastore { return open(t) })
}

// Opening twice keeps the tables, and the board in them.
func TestOpenAgain(t *testing.T) {
	t.Parallel()
	dsn := pgtest.DSN(t)
	s, err := pgstore.Open(t.Context(), dsn, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	l, err := oplog.Create(t.Context(), s, "issue", "", []oplog.Op{{Kind: "create", Time: 1}})
	s.Close()
	if err != nil {
		t.Fatal(err)
	}
	again, err := pgstore.Open(t.Context(), dsn, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	got, err := oplog.Load(again, "issue", l.ID)
	if err != nil || len(got.Entries) != 1 || got.Entries[0].ID != l.Entries[0].ID {
		t.Fatalf("loaded %+v, %v; want %+v", got, err, l)
	}
}

// A board copied into PostgreSQL keeps every id: its entities', its
// operations', and so every reference to them.
func TestCopyKeepsIDs(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	from := memstore.New(t.TempDir())
	l, err := oplog.Create(ctx, from, "issue", "", []oplog.Op{{Kind: "create", Author: "a", Time: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Append(ctx, from, []oplog.Op{{Kind: "comment", Author: "a", Time: 2}}); err != nil {
		t.Fatal(err)
	}
	if _, err := from.SetCount(ctx, []byte("prefix CO2\nstart 1\n"), ""); err != nil {
		t.Fatal(err)
	}
	s := open(t)
	if n, err := s.Copy(ctx, from); err != nil || n != 1 {
		t.Fatalf("copied %d, %v; want 1", n, err)
	}
	if n, err := s.Copy(ctx, from); err != nil || n != 0 {
		t.Fatalf("copied again %d, %v; want 0", n, err)
	}
	got, err := oplog.Load(s, "issue", l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Entries) != 2 || got.Entries[0].ID != l.Entries[0].ID || got.Entries[1].ID != l.Entries[1].ID {
		t.Fatalf("entries %+v, want the ids of %+v", got.Entries, l.Entries)
	}
	if v, _, err := s.Count(ctx); err != nil || string(v) != "prefix CO2\nstart 1\n" {
		t.Fatalf("counter %q, %v", v, err)
	}
	// it goes on from there: a write lands on the copied version
	if err := got.Append(ctx, s, []oplog.Op{{Kind: "comment", Author: "a", Time: 3}}); err != nil {
		t.Fatal(err)
	}
}

// Several processes opening a fresh database's index at once, as the
// server, a command and the TUI starting together do, all get it.
func TestIndexDBAtOnce(t *testing.T) {
	t.Parallel()
	for range 4 {
		dsn := pgtest.DSN(t)
		var stores []*pgstore.Store
		for range 8 {
			s, err := pgstore.Open(t.Context(), dsn, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { s.Close() })
			stores = append(stores, s)
		}
		var wg sync.WaitGroup
		errs := make([]error, len(stores))
		for i, s := range stores {
			wg.Go(func() {
				db, err := s.IndexDB(t.Context())
				if err == nil {
					db.Close()
				}
				errs[i] = err
			})
		}
		wg.Wait()
		if err := errors.Join(errs...); err != nil {
			t.Fatal(err)
		}
	}
}
