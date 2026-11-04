package issue

import (
	"errors"
	"reflect"
	"testing"

	"github.com/guilbep/git-board/core/oplog"
)

// A deleted comment leaves the comments, its text with it; the others
// stay. An edit or a set-meta of it, from a clone that hadn't the delete,
// changes nothing, and a second delete neither.
func TestDeleteComment(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c := mk(t)(Create(t.Context(), s, NewCreate("a", "T", ""), NewComment("a", "one"), NewComment("b", "secret")))
	gone := c.Comments[1].ID
	c = mk(t)(Do(t.Context(), s, c.ID, Delete("b", gone)))
	if got := bodies(c); !reflect.DeepEqual(got, []string{"one"}) {
		t.Fatalf("comments = %q", got)
	}
	if len(c.Removed) != 1 || c.Removed[0].ID != gone || c.Removed[0].Body != "" || !c.IsRemoved(gone) {
		t.Fatalf("removed = %+v", c.Removed)
	}
	if _, ok := c.Changes["comment:"+gone]; ok {
		t.Errorf("the deleted comment is still in changes")
	}
	c = mk(t)(Do(t.Context(), s, c.ID, EditText("b", gone, "back"), SetMeta("b", gone, map[string]string{"k": "v"}), Delete("a", gone)))
	if got := bodies(c); !reflect.DeepEqual(got, []string{"one"}) || len(c.Removed) != 1 || c.Removed[0].Body != "" || c.Removed[0].Meta != nil {
		t.Fatalf("after an edit of the deleted comment: comments %q, removed %+v", got, c.Removed)
	}
	if c.Deleted {
		t.Errorf("deleting a comment deleted the issue")
	}
}

// A forge comment pulled again after its delete is the same comment: it
// stays deleted.
func TestDeletedCommentPulledAgainStaysDeleted(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c := mk(t)(Create(t.Context(), s, NewCreate("a", "T", ""), withOrigin(NewComment("bob", "one"), "f", "comment:1")))
	c = mk(t)(Do(t.Context(), s, c.ID, Delete("a", c.Comments[0].ID)))
	c = mk(t)(Do(t.Context(), s, c.ID, withOrigin(NewComment("bob", "one"), "f", "comment:1")))
	if len(c.Comments) != 0 || len(c.Removed) != 1 {
		t.Fatalf("comments %+v, removed %+v", c.Comments, c.Removed)
	}
}

// Deleting the issue targets its create operation: the state is still
// read, marked deleted.
func TestDeleteIssue(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c := mk(t)(Create(t.Context(), s, NewCreate("a", "T", "B")))
	c = mk(t)(Do(t.Context(), s, c.ID, Delete("b", c.BodyOp)))
	if !c.Deleted || c.Changes["deleted"].Author != "b" || c.Title != "T" {
		t.Fatalf("issue = %+v", c)
	}
}

// A delete of nothing the issue has, or of an operation not written yet,
// is refused before anything is written.
func TestDeleteUnknownTarget(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	c := mk(t)(Create(t.Context(), s, NewCreate("a", "T", "")))
	for _, target := range []string{"0000000000000000000000000000000000000000:0", "pending:0", ""} {
		if _, err := Do(t.Context(), s, c.ID, Delete("a", target)); !errors.Is(err, ErrBadTarget) {
			t.Errorf("delete %q: err = %v, want ErrBadTarget", target, err)
		}
	}
	l, err := oplog.Load(s, "issue", c.ID)
	if err != nil || len(l.Entries) != 1 {
		t.Fatalf("entries = %v, %v", l, err)
	}
}

// mk is must for a call's two results.
func mk(t *testing.T) func(*Issue, error) *Issue {
	return func(is *Issue, err error) *Issue {
		t.Helper()
		return must(t, is, err)
	}
}
