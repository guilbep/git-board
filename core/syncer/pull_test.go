package syncer

import (
	"context"
	"maps"
	"os/exec"
	"reflect"
	"testing"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
)

const key = "example.com/p"

func newStore(t *testing.T) *gitstore.Store {
	t.Helper()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	s, err := gitstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func ann() forge.Account { return forge.Account{ID: "7", Login: "ann"} }

func sample() forge.Issue {
	return forge.Issue{ID: "100", Number: "1", URL: "https://example.com/p/1", Author: ann(), Created: 1000,
		Title: "T", Body: "B", Status: "closed", Labels: []string{"bug", "assignee:bob"},
		Comments: []forge.Comment{{ID: "c1", Body: "hi", Author: forge.Account{ID: "8", Login: "bob"}, Time: 1100}}}
}

func pull(t *testing.T, s *gitstore.Store, f forge.Forge) Report {
	t.Helper()
	r, err := Pull(context.Background(), s, f)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func only(t *testing.T, s *gitstore.Store) *issue.Issue {
	t.Helper()
	ids, err := issue.List(s)
	if err != nil || len(ids) != 1 {
		t.Fatalf("issues = %v, %v", ids, err)
	}
	is, err := issue.Load(s, ids[0])
	if err != nil {
		t.Fatal(err)
	}
	return is
}

func refs(t *testing.T, s *gitstore.Store) map[string]gitstore.Hash {
	t.Helper()
	m, err := s.Refs("refs/board/")
	if err != nil {
		t.Fatal(err)
	}
	return maps.Clone(m)
}

func TestPullCreatesIssue(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	if r := pull(t, s, f); r != (Report{Created: 1}) {
		t.Fatalf("report = %+v", r)
	}
	is := only(t, s)
	if is.Title != "T" || is.Body != "B" || is.Status != issue.Closed || !reflect.DeepEqual(is.Labels, []string{"bug"}) ||
		is.Author != "forge:ann" || is.Created != 1000 || is.Origin == nil || is.Origin.Event != "issue:100" {
		t.Fatalf("issue = %+v", is)
	}
	if len(is.Comments) != 1 || is.Comments[0].Body != "hi" || is.Comments[0].Author != "forge:bob" ||
		is.Comments[0].Origin == nil || is.Comments[0].Origin.Forge != key || is.Comments[0].Origin.Event != "comment:c1" {
		t.Fatalf("comments = %+v", is.Comments)
	}
	st, err := loadState(s, key)
	if err != nil {
		t.Fatal(err)
	}
	if st.links["100"] != is.ID {
		t.Fatalf("links = %v", st.links)
	}
	want := map[string]string{"title": "T", "body": "B", "status": "closed", "labels": "bug", "comment:c1": "hi"}
	if !reflect.DeepEqual(st.seen[is.ID], want) {
		t.Fatalf("seen = %v", st.seen[is.ID])
	}
}

func TestPullTwiceWritesNothing(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	pull(t, s, f)
	before := refs(t, s)
	if r := pull(t, s, f); r != (Report{Unchanged: 1}) {
		t.Fatalf("report = %+v", r)
	}
	if !reflect.DeepEqual(before, refs(t, s)) {
		t.Fatal("refs moved")
	}
}

func TestPullForgeChange(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	pull(t, s, f)
	f.Edit("1", func(i *forge.Issue) { i.Title = "T2"; i.Labels = append(i.Labels, "ui"); i.Comments[0].Body = "hi!" })
	if r := pull(t, s, f); r != (Report{Updated: 1}) {
		t.Fatalf("report = %+v", r)
	}
	is := only(t, s)
	if is.Title != "T2" || !reflect.DeepEqual(is.Labels, []string{"bug", "ui"}) || is.Comments[0].Body != "hi!" {
		t.Fatalf("issue = %+v", is)
	}
	st, _ := loadState(s, key)
	if st.seen[is.ID]["title"] != "T2" || st.seen[is.ID]["labels"] != "bug\nui" {
		t.Fatalf("seen = %v", st.seen[is.ID])
	}
	l, _ := loadIssueLog(t, s, is.ID)
	last := l[len(l)-1]
	if last.Origin == nil || last.Origin.Forge != key {
		t.Fatalf("last op = %+v", last)
	}
}

func TestPullKeepsLocalChange(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	pull(t, s, f)
	is := only(t, s)
	if _, err := issue.Do(s, is.ID, issue.SetTitle("me", "mine")); err != nil {
		t.Fatal(err)
	}
	before := refs(t, s)
	if r := pull(t, s, f); r != (Report{Unchanged: 1}) {
		t.Fatalf("report = %+v", r)
	}
	if only(t, s).Title != "mine" || !reflect.DeepEqual(before, refs(t, s)) {
		t.Fatal("local change not kept, or something written")
	}
	// both changed: still nothing written.
	f.Edit("1", func(i *forge.Issue) { i.Title = "theirs" })
	pull(t, s, f)
	if only(t, s).Title != "mine" || !reflect.DeepEqual(before, refs(t, s)) {
		t.Fatal("conflict wrote something")
	}
}

func TestPullNewComment(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	f.Add(sample())
	pull(t, s, f)
	f.Edit("1", func(i *forge.Issue) {
		i.Comments = append(i.Comments, forge.Comment{ID: "c2", Body: "two", Author: ann(), Time: 1200})
	})
	if r := pull(t, s, f); r != (Report{Updated: 1}) {
		t.Fatalf("report = %+v", r)
	}
	if n := len(only(t, s).Comments); n != 2 {
		t.Fatalf("comments = %d", n)
	}
	before := refs(t, s)
	pull(t, s, f)
	if !reflect.DeepEqual(before, refs(t, s)) || len(only(t, s).Comments) != 2 {
		t.Fatal("second pull wrote")
	}
}

func TestPullBadStatus(t *testing.T) {
	t.Parallel()
	s, f := newStore(t), fake.New(key)
	bad := sample()
	bad.Status = "merged"
	f.Add(bad)
	if _, err := Pull(context.Background(), s, f); err == nil {
		t.Fatal("want an error")
	}
	if len(refs(t, s)) != 0 {
		t.Fatal("wrote")
	}
}

func loadIssueLog(t *testing.T, s *gitstore.Store, id string) ([]oplog.Entry, error) {
	t.Helper()
	l, err := oplog.Load(s, "issue", id)
	if err != nil {
		t.Fatal(err)
	}
	return l.Entries, nil
}
