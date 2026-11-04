package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// A board kept on the forges alone: its copy is in the machine's folder,
// nothing of it in the repository's refs; each write is told to OnWrite;
// a copy thrown away is pulled back, the same issues once.
func TestForgeOnlyBoard(t *testing.T) {
	t.Parallel()
	dir := storetest.Repo(t, false)
	storetest.Git(t, dir, "config", store.DatastoreKey, store.ForgeOnly)
	storetest.Init(t, dir)
	b := storetest.Open(t, dir)
	if c := b.Capabilities(); !c.ForgeOnly || c.Replica || c.Counter {
		t.Fatalf("capabilities %+v", c)
	}
	var told []string
	b.OnWrite = func(id string) { told = append(told, id) }
	id, err := b.NewIssue("kept on the forge", "its body", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.AddComment(id, "a comment"); err != nil {
		t.Fatal(err)
	}
	if len(told) != 2 || told[0] != id || told[1] != id {
		t.Fatalf("told %v; want %s twice", told, id)
	}
	if refs := storetest.Git(t, dir, "for-each-ref", "refs/board/"); refs != "" {
		t.Fatalf("refs in the repository: %s", refs)
	}
	gitDir := strings.TrimSpace(storetest.Git(t, dir, "rev-parse", "--absolute-git-dir"))
	if _, err := os.Stat(filepath.Join(gitDir, "board", "forge-copy.git", "HEAD")); err != nil {
		t.Fatalf("the copy: %v", err)
	}
	if err := b.InitNumbers(store.NumbersRemote, "CO2", 1); !errors.Is(err, datastore.ErrMissing) {
		t.Fatalf("numbers: %v, want ErrMissing", err)
	}

	// not on the forge yet: the copy isn't thrown away
	if _, err := b.DropCopy(false); !errors.Is(err, store.ErrNotOnForge) {
		t.Fatalf("drop with a write waiting: %v, want ErrNotOnForge", err)
	}
	f := fake.New("example.com/p")
	if _, err := b.SyncForges(t.Context(), run(f)); err != nil {
		t.Fatal(err)
	}
	if n, err := b.DropCopy(false); err != nil || n != 0 {
		t.Fatalf("drop: %d, %v", n, err)
	}
	if issues, err := b.ListIssues(); err != nil || len(issues) != 0 {
		t.Fatalf("after the drop %d issues, %v; want none", len(issues), err)
	}
	for range 2 {
		if _, err := b.SyncForges(t.Context(), run(f)); err != nil {
			t.Fatal(err)
		}
	}
	issues, err := b.ListIssues()
	if err != nil || len(issues) != 1 || issues[0].Title != "kept on the forge" {
		t.Fatalf("pulled back %+v, %v; want the one issue", issues, err)
	}
	if all := f.All(); len(all) != 1 || len(all[0].Comments) != 1 {
		t.Fatalf("forge %+v; want one issue, one comment", all)
	}
}
