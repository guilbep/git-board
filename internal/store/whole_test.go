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

// A failed forge step doesn't stop the push: what it wrote and linked
// here must reach origin, or another clone creates it again. The run
// still fails, naming the step.
func TestSyncWholePushesAfterAForgeFailure(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	storetest.NewIssue(t, mine, "Export CSV")
	boom := errors.New("boom")
	heard, err := steps(t, mine, store.Whole{Remote: "origin"}, boom)
	if !errors.Is(err, boom) || !strings.HasPrefix(err.Error(), "forges: ") {
		t.Fatalf("err = %v, want the forges' failure", err)
	}
	if want := "pull running,pull done,forges running,forges ran,forges failed,project running,project ran,project done,push running,push done"; strings.Join(heard, ",") != want {
		t.Errorf("heard %s", strings.Join(heard, ","))
	}
	if err := theirs.Pull("origin"); err != nil {
		t.Fatal(err)
	}
	if is, _ := theirs.ListIssues(); len(is) != 1 {
		t.Fatalf("pushed %d issues after a forge failure, want 1", len(is))
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

// A counter strictly ahead of origin's (origin's is one of its
// ancestors: a switch's, or a numbering stopped before its push) is sent
// by the sync's git step, the numbers kept; its check says so and writes
// nothing. One apart from origin's is refused.
func TestSyncWholeSendsTheNumbers(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	if err := mine.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatal(err)
	}
	c, _, _ := mine.Counter()
	made := storetest.NewIssue(t, mine, "Made here")
	c.ByID[made] = 3052
	if err := store.CommitCounter(mine, c); err != nil {
		t.Fatal(err)
	}
	var note string
	w := store.Whole{Remote: "origin", DryRun: true, Step: func(name, state, n string) {
		if name == store.StepPull && state == "done" {
			note = n
		}
	}}
	if err := mine.SyncWhole(t.Context(), w); err != nil || !strings.Contains(note, "would send refs/board/seq") {
		t.Fatalf("check: %v, pull note %q", err, note)
	}
	if c2, _, _ := mine.Counter(); c2.ByID[made] != 3052 {
		t.Fatalf("the check changed the counter: %v", c2.ByID)
	}
	if _, err := steps(t, mine, store.Whole{Remote: "origin"}, nil); err != nil {
		t.Fatal(err)
	}
	if c2, _, _ := mine.Counter(); c2.ByID[made] != 3052 {
		t.Fatalf("the sync renumbered: %v", c2.ByID)
	}
	if err := theirs.Pull("origin"); err != nil {
		t.Fatal(err)
	}
	if err := theirs.FetchNumbers("origin"); err != nil {
		t.Fatal(err)
	}
	if c3, _, _ := theirs.Counter(); c3.ByID[made] != 3052 {
		t.Fatalf("origin's counter: %v, want %s at 3052", c3.ByID, made)
	}

	// apart: origin moved on, and this clone numbered alone
	other := storetest.NewIssue(t, theirs, "Made there")
	if _, err := theirs.Push("origin"); err != nil {
		t.Fatal(err)
	}
	if _, err := theirs.NumberIssues("origin"); err != nil {
		t.Fatal(err)
	}
	c4, _, _ := mine.Counter()
	c4.ByID[storetest.NewIssue(t, mine, "Made alone")] = 9999
	if err := store.CommitCounter(mine, c4); err != nil {
		t.Fatal(err)
	}
	heard, err := steps(t, mine, store.Whole{Remote: "origin", DryRun: true}, nil)
	if !errors.Is(err, store.ErrNumbersAhead) || heard[1] != "pull failed" {
		t.Fatalf("apart: err = %v, heard %v (origin numbered %s)", err, heard, other)
	}
}
