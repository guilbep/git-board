package forgecopy

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/guilbep/git-board/next/store"
	"github.com/guilbep/git-board/next/store/storetest"
)

// open is the copy of a new clone, in its folder inside its .git, as
// the board's folder is; and the clone.
func open(t *testing.T) (*Store, string) {
	t.Helper()
	clone := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", clone).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	s, err := Open(context.Background(), filepath.Join(clone, ".git", "board"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, clone
}

func TestConformance(t *testing.T) {
	t.Parallel()
	storetest.Run(t, storetest.Options{Open: func(t *testing.T) store.Datastore { s, _ := open(t); return s }})
}

// The copy is its own repository, in the clone's folder: a write leaves
// the clone's refs alone, and it keeps no counter.
func TestTheCloneUntouched(t *testing.T) {
	t.Parallel()
	s, clone := open(t)
	if s.Folder() != filepath.Join(clone, ".git", "board") || s.Caps().Counter != nil {
		t.Fatalf("Folder = %s, Counter = %v: want the clone's, and none", s.Folder(), s.Caps().Counter)
	}
	if _, err := s.Add(context.Background(), "issue", storetest.ID(1), storetest.Pack(1), ""); err != nil {
		t.Fatal(err)
	}
	if out, _ := exec.Command("git", "-C", clone, "for-each-ref", "refs/board").Output(); len(out) > 0 {
		t.Fatalf("a write to the copy wrote the clone's refs: %s", out)
	}
	if hs, _ := s.Heads(context.Background(), "issue"); len(hs) != 1 {
		t.Fatalf("Heads = %v", hs)
	}
}
