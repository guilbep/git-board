package kernel_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/guilbep/git-board/next/kernel"
	"github.com/guilbep/git-board/next/kernel/pack"
	"github.com/guilbep/git-board/next/store"
	"github.com/guilbep/git-board/next/store/mem"
	"github.com/guilbep/git-board/next/view"
)

var ctx = context.Background()

// note is a kind made for the tests: its operations add lines, a "hide"
// hides it, any other kind is an error.
type note struct {
	ID     kernel.ID
	Lines  []string
	Hidden bool
}

type noteRow struct{ Lines int }

func newKind(rev int) *kernel.K[note, noteRow] {
	return kernel.Define(kernel.Spec[note, noteRow]{
		Name: "note", Rev: rev,
		New: func(id kernel.ID) note { return note{ID: id} },
		Apply: func(n *note, e kernel.Entry) error {
			switch e.Kind {
			case "line":
				var d struct{ Text string }
				if err := json.Unmarshal(e.Data, &d); err != nil {
					return err
				}
				n.Lines = append(n.Lines, d.Text)
			case "hide":
				n.Hidden = true
			default:
				return fmt.Errorf("unknown operation %q", e.Kind)
			}
			return nil
		},
		Index: func(n *note) kernel.Doc[noteRow] {
			d := kernel.Doc[noteRow]{Row: noteRow{len(n.Lines)}, Hidden: n.Hidden, Texts: []kernel.Text{{Field: "body", Body: strings.Join(n.Lines, "\n")}}}
			for _, l := range n.Lines {
				d.Terms = append(d.Terms, "line:"+l)
			}
			if len(n.Lines) > 0 {
				d.Created = int64(len(n.Lines[0]))
			}
			return d
		},
	})
}

var notes = newKind(1)

func line(text string) kernel.Op {
	return kernel.Op{Kind: "line", Author: "a", Time: 1, Data: json.RawMessage(fmt.Sprintf(`{"text":%q}`, text))}
}

// open is a board on ds with a view in dir.
func open(t *testing.T, ds store.Datastore, dir string, kinds ...kernel.Kind) *kernel.Board {
	t.Helper()
	v, err := view.Open(ctx, filepath.Join(dir, "view.db"))
	if err != nil {
		t.Fatal(err)
	}
	if len(kinds) == 0 {
		kinds = []kernel.Kind{notes}
	}
	b, err := kernel.Open(ctx, ds, v, kinds...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { v.Close() })
	return b
}

// add writes a pack on an entity behind the board's back, as another
// process does.
func add(t *testing.T, ds store.Datastore, id string, clock uint64, prev string, ops ...kernel.Op) string {
	t.Helper()
	b, err := pack.Encode(pack.Pack{Clock: clock, Ops: ops})
	if err != nil {
		t.Fatal(err)
	}
	if id == "" {
		id = pack.Hash(b)
	}
	v, err := ds.Add(ctx, "note", id, b, prev)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func lines(t *testing.T, b *kernel.Board, q kernel.Query) map[kernel.ID]int {
	t.Helper()
	ls, err := notes.List(ctx, b, q)
	if err != nil {
		t.Fatal(err)
	}
	m := map[kernel.ID]int{}
	for _, l := range ls {
		m[l.ID] = l.Row.Lines
	}
	return m
}

func TestCreateDoGetLog(t *testing.T) {
	t.Parallel()
	ds := mem.New(t.TempDir())
	b := open(t, ds, t.TempDir())
	e, err := notes.Create(ctx, b, "", line("one"))
	if err != nil {
		t.Fatal(err)
	}
	if e.Clock != 1 || len(e.State.Lines) != 1 || !store.IsID(string(e.ID)) {
		t.Fatalf("Create = %+v", e)
	}
	// another writer moves it between Do's read and its write: Do decides again
	other := true
	e2, err := notes.Do(ctx, b, e.ID, func(cur kernel.Entity[note]) ([]kernel.Op, error) {
		if other {
			other = false
			add(t, ds, string(e.ID), cur.Clock+1, string(cur.Version), line("theirs"))
		}
		return []kernel.Op{line(fmt.Sprintf("after %d", len(cur.State.Lines)))}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"one", "theirs", "after 2"}; !slices.Equal(e2.State.Lines, want) || e2.Clock != 3 {
		t.Fatalf("Do = %v at clock %d, want %v at 3", e2.State.Lines, e2.Clock, want)
	}
	got, err := notes.Get(ctx, b, e.ID)
	if err != nil || !slices.Equal(got.State.Lines, e2.State.Lines) || got.Version != e2.Version {
		t.Fatalf("Get = %+v, %v; want Do's", got, err)
	}
	log, v, err := notes.Log(ctx, b, e.ID)
	if err != nil || len(log) != 3 || v != e2.Version || log[0].ID.Pack() != e.ID {
		t.Fatalf("Log = %v at %s, %v", log, v, err)
	}
	// write-through: the view has Do's row, no listing needed
	if m := lines(t, b, kernel.Query{}); m[e.ID] != 3 {
		t.Fatalf("List = %v, want 3 lines", m)
	}
	if _, err := notes.Get(ctx, b, kernel.ID(strings.Repeat("f", 40))); !errors.Is(err, kernel.ErrNotFound) {
		t.Fatalf("Get of nothing = %v, want ErrNotFound", err)
	}
	if _, err := notes.Do(ctx, b, e.ID, func(kernel.Entity[note]) ([]kernel.Op, error) { return nil, nil }); err != nil {
		t.Fatalf("Do of nothing = %v", err)
	}
}

// A list answers what others wrote: the view follows the datastore's
// heads, whoever moved them, and drops what is gone.
func TestFresh(t *testing.T) {
	t.Parallel()
	ds := mem.New(t.TempDir())
	dir := t.TempDir()
	b := open(t, ds, dir)
	v1 := add(t, ds, "", 1, "", line("a"))
	heads, _ := ds.Heads(ctx, "note")
	id := kernel.ID(heads[0].ID)
	if m := lines(t, b, kernel.Query{}); m[id] != 1 {
		t.Fatalf("List = %v", m)
	}
	add(t, ds, string(id), 2, v1, line("b"))
	if m := lines(t, b, kernel.Query{}); m[id] != 2 {
		t.Fatalf("after another's write, List = %v, want 2 lines", m)
	}
	// a second board on the same view file (another process) agrees
	b2 := open(t, ds, dir)
	if m := lines(t, b2, kernel.Query{}); m[id] != 2 {
		t.Fatalf("another process lists %v", m)
	}
}

// missing is a datastore whose listings miss an entity, as git's once
// did while it moved refs (fbda0b3).
type missing struct {
	store.Datastore
	mu   sync.Mutex
	miss string
}

func (m *missing) Heads(ctx context.Context, kinds ...string) ([]store.Head, error) {
	heads, err := m.Datastore.Heads(ctx, kinds...)
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.DeleteFunc(heads, func(h store.Head) bool { return h.ID == m.miss }), err
}

// A listing that misses an entity costs a read, never the entity: it is
// dropped only once a point read says it's gone (design.md, 2.6).
func TestMissedListingKeepsTheEntity(t *testing.T) {
	t.Parallel()
	ds := &missing{Datastore: mem.New(t.TempDir())}
	b := open(t, ds, t.TempDir())
	add(t, ds, "", 1, "", line("a"))
	add(t, ds, "", 1, "", line("b"))
	if m := lines(t, b, kernel.Query{}); len(m) != 2 {
		t.Fatalf("List = %v", m)
	}
	heads, _ := ds.Heads(ctx, "note")
	ds.miss = heads[0].ID
	if m := lines(t, b, kernel.Query{}); len(m) != 2 {
		t.Fatalf("a listing missed one: List = %v, want both still", m)
	}
}

// An entity that fails to fold is broken: no list answers it, Broken
// names it, and no query fails because of it; its next version that
// folds is listed again.
func TestBroken(t *testing.T) {
	t.Parallel()
	ds := mem.New(t.TempDir())
	b := open(t, ds, t.TempDir())
	add(t, ds, "", 1, "", line("fine"))
	v := add(t, ds, "", 1, "", line("soon broken"), kernel.Op{Kind: "frobnicate", Author: "a", Time: 1})
	if m := lines(t, b, kernel.Query{}); len(m) != 1 {
		t.Fatalf("List = %v, want the fine one alone", m)
	}
	broken, err := b.Broken(ctx)
	if err != nil || len(broken) != 1 || !strings.Contains(broken[0].Err.Error(), "frobnicate") {
		t.Fatalf("Broken = %v, %v", broken, err)
	}
	heads, _ := ds.Heads(ctx, "note")
	for _, h := range heads {
		if h.Version == v {
			// a fold that fails stays failed: the log keeps the operation
			add(t, ds, h.ID, 2, v, line("more"))
		}
	}
	if broken, _ := b.Broken(ctx); len(broken) != 1 {
		t.Fatalf("Broken after a new version = %v: the same log still fails", broken)
	}
}

// A kind whose rev changed is folded again, whole.
func TestRevRebuilds(t *testing.T) {
	t.Parallel()
	ds := mem.New(t.TempDir())
	dir := t.TempDir()
	add(t, ds, "", 1, "", line("a"))
	if m := lines(t, open(t, ds, dir), kernel.Query{}); len(m) != 1 {
		t.Fatal(m)
	}
	notes2 := newKind(2)
	b2 := open(t, ds, dir, notes2)
	ls, err := notes2.List(ctx, b2, kernel.Query{})
	if err != nil || len(ls) != 1 {
		t.Fatalf("after a new rev, List = %v, %v", ls, err)
	}
	// the old rev's process writes nothing over the new rows
	b1 := open(t, ds, dir)
	if _, err := notes.Create(ctx, b1, "", line("old")); err != nil {
		t.Fatal(err)
	}
	if ls, _ := notes2.List(ctx, b2, kernel.Query{}); len(ls) != 2 {
		t.Fatalf("List = %v, want both, folded by rev 2", ls)
	}
}

// Queries: terms, text (its index built by the first query asking for
// text), hidden rows, sort and limit, prefixes.
func TestQueries(t *testing.T) {
	t.Parallel()
	ds := mem.New(t.TempDir())
	b := open(t, ds, t.TempDir())
	ids := map[string]kernel.ID{}
	for _, ls := range [][]string{{"crash", "on save"}, {"café crème"}, {"nothing here at all"}, {"hidden crash"}} {
		ops := []kernel.Op{}
		for _, l := range ls {
			ops = append(ops, line(l))
		}
		e, err := notes.Create(ctx, b, "", ops...)
		if err != nil {
			t.Fatal(err)
		}
		ids[ls[0]] = e.ID
	}
	if _, err := notes.Do(ctx, b, ids["hidden crash"], func(kernel.Entity[note]) ([]kernel.Op, error) {
		return []kernel.Op{{Kind: "hide", Author: "a", Time: 2}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	match := func(q kernel.Query) []kernel.ID {
		got, err := notes.Match(ctx, b, q)
		if err != nil {
			t.Fatal(err)
		}
		slices.Sort(got)
		return got
	}
	sorted := func(ids ...kernel.ID) []kernel.ID { slices.Sort(ids); return ids }
	for name, c := range map[string]struct {
		q    kernel.Query
		want []kernel.ID
	}{
		"all but hidden": {kernel.Query{}, sorted(ids["crash"], ids["café crème"], ids["nothing here at all"])},
		"a term":         {kernel.Query{All: []string{"line:crash"}}, []kernel.ID{ids["crash"]}},
		"any":            {kernel.Query{Any: [][]string{{"line:crash", "line:café crème"}}}, sorted(ids["crash"], ids["café crème"])},
		"not":            {kernel.Query{Not: []string{"line:crash"}}, sorted(ids["café crème"], ids["nothing here at all"])},
		"text":           {kernel.Query{Text: "cras"}, []kernel.ID{ids["crash"]}},
		"diacritics":     {kernel.Query{Text: "cafe"}, []kernel.ID{ids["café crème"]}},
		"words":          {kernel.Query{Text: "on crash"}, []kernel.ID{ids["crash"]}},
		"syntax is text": {kernel.Query{Text: `"crash" OR NOT*`}, nil},
		"no token":       {kernel.Query{Text: "-"}, sorted(ids["crash"], ids["café crème"], ids["nothing here at all"])},
		"in nowhere":     {kernel.Query{Text: "crash", In: []string{"title"}}, nil},
		"in body":        {kernel.Query{Text: "crash", In: []string{"body"}}, []kernel.ID{ids["crash"]}},
	} {
		if got := match(c.q); !slices.Equal(got, c.want) {
			t.Errorf("%s: Match = %v, want %v", name, got, c.want)
		}
	}
	// created is the first line's length in bytes here: crash 5, café crème 12, nothing here at all 19
	ls, err := notes.List(ctx, b, kernel.Query{Sort: "created", Desc: true, Limit: 2})
	if err != nil || len(ls) != 2 || ls[0].ID != ids["nothing here at all"] || ls[1].ID != ids["café crème"] {
		t.Fatalf("sorted, limited = %v, %v", ls, err)
	}
	if _, err := notes.List(ctx, b, kernel.Query{Sort: "nope"}); err == nil {
		t.Fatal("a sort by nothing: nil error")
	}
	add(t, ds, "ab"+strings.Repeat("0", 38), 1, "", line("x"))
	add(t, ds, "ab"+strings.Repeat("0", 37)+"1", 1, "", line("y"))
	if _, err := notes.Resolve(ctx, b, "ab0"); !errors.Is(err, kernel.ErrAmbiguous) {
		t.Errorf("Resolve of a prefix of two = %v, want ErrAmbiguous", err)
	}
	if got, err := notes.Resolve(ctx, b, "ab"+strings.Repeat("0", 38)); err != nil || got != kernel.ID("ab"+strings.Repeat("0", 38)) {
		t.Errorf("Resolve of a whole id = %s, %v: a whole id is never ambiguous", got, err)
	}
	one := ids["crash"]
	for prefix, want := range map[string]error{string(one[:6]): nil, string(one): nil, "": kernel.ErrNotFound, "XYZ": kernel.ErrNotFound,
		string(ids["hidden crash"]): kernel.ErrNotFound} {
		got, err := notes.Resolve(ctx, b, prefix)
		if !errors.Is(err, want) || err == nil && got != one {
			t.Errorf("Resolve(%q) = %s, %v; want %v", prefix, got, err, want)
		}
	}
}

// An observer is told of each write this process made, once it landed:
// a write that writes nothing, and another process's, tell it nothing.
func TestObserve(t *testing.T) {
	t.Parallel()
	ds := mem.New(t.TempDir())
	b := open(t, ds, t.TempDir())
	var got []kernel.Change
	b.Observe(func(_ context.Context, c []kernel.Change) { got = append(got, c...) })
	e, err := notes.Create(ctx, b, "", line("one"))
	if err != nil {
		t.Fatal(err)
	}
	e2, err := notes.Do(ctx, b, e.ID, func(kernel.Entity[note]) ([]kernel.Op, error) { return []kernel.Op{line("two")}, nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := notes.Do(ctx, b, e.ID, func(kernel.Entity[note]) ([]kernel.Op, error) { return nil, nil }); err != nil {
		t.Fatal(err)
	}
	add(t, ds, string(e.ID), 3, string(e2.Version), line("theirs"))
	want := []kernel.Change{{Kind: "note", ID: e.ID, Version: e.Version}, {Kind: "note", ID: e.ID, Version: e2.Version}}
	if !slices.Equal(got, want) {
		t.Fatalf("observed %v, want %v", got, want)
	}
}
