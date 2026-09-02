package gitbugio_test

import (
	"testing"

	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

func TestListIssuesAndGetLabels(t *testing.T) {
	b := gitbugiotest.New(t)
	id1 := gitbugiotest.NewIssue(t, b, "Fix invoice bug")
	id2 := gitbugiotest.NewIssue(t, b, "Write specs")
	gitbugiotest.CloseIssue(t, b, id2)

	if _, err := b.ApplyLabelChange(id1, []string{"status:ready"}, nil); err != nil {
		t.Fatalf("ApplyLabelChange: %v", err)
	}

	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	if len(issues) != 2 {
		t.Fatalf("len(issues) = %d, want 2", len(issues))
	}

	byTitle := map[string]gitbugio.Issue{}
	for _, i := range issues {
		byTitle[i.Title] = i
	}

	fix := byTitle["Fix invoice bug"]
	if fix.Status != "open" {
		t.Errorf("fix.Status = %q, want open", fix.Status)
	}
	if len(fix.Labels) != 1 || fix.Labels[0] != "status:ready" {
		t.Errorf("fix.Labels = %v, want [status:ready]", fix.Labels)
	}

	specs := byTitle["Write specs"]
	if specs.Status != "closed" {
		t.Errorf("specs.Status = %q, want closed", specs.Status)
	}

	labels, err := b.GetLabels(id1)
	if err != nil {
		t.Fatalf("GetLabels: %v", err)
	}
	if len(labels) != 1 || labels[0] != "status:ready" {
		t.Errorf("GetLabels = %v", labels)
	}
}

func TestApplyLabelChangeAddAndRemove(t *testing.T) {
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "x")

	labels, err := b.ApplyLabelChange(id, []string{"status:backlog", "prio:p1"}, nil)
	if err != nil {
		t.Fatalf("add: %v", err)
	}
	if len(labels) != 2 {
		t.Fatalf("labels after add = %v", labels)
	}

	labels, err = b.ApplyLabelChange(id, []string{"status:ready"}, []string{"status:backlog"})
	if err != nil {
		t.Fatalf("swap: %v", err)
	}
	want := map[string]bool{"status:ready": true, "prio:p1": true}
	if len(labels) != 2 {
		t.Fatalf("labels after swap = %v", labels)
	}
	for _, l := range labels {
		if !want[l] {
			t.Errorf("unexpected label %q in %v", l, labels)
		}
	}
}

func TestApplyLabelChangeNoOpDoesNotError(t *testing.T) {
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "x")

	if _, err := b.ApplyLabelChange(id, nil, nil); err != nil {
		t.Fatalf("empty add/remove should be a no-op, got err: %v", err)
	}
}

func TestListIssuesExposesGithubURL(t *testing.T) {
	b := gitbugiotest.New(t)
	gitbugiotest.NewIssue(t, b, "local only")
	gitbugiotest.NewImportedIssue(t, b, "from github", "https://github.com/o/r/issues/42")

	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}
	byTitle := map[string]gitbugio.Issue{}
	for _, i := range issues {
		byTitle[i.Title] = i
	}
	if byTitle["local only"].GithubURL != "" {
		t.Errorf("local only GithubURL = %q, want empty", byTitle["local only"].GithubURL)
	}
	if got := byTitle["from github"].GithubURL; got != "https://github.com/o/r/issues/42" {
		t.Errorf("GithubURL = %q, want https://github.com/o/r/issues/42", got)
	}
}

func TestGetIssueResolvesByIDPrefix(t *testing.T) {
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "prefix test")

	issue, err := b.GetIssue(id[:8])
	if err != nil {
		t.Fatalf("GetIssue by prefix: %v", err)
	}
	if issue.Title != "prefix test" {
		t.Errorf("Title = %q", issue.Title)
	}
}
