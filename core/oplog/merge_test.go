package oplog

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"github.com/guilbep/git-board/core/gitstore"
)

func TestMergeFastForward(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	l := create(t, s, "issue", "", op("create"))
	next := commitOf(t, s, map[string]string{"pack": `{"clock":2,"ops":[{"kind":"comment"}]}`}, l.Head)
	got, err := Merge(s, "issue", l.ID, next)
	if err != nil {
		t.Fatal(err)
	}
	if h, _ := s.Ref(RefName("issue", l.ID)); got.Head != next || h != next {
		t.Fatalf("head = %v, ref %v, want %v", got.Head, h, next)
	}
	if !slices.Equal(opKinds(got), []string{"create", "comment"}) {
		t.Fatalf("kinds = %v", opKinds(got))
	}
}

func TestMergeContained(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	l := create(t, s, "issue", "", op("create"))
	first := l.Head
	appendOps(t, s, l, op("comment"))
	for _, other := range []gitstore.Hash{first, l.Head} {
		got, err := Merge(s, "issue", l.ID, other)
		if err != nil || got.Head != l.Head {
			t.Fatalf("merge of %v: head %v, %v; want %v", other, got.Head, err, l.Head)
		}
	}
}

func TestMergeDiverged(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	l := create(t, s, "issue", "", op("create"))
	first := l.Head
	// Ours: two appends, at clocks 2 and 3. Theirs: one commit at clock 2.
	appendOps(t, s, l, op("ours1"))
	ours1 := l.Head
	appendOps(t, s, l, op("ours2"))
	ours := l.Head
	theirs := commitOf(t, s, map[string]string{"pack": `{"clock":2,"ops":[{"kind":"theirs"}]}`}, first)

	got, err := Merge(s, "issue", l.ID, theirs)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.ReadCommit(got.Head)
	if err != nil || !slices.Equal(c.Parents, []gitstore.Hash{ours, theirs}) {
		t.Fatalf("merge parents = %v, %v; want %v", c.Parents, err, []gitstore.Hash{ours, theirs})
	}
	if p := readPack(t, s, got.Head); p != `{"clock":4,"ops":[]}` {
		t.Fatalf("merge pack = %s", p)
	}
	if h, _ := s.Ref(RefName("issue", l.ID)); h != got.Head {
		t.Fatalf("ref = %v, want %v", h, got.Head)
	}
	// At clock 2, the lower pack hash comes first.
	want := []string{"create", "ours1", "theirs", "ours2"}
	if packOf(t, s, theirs) < packOf(t, s, ours1) {
		want = []string{"create", "theirs", "ours1", "ours2"}
	}
	if !slices.Equal(opKinds(got), want) || !slices.Equal(clocks(got), []uint64{1, 2, 2, 3}) {
		t.Fatalf("kinds %v, clocks %v; want %v", opKinds(got), clocks(got), want)
	}

	// An append on the merge gets the merge's clock + 1.
	appendOps(t, s, got, op("after"))
	after := load(t, s, "issue", l.ID).Entries
	if last := after[len(after)-1]; last.Kind != "after" || last.Clock != 5 {
		t.Fatalf("last = %+v", last)
	}
}

func TestMergeRejectsBadHead(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	l := create(t, s, "issue", "", op("create"))
	bad := commitOf(t, s, map[string]string{"pack": `{"clock":5,"ops":[{"kind":"comment"}]}`}, l.Head)
	if _, err := Merge(s, "issue", l.ID, bad); err == nil {
		t.Fatal("merge of a bad head: no error")
	}
	if h, _ := s.Ref(RefName("issue", l.ID)); h != l.Head {
		t.Fatalf("ref = %v, want %v", h, l.Head)
	}
}

func TestMergeFetched(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	gitRun(t, root, "init", "-q", "--bare", origin)
	var stores []*gitstore.Store
	for _, n := range []string{"a", "b"} {
		dir := filepath.Join(root, n)
		gitRun(t, root, "init", "-q", dir)
		gitRun(t, dir, "remote", "add", "origin", origin)
		stores = append(stores, open(t, dir))
	}
	a, b := stores[0], stores[1]
	ctx := context.Background()

	iss := create(t, a, "issue", "", op("create"))
	// One sync entity, created on both clones before either fetched.
	const id = "56f7a8e819f90e968812bac54ae4b06142d8e735"
	sa := create(t, a, "sync", id, op("link-a"))
	sb := create(t, b, "sync", id, op("link-b"))
	if err := a.Push(ctx, "origin"); err != nil {
		t.Fatal(err)
	}
	if err := b.Fetch(ctx, "origin"); err != nil {
		t.Fatal(err)
	}
	// git-board's own refs under refs/board/ are not entities.
	setRef(t, b, "refs/board/seq", sb.Head, gitstore.ZeroHash)
	setRef(t, b, "refs/remotes/origin/board/seq", iss.Head, gitstore.ZeroHash)
	setRef(t, b, "refs/remotes/origin/board/other/x", iss.Head, gitstore.ZeroHash)

	n, err := MergeFetched(b, "origin")
	if err != nil || n != 2 {
		t.Fatalf("moved = %d, %v; want 2", n, err)
	}
	if h, _ := b.Ref(RefName("issue", iss.ID)); h != iss.Head {
		t.Fatalf("issue ref = %v, want %v", h, iss.Head)
	}
	got := load(t, b, "sync", id)
	c, err := b.ReadCommit(got.Head)
	if err != nil || !slices.Equal(c.Parents, []gitstore.Hash{sb.Head, sa.Head}) {
		t.Fatalf("merge parents = %v, %v", c.Parents, err)
	}
	want := []string{"link-a", "link-b"}
	if packOf(t, b, sb.Head) < packOf(t, b, sa.Head) {
		want = []string{"link-b", "link-a"}
	}
	if !slices.Equal(opKinds(got), want) || !slices.Equal(clocks(got), []uint64{1, 1}) {
		t.Fatalf("kinds %v, clocks %v; want %v", opKinds(got), clocks(got), want)
	}

	if n, err := MergeFetched(b, "origin"); err != nil || n != 0 {
		t.Fatalf("second merge moved %d, %v", n, err)
	}
	if h, _ := b.Ref("refs/board/seq"); h != sb.Head {
		t.Fatalf("seq = %v, want %v", h, sb.Head)
	}
	if h, _ := b.Ref("refs/board/other/x"); h != gitstore.ZeroHash {
		t.Fatalf("refs/board/other/x = %v", h)
	}
}
