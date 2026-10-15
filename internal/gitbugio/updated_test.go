package gitbugio_test

import (
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// An issue says when it last changed: the page marks what changed since
// your last visit.
func TestIssueKnowsWhenItLastChanged(t *testing.T) {
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
