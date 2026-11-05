package git

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/kernel/pack"
	"github.com/guilbep/git-board/store"
	"github.com/guilbep/git-board/store/storetest"
)

// clone is a repository cloned from origin, a bare one.
func clone(t *testing.T, origin string) *Store {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "clone")
	if out, err := exec.Command("git", "clone", "-q", origin, dir).CombinedOutput(); err != nil {
		t.Fatalf("git clone: %v: %s", err, out)
	}
	s, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func packAt(t *testing.T, clock uint64, kind string) []byte {
	t.Helper()
	b, err := pack.Encode(pack.Pack{Clock: clock, Ops: []pack.Op{{Kind: kind, Author: storetest.ID(0), Time: int64(clock)}}})
	if err != nil {
		t.Fatal(err)
	}
	return b
}

// ops are the entity's operations' kinds, in order, as a load reads them.
func ops(t *testing.T, s *Store, id string) []string {
	t.Helper()
	var out []string
	err := s.Load(ctx, []store.Ref{{Kind: "issue", ID: id}}, func(_ store.Ref, packs [][]byte, err error) error {
		if err != nil {
			return err
		}
		es, _, err := pack.Order(packs)
		for _, e := range es {
			out = append(out, e.Kind)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// Two clones push, pull and merge one entity, as v1's do: a fast-forward,
// then a merge of two heads, which a push then sends.
func TestRemote(t *testing.T) {
	t.Parallel()
	origin := filepath.Join(t.TempDir(), "origin.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", origin).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	a, b := clone(t, origin), clone(t, origin)
	id := storetest.ID(1)
	v1, err := a.Add(ctx, "issue", id, packAt(t, 1, "create"), "")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Push(ctx, "origin", "issue", "person"); err != nil {
		t.Fatal(err)
	}
	heads, err := b.Fetch(ctx, "origin", "issue", "person")
	if err != nil || len(heads) != 1 || heads[0] != (store.Head{Kind: "issue", ID: id, Version: v1}) {
		t.Fatalf("fetched %v, %v", heads, err)
	}
	if n, err := b.Merge(ctx, "origin", "issue"); n != 1 || err != nil {
		t.Fatalf("merged %d, %v", n, err)
	}
	// both write offline
	if _, err := a.Add(ctx, "issue", id, packAt(t, 2, "set-title"), v1); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Add(ctx, "issue", id, packAt(t, 2, "comment"), v1); err != nil {
		t.Fatal(err)
	}
	if err := a.Push(ctx, "origin", "issue"); err != nil {
		t.Fatal(err)
	}
	if err := b.Push(ctx, "origin", "issue"); err == nil || !strings.Contains(err.Error(), "rejected") {
		t.Fatalf("a push over the other's: %v", err)
	}
	if n, err := b.Merge(ctx, "origin", "issue"); n != 1 || err != nil {
		t.Fatalf("merged %d, %v", n, err)
	}
	got := ops(t, b, id)
	if len(got) != 3 || got[0] != "create" {
		t.Fatalf("after the merge: %v", got)
	}
	if n, err := b.Merge(ctx, "origin", "issue"); n != 0 || err != nil {
		t.Fatalf("merged again: %d, %v", n, err)
	}
	if err := b.Push(ctx, "origin", "issue"); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Fetch(ctx, "origin", "issue"); err != nil {
		t.Fatal(err)
	}
	if n, err := a.Merge(ctx, "origin", "issue"); n != 1 || err != nil {
		t.Fatalf("a fast-forward: %d, %v", n, err)
	}
	if mine := ops(t, a, id); strings.Join(mine, " ") != strings.Join(got, " ") {
		t.Fatalf("the clones read %v and %v", mine, got)
	}
	if _, err := a.Fetch(ctx, "--upload-pack=x", "issue"); err == nil {
		t.Fatal("a remote that is an option")
	}
}

// The merge after a fetch costs a few git processes, however many
// entities (design.md: a fetch's merges go in one git update-ref
// --stdin): none per entity unchanged, none per entity moved. It counts
// processes, not time.
func TestMergeCost(t *testing.T) {
	t.Parallel()
	const n = 300
	s := clone(t, initBare(t))
	var old, copies bytes.Buffer
	for i := range n {
		id := storetest.ID(i + 1)
		v1, err := s.Add(ctx, "issue", id, packAt(t, 1, "create"), "")
		if err != nil {
			t.Fatal(err)
		}
		v2, err := s.Add(ctx, "issue", id, packAt(t, 2, "comment"), v1)
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&old, "update refs/board/issue/%s %s\n", id, v1)
		fmt.Fprintf(&copies, "create refs/board-remotes/origin/issue/%s %s\n", id, v2)
	}
	if _, err := s.git(ctx, copies.Bytes(), "update-ref", "--stdin"); err != nil {
		t.Fatal(err)
	}
	merge := func(what string, want int, most int64) {
		t.Helper()
		var c store.Counts
		moved, err := s.Merge(store.WithCounts(ctx, &c), "origin", "issue", "person")
		if err != nil || moved != want {
			t.Fatalf("%s: moved %d, %v; want %d", what, moved, err, want)
		}
		if p := c.Processes.Load(); p > most {
			t.Fatalf("%s: %d git processes for %d entities, want at most %d", what, p, n, most)
		}
	}
	merge("nothing new", 0, 2)
	if _, err := s.git(ctx, old.Bytes(), "update-ref", "--stdin"); err != nil {
		t.Fatal(err)
	}
	merge("every entity a fast-forward", n, 4)
	if got := ops(t, s, storetest.ID(n)); len(got) != 2 {
		t.Fatalf("after the fast-forwards: %v", got)
	}
}

func initBare(t *testing.T) string {
	t.Helper()
	origin := filepath.Join(t.TempDir(), "origin.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", origin).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	return origin
}
