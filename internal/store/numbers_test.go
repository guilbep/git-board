package store_test

import (
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// The numbers file: a prefix, where the counter starts, then one line per
// numbered issue. GitHub numbers below start are the issues' own.
func TestCounter(t *testing.T) {
	t.Parallel()
	c, err := store.ParseCounter([]byte("prefix CO2\nstart 3052\n3053 bbb\n3052 aaa\n"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Prefix != "CO2" || c.Start != 3052 || c.ByID["aaa"] != 3052 || c.ByID["bbb"] != 3053 {
		t.Fatalf("parsed %+v", c)
	}
	if got, want := string(c.Bytes()), "prefix CO2\nstart 3052\n3052 aaa\n3053 bbb\n"; got != want {
		t.Errorf("Bytes() = %q, want %q", got, want)
	}
	if c.Next() != 3054 {
		t.Errorf("Next() = %d, want 3054", c.Next())
	}
	empty, _ := store.ParseCounter([]byte("prefix CO2\nstart 3052\n"))
	if empty.Next() != 3052 {
		t.Errorf("Next() of an empty counter = %d, want its start", empty.Next())
	}

	github := func(n string) []store.Forge { return []store.Forge{{Name: "GitHub", Number: n}} }
	for _, tc := range []struct {
		name   string
		id     string
		forges []store.Forge
		want   int
	}{
		{"its line", "aaa", nil, 3052},
		{"its line over its GitHub number", "bbb", github("2518"), 3053},
		{"GitHub, below start", "ccc", github("2518"), 2518},
		{"GitHub, at start: not yet", "ddd", github("3052"), 0},
		{"GitLab's never counts", "eee", []store.Forge{{Name: "GitLab", Number: "34"}}, 0},
		{"nowhere", "fff", nil, 0},
	} {
		if got := c.NumberOf(tc.id, tc.forges); got != tc.want {
			t.Errorf("%s: NumberOf = %d, want %d", tc.name, got, tc.want)
		}
	}
	if c.Format(2518) != "CO2-2518" {
		t.Errorf("Format(2518) = %q", c.Format(2518))
	}

	// a later version's counter: blank lines and keys this one doesn't know
	later, err := store.ParseCounter([]byte("prefix CO2\nstart 1\n\nlease alice 10-20\n3 aaa\n"))
	if err != nil || later.ByID["aaa"] != 3 {
		t.Errorf("a later counter: %+v, %v", later, err)
	}

	for _, bad := range []string{"", "start 3052\n", "prefix CO2\n", "prefix 2CO\nstart 1\n", "prefix CO2\nstart x\n", "prefix CO2\nstart 1\n3\n", "prefix CO2\nstart 1\n3 aaa\n3 bbb\n"} {
		if _, err := store.ParseCounter([]byte(bad)); err == nil {
			t.Errorf("ParseCounter(%q): no error", bad)
		}
	}
}

// Before the switch nothing changes: no counter, no number, and fetching
// from a remote that has none is no error.
func TestNumbersBeforeTheSwitch(t *testing.T) {
	t.Parallel()
	mine, _ := storetest.NewPair(t)
	id := storetest.NewIssue(t, mine, "Crash")
	if err := mine.FetchNumbers("origin"); err != nil {
		t.Fatalf("FetchNumbers: %v", err)
	}
	if n, err := mine.NumberIssues("origin"); n != 0 || err != nil {
		t.Fatalf("NumberIssues = %d, %v", n, err)
	}
	is, err := mine.GetIssue(id)
	if err != nil || is.Counted || is.Number != "" {
		t.Fatalf("GetIssue = %+v, %v", is, err)
	}
}

// GitHub numbers below start are the issues' own, whenever they are
// imported; an issue opened on GitHub after the switch gets the next one.
func TestNumbersGitHubBelowStart(t *testing.T) {
	t.Parallel()
	mine, _ := storetest.NewPair(t)
	old := storetest.NewImportedIssue(t, mine, "Old", "https://github.com/o/r/issues/2518")
	late := storetest.NewImportedIssue(t, mine, "Late", "https://github.com/o/r/issues/3060")
	if err := mine.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatalf("InitNumbers: %v", err)
	}
	pushed(t, mine)
	if n, err := mine.NumberIssues("origin"); n != 1 || err != nil {
		t.Fatalf("NumberIssues = %d, %v; want 1, the late one", n, err)
	}
	for id, want := range map[string]string{old: "CO2-2518", late: "CO2-3052"} {
		if is, err := mine.GetIssue(id); err != nil || is.Number != want || !is.Counted {
			t.Errorf("GetIssue(%s) = %q counted %v, %v; want %s", id[:7], is.Number, is.Counted, err, want)
		}
	}
	c, _, _ := mine.Counter()
	if _, listed := c.ByID[old]; listed {
		t.Error("the counter lists an issue its GitHub number already numbers")
	}

	// an issue's address takes its project number and a forge's, as the
	// palette's jump does, before an id prefix
	for name, want := range map[string]string{"CO2-3052": late, "co2-2518": old, "#3060": late, "#2518": old, late[:7]: late} {
		if is, err := mine.GetIssue(name); err != nil || is.ID != want {
			t.Errorf("GetIssue(%s) = %s, %v; want %s", name, is.ID, err, want[:7])
		}
	}
	for _, name := range []string{"CO2-9999", "#1", "GB-3052"} {
		if _, err := mine.GetIssue(name); !errors.Is(err, store.ErrNotFound) {
			t.Errorf("GetIssue(%s): %v, want not found", name, err)
		}
	}
}

// Two clones number at once: the push is the lock. Theirs, on a counter
// mine has moved, is refused and keeps no number of its own; its next try
// takes the next number.
func TestNumbersPushIsTheLock(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	if err := mine.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatalf("InitNumbers: %v", err)
	}
	if err := theirs.FetchNumbers("origin"); err != nil {
		t.Fatalf("FetchNumbers: %v", err)
	}
	a := storetest.NewIssue(t, mine, "Mine")
	b := storetest.NewIssue(t, theirs, "Theirs")
	pushed(t, mine)
	pushed(t, theirs)
	if n, err := mine.NumberIssues("origin"); n != 1 || err != nil {
		t.Fatalf("mine: %d, %v", n, err)
	}
	if _, rejected, err := store.NumberOnce(theirs, "origin"); !rejected || err == nil {
		t.Fatalf("theirs on a stale counter: rejected %v, %v", rejected, err)
	}
	if c, _, _ := theirs.Counter(); c.ByID[b] != 0 {
		t.Fatalf("a refused push left their issue numbered %d, a number nobody else has", c.ByID[b])
	}
	if n, err := theirs.NumberIssues("origin"); n != 1 || err != nil {
		t.Fatalf("theirs: %d, %v", n, err)
	}
	for _, clone := range []*store.Backend{mine, theirs} {
		if err := clone.FetchNumbers("origin"); err != nil {
			t.Fatal(err)
		}
		if c, _, _ := clone.Counter(); c.ByID[a] != 3052 || c.ByID[b] != 3053 {
			t.Errorf("counter %v; want %s 3052, %s 3053", c.ByID, a[:7], b[:7])
		}
	}
}

// A clone that hasn't fetched the counter yet keeps its issue unnumbered
// and fails nothing; the next fetch and numbering give it its number.
func TestNumbersNotYetFetched(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	if err := mine.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatal(err)
	}
	id := storetest.NewIssue(t, theirs, "Offline")
	pushed(t, theirs)
	if n, err := theirs.NumberIssues("origin"); n != 0 || err != nil {
		t.Fatalf("before fetching: %d, %v", n, err)
	}
	if err := theirs.FetchNumbers("origin"); err != nil {
		t.Fatal(err)
	}
	if is, _ := theirs.GetIssue(id); !is.Counted || is.Number != "" {
		t.Fatalf("fetched, not numbered yet: %+v", is)
	}
	if n, err := theirs.NumberIssues("origin"); n != 1 || err != nil {
		t.Fatalf("after fetching: %d, %v", n, err)
	}
	if is, _ := theirs.GetIssue(id); is.Number != "CO2-3052" {
		t.Fatalf("number %q", is.Number)
	}
}

// Init once: a second init, from either clone, names the counter there is;
// a prefix is a letter then letters or digits.
func TestInitNumbersOnce(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	if err := mine.InitNumbers("origin", "co-2", 3052); err == nil {
		t.Fatal("prefix co-2 taken")
	}
	if err := mine.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatal(err)
	}
	for _, clone := range []*store.Backend{mine, theirs} {
		if err := clone.InitNumbers("origin", "XY", 1); !errors.Is(err, store.ErrNumbered) || !strings.Contains(err.Error(), "CO2, from 3052") {
			t.Errorf("second init: %v", err)
		}
	}
}

// Two clones start the numbers at the very same time: one does, the other
// is told the project numbers its issues already, and takes that counter.
func TestInitNumbersRace(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, clone := range []*store.Backend{mine, theirs} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = clone.InitNumbers("origin", fmt.Sprintf("P%d", i), 1)
		}()
	}
	wg.Wait()
	won := 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case !errors.Is(err, store.ErrNumbered):
			t.Errorf("the loser: %v, want ErrNumbered", err)
		}
	}
	if won != 1 {
		t.Fatalf("%d started the numbers, want 1: %v", won, errs)
	}
	a, _, _ := mine.Counter()
	b, _, _ := theirs.Counter()
	if a.Prefix == "" || a.Prefix != b.Prefix {
		t.Errorf("the clones disagree: %q and %q", a.Prefix, b.Prefix)
	}
}

// Numbering says how the issues stand: before the switch, every issue on
// origin by its GitHub number, for a preview; after, how many have a
// number, how many origin has without one, how many wait for their push.
func TestNumbering(t *testing.T) {
	t.Parallel()
	mine, _ := storetest.NewPair(t)
	storetest.NewImportedIssue(t, mine, "Old", "https://github.com/o/r/issues/2518")
	storetest.NewImportedIssue(t, mine, "Late", "https://github.com/o/r/issues/3060")
	storetest.NewIssue(t, mine, "Made here, pushed")
	pushed(t, mine)
	storetest.NewIssue(t, mine, "Made here, only here")

	before, err := mine.Numbering("origin")
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(slices.Sorted(slices.Values(before.GitHub))); got != "[0 2518 3060]" {
		t.Errorf("before: GitHub numbers on origin %s, want [0 2518 3060]", got)
	}
	if before.Numbered != 0 || before.Waiting != 3 || before.Unpushed != 1 || before.Highest != 3060 {
		t.Errorf("before: %+v", before)
	}

	if err := mine.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatal(err)
	}
	if n, _ := mine.Numbering("origin"); n.Numbered != 1 || n.Waiting != 2 || n.Unpushed != 1 {
		t.Errorf("started: %+v, want #2518 numbered, 2 waiting, 1 to push", n)
	}
	if _, err := mine.NumberIssues("origin"); err != nil {
		t.Fatal(err)
	}
	if n, _ := mine.Numbering("origin"); n.Numbered != 3 || n.Waiting != 0 || n.Unpushed != 1 {
		t.Errorf("numbered: %+v, want 3 numbered, 1 to push", n)
	}
}

// Two clones number at the very same time, round after round: whoever
// loses the push takes the next number, whatever the server's refusal says.
func TestNumbersRealRace(t *testing.T) {
	t.Parallel()
	mine, theirs := storetest.NewPair(t)
	if err := mine.InitNumbers("origin", "CO2", 1); err != nil {
		t.Fatal(err)
	}
	if err := theirs.FetchNumbers("origin"); err != nil {
		t.Fatal(err)
	}
	for round := range 8 {
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, clone := range []*store.Backend{mine, theirs} {
			storetest.NewIssue(t, clone, fmt.Sprintf("round %d, clone %d", round, i))
			pushed(t, clone)
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, errs[i] = clone.NumberIssues("origin")
			}()
		}
		wg.Wait()
		if err := errors.Join(errs...); err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
	}
	_ = mine.FetchNumbers("origin")
	c, _, _ := mine.Counter()
	seen := map[int]string{}
	for id, n := range c.ByID {
		if other, dup := seen[n]; dup {
			t.Errorf("%d is both %s and %s", n, other[:7], id[:7])
		}
		seen[n] = id
	}
	if len(c.ByID) != 16 {
		t.Errorf("%d issues numbered, want 16", len(c.ByID))
	}
}

// An origin that also pushes to a mirror (co2's pushes to GitHub, then
// GitLab): a number counts once it lands where origin fetches from,
// whatever the mirror says.
func TestNumbersOriginWithAMirror(t *testing.T) {
	t.Parallel()
	remote, clone := storetest.Repo(t, true), storetest.Repo(t, false)
	storetest.Git(t, clone, "remote", "add", "origin", remote)
	for _, u := range []string{remote, filepath.Join(t.TempDir(), "mirror-gone.git")} {
		storetest.Git(t, clone, "config", "--add", "remote.origin.pushurl", u)
	}
	storetest.Init(t, clone)
	b := storetest.Open(t, clone)

	if err := b.InitNumbers("origin", "CO2", 1); err != nil {
		t.Fatalf("InitNumbers: %v", err)
	}
	storetest.NewIssue(t, b, "Made here")
	_, _ = b.Push("origin") // origin gets it; the mirror refuses, as for the counter
	if n, err := b.NumberIssues("origin"); n != 1 || err != nil {
		t.Fatalf("NumberIssues = %d, %v; want 1, nil", n, err)
	}
}

// A counter this git-board can't read shows the issues without numbers,
// and numbering refuses to write on top of it.
func TestNumbersUnreadableCounter(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	storetest.NewIssue(t, b, "Still listed")
	if err := store.WriteCounter(b, []byte("not a counter")); err != nil {
		t.Fatal(err)
	}
	issues, err := b.ListIssues()
	if err != nil || len(issues) != 1 || issues[0].Counted {
		t.Fatalf("ListIssues = %+v, %v", issues, err)
	}
	if _, err := b.NumberIssues("origin"); err == nil {
		t.Error("numbered on top of a counter it can't read")
	}
}

// The counter names only issues origin has: one made here waits for its
// push, so other clones never see a number for an issue they can't get.
func TestNumbersOnlyWhatOriginHas(t *testing.T) {
	t.Parallel()
	mine, _ := storetest.NewPair(t)
	if err := mine.InitNumbers("origin", "CO2", 1); err != nil {
		t.Fatal(err)
	}
	id := storetest.NewIssue(t, mine, "Only here")
	if n, err := mine.NumberIssues("origin"); n != 0 || err != nil {
		t.Fatalf("before its push: numbered %d, %v", n, err)
	}
	if _, err := mine.Push("origin"); err != nil {
		t.Fatal(err)
	}
	if n, err := mine.NumberIssues("origin"); n != 1 || err != nil {
		t.Fatalf("after its push: numbered %d, %v", n, err)
	}
	if is, _ := mine.GetIssue(id); is.Number != "CO2-1" {
		t.Errorf("number %q", is.Number)
	}
}

// pushed sends b's issues to origin, as a sync or the Sync page's Push
// does before numbering.
func pushed(t *testing.T, b *store.Backend) {
	t.Helper()
	if _, err := b.Push("origin"); err != nil {
		t.Fatal(err)
	}
}

// A counter ahead of origin's, as switch --apply leaves it until its push,
// isn't taken back by a fetch: that would drop the re-key, and the next
// numbering would give the switched issues new numbers.
func TestFetchKeepsNumbersOriginHasnt(t *testing.T) {
	t.Parallel()
	mine, _ := storetest.NewPair(t)
	old := storetest.NewIssue(t, mine, "Old")
	if err := mine.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatal(err)
	}
	pushed(t, mine)
	if n, err := mine.NumberIssues("origin"); n != 1 || err != nil {
		t.Fatal(n, err)
	}
	nu := storetest.NewIssue(t, mine, "New") // the switch's issue for old
	c, _, _ := mine.Counter()
	c.ByID[nu] = c.ByID[old]
	delete(c.ByID, old)
	if err := store.CommitCounter(mine, c); err != nil {
		t.Fatal(err)
	}
	if err := mine.FetchNumbers("origin"); !errors.Is(err, store.ErrNumbersAhead) {
		t.Fatalf("err = %v, want ErrNumbersAhead", err)
	}
	if _, err := mine.NumberIssues("origin"); !errors.Is(err, store.ErrNumbersAhead) {
		t.Fatalf("numbering: err = %v, want ErrNumbersAhead", err)
	}
	if c2, _, _ := mine.Counter(); c2.ByID[nu] != 3052 {
		t.Fatalf("the re-key is gone: %v", c2.ByID)
	}
	if ahead, err := mine.NumbersAhead("origin"); !ahead || err != nil {
		t.Fatalf("NumbersAhead = %v, %v; want true", ahead, err)
	}
	storetest.Git(t, mine.Store().CommonDir(), "push", "-q", "origin", "refs/board/seq")
	if ahead, err := mine.NumbersAhead("origin"); ahead || err != nil {
		t.Fatalf("NumbersAhead after the push = %v, %v; want false", ahead, err)
	}
	if err := mine.FetchNumbers("origin"); err != nil {
		t.Fatal(err)
	}
}
