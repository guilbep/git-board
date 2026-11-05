package app

import (
	"os/exec"
	"slices"
	"testing"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/issues/issuesync"
	"github.com/guilbep/git-board/next/forge"
	"github.com/guilbep/git-board/next/forge/fake"
)

// A forge run on an open board: what it pushes and pulls is the board's,
// and ls shows where each issue is.
func TestSyncForges(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	me, err := issuesync.CreatePerson(t.Context(), b.Kernel, issuesync.NewPerson("", "me", ""))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := issuesync.CreateIssue(t.Context(), b.Kernel, issuesync.NewCreate(me.ID, "Here", "made here")); err != nil {
		t.Fatal(err)
	}
	f := fake.New("example.com/p")
	f.Add(forge.Issue{Author: forge.Account{ID: "7", Login: "ann"}, Title: "There", Body: "made there"})
	var events []string
	run := ForgeRun{Forges: []NamedForge{{"p", f}}, See: func(e SyncEvent) { events = append(events, e.Forge+" "+e.What) }}

	// a first sync creates nothing without consent
	if _, err := b.SyncForges(t.Context(), WaySync, run); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(events, "p held") || !slices.Contains(events, "p created") || len(f.All()) != 1 {
		t.Fatalf("events %q, %d forge issues", events, len(f.All()))
	}
	run.AllowCreate = true
	if _, err := b.SyncForges(t.Context(), WayPush, run); err != nil {
		t.Fatal(err)
	}
	if len(f.All()) != 2 || !slices.Contains(events, "p posted") {
		t.Fatalf("events %q, %d forge issues", events, len(f.All()))
	}
	list, err := b.Issues.List(t.Context(), issues.ListReq{})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || len(list[0].Forges) != 1 || len(list[1].Forges) != 1 {
		t.Fatalf("ls = %+v", list)
	}
}
