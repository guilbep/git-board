package store_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/model"
	"github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/internal/store/storetest"
)

// Each kind of change undone: labels, status, title, a comment's edit, a
// new comment. Undoing twice redoes.
func TestUndo(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	id, err := b.NewIssue("First", "", []string{"bug"})
	if err != nil {
		t.Fatal(err)
	}
	undo := func() store.Undone {
		t.Helper()
		u, err := b.Undo(id[:8], false)
		if err != nil {
			t.Fatalf("undo: %v", err)
		}
		return u
	}
	if _, err := b.Undo(id, false); !errors.Is(err, store.ErrCantUndo) {
		t.Errorf("undo of the create = %v, want can't undo", err)
	}

	if _, err := b.ApplyLabelChange(id, []string{"ui", "bug"}, nil); err != nil { // bug: there already
		t.Fatal(err)
	}
	undo()
	if l, _ := b.GetLabels(id); !slices.Equal(l, []string{"bug"}) {
		t.Errorf("labels after undo = %v", l)
	}
	undo() // redo
	if l, _ := b.GetLabels(id); !slices.Equal(l, []string{"bug", "ui"}) {
		t.Errorf("labels after a second undo = %v", l)
	}

	if err := b.CloseBug(id); err != nil {
		t.Fatal(err)
	}
	undo()
	if is, _ := b.GetIssue(id); is.Status != "open" {
		t.Errorf("status after undo = %s", is.Status)
	}

	if err := b.SetTitle(id, "Second"); err != nil {
		t.Fatal(err)
	}
	if u := undo(); is(t, b, id).Title != "First" || len(u.What) != 1 {
		t.Errorf("title after undo = %q, %+v", is(t, b, id).Title, u)
	}

	c, err := b.AddComment(id, "one")
	if err != nil {
		t.Fatal(err)
	}
	if err := b.EditComment(id, c, "two", ""); err != nil {
		t.Fatal(err)
	}
	undo()
	if th, _ := b.Thread(id); th.Events[len(th.Events)-1].Body != "one" {
		t.Errorf("comment after undoing its edit = %q", th.Events[len(th.Events)-1].Body)
	}
	undo()                                       // the redo of the edit
	undo()                                       // the redo's undo: one again
	if _, err := b.Undo("", false); err != nil { // anywhere: the last change is still on id
		t.Fatal(err)
	}
	if th, _ := b.Thread(id); th.Events[len(th.Events)-1].Body != "two" {
		t.Errorf("comment after a redo = %q", th.Events[len(th.Events)-1].Body)
	}

	c2, err := b.AddComment(id, "oops")
	if err != nil {
		t.Fatal(err)
	}
	undo()
	th, _ := b.Thread(id)
	if slices.ContainsFunc(th.Events, func(e model.Event) bool { return e.ID == c2 }) {
		t.Errorf("the comment undone is still there")
	}
	if _, err := b.Undo(id, false); !errors.Is(err, store.ErrCantUndo) {
		t.Errorf("undo of a delete = %v, want can't undo", err)
	}
}

// Someone else wrote after your change: refused, unless --force.
func TestUndoAfterSomeoneElse(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "T")
	if _, err := b.ApplyLabelChange(id, []string{"mine"}, nil); err != nil {
		t.Fatal(err)
	}
	ann := storetest.NewPerson(t, b, "Ann", "ann")
	op := issue.ChangeLabels(ann.ID, []string{"hers"}, nil)
	op.Time = time.Now().Unix()
	storetest.Do(t, b, id, op)
	_, err := b.Undo(id, false)
	if !errors.Is(err, store.ErrWroteAfter) {
		t.Fatalf("undo = %v, want someone wrote after", err)
	}
	if _, err := b.Undo(id, true); err != nil {
		t.Fatal(err)
	}
	if l, _ := b.GetLabels(id); !slices.Equal(l, []string{"hers"}) {
		t.Errorf("labels = %v", l)
	}
}

// Undo anywhere takes your latest change, on any issue; a delete there
// is refused, not skipped for an older change.
func TestUndoAnywhere(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	a := storetest.NewIssue(t, b, "A")
	c := storetest.NewIssue(t, b, "C")
	if _, err := b.ApplyLabelChange(a, []string{"x"}, nil); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond) // times are to the second
	if _, err := b.ApplyLabelChange(c, []string{"y"}, nil); err != nil {
		t.Fatal(err)
	}
	u, err := b.Undo("", false)
	if err != nil || u.Issue != c {
		t.Fatalf("undo = %+v, %v; want on C", u, err)
	}
	time.Sleep(1100 * time.Millisecond)
	if _, err := b.DeleteIssue(a); err != nil {
		t.Fatal(err)
	}
	if _, err := b.Undo("", false); !errors.Is(err, store.ErrCantUndo) {
		t.Errorf("undo after a delete = %v, want can't undo", err)
	}
}

func is(t *testing.T, b *store.Backend, id string) model.Issue {
	t.Helper()
	got, err := b.GetIssue(id)
	if err != nil {
		t.Fatal(err)
	}
	return got
}
