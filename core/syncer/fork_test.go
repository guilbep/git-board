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

// attributed is a text the fork posted for someone else, as on the wiki's
// GitLab: its marker with a word, and a header linking the original.
func attributed(c byte, who, link, text string) string {
	return "<!-- git-bug:" + strings.Repeat(string(c), 64) + " attributed -->\n*Originally posted by " + who +
		" on 2026-09-30, [view on GitHub](" + link + ")*\n\n" + text
}

func TestForkStart(t *testing.T) {
	t.Parallel()
	gh := "https://github.com/EPFL-ENAC/wiki/issues/6#issuecomment-5907585575"
	head := "*Originally posted by Nicolas Dubois on 2026-09-30, [view on GitHub](" + gh + ")*"
	for _, c := range []struct{ text, body, id, link string }{
		{forkMarker('a') + "\nhi", "hi", strings.Repeat("a", 64), ""},
		{forkMarker('a'), "", strings.Repeat("a", 64), ""},
		{forkMarker('a') + "\n", "", strings.Repeat("a", 64), ""},
		{attributed('a', "Nicolas Dubois", gh, "doc updated"), "doc updated", strings.Repeat("a", 64), gh},
		{head + "\n\nhi", head + "\n\nhi", "", ""}, // a header without the marker: text
		{"hi\n" + forkMarker('a'), "hi\n" + forkMarker('a'), "", ""},
	} {
		w := unwrap(c.text)
		if w.body != c.body || w.forkID != c.id || w.link != c.link || w.String() != c.text {
			t.Errorf("unwrap(%q) = %+v", c.text, w)
		}
	}
	for link, want := range map[string][2]string{
		gh: {"github.com", "5907585575"},
		"https://gitlab.epfl.ch/EPFL-ENAC/wiki/-/work_items/7#note_641057": {"gitlab.epfl.ch", "641057"},
		"https://github.com/EPFL-ENAC/wiki/issues/22":                      {"", ""},
	} {
		if host, id := commentLinked(link); [2]string{host, id} != want {
			t.Errorf("commentLinked(%s) = %s, %s", link, host, id)
		}
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
		Body:   attributed('1', "ann", "https://example.com/a/issues/1", "B `@bob`"),
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
			Body: attributed('3', "x", "https://example.com/a/issues/1", "three")})
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

// A text in the fork's form that the forge edited after creation, as a
// browser leaves it with a line end after its text, is pulled once: the
// next pulls write nothing, to the issue or to the sync entity.
func TestForkEditedTextPullsOnce(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New("github.com/o/r")
	board := forge.Account{ID: "1", Login: "board"}
	f.Add(forge.Issue{ID: "100", Author: board, Created: 1000, Updated: 3000, Title: "T",
		Body: forkMarker('a') + "\nedited on GitHub\n", Changed: map[string]int64{"body": 3000},
		Comments: []forge.Comment{{ID: "c1", Author: board, Time: 1100, Edited: 3000, Body: forkMarker('b') + "\na comment, edited\n"}}})
	pull(t, s, f)
	is := only(t, s)
	if is.Body != "edited on GitHub\n" || len(is.Comments) != 1 || is.Comments[0].Body != "a comment, edited\n" {
		t.Fatalf("pulled %q, %+v", is.Body, is.Comments)
	}
	for range 2 {
		before := refs(t, s)
		pull(t, s, f)
		if !maps.Equal(before, refs(t, s)) {
			t.Fatal("a pull with nothing new wrote")
		}
	}
}

// The wiki's texts, as the fork left them on its GitLab: an attributed
// copy joins its original by its header's link, an issue and a comment,
// even where the text differs. The original's text wins: sync's one write
// edits the comment's copy to it, keeping the fork's marker and header,
// and a second sync writes nothing.
func TestForkAttributedJoins(t *testing.T) {
	t.Parallel()
	for _, glFirst := range []bool{false, true} {
		s := newStore(t)
		gh, gl := fake.New("github.com/EPFL-ENAC/wiki"), fake.New("gitlab.epfl.ch/41960")
		board, nicdub := forge.Account{ID: "1", Login: "board"}, forge.Account{ID: "12", Login: "nicdub"}
		pierre := forge.Account{ID: "13", Login: "pripoll"}
		gh.Add(forge.Issue{Number: "6", Author: ann(), Title: "Shared web hosting", Body: "@nicdub, dans le wiki\n\n",
			Comments: []forge.Comment{{ID: "5907585575", Author: nicdub, Body: "doc updated, le shared hosting A10"}}})
		gh.Add(forge.Issue{Number: "7", Author: pierre, Title: "[SUGGESTION] Use vector embedding",
			Body: "Use RCP to do semantic search"})
		gl.Add(forge.Issue{Number: "7", Author: board, Title: "Shared web hosting", Body: forkMarker('d') + "\n`@nicdub`, dans le wiki",
			Comments: []forge.Comment{{ID: "641057", Author: board,
				Body: attributed('4', "Nicolas Dubois", "https://github.com/EPFL-ENAC/wiki/issues/6#issuecomment-5907585575",
					"doc updated, le shared hosting A10, edited on GitLab")}}})
		gl.Add(forge.Issue{Number: "8", Author: board, Title: "[SUGGESTION] Use vector embedding",
			Body: attributed('c', "Pierre Ripoll", "https://github.com/EPFL-ENAC/wiki/issues/7", "Use RCP to do semantic search")})
		fs := []forge.Forge{gh, gl}
		if glFirst {
			fs = []forge.Forge{gl, gh}
		}
		opt := Options{Mirrors: [][]string{{"github.com/epfl-enac/wiki#6", "gitlab.epfl.ch/41960#7"}}}
		if _, err := Pull(t.Context(), s, opt, fs...); err != nil {
			t.Fatal(err)
		}
		all := issues(t, s)
		if len(all) != 2 {
			t.Fatalf("gl first %v: %d issues", glFirst, len(all))
		}
		for _, is := range all {
			if strings.Contains(is.Body, "git-bug") || strings.Contains(is.Body, "Originally") || len(is.Comments) > 1 {
				t.Fatalf("gl first %v: %q, %+v", glFirst, is.Body, is.Comments)
			}
		}
		for range 2 {
			opt.Mirrors = nil
			if _, err := Sync(t.Context(), s, opt, gh, gl); err != nil {
				t.Fatal(err)
			}
		}
		w := append(gh.Writes(), gl.Writes()...)
		head := strings.SplitAfter(gl.All()[0].Comments[0].Body, "\n\n")[0]
		if len(w) != 1 || w[0].Op != "edit-comment" || w[0].Args[1] != head+"doc updated, le shared hosting A10" ||
			!strings.Contains(head, " attributed -->\n*Originally posted by Nicolas Dubois") {
			t.Fatalf("gl first %v: writes %+v", glFirst, w)
		}
	}
}
