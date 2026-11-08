package issuesync

import (
	"testing"

	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
	bsync "github.com/guilbep/git-board/sync"
)

// A forge's way ([forges.<name>] way, ADR 0022): the run pulls from
// those that pull, and pushes to those that push. counted states it;
// the tests wrap the fake with one.

type wayForge struct {
	forge.Forge
	way bsync.Way
}

var _ interface{ ForgeWay() bsync.Way } = wayForge{}

func (w wayForge) ForgeWay() bsync.Way { return w.way }

func withWay(f *fake.Forge, way bsync.Way) wayForge { return wayForge{f, way} }

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
	if _, err := Sync(t.Context(), s, Options{}, fb, withWay(fp, bsync.Push)); err != nil {
		t.Fatal(err)
	}
	if len(fb.All()) != 1 || len(fp.All()) != 1 {
		t.Fatalf("created %d on the both-way forge, %d on the push-only one", len(fb.All()), len(fp.All()))
	}
	fp.Edit("1", func(is *forge.Issue) { is.Title = "changed on the backup" })
	if _, err := Sync(t.Context(), s, Options{}, fb, withWay(fp, bsync.Push)); err != nil {
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
	r, err := Sync(t.Context(), s, Options{}, withWay(fl, bsync.Pull))
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
	if _, err := Sync(t.Context(), s, Options{}, withWay(fl, bsync.Pull)); err != nil {
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
	if _, err := Sync(t.Context(), s, Options{}, fb, withWay(fp, bsync.Push), withWay(fl, bsync.Pull)); err != nil {
		t.Fatal(err)
	}
	// the pull-only forge's issue is here; the push-only one holds a copy
	if r, err := Push(t.Context(), s, Options{}, withWay(fl, bsync.Pull)); err != nil || r.Pushed != 0 {
		t.Fatalf("push to the read-only forge: %+v, %v", r, err)
	}
	if r, err := Pull(t.Context(), s, Options{}, withWay(fp, bsync.Push)); err != nil || r.Updated+r.Created != 0 {
		t.Fatalf("pull from the backup: %+v, %v", r, err)
	}
	// an edit here goes to the push-only forge, through the run that
	// links it by marker
	if _, err := DoIssue(t.Context(), s, only(t, s).ID, SetTitle("me", "T2")); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(t.Context(), s, Options{}, fb, withWay(fp, bsync.Push), withWay(fl, bsync.Pull)); err != nil {
		t.Fatal(err)
	}
	if got, _ := fp.Get("1"); got.Title != "T2" {
		t.Fatalf("the backup took no title: %q", got.Title)
	}
	if got, _ := fl.Get("1"); got.Title != "T" {
		t.Fatalf("the read-only forge took a title: %q", got.Title)
	}
}
