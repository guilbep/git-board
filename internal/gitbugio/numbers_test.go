package gitbugio_test

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/git-bug/git-bug/repository/repotest"

	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// The numbers file: a prefix, where the counter starts, then one line per
// numbered issue. GitHub numbers below start are the issues' own.
func TestCounter(t *testing.T) {
	c, err := gitbugio.ParseCounter([]byte("prefix CO2\nstart 3052\n3053 bbb\n3052 aaa\n"))
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
	empty, _ := gitbugio.ParseCounter([]byte("prefix CO2\nstart 3052\n"))
	if empty.Next() != 3052 {
		t.Errorf("Next() of an empty counter = %d, want its start", empty.Next())
	}

	github := func(n string) []gitbugio.Forge { return []gitbugio.Forge{{Name: "GitHub", Number: n}} }
	for _, tc := range []struct {
		name   string
		id     string
		forges []gitbugio.Forge
		want   int
	}{
		{"its line", "aaa", nil, 3052},
		{"its line over its GitHub number", "bbb", github("2518"), 3053},
		{"GitHub, below start", "ccc", github("2518"), 2518},
		{"GitHub, at start: not yet", "ddd", github("3052"), 0},
		{"GitLab's never counts", "eee", []gitbugio.Forge{{Name: "GitLab", Number: "34"}}, 0},
		{"nowhere", "fff", nil, 0},
	} {
		if got := c.NumberOf(tc.id, tc.forges); got != tc.want {
			t.Errorf("%s: NumberOf = %d, want %d", tc.name, got, tc.want)
		}
	}
	if c.Format(2518) != "CO2-2518" {
		t.Errorf("Format(2518) = %q", c.Format(2518))
	}

	for _, bad := range []string{"", "start 3052\n", "prefix CO2\n", "prefix 2CO\nstart 1\n", "prefix CO2\nstart x\n", "prefix CO2\nstart 1\nnot-a-line\n"} {
		if _, err := gitbugio.ParseCounter([]byte(bad)); err == nil {
			t.Errorf("ParseCounter(%q): no error", bad)
		}
	}
}

// Before the switch nothing changes: no counter, no number, and fetching
// from a remote that has none is no error.
func TestNumbersBeforeTheSwitch(t *testing.T) {
	mine, _ := gitbugiotest.NewPair(t)
	id := gitbugiotest.NewIssue(t, mine, "Crash")
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
	mine, _ := gitbugiotest.NewPair(t)
	old := gitbugiotest.NewImportedIssue(t, mine, "Old", "https://github.com/o/r/issues/2518")
	late := gitbugiotest.NewImportedIssue(t, mine, "Late", "https://github.com/o/r/issues/3060")
	if err := mine.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatalf("InitNumbers: %v", err)
	}
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
}

// Two clones number at once: the push is the lock. Theirs, on a counter
// mine has moved, is refused and keeps no number of its own; its next try
// takes the next number.
func TestNumbersPushIsTheLock(t *testing.T) {
	mine, theirs := gitbugiotest.NewPair(t)
	if err := mine.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatalf("InitNumbers: %v", err)
	}
	if err := theirs.FetchNumbers("origin"); err != nil {
		t.Fatalf("FetchNumbers: %v", err)
	}
	a := gitbugiotest.NewIssue(t, mine, "Mine")
	b := gitbugiotest.NewIssue(t, theirs, "Theirs")
	if n, err := mine.NumberIssues("origin"); n != 1 || err != nil {
		t.Fatalf("mine: %d, %v", n, err)
	}
	if _, rejected, err := gitbugio.NumberOnce(theirs, "origin"); !rejected || err == nil {
		t.Fatalf("theirs on a stale counter: rejected %v, %v", rejected, err)
	}
	if c, _, _ := theirs.Counter(); c.ByID[b] != 0 {
		t.Fatalf("a refused push left their issue numbered %d, a number nobody else has", c.ByID[b])
	}
	if n, err := theirs.NumberIssues("origin"); n != 1 || err != nil {
		t.Fatalf("theirs: %d, %v", n, err)
	}
	for _, clone := range []*gitbugio.Backend{mine, theirs} {
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
	mine, theirs := gitbugiotest.NewPair(t)
	if err := mine.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatal(err)
	}
	id := gitbugiotest.NewIssue(t, theirs, "Offline")
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
	mine, theirs := gitbugiotest.NewPair(t)
	if err := mine.InitNumbers("origin", "co-2", 3052); err == nil {
		t.Fatal("prefix co-2 taken")
	}
	if err := mine.InitNumbers("origin", "CO2", 3052); err != nil {
		t.Fatal(err)
	}
	for _, clone := range []*gitbugio.Backend{mine, theirs} {
		if err := clone.InitNumbers("origin", "XY", 1); err == nil || !strings.Contains(err.Error(), "CO2, from 3052") {
			t.Errorf("second init: %v", err)
		}
	}
}

// Two clones number at the very same time, round after round: whoever
// loses the push takes the next number, whatever the server's refusal says.
func TestNumbersRealRace(t *testing.T) {
	mine, theirs := gitbugiotest.NewPair(t)
	if err := mine.InitNumbers("origin", "CO2", 1); err != nil {
		t.Fatal(err)
	}
	if err := theirs.FetchNumbers("origin"); err != nil {
		t.Fatal(err)
	}
	for round := range 8 {
		var wg sync.WaitGroup
		errs := make([]error, 2)
		for i, clone := range []*gitbugio.Backend{mine, theirs} {
			gitbugiotest.NewIssue(t, clone, fmt.Sprintf("round %d, clone %d", round, i))
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
	remote := repotest.NewGoGitRepo(t, true)
	clone := repotest.NewGoGitRepo(t, false)
	repotest.AddRemote(t, clone, "origin", remote.GitDir)
	for _, u := range []string{remote.GitDir, filepath.Join(t.TempDir(), "mirror-gone.git")} {
		if out, err := exec.Command("git", "-C", clone.GitDir, "config", "--add", "remote.origin.pushurl", u).CombinedOutput(); err != nil {
			t.Fatalf("%v: %s", err, out)
		}
	}
	b, err := gitbugio.NewForRepo(clone)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = b.Close() })
	id, err := b.TestCache().Identities().NewFromGitUser()
	if err != nil {
		t.Fatal(err)
	}
	if err := b.TestCache().SetUserIdentity(id); err != nil {
		t.Fatal(err)
	}

	if err := b.InitNumbers("origin", "CO2", 1); err != nil {
		t.Fatalf("InitNumbers: %v", err)
	}
	gitbugiotest.NewIssue(t, b, "Made here")
	if n, err := b.NumberIssues("origin"); n != 1 || err != nil {
		t.Fatalf("NumberIssues = %d, %v; want 1, nil", n, err)
	}
}
