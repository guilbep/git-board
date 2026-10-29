package syncer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/gitstore"
)

// reads is a fake whose reads of an issue the pull reaches are told, by
// title, and one of which fails as a dropped connection would.
type reads struct {
	*fake.Forge
	mu     sync.Mutex
	read   []string
	failOn string
}

func (r *reads) Complete(ctx context.Context, is forge.Issue) (forge.Issue, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if is.Title == r.failOn {
		return is, errors.New("the connection dropped")
	}
	r.read = append(r.read, is.Title)
	return r.Forge.Complete(ctx, is)
}

// then is the reads since the last, which it forgets, with the next to fail.
func (r *reads) then(failOn string) []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	got := r.read
	r.read, r.failOn = nil, failOn
	return got
}

// tenOld is a forge of ten issues, i0 to i9, not updated for long.
func tenOld(t *testing.T) (*gitstore.Store, *reads, []forge.Issue) {
	t.Helper()
	f := fake.New(key)
	var added []forge.Issue
	for i := range 10 {
		added = append(added, f.Add(forge.Issue{Author: ann(), Title: fmt.Sprint("i", i), Status: "open", Created: int64(1000 + i)}))
	}
	return newStore(t), &reads{Forge: f}, added
}

// cut runs a sync that stops at the issue failOn, as a lost connection
// stops it.
func cut(t *testing.T, s *gitstore.Store, r *reads, failOn string) {
	t.Helper()
	r.then(failOn)
	if _, err := Sync(t.Context(), s, Options{}, r); err == nil || !strings.Contains(err.Error(), "the connection dropped") {
		t.Fatalf("the cut run: %v", err)
	}
}

func titles(l []string) string { return strings.Join(l, " ") }

// A sync cut mid-pull leaves what it read; the next reads only the rest.
func TestACutPullGoesOn(t *testing.T) {
	t.Parallel()
	s, r, _ := tenOld(t)
	cut(t, s, r, "i6")
	if n := len(issues(t, s)); n != 6 {
		t.Fatalf("the cut run pulled %d issues, want 6", n)
	}
	r.then("")
	var bar []string
	opt := Options{Progress: func(_, way string, done, total int, _ string) {
		if way == "pull" {
			bar = append(bar, fmt.Sprint(done, "/", total))
		}
	}}
	if _, err := Sync(t.Context(), s, opt, r); err != nil {
		t.Fatal(err)
	}
	if got := r.then(""); titles(got) != "i6 i7 i8 i9" {
		t.Fatalf("going on read %q, want i6 to i9", got)
	}
	if n := len(issues(t, s)); n != 10 {
		t.Fatalf("%d issues, want 10", n)
	}
	if !slices.Contains(bar, "6/10") || bar[0] != "1/10" {
		t.Fatalf("the bar = %v: what was read counts as done", bar)
	}
}

// An issue read by the cut run, and changed on the forge since, is read
// again.
func TestGoingOnReadsWhatChanged(t *testing.T) {
	t.Parallel()
	s, r, added := tenOld(t)
	cut(t, s, r, "i6")
	if err := r.SetTitle(t.Context(), added[2].Ref(), "i2, renamed"); err != nil {
		t.Fatal(err)
	}
	r.then("")
	syncAll(t, s, r)
	if got := r.then(""); titles(got) != "i2, renamed i6 i7 i8 i9" {
		t.Fatalf("going on read %q", got)
	}
	var renamed bool
	for _, is := range issues(t, s) {
		renamed = renamed || is.Title == "i2, renamed"
	}
	if !renamed {
		t.Fatal("the rename wasn't pulled")
	}
}

// An issue linked by an earlier, complete run, changed since, and not
// reached by the cut run, is read: the journal says nothing of it. (i1,
// reached, changed a moment before: within the margin, it's read again.)
func TestGoingOnReadsWhatItDidntReach(t *testing.T) {
	t.Parallel()
	s, r, added := tenOld(t)
	syncAll(t, s, r)
	for _, i := range []int{1, 5} {
		if err := r.SetTitle(t.Context(), added[i].Ref(), fmt.Sprint("i", i, ", renamed")); err != nil {
			t.Fatal(err)
		}
	}
	cut(t, s, r, "i5, renamed")
	r.then("")
	syncAll(t, s, r)
	if got := r.then(""); !slices.Contains(got, "i5, renamed") {
		t.Fatalf("going on read %q, without i5", got)
	}
}

// A second cut keeps what the first read, and adds its own.
func TestTwoCutsInARow(t *testing.T) {
	t.Parallel()
	s, r, _ := tenOld(t)
	cut(t, s, r, "i4")
	cut(t, s, r, "i8")
	if got := r.then(""); titles(got) != "i4 i5 i6 i7" {
		t.Fatalf("the second cut run read %q", got)
	}
	syncAll(t, s, r)
	if got := r.then(""); titles(got) != "i8 i9" {
		t.Fatalf("going on read %q", got)
	}
}

// A sync of one issue neither reads the journal nor changes it.
func TestOneIssueLeavesTheJournal(t *testing.T) {
	t.Parallel()
	s, r, _ := tenOld(t)
	cut(t, s, r, "i6")
	r.then("")
	if _, err := Sync(t.Context(), s, Options{Issue: issues(t, s)[0].ID}, r); err != nil {
		t.Fatal(err)
	}
	r.then("")
	syncAll(t, s, r)
	if got := r.then(""); titles(got) != "i6 i7 i8 i9" {
		t.Fatalf("going on read %q", got)
	}
}

// A run that records its run leaves the journal empty: the next lists from
// that run's start.
func TestACompleteRunEmptiesTheJournal(t *testing.T) {
	t.Parallel()
	s, r, _ := tenOld(t)
	cut(t, s, r, "i6")
	r.then("")
	syncAll(t, s, r)
	data, err := os.ReadFile(filepath.Join(s.CommonDir(), "board", "sync.lock"))
	if err != nil {
		t.Fatal(err)
	}
	if lines := strings.Split(strings.TrimSpace(string(data)), "\n"); len(lines) != 1 {
		t.Fatalf("the journal holds %d lines past the run's:\n%s", len(lines)-1, data)
	}
}

// A mirror the fork posted, waiting for its original when the run is cut,
// isn't journaled: the next run reads it, and it joins.
func TestAWaitingMirrorIsntReached(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a, b := forkPair()
	ra, rb := &reads{Forge: a, failOn: "T"}, &reads{Forge: b}
	if _, err := Pull(t.Context(), s, Options{}, rb, ra); err == nil { // b first: its mirror waits for a's original
		t.Fatal("the cut run didn't stop")
	}
	rb.then("")
	ra.then("")
	if _, err := Pull(t.Context(), s, Options{}, rb, ra); err != nil {
		t.Fatal(err)
	}
	if got := rb.then(""); titles(got) != "T" {
		t.Fatalf("going on read %q of b, want its mirror", got)
	}
	if all := issues(t, s); len(all) != 1 {
		t.Fatalf("%d issues: the mirror didn't join", len(all))
	}
}
