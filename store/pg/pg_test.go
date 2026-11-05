package pg

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/guilbep/git-board/store"
	"github.com/guilbep/git-board/store/pg/pgtest"
	"github.com/guilbep/git-board/store/storetest"
)

var ctx = context.Background()

func open(t *testing.T, dsn string) *Store {
	t.Helper()
	s, err := Open(ctx, dsn, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestConformance(t *testing.T) {
	t.Parallel()
	storetest.Run(t, storetest.Options{
		Open: func(t *testing.T) store.Datastore { return open(t, pgtest.DSN(t)) },
		Churn: func(ctx context.Context, ds store.Datastore) error {
			v := ""
			for c := uint64(1); ctx.Err() == nil; c++ {
				var err error
				if v, err = ds.Add(ctx, "person", storetest.ID(100), storetest.Pack(c), v); err != nil && ctx.Err() == nil {
					return err
				}
			}
			return nil
		},
		Junk: func(t *testing.T, ds store.Datastore) {
			if _, err := ds.(*Store).pool.Exec(ctx, "INSERT INTO git_board.entities VALUES ('issue', 'notes', 1)"); err != nil {
				t.Fatal(err)
			}
		},
	})
}

// Two first uses at once make the tables once; a database of another
// version is refused.
func TestSetup(t *testing.T) {
	t.Parallel()
	dsn := pgtest.DSN(t)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			s, err := Open(ctx, dsn, t.TempDir())
			if err != nil {
				t.Error(err)
				return
			}
			s.Close()
		})
	}
	wg.Wait()
	s := open(t, dsn)
	if _, err := s.pool.Exec(ctx, "UPDATE git_board.meta SET value = '2'"); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, dsn, t.TempDir()); err == nil || !strings.Contains(err.Error(), "the tables are version 2, this git-board reads 1") {
		t.Fatalf("Open = %v", err)
	}
}

// The counter, one row; the lock, held by one session at a time.
func TestCounterAndLock(t *testing.T) {
	t.Parallel()
	s := open(t, pgtest.DSN(t))
	if v, _, err := s.Count(ctx); v != nil || err != nil {
		t.Fatalf("Count = %q, %v", v, err)
	}
	v1, err := s.SetCount(ctx, []byte("prefix PG\nstart 1\n"), "")
	if err != nil || v1 != "1" {
		t.Fatalf("SetCount = %q, %v", v1, err)
	}
	if _, err := s.SetCount(ctx, []byte("x"), ""); !errors.Is(err, store.ErrMoved) {
		t.Fatalf("a second first counter: %v", err)
	}
	if v2, err := s.SetCount(ctx, []byte("prefix PG\nstart 2\n"), v1); err != nil || v2 != "2" {
		t.Fatalf("SetCount = %q, %v", v2, err)
	}
	unlock, err := s.Lock(ctx, "sync")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Lock(ctx, "sync"); !errors.Is(err, store.ErrHeld) {
		t.Fatalf("a lock held: %v", err)
	}
	unlock()
	again, err := s.Lock(ctx, "sync")
	if err != nil {
		t.Fatal(err)
	}
	again()
}

func TestRedact(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"postgres://me:secret@host/db":         "postgres://me:xxxxx@host/db",
		"bad postgresql://u:p@h:5/d?x=1 there": "bad postgresql://u:xxxxx@h:5/d?x=1 there",
		"host=h password=secret user=u":        "host=h password=xxxxx user=u",
		"postgres://me@host/db":                "postgres://me@host/db",
	} {
		if got := Redact(in); got != want {
			t.Errorf("Redact(%q) = %q, want %q", in, got, want)
		}
	}
}
