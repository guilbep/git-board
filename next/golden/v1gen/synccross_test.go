package main

// The sync's cross test (design.md, M4): v1's syncer and v2's issuesync
// sync one board alternately against one fake forge, v1's, which v2 reads
// through an adapter (v2Forge). They must agree: what one wrote, the
// other takes as its own and doesn't write again. It lives with v1gen, the
// one package of next/ that may import v1, and goes with it.

import (
	"context"
	"errors"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	v1forge "github.com/guilbep/git-board/core/forge"
	v1fake "github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/gitstore"
	v1issue "github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
	v1people "github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/core/syncer"
	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/issues/issuesync"
	people2 "github.com/guilbep/git-board/next/domain/people"
	"github.com/guilbep/git-board/next/forge"
	"github.com/guilbep/git-board/next/kernel"
	"github.com/guilbep/git-board/next/store/git"
	"github.com/guilbep/git-board/next/view"
)

// v2Forge is v1's fake as v2's forge.Forge.
type v2Forge struct{ f v1forge.Forge }

func account2(a v1forge.Account) forge.Account { return forge.Account{ID: a.ID, Login: a.Login} }
func account1(a forge.Account) v1forge.Account { return v1forge.Account{ID: a.ID, Login: a.Login} }
func ref1(r forge.Ref) v1forge.Ref             { return v1forge.Ref{ID: r.ID, Number: r.Number} }

func comment2(c v1forge.Comment) forge.Comment {
	return forge.Comment{ID: c.ID, Body: c.Body, Author: account2(c.Author), Time: c.Time, Edited: c.Edited}
}

func issue2(is v1forge.Issue) forge.Issue {
	out := forge.Issue{ID: is.ID, Number: is.Number, URL: is.URL, Author: account2(is.Author), Created: is.Created,
		Updated: is.Updated, Title: is.Title, Body: is.Body, Status: is.Status, Labels: is.Labels, Changed: is.Changed, Partial: is.Partial}
	for _, a := range is.Assignees {
		out.Assignees = append(out.Assignees, account2(a))
	}
	for _, c := range is.Comments {
		out.Comments = append(out.Comments, comment2(c))
	}
	return out
}

func issue1(is forge.Issue) v1forge.Issue {
	out := v1forge.Issue{ID: is.ID, Number: is.Number, URL: is.URL, Author: account1(is.Author), Created: is.Created,
		Updated: is.Updated, Title: is.Title, Body: is.Body, Status: is.Status, Labels: is.Labels, Changed: is.Changed, Partial: is.Partial}
	for _, a := range is.Assignees {
		out.Assignees = append(out.Assignees, account1(a))
	}
	for _, c := range is.Comments {
		out.Comments = append(out.Comments, v1forge.Comment{ID: c.ID, Body: c.Body, Author: account1(c.Author), Time: c.Time, Edited: c.Edited})
	}
	return out
}

// err2 is v1's error as v2's: the same sentinel, the same refusal.
func err2(err error) error {
	var r *v1forge.Refused
	switch {
	case err == nil:
		return nil
	case errors.As(err, &r):
		return &forge.Refused{Values: r.Values, Why: r.Why}
	case errors.Is(err, v1forge.ErrNotFound):
		return errors.Join(forge.ErrNotFound, err)
	case errors.Is(err, v1forge.ErrConfidential):
		return errors.Join(forge.ErrConfidential, err)
	case errors.Is(err, v1forge.ErrRefused):
		return errors.Join(forge.ErrRefused, err)
	case errors.Is(err, v1forge.ErrNoToken):
		return errors.Join(forge.ErrNoToken, err)
	}
	return err
}

func (g v2Forge) Key() string    { return g.f.Key() }
func (g v2Forge) Server() string { return g.f.Server() }
func (g v2Forge) Me(ctx context.Context) (forge.Account, error) {
	a, err := g.f.Me(ctx)
	return account2(a), err2(err)
}
func (g v2Forge) Visibility(ctx context.Context) (forge.Visibility, error) {
	v, err := g.f.Visibility(ctx)
	return forge.Visibility(v), err2(err)
}
func (g v2Forge) As(ctx context.Context, a forge.Account) (forge.Forge, error) {
	f, err := g.f.As(ctx, account1(a))
	if err != nil {
		return nil, err2(err)
	}
	return v2Forge{f}, nil
}
func (g v2Forge) Issues(ctx context.Context, since int64) ([]forge.Issue, error) {
	l, err := g.f.Issues(ctx, since)
	var out []forge.Issue
	for _, is := range l {
		out = append(out, issue2(is))
	}
	return out, err2(err)
}
func (g v2Forge) Complete(ctx context.Context, is forge.Issue) (forge.Issue, error) {
	c, err := g.f.Complete(ctx, issue1(is))
	return issue2(c), err2(err)
}
func (g v2Forge) Issue(ctx context.Context, r forge.Ref) (forge.Issue, error) {
	is, err := g.f.Issue(ctx, ref1(r))
	return issue2(is), err2(err)
}
func (g v2Forge) CreateIssue(ctx context.Context, title, body string) (forge.Issue, error) {
	is, err := g.f.CreateIssue(ctx, title, body)
	return issue2(is), err2(err)
}
func (g v2Forge) CreateComment(ctx context.Context, r forge.Ref, body string) (forge.Comment, error) {
	c, err := g.f.CreateComment(ctx, ref1(r), body)
	return comment2(c), err2(err)
}
func (g v2Forge) EditComment(ctx context.Context, r forge.Ref, id, body string) error {
	return err2(g.f.EditComment(ctx, ref1(r), id, body))
}
func (g v2Forge) SetTitle(ctx context.Context, r forge.Ref, title string) error {
	return err2(g.f.SetTitle(ctx, ref1(r), title))
}
func (g v2Forge) SetBody(ctx context.Context, r forge.Ref, body string) error {
	return err2(g.f.SetBody(ctx, ref1(r), body))
}
func (g v2Forge) SetStatus(ctx context.Context, r forge.Ref, status string) error {
	return err2(g.f.SetStatus(ctx, ref1(r), status))
}
func (g v2Forge) ChangeLabels(ctx context.Context, r forge.Ref, add, remove []string) error {
	return err2(g.f.ChangeLabels(ctx, ref1(r), add, remove))
}
func (g v2Forge) SetAssignees(ctx context.Context, r forge.Ref, users []forge.Account) ([]forge.Account, error) {
	var in []v1forge.Account
	for _, u := range users {
		in = append(in, account1(u))
	}
	kept, err := g.f.SetAssignees(ctx, ref1(r), in)
	var out []forge.Account
	for _, a := range kept {
		out = append(out, account2(a))
	}
	return out, err2(err)
}

// crossBoard is one repository, opened by v1 and by v2.
type crossBoard struct {
	t   *testing.T
	dir string
	v1  *gitstore.Store
	me  string
}

func newCrossBoard(t *testing.T) *crossBoard {
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	s, err := gitstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	p, err := v1people.Create(t.Context(), s, v1people.NewCreate("", "me", ""))
	if err != nil {
		t.Fatal(err)
	}
	return &crossBoard{t: t, dir: dir, v1: s, me: p.ID}
}

// v2 opens the board with v2, fresh: what v1 wrote since is read.
func (c *crossBoard) v2() *kernel.Board {
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

func (c *crossBoard) syncV1(f *v1fake.Forge) syncer.Report {
	c.t.Helper()
	r, err := syncer.Sync(c.t.Context(), c.v1, syncer.Options{}, f)
	if err != nil {
		c.t.Fatalf("v1's sync: %v", err)
	}
	return r
}

func (c *crossBoard) syncV2(f *v1fake.Forge) issuesync.Report {
	c.t.Helper()
	r, err := issuesync.Sync(c.t.Context(), c.v2(), issuesync.Options{}, v2Forge{f})
	if err != nil {
		c.t.Fatalf("v2's sync: %v", err)
	}
	return r
}

// state is the board as both read it: each issue's title, body, status,
// labels and comments, by id, as v1 folds it and as v2 does; they must be
// the same.
func (c *crossBoard) state() map[string][]string {
	c.t.Helper()
	ids, err := v1issue.List(c.v1)
	if err != nil {
		c.t.Fatal(err)
	}
	b := c.v2()
	out := map[string][]string{}
	for _, id := range ids {
		is, err := v1issue.Load(c.v1, id)
		if err != nil {
			c.t.Fatal(err)
		}
		one := append([]string{is.Title, is.Body, string(is.Status)}, is.Labels...)
		for _, cm := range is.Comments {
			one = append(one, cm.Body)
		}
		two, err := issuesync.LoadIssue(b, id)
		if err != nil {
			c.t.Fatal(err)
		}
		other := append([]string{two.Title, two.Body, string(two.Status)}, two.Labels...)
		for _, cm := range two.Comments {
			other = append(other, cm.Body)
		}
		if !reflect.DeepEqual(one, other) {
			c.t.Errorf("issue %s: v1 reads %q, v2 %q", id, one, other)
		}
		out[id] = one
	}
	return out
}

// quiet checks that a run of each writes nothing more, on either side.
func (c *crossBoard) quiet(f *v1fake.Forge) {
	c.t.Helper()
	n := len(f.Writes())
	if r := c.syncV1(f); r.Created+r.Updated+r.Posted+r.Pushed != 0 {
		c.t.Errorf("v1 after: %+v", r)
	}
	if r := c.syncV2(f); r.Created+r.Updated+r.Posted+r.Pushed != 0 {
		c.t.Errorf("v2 after: %+v", r)
	}
	if w := f.Writes(); len(w) != n {
		c.t.Errorf("the runs after made %d writes: %+v", len(w)-n, w[n:])
	}
}

// byTitle is the one issue with that title, as v1 reads it.
func (c *crossBoard) byTitle(title string) string {
	c.t.Helper()
	ids, err := v1issue.List(c.v1)
	if err != nil {
		c.t.Fatal(err)
	}
	var found []string
	for _, id := range ids {
		if is, err := v1issue.Load(c.v1, id); err == nil && is.Title == title {
			found = append(found, id)
		}
	}
	if len(found) != 1 {
		c.t.Fatalf("%d issues titled %q", len(found), title)
	}
	return found[0]
}

func forgeTitles(f *v1fake.Forge) []string {
	var out []string
	for _, is := range f.All() {
		out = append(out, is.Title)
	}
	slices.Sort(out)
	return out
}

// v1 and v2 sync one board alternately: each takes what the other did,
// and pushes or pulls only what changed since.
func TestSyncCross(t *testing.T) {
	t.Parallel()
	c := newCrossBoard(t)
	f := v1fake.New("example.com/p")
	ann, bob := v1forge.Account{ID: "7", Login: "ann"}, v1forge.Account{ID: "8", Login: "bob"}
	f.Add(v1forge.Issue{Author: ann, Title: "F1", Body: "fb", Labels: []string{"bug"},
		Comments: []v1forge.Comment{{Author: bob, Body: "fc"}}})
	l1, err := v1issue.Create(t.Context(), c.v1, v1issue.NewCreate(c.me, "L1", "lb"), v1issue.NewComment(c.me, "lc"),
		v1issue.ChangeLabels(c.me, []string{"ui"}, nil))
	if err != nil {
		t.Fatal(err)
	}

	// v1 first: it mirrors both ways
	if r := c.syncV1(f); r.Created != 1 {
		t.Errorf("v1's first run = %+v", r)
	}
	c.quiet(f)

	// v2 writes here, the forge changes: v2 syncs both
	b := c.v2()
	l2, err := issuesync.CreateIssue(t.Context(), b, issuesync.NewCreate(c.me, "L2", "from v2"),
		issuesync.ChangeLabels(c.me, []string{"bug"}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := issuesync.DoIssue(t.Context(), b, l1.ID, issuesync.SetTitle(c.me, "L1, retitled by v2"),
		issuesync.NewComment(c.me, "a second, from v2")); err != nil {
		t.Fatal(err)
	}
	f.Edit("1", func(is *v1forge.Issue) {
		is.Body = "fb, edited on the forge"
		is.Labels = append(is.Labels, "p1")
	})
	if r := c.syncV2(f); r.Posted != 1 || r.Updated == 0 {
		t.Errorf("v2's run = %+v", r)
	}
	if got, want := forgeTitles(f), []string{"F1", "L1, retitled by v2", "L2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("after v2, forge titles = %q, want %q", got, want)
	}
	if is, err := v1issue.Load(c.v1, c.byTitle("F1")); err != nil || is.Body != "fb, edited on the forge" || !slices.Contains(is.Labels, "p1") {
		t.Errorf("after v2, F1 = %+v, %v", is, err)
	}
	c.quiet(f)

	// v1 writes here, the forge comments: v1 syncs both
	if _, err := v1issue.Do(t.Context(), c.v1, l2.ID, v1issue.NewComment(c.me, "from v1"), v1issue.SetStatus(c.me, v1issue.Closed)); err != nil {
		t.Fatal(err)
	}
	f.Edit("2", func(is *v1forge.Issue) {
		is.Comments = append(is.Comments, v1forge.Comment{Author: bob, Body: "a forge comment"})
	})
	if r := c.syncV1(f); r.Updated == 0 {
		t.Errorf("v1's second run = %+v", r)
	}
	c.quiet(f)

	if got, want := forgeTitles(f), []string{"F1", "L1, retitled by v2", "L2"}; !reflect.DeepEqual(got, want) {
		t.Errorf("forge titles = %q, want %q", got, want)
	}
	st := c.state()
	if len(st) != 3 {
		t.Fatalf("%d issues, want 3: %v", len(st), st)
	}
	l2now := st[l2.ID]
	if !slices.Contains(l2now, "from v1") || l2now[2] != "closed" {
		t.Errorf("L2 = %q", l2now)
	}
	if fi, _ := f.Get("3"); fi.Status != "closed" || len(fi.Comments) != 1 {
		t.Errorf("L2 on the forge = %+v", fi)
	}
}

// v1 and v2 pull and push one board between two clones, each merging the
// other's offline writes: both read every merge the same.
func TestRemoteCross(t *testing.T) {
	t.Parallel()
	origin := filepath.Join(t.TempDir(), "origin.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", origin).CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	clone := func() string {
		dir := filepath.Join(t.TempDir(), "clone")
		if out, err := exec.Command("git", "clone", "-q", origin, dir).CombinedOutput(); err != nil {
			t.Fatalf("git clone: %v\n%s", err, out)
		}
		return dir
	}
	a, err := gitstore.Open(clone()) // v1's clone
	if err != nil {
		t.Fatal(err)
	}
	bDir := clone() // v2's
	ctx := t.Context()
	g, err := git.Open(ctx, bDir)
	if err != nil {
		t.Fatal(err)
	}
	defer g.Close()
	kinds := []string{"issue", "person", "sync", "release"}
	v2pull := func() {
		t.Helper()
		if _, err := g.Fetch(ctx, "origin", kinds...); err != nil {
			t.Fatal(err)
		}
		if _, err := g.Merge(ctx, "origin", kinds...); err != nil {
			t.Fatal(err)
		}
	}
	v1pull := func() {
		t.Helper()
		if err := a.Fetch(ctx, "origin"); err != nil {
			t.Fatal(err)
		}
		if _, err := oplog.MergeFetched(ctx, a, "origin"); err != nil {
			t.Fatal(err)
		}
	}
	v2board := func() *kernel.Board {
		t.Helper()
		v, err := view.Open(ctx, filepath.Join(t.TempDir(), "view.db"))
		if err != nil {
			t.Fatal(err)
		}
		gg, err := git.Open(ctx, bDir)
		if err != nil {
			t.Fatal(err)
		}
		kb, err := kernel.Open(ctx, gg, v, issues.Kind, people2.Kind)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { kb.Close() })
		return kb
	}
	same := func(id string) {
		t.Helper()
		one, err := v1issue.Load(a, id)
		if err != nil {
			t.Fatal(err)
		}
		two, err := issuesync.LoadIssue(v2board(), id)
		if err != nil {
			t.Fatal(err)
		}
		var c1, c2 []string
		for _, c := range one.Comments {
			c1 = append(c1, c.Body)
		}
		for _, c := range two.Comments {
			c2 = append(c2, c.Body)
		}
		if one.Title != two.Title || !reflect.DeepEqual(c1, c2) || !reflect.DeepEqual(one.Labels, two.Labels) {
			t.Errorf("v1 reads %q %q %q, v2 %q %q %q", one.Title, c1, one.Labels, two.Title, c2, two.Labels)
		}
	}

	is, err := v1issue.Create(ctx, a, v1issue.NewCreate("a", "Made by v1", "b"))
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Push(ctx, "origin"); err != nil {
		t.Fatal(err)
	}
	v2pull()
	same(is.ID)

	// both write offline; v1 pushes first, v2 merges and pushes the merge
	if _, err := v1issue.Do(ctx, a, is.ID, v1issue.NewComment("a", "from v1")); err != nil {
		t.Fatal(err)
	}
	if _, err := issuesync.DoIssue(ctx, v2board(), is.ID, issuesync.SetTitle("b", "Retitled by v2")); err != nil {
		t.Fatal(err)
	}
	if err := a.Push(ctx, "origin"); err != nil {
		t.Fatal(err)
	}
	if err := g.Push(ctx, "origin", kinds...); err == nil {
		t.Fatal("v2 pushed over v1's")
	}
	v2pull()
	if err := g.Push(ctx, "origin", kinds...); err != nil {
		t.Fatal(err)
	}
	v1pull()
	same(is.ID)

	// again, v2 first: v1 merges
	if _, err := issuesync.DoIssue(ctx, v2board(), is.ID, issuesync.ChangeLabels("b", []string{"bug"}, nil)); err != nil {
		t.Fatal(err)
	}
	if _, err := v1issue.Do(ctx, a, is.ID, v1issue.NewComment("a", "v1 again")); err != nil {
		t.Fatal(err)
	}
	if err := g.Push(ctx, "origin", kinds...); err != nil {
		t.Fatal(err)
	}
	v1pull()
	if err := a.Push(ctx, "origin"); err != nil {
		t.Fatal(err)
	}
	v2pull()
	same(is.ID)
	one, _ := v1issue.Load(a, is.ID)
	if one.Title != "Retitled by v2" || len(one.Comments) != 2 || !slices.Contains(one.Labels, "bug") {
		t.Errorf("after both merges: %+v", one)
	}
}
