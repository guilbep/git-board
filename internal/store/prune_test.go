package store_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// pruneSetup is a switched clone holding git-bug's data, here and on its
// origin, next to what a prune keeps: git-board's refs, a branch named like
// git-bug's namespace, and board.me.
func pruneSetup(t *testing.T) (b *store.Backend, dir, origin string) {
	t.Helper()
	dir, origin = storetest.Repo(t, false), storetest.Repo(t, true)
	storetest.Git(t, dir, "remote", "add", "origin", origin)
	storetest.Init(t, dir)
	b = storetest.Open(t, dir)
	c, err := store.StoreCommit(b, "x")
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []string{"refs/bugs/" + forkID('a'), "refs/identities/" + forkID('b'), "refs/releases/" + forkID('c'),
		"refs/remotes/origin/bugs/" + forkID('a'), "refs/remotes/origin/releases/1.2", "refs/heads/main", "refs/board/switched"} {
		if err := b.Store().UpdateRef(t.Context(), ref, c, gitstore.ZeroHash); err != nil {
			t.Fatal(err)
		}
	}
	storetest.Git(t, dir, "push", "-q", "origin", "refs/bugs/*:refs/bugs/*", "refs/releases/*:refs/releases/*", "main")
	storetest.Git(t, dir, "config", "git-bug.identity", forkID('b'))
	storetest.Git(t, dir, "config", "git-bug.bridge.gh.owner", "o")
	if err := os.MkdirAll(filepath.Join(b.Store().CommonDir(), "git-bug", "clocks"), 0o755); err != nil {
		t.Fatal(err)
	}
	return b, dir, origin
}

func TestPrune(t *testing.T) {
	t.Parallel()
	b, dir, origin := pruneSetup(t)
	before, _ := b.Store().Refs("refs/")

	rep, err := b.Prune("origin", true)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Refs["refs/bugs/"] != 1 || rep.Refs["refs/identities/"] != 1 || rep.Refs["refs/releases/"] != 1 ||
		rep.Refs["refs/remotes/origin/bugs/"] != 1 || len(rep.Refs) != 4 {
		t.Fatalf("refs %v", rep.Refs)
	}
	if rep.Cache == "" || strings.Join(rep.Keys, " ") != "git-bug.identity git-bug.bridge.gh.owner" || rep.Remote != 2 || rep.Backup != "" {
		t.Fatalf("report %+v", rep)
	}
	if after, _ := b.Store().Refs("refs/"); len(after) != len(before) {
		t.Fatalf("the dry run removed refs: %d, then %d", len(before), len(after))
	}

	if rep, err = b.Prune("origin", false); err != nil {
		t.Fatal(err)
	}
	after, _ := b.Store().Refs("refs/")
	for name := range after {
		if strings.HasPrefix(name, "refs/bugs/") || strings.HasPrefix(name, "refs/identities/") || strings.HasPrefix(name, "refs/releases/") ||
			strings.HasPrefix(name, "refs/remotes/origin/bugs/") {
			t.Errorf("%s is left", name)
		}
	}
	for _, kept := range []string{"refs/remotes/origin/releases/1.2", "refs/heads/main", "refs/board/switched"} {
		if _, ok := after[kept]; !ok {
			t.Errorf("%s is gone", kept)
		}
	}
	if _, err := os.Stat(rep.Cache); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("git-bug's cache: %v", err)
	}
	if keys := storetest.Git(t, dir, "config", "--get-regexp", "^(git-bug|board)\\."); strings.Contains(keys, "git-bug.") || !strings.Contains(keys, "board.me") {
		t.Errorf("config keys left: %q", keys)
	}
	if left := storetest.Git(t, origin, "for-each-ref", "--format=%(refname)"); left != "refs/heads/main\n" {
		t.Errorf("origin's refs: %q", left)
	}
	if heads := storetest.Git(t, dir, "bundle", "list-heads", rep.Backup); strings.Count(heads, "\n") != 4 {
		t.Errorf("the backup: %q", heads)
	}
}

// A repository whose issues are still git-bug's keeps them: they are the
// only copy until a switch.
func TestPruneWaitsForTheSwitch(t *testing.T) {
	t.Parallel()
	b, dir, _ := pruneSetup(t)
	storetest.Git(t, dir, "update-ref", "-d", "refs/board/switched")
	if _, err := b.Prune("", false); !errors.Is(err, store.ErrNotSwitched) {
		t.Fatalf("got %v, want ErrNotSwitched", err)
	}
	if refs, _ := b.Store().Refs("refs/bugs/"); len(refs) != 1 {
		t.Fatalf("refs/bugs: %v", refs)
	}
}

func TestPruneWithoutGitBug(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	rep, err := b.Prune("", false)
	if err != nil || len(rep.Refs) != 0 || rep.Cache != "" || len(rep.Keys) != 0 || rep.Backup != "" {
		t.Fatalf("%+v, %v", rep, err)
	}
}
