package syncer

import (
	"maps"
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
)

func forkMarker(c byte) string { return "<!-- git-bug:" + strings.Repeat(string(c), 64) + " -->" }

func TestForkStart(t *testing.T) {
	t.Parallel()
	head := "*From ann, 2026-01-01, on [example.com/a#1](https://example.com/a/issues/1).*"
	for _, c := range []struct{ text, body, id, place string }{
		{forkMarker('a') + "\nhi", "hi", strings.Repeat("a", 64), ""},
		{forkMarker('a'), "", strings.Repeat("a", 64), ""},
		{forkMarker('a') + "\n" + head + "\n\nhi", "hi", strings.Repeat("a", 64), "example.com/a#1"},
		{head + "\n\nhi", "hi", "", "example.com/a#1"},
		{head + "\n\n" + forkMarker('b') + "\nhi", "hi", strings.Repeat("b", 64), "example.com/a#1"},
		{"*From the team*\n\nhi", "*From the team*\n\nhi", "", ""}, // no link, no marker: text
		{"hi\n" + forkMarker('a'), "hi\n" + forkMarker('a'), "", ""},
	} {
		w := unwrap(c.text)
		if w.body != c.body || w.forkID != c.id || w.place != c.place || w.String() != c.text {
			t.Errorf("unwrap(%q) = %+v", c.text, w)
		}
	}
	// the core's own form is read as the core's, its header included
	core := head + "\n\nhi\n\n" + marker("issue", strings.Repeat("c", 40))
	if w := unwrap(core); w.forked() || w.body != "hi" || w.marker == "" {
		t.Errorf("unwrap(core) = %+v", w)
	}
	if !same("`@bob` see [example.com/a#3](https://x/3)", "@bob see #5") || same("hi", "ho") {
		t.Error("same")
	}
}

// forkPair is an issue the fork mirrored: an original on a, ann's, with a
// comment of bob's, and its copy on b, posted by the posting account with
// the fork's marker and header. Each carries the other's comment as a copy;
// carl's comment on b is an original.
func forkPair() (a, b *fake.Forge) {
	a, b = fake.New("example.com/a"), fake.New("example.com/b")
	board, carl := forge.Account{ID: "1", Login: "board"}, forge.Account{ID: "9", Login: "carl"}
	bob := forge.Account{ID: "8", Login: "bob"}
	a.Add(forge.Issue{ID: "100", Author: ann(), Created: 1000, Title: "T", Body: "B @bob", Labels: []string{"bug"},
		Comments: []forge.Comment{
			{ID: "a1", Body: "hi", Author: bob, Time: 1100},
			{ID: "a2", Body: forkMarker('2') + "\nfrom b, see #1", Author: board, Time: 1300},
		}})
	b.Add(forge.Issue{ID: "200", Author: board, Created: 1001, Title: "T",
		Body:   forkMarker('1') + "\n*From ann, 1970-01-01, on [example.com/a#1](https://example.com/a/issues/1).*\n\nB `@bob`",
		Labels: []string{"ui"},
		Comments: []forge.Comment{
			{ID: "b1", Body: forkMarker('3') + "\nhi", Author: board, Time: 1200},
			{ID: "b2", Body: "from b, see #1", Author: carl, Time: 1250},
		}})
	return a, b
}

func issues(t *testing.T, s *gitstore.Store) []*issue.Issue {
	t.Helper()
	ids, err := issue.List(s)
	if err != nil {
		t.Fatal(err)
	}
	var out []*issue.Issue
	for _, id := range ids {
		out = append(out, must(issue.Load(s, id)))
	}
	return out
}

// A mirror the fork posted joins the issue pulled from its original,
// whichever forge is pulled first, its comments included; then sync
// creates nothing on either forge, and a second sync writes nothing.
func TestForkMirrorJoins(t *testing.T) {
	t.Parallel()
	for _, bFirst := range []bool{false, true} {
		s := newStore(t)
		a, b := forkPair()
		fs := []forge.Forge{a, b}
		if bFirst {
			fs = []forge.Forge{b, a}
		}
		var joined int
		r, err := Pull(t.Context(), s, Options{Did: func(_, what, _ string, _ int) {
			if what == "joined" {
				joined++
			}
		}}, fs...)
		if err != nil {
			t.Fatal(err)
		}
		all := issues(t, s)
		if len(all) != 1 || r.Created != 1 || joined != 1 {
			t.Fatalf("b first %v: %d issues, %+v, %d joined", bFirst, len(all), r, joined)
		}
		is := all[0]
		if is.Body != "B @bob" || len(is.Comments) != 2 || is.Comments[0].Body != "hi" || is.Comments[1].Body != "from b, see #1" {
			t.Fatalf("b first %v: issue %q, comments %+v", bFirst, is.Body, is.Comments)
		}
		if strings.Join(is.Labels, ",") != "bug,ui" {
			t.Fatalf("b first %v: labels %v, want both sides'", bFirst, is.Labels)
		}
		syncAll(t, s, a, b)
		for _, f := range []*fake.Forge{a, b} {
			for _, w := range f.Writes() {
				if w.Op == "create-issue" || w.Op == "create-comment" {
					t.Fatalf("b first %v: %s: %+v", bFirst, f.Key(), w)
				}
			}
		}
		n := len(a.Writes()) + len(b.Writes())
		syncAll(t, s, a, b)
		if more := len(a.Writes()) + len(b.Writes()) - n; more != 0 {
			t.Fatalf("b first %v: the second sync wrote %d times: %+v %+v", bFirst, more, a.Writes(), b.Writes())
		}
	}
}

// A marker-only copy is joined by the fork's data, and so are two
// originals it names as one; without it, the copy is an issue of its own.
// A stranger's text in the fork's form joins nothing.
func TestForkMirrorsOption(t *testing.T) {
	t.Parallel()
	setup := func() (*fake.Forge, *fake.Forge) {
		a, b := fake.New("example.com/a"), fake.New("example.com/b")
		board := forge.Account{ID: "1", Login: "board"}
		a.Add(forge.Issue{Author: ann(), Title: "One", Body: "one"})
		a.Add(forge.Issue{Author: ann(), Title: "Two", Body: "two"})
		b.Add(forge.Issue{Author: board, Title: "One", Body: forkMarker('1') + "\none"})
		b.Add(forge.Issue{Author: ann(), Title: "Two", Body: "two"})
		b.Add(forge.Issue{Author: forge.Account{ID: "9", Login: "carl"}, Title: "Three",
			Body: "*From x, on [example.com/a#1](u).*\n\nthree"})
		return a, b
	}
	s := newStore(t)
	a, b := setup()
	if _, err := Pull(t.Context(), s, Options{}, b, a); err != nil {
		t.Fatal(err)
	}
	if n := len(issues(t, s)); n != 5 {
		t.Fatalf("without the fork's data: %d issues, want 5", n)
	}
	s = newStore(t)
	a, b = setup()
	opt := Options{Mirrors: [][]string{{"example.com/a#1", "EXAMPLE.com/b#1"}, {"example.com/a#2", "example.com/b#2"}}}
	if _, err := Pull(t.Context(), s, opt, b, a); err != nil {
		t.Fatal(err)
	}
	all := issues(t, s)
	if len(all) != 3 {
		t.Fatalf("with the fork's data: %d issues, want 3", len(all))
	}
	for _, is := range all {
		if strings.Contains(is.Body, "git-bug") {
			t.Fatalf("body %q keeps the fork's marker", is.Body)
		}
	}
	links, err := Links(s)
	if err != nil {
		t.Fatal(err)
	}
	two := 0
	for _, ls := range links {
		if len(ls) == 2 {
			two++
		}
	}
	if two != 2 {
		t.Fatalf("links = %v, want two issues on both forges", links)
	}
}

// A text in the fork's form is kept without its marker and header; its
// last seen value is the forge's, and a second pull writes nothing.
func TestForkTextsLoseTheirStart(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(forge.Issue{ID: "100", Author: ann(), Title: "T", Body: forkMarker('1') + "\nbody",
		Comments: []forge.Comment{{ID: "c1", Author: ann(), Body: forkMarker('2') + "\nnote", Time: 1100}}})
	pull(t, s, f)
	is := only(t, s)
	if is.Body != "body" || is.Comments[0].Body != "note" {
		t.Fatalf("issue %q, comment %q", is.Body, is.Comments[0].Body)
	}
	before := refs(t, s)
	pull(t, s, f)
	if !maps.Equal(before, refs(t, s)) {
		t.Fatal("a second pull wrote")
	}
	f.Edit("1", func(fi *forge.Issue) { fi.Body = forkMarker('1') + "\nbody, edited" })
	pull(t, s, f)
	if got := only(t, s).Body; got != "body, edited" {
		t.Fatalf("edited body %q", got)
	}
}

// A copy whose poster isn't one of sync's accounts still waits for its
// original, and joins it by the fork's data: the original stays as its
// author wrote it, and sync edits neither.
func TestForkCopyOfAStrangerWaits(t *testing.T) {
	t.Parallel()
	s, a, b := newStore(t), fake.New("example.com/a"), fake.New("example.com/b")
	b.Add(forge.Issue{Author: forge.Account{ID: "5", Login: "fork-bot"}, Title: "T", Body: forkMarker('1') + "\nB `@bob`"})
	a.Add(forge.Issue{Author: ann(), Title: "T", Body: "B @bob"})
	opt := Options{Mirrors: [][]string{{"example.com/a#1", "example.com/b#1"}}}
	if _, err := Pull(t.Context(), s, opt, b, a); err != nil {
		t.Fatal(err)
	}
	if is := only(t, s); is.Body != "B @bob" {
		t.Fatalf("body %q, want the original's", is.Body)
	}
	syncAll(t, s, a, b)
	if w := append(a.Writes(), b.Writes()...); len(w) != 0 {
		t.Fatalf("writes %+v", w)
	}
}
