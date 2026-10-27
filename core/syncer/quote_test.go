package syncer

import (
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/issue"
)

const (
	sixFQuote = "> **On v1 · § Expected · 3** · _A missing identity sends the user to Settings › identity, not to a CLI command._"
	sixFBody  = "Then say what replaces the CLI text.\n\n```suggestion\nA missing identity sends the user to Settings › identity; the CLI text is never shown.\n```"
)

func quoted() Options {
	opt := keyed()
	opt.Quote = func(_ string, c issue.Comment) string {
		if c.Meta["anchor"] == "" {
			return ""
		}
		return sixFQuote
	}
	return opt
}

// 6f's comment goes to the forge under its quote, its keys in the marker.
func TestQuotedComment(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	fr := &fakeRun{t: t, opt: quoted(), s: s}
	fr.f = newFake()
	is, err := issue.Create(t.Context(), s, issue.NewCreate("a", "T", "B"))
	if err != nil {
		t.Fatal(err)
	}
	if is, err = issue.Do(t.Context(), s, is.ID, issue.NewCommentMeta("a", sixFBody, map[string]string{"step": "1", "anchor": "body@v1/expected#3"})); err != nil {
		t.Fatal(err)
	}
	fr.sync()
	want := sixFQuote + "\n\n" + sixFBody + "\n\n<!-- git-board comment " + is.Comments[0].ID + " quoted step=1 anchor=body@v1/expected#3 -->"
	if got := fr.forgeComment().Body; !strings.HasSuffix(got, "\n\n"+want) && got != want {
		t.Fatalf("posted\n%s\nwant it ending\n%s", got, want)
	}
}

// A quoted comment settles: the next run pushes nothing, and its record's
// seen text is git-board's body.
func TestQuotedCommentSettles(t *testing.T) {
	t.Parallel()
	fr, is := commented(t, quoted(), map[string]string{"step": "1", "anchor": "body@v1/expected#3"})
	if r := fr.sync(); r.Pushed != 0 || r.Posted != 0 {
		t.Errorf("second run = %+v", r)
	}
	recs, err := Records(fr.s)
	if err != nil {
		t.Fatal(err)
	}
	r := recs[is.ID][0]
	if fc := r.Posted[is.Comments[0].ID]; r.Seen["comment:"+fc] != "the question" {
		t.Errorf("seen %q, want the body alone", r.Seen["comment:"+fc])
	}
}

// The forge edits the text under the quote: the pull takes the text alone.
func TestQuoteStrippedOnPull(t *testing.T) {
	t.Parallel()
	fr, _ := commented(t, quoted(), map[string]string{"anchor": "body@v1/expected"})
	fr.editComment(func(b string) string { return strings.Replace(b, "the question", "the question, edited there", 1) })
	fr.sync()
	if got := only(t, fr.s).Comments[0].Body; got != "the question, edited there" {
		t.Errorf("body %q", got)
	}
	if !strings.Contains(fr.forgeComment().Body, sixFQuote+"\n\nthe question, edited there") {
		t.Errorf("forge %q", fr.forgeComment().Body)
	}
}

// A quote counts only where sync's marker says it wrote one: a body of its
// own starting with a quote keeps it, posted and pulled.
func TestQuoteOnlyInAMarkedText(t *testing.T) {
	t.Parallel()
	own := "> **On v1** we said so\n\nand still do"
	s := newStore(t)
	fr := &fakeRun{t: t, opt: quoted(), s: s, f: newFake()}
	is, err := issue.Create(t.Context(), s, issue.NewCreate("a", "T", "B"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = issue.Do(t.Context(), s, is.ID, issue.NewCommentMeta("a", own, map[string]string{"step": "1"})); err != nil {
		t.Fatal(err)
	}
	fr.sync()
	if r := fr.sync(); r.Pushed != 0 {
		t.Errorf("second run = %+v", r)
	}
	if got := fr.forgeComment().Body; strings.Count(got, "> **On v1**") != 1 || strings.Contains(got, " quoted") {
		t.Errorf("forge %q", got)
	}
	if w := unwrap(fr.forgeComment().Body); w.body != own || w.quote != "" {
		t.Errorf("unwrapped %+v", w)
	}
}

// On a forge with threads, a reply goes to its root's forge comment, posted
// in the same run; with its root not posted, it is a comment.
func TestReplyJoinsItsRoot(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), newFake()
	is, err := issue.Create(t.Context(), s, issue.NewCreate("a", "T", "B"))
	if err != nil {
		t.Fatal(err)
	}
	if is, err = issue.Do(t.Context(), s, is.ID, issue.NewComment("a", "root")); err != nil {
		t.Fatal(err)
	}
	root := is.Comments[0].ID
	if _, err = issue.Do(t.Context(), s, is.ID,
		issue.NewCommentMeta("a", "answer", map[string]string{"reply-to": root}),
		issue.NewCommentMeta("a", "orphan", map[string]string{"reply-to": "0000000000000000000000000000000000000000:1"})); err != nil {
		t.Fatal(err)
	}
	opt := Options{ReplyTo: func(m map[string]string) string { return m["reply-to"] }}
	if _, err := Sync(t.Context(), s, opt, f); err != nil {
		t.Fatal(err)
	}
	cs := f.All()[0].Comments
	var ops []string
	for _, w := range f.Writes() {
		if w.Op == "reply" {
			ops = append(ops, "reply to "+w.Args[0])
		} else if w.Op == "create-comment" {
			ops = append(ops, "comment")
		}
	}
	if want := []string{"comment", "reply to " + cs[0].ID, "comment"}; strings.Join(ops, ", ") != strings.Join(want, ", ") {
		t.Errorf("writes %v, want %v", ops, want)
	}
}
