package ghproject

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/people"
	"github.com/guilbep/git-board/next/forge"
	"github.com/guilbep/git-board/next/forge/github"
	"github.com/guilbep/git-board/next/kernel"
	"github.com/guilbep/git-board/next/store/git"
	bsync "github.com/guilbep/git-board/next/sync"
	"github.com/guilbep/git-board/next/tools/ghproject/projectfake"
	"github.com/guilbep/git-board/next/view"
)

func kop(kind string, author kernel.ID, data any) kernel.Op {
	raw, _ := json.Marshal(data)
	return kernel.Op{Kind: kind, Author: string(author), Time: 1700000000, Data: raw}
}

// linkedBoard is a board of n issues, each linked to github.com/o/r's
// issue of its number, and its issues' service.
func linkedBoard(t *testing.T, n int) (*issues.Service, []string) {
	t.Helper()
	ctx := t.Context()
	dir := t.TempDir()
	if out, err := exec.Command("git", "init", "-q", dir).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, out)
	}
	ds, err := git.Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	v, err := view.Open(ctx, filepath.Join(t.TempDir(), "view.db"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := kernel.Open(ctx, ds, v, issues.Kind, people.Kind)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	me, err := people.Kind.Create(ctx, b, "", kop("create", "", map[string]string{"name": "Ada", "email": "ada@example.com"}))
	if err != nil {
		t.Fatal(err)
	}
	var ids []string
	var links []kernel.Op
	for i := 1; i <= n; i++ {
		e, err := issues.Kind.Create(ctx, b, "", kop("create", me.ID, map[string]string{"title": fmt.Sprintf("Issue %d", i), "body": ""}))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, string(e.ID))
		links = append(links, kop("link", me.ID, bsync.LinkData{Issue: string(e.ID), ForgeID: fmt.Sprint(100 + i), Number: fmt.Sprint(i),
			URL: fmt.Sprintf("https://github.com/o/r/issues/%d", i)}))
	}
	if _, err := bsync.Kind.Create(ctx, b, bsync.StateID("github.com/o/r"), links...); err != nil {
		t.Fatal(err)
	}
	s := &issues.Service{Board: b, Schema: &issues.Schema{Fields: map[string]issues.Field{
		"status": {Single: true, Options: []string{"backlog", "ready", "in-review", "qa"}, Project: map[string]string{"qa": "QA Validation"}},
		"prio":   {Single: true, Options: []string{"p0", "p1", "p2"}},
	}}, Me: me.ID}
	return s, ids
}

// fakeProject is o's Project 1, its fields and two items, served.
func fakeProject(t *testing.T) (*projectfake.Fake, *github.ProjectV2) {
	t.Helper()
	f := projectfake.New("o", 1)
	f.AddField(projectfake.Field{ID: "F-status", Name: "Status", DataType: "SINGLE_SELECT", Choices: []projectfake.Choice{
		{ID: "o-backlog", Name: "Backlog"}, {ID: "o-ready", Name: "Ready"}, {ID: "o-review", Name: "In review"}, {ID: "o-qa", Name: "QA Validation"}}})
	f.AddField(projectfake.Field{ID: "F-prio", Name: "Priority", DataType: "SINGLE_SELECT", Choices: []projectfake.Choice{
		{ID: "o-p0", Name: "P0"}, {ID: "o-p1", Name: "P1"}, {ID: "o-p2", Name: "P2"}}})
	f.AddField(projectfake.Field{ID: "F-est", Name: "Est. hours", DataType: "NUMBER"})
	f.AddField(projectfake.Field{ID: "F-sprint", Name: "Sprint", DataType: "ITERATION", Choices: []projectfake.Choice{{ID: "i-4", Name: "Sprint 4"}}})
	f.AddField(projectfake.Field{ID: "F-title", Name: "Title", DataType: "TITLE"})
	f.Add(projectfake.Item{Number: 1, URL: "https://github.com/o/r/issues/1", Title: "Issue 1", Assignees: []string{"ada"},
		Values: map[string]string{"Status": "Ready", "Priority": "P1", "Est. hours": "2", "Sprint": "Sprint 4"}})
	f.Add(projectfake.Item{Number: 2, URL: "https://github.com/o/r/issues/2", Title: "Issue 2", Values: map[string]string{"Status": "Backlog"}})
	f.Add(projectfake.Item{Type: "DraftIssue", Title: "An idea"})
	srv := f.Server(t)
	p, err := github.OpenProject(t.Context(), github.Options{API: srv.URL,
		Token: func(context.Context, forge.Account) (string, error) { return "t", nil }}, "o", 1)
	if err != nil {
		t.Fatal(err)
	}
	return f, p
}

func labelsOf(t *testing.T, s *issues.Service, id string) []string {
	t.Helper()
	is, err := s.Get(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return is.Labels
}

// The bridge over GitHub's API, against a fake Project: pull, push, a
// conflict held then resolved, and a second run with nothing to do.
func TestRunAgainstTheAPI(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, ids := linkedBoard(t, 2)
	f, p := fakeProject(t)
	c := &Config{Owner: "o", Number: 1, Write: true, Schema: s.Schema, Forge: ghTable}
	b := Service{s}

	r, err := Run(ctx, p, b, c, 0, Labels, true)
	if err != nil {
		t.Fatal(err)
	}
	if n := r.Plan.Counts(); n.Matched != 2 || n.Unmatched != 1 || n.LabelChanges != 2 || n.ProjectWrites != 0 {
		t.Errorf("first pull: %+v", n)
	}
	want := []string{"assignee:ada", "est:2", "prio:p1", "sprint:sprint-4", "status:ready"}
	if got := labelsOf(t, s, ids[0]); !slices.Equal(got, want) {
		t.Errorf("issue 1's labels %v, want %v", got, want)
	}
	if r.Title != "o's Project" || r.Columns["Est. hours"] != "NUMBER" {
		t.Errorf("report %+v", r)
	}

	// nothing changed: a sync is quiet
	if r, err := Run(ctx, p, b, c, 0, Both, true); err != nil || r.Plan.Counts().LabelChanges+r.Plan.Counts().ProjectWrites != 0 {
		t.Errorf("second run: %+v, %v", r.Plan.Counts(), err)
	}

	// git-board moves issue 1 on, the Project moves issue 2: a sync takes both
	if _, err := s.ChangeLabels(ctx, ids[0], []string{"status:in-review", "prio:p0"}, []string{"status:ready", "prio:p1"}); err != nil {
		t.Fatal(err)
	}
	f.Edit("PVTI_2", func(it *projectfake.Item) { it.Values["Status"] = "QA Validation" })
	if _, err := Run(ctx, p, b, c, 0, Both, true); err != nil {
		t.Fatal(err)
	}
	if w := f.Writes(); !slices.Equal(w, []string{"#1 Status = In review", "#1 Priority = P0"}) && !slices.Equal(w, []string{"#1 Priority = P0", "#1 Status = In review"}) {
		t.Errorf("writes %q", w)
	}
	if got := labelsOf(t, s, ids[1]); !slices.Contains(got, "status:qa") {
		t.Errorf("issue 2's labels %v", got)
	}

	// both change issue 2's status: held, then the Project's kept
	if _, err := s.ChangeLabels(ctx, ids[1], []string{"status:ready"}, []string{"status:qa"}); err != nil {
		t.Fatal(err)
	}
	f.Edit("PVTI_2", func(it *projectfake.Item) { it.Values["Status"] = "Backlog" })
	r, err = Run(ctx, p, b, c, 0, Both, true)
	if err != nil || len(r.Plan.Conflicts) != 1 || r.Plan.Conflicts[0].Local != "ready" || r.Plan.Conflicts[0].Project != "backlog" {
		t.Fatalf("conflict: %+v, %v", r.Plan.Conflicts, err)
	}
	st, err := Load(ctx, p, b, c, 0)
	if err != nil {
		t.Fatal(err)
	}
	if kept, err := ResolveIn(ctx, b, p, c, st, "", 2, "status", false); err != nil || kept != "backlog" {
		t.Fatalf("resolve: %q, %v", kept, err)
	}
	if got := labelsOf(t, s, ids[1]); !slices.Contains(got, "status:backlog") {
		t.Errorf("issue 2's labels after resolve %v", got)
	}
	n := len(f.Writes())
	if r, err := Run(ctx, p, b, c, 0, Both, true); err != nil || len(r.Plan.Conflicts)+r.Plan.Counts().LabelChanges+len(r.Plan.Edits) != 0 {
		t.Errorf("after resolve: %+v, %v", r.Plan, err)
	}
	if len(f.Writes()) != n {
		t.Errorf("writes after: %q", f.Writes()[n:])
	}

	// the base is the issue's metadata, under the Project's prefix
	meta, err := s.Meta(ctx, ids[1])
	if err != nil || meta["git-board:o/1:status"] != "backlog" {
		t.Errorf("meta %v, %v", meta, err)
	}
}

// A limit reads that many items, and says the Project has more.
func TestItemsLimit(t *testing.T) {
	t.Parallel()
	_, p := fakeProject(t)
	items, more, err := p.Items(t.Context(), 2)
	if err != nil || len(items) != 2 || !more {
		t.Errorf("items %d, more %v, %v", len(items), more, err)
	}
	items, more, err = p.Items(t.Context(), 0)
	if err != nil || len(items) != 3 || more || items[2].Type != "DraftIssue" {
		t.Errorf("items %+v, more %v, %v", items, more, err)
	}
	if _, err := github.OpenProject(t.Context(), github.Options{}, "", 1); err == nil {
		t.Error("a Project without an owner opened")
	}
}

// Assignees go to the GitHub issue, through REST, with write.
func TestAssigneesReachTheIssue(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	s, ids := linkedBoard(t, 2)
	f, p := fakeProject(t)
	f.SetAssignable("o/r", "ada", "bob")
	c := &Config{Owner: "o", Number: 1, Write: true, Schema: s.Schema, Forge: ghTable}
	b := Service{s}
	if _, err := Run(ctx, p, b, c, 0, Both, true); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ChangeLabels(ctx, ids[0], []string{"assignee:bob", "assignee:eve"}, []string{"assignee:ada"}); err != nil {
		t.Fatal(err)
	}
	r, err := Run(ctx, p, b, c, 0, Both, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := f.Items()[0].Assignees; !slices.Equal(got, []string{"bob"}) {
		t.Errorf("assignees on GitHub %v", got)
	}
	if len(r.Plan.Changes[0].Warnings) != 1 {
		t.Errorf("warnings %v, want eve's", r.Plan.Changes[0].Warnings)
	}
}
