package issuesync

import (
	"context"
	"errors"
	"fmt"
	bsync "github.com/guilbep/git-board/sync"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
	"github.com/guilbep/git-board/kernel"
)

func syncAll(t *testing.T, s *kernel.Board, fs ...forge.Forge) Report {
	t.Helper()
	r, err := Sync(t.Context(), s, Options{}, fs...)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// The first link in the order wins, for an issue and for a forge issue; so
// does the first posted record, for a comment and for a forge comment.
func TestStateFirstWins(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	st, _ := loadState(s, key)
	ops := []kernel.Op{
		linkOp("a", 1, "i1", "100", "1", ""),
		linkOp("a", 1, "i2", "100", "1", ""), // the forge issue is taken
		linkOp("a", 1, "i1", "200", "2", ""), // the issue is taken
		postedOp("a", 1, "i1", "c1", "f1"),
		postedOp("a", 1, "i1", "c1", "f2"), // the comment is recorded
		postedOp("a", 1, "i1", "c2", "f1"), // the forge comment is recorded
		postedOp("a", 1, "i2", "c3", "f3"), // i2 isn't linked
	}
	if err := st.write(t.Context(), s, key, ops); err != nil {
		t.Fatal(err)
	}
	for _, st := range []*state{st, must(loadState(s, key))} {
		if !reflect.DeepEqual(st.links, map[string]string{"100": "i1"}) || !st.anyLink["i2"] {
			t.Fatalf("links = %v, any %v", st.links, st.anyLink)
		}
		if !reflect.DeepEqual(st.posted, map[[2]string]string{{"i1", "c1"}: "f1"}) {
			t.Fatalf("posted = %v", st.posted)
		}
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// A first write of the sync entity while another holds its ref's lock
// tries again, then fails, writing nothing.
func TestStateFirstWriteLocked(t *testing.T) {
	t.Parallel()
	dir := newRepo(t)
	s := open(t, dir)
	lock := filepath.Join(dir, ".git", "refs", "board", "sync", string(bsync.StateID(key))+".lock")
	if err := os.MkdirAll(filepath.Dir(lock), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	st, _ := loadState(s, key)
	if err := st.write(t.Context(), s, key, []kernel.Op{linkOp("a", 1, "i", "100", "1", "")}); !errors.Is(err, kernel.ErrMoved) {
		t.Fatalf("err = %v, want ErrRefMoved", err)
	}
	if len(refs(t, s)) != 0 {
		t.Fatal("wrote")
	}
}

// An issue pulled, the pull stopping before its link, is found by its
// origin and linked: the next pull creates no second one.
func TestPullFindsUnlinkedByOrigin(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	ev := &kernel.Origin{Forge: key, Event: "issue:100"}
	c := NewCreate("a", "T", "B")
	c.Origin = ev
	is, err := CreateIssue(t.Context(), s, c)
	if err != nil {
		t.Fatal(err)
	}
	pull(t, s, f)
	if ids, _ := ListIssues(s); len(ids) != 1 {
		t.Fatalf("issues = %v, want the one found", ids)
	}
	st, _ := loadState(s, key)
	if st.links["100"] != is.ID {
		t.Fatalf("links = %v", st.links)
	}
}

// Two clones pull one forge comment, the forge editing it in between.
// Merged, the issue keeps the first copy and seen the later text: the copy
// came from the forge, so the forge's edit is pulled, never pushed over.
func TestTwoPullsOfOneComment(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	pull(t, s, f) // clone one: "hi"
	is := only(t, s)
	f.Edit("1", func(i *forge.Issue) { i.Comments[0].Body = "hi!" })
	// clone two pulled "hi!", wrote its copy and its seen; merged here.
	dup := NewComment(is.Comments[0].Author, "hi!")
	dup.Origin = is.Comments[0].Origin
	if _, err := DoIssue(t.Context(), s, is.ID, dup); err != nil {
		t.Fatal(err)
	}
	st, _ := loadState(s, key)
	if err := st.write(t.Context(), s, key, []kernel.Op{seenOp(is.Author, 1, is.ID, "comment:c1", "hi!")}); err != nil {
		t.Fatal(err)
	}
	if only(t, s).Comments[0].Body != "hi" {
		t.Fatal("setup: the first copy should hold")
	}
	syncAll(t, s, f)
	got, _ := f.Get("1")
	if got.Comments[0].Body != "hi!" || only(t, s).Comments[0].Body != "hi!" {
		t.Fatalf("forge %q, git-board %q; want hi! on both", got.Comments[0].Body, only(t, s).Comments[0].Body)
	}
}

// Two clones pull one forge issue: merged, the second issue's link is
// ignored, and that duplicate goes to no project.
func TestDuplicateNotMirrored(t *testing.T) {
	t.Parallel()
	s, a, b := newStore(t), fake.New(key), fake.New("example.com/q")
	a.Add(sample())
	pull(t, s, a)
	first := only(t, s)
	c := NewCreate("a", "T", "B")
	c.Origin = first.Origin
	dup, err := CreateIssue(t.Context(), s, c)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := loadState(s, key)
	if err := st.write(t.Context(), s, key, []kernel.Op{linkOp("a", 1, dup.ID, "100", "1", "")}); err != nil {
		t.Fatal(err)
	}
	syncAll(t, s, a, b)
	if n := len(b.All()); n != 1 {
		t.Fatalf("b has %d issues, want the first only", n)
	}
	if n := len(a.All()); n != 1 {
		t.Fatalf("a has %d issues", n)
	}
}

// A run holding the lock keeps another out, which says so.
func TestRunLock(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	j, err := bsync.Lock(s.Store(), 1)
	if err != nil {
		t.Fatal(err)
	}
	_, err = Sync(t.Context(), s, Options{}, f)
	if !errors.Is(err, bsync.ErrLocked) || !strings.Contains(err.Error(), fmt.Sprintf("process %d", os.Getpid())) {
		t.Fatalf("err = %v, want bsync.ErrLocked naming process %d", err, os.Getpid())
	}
	j.Unlock()
	syncAll(t, s, f)
}

// The fake lists an issue without its comments, as GitHub and GitLab do:
// the pull completes each issue it reaches, and the push, which plans
// against the forge's comments, never posts one again.
func TestAPartialListingPostsNothingTwice(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	if list, _ := f.Issues(t.Context(), 0); len(list) != 1 || !list[0].Partial || list[0].Comments != nil {
		t.Fatalf("the fake's listing isn't partial: %+v", list)
	}
	syncAll(t, s, f)
	is := only(t, s)
	if len(is.Comments) != 1 || is.Comments[0].Body != "hi" {
		t.Fatalf("pulled comments = %+v", is.Comments)
	}
	if _, err := DoIssue(t.Context(), s, is.ID, NewComment("a", "mine")); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		syncAll(t, s, f)
	}
	fi, _ := f.Issue(t.Context(), forge.Ref{ID: "100", Number: "1"})
	if len(fi.Comments) != 2 {
		t.Fatalf("forge comments = %+v", fi.Comments)
	}
	if is := only(t, s); len(is.Comments) != 2 {
		t.Fatalf("git-board comments = %+v", is.Comments)
	}
}

// Progress hears each issue the pull reaches and each the push walks,
// done of a total known from the start, with the issue's name.
func TestProgressCountsEveryIssue(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	for i, title := range []string{"one", "two", "three"} {
		f.Add(forge.Issue{Author: ann(), Title: title, Status: "open", Created: int64(1000 + i)}) // the push walks them oldest first
	}
	var got []string
	opt := Options{Progress: func(k, way string, done, total int, name string) {
		if k == key {
			got = append(got, fmt.Sprintf("%s %d/%d %s", way, done, total, name))
		}
	}}
	if _, err := Sync(t.Context(), s, opt, f); err != nil {
		t.Fatal(err)
	}
	want := []string{"pull 1/3 #1 one", "pull 2/3 #2 two", "pull 3/3 #3 three", "push 1/3 #1 one", "push 2/3 #2 two", "push 3/3 #3 three"}
	if !slices.Equal(got, want) {
		t.Fatalf("progress = %q\nwant %q", got, want)
	}
}

// Running says a run holds the lock: not a file left behind.
func TestRunningSaysAHeldLock(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	if bsync.Running(s.Store()) {
		t.Fatal("running before any run")
	}
	j, err := bsync.Lock(s.Store(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if !bsync.Running(s.Store()) {
		t.Fatal("not running while the lock is held")
	}
	j.Unlock()
	if bsync.Running(s.Store()) {
		t.Fatal("running after the run, its file left")
	}
}

// A lock file a killed run left, which no process holds, keeps no run out.
func TestALeftLockBlocksNothing(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	path := filepath.Join(s.Store().Folder(), "sync.lock")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("12345\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if r := syncAll(t, s, f); r.Created != 1 {
		t.Fatalf("report = %+v", r)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the lock file is gone: %v", err) // never removed: a second run can't lock another of that name
	}
}

// A run that wrote records its start; the next lists from there, so an
// issue not updated since isn't read again.
func TestRunRecordsItsStart(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	old := sample()
	old.Created, old.Updated = 1000, 1000
	f.Add(old)
	syncAll(t, s, f)
	st, _ := loadState(s, key)
	if st.lastRun == 0 {
		t.Fatal("no run recorded")
	}
	if r := syncAll(t, s, f); r != (Report{}) {
		t.Fatalf("report = %+v; want nothing listed", r)
	}
}

func TestTranslate(t *testing.T) {
	t.Parallel()
	ref := func(n string) string { return "<" + n + ">" }
	for _, c := range []struct {
		in, body, title string
	}{
		{"hi @ann.", "hi `@ann`.", "hi @ann."},
		{"see #1, #22", "see <1>, <22>", "see <1>, <22>"},
		{"`@ann #1` and ``x ` @bob``", "`@ann #1` and ``x ` @bob``", "`@ann #1` and ``x ` @bob``"},
		{"a lone ` then @ann", "a lone ` then `@ann`", "a lone ` then @ann"},
		{"`@ann", "` `@ann`", "`@ann"},
		{"mail a@b.c, &#12; x/#3 #4x a#5", "mail a@b.c, &#12; x/#3 #4x a#5", "mail a@b.c, &#12; x/#3 #4x a#5"},
	} {
		if got := bsync.Translate(c.in, false, ref); got != c.body {
			t.Errorf("body %q = %q, want %q", c.in, got, c.body)
		}
		if got := bsync.Translate(c.in, true, ref); got != c.title {
			t.Errorf("title %q = %q, want %q", c.in, got, c.title)
		}
	}
}

func TestWrap(t *testing.T) {
	t.Parallel()
	id := "3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5"
	h := bsync.HeaderLine("ann", 1792836800, "in git-board")
	for _, w := range []bsync.Wrapped{
		{Body: "plain"},
		{Body: "text", Marker: bsync.MarkerOf("issue", id, false, "")},
		{Header: h, Body: "text\n\nmore", Marker: bsync.MarkerOf("comment", id+":3", false, "")},
		{Header: h, Marker: bsync.MarkerOf("comment", id+":0", false, "")},
		{Body: "*From x, 2026-01-01, in git-board.*", Marker: bsync.MarkerOf("issue", id, false, "")}, // a token owner's text, read as a header
	} {
		got := bsync.Unwrap(w.String())
		if got.String() != w.String() {
			t.Errorf("%q: round trip gives %q", w.String(), got.String())
		}
		if w.Header != "" && (got.Header != w.Header || got.Body != w.Body) {
			t.Errorf("%q: parts = %+v", w.String(), got)
		}
	}
	if h != "*From ann, 2026-10-24, in git-board.*" {
		t.Fatalf("header = %q", h)
	}
	if w := bsync.Unwrap("x <!-- git-board issue " + id + " -->"); w.Marker != "" {
		t.Fatal("a marker not on its own last paragraph")
	}
}

// A comment Local keeps never reaches a forge; Did hears what the run
// wrote, issue by issue; Records holds what each forge last saw, texts
// unwrapped.
func TestLocalDidAndRecords(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	is, err := CreateIssue(t.Context(), s, NewCreate("a", "Mine", "body"))
	if err != nil {
		t.Fatal(err)
	}
	review := map[string]string{"review:path": "a.go"}
	if is, err = DoIssue(t.Context(), s, is.ID, NewComment("a", "posted"), NewCommentMeta("a", "on a line", review)); err != nil {
		t.Fatal(err)
	}
	var heard []string
	opt := Options{
		Local: func(m map[string]string) bool { return m["review:path"] != "" },
		Did:   func(forge, what, id string, ops int) { heard = append(heard, forge+" "+what) },
	}
	if _, err := Sync(t.Context(), s, opt, f); err != nil {
		t.Fatal(err)
	}
	for _, w := range f.Writes() {
		if w.Op == "create-comment" && strings.Contains(strings.Join(w.Args, " "), "on a line") {
			t.Errorf("the local comment was posted: %+v", w)
		}
	}
	if want := []string{key + " created", key + " posted", key + " pushed"}; !reflect.DeepEqual(heard, want) {
		t.Errorf("heard %v, want %v", heard, want)
	}
	recs, err := Records(s)
	if err != nil {
		t.Fatal(err)
	}
	r := recs[is.ID]
	if len(r) != 1 || r[0].Forge != bsync.StateID(key) || r[0].Seen["body"] != "body" || r[0].Posted[is.Comments[0].ID] == "" || r[0].Posted[is.Comments[1].ID] != "" {
		t.Fatalf("records = %+v", r)
	}
	if fc := r[0].Posted[is.Comments[0].ID]; r[0].Seen["comment:"+fc] != "posted" {
		t.Errorf("seen %q, want the posted text unwrapped", r[0].Seen["comment:"+fc])
	}
}

// A held label stays in git-board: never sent to its project, and the
// project's own copy, an older sync's, neither pulled nor removed.
func TestHeldLabels(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	one := sample()
	one.Labels = []string{"bug", "status:ready"}
	f.Add(one)
	opt := Options{Held: func(k, l string) bool { return k == key && strings.HasPrefix(l, "status:") }}
	if _, err := Sync(t.Context(), s, opt, f); err != nil {
		t.Fatal(err)
	}
	is := only(t, s)
	if !reflect.DeepEqual(is.Labels, []string{"bug"}) {
		t.Fatalf("pulled %v, want the held label left out", is.Labels)
	}
	if _, err := DoIssue(t.Context(), s, is.ID, ChangeLabels("a", []string{"status:in-review", "prio:p1"}, nil)); err != nil {
		t.Fatal(err)
	}
	for range 2 { // the second run finds nothing to do
		if _, err := Sync(t.Context(), s, opt, f); err != nil {
			t.Fatal(err)
		}
	}
	fi, _ := f.Get("1")
	if got := slices.Sorted(slices.Values(fi.Labels)); !reflect.DeepEqual(got, []string{"bug", "prio:p1", "status:ready"}) {
		t.Errorf("the forge has %v, want prio:p1 sent and status: left as it was", got)
	}
	if got := only(t, s).Labels; !slices.Contains(got, "status:in-review") || slices.Contains(got, "status:ready") {
		t.Errorf("git-board has %v, want its own status kept", got)
	}
}

// The assignees live where their field's mapping says: Options.Assignees
// false plans no native write or read — their home is elsewhere on that
// forge, and a field is read from its home alone. The assignee labels go
// through as ordinary labels meanwhile.
func TestAssigneesAnotherHome(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	one := sample()
	one.Labels = []string{"bug"}
	one.Assignees = []forge.Account{{ID: "8", Login: "bob"}}
	f.Add(one)
	opt := Options{Assignees: func(string) bool { return false }}
	if _, err := Sync(t.Context(), s, opt, f); err != nil {
		t.Fatal(err)
	}
	if got := only(t, s).Labels; slices.ContainsFunc(got, func(l string) bool { return strings.HasPrefix(l, "assignee:") }) {
		t.Errorf("git-board has %v, want the forge's assignee not pulled", got)
	}

	// and neither side moves: a change locally stays a label, the forge's
	// own assignees stay the forge's
	if _, err := Sync(t.Context(), s, opt, f); err != nil {
		t.Fatal(err)
	}
	is := only(t, s)
	if _, err := DoIssue(t.Context(), s, is.ID, ChangeLabels("a", []string{"assignee:bob"}, nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(t.Context(), s, opt, f); err != nil {
		t.Fatal(err)
	}
	if fi, _ := f.Get("1"); len(fi.Assignees) != 1 {
		t.Errorf("the forge has %v, want its own assignee kept", fi.Assignees)
	}

	// and the write: an assignee named in git-board goes nowhere native
	s2, f2 := newStore(t), fake.New(key)
	two := sample()
	two.Labels = []string{"bug"}
	f2.Add(two)
	if _, err := Sync(t.Context(), s2, opt, f2); err != nil {
		t.Fatal(err)
	}
	is2 := only(t, s2)
	if _, err := DoIssue(t.Context(), s2, is2.ID, ChangeLabels("a", []string{"assignee:bob"}, nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(t.Context(), s2, opt, f2); err != nil {
		t.Fatal(err)
	}
	if fi, _ := f2.Get("1"); len(fi.Assignees) != 0 {
		t.Errorf("the forge has %v, want no assignee written", fi.Assignees)
	}
}

// A label seen before it was held stays on both sides, and stays seen, the
// issue read again meanwhile: once the hold is lifted (an older binary, or
// the Project gone), git-board's change since wins, and no stale copy comes
// back.
func TestHeldAfterSeen(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	one := sample()
	one.Labels = []string{"bug", "status:ready"}
	f.Add(one)
	syncAll(t, s, f)
	opt := Options{Held: func(_, l string) bool { return strings.HasPrefix(l, "status:") }}
	if _, err := Sync(t.Context(), s, opt, f); err != nil {
		t.Fatal(err)
	}
	if got := only(t, s).Labels; !slices.Contains(got, "status:ready") {
		t.Errorf("git-board has %v, want status:ready kept", got)
	}
	if fi, _ := f.Get("1"); !slices.Contains(fi.Labels, "status:ready") {
		t.Errorf("the forge has %v, want status:ready kept", fi.Labels)
	}

	is := only(t, s)
	if _, err := DoIssue(t.Context(), s, is.ID, ChangeLabels("a", []string{"status:in-review"}, []string{"status:ready"})); err != nil {
		t.Fatal(err)
	}
	f.Edit("1", func(fi *forge.Issue) { fi.Title = "T2" }) // read again, held
	if _, err := Sync(t.Context(), s, opt, f); err != nil {
		t.Fatal(err)
	}
	syncAll(t, s, f) // the hold lifted
	want := []string{"bug", "status:in-review"}
	if got := slices.Sorted(slices.Values(only(t, s).Labels)); !reflect.DeepEqual(got, want) {
		t.Errorf("git-board has %v, want %v", got, want)
	}
	if fi, _ := f.Get("1"); !reflect.DeepEqual(slices.Sorted(slices.Values(fi.Labels)), want) {
		t.Errorf("the forge has %v, want %v", fi.Labels, want)
	}
}

// Two issues can hold the same comment id: the same text by the same
// person in the same second makes the same pack. Each is posted.
func TestSameCommentInTwoIssues(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	var ids []string
	for _, title := range []string{"two", "three"} {
		is, err := CreateIssue(t.Context(), s, NewCreate("a", title, ""))
		if err != nil {
			t.Fatal(err)
		}
		c := NewComment("a", "done")
		c.Time = 1
		if is, err = DoIssue(t.Context(), s, is.ID, c); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, is.Comments[0].ID)
	}
	if ids[0] != ids[1] {
		t.Skip("the two comments have their own ids: nothing to show")
	}
	if _, err := Push(t.Context(), s, Options{}, f); err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, w := range f.Writes() {
		if w.Op == "create-comment" {
			n++
		}
	}
	if n != 2 {
		t.Errorf("posted %d comments, want both", n)
	}
}

// A title or body last changed by a pull from a private project keeps the
// issue off a public one: creating it there would publish them.
func TestPrivateTextNotCreatedOnPublic(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	q, p := fake.New("gitlab.example/q"), fake.New("github.com/p")
	p.SetVisibility(forge.Public)
	if _, err := CreateIssue(t.Context(), s, NewCreate("a", "T", "public body")); err != nil {
		t.Fatal(err)
	}
	syncAll(t, s, q)
	q.Edit("1", func(i *forge.Issue) {
		i.Body = strings.Replace(i.Body, "public body", "SECRET", 1)
		i.Title = "SECRET TITLE"
	})
	syncAll(t, s, q)
	syncAll(t, s, q, p)
	for _, fi := range p.All() {
		if strings.Contains(fi.Body, "SECRET") || strings.Contains(fi.Title, "SECRET") {
			t.Errorf("private text published: %q / %q", fi.Title, fi.Body)
		}
	}
}

type unreadable struct{ *fake.Forge }

func (unreadable) Visibility(context.Context) (forge.Visibility, error) {
	return "", errors.New("503")
}

// A project whose visibility can't be read stops its own sync, as not
// public it would take what is private; the others sync, and the run
// returns its error. The lock goes with the run.
func TestUnreadableVisibilitySkipsTheProject(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	q, p := fake.New("gitlab.example/q"), fake.New("github.com/p")
	p.SetVisibility(forge.Public)
	q.Add(forge.Issue{Author: ann(), Title: "secret", Body: "private"})
	r, err := Sync(t.Context(), s, Options{}, q, unreadable{p})
	if err == nil || !strings.Contains(err.Error(), "github.com/p") {
		t.Fatalf("err = %v, want github.com/p's", err)
	}
	if r.Created != 1 {
		t.Errorf("q not synced: %+v", r)
	}
	if len(p.All()) != 0 {
		t.Error("private issue published")
	}
	syncAll(t, s, q)
}

// An internal project, read by every user of its forge, counts as public
// as a destination and as not public as a source: nothing private goes
// to it, and nothing of it goes to a public one.
func TestInternalFailsClosed(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	priv, in, pub := fake.New("gitlab.example/priv"), fake.New("gitlab.example/in"), fake.New("github.com/pub")
	in.SetVisibility(forge.Internal)
	pub.SetVisibility(forge.Public)
	priv.Add(forge.Issue{Author: ann(), Title: "private", Body: "p"})
	in.Add(forge.Issue{Author: ann(), Title: "inside", Body: "i"})
	syncAll(t, s, priv, in, pub)
	titles := func(f *fake.Forge) (out []string) {
		for _, fi := range f.All() {
			out = append(out, fi.Title)
		}
		return out
	}
	if got := titles(in); !reflect.DeepEqual(got, []string{"inside"}) {
		t.Errorf("internal has %q", got)
	}
	if got := titles(pub); len(got) != 0 {
		t.Errorf("public has %q", got)
	}
	if got := titles(priv); !reflect.DeepEqual(got, []string{"private", "inside"}) {
		t.Errorf("private has %q", got)
	}
}

type failingTokens struct{ *fake.Forge }

func (failingTokens) As(context.Context, forge.Account) (forge.Forge, error) {
	return nil, context.DeadlineExceeded // a credential helper that timed out
}

// A token lookup that fails counts as no token: the run goes on, and lets
// its lock go at the end.
func TestFailedLookupReleasesTheLock(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(forge.Issue{Author: forge.Account{ID: "8", Login: "bob"}, Title: "T", Body: bsync.MarkerOf("issue", "x", false, "") + "\nb"})
	if r := syncAll(t, s, failingTokens{f}); r.Created != 1 {
		t.Fatalf("report %+v", r)
	}
	j, err := bsync.Lock(s.Store(), 1)
	if err != nil {
		t.Fatal(err)
	}
	j.Unlock()
}

// A push alone after a crash between creating a forge issue and linking it
// links it by its marker, and doesn't create it again.
func TestPushAloneLinksByMarker(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New("github.com/p")
	if _, err := CreateIssue(t.Context(), s, NewCreate("a", "T", "B")); err != nil {
		t.Fatal(err)
	}
	f.After(func(w fake.Write) error {
		if w.Op == "create-issue" {
			f.After(nil)
			return fake.ErrCrash
		}
		return nil
	})
	if _, err := Push(t.Context(), s, Options{}, f); err == nil {
		t.Fatal("no crash")
	}
	if _, err := Push(t.Context(), s, Options{}, f); err != nil {
		t.Fatal(err)
	}
	if len(f.All()) != 1 {
		t.Errorf("made %d times", len(f.All()))
	}
}

// A forge issue linked by its marker is last seen as the push sent it: a
// forge edit made since is the forge's change, pulled, not overwritten.
func TestMarkerLinkSeesWhatWasSent(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New("github.com/p")
	is, err := CreateIssue(t.Context(), s, NewCreate("a", "T", "B"))
	if err != nil {
		t.Fatal(err)
	}
	f.After(func(w fake.Write) error {
		if w.Op == "create-issue" {
			f.After(nil)
			return fake.ErrCrash
		}
		return nil
	})
	if _, err := Push(t.Context(), s, Options{}, f); err == nil {
		t.Fatal("no crash")
	}
	f.Edit(f.All()[0].Number, func(i *forge.Issue) { i.Title = "Edited there" })
	syncAll(t, s, f)
	if got, _ := LoadIssue(s, is.ID); got.Title != "Edited there" {
		t.Errorf("title %q, want the forge's edit", got.Title)
	}
	if got := f.All()[0].Title; got != "Edited there" {
		t.Errorf("forge title %q", got)
	}
}

// A push that stopped before its link, then a title and a body edited
// here: the marker link's last seen is what the create sent, not the
// issue as it is now, so the edits are pushed instead of overwritten.
func TestMarkerLinkKeepsALocalEdit(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New("github.com/p")
	is, err := CreateIssue(t.Context(), s, NewCreate("a", "T", "B"))
	if err != nil {
		t.Fatal(err)
	}
	f.After(func(w fake.Write) error {
		if w.Op == "create-issue" {
			f.After(nil)
			return fake.ErrCrash
		}
		return nil
	})
	if _, err := Push(t.Context(), s, Options{}, f); err == nil {
		t.Fatal("no crash")
	}
	if _, err := DoIssue(t.Context(), s, is.ID, SetTitle("a", "Edited here"), EditText("a", is.BodyOp, "B2")); err != nil {
		t.Fatal(err)
	}
	syncAll(t, s, f)
	got, _ := LoadIssue(s, is.ID)
	if got.Title != "Edited here" || got.Body != "B2" {
		t.Errorf("git-board has %q, %q, want the local edits", got.Title, got.Body)
	}
	fi := f.All()[0]
	if fi.Title != "Edited here" || !strings.Contains(fi.Body, "B2") {
		t.Errorf("forge has %q, %q, want the local edits pushed", fi.Title, fi.Body)
	}
}

type rereadFails struct {
	*fake.Forge
	fail *bool
}

func (f rereadFails) Issue(ctx context.Context, id forge.Ref) (forge.Issue, error) {
	if *f.fail {
		return forge.Issue{}, errors.New("502")
	}
	return f.Forge.Issue(ctx, id)
}

// A write's "not found" is a refusal only once the posting account reads
// the issue: a re-read that fails otherwise stops the run, and records
// nothing.
func TestNotFoundThenFailedReread(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	is, err := CreateIssue(t.Context(), s, NewCreate("a", "T", "B"))
	if err != nil {
		t.Fatal(err)
	}
	fail := false
	ff := rereadFails{f, &fail}
	syncAll(t, s, ff)
	if _, err := DoIssue(t.Context(), s, is.ID, SetTitle("a", "T2")); err != nil {
		t.Fatal(err)
	}
	f.Before(func(w fake.Write) error {
		if w.Op == "set-title" {
			fail = true
			return forge.ErrNotFound
		}
		return nil
	})
	if _, err := Sync(t.Context(), s, Options{}, ff); err == nil {
		t.Fatal("no error")
	}
	fail = false
	f.Before(nil)
	syncAll(t, s, ff)
	if got := f.All()[0].Title; got != "T2" {
		t.Errorf("title %q: the failure was recorded as a refusal", got)
	}
}

// Create is asked once per project before its creations, with their count
// and whether the run started with no sync entity there: false creates
// none, and the rest of the push goes on; the next run asks again.
func TestCreateAsksFirst(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	for _, title := range []string{"one", "two"} {
		if _, err := CreateIssue(t.Context(), s, NewCreate("a", title, "")); err != nil {
			t.Fatal(err)
		}
	}
	var asked []string
	ask := func(allow bool) Options {
		return Options{Create: func(k string, n int, first bool) bool {
			asked = append(asked, fmt.Sprintf("%s %d %t", k, n, first))
			return allow
		}}
	}
	if _, err := Sync(t.Context(), s, ask(false), f); err != nil {
		t.Fatal(err)
	}
	if len(f.All()) != 0 || len(refs(t, s)) != 2 {
		t.Fatalf("held: the forge has %v, refs %v", f.All(), refs(t, s))
	}
	if _, err := Sync(t.Context(), s, ask(true), f); err != nil {
		t.Fatal(err)
	}
	if want := []string{key + " 2 true", key + " 2 true"}; !reflect.DeepEqual(asked, want) || len(f.All()) != 2 {
		t.Fatalf("asked %v, want %v; the forge has %d", asked, want, len(f.All()))
	}
}
