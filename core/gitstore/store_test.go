package gitstore

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha1"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
)

var ctx = context.Background()

func gitRun(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(cmd.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func newStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	gitRun(t, dir, "init", "-q")
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func commitOf(t *testing.T, s *Store, parents ...Hash) Hash {
	t.Helper()
	b, err := s.WriteBlob([]byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	tr, err := s.WriteTree([]TreeEntry{{Name: "pack", Blob: b}})
	if err != nil {
		t.Fatal(err)
	}
	c, err := s.WriteCommit(Commit{Tree: tr, Parents: parents})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestOpenFreshRepo(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	refs, err := s.Refs("refs/board/")
	if err != nil || len(refs) != 0 {
		t.Fatalf("refs = %v, %v", refs, err)
	}
	h, err := s.Ref("refs/board/issue/x")
	if err != nil || h != ZeroHash {
		t.Fatalf("ref = %v, %v", h, err)
	}
}

func TestBlobTreeCommitRoundTrip(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	b, _ := s.WriteBlob([]byte("hello"))
	got, err := s.ReadBlob(b)
	if err != nil || string(got) != "hello" {
		t.Fatalf("blob = %q, %v", got, err)
	}
	want := []TreeEntry{{Name: "pack", Blob: b}}
	tr, _ := s.WriteTree(want)
	ents, err := s.ReadTree(tr)
	if err != nil || len(ents) != 1 || ents[0] != want[0] {
		t.Fatalf("tree = %v, %v", ents, err)
	}
	c1 := commitOf(t, s)
	c2 := commitOf(t, s, c1)
	c, err := s.ReadCommit(c2)
	if err != nil || c.Tree == ZeroHash || len(c.Parents) != 1 || c.Parents[0] != c1 {
		t.Fatalf("commit = %v, %v", c, err)
	}
}

func TestUpdateRefCreatesAndMoves(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c1 := commitOf(t, s)
	c2 := commitOf(t, s, c1)
	const name = "refs/board/issue/x"
	if err := s.UpdateRef(ctx, name, c1, ZeroHash); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRef(ctx, name, c2, c1); err != nil {
		t.Fatal(err)
	}
	if h, _ := s.Ref(name); h != c2 {
		t.Fatalf("ref = %v", h)
	}
	refs, _ := s.Refs("refs/board/issue/")
	if len(refs) != 1 || refs[name] != c2 {
		t.Fatalf("refs = %v", refs)
	}
}

// Refs lists packed and loose refs past another writer's lock file and a ref
// file still empty. go-git's own listing fails on both.
func TestRefsPastLocks(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c1 := commitOf(t, s)
	c2 := commitOf(t, s, c1)
	a := "refs/board/issue/" + strings.Repeat("a", 40)
	b := "refs/board/issue/" + strings.Repeat("b", 40)
	for _, name := range []string{a, b, "refs/board/person/x"} {
		if err := s.UpdateRef(ctx, name, c1, ZeroHash); err != nil {
			t.Fatal(err)
		}
	}
	gitRun(t, s.gitDir, "pack-refs", "--all")
	if err := s.UpdateRef(ctx, b, c2, c1); err != nil { // loose, over its packed copy
		t.Fatal(err)
	}
	for _, name := range []string{a + ".lock", "refs/board/issue/" + strings.Repeat("c", 40)} {
		if err := os.WriteFile(filepath.Join(s.gitDir, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	refs, err := s.Refs("refs/board/issue/")
	if want := map[string]Hash{a: c1, b: c2}; err != nil || !maps.Equal(refs, want) {
		t.Fatalf("refs = %v, %v; want %v", refs, err, want)
	}
	if _, err := s.Refs("board/"); err == nil {
		t.Fatal("a prefix outside refs/ is listed")
	}
}

// A cancelled git gets SIGINT, not SIGKILL, so it removes its lock file.
func TestCancelledGitLeavesNoLock(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c1 := commitOf(t, s)
	const name = "refs/board/issue/x"
	cctx, cancel := context.WithCancel(ctx)
	cmd := s.command(cctx, "update-ref", "--stdin")
	in, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	if _, err := in.Write([]byte("start\ncreate " + name + " " + c1.String() + "\nprepare\n")); err != nil {
		t.Fatal(err)
	}
	lines := bufio.NewScanner(out)
	for lines.Scan() && lines.Text() != "prepare: ok" {
	}
	lock := filepath.Join(s.gitDir, name+".lock")
	if _, err := os.Stat(lock); err != nil {
		t.Fatalf("the prepared transaction holds no lock: %v", err)
	}
	cancel()
	_ = cmd.Wait()
	if _, err := os.Stat(lock); !os.IsNotExist(err) {
		t.Fatalf("lock left behind: %v", err)
	}
}

func TestUpdateRefRefusesMoved(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c1 := commitOf(t, s)
	c2 := commitOf(t, s, c1)
	c3 := commitOf(t, s, c2)
	const name = "refs/board/issue/x"
	if err := s.UpdateRef(ctx, name, c2, ZeroHash); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRef(ctx, name, c3, c1); !errors.Is(err, ErrRefMoved) {
		t.Fatalf("err = %v", err)
	}
	if h, _ := s.Ref(name); h != c2 {
		t.Fatalf("ref = %v", h)
	}
}

func TestUpdateRefRefusesExisting(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c1 := commitOf(t, s)
	const name = "refs/board/issue/x"
	if err := s.UpdateRef(ctx, name, c1, ZeroHash); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRef(ctx, name, c1, ZeroHash); !errors.Is(err, ErrRefMoved) {
		t.Fatalf("err = %v", err)
	}
}

func TestReadAfterExternalRepack(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	b, _ := s.WriteBlob([]byte("packed"))
	tr, _ := s.WriteTree([]TreeEntry{{Name: "pack", Blob: b}})
	c, _ := s.WriteCommit(Commit{Tree: tr})
	_, _ = s.ReadBlob(b) // the store has read its pack index
	gitRun(t, s.gitDir, "update-ref", "refs/board/issue/x", c.String())
	gitRun(t, s.gitDir, "repack", "-adq")
	gitRun(t, s.gitDir, "prune-packed")
	if got, err := s.ReadBlob(b); err != nil || string(got) != "packed" {
		t.Fatalf("blob = %q, %v", got, err)
	}
	if _, err := s.ReadTree(tr); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadCommit(c); err != nil {
		t.Fatal(err)
	}
}

func TestStoreConcurrentReadsAndReindex(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	b, _ := s.WriteBlob([]byte("shared"))
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for range 50 {
				if got, err := s.ReadBlob(b); err != nil || string(got) != "shared" {
					t.Errorf("blob = %q, %v", got, err)
					return
				}
			}
		}()
		go func() { // a miss reindexes
			defer wg.Done()
			for range 50 {
				_, _ = s.ReadCommit(ZeroHash)
			}
		}()
	}
	wg.Wait()
}

// A written blob is read back at the id it was written under, with git
// running alongside. sha1cd v0.6.0's arm64 assembly bounded its loop by R16,
// a register it never set: under -race, with git subprocesses about, R16 was
// sometimes within a block of 2^64, a block went unhashed, and WriteBlob
// returned an id no object had.
func TestWriteBlobIDUnderLoad(t *testing.T) {
	t.Parallel()
	done := make(chan struct{})
	var git sync.WaitGroup
	for range 4 {
		s := newStore(t)
		git.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
					_, _ = s.Config(ctx, "core.bare")
				}
			}
		})
	}
	var writers sync.WaitGroup
	for g := range 8 {
		s := newStore(t)
		writers.Go(func() {
			for i := range 500 {
				data := bytes.Repeat([]byte{byte(g)}, i*7%2000) // most span blocks
				want := sha1.Sum(fmt.Appendf(nil, "blob %d\x00%s", len(data), data))
				h, err := s.WriteBlob(data)
				if err != nil || h != Hash(want) {
					t.Errorf("blob of %d bytes: id %s, want %x: %v", len(data), h, want, err)
					return
				}
				if got, err := s.ReadBlob(h); err != nil || !bytes.Equal(got, data) {
					t.Errorf("blob %s: %v", h, err)
					return
				}
			}
		})
	}
	writers.Wait()
	close(done)
	git.Wait()
}

func TestUpdateRefLockFileIsMoved(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c1 := commitOf(t, s)
	const name = "refs/board/issue/x"
	lock := filepath.Join(s.gitDir, name+".lock")
	if err := os.MkdirAll(filepath.Dir(lock), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRef(ctx, name, c1, ZeroHash); !errors.Is(err, ErrRefMoved) {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateRefDeletedRefIsMoved(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c1 := commitOf(t, s)
	c2 := commitOf(t, s, c1)
	if err := s.UpdateRef(ctx, "refs/board/issue/x", c2, c1); !errors.Is(err, ErrRefMoved) {
		t.Fatalf("err = %v", err)
	}
}

func TestUpdateRefOtherFileExistsIsNot(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c1 := commitOf(t, s)
	// A directory, not empty, where the ref file goes.
	c2 := commitOf(t, s, c1)
	if err := s.UpdateRef(ctx, "refs/board/issue/x/y", c2, ZeroHash); err != nil {
		t.Fatal(err)
	}
	err := s.UpdateRef(ctx, "refs/board/issue/x", c1, ZeroHash)
	if err == nil || errors.Is(err, ErrRefMoved) {
		t.Fatalf("err = %v", err)
	}
}

func TestGitRunsInCLocale(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	if !slices.Contains(s.command(ctx, "status").Env, "LC_ALL=C") {
		t.Fatal("LC_ALL=C missing")
	}
}

func TestOpenNestedBare(t *testing.T) {
	t.Parallel()
	outer := t.TempDir()
	gitRun(t, outer, "init", "-q")
	inner := filepath.Join(outer, "inner.git")
	gitRun(t, outer, "init", "-q", "--bare", inner)
	s, err := Open(inner)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(inner)
	got, _ := filepath.EvalSymlinks(s.gitDir)
	if got != want {
		t.Fatalf("gitDir = %s, want %s", got, want)
	}
	gotc, _ := filepath.EvalSymlinks(s.CommonDir())
	if gotc != want {
		t.Fatalf("CommonDir = %s, want %s", gotc, want)
	}
}

func TestCommonDirInWorktree(t *testing.T) {
	t.Parallel()
	main := t.TempDir()
	gitRun(t, main, "init", "-q")
	gitRun(t, main, "commit", "-q", "--allow-empty", "-m", "x")
	wt := filepath.Join(t.TempDir(), "wt")
	gitRun(t, main, "worktree", "add", "-q", wt)
	s, err := Open(wt)
	if err != nil {
		t.Fatal(err)
	}
	want, _ := filepath.EvalSymlinks(filepath.Join(main, ".git"))
	got, _ := filepath.EvalSymlinks(s.CommonDir())
	if !filepath.IsAbs(s.CommonDir()) || got != want {
		t.Fatalf("CommonDir = %s, want %s", got, want)
	}
}

func TestConfig(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	if v, err := s.Config(ctx, "board.me"); err != nil || v != "" {
		t.Fatalf("unset = %q, %v", v, err)
	}
	if err := s.SetConfig(ctx, "board.me", "abc"); err != nil {
		t.Fatal(err)
	}
	if v, err := s.Config(ctx, "board.me"); err != nil || v != "abc" {
		t.Fatalf("set = %q, %v", v, err)
	}
	if err := s.SetConfig(ctx, "board.me", ""); err != nil {
		t.Fatal(err)
	}
	if v, err := s.Config(ctx, "board.me"); err != nil || v != "" {
		t.Fatalf("emptied = %q, %v", v, err)
	}
}

func TestRefused(t *testing.T) {
	t.Parallel()
	h := commitOf(t, newStore(t))
	for _, c := range []struct {
		msg  string
		prev Hash
		want bool
	}{
		{"cannot lock ref 'r': is at 1 but expected 2", ZeroHash, true},
		{"cannot lock ref 'r': reference already exists", ZeroHash, true},
		{"fatal: Unable to create '/x/refs/board/issue/a.lock': File exists.", ZeroHash, true},
		{"fatal: Unable to create '/x/refs/board/issue/a': File exists.", ZeroHash, false},
		{"cannot lock ref 'r': unable to resolve reference 'r'", h, true},
		{"cannot lock ref 'r': unable to resolve reference 'r'", ZeroHash, false},
	} {
		if got := refused(c.msg, c.prev); got != c.want {
			t.Errorf("refused(%q, %v) = %v", c.msg, c.prev, got)
		}
	}
}
