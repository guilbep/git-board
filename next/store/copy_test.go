package store_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/guilbep/git-board/next/kernel/pack"
	"github.com/guilbep/git-board/next/store"
	"github.com/guilbep/git-board/next/store/git"
	"github.com/guilbep/git-board/next/store/md"
	"github.com/guilbep/git-board/next/store/pg"
	"github.com/guilbep/git-board/next/store/pg/pgtest"
	"github.com/guilbep/git-board/next/store/storetest"
)

var ctx = context.Background()

func gitStore(t *testing.T) *git.Store {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	s, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func mdStore(t *testing.T) *md.Store {
	t.Helper()
	s, err := md.Open(filepath.Join(t.TempDir(), "issues"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func op(clock uint64, text string) []byte {
	b, err := pack.Encode(pack.Pack{Clock: clock, Ops: []pack.Op{{Kind: "comment", Author: storetest.ID(0), Time: int64(clock), Data: []byte(`{"text":"` + text + `"}`)}}})
	if err != nil {
		panic(err)
	}
	return b
}

// order is an entity's operations' ids, in order: what a reader makes of
// its packs, the same in every datastore.
func order(t *testing.T, ds store.Datastore, kind, id string) []string {
	t.Helper()
	var ids []string
	err := ds.Load(ctx, []store.Ref{{Kind: kind, ID: id}}, func(_ store.Ref, packs [][]byte, err error) error {
		if err != nil {
			return err
		}
		es, _, err := pack.Order(packs)
		for _, e := range es {
			ids = append(ids, e.ID)
		}
		return err
	})
	if err != nil {
		t.Fatalf("%s %s: %v", kind, id, err)
	}
	return ids
}

// A board copies from any datastore into any other that takes packs as
// they are, its ids and its order kept, its counter too; a second copy
// writes nothing. The first is a Markdown folder after a union merge:
// two packs of one clock.
func TestCopy(t *testing.T) {
	t.Parallel()
	src := mdStore(t)
	one, two := storetest.ID(1), storetest.ID(2)
	v, err := src.Add(ctx, "issue", one, op(1, "first"), "")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(src.Dir(), md.Board, "issue", one+".jsonl")
	data, _ := os.ReadFile(path)
	data = append(append(append(data, op(2, "ours")...), '\n'), append(op(2, "theirs"), '\n')...) // union's
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if v, err = src.Add(ctx, "issue", two, op(1, "another"), ""); err != nil {
		t.Fatal(err)
	}
	if _, err := src.Add(ctx, "person", storetest.ID(3), op(1, "a person"), ""); err != nil {
		t.Fatal(err)
	}
	counter := []byte("prefix MD\nstart 1\n1 " + one + "\n")
	if _, err := src.SetCount(ctx, counter, ""); err != nil {
		t.Fatal(err)
	}
	chain := []store.Datastore{src, gitStore(t)}
	if os.Getenv(pgtest.Env) != "" {
		p, err := pg.Open(ctx, pgtest.DSN(t), t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { p.Close() })
		chain = append(chain, p)
	}
	chain = append(chain, mdStore(t))
	kinds := []string{"issue", "person"}
	for i := 1; i < len(chain); i++ {
		from, to := chain[i-1], chain[i]
		c, err := store.Copy(ctx, from, to, kinds...)
		if err != nil || c.Entities != 3 || !c.Counter {
			t.Fatalf("copy %d: %+v, %v; want 3 entities and the counter", i, c, err)
		}
		if c, err := store.Copy(ctx, from, to, kinds...); err != nil || c.Entities != 0 || c.Counter {
			t.Fatalf("copy %d again: %+v, %v; want nothing", i, c, err)
		}
		for _, id := range []string{one, two} {
			if a, b := order(t, src, "issue", id), order(t, to, "issue", id); !slices.Equal(a, b) {
				t.Fatalf("copy %d: %s is %v, want %v", i, id, b, a)
			}
		}
		if got, _, _ := to.Caps().Counter.Count(ctx); string(got) != string(counter) {
			t.Fatalf("copy %d: counter %q", i, got)
		}
	}
	// a pack added since: the next copy takes it alone
	if _, err := src.Add(ctx, "issue", two, op(2, "later"), v); err != nil {
		t.Fatal(err)
	}
	g := chain[1]
	if c, err := store.Copy(ctx, src, g, kinds...); err != nil || c.Entities != 1 {
		t.Fatalf("copy of one more pack: %+v, %v", c, err)
	}
	if a, b := order(t, src, "issue", two), order(t, g, "issue", two); !slices.Equal(a, b) {
		t.Fatalf("after one more pack: %v, want %v", b, a)
	}
}
