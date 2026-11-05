// Package viewtest is the suite every view runs, sqlite's and
// PostgreSQL's (docs/rewrite/design.md, Testing): each answers every
// query as the semantics say (queries.md), so alike; a refresh racing a
// write and another refresh never loses a row nor takes one back; a
// listing that missed an entity never drops it.
package viewtest

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guilbep/git-board/next/kernel"
	"github.com/guilbep/git-board/next/kernel/pack"
	"github.com/guilbep/git-board/next/kernel/search"
	"github.com/guilbep/git-board/next/store"
	"github.com/guilbep/git-board/next/store/mem"
)

// Options say how to run the suite on a view.
type Options struct {
	// Open gives a function that opens one view, again and again: each
	// open of it is another process's on the same view.
	Open func(t *testing.T) func(t *testing.T) kernel.View
}

var ctx = context.Background()

// Run runs the suite.
func Run(t *testing.T, o Options) {
	t.Run("answers", func(t *testing.T) { t.Parallel(); answers(t, o) })
	t.Run("refreshes racing writes", func(t *testing.T) { t.Parallel(); racing(t, o) })
	t.Run("a missed entity stays", func(t *testing.T) { t.Parallel(); missed(t, o) })
	t.Run("broken", func(t *testing.T) { t.Parallel(); broken(t, o) })
}

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

// note is a kind of texts and terms: each operation sets its fields.
type note struct {
	ID            kernel.ID
	Title, Body   string
	Terms         []string
	Created, Upd  int64
	Hidden, Burst bool
}

type noteRow struct{ Title string }

var notes = kernel.Define(kernel.Spec[note, noteRow]{
	Name: "note", Rev: 1,
	New: func(id kernel.ID) note { return note{ID: id} },
	Apply: func(n *note, e kernel.Entry) error {
		if e.Kind == "burst" {
			return fmt.Errorf("unknown operation %q", e.Kind)
		}
		var d struct {
			Title, Body string
			Terms       []string
			Hidden      bool
		}
		if err := json.Unmarshal(e.Data, &d); err != nil {
			return err
		}
		if n.Created == 0 {
			n.Created = e.Time
		}
		n.Upd = e.Time
		n.Title, n.Body, n.Terms, n.Hidden = d.Title, d.Body, d.Terms, d.Hidden
		return nil
	},
	Index: func(n *note) kernel.Doc[noteRow] {
		return kernel.Doc[noteRow]{Row: noteRow{n.Title}, Terms: n.Terms, Created: n.Created, Updated: n.Upd, Hidden: n.Hidden,
			Texts: []kernel.Text{{Field: "title", Body: n.Title}, {Field: "body", Body: n.Body}}}
	},
})

func open(t *testing.T, ds store.Datastore, v kernel.View, kinds ...kernel.Kind) *kernel.Board {
	t.Helper()
	b, err := kernel.Open(ctx, ds, v, kinds...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { v.Close() })
	return b
}

func op(kind string, at int64, data any) kernel.Op {
	b, _ := json.Marshal(data)
	return kernel.Op{Kind: kind, Author: "a", Time: at, Data: b}
}

// fixture is the notes the answers are asked of.
var fixture = []struct {
	title, body string
	terms       []string
	at          int64
	hidden      bool
}{
	{"Crème brûlée recipe", "sugar on top, burnt", []string{"is:open", "food"}, 300, false},
	{"Fix the login page", "users can't log in after the upgrade", []string{"is:open", "bug", "prio:p1"}, 100, false},
	{"Login timeouts", "timeouts on the login page; see the page log", []string{"is:closed", "bug", "prio:p2"}, 200, false},
	{"Upgrade Go", "go 1.26, then the linters", []string{"is:open", "chore", "prio:p1"}, 400, false},
	{"Deleted", "login gone", []string{"is:open", "bug"}, 500, true},
	{"Naïve façade", "日本語 text and émojis 🎉", []string{"is:open", "prio:p3"}, 250, false},
}

// answers: every query answered as the semantics say, by an answer
// computed here from the fixture.
func answers(t *testing.T, o Options) {
	ds := mem.New(t.TempDir())
	b := open(t, ds, o.Open(t)(t), notes)
	type fixed struct {
		id kernel.ID
		i  int
	}
	var all []fixed
	for i, f := range fixture {
		e, err := notes.Create(ctx, b, "", op("set", f.at, map[string]any{"Title": f.title, "Body": f.body, "Terms": f.terms, "Hidden": f.hidden}))
		if err != nil {
			t.Fatal(err)
		}
		all = append(all, fixed{e.ID, i})
	}
	// a second, fresh process answers from the same view
	b2 := open(t, ds, o.Open(t)(t), notes)
	for _, q := range []kernel.Query{
		{},
		{All: []string{"bug"}},
		{All: []string{"bug", "is:open"}},
		{Any: [][]string{{"prio:p1", "prio:p2"}}},
		{Any: [][]string{{"prio:p1", "prio:p2"}, {"is:closed"}}},
		{Not: []string{"bug"}},
		{All: []string{"nosuch"}},
		{Text: "login"},
		{Text: "log"},
		{Text: "login page"},
		{Text: "the page"},
		{Text: "page log", In: []string{"body"}},
		{Text: "login", In: []string{"title"}},
		{Text: "login", In: []string{"nowhere"}},
		{Text: "creme"},
		{Text: "BRÛL"},
		{Text: "fa"},
		{Text: "日本語"},
		{Text: "can't"},
		{Text: "can t"},
		{Text: "1.26"},
		{Text: "--- ..."},
		{Text: "login", All: []string{"is:open"}},
		{Sort: "created"},
		{Sort: "created", Desc: true},
		{Sort: "updated", Desc: true, Limit: 2},
		{Limit: 3},
	} {
		var want []kernel.ID
		for _, f := range all {
			x := fixture[f.i]
			if !x.hidden && holds(x.terms, q) && (q.Text == "" || textMatch(x.title, x.body, q)) {
				want = append(want, f.id)
			}
		}
		at := func(id kernel.ID) int64 {
			i := slices.IndexFunc(all, func(f fixed) bool { return f.id == id })
			return fixture[all[i].i].at
		}
		slices.SortFunc(want, func(a, b kernel.ID) int {
			if q.Sort != "" {
				return cmp.Or(cmp.Compare(at(a), at(b)), strings.Compare(string(a), string(b)))
			}
			return strings.Compare(string(a), string(b))
		})
		if q.Desc {
			slices.Reverse(want)
		}
		if q.Limit > 0 && len(want) > q.Limit {
			want = want[:q.Limit]
		}
		for _, board := range []*kernel.Board{b, b2} {
			ls, err := notes.List(ctx, board, q)
			if err != nil {
				t.Fatalf("%+v: %v", q, err)
			}
			got := make([]kernel.ID, len(ls))
			for i, l := range ls {
				got[i] = l.ID
			}
			if !slices.Equal(got, want) {
				t.Errorf("%+v: got %v, want %v", q, got, want)
			}
		}
	}
	// an id prefix: a hidden one isn't found
	for _, f := range all {
		id, err := notes.Resolve(ctx, b2, string(f.id[:6]))
		if fixture[f.i].hidden != (err != nil) || err == nil && id != f.id {
			t.Errorf("Resolve(%s) = %s, %v", f.id[:6], id, err)
		}
	}
}

func holds(terms []string, q kernel.Query) bool {
	for _, t := range q.All {
		if !slices.Contains(terms, t) {
			return false
		}
	}
	for _, g := range q.Any {
		if !slices.ContainsFunc(g, func(t string) bool { return slices.Contains(terms, t) }) {
			return false
		}
	}
	return !slices.ContainsFunc(q.Not, func(t string) bool { return slices.Contains(terms, t) })
}

func textMatch(title, body string, q kernel.Query) bool {
	var texts []string
	for _, f := range []struct{ name, body string }{{"title", title}, {"body", body}} {
		if len(q.In) == 0 || slices.Contains(q.In, f.name) {
			texts = append(texts, f.body)
		}
	}
	return len(texts) > 0 && search.Match(q.Text, texts...)
}

// racing: two processes refresh one view while a third writes: no
// refresh loses a row or takes one back to an older version, and once
// the writes end every row is its entity's fold at its head.
func racing(t *testing.T, o Options) {
	ds := mem.New(t.TempDir())
	opener := o.Open(t)
	const n = 8
	ids := make([]string, n)
	versions := make([]string, n)
	for i := range n {
		ids[i] = fmt.Sprintf("%040x", i+1)
		data, _ := pack.Encode(pack.Pack{Clock: 1, Ops: []kernel.Op{op("tick", 1, struct{}{})}})
		v, err := ds.Add(ctx, "counter", ids[i], data, "")
		if err != nil {
			t.Fatal(err)
		}
		versions[i] = v
	}
	readers := []*kernel.Board{open(t, ds, opener(t), counter), open(t, ds, opener(t), counter)}
	rctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	var wg sync.WaitGroup
	wg.Go(func() {
		for c := uint64(2); rctx.Err() == nil; c++ {
			for i := range n {
				data, _ := pack.Encode(pack.Pack{Clock: c, Ops: []kernel.Op{op("tick", int64(c), struct{}{})}})
				v, err := ds.Add(ctx, "counter", ids[i], data, versions[i])
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
			for rctx.Err() == nil {
				ls, err := counter.List(ctx, b, kernel.Query{})
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
	ls, err := counter.List(ctx, readers[0], kernel.Query{All: []string{"n:" + versions[0]}})
	if err != nil {
		t.Fatal(err)
	}
	if len(ls) == 0 {
		t.Fatal("after the writes, no row has its head's count")
	}
	for _, l := range ls {
		if string(l.Version) != versions[0] {
			t.Fatalf("after the writes, %s is at %s, want %s", l.ID, l.Version, versions[0])
		}
	}
}

// missing is a datastore whose listings miss an entity when told to, as
// git's once did (design.md, 2.6).
type missing struct {
	*mem.Store
	miss atomic.Value // string: the id a listing leaves out
}

func (m *missing) Heads(ctx context.Context, kinds ...string) ([]store.Head, error) {
	hs, err := m.Store.Heads(ctx, kinds...)
	id, _ := m.miss.Load().(string)
	return slices.DeleteFunc(hs, func(h store.Head) bool { return h.ID == id }), err
}

// missed: a listing that misses an entity costs a read, never the row.
func missed(t *testing.T, o Options) {
	ds := &missing{Store: mem.New(t.TempDir())}
	b := open(t, ds, o.Open(t)(t), counter)
	var ids []kernel.ID
	for range 3 {
		e, err := counter.Create(ctx, b, "", op("tick", 1, struct{}{}))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, e.ID)
	}
	if ls, _ := counter.List(ctx, b, kernel.Query{}); len(ls) != 3 {
		t.Fatalf("List = %v", ls)
	}
	ds.miss.Store(string(ids[1]))
	if _, err := counter.List(ctx, b, kernel.Query{}); err != nil {
		t.Fatal(err)
	}
	ds.miss.Store("")
	if ls, _ := counter.List(ctx, b, kernel.Query{}); len(ls) != 3 {
		t.Fatalf("after a listing missed one, List = %d rows, want 3", len(ls))
	}
}

// broken: an entity that doesn't fold is kept out of every answer, a
// search's too, and recorded.
func broken(t *testing.T, o Options) {
	ds := mem.New(t.TempDir())
	b := open(t, ds, o.Open(t)(t), notes)
	good, err := notes.Create(ctx, b, "", op("set", 1, map[string]any{"Title": "good"}))
	if err != nil {
		t.Fatal(err)
	}
	bad, err := notes.Create(ctx, b, "", op("set", 1, map[string]any{"Title": "bad soon"}))
	if err != nil {
		t.Fatal(err)
	}
	data, _ := pack.Encode(pack.Pack{Clock: 2, Ops: []kernel.Op{op("burst", 2, struct{}{})}})
	if _, err := ds.Add(ctx, "note", string(bad.ID), data, string(bad.Version)); err != nil {
		t.Fatal(err)
	}
	other := open(t, ds, o.Open(t)(t), notes)
	if ls, err := notes.List(ctx, other, kernel.Query{Text: "soon"}); err != nil || len(ls) != 0 {
		t.Fatalf("a search = %v, %v; want nothing", ls, err)
	}
	ls, err := notes.List(ctx, other, kernel.Query{})
	if err != nil || len(ls) != 1 || ls[0].ID != good.ID {
		t.Fatalf("List = %v, %v; want the good one alone", ls, err)
	}
	bs, err := other.Broken(ctx)
	if err != nil || len(bs) != 1 || bs[0].ID != bad.ID || !strings.Contains(bs[0].Err.Error(), "burst") {
		t.Fatalf("Broken = %v, %v", bs, err)
	}
}
