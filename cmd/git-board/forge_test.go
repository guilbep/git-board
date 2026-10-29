package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/internal/store"
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

// A forge pull over a cut sync's journal says where that sync stopped.
func TestForgePullSaysACutSync(t *testing.T) {
	dir, _, _ := forgeSetup(t)
	board := filepath.Join(dir, ".git", "board")
	if err := os.MkdirAll(board, 0o755); err != nil {
		t.Fatal(err)
	}
	journal := "{\"pid\":1,\"started\":5}\n{\"forge\":\"github.com/o/r\",\"total\":1034}\n"
	if err := os.WriteFile(filepath.Join(board, "sync.lock"), []byte(journal), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := execute(t, []string{"forge", "pull"})
	if err != nil || !strings.Contains(out, "read 0 of 1,034 issues of github.com/o/r: a pull goes on from there") {
		t.Fatalf("pull: %v\n%s", err, out)
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

// A long pull says so at its first issue, then where it is every hundred;
// a short one, and a push, only every hundred.
func TestProgressLines(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		e    store.SyncEvent
		want string
	}{
		{store.SyncEvent{Way: "pull", Done: 1, Total: 1034}, "1,034 issues to read: this takes several minutes"},
		{store.SyncEvent{Way: "pull", Done: 2, Total: 1034}, ""},
		{store.SyncEvent{Way: "pull", Done: 400, Total: 1034}, "pulling 400 of 1,034"},
		{store.SyncEvent{Way: "pull", Done: 1034, Total: 1034}, "pulling 1,034 of 1,034"},
		{store.SyncEvent{Way: "pull", Done: 1, Total: 12}, ""},
		{store.SyncEvent{Way: "push", Done: 1, Total: 1034}, ""},
		{store.SyncEvent{Way: "push", Done: 100, Total: 1034}, "pushing 100 of 1,034"},
	} {
		if got := progressLine(c.e); !strings.HasPrefix(got, c.want) || (c.want == "") != (got == "") {
			t.Errorf("%+v: %q, want %q…", c.e, got, c.want)
		}
	}
}
