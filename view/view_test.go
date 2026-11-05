package view_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/guilbep/git-board/kernel"
	"github.com/guilbep/git-board/store/mem"
	"github.com/guilbep/git-board/store/pg/pgtest"
	"github.com/guilbep/git-board/view"
	"github.com/guilbep/git-board/view/viewtest"
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

// The suite every view runs, on sqlite: each open of one file is
// another process's.
func TestSQLite(t *testing.T) {
	t.Parallel()
	viewtest.Run(t, viewtest.Options{Open: func(t *testing.T) func(t *testing.T) kernel.View {
		path := filepath.Join(t.TempDir(), "view.db")
		return func(t *testing.T) kernel.View {
			v, err := view.Open(ctx, path)
			if err != nil {
				t.Fatal(err)
			}
			return v
		}
	}})
}

// The suite on PostgreSQL: each open of one database is another
// machine's, its rows files in a folder of its own.
func TestPostgreSQL(t *testing.T) {
	t.Parallel()
	viewtest.Run(t, viewtest.Options{Open: func(t *testing.T) func(t *testing.T) kernel.View {
		dsn := pgtest.DSN(t)
		return func(t *testing.T) kernel.View {
			v, err := view.OpenPG(ctx, dsn, t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			return v
		}
	}})
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

// A view that opens but fails for another reason is kept: a cancelled
// open, a busy one (a server has it), or a newer binary's file, which is
// refused with a sentence. One of an older version is built again.
func TestKeptWhenNotUnreadable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "view.db")
	v, err := view.Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	v.Close()
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := view.Open(cancelled, path); err == nil {
		t.Fatal("Open(cancelled) = nil")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("after a cancelled open: %v", err)
	}
	setVersion := func(n int) {
		t.Helper()
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		if _, err := db.Exec("PRAGMA user_version = " + strconv.Itoa(n)); err != nil {
			t.Fatal(err)
		}
	}
	setVersion(99)
	if _, err := view.Open(ctx, path); err == nil || !strings.Contains(err.Error(), "newer") {
		t.Fatalf("a newer binary's view: %v; want it refused", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("after a newer binary's view: %v", err)
	}
	setVersion(1)
	v, err = view.Open(ctx, path)
	if err != nil {
		t.Fatalf("an older view: %v; want it built again", err)
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
