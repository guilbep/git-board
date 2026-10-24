package issue

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/oplog"
)

// packHash is the hash of the pack blob in the issue's head commit.
func packHash(t *testing.T, s *gitstore.Store, id string) string {
	t.Helper()
	l, err := oplog.Load(s, "issue", id)
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.ReadCommit(l.Head)
	if err != nil {
		t.Fatal(err)
	}
	tree, err := s.ReadTree(c.Tree)
	if err != nil || len(tree) != 1 {
		t.Fatalf("tree = %v, %v", tree, err)
	}
	return tree[0].Blob.String()
}

func TestMergeConverges(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	gitRun(t, root, "init", "-q", "--bare", origin)
	dirA, dirB := filepath.Join(root, "a"), filepath.Join(root, "b")
	for _, dir := range []string{dirA, dirB} {
		gitRun(t, root, "clone", "-q", origin, dir)
	}
	// Each clone also knows the other, so each merges on its own.
	gitRun(t, dirA, "remote", "add", "peer", dirB)
	gitRun(t, dirB, "remote", "add", "peer", dirA)
	a, b := open(t, dirA), open(t, dirB)
	ctx := context.Background()
	check := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	// exchange has both clones fetch, then both merge, so each merges the
	// other's head from before the other merged.
	exchange := func() {
		t.Helper()
		for _, s := range []*gitstore.Store{a, b} {
			check(s.Fetch(ctx, "peer"))
		}
		for _, s := range []*gitstore.Store{a, b} {
			_, err := oplog.MergeFetched(s, "peer")
			check(err)
		}
	}

	c, err := Create(a, NewCreate("ann", "T", "B"))
	c = must(t, c, err)
	check(a.Push(ctx, "origin"))
	check(b.Fetch(ctx, "origin"))
	_, err = oplog.MergeFetched(b, "origin")
	check(err)

	_, err = Do(a, c.ID, SetTitle("ann", "from A"), ChangeLabels("ann", []string{"a"}, nil))
	check(err)
	_, err = Do(b, c.ID, NewComment("bob", "hi"), SetTitle("bob", "from B"))
	check(err)
	// Both packs have clock 2, so the greater hash sorts last.
	want := "from B"
	if packHash(t, a, c.ID) > packHash(t, b, c.ID) {
		want = "from A"
	}

	exchange()
	// Each side wrote its own merge commit.
	var merges []gitstore.Hash
	for _, s := range []*gitstore.Store{a, b} {
		l, err := oplog.Load(s, "issue", c.ID)
		check(err)
		mc, err := s.ReadCommit(l.Head)
		check(err)
		if len(mc.Parents) != 2 {
			t.Fatalf("head %v has %d parents, want 2", l.Head, len(mc.Parents))
		}
		merges = append(merges, l.Head)
	}
	if merges[0] == merges[1] {
		t.Fatalf("both sides have the same merge commit %v", merges[0])
	}
	exchange()

	ia, err := Load(a, c.ID)
	ia = must(t, ia, err)
	ib, err := Load(b, c.ID)
	ib = must(t, ib, err)
	if !reflect.DeepEqual(ia, ib) {
		t.Fatalf("A = %+v\nB = %+v", ia, ib)
	}
	if ia.Title != want || len(ia.Comments) != 1 || !reflect.DeepEqual(ia.Labels, []string{"a"}) {
		t.Fatalf("issue = %+v, want title %q", ia, want)
	}
}
