package app

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/guilbep/git-board/conf"
	"github.com/guilbep/git-board/domain/issues/issuesync"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
	bsync "github.com/guilbep/git-board/sync"
)

func actionIDs(s State) []string {
	var ids []string
	for _, a := range s.Next {
		ids = append(ids, a.ID)
	}
	return ids
}

// A new clone is "new": no file, no identity, no forge, nothing run; its
// next actions say so, the identity first.
func TestStateOfANewBoard(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	b, err := Open(t.Context(), dir, Options{Who: true})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	s, err := b.State(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if s.Phase != "new" || s.File.State != "none" || s.Me.State != "none" || s.Datastore.State != "git" ||
		len(s.Forges) != 0 || s.Project.State != "none" || s.Run.State != "idle" || s.Issues != 0 {
		t.Fatalf("state %+v", s)
	}
	if ids := actionIDs(s); !slices.Equal(ids, []string{"me", "forge"}) {
		t.Errorf("next %q", ids)
	}
	j, err := bsync.Lock(b.Kernel.Store(), 1) // a run, as the CLI's would hold it
	if err != nil {
		t.Fatal(err)
	}
	if s, err := b.State(t.Context()); err != nil || s.Run.State != "running" {
		t.Errorf("run %+v, %v", s.Run, err)
	}
	j.Unlock()
	if s, err := b.State(t.Context()); err != nil || s.Run.State != "idle" {
		t.Errorf("run %+v, %v", s.Run, err)
	}
}

// A forge table walks no-token, first, synced.
func TestStateOfAForge(t *testing.T) { // swaps FindToken: serial
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	toml := fmt.Sprintf("schema = %d\n\n[forges.gh]\nkind = \"github\"\nproject = \"o/r\"\nsync = true\nway = \"pull\"\n\n[github-project]\nowner = \"o\"\n", conf.Current)
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	token, asked := "", 0
	was := FindToken
	FindToken = func(context.Context, forge.Table, string) (string, error) { asked++; return token, nil }
	t.Cleanup(func() { FindToken = was })

	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()
	state := func() State {
		s, err := b.State(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	s := state()
	if len(s.Forges) != 1 || s.Forges[0].State != "no-token" || s.Forges[0].Way != "pull" || s.File.State != "current" {
		t.Fatalf("state %+v", s)
	}
	if s.Project.State != "none" { // a table without its number names no Project
		t.Errorf("project %+v", s.Project)
	}
	if ids := actionIDs(s); !slices.Equal(ids, []string{"me", "login:gh"}) {
		t.Errorf("next %q", ids)
	}

	// a token not found is asked each time: one added shows at once
	n := asked
	state()
	if asked == n {
		t.Errorf("no token: not asked again")
	}
	token = "t"
	if s := state(); s.Forges[0].State != "first" || s.Forges[0].Token == "" {
		t.Fatalf("forge %+v", s.Forges[0])
	} else if i := slices.Index(actionIDs(s), "sync:gh"); i < 0 || s.Next[i].Say != "Run the first sync with gh (o/r)." {
		t.Errorf("next %+v", s.Next) // a pull creates nothing there: no consent to ask
	}
	// a token found is remembered: git's credential helper isn't asked again
	n = asked
	if s := state(); s.Forges[0].Token == "" || asked != n {
		t.Errorf("found: token %q, asked %d more times", s.Forges[0].Token, asked-n)
	}

	if _, err := issuesync.CreatePerson(t.Context(), b.Kernel, issuesync.NewPerson("", "me", "")); err != nil {
		t.Fatal(err)
	}
	f := fake.New("github.com/o/r") // the key the GitHub adapter gives o/r
	f.Add(forge.Issue{Author: forge.Account{ID: "7", Login: "ann"}, Title: "There", Body: "made there"})
	if _, err := b.SyncForges(t.Context(), WayPull, ForgeRun{Forges: []NamedForge{{Name: "gh", Forge: f}}, See: func(SyncEvent) {}}); err != nil {
		t.Fatal(err)
	}
	if s := state(); s.Forges[0].State != "synced" || s.Issues != 1 {
		t.Fatalf("state %+v", s)
	}
}
