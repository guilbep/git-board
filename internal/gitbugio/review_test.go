package gitbugio_test

import (
	"fmt"
	"testing"

	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// eventByID is the thread's event with that id.
func eventByID(t *testing.T, b *gitbugio.Backend, issue, id string) gitbugio.Event {
	t.Helper()
	th, err := b.Thread(issue)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range th.Events {
		if e.ID == id {
			return e
		}
	}
	t.Fatalf("no event %s in %+v", id, th.Events)
	return gitbugio.Event{}
}

func TestReviewComments(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	issue := gitbugiotest.NewIssue(t, b, "review")
	sha := "0123456789abcdef0123456789abcdef01234567"

	line, err := b.CommentWith(issue, "why?", map[string]string{
		"review:path": "a.txt", "review:line": "3", "review:side": "new", "review:commit": sha, "review:pr": "github.com/o/r#26",
	})
	if err != nil {
		t.Fatal(err)
	}
	r := eventByID(t, b, issue, line).Review
	if r == nil || r.Path != "a.txt" || r.Line != 3 || r.Side != "new" || r.Commit != sha || r.PR != "github.com/o/r#26" || r.Resolved {
		t.Fatalf("line comment review = %+v", r)
	}

	reply, err := b.CommentWith(issue, "because", map[string]string{"review:path": "a.txt", "review:line": "3", "review:side": "new", "review:commit": sha, "review:reply-to": line})
	if err != nil {
		t.Fatal(err)
	}
	if r := eventByID(t, b, issue, reply).Review; r == nil || r.ReplyTo != line {
		t.Fatalf("reply review = %+v", r)
	}

	approve, err := b.CommentWith(issue, "", map[string]string{"review:state": "APPROVE", "review:commit": sha})
	if err != nil {
		t.Fatal(err)
	}
	if r := eventByID(t, b, issue, approve).Review; r == nil || r.State != "APPROVE" {
		t.Fatalf("review = %+v", r)
	}

	plain, err := b.AddComment(issue, "plain")
	if err != nil {
		t.Fatal(err)
	}
	if r := eventByID(t, b, issue, plain).Review; r != nil {
		t.Fatalf("plain comment review = %+v", r)
	}

	bad, err := b.CommentWith(issue, "x", map[string]string{"review:path": "a.txt", "review:line": "three", "review:side": "new", "review:commit": sha})
	if err != nil {
		t.Fatal(err)
	}
	if r := eventByID(t, b, issue, bad).Review; r == nil || r.Line != 0 || r.Path != "a.txt" {
		t.Fatalf("non-numeric line: review = %+v", r)
	}

	got, err := b.CommentReview(issue, line)
	if err != nil || got == nil || got.Path != "a.txt" || got.Line != 3 {
		t.Fatalf("CommentReview = %+v, %v", got, err)
	}
	if got, err := b.CommentReview(issue, plain); err != nil || got != nil {
		t.Fatalf("CommentReview(plain) = %+v, %v; want nil", got, err)
	}
	if _, err := b.CommentReview(issue, "nope"); err == nil {
		t.Fatal("CommentReview of no comment")
	}
}

// Resolve is a SetMetadata on the thread's first comment: the last one wins,
// though git-bug's own metadata keeps the first.
func TestResolveTwice(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	issue := gitbugiotest.NewIssue(t, b, "review")
	line, err := b.CommentWith(issue, "why?", map[string]string{"review:path": "a.txt", "review:line": "1", "review:side": "new", "review:commit": "abc"})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []string{"true", "false"} {
		if err := b.SetCommentMetadata(issue, line, map[string]string{"review:resolved": v}); err != nil {
			t.Fatal(err)
		}
		if r := eventByID(t, b, issue, line).Review; r == nil || r.Resolved != (v == "true") {
			t.Fatalf("after resolved=%s: review = %+v", v, r)
		}
	}
	if err := b.SetCommentMetadata(issue, "nope", map[string]string{"review:resolved": "true"}); err == nil {
		t.Fatal("SetCommentMetadata on no comment")
	}
}

// A review comment goes to GitHub's pull request only: it never waits for
// another forge, and one pulled from the pull request waits for none.
func TestReviewCommentsPending(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	author, err := b.TestCache().Identities().New("Alice", "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	bug, _, err := b.TestCache().Bugs().NewRaw(author, 1, "synced", "", nil, map[string]string{
		"sync:github.com/o/r:id":          "I_1",
		"sync:github.com/o/r:url":         "https://github.com/o/r/issues/7",
		"sync:gitlab.example.com/g/r:id":  "gid://gitlab/WorkItem/3",
		"sync:gitlab.example.com/g/r:url": "https://gitlab.example.com/g/r/-/work_items/3",
		"sync:gitlab.example.com/g/r:iid": "3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := bug.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	issue := bug.Id().String()
	plain, _ := b.AddComment(issue, "plain")
	line, err := b.CommentWith(issue, "why?", map[string]string{
		"review:path": "a.txt", "review:line": "3", "review:side": "new", "review:commit": "0123456789abcdef0123456789abcdef01234567",
	})
	if err != nil {
		t.Fatal(err)
	}
	pulled, err := b.CommentWith(issue, "from GitHub", map[string]string{
		"review:path": "a.txt", "review:line": "4", "review:side": "new", "review:commit": "0123456789abcdef0123456789abcdef01234567",
		"sync:github.com/o/r:pr-comment": "PRRC_1",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		id   string
		want string
	}{{plain, "[GitHub GitLab]"}, {line, "[GitHub]"}, {pulled, "[]"}} {
		if got := fmt.Sprint(eventByID(t, b, issue, c.id).Pending); got != c.want {
			t.Errorf("%s pending %s, want %s", c.id, got, c.want)
		}
	}
}

// A pull request's conversation comment, brought in by the sync, says
// which pull request it is on.
func TestReviewPROnly(t *testing.T) {
	t.Parallel()
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "review")
	cid, err := b.CommentWith(id, "on the PR", map[string]string{"review:pr": "github.com/o/r#26"})
	if err != nil {
		t.Fatal(err)
	}
	if rv := eventByID(t, b, id, cid).Review; rv == nil || rv.PR != "github.com/o/r#26" {
		t.Fatalf("review = %+v, want its pull request", rv)
	}
}
