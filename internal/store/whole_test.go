package store_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// steps runs a whole sync, the forge and Project steps recorded, and
// answers every step's state as heard: "pull running", "pull done"…
func steps(t *testing.T, b *store.Backend, w store.Whole, forges error) ([]string, error) {
	t.Helper()
	var heard []string
	w.Forges = func(context.Context) error { heard = append(heard, "forges ran"); return forges }
	w.Project = func(context.Context) error { heard = append(heard, "project ran"); return nil }
	w.Step = func(name, state, note string) { heard = append(heard, name+" "+state) }
	err := b.SyncWhole(t.Context(), w)
	return heard, err
}

// The whole sync pulls, syncs the forges and the Project, then pushes; its
// check fetches and compares and pushes nothing.
func TestSyncWhole(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	storetest.NewIssue(t, mine, "Export CSV")
	before := mine.RefsFingerprint()

	heard, err := steps(t, mine, store.Whole{Remote: "origin", DryRun: true}, nil)
	if want := "pull running,pull done,forges running,forges ran,forges done,project running,project ran,project done,push skipped"; err != nil || strings.Join(heard, ",") != want {
		t.Fatalf("check: %v\n%s\nwant %s", err, strings.Join(heard, ","), want)
	}
	if mine.RefsFingerprint() != before {
		t.Error("the check wrote here")
	}
	if err := theirs.Pull("origin"); err != nil {
		t.Fatal(err)
	}
	if is, _ := theirs.ListIssues(); len(is) != 0 {
		t.Fatalf("the check pushed %d issues", len(is))
	}

	heard, err = steps(t, mine, store.Whole{Remote: "origin"}, nil)
	if want := "pull running,pull done,forges running,forges ran,forges done,project running,project ran,project done,push running,push done"; err != nil || strings.Join(heard, ",") != want {
		t.Fatalf("sync: %v\n%s", err, strings.Join(heard, ","))
	}
	if err := theirs.Pull("origin"); err != nil {
		t.Fatal(err)
	}
	if is, _ := theirs.ListIssues(); len(is) != 1 {
		t.Fatalf("pushed %d issues, want 1", len(is))
	}
}

// A step that fails stops the steps after it: a forge sync that fails
// leaves the Project alone and pushes nothing.
func TestSyncWholeStopsAtAFailure(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	storetest.NewIssue(t, mine, "Export CSV")
	boom := errors.New("boom")
	heard, err := steps(t, mine, store.Whole{Remote: "origin"}, boom)
	if !errors.Is(err, boom) || !strings.HasPrefix(err.Error(), "forges: ") {
		t.Fatalf("err = %v, want the forges' failure", err)
	}
	if want := "pull running,pull done,forges running,forges ran,forges failed,project skipped,push skipped"; strings.Join(heard, ",") != want {
		t.Errorf("heard %s", strings.Join(heard, ","))
	}
	if err := theirs.Pull("origin"); err != nil {
		t.Fatal(err)
	}
	if is, _ := theirs.ListIssues(); len(is) != 0 {
		t.Fatalf("pushed %d issues after a failure", len(is))
	}
}

// Without the remote the git steps are skipped, and the forges still sync.
func TestSyncWholeWithoutTheRemote(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	heard, err := steps(t, b, store.Whole{Remote: "origin"}, nil)
	if want := "pull skipped,forges running,forges ran,forges done,project running,project ran,project done,push skipped"; err != nil || strings.Join(heard, ",") != want {
		t.Fatalf("%v\n%s", err, strings.Join(heard, ","))
	}
}

// A counter ahead of origin's fails the check at its git step, the
// counter kept: git push origin refs/board/seq first.
func TestSyncWholeChecksTheNumbers(t *testing.T) {
	t.Parallel()
	mine, _ := storetest.NewPair(t)
	if err := mine.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatal(err)
	}
	c, _, _ := mine.Counter()
	c.ByID[storetest.NewIssue(t, mine, "Made here")] = 3052
	if err := store.CommitCounter(mine, c); err != nil {
		t.Fatal(err)
	}
	heard, err := steps(t, mine, store.Whole{Remote: "origin", DryRun: true}, nil)
	if !errors.Is(err, store.ErrNumbersAhead) || !strings.Contains(err.Error(), "git push origin refs/board/seq") || heard[1] != "pull failed" {
		t.Fatalf("err = %v, heard %v", err, heard)
	}
	if c2, _, _ := mine.Counter(); len(c2.ByID) != 1 {
		t.Errorf("the check changed the counter: %v", c2.ByID)
	}
}
