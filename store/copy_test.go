package store_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/kernel/pack"
	"github.com/guilbep/git-board/store"
	"github.com/guilbep/git-board/store/git"
	"github.com/guilbep/git-board/store/md"
	"github.com/guilbep/git-board/store/pg"
	"github.com/guilbep/git-board/store/pg/pgtest"
	"github.com/guilbep/git-board/store/storetest"
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

// uneven writes an issue in git as two clones make it: c1, then A and A2
// on one clone and B on the other, then a merge of A2 and B, at clock 4,
// whose level below holds A2 alone. It gives the entity's id.
func uneven(t *testing.T, s *git.Store) string {
	t.Helper()
	git := func(stdin []byte, args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"--git-dir", s.GitDir()}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		if stdin != nil {
			cmd.Stdin = bytes.NewReader(stdin)
		}
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git %v: %v", args, err)
		}
		return strings.TrimSpace(string(out))
	}
	commit := func(p []byte, parents ...string) string {
		blob := git(p, "hash-object", "-w", "--stdin")
		tree := git([]byte("100644 blob "+blob+"\tpack\n"), "mktree")
		args := []string{"commit-tree", tree, "-m", ""}
		for _, q := range parents {
			args = append(args, "-p", q)
		}
		return git(nil, args...)
	}
	c1 := commit(op(1, "first"))
	a := commit(op(2, "a"), c1)
	a2 := commit(op(3, "a2"), a)
	b := commit(op(2, "b"), c1)
	m := commit([]byte(`{"clock":4,"ops":[]}`), a2, b)
	top := commit(op(5, "after"), m)
	id := storetest.ID(7)
	git(nil, "update-ref", "refs/board/issue/"+id, top)
	return id
}

// A copy of an uneven merge (one side wrote twice, the other once) reads
// back the same in every datastore, and back in git, whatever the way:
// git, then pg, md or git, then git again. A second copy writes nothing.
func TestCopyUnevenMerge(t *testing.T) {
	t.Parallel()
	src := gitStore(t)
	id := uneven(t, src)
	want := order(t, src, "issue", id)
	if len(want) != 5 {
		t.Fatalf("the source reads %v", want)
	}
	via := map[string]func(t *testing.T) store.Datastore{
		"git": func(t *testing.T) store.Datastore { return gitStore(t) },
		"md":  func(t *testing.T) store.Datastore { return mdStore(t) },
	}
	if os.Getenv(pgtest.Env) != "" {
		via["pg"] = func(t *testing.T) store.Datastore {
			p, err := pg.Open(ctx, pgtest.DSN(t), t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { p.Close() })
			return p
		}
	}
	for name, open := range via {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			chain := []store.Datastore{src, open(t), gitStore(t)}
			for i := 1; i < len(chain); i++ {
				from, to := chain[i-1], chain[i]
				if c, err := store.Copy(ctx, from, to, "issue"); err != nil || c.Entities != 1 {
					t.Fatalf("copy %d: %+v, %v", i, c, err)
				}
				if got := order(t, to, "issue", id); !slices.Equal(got, want) {
					t.Fatalf("copy %d reads %v, want %v", i, got, want)
				}
				if c, err := store.Copy(ctx, from, to, "issue"); err != nil || c.Entities != 0 {
					t.Fatalf("copy %d again: %+v, %v; want nothing", i, c, err)
				}
			}
		})
	}
}

// A merge put on a head git already has, whose level below is that head
// alone, joins the head and a commit under it.
func TestPutMergeOnAHead(t *testing.T) {
	t.Parallel()
	g, id := gitStore(t), storetest.ID(8)
	v, err := g.Put(ctx, "issue", id, [][]byte{op(1, "first"), op(2, "a"), op(2, "b"), op(3, "a2")}, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.Put(ctx, "issue", id, [][]byte{[]byte(`{"clock":4,"ops":[]}`), op(5, "after")}, v); err != nil {
		t.Fatal(err)
	}
	if got := order(t, g, "issue", id); len(got) != 5 {
		t.Fatalf("reads %v", got)
	}
}
