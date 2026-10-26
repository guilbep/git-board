package main

import (
	"context"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// twoReposWithRemote builds two real repositories on disk plus a bare
// "origin" both point at, each with someone to write as: runSync and
// syncOne open real filesystem paths (store.Open).
func twoReposWithRemote(t *testing.T) (pathA, pathB string) {
	t.Helper()
	remote := storetest.Repo(t, true)
	pathA, pathB = storetest.Repo(t, false), storetest.Repo(t, false)
	for _, path := range []string{pathA, pathB} {
		storetest.Git(t, path, "remote", "add", "origin", remote)
		storetest.Init(t, path)
	}
	return pathA, pathB
}

func seedBug(t *testing.T, path, title string) {
	t.Helper()
	storetest.NewIssue(t, storetest.Open(t, path), title)
}

func bugTitles(t *testing.T, path string) []string {
	t.Helper()
	b, err := store.Open(path)
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

	// A pushes it to the shared remote, with git alone.
	out, err := execute(t, []string{"sync", "--git", pathA})
	if err != nil || strings.Contains(out, "forge") {
		t.Fatalf("sync A: %v\noutput:\n%s", err, out)
	}

	// B doesn't have it yet.
	if got := bugTitles(t, pathB); len(got) != 0 {
		t.Fatalf("B before its own sync = %v, want empty", got)
	}

	// B pulls it from the shared remote.
	out, err = execute(t, []string{"sync", "--git", pathB})
	if err != nil {
		t.Fatalf("sync B: %v\noutput:\n%s", err, out)
	}
	if got := bugTitles(t, pathB); !containsLabel(got, "seen only in A until sync") {
		t.Errorf("B after sync = %v, want the bug pushed from A", got)
	}
}

// The whole sync: git pull, the forges found in the remotes, git push;
// its check writes nothing, here, on origin or on the forge. Serial: it
// swaps the forges' opener and reader.
func TestSyncTheWhole(t *testing.T) {
	pathA, pathB := twoReposWithRemote(t)
	storetest.ForgeOrigin(t, pathA, "https://github.com/o/r.git")
	f := fake.New("github.com/o/r")
	f.Add(forge.Issue{Title: "From GitHub", Body: "b", Author: forge.Account{ID: "7", Login: "alice"}})
	stubForges(t, f)
	storetest.GitHub(t, map[string]string{"o/r": "o/r"})

	out, err := execute(t, []string{"sync", "--check", pathA})
	if err != nil || !strings.Contains(out, "forge github, found in the remotes: github.com o/r\n") ||
		!strings.Contains(out, "git fetch from origin...") || !strings.Contains(out, "github: would pull a new issue") ||
		!strings.Contains(out, "git push to origin: skipped") || !strings.Contains(out, "checked: nothing written") {
		t.Fatalf("check: %v\n%s", err, out)
	}
	if got := bugTitles(t, pathA); len(got) != 0 || len(f.Writes()) != 0 {
		t.Fatalf("the check wrote %v here, %v on the forge", got, f.Writes())
	}

	out, err = execute(t, []string{"sync", pathA})
	if err != nil || !strings.Contains(out, "git pull from origin...") || !strings.Contains(out, "github: pull a new issue") ||
		!strings.Contains(out, "the GitHub Project: skipped, no [github-project]") || !strings.Contains(out, "git push to origin...") {
		t.Fatalf("sync: %v\n%s", err, out)
	}
	if out, err := execute(t, []string{"sync", "--git", pathB}); err != nil {
		t.Fatalf("sync B: %v\n%s", err, out)
	}
	if got := bugTitles(t, pathB); !containsLabel(got, "From GitHub") {
		t.Errorf("origin after the sync: %v, want the forge's issue", got)
	}
}

// stubForges opens f for every forge a run opens.
func stubForges(t *testing.T, f *fake.Forge) {
	t.Helper()
	saved := openForges
	t.Cleanup(func() { openForges = saved })
	openForges = func(_ context.Context, fs []config.Forge) ([]store.NamedForge, error) {
		var out []store.NamedForge
		for _, c := range fs {
			out = append(out, store.NamedForge{Name: c.Name, Forge: f})
		}
		return out, nil
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
	// tried the current directory rather than silently doing nothing. The
	// whole sync skips the git steps instead.
	_, err := execute(t, []string{"sync", "--git"})
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
	a, err := store.Open(pathA)
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

// A repository whose issues are still git-bug's, in refs/bugs, and that no
// switch marked, isn't synced: the sync says to run git-board switch.
func TestSyncWaitsForTheSwitch(t *testing.T) {
	t.Parallel()
	path, _ := twoReposWithRemote(t)
	storetest.Git(t, path, "update-ref", "refs/bugs/"+strings.Repeat("a", 64), strings.TrimSpace(storetest.Git(t, path, "hash-object", "-w", "--stdin")))
	out, err := execute(t, []string{"sync", path})
	if err == nil || !strings.Contains(out, "run git-board switch") {
		t.Fatalf("err = %v\n%s", err, out)
	}
}

// A plain sync creates nothing on a forge it never synced with: it says
// how many wait, and --allow-create makes them.
func TestSyncCreatesWithConsent(t *testing.T) {
	pathA, _ := twoReposWithRemote(t)
	for _, title := range []string{"internal: rotate the keys", "b", "c"} {
		seedBug(t, pathA, title)
	}
	storetest.ForgeOrigin(t, pathA, "https://github.com/o/r.git")
	f := fake.New("github.com/o/r")
	stubForges(t, f)
	storetest.GitHub(t, map[string]string{"o/r": "o/r"})

	out, err := execute(t, []string{"sync", pathA})
	if err != nil || len(f.All()) != 0 || !strings.Contains(out, "3 issues not created: they need consent, the first sync with this forge (--allow-create)") {
		t.Fatalf("plain sync: %v, the forge holds %d\n%s", err, len(f.All()), out)
	}
	if out, err := execute(t, []string{"sync", "--allow-create", pathA}); err != nil || len(f.All()) != 3 {
		t.Fatalf("with consent: %v, the forge holds %d\n%s", err, len(f.All()), out)
	}
}
