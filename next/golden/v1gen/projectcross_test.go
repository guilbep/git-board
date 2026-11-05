package main

// The GitHub Project bridge's cross test (ADR 0021): v1's ghbridge and
// v2's ghproject take turns on one board against one fake Project
// (projectfake). v1 reads it as gh would have answered, through an
// adapter; v2 through GitHub's GraphQL API, served by the fake. At every
// turn both plan the same, and once one has run, neither has anything
// left to do: what one wrote (labels, bases, Project values), the other
// takes as agreed.

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	v1config "github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghbridge"
	v1store "github.com/guilbep/git-board/internal/store"
	"github.com/guilbep/git-board/next/conf"
	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/issues/issuesync"
	people2 "github.com/guilbep/git-board/next/domain/people"
	"github.com/guilbep/git-board/next/forge"
	"github.com/guilbep/git-board/next/forge/fake"
	ghv2 "github.com/guilbep/git-board/next/forge/github"
	"github.com/guilbep/git-board/next/kernel"
	"github.com/guilbep/git-board/next/store/git"
	"github.com/guilbep/git-board/next/tools/ghproject"
	"github.com/guilbep/git-board/next/tools/ghproject/projectfake"
	"github.com/guilbep/git-board/next/view"
)

const crossProjectToml = `[fields.status]
single = true
options = ["backlog", "ready", "in-review", "qa"]

[fields.status.project]
qa = "QA Validation"

[fields.prio]
single = true
options = ["p0", "p1", "p2"]

[fields.est]
single = true
numeric = true

[[sprints.list]]
slug = "sprint-4"
title = "Sprint 4"
start = 2026-10-05

[github-project]
owner = "o"
number = 1
write = true
`

// v1Project is the fake Project as v1's ghbridge reads and writes it: the
// rows gh project item-list would print, its fields, its edits.
func v1Project(f *projectfake.Fake) ghbridge.Project {
	return ghbridge.Project{
		Items: func(owner string, number, limit int) ([]ghbridge.Item, error) {
			var out []ghbridge.Item
			for _, it := range f.Items() {
				v := maps.Clone(it.Values)
				take := func(k string) string { s := v[k]; delete(v, k); return s }
				row := ghbridge.Item{ID: it.ID, Title: it.Title, Assignees: it.Assignees, Labels: it.Labels,
					Content: ghbridge.Content{Number: it.Number, Type: it.Type, URL: it.URL},
					Status:  take("Status"), Priority: take("Priority"), Module: take("Module"), Category: take("Category"), MoSCoW: take("MoSCoW")}
				if s := take("Est. hours"); s != "" {
					x, _ := strconv.ParseFloat(s, 64)
					row.EstHours = &x
				}
				if s := take("Sprint"); s != "" {
					row.Sprint = &ghbridge.Sprint{Title: s}
				}
				if it.Milestone != "" {
					row.Milestone = &ghbridge.Milestone{Title: it.Milestone}
				}
				delete(v, "Title")
				if len(v) > 0 {
					row.Extra = v
				}
				out = append(out, row)
			}
			return out, nil
		},
		Meta: func(owner string, number int) (ghbridge.ProjectMeta, error) {
			m := ghbridge.ProjectMeta{ID: f.ID, Title: f.Title, Fields: map[string]ghbridge.ProjectField{}}
			for _, fl := range f.Fields() {
				pf := ghbridge.ProjectField{ID: fl.ID, Options: map[string]string{}}
				for _, c := range fl.Choices {
					pf.Options[c.Name] = c.ID
				}
				m.Fields[fl.Name] = pf
			}
			return m, nil
		},
		Writer:     v1Writer{f},
		Assignable: func(repo string) ([]string, error) { return f.Assignable(repo), nil },
	}
}

type v1Writer struct{ f *projectfake.Fake }

func (w v1Writer) Edit(projectID string, e ghbridge.ProjectEdit) error {
	if projectID != w.f.ID {
		return fmt.Errorf("no Project %s", projectID)
	}
	if e.Kind == ghbridge.EditAssignees {
		w.f.Assign(e.URL, e.Add, e.Remove)
		return nil
	}
	return w.f.Set(e.ItemID, e.FieldID, e.Kind, e.Arg)
}

// projectCross is one repository, its Project, and both bridges.
type projectCross struct {
	t   *testing.T
	dir string
	f   *projectfake.Fake
	p2  *ghv2.ProjectV2
}

func newProjectCross(t *testing.T) *projectCross {
	t.Helper()
	ctx := t.Context()
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(crossProjectToml), 0o644); err != nil {
		t.Fatal(err)
	}
	c := &projectCross{t: t, dir: dir}
	b := c.kernel()
	me, err := issuesync.CreatePerson(ctx, b, issuesync.NewPerson("", "me", ""))
	if err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", dir, "config", "board.me", string(me.ID)).CombinedOutput(); err != nil {
		t.Fatalf("git config: %v\n%s", err, out)
	}
	gh := fake.New("github.com/o/r")
	for _, title := range []string{"One", "Two", "Three"} {
		gh.Add(forge.Issue{Author: forge.Account{ID: "7", Login: "ann"}, Title: title})
	}
	if _, err := issuesync.Pull(ctx, b, issuesync.Options{}, gh); err != nil {
		t.Fatal(err)
	}

	f := projectfake.New("o", 1)
	f.AddField(projectfake.Field{ID: "F-status", Name: "Status", DataType: "SINGLE_SELECT", Choices: []projectfake.Choice{
		{ID: "o-backlog", Name: "Backlog"}, {ID: "o-ready", Name: "Ready"}, {ID: "o-review", Name: "In review"}, {ID: "o-qa", Name: "QA Validation"}}})
	f.AddField(projectfake.Field{ID: "F-prio", Name: "Priority", DataType: "SINGLE_SELECT", Choices: []projectfake.Choice{
		{ID: "o-p0", Name: "P0"}, {ID: "o-p1", Name: "P1"}, {ID: "o-p2", Name: "P2"}}})
	f.AddField(projectfake.Field{ID: "F-est", Name: "Est. hours", DataType: "NUMBER"})
	f.AddField(projectfake.Field{ID: "F-sprint", Name: "Sprint", DataType: "ITERATION", Choices: []projectfake.Choice{
		{ID: "i-4", Name: "Sprint 4"}, {ID: "i-h", Name: "Hypercare"}}})
	f.AddField(projectfake.Field{ID: "F-title", Name: "Title", DataType: "TITLE"})
	f.Add(projectfake.Item{Number: 1, URL: "https://github.com/o/r/issues/1", Title: "One", Assignees: []string{"ann"},
		Values: map[string]string{"Status": "Ready", "Priority": "P1", "Est. hours": "2", "Sprint": "Sprint 4"}})
	f.Add(projectfake.Item{Number: 2, URL: "https://github.com/o/r/issues/2", Title: "Two",
		Values: map[string]string{"Status": "QA Validation", "Est. hours": "0.5", "Sprint": "Hypercare"}})
	f.Add(projectfake.Item{Number: 3, URL: "https://github.com/o/r/issues/3", Title: "Three", Values: map[string]string{"Status": "Backlog"}})
	f.Add(projectfake.Item{Number: 9, Type: "PullRequest", URL: "https://github.com/o/r/pull/9", Title: "A pull"})
	f.SetAssignable("o/r", "ann", "bob")
	srv := f.Server(t)
	p2, err := ghv2.OpenProject(ctx, ghv2.Options{API: srv.URL, Token: func(context.Context, forge.Account) (string, error) { return "t", nil }}, "o", 1)
	if err != nil {
		t.Fatal(err)
	}
	c.f, c.p2 = f, p2
	return c
}

// kernel opens the board with v2, fresh: what v1 wrote since is read.
func (c *projectCross) kernel() *kernel.Board {
	c.t.Helper()
	g, err := git.Open(c.t.Context(), c.dir)
	if err != nil {
		c.t.Fatal(err)
	}
	v, err := view.Open(c.t.Context(), filepath.Join(c.t.TempDir(), "view.db"))
	if err != nil {
		c.t.Fatal(err)
	}
	b, err := kernel.Open(c.t.Context(), g, v, issues.Kind, people2.Kind)
	if err != nil {
		c.t.Fatal(err)
	}
	c.t.Cleanup(func() { b.Close() })
	return b
}

// v2 is v2's side: the board's issues, and the bridge's config.
func (c *projectCross) v2() (ghproject.Service, *ghproject.Config) {
	c.t.Helper()
	f, err := conf.Load(filepath.Join(c.dir, ".git-board.toml"))
	if err != nil {
		c.t.Fatal(err)
	}
	schema, probs := issues.Declare(f).Schema()
	if len(probs) > 0 {
		c.t.Fatal(probs[0])
	}
	me, _ := exec.Command("git", "-C", c.dir, "config", "board.me").Output()
	s := &issues.Service{Board: c.kernel(), Schema: schema, Me: kernel.ID(strings.TrimSpace(string(me)))}
	return ghproject.Service{S: s}, &ghproject.Config{Owner: "o", Number: 1, Write: true, Schema: schema,
		Forge: forge.Table{Name: "gh", Kind: "github", Host: "github.com"}}
}

// v1 is v1's side: its store, opened fresh, and its config.
func (c *projectCross) v1() (*v1store.Backend, *v1config.Config) {
	c.t.Helper()
	b, err := v1store.Open(c.dir)
	if err != nil {
		c.t.Fatal(err)
	}
	c.t.Cleanup(func() { b.Close() })
	cfg, err := v1config.Load(v1config.PathIn(c.dir))
	if err != nil {
		c.t.Fatal(err)
	}
	return b, cfg
}

// summary is a plan, the same words for v1's and v2's: per issue number,
// its label changes, its Project writes and its conflicts, sorted.
func summary1(p ghbridge.SyncPlan) []string {
	var out []string
	for _, ch := range p.Changes {
		for _, l := range ch.Added {
			out = append(out, fmt.Sprintf("#%d +%s", ch.Number, l))
		}
		for _, l := range ch.Removed {
			out = append(out, fmt.Sprintf("#%d -%s", ch.Number, l))
		}
		for _, w := range ch.Warnings {
			out = append(out, fmt.Sprintf("#%d ! %s", ch.Number, w))
		}
	}
	for _, e := range p.Edits {
		out = append(out, fmt.Sprintf("#%d -> %s=%s %s %v %v", e.Number, e.Field, e.Value, e.Arg, e.Add, e.Remove))
	}
	for _, k := range p.Conflicts {
		out = append(out, fmt.Sprintf("#%d conflict %s: %q %q %q %v", k.Number, k.Field, k.Local, k.Project, k.Base, k.HasBase))
	}
	for _, u := range p.Unmatched {
		out = append(out, fmt.Sprintf("unmatched #%d %v", u.Number, u.NotAnIssue))
	}
	slices.Sort(out)
	return out
}

func summary2(p ghproject.Plan) []string {
	var out []string
	for _, ch := range p.Changes {
		for _, l := range ch.Added {
			out = append(out, fmt.Sprintf("#%d +%s", ch.Number, l))
		}
		for _, l := range ch.Removed {
			out = append(out, fmt.Sprintf("#%d -%s", ch.Number, l))
		}
		for _, w := range ch.Warnings {
			out = append(out, fmt.Sprintf("#%d ! %s", ch.Number, w))
		}
	}
	for _, e := range p.Edits {
		out = append(out, fmt.Sprintf("#%d -> %s=%s %s %v %v", e.Number, e.Field, e.Value, e.Arg, e.Add, e.Remove))
	}
	for _, k := range p.Conflicts {
		out = append(out, fmt.Sprintf("#%d conflict %s: %q %q %q %v", k.Number, k.Field, k.Local, k.Project, k.Base, k.HasBase))
	}
	for _, u := range p.Unmatched {
		out = append(out, fmt.Sprintf("unmatched #%d %v", u.Number, u.NotAnIssue))
	}
	slices.Sort(out)
	return out
}

// plans are both bridges' plans of a sync, as they stand; they must be
// the same.
func (c *projectCross) plans() []string {
	c.t.Helper()
	b1, cfg1 := c.v1()
	s1, err := ghbridge.LoadState(b1, v1Project(c.f), cfg1, 0)
	if err != nil {
		c.t.Fatal(err)
	}
	b2, cfg2 := c.v2()
	s2, err := ghproject.Load(c.t.Context(), c.p2, b2, cfg2, 0)
	if err != nil {
		c.t.Fatal(err)
	}
	one, two := summary1(s1.Plan(cfg1)), summary2(s2.Plan(cfg2))
	if !reflect.DeepEqual(one, two) {
		c.t.Errorf("the plans differ:\nv1 %q\nv2 %q", one, two)
	}
	return one
}

func (c *projectCross) syncV1() {
	c.t.Helper()
	b, cfg := c.v1()
	s, err := ghbridge.LoadState(b, v1Project(c.f), cfg, 0)
	if err != nil {
		c.t.Fatal(err)
	}
	if _, err := ghbridge.ApplySync(b, v1Writer{c.f}, b, s.Plan(cfg), cfg, s.Meta); err != nil {
		c.t.Fatalf("v1's sync: %v", err)
	}
}

func (c *projectCross) syncV2() {
	c.t.Helper()
	b, cfg := c.v2()
	if _, err := ghproject.Run(c.t.Context(), c.p2, b, cfg, 0, ghproject.Both, true); err != nil {
		c.t.Fatalf("v2's sync: %v", err)
	}
}

// quiet checks that both plan nothing but the pull request left out, and
// that a run of each writes nothing on the Project.
func (c *projectCross) quiet(why string) {
	c.t.Helper()
	if got := c.plans(); !slices.Equal(got, []string{"unmatched #9 true"}) {
		c.t.Errorf("%s: left to do %q", why, got)
	}
	n := len(c.f.Writes())
	c.syncV1()
	c.syncV2()
	if w := c.f.Writes(); len(w) != n {
		c.t.Errorf("%s: the runs after wrote %q", why, w[n:])
	}
}

// labels are an issue's labels, by its GitHub number, as v1 and v2 read
// them; they must be the same.
func (c *projectCross) labels(n int) []string {
	c.t.Helper()
	url := fmt.Sprintf("https://github.com/o/r/issues/%d", n)
	b1, _ := c.v1()
	l1, err := b1.ListIssues()
	if err != nil {
		c.t.Fatal(err)
	}
	b2, _ := c.v2()
	l2, err := b2.Issues(c.t.Context())
	if err != nil {
		c.t.Fatal(err)
	}
	var one, two []string
	for _, is := range l1 {
		if is.GithubURL == url {
			one = slices.Sorted(slices.Values(is.Labels))
		}
	}
	for _, is := range l2 {
		if is.URL == url {
			two = slices.Sorted(slices.Values(is.Labels))
		}
	}
	if !slices.Equal(one, two) {
		c.t.Errorf("#%d: v1 reads %q, v2 %q", n, one, two)
	}
	return one
}

func (c *projectCross) issueID(n int) string {
	c.t.Helper()
	b2, _ := c.v2()
	l2, err := b2.Issues(c.t.Context())
	if err != nil {
		c.t.Fatal(err)
	}
	for _, is := range l2 {
		if is.URL == fmt.Sprintf("https://github.com/o/r/issues/%d", n) {
			return is.ID
		}
	}
	c.t.Fatalf("no #%d", n)
	return ""
}

func TestProjectCross(t *testing.T) {
	t.Parallel()
	c := newProjectCross(t)
	ctx := t.Context()

	// the first run: both plan the Project's values in; v1 takes them
	if got := c.plans(); len(got) < 8 {
		t.Fatalf("the first plan: %q", got)
	}
	c.syncV1()
	if got := c.labels(1); !slices.Equal(got, []string{"assignee:ann", "est:2", "prio:p1", "sprint:sprint-4", "status:ready"}) {
		t.Errorf("#1 after v1: %q", got)
	}
	if got := c.labels(2); !slices.Equal(got, []string{"est:0.5", "sprint:hypercare", "status:qa"}) {
		t.Errorf("#2 after v1: %q", got)
	}
	c.quiet("after v1's first run")

	// v2 writes here, the Project changes: v2 syncs both
	b2, _ := c.v2()
	if _, err := b2.S.ChangeLabels(ctx, c.issueID(1), []string{"status:in-review", "assignee:bob"}, []string{"status:ready", "assignee:ann"}); err != nil {
		t.Fatal(err)
	}
	c.f.Edit("PVTI_2", func(it *projectfake.Item) { it.Values["Priority"] = "P0"; delete(it.Values, "Sprint") })
	c.plans()
	c.syncV2()
	if got := c.labels(2); !slices.Equal(got, []string{"est:0.5", "prio:p0", "status:qa"}) {
		t.Errorf("#2 after v2: %q", got)
	}
	if it := c.f.Items()[0]; it.Values["Status"] != "In review" || !slices.Equal(it.Assignees, []string{"bob"}) {
		t.Errorf("#1 on the Project after v2: %+v", it)
	}
	c.quiet("after v2's run")

	// v1 writes here, the Project changes: v1 syncs both
	b1, _ := c.v1()
	if _, err := b1.ApplyLabelChange(c.issueID(3), []string{"est:4", "prio:p2"}, nil); err != nil {
		t.Fatal(err)
	}
	c.f.Edit("PVTI_1", func(it *projectfake.Item) { it.Values["Est. hours"] = "3.5" })
	c.plans()
	c.syncV1()
	if it := c.f.Items()[2]; it.Values["Est. hours"] != "4" || it.Values["Priority"] != "P2" {
		t.Errorf("#3 on the Project after v1: %+v", it.Values)
	}
	if got := c.labels(1); !slices.Contains(got, "est:3.5") {
		t.Errorf("#1 after v1: %q", got)
	}
	c.quiet("after v1's second run")

	// both change #1's status: held by both, the same; v2 keeps
	// git-board's, and v1 has nothing left
	b2, _ = c.v2()
	if _, err := b2.S.ChangeLabels(ctx, c.issueID(1), []string{"status:qa"}, []string{"status:in-review"}); err != nil {
		t.Fatal(err)
	}
	c.f.Edit("PVTI_1", func(it *projectfake.Item) { it.Values["Status"] = "Backlog" })
	if got := c.plans(); !slices.Contains(got, `#1 conflict status: "qa" "backlog" "in-review" true`) {
		t.Errorf("the conflict: %q", got)
	}
	c.syncV1() // holds it: writes nothing of it
	if it := c.f.Items()[0]; it.Values["Status"] != "Backlog" {
		t.Errorf("v1 wrote a held field: %+v", it.Values)
	}
	b2, cfg2 := c.v2()
	st, err := ghproject.Load(ctx, c.p2, b2, cfg2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if kept, err := ghproject.ResolveIn(ctx, b2, c.p2, cfg2, st, "", 1, "status", true); err != nil || kept != "qa" {
		t.Fatalf("v2's resolve: %q, %v", kept, err)
	}
	if it := c.f.Items()[0]; it.Values["Status"] != "QA Validation" {
		t.Errorf("#1 on the Project after the resolve: %+v", it.Values)
	}
	c.quiet("after v2's resolve")

	// and the other way: v1 resolves a conflict v2 then agrees with
	b1, _ = c.v1()
	if _, err := b1.ApplyLabelChange(c.issueID(3), []string{"prio:p0"}, []string{"prio:p2"}); err != nil {
		t.Fatal(err)
	}
	c.f.Edit("PVTI_3", func(it *projectfake.Item) { it.Values["Priority"] = "P1" })
	c.plans()
	b1, cfg1 := c.v1()
	s1, err := ghbridge.LoadState(b1, v1Project(c.f), cfg1, 0)
	if err != nil {
		t.Fatal(err)
	}
	if kept, err := ghbridge.ResolveConflict(b1, v1Project(c.f), cfg1, s1, 3, "prio", false); err != nil || kept != "p1" {
		t.Fatalf("v1's resolve: %q, %v", kept, err)
	}
	if got := c.labels(3); !slices.Contains(got, "prio:p1") {
		t.Errorf("#3 after v1's resolve: %q", got)
	}
	c.quiet("after v1's resolve")
}
