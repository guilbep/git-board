package main

import (
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// forgeSetup is a clone of a bare origin that names the fake GitHub
// project o/r, which holds "From GitHub", while the clone holds "Local
// only". It chdirs into the clone and swaps the forges' opener: its tests
// stay serial.
func forgeSetup(t *testing.T) (dir, origin string, f *fake.Forge) {
	t.Helper()
	origin, dir = storetest.Repo(t, true), storetest.Repo(t, false)
	storetest.Git(t, dir, "remote", "add", "origin", origin)
	storetest.Init(t, dir)
	seedBug(t, dir, "Local only")
	storetest.ForgeOrigin(t, dir, "https://github.com/o/r.git")
	f = fake.New("github.com/o/r")
	f.Add(forge.Issue{Title: "From GitHub", Body: "b", Author: forge.Account{ID: "7", Login: "alice"}})
	stubForges(t, f)
	storetest.GitHub(t, map[string]string{"o/r": "o/r"})
	t.Chdir(dir)
	return dir, origin, f
}

// A forge pull takes the forge's issues and sends nothing: no issue made
// on the forge, and no git step, so origin gets nothing either.
func TestForgePullTakesOnly(t *testing.T) {
	dir, origin, f := forgeSetup(t)

	out, err := execute(t, []string{"forge", "pull", "--check"})
	if err != nil || !strings.Contains(out, "github: would pull a new issue") || !strings.Contains(out, "checked: nothing written") {
		t.Fatalf("check: %v\n%s", err, out)
	}
	if got := bugTitles(t, dir); len(got) != 1 || len(f.Writes()) != 0 {
		t.Fatalf("the check wrote: here %v, on the forge %v", got, f.Writes())
	}

	if out, err = execute(t, []string{"forge", "pull"}); err != nil {
		t.Fatalf("pull: %v\n%s", err, out)
	}
	if got := bugTitles(t, dir); !containsLabel(got, "From GitHub") {
		t.Errorf("after the pull: %v", got)
	}
	if len(f.Writes()) != 0 || len(f.All()) != 1 {
		t.Errorf("the pull wrote on the forge: %v", f.Writes())
	}
	if refs := storetest.Git(t, origin, "for-each-ref", "refs/board"); refs != "" {
		t.Errorf("origin got %q", refs)
	}
}

// A forge push sends this clone's issues and takes nothing.
func TestForgePushSendsOnly(t *testing.T) {
	dir, _, f := forgeSetup(t)
	if out, err := execute(t, []string{"forge", "push", "--allow-create"}); err != nil {
		t.Fatalf("push: %v\n%s", err, out)
	}
	if len(f.All()) != 2 {
		t.Errorf("the forge holds %d issues, want Local only made there", len(f.All()))
	}
	if got := bugTitles(t, dir); containsLabel(got, "From GitHub") {
		t.Errorf("the push took the forge's issue: %v", got)
	}
}

func TestForgeNeedsAForge(t *testing.T) {
	path, _ := twoReposWithRemote(t)
	t.Chdir(path)
	out, err := execute(t, []string{"forge", "sync"})
	if err == nil || !strings.Contains(err.Error(), "no forge has its sync on") {
		t.Fatalf("err = %v\n%s", err, out)
	}
}
