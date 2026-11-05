package view_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/guilbep/git-board/next/kernel"
	"github.com/guilbep/git-board/next/kernel/pack"
	"github.com/guilbep/git-board/next/store/mem"
	"github.com/guilbep/git-board/next/view"
)

var ctx = context.Background()

type count struct{ N int }

// counter is a kind whose state counts its operations.
var counter = kernel.Define(kernel.Spec[count, count]{
	Name: "counter", Rev: 1,
	New:   func(kernel.ID) count { return count{} },
	Apply: func(c *count, _ kernel.Entry) error { c.N++; return nil },
	Index: func(c *count) kernel.Doc[count] {
		return kernel.Doc[count]{Row: *c, Terms: []string{"n:" + strconv.Itoa(c.N)}}
	},
})

func openBoard(t *testing.T, ds *mem.Store, path string) *kernel.Board {
	t.Helper()
	v, err := view.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	b, err := kernel.Open(ctx, ds, v, counter)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	return b
}

func op(n int) kernel.Op {
	return kernel.Op{Kind: "tick", Author: "a", Time: int64(n), Data: json.RawMessage(`{}`)}
}

// Two processes refresh one view file while a third writes: no refresh
// loses a row or takes one back to an older version, and once the writes
// end every row is its entity's fold at its head.
func TestRefreshesRacingWrites(t *testing.T) {
	t.Parallel()
	ds := mem.New(t.TempDir())
	path := filepath.Join(t.TempDir(), "view.db")
	const n = 8
	ids := make([]string, n)
	versions := make([]string, n)
	for i := range n {
		ids[i] = fmt.Sprintf("%040x", i+1)
		data, _ := pack.Encode(pack.Pack{Clock: 1, Ops: []kernel.Op{op(1)}})
		v, err := ds.Add(ctx, "counter", ids[i], data, "")
		if err != nil {
			t.Fatal(err)
		}
		versions[i] = v
	}
	readers := []*kernel.Board{openBoard(t, ds, path), openBoard(t, ds, path)}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var wg sync.WaitGroup
	wg.Go(func() {
		for c := uint64(2); ctx.Err() == nil; c++ {
			for i := range n {
				data, _ := pack.Encode(pack.Pack{Clock: c, Ops: []kernel.Op{op(int(c))}})
				v, err := ds.Add(context.Background(), "counter", ids[i], data, versions[i])
				if err != nil {
					t.Error(err)
					return
				}
				versions[i] = v
			}
		}
	})
	for _, b := range readers {
		wg.Go(func() {
			last := map[kernel.ID]int{}
			for ctx.Err() == nil {
				ls, err := counter.List(context.Background(), b, kernel.Query{})
				if err != nil {
					t.Error(err)
					return
				}
				if len(ls) != n {
					t.Errorf("a list of %d rows, want %d", len(ls), n)
					return
				}
				for _, l := range ls {
					if v, _ := strconv.Atoi(string(l.Version)); l.Row.N != v {
						t.Errorf("%s at %s holds %d operations: a row isn't its version's fold", l.ID, l.Version, l.Row.N)
					}
					if l.Row.N < last[l.ID] {
						t.Errorf("%s went back from %d to %d", l.ID, last[l.ID], l.Row.N)
					}
					last[l.ID] = l.Row.N
				}
			}
		})
	}
	wg.Wait()
	ls, err := counter.List(context.Background(), readers[0], kernel.Query{All: []string{"n:" + versions[0]}})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range ls {
		if string(l.Version) != versions[0] {
			t.Fatalf("after the writes, %s is at %s, want %s", l.ID, l.Version, versions[0])
		}
	}
	if len(ls) == 0 {
		t.Fatal("after the writes, no row has its head's count")
	}
}

// A file of another version, or not a database at all, is built again.
func TestRebuiltWhenUnreadable(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "view.db")
	if err := os.WriteFile(path, []byte("not a database, nor anything sqlite reads"), 0o644); err != nil {
		t.Fatal(err)
	}
	v, err := view.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if m, err := v.Mark(ctx, "counter"); err != nil || m.Rev != 0 {
		t.Fatalf("Mark = %+v, %v", m, err)
	}
	v.Close()
}

// The rows file is a copy of the rows at a mark: one at another mark,
// or cut short, is read from the table again.
func TestRowsFileFollowsTheMark(t *testing.T) {
	t.Parallel()
	ds := mem.New(t.TempDir())
	dir := t.TempDir()
	b := openBoard(t, ds, filepath.Join(dir, "view.db"))
	e, err := counter.Create(ctx, b, "", op(1))
	if err != nil {
		t.Fatal(err)
	}
	if ls, _ := counter.List(ctx, b, kernel.Query{}); len(ls) != 1 || ls[0].Row.N != 1 {
		t.Fatalf("List = %v", ls)
	}
	if _, err := counter.Do(ctx, b, e.ID, func(kernel.Entity[count]) ([]kernel.Op, error) { return []kernel.Op{op(2)}, nil }); err != nil {
		t.Fatal(err)
	}
	// another process reads the file left at the first mark
	other := openBoard(t, ds, filepath.Join(dir, "view.db"))
	if ls, _ := counter.List(ctx, other, kernel.Query{}); len(ls) != 1 || ls[0].Row.N != 2 {
		t.Fatalf("List = %v, want the row at 2 operations", ls)
	}
	f := filepath.Join(dir, "rows-counter")
	data, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f, data[:len(data)-1], 0o644); err != nil {
		t.Fatal(err)
	}
	third := openBoard(t, ds, filepath.Join(dir, "view.db"))
	if ls, _ := counter.List(ctx, third, kernel.Query{}); len(ls) != 1 || ls[0].Row.N != 2 {
		t.Fatalf("with a file cut short, List = %v", ls)
	}
}

// A write appends its rows to the patch file: another process lists
// through it; a patch cut short, it reads the table; past the records a
// list keeps, it writes one rows file again.
func TestRowsPatches(t *testing.T) {
	t.Parallel()
	ds := mem.New(t.TempDir())
	dir := t.TempDir()
	b := openBoard(t, ds, filepath.Join(dir, "view.db"))
	var ids []kernel.ID
	for i := range 3 {
		e, err := counter.Create(ctx, b, "", op(i+1))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, e.ID)
	}
	if ls, _ := counter.List(ctx, b, kernel.Query{}); len(ls) != 3 {
		t.Fatalf("List = %v", ls)
	}
	bump := func(id kernel.ID, n int) {
		t.Helper()
		if _, err := counter.Do(ctx, b, id, func(kernel.Entity[count]) ([]kernel.Op, error) { return []kernel.Op{op(n)}, nil }); err != nil {
			t.Fatal(err)
		}
	}
	bump(ids[0], 10)
	bump(ids[2], 20)
	if _, err := counter.Create(ctx, b, "", op(7)); err != nil {
		t.Fatal(err)
	}
	total := func(v *kernel.Board) (n, sum int) {
		t.Helper()
		ls, err := counter.List(ctx, v, kernel.Query{})
		if err != nil {
			t.Fatal(err)
		}
		for _, l := range ls {
			sum += l.Row.N
		}
		return len(ls), sum
	}
	if n, sum := total(openBoard(t, ds, filepath.Join(dir, "view.db"))); n != 4 || sum != 6 {
		t.Fatalf("through the patches: %d rows, sum %d", n, sum)
	}
	patches := filepath.Join(dir, "rows-counter.patch")
	data, err := os.ReadFile(patches)
	if err != nil || len(data) == 0 {
		t.Fatalf("no patch file: %v", err)
	}
	if err := os.WriteFile(patches, data[:len(data)-3], 0o644); err != nil {
		t.Fatal(err)
	}
	if n, sum := total(openBoard(t, ds, filepath.Join(dir, "view.db"))); n != 4 || sum != 6 {
		t.Fatalf("a patch cut short: %d rows, sum %d", n, sum)
	}
	for i := range 250 {
		bump(ids[1], 100+i)
	}
	if n, sum := total(openBoard(t, ds, filepath.Join(dir, "view.db"))); n != 4 || sum != 6+250 {
		t.Fatalf("after 250 patches: %d rows, sum %d", n, sum)
	}
	if _, err := os.Stat(patches); !os.IsNotExist(err) {
		t.Errorf("past the records a list keeps, the patch file stays: %v", err)
	}
}
