package oplog

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/guilbep/git-board/core/gitstore"
)

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	return dir
}

func open(t *testing.T, dir string) *gitstore.Store {
	t.Helper()
	s, err := gitstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func newStore(t *testing.T) *gitstore.Store { return open(t, newRepo(t)) }

func op(kind string) Op { return Op{Kind: kind, Author: "a", Time: 1} }

// badIDs aren't entity ids. None is an id written here in another case:
// on a case-insensitive file system, two such refs would be one file.
var badIDs = []string{
	"", "a b", "a/b",
	"3F7A9C2E41B6D08A5E1F93C7B2D4A6E8F0C1B3D5", // uppercase
	"3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d",  // 39 digits
}

// badRefs are names git takes for a ref, but not entity ids.
var badRefs = append([]string{"notes"}, badIDs[2:]...)

// swapUpdateRef has oplog move refs with f until the test ends. A test that
// calls it can't be parallel.
func swapUpdateRef(t *testing.T, f func(s *gitstore.Store, ctx context.Context, name string, next, prev gitstore.Hash) error) {
	orig := updateRef
	updateRef = f
	t.Cleanup(func() { updateRef = orig })
}

func create(t *testing.T, s *gitstore.Store, kind, id string, ops ...Op) *Log {
	t.Helper()
	l, err := Create(t.Context(), s, kind, id, ops)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func load(t *testing.T, s *gitstore.Store, kind, id string) *Log {
	t.Helper()
	l, err := Load(s, kind, id)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

func appendOps(t *testing.T, s *gitstore.Store, l *Log, ops ...Op) {
	t.Helper()
	if err := l.Append(t.Context(), s, ops); err != nil {
		t.Fatal(err)
	}
}

func opKinds(l *Log) []string {
	var out []string
	for _, e := range l.Entries {
		out = append(out, e.Kind)
	}
	return out
}

func clocks(l *Log) []uint64 {
	var out []uint64
	for _, e := range l.Entries {
		out = append(out, e.Clock)
	}
	return out
}

// commitOf writes a commit with gitstore directly, a blob per file.
func commitOf(t *testing.T, s *gitstore.Store, files map[string]string, parents ...gitstore.Hash) gitstore.Hash {
	t.Helper()
	var entries []gitstore.TreeEntry
	for name, data := range files {
		b, err := s.WriteBlob([]byte(data))
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, gitstore.TreeEntry{Name: name, Blob: b})
	}
	tree, err := s.WriteTree(entries)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.WriteCommit(gitstore.Commit{Tree: tree, Parents: parents})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func setRef(t *testing.T, s *gitstore.Store, name string, next, prev gitstore.Hash) {
	t.Helper()
	if err := s.UpdateRef(context.Background(), name, next, prev); err != nil {
		t.Fatal(err)
	}
}

// packBlob returns a commit's pack blob, the tree's only entry.
func packBlob(t *testing.T, s *gitstore.Store, c gitstore.Hash) gitstore.Hash {
	t.Helper()
	cm, err := s.ReadCommit(c)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := s.ReadTree(cm.Tree)
	if err != nil || len(entries) != 1 || entries[0].Name != "pack" {
		t.Fatalf("tree = %v, %v", entries, err)
	}
	return entries[0].Blob
}

// packOf returns a commit's pack hash, in hex.
func packOf(t *testing.T, s *gitstore.Store, c gitstore.Hash) string {
	t.Helper()
	return packBlob(t, s, c).String()
}

func readPack(t *testing.T, s *gitstore.Store, c gitstore.Hash) string {
	t.Helper()
	data, err := s.ReadBlob(packBlob(t, s, c))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCreateLoad(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	if _, err := Create(t.Context(), s, "issue", "", nil); err == nil {
		t.Fatal("create with no operation: no error")
	}
	ops := []Op{
		{Kind: "create", Author: "guilbep", Time: 1792834200, Data: json.RawMessage(`{"title":"t","body":"b"}`)},
		{Kind: "comment", Author: "forge:octocat", Time: 1792836300, Origin: &Origin{Forge: "github.com/o/r", Event: "comment:1"}},
	}
	l := create(t, s, "issue", "", ops...)
	h := packOf(t, s, l.Head)
	if l.ID != h {
		t.Fatalf("id = %s, want the pack's hash %s", l.ID, h)
	}
	got := load(t, s, "issue", l.ID)
	want := []Entry{{ops[0], h + ":0", 1}, {ops[1], h + ":1", 1}}
	if got.Kind != "issue" || got.ID != l.ID || got.Head != l.Head || !reflect.DeepEqual(got.Entries, want) {
		t.Fatalf("log = %+v\nwant entries %+v", got, want)
	}
	if p := readPack(t, s, l.Head); !strings.HasPrefix(p, `{"clock":1,"nonce":"`) {
		t.Fatalf("pack = %s", p)
	}
}

func TestCreateIDs(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a := create(t, s, "issue", "", op("create"))
	b := create(t, s, "issue", "", op("create"))
	if a.ID == b.ID {
		t.Fatalf("same ops gave one id: %s", a.ID)
	}
	const id = "56f7a8e819f90e968812bac54ae4b06142d8e735"
	c := create(t, s, "sync", id, op("link"))
	if c.ID != id || RefName("sync", id) != "refs/board/sync/"+id {
		t.Fatalf("id = %s, ref %s", c.ID, RefName("sync", id))
	}
	if h, _ := s.Ref("refs/board/sync/" + id); h != c.Head {
		t.Fatalf("ref = %v, want %v", h, c.Head)
	}
	if c.Entries[0].ID != packOf(t, s, c.Head)+":0" {
		t.Fatalf("op id = %s", c.Entries[0].ID)
	}
	if _, err := Create(t.Context(), s, "sync", id, []Op{op("link")}); err == nil {
		t.Fatal("second create of one id: no error")
	}
}

func TestList(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	var want []string
	var head gitstore.Hash
	for range 3 {
		l := create(t, s, "issue", "", op("create"))
		want, head = append(want, l.ID), l.Head
	}
	slices.Sort(want)
	create(t, s, "sync", "56f7a8e819f90e968812bac54ae4b06142d8e735", op("link"))
	// Refs whose names aren't ids aren't entities: listing ignores them.
	for _, name := range badRefs {
		setRef(t, s, "refs/board/issue/"+name, head, gitstore.ZeroHash)
	}
	got, err := List(s, "issue")
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("list = %v, %v; want %v", got, err, want)
	}
	if got, err := List(s, "person"); err != nil || len(got) != 0 {
		t.Fatalf("persons = %v, %v", got, err)
	}
}

func TestAppendOrder(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	l := create(t, s, "issue", "", op("create"))
	for _, k := range []string{"a", "b", "c"} {
		head := l.Head
		appendOps(t, s, l, op(k))
		if l.Head == head {
			t.Fatalf("append %s: head unchanged", k)
		}
	}
	head := l.Head
	appendOps(t, s, l)
	if l.Head != head {
		t.Fatal("appending nothing wrote a commit")
	}
	got := load(t, s, "issue", l.ID)
	if !slices.Equal(opKinds(got), []string{"create", "a", "b", "c"}) || !slices.Equal(clocks(got), []uint64{1, 2, 3, 4}) {
		t.Fatalf("kinds %v, clocks %v", opKinds(got), clocks(got))
	}
	if !reflect.DeepEqual(got, l) {
		t.Fatalf("loaded %+v\nappended %+v", got, l)
	}
}

func TestOrderIgnoresWallTime(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	l := create(t, s, "issue", "", Op{Kind: "create", Time: 2000})
	appendOps(t, s, l, Op{Kind: "comment", Time: 1000})
	if got := opKinds(load(t, s, "issue", l.ID)); !slices.Equal(got, []string{"create", "comment"}) {
		t.Fatalf("kinds = %v", got)
	}
}

func TestAppendRaceKeepsBoth(t *testing.T) {
	t.Parallel()
	dir := newRepo(t)
	stores := []*gitstore.Store{open(t, dir), open(t, dir)}
	id := create(t, stores[0], "issue", "", op("create")).ID
	// Both logs are loaded first, so both append on the same head and one
	// of them retries.
	start := make(chan struct{})
	var wg sync.WaitGroup
	errs := make([]error, len(stores))
	for i, s := range stores {
		l := load(t, s, "issue", id)
		wg.Go(func() {
			<-start
			errs[i] = l.Append(t.Context(), s, []Op{op(fmt.Sprint("op", i))})
		})
	}
	close(start)
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		t.Fatal(err)
	}
	got := load(t, stores[0], "issue", id)
	k := opKinds(got)
	slices.Sort(k[1:])
	if !slices.Equal(k, []string{"create", "op0", "op1"}) || !slices.Equal(clocks(got), []uint64{1, 2, 3}) {
		t.Fatalf("kinds %v, clocks %v", opKinds(got), clocks(got))
	}
}

func TestLoadMissing(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	if _, err := Load(s, "issue", "3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadRejects(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	root := commitOf(t, s, map[string]string{"pack": `{"clock":1,"nonce":"ab","ops":[{"kind":"create","time":1}]}`})
	cases := []struct {
		name    string
		files   map[string]string
		parents []gitstore.Hash
	}{
		{"no pack", map[string]string{"notes": `{"clock":1,"ops":[{"kind":"create","time":1}]}`}, nil},
		{"not JSON", map[string]string{"pack": `{"clock":1,`}, nil},
		{"first clock not 1", map[string]string{"pack": `{"clock":2,"ops":[{"kind":"create","time":1}]}`}, nil},
		{"clock not parent + 1", map[string]string{"pack": `{"clock":3,"ops":[{"kind":"comment","time":1}]}`}, []gitstore.Hash{root}},
		{"ops absent", map[string]string{"pack": `{"clock":1,"nonce":"ab"}`}, nil},
		{"ops empty", map[string]string{"pack": `{"clock":2,"ops":[]}`}, []gitstore.Hash{root}},
	}
	for i, c := range cases {
		bad := commitOf(t, s, c.files, c.parents...)
		id := fmt.Sprintf("%040x", i)
		setRef(t, s, RefName("issue", id), bad, gitstore.ZeroHash)
		_, err := Load(s, "issue", id)
		if err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), bad.String()) {
			t.Errorf("%s: err = %v, want one naming %s", c.name, err, bad)
		}
	}
}

func TestIdenticalPacksCountOnce(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	l := create(t, s, "issue", "", op("create"))
	// One pack in two commits, as two clones pulling the same change would
	// write. The trees differ by an entry the reader ignores, and the pack
	// holds fields it doesn't know.
	p := `{"clock":2,"later":true,"ops":[{"kind":"comment","time":1,"later":1}]}`
	x := commitOf(t, s, map[string]string{"pack": p}, l.Head)
	y := commitOf(t, s, map[string]string{"pack": p, "notes": "ignored"}, l.Head)
	m := commitOf(t, s, map[string]string{"pack": `{"clock":3,"ops":[]}`}, x, y)
	setRef(t, s, RefName("issue", l.ID), m, l.Head)
	if got := opKinds(load(t, s, "issue", l.ID)); !slices.Equal(got, []string{"create", "comment"}) {
		t.Fatalf("kinds = %v", got)
	}
}

// A kind not in gitstore.Kinds, or an id that isn't 40 lowercase hex digits,
// is refused before anything is read or written.
func TestBadKindOrID(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	good := create(t, s, "issue", "", op("create"))
	ops := []Op{op("comment")}
	is := func(what string, err error) {
		t.Helper()
		if !errors.Is(err, ErrBadName) {
			t.Errorf("%s: err = %v, want ErrBadName", what, err)
		}
	}
	for _, kind := range []string{"seq", ""} {
		_, err := Create(t.Context(), s, kind, "", ops)
		is("create "+kind, err)
		_, err = Create(t.Context(), s, kind, good.ID, ops)
		is("create "+kind+" with an id", err)
		_, err = Load(s, kind, good.ID)
		is("load "+kind, err)
		_, err = List(s, kind)
		is("list "+kind, err)
		_, err = Merge(t.Context(), s, kind, good.ID, good.Head)
		is("merge "+kind, err)
		is("append "+kind, (&Log{Kind: kind, ID: good.ID, Head: good.Head}).Append(t.Context(), s, ops))
	}
	for _, id := range badIDs {
		if id != "" { // Create with no id takes the pack's hash
			_, err := Create(t.Context(), s, "issue", id, ops)
			is(fmt.Sprintf("create %q", id), err)
		}
		_, err := Load(s, "issue", id)
		is(fmt.Sprintf("load %q", id), err)
		_, err = Merge(t.Context(), s, "issue", id, good.Head)
		is(fmt.Sprintf("merge %q", id), err)
		is(fmt.Sprintf("append %q", id), (&Log{Kind: "issue", ID: id, Head: good.Head}).Append(t.Context(), s, ops))
	}
	if refs, err := s.Refs("refs/"); err != nil || len(refs) != 1 {
		t.Fatalf("refs = %v, %v; want only the good entity's", refs, err)
	}
}

// The reader rejects an operation with no kind or no time. An author is for
// each kind's rules to judge. A write of what the reader rejects is refused.
func TestOpMissingFieldIsRejected(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	for i, ops := range []string{
		`[{"author":"a","time":1}]`,
		`[{"kind":"","author":"a","time":1}]`,
		`[{"kind":"create","author":"a"}]`,
		`[{"kind":"create","author":"a","time":0}]`,
		`[{"kind":"create","author":"a","time":1},{"kind":"comment","author":"a"}]`,
	} {
		bad := commitOf(t, s, map[string]string{"pack": `{"clock":1,"ops":` + ops + `}`})
		id := fmt.Sprintf("%040x", i)
		setRef(t, s, RefName("issue", id), bad, gitstore.ZeroHash)
		_, err := Load(s, "issue", id)
		if err == nil || errors.Is(err, ErrNotFound) || !strings.Contains(err.Error(), bad.String()) {
			t.Errorf("ops %s: err = %v, want one naming %s", ops, err, bad)
		}
	}
	for i, ops := range []string{`[{"kind":"create","time":1}]`, `[{"kind":"create","author":"","time":1}]`} {
		c := commitOf(t, s, map[string]string{"pack": `{"clock":1,"ops":` + ops + `}`})
		id := fmt.Sprintf("%040x", 100+i)
		setRef(t, s, RefName("issue", id), c, gitstore.ZeroHash)
		if l, err := Load(s, "issue", id); err != nil || l.Entries[0].Author != "" {
			t.Errorf("ops %s: %+v, %v; want an empty author", ops, l, err)
		}
	}

	before, err := s.Refs("refs/")
	if err != nil {
		t.Fatal(err)
	}
	for _, o := range []Op{{Author: "a", Time: 1}, {Kind: "create", Author: "a"}} {
		if _, err := Create(t.Context(), s, "issue", "", []Op{o}); err == nil {
			t.Errorf("create %+v: no error", o)
		}
	}
	l := create(t, s, "issue", "", op("create"))
	head := l.Head
	if err := l.Append(t.Context(), s, []Op{op("comment"), {Kind: "comment"}}); err == nil || l.Head != head {
		t.Errorf("append with no time: %v, head %v; want an error and %v", err, l.Head, head)
	}
	after, err := s.Refs("refs/")
	if err != nil || len(after) != len(before)+1 || after[RefName("issue", l.ID)] != head {
		t.Fatalf("refs = %v, %v; want one more, at %v", after, err, head)
	}
}

// Append builds its entries without reading the entity again, and they are
// the ones a Load reads.
func TestAppendEntriesMatchLoad(t *testing.T) {
	s := newStore(t)
	l := create(t, s, "issue", "", op("create"), Op{Kind: "comment", Time: 1, Data: json.RawMessage(` { "body" : "<b>" } `)})
	if got := load(t, s, "issue", l.ID); !reflect.DeepEqual(l, got) {
		t.Fatalf("created %+v\nloaded %+v", l, got)
	}
	first := l.Head
	appendOps(t, s, l, op("ours"))
	theirs := commitOf(t, s, map[string]string{"pack": `{"clock":2,"ops":[{"kind":"theirs","time":1}]}`}, first)
	l, err := Merge(t.Context(), s, "issue", l.ID, theirs)
	if err != nil {
		t.Fatal(err)
	}
	if got := load(t, s, "issue", l.ID); !reflect.DeepEqual(l, got) {
		t.Fatalf("merged %+v\nloaded %+v", l, got)
	}

	// Once the append lands, the ref points at a commit no read takes: a
	// reload would fail.
	name := RefName("issue", l.ID)
	corrupt := commitOf(t, s, map[string]string{"notes": "x"})
	var wrote gitstore.Hash
	swapUpdateRef(t, func(s *gitstore.Store, ctx context.Context, name string, next, prev gitstore.Hash) error {
		if err := s.UpdateRef(ctx, name, next, prev); err != nil {
			return err
		}
		wrote = next
		return s.UpdateRef(ctx, name, corrupt, next)
	})
	appendOps(t, s, l, Op{Kind: "comment", Time: 1, Data: json.RawMessage(`{"body": "a & b"}`)}, op("last"))
	if l.Head != wrote {
		t.Fatalf("head = %v, want the commit written, %v", l.Head, wrote)
	}
	setRef(t, s, name, wrote, corrupt)
	got := load(t, s, "issue", l.ID)
	if !reflect.DeepEqual(l, got) {
		t.Fatalf("appended %+v\nloaded %+v", l, got)
	}
	// The merge's clock is 3, above every entry's: the append's is 4.
	if !slices.Equal(clocks(got), []uint64{1, 1, 2, 2, 4, 4}) || opKinds(got)[5] != "last" {
		t.Fatalf("kinds %v, clocks %v", opKinds(got), clocks(got))
	}
}

func TestAppendGivesUpAfterFive(t *testing.T) {
	s := newStore(t)
	l := create(t, s, "issue", "", op("create"))
	head := l.Head
	calls := 0
	swapUpdateRef(t, func(*gitstore.Store, context.Context, string, gitstore.Hash, gitstore.Hash) error {
		calls++
		return fmt.Errorf("%w: always", gitstore.ErrRefMoved)
	})
	err := l.Append(t.Context(), s, []Op{op("comment")})
	if !errors.Is(err, gitstore.ErrRefMoved) || calls != 5 {
		t.Fatalf("err = %v after %d attempts; want ErrRefMoved after 5", err, calls)
	}
	if h, _ := s.Ref(RefName("issue", l.ID)); h != head || l.Head != head {
		t.Fatalf("ref %v, log head %v; want %v", h, l.Head, head)
	}
}
