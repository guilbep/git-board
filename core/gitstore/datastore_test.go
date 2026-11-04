package gitstore_test

import (
	"bytes"
	"os/exec"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/datastore/dstest"
	"github.com/guilbep/git-board/core/datastore/memstore"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/people"
)

func TestDatastore(t *testing.T) {
	t.Parallel()
	dstest.Run(t, func(t *testing.T) datastore.Datastore {
		dir := t.TempDir()
		if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
			t.Fatalf("git init: %v: %s", err, out)
		}
		s, err := gitstore.Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		return s
	})
}

func gitRepo(t *testing.T) *gitstore.Store {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	s, err := gitstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// Add refuses a pack whose clock doesn't follow its parent's: written, it
// would leave an entity Walk refuses.
func TestAddRefusesAClockThatDoesntFit(t *testing.T) {
	t.Parallel()
	ctx, s := t.Context(), gitRepo(t)
	id := strings.Repeat("a", 40)
	if _, err := s.Add(ctx, "person", id, []byte(`{"clock":2,"ops":[{"kind":"x"}]}`), ""); err == nil {
		t.Error("a first pack of clock 2 was taken")
	}
	v, err := s.Add(ctx, "person", id, []byte(`{"clock":1,"ops":[{"kind":"x"}]}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Add(ctx, "person", id, []byte(`{"clock":3,"ops":[{"kind":"y"}]}`), v); err == nil {
		t.Error("a pack of clock 3 on one of clock 1 was taken")
	}
	if _, err := s.Add(ctx, "person", id, []byte(`{"clock":2,"ops":[{"kind":"y"}]}`), v); err != nil {
		t.Error(err)
	}
}

// Copying into git a person written in several packs, which another
// datastore gives in any order, chains them by clock: the copy loads.
func TestCopyIntoGitChainsByClock(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	src := gitRepo(t)
	p, err := people.Create(ctx, src, people.NewCreate("", "ann", ""))
	if err != nil {
		t.Fatal(err)
	}
	for _, n := range []string{"b", "c", "d"} {
		if _, err := people.Do(ctx, src, p.ID, people.SetName(p.ID, n)); err != nil {
			t.Fatal(err)
		}
	}
	packs, _, err := src.Packs(ctx, "person", p.ID)
	if err != nil {
		t.Fatal(err)
	}
	slices.SortFunc(packs, func(a, b []byte) int { return -bytes.Compare(a, b) }) // not the order they chain in
	keep := memstore.New(t.TempDir())
	v := ""
	for _, pk := range packs {
		if v, err = keep.Add(ctx, "person", p.ID, pk, v); err != nil {
			t.Fatal(err)
		}
	}
	dst := gitRepo(t)
	if _, err := datastore.Copy(ctx, keep, dst); err != nil {
		t.Fatal(err)
	}
	got, err := people.Load(dst, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "d" {
		t.Errorf("name %q, want d", got.Name)
	}
}
