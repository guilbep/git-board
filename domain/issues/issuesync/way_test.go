package issuesync

import (
	"strings"
	"testing"

	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
	bsync "github.com/guilbep/git-board/sync"
)

// A forge's way ([forges.<name>] way, ADR 0022): the run pulls from
// those that pull, and pushes to those that push. Options.Ways states it,
// by the forge's key.

// ways is Options with each forge's way, by its key.
func ways(fw ...any) Options {
	o := Options{Ways: map[string]bsync.Way{}}
	for i := 0; i < len(fw); i += 2 {
		o.Ways[fw[i].(*fake.Forge).Key()] = fw[i+1].(bsync.Way)
	}
	return o
}

// A push-only forge takes the issues, and gives nothing back: a sync
// creates there, and an edit made on it never reaches the board.
func TestWayPushOnly(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c := NewCreate("a", "T", "B")
	if _, err := CreateIssue(t.Context(), s, c); err != nil {
		t.Fatal(err)
	}
	fb, fp := fake.New(key), fake.New(key+"2")
	if _, err := Sync(t.Context(), s, ways(fp, bsync.Push), fb, fp); err != nil {
		t.Fatal(err)
	}
	if len(fb.All()) != 1 || len(fp.All()) != 1 {
		t.Fatalf("created %d on the both-way forge, %d on the push-only one", len(fb.All()), len(fp.All()))
	}
	fp.Edit("1", func(is *forge.Issue) { is.Title = "changed on the backup" })
	if _, err := Sync(t.Context(), s, ways(fp, bsync.Push), fb, fp); err != nil {
		t.Fatal(err)
	}
	if _, here := fb.Get("1"); !here {
		t.Fatal("the issue is not on the both-way forge")
	}
	if is := only(t, s); is.Title != "T" {
		t.Fatalf("the backup's edit reached the board: title %q", is.Title)
	}
}

// A pull-only forge gives its issues, and takes nothing: a sync pulls
// from it, and writes to it never, its issue linked or not.
func TestWayPullOnly(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	fl := fake.New(key)
	fl.Add(sample())
	r, err := Sync(t.Context(), s, ways(fl, bsync.Pull), fl)
	if err != nil {
		t.Fatal(err)
	}
	if r.Created != 1 {
		t.Fatalf("report = %+v, want one created", r)
	}
	is := only(t, s)
	if is.Title != "T" || is.Status != "closed" {
		t.Fatalf("issue = %q %q", is.Title, is.Status)
	}
	if _, err := Sync(t.Context(), s, ways(fl, bsync.Pull), fl); err != nil {
		t.Fatal(err)
	}
	if got, _ := fl.Get("1"); got.Title != "T" || len(fl.Writes()) != 0 {
		t.Fatalf("the run wrote to the read-only forge: %+v, writes %d", got, len(fl.Writes()))
	}
}

// A push-only forge is pushed to through its link, and a pull run leaves
// it alone; a pull-only one is pulled from, and a push run leaves it.
func TestWayRuns(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	fb, fp, fl := fake.New(key), fake.New(key+"2"), fake.New(key+"3")
	fl.Add(sample())
	if _, err := Sync(t.Context(), s, ways(fp, bsync.Push, fl, bsync.Pull), fb, fp, fl); err != nil {
		t.Fatal(err)
	}
	// the pull-only forge's issue is here; the push-only one holds a copy
	if r, err := Push(t.Context(), s, ways(fl, bsync.Pull), fl); err != nil || r.Pushed != 0 {
		t.Fatalf("push to the read-only forge: %+v, %v", r, err)
	}
	if r, err := Pull(t.Context(), s, ways(fp, bsync.Push), fp); err != nil || r.Updated+r.Created != 0 {
		t.Fatalf("pull from the backup: %+v, %v", r, err)
	}
	// an edit here goes to the push-only forge, through the run that
	// links it by marker
	if _, err := DoIssue(t.Context(), s, only(t, s).ID, SetTitle("me", "T2")); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(t.Context(), s, ways(fp, bsync.Push, fl, bsync.Pull), fb, fp, fl); err != nil {
		t.Fatal(err)
	}
	if got, _ := fp.Get("1"); got.Title != "T2" {
		t.Fatalf("the backup took no title: %q", got.Title)
	}
	if got, _ := fl.Get("1"); got.Title != "T" {
		t.Fatalf("the read-only forge took a title: %q", got.Title)
	}
}

// A backup equals the board: what was changed there alone is put back
// at the next sync, and the report counts it.
func TestWayBackupPutBack(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	if _, err := CreateIssue(t.Context(), s, NewCreate("a", "T", "B")); err != nil {
		t.Fatal(err)
	}
	fp := fake.New(key)
	if _, err := Sync(t.Context(), s, ways(fp, bsync.Push), fp); err != nil {
		t.Fatal(err)
	}
	fp.Edit("1", func(is *forge.Issue) {
		is.Title, is.Status, is.Labels = "changed on the backup", "closed", append(is.Labels, "stray")
	})
	r, err := Sync(t.Context(), s, ways(fp, bsync.Push), fp)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := fp.Get("1"); got.Title != "T" || got.Status != "open" || len(got.Labels) != 0 {
		t.Fatalf("the backup kept its own edit: %q %q %v", got.Title, got.Status, got.Labels)
	}
	if r.Restored != 1 {
		t.Fatalf("report = %+v, want one put back", r)
	}
	if is := only(t, s); is.Title != "T" || is.Status != "open" {
		t.Fatalf("the backup's edit reached the board: %q %q", is.Title, is.Status)
	}
}

// A backup's edit never reaches the board, even when the board changed
// the same issue in the meantime: the run is a sync, the backup's way
// is push, and push wins.
func TestWayBackupNeverFeedsTheBoard(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	if _, err := CreateIssue(t.Context(), s, NewCreate("a", "T", "B")); err != nil {
		t.Fatal(err)
	}
	fp := fake.New(key)
	if _, err := Sync(t.Context(), s, ways(fp, bsync.Push), fp); err != nil {
		t.Fatal(err)
	}
	fp.Edit("1", func(is *forge.Issue) { is.Body = "edited on the backup" })
	if _, err := DoIssue(t.Context(), s, only(t, s).ID, SetTitle("me", "T2")); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(t.Context(), s, ways(fp, bsync.Push), fp); err != nil {
		t.Fatal(err)
	}
	if is := only(t, s); is.Body != "B" || is.Title != "T2" {
		t.Fatalf("board = %q %q, want T2 B", is.Title, is.Body)
	}
	if got, _ := fp.Get("1"); got.Title != "T2" || !strings.Contains(got.Body, "B") || strings.Contains(got.Body, "edited on the backup") {
		t.Fatalf("backup = %q %q, want T2 and the board's body", got.Title, got.Body)
	}
}

// A backup made both ways: what is edited there from then on is pulled,
// as on any forge; the records it kept as a backup make it a sync of what
// changed since, not a first sync.
func TestWayBackupBecomesBoth(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	if _, err := CreateIssue(t.Context(), s, NewCreate("a", "T", "B")); err != nil {
		t.Fatal(err)
	}
	fp := fake.New(key)
	if _, err := Sync(t.Context(), s, ways(fp, bsync.Push), fp); err != nil {
		t.Fatal(err)
	}
	fp.Edit("1", func(is *forge.Issue) { is.Title = "edited once it syncs both ways" })
	r, err := Sync(t.Context(), s, ways(fp, bsync.Both), fp)
	if err != nil {
		t.Fatal(err)
	}
	if is := only(t, s); is.Title != "edited once it syncs both ways" || r.Created != 0 || r.Restored != 0 || len(fp.All()) != 1 {
		t.Fatalf("board %q, report %+v, %d on the forge", is.Title, r, len(fp.All()))
	}
}
