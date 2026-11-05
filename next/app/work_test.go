package app

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/next/domain/cycle"
	"github.com/guilbep/git-board/next/domain/cycle/work"
	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/issues/issuesync"
	"github.com/guilbep/git-board/next/kernel"
	bsync "github.com/guilbep/git-board/next/sync"
)

// workBoard is co2Board with the cycle on.
func workBoard(t *testing.T) (string, *Board) {
	t.Helper()
	dir, b := co2Board(t)
	f := filepath.Join(dir, ".git-board.toml")
	toml, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f, append(toml, "\n[experimental]\nfeatures = [\"cycle\"]\n"...), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	c.Issues.Me = b.Issues.Me
	return dir, c
}

// workAsShow is the Work page as it was computed before its rows were
// kept: each open issue's thread from Show, its evidence read from git
// with nothing kept. The reference workRows must answer.
func workAsShow(t *testing.T, b *Board, me string, team bool) work.Groups {
	t.Helper()
	ctx := t.Context()
	m, err := b.viewer(ctx, me)
	if err != nil {
		t.Fatal(err)
	}
	list, err := b.Issues.List(ctx, issues.ListReq{Filter: issues.DefaultFilter, Me: m.Login})
	if err != nil {
		t.Fatal(err)
	}
	var rows []work.Row
	for _, is := range list {
		th, err := b.Issues.Show(ctx, is.ID)
		if err != nil {
			t.Fatal(err)
		}
		in, err := b.Evidence(ctx, th, m.Login, nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		rows = append(rows, work.RowOf(th, cycle.Compute(in), m))
	}
	return work.Group(rows, m.Login, team)
}

// linkedIssue is forgeIssue on a repository of its own, o/r<n>: one
// sync record each.
func linkedIssue(t *testing.T, b *Board, n int, title string, ops ...kernel.Op) string {
	t.Helper()
	me := string(b.Issues.Me)
	is, err := issuesync.CreateIssue(t.Context(), b.Kernel, append([]kernel.Op{issuesync.NewCreate(me, title, "")}, ops...)...)
	if err != nil {
		t.Fatal(err)
	}
	u := fmt.Sprintf("https://github.com/o/r%d/issues/%d", n, n)
	data, _ := json.Marshal(map[string]string{"issue": is.ID, "forge_id": u, "number": fmt.Sprint(n), "url": u})
	if _, err := bsync.Kind.Create(t.Context(), b.Kernel, bsync.StateID(fmt.Sprintf("github.com/o/r%d", n)), kernel.Op{Kind: "link", Author: me, Time: time.Now().Unix(), Data: data}); err != nil {
		t.Fatal(err)
	}
	return is.ID
}

func sameWork(t *testing.T, b *Board, when string) {
	t.Helper()
	for _, team := range []bool{false, true} {
		got, err := b.Work(t.Context(), "", team)
		if err != nil {
			t.Fatal(err)
		}
		want := workAsShow(t, b, "", team)
		g, _ := json.MarshalIndent(got, "", " ")
		w, _ := json.MarshalIndent(want, "", " ")
		if string(g) != string(w) {
			t.Fatalf("%s, team %v: the Work page differs from each issue's Show and evidence:\n%s\nwant\n%s", when, team, g, w)
		}
		if len(got.Needs)+len(got.Waiting) == 0 {
			t.Fatalf("%s, team %v: no row", when, team)
		}
	}
}

// The Work page's rows, read at once and kept, are each open issue's
// Show and evidence, as they were read one by one: on a first read, a
// second, and after each kind of change a row reads (its thread, a
// person, a forge's record, a branch, a plan file).
func TestWorkAsShow(t *testing.T) {
	t.Parallel()
	dir, b := workBoard(t)
	ctx := t.Context()
	me := string(b.Issues.Me)
	erin, err := b.People.New(ctx, "erin", "erin@example.com")
	if err != nil {
		t.Fatal(err)
	}
	other := string(erin.ID)
	plan := "docs/src/implementation-plans/2518-x.md"
	commitFile(t, dir, plan, strings.Replace(planDoc, "%s", "accepted", 1))
	gitT(t, dir, "switch", "-q", "-c", "fix/2518-x")
	commitFile(t, dir, "api/a_test.go", "package api\n")
	gitT(t, dir, "switch", "-q", "dev")

	ids := []string{
		forgeIssue(t, b, 2518, "Crash on save (#2518)",
			issuesync.ChangeLabels(me, []string{"status:in-progress", "risk:normal", "assignee:dev"}, nil),
			issuesync.NewCommentMeta(me, "Plan approved", map[string]string{"plan:approve": "1", "plan:commit": "abc1234"}),
			issuesync.NewCommentMeta(other, "Signed off qa", map[string]string{"signoff:status": "qa", "signoff:commit": "abc1234", "signoff:why": "by hand"})),
		linkedIssue(t, b, 2519, "A question",
			issuesync.ChangeLabels(me, []string{"status:ready"}, nil),
			issuesync.NewComment(other, "Which table holds it?"),
			issuesync.NewComment(other, "A plain note")),
		linkedIssue(t, b, 2520, "An agent asks",
			issuesync.NewCommentMeta(other, "Go on", map[string]string{"ask": "1"}),
			issuesync.NewComment(me, "Answered?")),
		linkedIssue(t, b, 2521, "Criteria",
			issuesync.ChangeLabels(me, []string{"status:in-review", "assignee:erin"}, nil),
			issuesync.NewCommentMeta(me, "", map[string]string{"intent:criteria": "none"})),
		linkedIssue(t, b, 2522, "Closed", issuesync.SetStatus(me, "closed")),
	}
	for i := range 8 { // plain ones, the most common
		is, err := issuesync.CreateIssue(ctx, b.Kernel, issuesync.NewCreate(me, fmt.Sprintf("Plain %d", i),
			"## Acceptance criteria\n\n- [ ] it works\n- [ ] it is fast\n"),
			issuesync.ChangeLabels(me, []string{"status:backlog"}, nil))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, is.ID)
	}
	sameWork(t, b, "first read")
	sameWork(t, b, "read again")

	if _, _, err := b.Issues.Comment(ctx, ids[1], "Is it the rows table?"); err != nil {
		t.Fatal(err)
	}
	sameWork(t, b, "a comment")
	if _, err := b.People.New(ctx, "frank", "frank@example.com"); err != nil {
		t.Fatal(err)
	}
	sameWork(t, b, "a person")
	linkedIssue(t, b, 2523, "Another forge's", issuesync.ChangeLabels(me, []string{"status:qa"}, nil))
	sameWork(t, b, "a record")
	gitT(t, dir, "switch", "-q", "-c", "feat/2519-table")
	commitFile(t, dir, "api/b_test.go", "package api\n")
	gitT(t, dir, "switch", "-q", "dev")
	sameWork(t, b, "a branch")
	commitFile(t, dir, "docs/src/implementation-plans/2519-y.md", "---\nissue: 2519\nstatus: draft\n---\n# Plan\n")
	sameWork(t, b, "a plan file on the base")
	if err := b.Issues.SetStatus(ctx, ids[2], "closed"); err != nil {
		t.Fatal(err)
	}
	sameWork(t, b, "a close")
}
