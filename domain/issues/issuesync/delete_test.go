package issuesync

import (
	bsync "github.com/guilbep/git-board/sync"
	"reflect"
	"testing"

	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
)

// An issue pulled, then deleted here, stays deleted: the next sync pulls
// none of the forge's changes into it, makes no new issue of it, and
// writes nothing to the forge.
func TestDeletedIssueStaysDeleted(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	pull(t, s, f)
	is := only(t, s)
	if _, err := DoIssue(t.Context(), s, is.ID, Delete(is.Author, is.BodyOp)); err != nil {
		t.Fatal(err)
	}
	f.Edit("1", func(i *forge.Issue) {
		i.Title = "T2"
		i.Comments = append(i.Comments, forge.Comment{ID: "c2", Body: "new", Author: ann(), Time: 1200})
	})
	before := refs(t, s)
	syncAll(t, s, f)
	got := only(t, s) // still one issue
	if !got.Deleted || got.Title != "T" || len(got.Comments) != 1 {
		t.Fatalf("issue = %+v", got)
	}
	if after := refs(t, s); after[issueRef(is.ID)] != before[issueRef(is.ID)] {
		t.Errorf("the deleted issue moved")
	}
	if w := f.Writes(); len(w) != 0 {
		t.Errorf("forge writes = %+v", w)
	}
}

// A comment pulled, then deleted here, isn't pulled again, even edited on
// the forge; nor is its text pushed back.
func TestDeletedCommentStaysDeleted(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	pull(t, s, f)
	is := only(t, s)
	if _, err := DoIssue(t.Context(), s, is.ID, Delete(is.Author, is.Comments[0].ID)); err != nil {
		t.Fatal(err)
	}
	f.Edit("1", func(i *forge.Issue) { i.Comments[0].Body = "hi, edited" })
	before := refs(t, s)
	syncAll(t, s, f)
	if got := only(t, s); len(got.Comments) != 0 || len(got.Removed) != 1 {
		t.Fatalf("comments %+v, removed %+v", got.Comments, got.Removed)
	}
	if after := refs(t, s); after[issueRef(is.ID)] != before[issueRef(is.ID)] {
		t.Errorf("the issue moved")
	}
	if w := f.Writes(); len(w) != 0 {
		t.Errorf("forge writes = %+v", w)
	}
}

// An issue and a comment made here and deleted before a push never go to
// the forge.
func TestDeletedBeforePushNeverSent(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	gone, err := CreateIssue(t.Context(), s, NewCreate("a", "Secret", "token=x"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DoIssue(t.Context(), s, gone.ID, Delete("a", gone.BodyOp)); err != nil {
		t.Fatal(err)
	}
	kept, err := CreateIssue(t.Context(), s, NewCreate("a", "Kept", ""), NewComment("a", "token=y"), NewComment("a", "fine"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DoIssue(t.Context(), s, kept.ID, Delete("a", kept.Comments[0].ID)); err != nil {
		t.Fatal(err)
	}
	syncAll(t, s, f)
	all := f.All()
	if len(all) != 1 || all[0].Title != "Kept" {
		t.Fatalf("forge issues = %+v", all)
	}
	var bodies []string
	for _, c := range all[0].Comments {
		bodies = append(bodies, bsync.Unwrap(c.Body).Body)
	}
	if !reflect.DeepEqual(bodies, []string{"fine"}) {
		t.Errorf("forge comments = %q", bodies)
	}
}

func issueRef(id string) string { return "refs/board/issue/" + id }
