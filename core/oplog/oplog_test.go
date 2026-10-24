package oplog

import (
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

func create(t *testing.T, s *gitstore.Store, kind, id string, ops ...Op) *Log {
	t.Helper()
	l, err := Create(s, kind, id, ops)
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
	if err := l.Append(s, ops); err != nil {
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
	if err := s.UpdateRef(name, next, prev); err != nil {
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
	if _, err := Create(s, "issue", "", nil); err == nil {
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
	if _, err := Create(s, "sync", id, []Op{op("link")}); err == nil {
		t.Fatal("second create of one id: no error")
	}
}

func TestList(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	var want []string
	for range 3 {
		want = append(want, create(t, s, "issue", "", op("create")).ID)
	}
	slices.Sort(want)
	create(t, s, "sync", "ab", op("link"))
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
			t.Fatalf("append %s: log not reloaded", k)
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
			errs[i] = l.Append(s, []Op{op(fmt.Sprint("op", i))})
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
	if _, err := Load(s, "issue", "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestLoadRejects(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	root := commitOf(t, s, map[string]string{"pack": `{"clock":1,"nonce":"ab","ops":[{"kind":"create"}]}`})
	cases := []struct {
		name    string
		files   map[string]string
		parents []gitstore.Hash
	}{
		{"no pack", map[string]string{"notes": `{"clock":1,"ops":[{"kind":"create"}]}`}, nil},
		{"not JSON", map[string]string{"pack": `{"clock":1,`}, nil},
		{"first clock not 1", map[string]string{"pack": `{"clock":2,"ops":[{"kind":"create"}]}`}, nil},
		{"clock not parent + 1", map[string]string{"pack": `{"clock":3,"ops":[{"kind":"comment"}]}`}, []gitstore.Hash{root}},
		{"ops absent", map[string]string{"pack": `{"clock":1,"nonce":"ab"}`}, nil},
		{"ops empty", map[string]string{"pack": `{"clock":2,"ops":[]}`}, []gitstore.Hash{root}},
	}
	for i, c := range cases {
		bad := commitOf(t, s, c.files, c.parents...)
		id := fmt.Sprint("bad", i)
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
	p := `{"clock":2,"later":true,"ops":[{"kind":"comment","later":1}]}`
	x := commitOf(t, s, map[string]string{"pack": p}, l.Head)
	y := commitOf(t, s, map[string]string{"pack": p, "notes": "ignored"}, l.Head)
	m := commitOf(t, s, map[string]string{"pack": `{"clock":3,"ops":[]}`}, x, y)
	setRef(t, s, RefName("issue", l.ID), m, l.Head)
	if got := opKinds(load(t, s, "issue", l.ID)); !slices.Equal(got, []string{"create", "comment"}) {
		t.Fatalf("kinds = %v", got)
	}
}
