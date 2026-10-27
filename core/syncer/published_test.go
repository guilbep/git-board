package syncer

import (
	"strings"
	"testing"

	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
)

const noted = "**Published v2** · § Expected changed"

// published notes a body that changed, under the issue's id and ":9".
func published() Options {
	return Options{Published: func(id, before, after string) (string, string) {
		if before == after {
			return "", ""
		}
		return id + ":9", noted
	}}
}

// edited makes an issue synced once, then edits its body.
func edited(t *testing.T, opt Options) (*gitstore.Store, *fake.Forge, *issue.Issue) {
	t.Helper()
	s, f := newStore(t), newFake()
	is, err := issue.Create(t.Context(), s, issue.NewCreate("a", "T", "B"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(t.Context(), s, opt, f); err != nil {
		t.Fatal(err)
	}
	if is, err = issue.Do(t.Context(), s, is.ID, issue.EditText("a", is.BodyOp, "B, v2")); err != nil {
		t.Fatal(err)
	}
	return s, f, is
}

// A body pushed gets its note, after the edit.
func TestPublishedNote(t *testing.T) {
	t.Parallel()
	s, f, is := edited(t, published())
	if _, err := Sync(t.Context(), s, published(), f); err != nil {
		t.Fatal(err)
	}
	cs := f.All()[0].Comments
	want := noted + "\n\n<!-- git-board version " + is.ID + ":9 -->"
	if len(cs) != 1 || !strings.HasSuffix(cs[0].Body, want) {
		t.Fatalf("comments = %+v, want one ending %q", cs, want)
	}
	var ops []string
	for _, w := range f.Writes() {
		ops = append(ops, w.Op)
	}
	if n := len(ops); n < 2 || ops[n-2] != "set-body" || ops[n-1] != "create-comment" {
		t.Errorf("writes %v, want the body then the note", ops)
	}
}

// The note is posted once: on the next run, and after a crash between
// posting it and recording it. It never becomes a comment.
func TestPublishedOnce(t *testing.T) {
	t.Parallel()
	s, f, _ := edited(t, published())
	f.After(func(w fake.Write) error {
		if w.Op == "create-comment" {
			f.After(nil)
			return fake.ErrCrash
		}
		return nil
	})
	if _, err := Sync(t.Context(), s, published(), f); err == nil {
		t.Fatal("no crash")
	}
	for range 2 {
		if _, err := Sync(t.Context(), s, published(), f); err != nil {
			t.Fatal(err)
		}
	}
	if n := len(f.All()[0].Comments); n != 1 {
		t.Errorf("noted %d times", n)
	}
	if got := only(t, s); len(got.Comments) != 0 {
		t.Errorf("pulled as comments: %+v", got.Comments)
	}
}

// No note for a body the push doesn't change, nor without Published.
func TestPublishedNotWithoutAChange(t *testing.T) {
	t.Parallel()
	s, f, _ := edited(t, Options{})
	if _, err := Sync(t.Context(), s, Options{}, f); err != nil {
		t.Fatal(err)
	}
	if _, err := Sync(t.Context(), s, published(), f); err != nil {
		t.Fatal(err)
	}
	if n := len(f.All()[0].Comments); n != 0 {
		t.Errorf("noted %d times", n)
	}
}
