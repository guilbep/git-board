package md

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/next/kernel/pack"
	"github.com/guilbep/git-board/next/store"
	"github.com/guilbep/git-board/next/store/storetest"
)

var ctx = context.Background()

func open(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "issues"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestConformance(t *testing.T) {
	t.Parallel()
	storetest.Run(t, storetest.Options{
		Open: func(t *testing.T) store.Datastore { return open(t) },
		// another writer appends to other entities meanwhile
		Churn: func(ctx context.Context, ds store.Datastore) error {
			v := ""
			for c := uint64(1); ctx.Err() == nil; c++ {
				var err error
				if v, err = ds.Add(ctx, "person", storetest.ID(100), storetest.Pack(c), v); err != nil && ctx.Err() == nil {
					return err
				}
			}
			return nil
		},
		Junk: func(t *testing.T, ds store.Datastore) {
			root := ds.(*Store).root()
			for name, body := range map[string]string{
				"issue/notes.jsonl":                        string(storetest.Pack(1)) + "\n",
				"issue/" + storetest.ID(9):                 string(storetest.Pack(1)) + "\n", // no .jsonl
				"issue/" + storetest.ID(8) + ".jsonl.lock": "",
				"issue/" + storetest.ID(7) + ".jsonl":      " \n\t\n", // white space alone: no entity
			} {
				if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Mkdir(filepath.Join(root, "issue", storetest.ID(6)+".jsonl"), 0o755); err != nil {
				t.Fatal(err)
			}
		},
	})
}

// A reader sees an append in parts (v1's 12cfe5d): a last line cut
// short is not read, nor counted in the version, and a write on the
// version without it lands after it ends.
func TestPackBeingAppendedIsLeftOut(t *testing.T) {
	t.Parallel()
	s := open(t)
	v1, err := s.Add(ctx, "issue", storetest.ID(1), storetest.Pack(1), "")
	if err != nil {
		t.Fatal(err)
	}
	path := s.path("issue", storetest.ID(1))
	two := storetest.Pack(2)
	for cut := 1; cut < len(two); cut++ {
		if err := appendFile(path, two[:cut]); err != nil {
			t.Fatal(err)
		}
		heads, err := s.Heads(ctx, "issue")
		if err != nil || len(heads) != 1 || heads[0].Version != v1 {
			t.Fatalf("cut at %d: Heads = %v, %v; want the version without the cut pack", cut, heads, err)
		}
		n := 0
		err = s.Load(ctx, []store.Ref{{Kind: "issue", ID: storetest.ID(1)}}, func(r store.Ref, packs [][]byte, err error) error {
			n = len(packs)
			if r.Version != v1 {
				t.Errorf("cut at %d: loaded at %s, want %s", cut, r.Version, v1)
			}
			return err
		})
		if err != nil || n != 1 {
			t.Fatalf("cut at %d: Load = %d packs, %v", cut, n, err)
		}
		data, _ := os.ReadFile(path)
		if err := os.WriteFile(path, data[:len(data)-cut], 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// A hand edit that took the last line feed leaves a whole pack, read; the
// next append puts a line feed before its own.
func TestHandEditWithoutLastLineFeed(t *testing.T) {
	t.Parallel()
	s := open(t)
	path := s.path("issue", storetest.ID(1))
	os.MkdirAll(filepath.Dir(path), 0o755)
	one := storetest.Pack(1)
	if err := os.WriteFile(path, append([]byte("  \r\n"), one...), 0o644); err != nil {
		t.Fatal(err)
	}
	heads, _ := s.Heads(ctx, "issue")
	if len(heads) != 1 {
		t.Fatalf("Heads = %v, want the entity", heads)
	}
	if _, err := s.Add(ctx, "issue", storetest.ID(1), storetest.Pack(2), heads[0].Version); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	want := "  \r\n" + string(one) + "\n" + string(storetest.Pack(2)) + "\n"
	if string(data) != want {
		t.Fatalf("file = %q, want %q", data, want)
	}
}

// Two branches' appends merged by git's union driver: both packs kept,
// of one clock; the next write's clock is one more; a line twice is
// one pack.
func TestUnionMerge(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t", "-c", "init.defaultBranch=main"}, args...)...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	s, err := Open(filepath.Join(dir, "issues"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	id := storetest.ID(1)
	v, err := s.Add(ctx, "issue", id, storetest.Pack(1), "")
	if err != nil {
		t.Fatal(err)
	}
	git("add", "-A")
	git("commit", "-qm", "one")
	git("checkout", "-qb", "other")
	other := op(2, "theirs")
	if _, err := s.Add(ctx, "issue", id, other, v); err != nil {
		t.Fatal(err)
	}
	git("commit", "-qam", "theirs")
	git("checkout", "-q", "main")
	ours := op(2, "ours")
	if _, err := s.Add(ctx, "issue", id, ours, v); err != nil {
		t.Fatal(err)
	}
	git("commit", "-qam", "ours")
	git("merge", "-q", "--no-edit", "other")
	var packs [][]byte
	var at string
	s.Load(ctx, []store.Ref{{Kind: "issue", ID: id}}, func(r store.Ref, p [][]byte, err error) error {
		packs, at = p, r.Version
		return err
	})
	if len(packs) != 3 || string(packs[1]) != string(ours) || string(packs[2]) != string(other) {
		t.Fatalf("after the merge, packs = %q; want the first, ours, then theirs", packs)
	}
	if _, err := s.Add(ctx, "issue", id, op(2, "again"), at); err == nil {
		t.Fatal("a clock both branches had: want refused")
	}
	if _, err := s.Add(ctx, "issue", id, op(3, "next"), at); err != nil {
		t.Fatalf("clock 3 after two of 2: %v", err)
	}
	// a merge that leaves a line twice: one pack in the order
	path := s.path("issue", id)
	data, _ := os.ReadFile(path)
	os.WriteFile(path, append(data, append(ours, '\n')...), 0o644)
	s.Load(ctx, []store.Ref{{Kind: "issue", ID: id}}, func(r store.Ref, p [][]byte, err error) error {
		packs = p
		return err
	})
	es, _, err := pack.Order(packs)
	if err != nil || len(es) != 4 {
		t.Fatalf("Order = %d operations, %v; want 4, the line twice once", len(es), err)
	}
}

func op(clock uint64, text string) []byte {
	b, err := pack.Encode(pack.Pack{Clock: clock, Ops: []pack.Op{{Kind: "comment", Author: storetest.ID(0), Time: 1, Data: []byte(`{"text":"` + text + `"}`)}}})
	if err != nil {
		panic(err)
	}
	return b
}

// A lock file older than 30 seconds is a dead writer's: taken over. A
// fresh one is waited for.
func TestStaleLockTakenOver(t *testing.T) {
	t.Parallel()
	s := open(t)
	path := s.path("issue", storetest.ID(1))
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path+".lock", nil, 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Minute)
	os.Chtimes(path+".lock", old, old)
	if _, err := s.Add(ctx, "issue", storetest.ID(1), storetest.Pack(1), ""); err != nil {
		t.Fatalf("with a stale lock: %v", err)
	}
	if _, err := os.Stat(path + ".lock"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the lock stays after the write: %v", err)
	}
	// a fresh lock, let go 50 ms later: the write waits for it
	os.WriteFile(path+".lock", nil, 0o644)
	go func() { time.Sleep(50 * time.Millisecond); os.Remove(path + ".lock") }()
	heads, _ := s.Heads(ctx, "issue")
	start := time.Now()
	if _, err := s.Add(ctx, "issue", storetest.ID(1), storetest.Pack(2), heads[0].Version); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) < 40*time.Millisecond {
		t.Fatal("the write didn't wait for the lock")
	}
}

// The counter is written on the default branch alone, and outside git.
func TestCounter(t *testing.T) {
	t.Parallel()
	s := open(t) // outside any checkout: writes
	if v, _, err := s.Count(ctx); v != nil || err != nil {
		t.Fatalf("Count = %q, %v; want none", v, err)
	}
	v1, err := s.SetCount(ctx, []byte("prefix MD\nstart 1\n"), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetCount(ctx, []byte("prefix MD\nstart 2\n"), ""); !errors.Is(err, store.ErrMoved) {
		t.Fatalf("a second first counter: %v, want ErrMoved", err)
	}
	if _, err := s.SetCount(ctx, []byte("prefix MD\nstart 1\n1 "+storetest.ID(1)+"\n"), v1); err != nil {
		t.Fatal(err)
	}
	attrs, _ := os.ReadFile(filepath.Join(s.root(), ".gitattributes"))
	if !strings.Contains(string(attrs), "/numbers merge=binary") {
		t.Fatalf(".gitattributes = %q", attrs)
	}

	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		if out, err := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@t"}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main")
	g, err := Open(filepath.Join(dir, "issues"), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := g.CountWaits(ctx); err != nil {
		t.Fatalf("on a new repository's main: %v", err)
	}
	run("commit", "-q", "--allow-empty", "-m", "one")
	run("checkout", "-qb", "feature")
	if _, err := g.SetCount(ctx, []byte("prefix MD\nstart 1\n"), ""); !errors.Is(err, store.ErrMissing) || !strings.Contains(err.Error(), "on main alone: this checkout is on feature") {
		t.Fatalf("on a branch: %v", err)
	}
	run("checkout", "-q", "main")
	if _, err := g.SetCount(ctx, []byte("prefix MD\nstart 1\n"), ""); err != nil {
		t.Fatalf("back on main: %v", err)
	}
}
