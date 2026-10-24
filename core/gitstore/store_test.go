package gitstore

import (
	"errors"
	"os/exec"
	"testing"
)

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
	if err := s.UpdateRef(name, c1, ZeroHash); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRef(name, c2, c1); err != nil {
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

func TestUpdateRefRefusesMoved(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c1 := commitOf(t, s)
	c2 := commitOf(t, s, c1)
	c3 := commitOf(t, s, c2)
	const name = "refs/board/issue/x"
	if err := s.UpdateRef(name, c2, ZeroHash); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRef(name, c3, c1); !errors.Is(err, ErrRefMoved) {
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
	if err := s.UpdateRef(name, c1, ZeroHash); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateRef(name, c1, ZeroHash); !errors.Is(err, ErrRefMoved) {
		t.Fatalf("err = %v", err)
	}
}
