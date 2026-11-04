package syncer

import (
	"reflect"
	"testing"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/issue"
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
	if _, err := issue.Do(t.Context(), s, is.ID, issue.Delete(is.Author, is.BodyOp)); err != nil {
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
	if _, err := issue.Do(t.Context(), s, is.ID, issue.Delete(is.Author, is.Comments[0].ID)); err != nil {
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
	gone, err := issue.Create(t.Context(), s, issue.NewCreate("a", "Secret", "token=x"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := issue.Do(t.Context(), s, gone.ID, issue.Delete("a", gone.BodyOp)); err != nil {
		t.Fatal(err)
	}
	kept, err := issue.Create(t.Context(), s, issue.NewCreate("a", "Kept", ""), issue.NewComment("a", "token=y"), issue.NewComment("a", "fine"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := issue.Do(t.Context(), s, kept.ID, issue.Delete("a", kept.Comments[0].ID)); err != nil {
		t.Fatal(err)
	}
	syncAll(t, s, f)
	all := f.All()
	if len(all) != 1 || all[0].Title != "Kept" {
		t.Fatalf("forge issues = %+v", all)
	}
	var bodies []string
	for _, c := range all[0].Comments {
		bodies = append(bodies, unwrap(c.Body).body)
	}
	if !reflect.DeepEqual(bodies, []string{"fine"}) {
		t.Errorf("forge comments = %q", bodies)
	}
}

func issueRef(id string) string { return "refs/board/issue/" + id }
