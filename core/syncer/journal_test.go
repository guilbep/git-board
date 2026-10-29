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
	"time"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/oplog"
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
	if bar[0] != "7/10" || bar[len(bar)-1] != "10/10" || len(bar) != 4 {
		t.Fatalf("the bar = %v: what was read counts as done, the first step being the first issue read", bar)
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

// Cut says how far a cut run got, per forge, and nothing once a run went
// through, or while one holds the lock.
func TestCutSaysWhereARunStopped(t *testing.T) {
	t.Parallel()
	s, r, _ := tenOld(t)
	if c, err := Cut(s); err != nil || c != nil {
		t.Fatalf("before any run: %+v, %v", c, err)
	}
	cut(t, s, r, "i6")
	c, err := Cut(s)
	if err != nil || c == nil || len(c.Forges) != 1 || c.Forges[0] != (CutForge{Key: key, Read: 6, Total: 10}) || c.At.IsZero() {
		t.Fatalf("after the cut: %+v, %v", c, err)
	}
	j, err := lock(s, 1)
	if err != nil {
		t.Fatal(err)
	}
	if c, _ := Cut(s); c != nil {
		t.Fatalf("while a run holds the lock: %+v", c)
	}
	j.unlock()
	r.then("")
	syncAll(t, s, r)
	if c, err := Cut(s); err != nil || c != nil {
		t.Fatalf("after a run went through: %+v, %v", c, err)
	}
}

// An issue gone, or made confidential, between the listing and its read is
// as if not listed: the run goes on.
func TestAnIssueGoneMidPull(t *testing.T) {
	t.Parallel()
	s, r, _ := tenOld(t)
	g := &gone{reads: r, title: "i4"}
	if _, err := Sync(t.Context(), s, Options{}, g); err != nil {
		t.Fatal(err)
	}
	if n := len(issues(t, s)); n != 9 {
		t.Fatalf("%d issues, want the 9 not gone", n)
	}
}

// gone is a forge whose issue title vanished after the listing.
type gone struct {
	*reads
	title string
}

func (g *gone) Complete(ctx context.Context, is forge.Issue) (forge.Issue, error) {
	if is.Title == g.title {
		return forge.Issue{}, forge.ErrNotFound
	}
	return g.reads.Complete(ctx, is)
}

// A pull that walked its listing to the end isn't cut, though it reached
// fewer issues than listed: one linked to an issue not here is left.
func TestAPullThatWentThroughIsntCut(t *testing.T) {
	t.Parallel()
	s, r, added := tenOld(t)
	st, err := loadState(s, key)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.write(t.Context(), s, key, []oplog.Op{linkOp("a", 1, "nothere", added[3].ID, added[3].Number, "")}); err != nil {
		t.Fatal(err)
	}
	if _, err := Pull(t.Context(), s, Options{}, r); err != nil {
		t.Fatal(err)
	}
	if c, err := Cut(s); err != nil || c != nil {
		t.Fatalf("a pull that went through: %+v, %v", c, err)
	}
}

// The cut's time is when the journal last gained a line: a run after it
// that reads nothing, as a sync of one issue, doesn't move it.
func TestTheCutKeepsItsTime(t *testing.T) {
	t.Parallel()
	s, r, _ := tenOld(t)
	cut(t, s, r, "i6")
	then := time.Now().Add(-time.Hour).Truncate(time.Second)
	if err := os.Chtimes(filepath.Join(s.CommonDir(), "board", "sync.lock"), then, then); err != nil {
		t.Fatal(err)
	}
	r.then("")
	if _, err := Sync(t.Context(), s, Options{Issue: issues(t, s)[0].ID}, r); err != nil {
		t.Fatal(err)
	}
	if c, err := Cut(s); err != nil || c == nil || !c.At.Equal(then) {
		t.Fatalf("after a sync of one issue: %+v, %v, want the cut at %v", c, err, then)
	}
}

// A forge out of the sync, taken out or turned off, takes its journal with
// it: no notice says where it stopped.
func TestAForgeOutOfTheSyncTakesItsJournal(t *testing.T) {
	t.Parallel()
	s, r, _ := tenOld(t)
	cut(t, s, r, "i6")
	if _, err := Pull(t.Context(), s, Options{}, fake.New("example.com/other")); err != nil {
		t.Fatal(err)
	}
	if c, err := Cut(s); err != nil || c != nil {
		t.Fatalf("after a pull without it: %+v, %v", c, err)
	}
}

// An original a run skips, read by one before, is still the issue its
// mirror joins: a pull of a, then of a and b, makes one issue.
func TestASkippedOriginalJoinsItsMirror(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	a, b := fake.New("example.com/a"), fake.New("example.com/b")
	board := forge.Account{ID: "1", Login: "board"}
	for _, f := range []*fake.Forge{a, b} {
		f.Add(forge.Issue{Author: board, Created: 1000, Title: "T", Body: forkMarker('5') + "\nB", Status: "open"})
	}
	if _, err := Pull(t.Context(), s, Options{}, a); err != nil {
		t.Fatal(err)
	}
	if _, err := Pull(t.Context(), s, Options{}, a, b); err != nil {
		t.Fatal(err)
	}
	if all := issues(t, s); len(all) != 1 {
		t.Fatalf("%d issues: b's copy didn't join the one pulled from a", len(all))
	}
}
