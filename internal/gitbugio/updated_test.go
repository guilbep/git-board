package gitbugio_test

import (
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// An issue says when it last changed: the page marks what changed since
// your last visit.
func TestIssueKnowsWhenItLastChanged(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	before := time.Now().Add(-time.Second)
	id := gitbugiotest.NewIssue(t, b, "a")
	issue, err := b.GetIssue(id)
	if err != nil {
		t.Fatal(err)
	}
	if issue.UpdatedAt.Before(before) || issue.UpdatedAt.After(time.Now().Add(time.Second)) {
		t.Errorf("UpdatedAt %v, want about now", issue.UpdatedAt)
	}
	list, err := b.ListIssues()
	if err != nil || len(list) != 1 || list[0].UpdatedAt.IsZero() {
		t.Errorf("listed %+v %v", list, err)
	}
}

// An issue says when it was opened, besides its last change: the palette
// breaks a tie between issues a sync stamped at once by it.
func TestIssueKnowsWhenItWasOpened(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	before := time.Now().Add(-time.Second)
	id := gitbugiotest.NewIssue(t, b, "a")
	after := time.Now().Add(time.Second)
	later := time.Now().Add(time.Hour).Unix()
	gitbugiotest.CommentAs(t, b, id, gitbugiotest.NewPerson(t, b, "Person A", "a"), later, "later")
	issue, err := b.GetIssue(id)
	if err != nil {
		t.Fatal(err)
	}
	list, err := b.ListIssues()
	if err != nil || len(list) != 1 {
		t.Fatalf("listed %+v %v", list, err)
	}
	for _, is := range []gitbugio.Issue{issue, list[0]} {
		if is.CreatedAt.Before(before) || is.CreatedAt.After(after) {
			t.Errorf("CreatedAt %v, want about now", is.CreatedAt)
		}
		if is.UpdatedAt.Unix() != later {
			t.Errorf("UpdatedAt %v, want the comment's %v", is.UpdatedAt, time.Unix(later, 0))
		}
	}
}
