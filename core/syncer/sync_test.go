package syncer

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
)

func syncAll(t *testing.T, s *gitstore.Store, fs ...forge.Forge) Report {
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
	ops := []oplog.Op{
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
	lock := filepath.Join(dir, ".git", "refs", "board", "sync", StateID(key)+".lock")
	if err := os.MkdirAll(filepath.Dir(lock), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lock, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	st, _ := loadState(s, key)
	if err := st.write(t.Context(), s, key, []oplog.Op{linkOp("a", 1, "i", "100", "1", "")}); !errors.Is(err, gitstore.ErrRefMoved) {
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
	ev := &oplog.Origin{Forge: key, Event: "issue:100"}
	c := issue.NewCreate("a", "T", "B")
	c.Origin = ev
	is, err := issue.Create(t.Context(), s, c)
	if err != nil {
		t.Fatal(err)
	}
	pull(t, s, f)
	if ids, _ := issue.List(s); len(ids) != 1 {
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
	dup := issue.NewComment(is.Comments[0].Author, "hi!")
	dup.Origin = is.Comments[0].Origin
	if _, err := issue.Do(t.Context(), s, is.ID, dup); err != nil {
		t.Fatal(err)
	}
	st, _ := loadState(s, key)
	if err := st.write(t.Context(), s, key, []oplog.Op{seenOp(is.Author, 1, is.ID, "comment:c1", "hi!")}); err != nil {
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
	c := issue.NewCreate("a", "T", "B")
	c.Origin = first.Origin
	dup, err := issue.Create(t.Context(), s, c)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := loadState(s, key)
	if err := st.write(t.Context(), s, key, []oplog.Op{linkOp("a", 1, dup.ID, "100", "1", "")}); err != nil {
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
	unlock, err := lock(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(t.Context(), s, Options{}, f); !errors.Is(err, ErrLocked) {
		t.Fatalf("err = %v, want ErrLocked", err)
	}
	unlock()
	syncAll(t, s, f)
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
		if got := translate(c.in, false, ref); got != c.body {
			t.Errorf("body %q = %q, want %q", c.in, got, c.body)
		}
		if got := translate(c.in, true, ref); got != c.title {
			t.Errorf("title %q = %q, want %q", c.in, got, c.title)
		}
	}
}

func TestWrap(t *testing.T) {
	t.Parallel()
	id := "3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5"
	h := header("ann", 1792836800, "in git-board")
	for _, w := range []wrapped{
		{body: "plain"},
		{body: "text", marker: marker("issue", id)},
		{header: h, body: "text\n\nmore", marker: marker("comment", id+":3")},
		{header: h, marker: marker("comment", id+":0")},
		{body: "*From x, 2026-01-01, in git-board.*", marker: marker("issue", id)}, // a token owner's text, read as a header
	} {
		got := unwrap(w.String())
		if got.String() != w.String() {
			t.Errorf("%q: round trip gives %q", w.String(), got.String())
		}
		if w.header != "" && (got.header != w.header || got.body != w.body) {
			t.Errorf("%q: parts = %+v", w.String(), got)
		}
	}
	if h != "*From ann, 2026-10-24, in git-board.*" {
		t.Fatalf("header = %q", h)
	}
	if w := unwrap("x <!-- git-board issue " + id + " -->"); w.marker != "" {
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
	is, err := issue.Create(t.Context(), s, issue.NewCreate("a", "Mine", "body"))
	if err != nil {
		t.Fatal(err)
	}
	review := map[string]string{"review:path": "a.go"}
	if is, err = issue.Do(t.Context(), s, is.ID, issue.NewComment("a", "posted"), issue.NewCommentMeta("a", "on a line", review)); err != nil {
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
	if len(r) != 1 || r[0].Forge != StateID(key) || r[0].Seen["body"] != "body" || r[0].Posted[is.Comments[0].ID] == "" || r[0].Posted[is.Comments[1].ID] != "" {
		t.Fatalf("records = %+v", r)
	}
	if fc := r[0].Posted[is.Comments[0].ID]; r[0].Seen["comment:"+fc] != "posted" {
		t.Errorf("seen %q, want the posted text unwrapped", r[0].Seen["comment:"+fc])
	}
}

// Two issues can hold the same comment id: the same text by the same
// person in the same second makes the same pack. Each is posted.
func TestSameCommentInTwoIssues(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	var ids []string
	for _, title := range []string{"two", "three"} {
		is, err := issue.Create(t.Context(), s, issue.NewCreate("a", title, ""))
		if err != nil {
			t.Fatal(err)
		}
		c := issue.NewComment("a", "done")
		c.Time = 1
		if is, err = issue.Do(t.Context(), s, is.ID, c); err != nil {
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
