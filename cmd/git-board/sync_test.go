package main

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/git-bug/git-bug/cache"
	"github.com/git-bug/git-bug/repository"
	"github.com/git-bug/git-bug/repository/repotest"

	"github.com/guilbep/git-board/internal/gitbugio"
)

// twoReposWithRemote builds two real, non-bare git-bug repos on disk plus
// a bare "origin" both point at — the same shape git-bug's own pull/push
// tests use (repoA/repoB/remote with AddRemote), needed here because
// runSync/syncOne open real filesystem paths (gitbugio.Open), not
// in-memory repo handles.
func twoReposWithRemote(t *testing.T) (pathA, pathB string) {
	t.Helper()
	repoA, repoB, _ := repotest.SetupGoGitReposAndRemote(t)
	pathA, pathB = filepath.Dir(repoA.GitDir), filepath.Dir(repoB.GitDir)
	// Every repo needs its own local identity set before it can
	// pull/merge at all — not just the one that happens to seed a bug.
	// Caught by a real test failure ("No identity is set") on the repo
	// that only ever pulls, never seeds: identity setup belongs here,
	// not folded into seedBug.
	for _, path := range []string{pathA, pathB} {
		setIdentity(t, path)
	}
	return pathA, pathB
}

func setIdentity(t *testing.T, path string) {
	t.Helper()
	gitDir, err := repository.FindGitDir(path)
	if err != nil {
		t.Fatalf("find %s: %v", path, err)
	}
	repo, err := repository.OpenDefault(gitDir, "git-bug")
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	c, err := cache.NewRepoCacheNoEvents(repo, nil)
	if err != nil {
		t.Fatalf("cache for %s: %v", path, err)
	}
	defer c.Close()
	id, err := c.Identities().NewFromGitUser()
	if err != nil {
		t.Fatalf("identity for %s: %v", path, err)
	}
	if err := c.SetUserIdentity(t.Context(), id); err != nil {
		t.Fatalf("set identity for %s: %v", path, err)
	}
}

func seedBug(t *testing.T, path, title string) {
	t.Helper()
	gitDir, err := repository.FindGitDir(path)
	if err != nil {
		t.Fatalf("find %s: %v", path, err)
	}
	repo, err := repository.OpenDefault(gitDir, "git-bug")
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	c, err := cache.NewRepoCacheNoEvents(repo, nil)
	if err != nil {
		t.Fatalf("cache for %s: %v", path, err)
	}
	defer c.Close()
	// Bugs().New stages the create operation, as any other: Commit
	// writes it to the repo.
	bug, _, err := c.Bugs().New(t.Context(), title, "")
	if err != nil {
		t.Fatalf("new bug in %s: %v", path, err)
	}
	if err := bug.Commit(t.Context()); err != nil {
		t.Fatalf("commit bug in %s: %v", path, err)
	}
}

func bugTitles(t *testing.T, path string) []string {
	t.Helper()
	b, err := gitbugio.Open(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer b.Close()
	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues %s: %v", path, err)
	}
	titles := make([]string, len(issues))
	for i, issue := range issues {
		titles[i] = issue.Title
	}
	return titles
}

func TestSyncPushesThenPullsBetweenTwoRepos(t *testing.T) {
	t.Parallel()
	pathA, pathB := twoReposWithRemote(t)
	seedBug(t, pathA, "seen only in A until sync")

	// A pushes it to the shared remote.
	out, err := execute(t, []string{"sync", pathA})
	if err != nil {
		t.Fatalf("sync A: %v\noutput:\n%s", err, out)
	}

	// B doesn't have it yet.
	if got := bugTitles(t, pathB); len(got) != 0 {
		t.Fatalf("B before its own sync = %v, want empty", got)
	}

	// B pulls it from the shared remote.
	out, err = execute(t, []string{"sync", pathB})
	if err != nil {
		t.Fatalf("sync B: %v\noutput:\n%s", err, out)
	}
	if got := bugTitles(t, pathB); !containsLabel(got, "seen only in A until sync") {
		t.Errorf("B after sync = %v, want the bug pushed from A", got)
	}
}

func TestSyncMultiplePathsInOneCall(t *testing.T) {
	t.Parallel()
	pathA, pathB := twoReposWithRemote(t)
	seedBug(t, pathA, "from A")
	seedBug(t, pathB, "from B")

	// One sync call, both paths: each pushes its own bug and pulls the
	// other's -- order matters (A must push before B can pull it) but
	// this is exactly what "sync every configured repo" needs to do
	// in one command, so cover it end to end rather than just per-path.
	out, err := execute(t, []string{"sync", pathA, pathB})
	if err != nil {
		t.Fatalf("sync: %v\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, pathA) || !strings.Contains(out, pathB) {
		t.Errorf("output = %q, want both paths mentioned", out)
	}

	// A second sync of B now sees A's bug too.
	if _, err := execute(t, []string{"sync", pathB}); err != nil {
		t.Fatalf("second sync B: %v", err)
	}
	if got := bugTitles(t, pathB); !containsLabel(got, "from A") {
		t.Errorf("B after the second sync = %v, want A's bug pulled in", got)
	}
}

func TestSyncDefaultsToCwdWithNoPaths(t *testing.T) {
	newTestRepo(t) // chdirs into a real git-bug repo with a .git-board.toml

	// No remote configured on this fixture -- pull/push against "origin"
	// should fail cleanly (an error, not a panic), proving sync actually
	// tried the current directory rather than silently doing nothing.
	_, err := execute(t, []string{"sync"})
	if err == nil {
		t.Fatal("expected an error (no origin remote configured), got nil")
	}
}

func TestSyncContinuesPastOneFailingPath(t *testing.T) {
	t.Parallel()
	pathA, pathB := twoReposWithRemote(t)
	seedBug(t, pathA, "should still sync despite the bad path")

	_, err := execute(t, []string{"sync", "/nonexistent/path/nope", pathA})
	if err == nil {
		t.Fatal("expected an error naming the bad path, got nil")
	}
	if !strings.Contains(err.Error(), "1 of 2") {
		t.Errorf("error = %q, want it to say 1 of 2 repos failed", err.Error())
	}

	// A's own sync (push) must still have gone through despite the
	// other path failing first.
	if got := bugTitles(t, pathB); len(got) != 0 {
		t.Fatalf("sanity: B shouldn't have it yet, got %v", got)
	}
	if _, err := execute(t, []string{"sync", pathB}); err != nil {
		t.Fatalf("sync B: %v", err)
	}
	if got := bugTitles(t, pathB); !containsLabel(got, "should still sync despite the bad path") {
		t.Errorf("B after pulling = %v, want A's bug (proves A's sync wasn't skipped)", got)
	}
}

// sync fetches the counter and numbers what this clone made since.
func TestSyncNumbers(t *testing.T) {
	t.Parallel()
	pathA, pathB := twoReposWithRemote(t)
	a, err := gitbugio.Open(pathA)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatal(err)
	}
	a.Close()
	seedBug(t, pathB, "Made before B knew")
	var out strings.Builder
	if err := runSync(&out, []string{pathB}, "origin"); err != nil {
		t.Fatalf("sync: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "numbered 1 issue") {
		t.Errorf("sync said:\n%s", out.String())
	}
}
