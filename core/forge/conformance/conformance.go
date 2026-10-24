// Package conformance is the suite every forge adapter runs: sync's
// scenarios from the PRD's M3, against the adapter, over a fake forge.
//
// The fake holds the forge's state, and the suite changes it as the forge's
// users would. An adapter serves that state through the forge's API (M4:
// an httptest server over the fake), and the suite syncs through the
// adapter.
package conformance

import (
	"context"
	"errors"
	"maps"
	"os/exec"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/forge"
	"github.com/guilbep/git-board/core/forge/fake"
	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/issue"
	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/core/people"
	"github.com/guilbep/git-board/core/syncer"
)

// NewForge gives the adapter under test over a fake forge. The fake itself
// is its own adapter. An adapter's tokens are the fake's: an account has one
// when the fake granted it, and the fake's posting account always has one.
type NewForge func(t *testing.T, f *fake.Forge) forge.Forge

// Run runs every scenario against the adapter newForge gives.
func Run(t *testing.T, newForge NewForge) {
	for _, sc := range []struct {
		name string
		run  func(*env)
	}{
		{"MirrorThenStable", mirrorThenStable},
		{"AnIssueOpenedOnEachSide", openedOnEachSide},
		{"ALocalEditSurvives", localEditSurvives},
		{"AForgeEditIsPulled", forgeEditIsPulled},
		{"AConflictKeepsTheLaterEdit", conflictKeepsLater},
		{"ACrashAfterAWrite", crashAfterWrite},
		{"LabelsMerge", labelsMerge},
		{"PushingAloneKeepsLabels", pushingAloneKeepsLabels},
		{"ATokenOwnerActsDirectly", tokenOwnerActsDirectly},
		{"RefsNeverMirrored", refsNeverMirrored},
		{"DeletionsNotPropagated", deletionsNotPropagated},
		{"OneIssueSyncedAlone", oneIssueAlone},
		{"TheGuard", theGuard},
		{"ConfidentialIssues", confidentialIssues},
		{"ARefusalWarnsOnce", refusalWarnsOnce},
		{"AssigneesAreTheForges", assigneesAreTheForges},
	} {
		t.Run(sc.name, func(t *testing.T) {
			t.Parallel()
			sc.run(newEnv(t, newForge))
		})
	}
}

var (
	ann = forge.Account{ID: "7", Login: "ann"}
	bob = forge.Account{ID: "8", Login: "bob"}
)

// env is one scenario's git-board repository, with me, the person who
// makes its local changes, who has no forge account.
type env struct {
	t        *testing.T
	ctx      context.Context
	s        *gitstore.Store
	me       string
	newForge NewForge
	warnings []string
}

func newEnv(t *testing.T, newForge NewForge) *env {
	dir := t.TempDir()
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	s, err := gitstore.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, ctx: t.Context(), s: s, newForge: newForge}
	p, err := people.Create(e.ctx, s, people.NewCreate("", "me", ""))
	if err != nil {
		t.Fatal(err)
	}
	e.me = p.ID
	return e
}

// forge makes a fake forge and its adapter.
func (e *env) forge(key string) (*fake.Forge, forge.Forge) {
	f := fake.New(key)
	return f, e.newForge(e.t, f)
}

func (e *env) opts(o syncer.Options) syncer.Options {
	o.Warn = func(w string) { e.warnings = append(e.warnings, w) }
	return o
}

func (e *env) sync(o syncer.Options, fs ...forge.Forge) syncer.Report {
	e.t.Helper()
	r, err := syncer.Sync(e.ctx, e.s, e.opts(o), fs...)
	if err != nil {
		e.t.Fatalf("sync: %v", err)
	}
	return r
}

// stable checks that one more run writes nothing, on either side.
func (e *env) stable(fs map[*fake.Forge]forge.Forge) {
	e.t.Helper()
	refs := e.refs()
	writes := map[*fake.Forge]int{}
	var adapters []forge.Forge
	for f, a := range fs {
		writes[f] = len(f.Writes())
		adapters = append(adapters, a)
	}
	if r := e.sync(syncer.Options{}, adapters...); r.Created+r.Updated+r.Posted+r.Pushed != 0 {
		e.t.Errorf("the run after wrote: %+v", r)
	}
	for f, n := range writes {
		if w := f.Writes(); len(w) != n {
			e.t.Errorf("%s: the run after made %d writes: %+v", f.Key(), len(w)-n, w[n:])
		}
	}
	if !reflect.DeepEqual(refs, e.refs()) {
		e.t.Error("the run after moved refs")
	}
}

func (e *env) refs() map[string]gitstore.Hash {
	e.t.Helper()
	m, err := e.s.Refs("refs/board/")
	if err != nil {
		e.t.Fatal(err)
	}
	return maps.Clone(m)
}

func (e *env) create(title, body string, ops ...oplog.Op) *issue.Issue {
	e.t.Helper()
	is, err := issue.Create(e.ctx, e.s, append([]oplog.Op{issue.NewCreate(e.me, title, body)}, ops...)...)
	if err != nil {
		e.t.Fatal(err)
	}
	return is
}

func (e *env) do(id string, ops ...oplog.Op) *issue.Issue {
	e.t.Helper()
	is, err := issue.Do(e.ctx, e.s, id, ops...)
	if err != nil {
		e.t.Fatal(err)
	}
	return is
}

func (e *env) issues() []*issue.Issue {
	e.t.Helper()
	ids, err := issue.List(e.s)
	if err != nil {
		e.t.Fatal(err)
	}
	var out []*issue.Issue
	for _, id := range ids {
		out = append(out, e.load(id))
	}
	return out
}

func (e *env) load(id string) *issue.Issue {
	e.t.Helper()
	is, err := issue.Load(e.s, id)
	if err != nil {
		e.t.Fatal(err)
	}
	return is
}

// byTitle is git-board's only issue with that title.
func (e *env) byTitle(title string) *issue.Issue {
	e.t.Helper()
	var found []*issue.Issue
	for _, is := range e.issues() {
		if is.Title == title {
			found = append(found, is)
		}
	}
	if len(found) != 1 {
		e.t.Fatalf("%d issues titled %q", len(found), title)
	}
	return found[0]
}

// onForge is the forge's only issue with that title.
func onForge(t *testing.T, f *fake.Forge, title string) forge.Issue {
	t.Helper()
	var found []forge.Issue
	for _, is := range f.All() {
		if is.Title == title {
			found = append(found, is)
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s: %d issues titled %q", f.Key(), len(found), title)
	}
	return found[0]
}

func titles(f *fake.Forge) []string {
	var out []string
	for _, is := range f.All() {
		out = append(out, is.Title)
	}
	slices.Sort(out)
	return out
}

func gbTitles(e *env) []string {
	var out []string
	for _, is := range e.issues() {
		out = append(out, is.Title)
	}
	slices.Sort(out)
	return out
}

// text is a forge text without sync's header and marker.
func text(s string) string {
	if i := strings.LastIndex(s, "\n\n<!-- git-board "); i >= 0 {
		s = s[:i]
	} else if strings.HasPrefix(s, "<!-- git-board ") {
		s = ""
	}
	if strings.HasPrefix(s, "*From ") {
		if _, rest, ok := strings.Cut(s, "\n\n"); ok {
			return rest
		}
		return ""
	}
	return s
}

func sorted(l []string) []string {
	l = slices.Clone(l)
	slices.Sort(l)
	return l
}

// mirrored makes one forge issue and one local issue, each with a
// comment and the label bug, and syncs them.
func mirrored(e *env) (*fake.Forge, forge.Forge, *issue.Issue, *issue.Issue) {
	f, a := e.forge("example.com/p")
	f.Add(forge.Issue{Author: ann, Title: "F", Body: "fb", Labels: []string{"bug"},
		Comments: []forge.Comment{{Author: bob, Body: "fc"}}})
	l := e.create("L", "lb", issue.NewComment(e.me, "lc"), issue.ChangeLabels(e.me, []string{"bug"}, nil))
	e.sync(syncer.Options{}, a)
	return f, a, e.byTitle("F"), e.load(l.ID)
}

func mirrorThenStable(e *env) {
	f, a := e.forge("example.com/p")
	f.Add(forge.Issue{Author: ann, Title: "F1", Body: "fb", Labels: []string{"bug"},
		Comments: []forge.Comment{{Author: bob, Body: "fc"}}})
	f.Add(forge.Issue{Author: ann, Title: "F2", Status: "closed"})
	e.create("L1", "lb", issue.NewComment(e.me, "lc"), issue.ChangeLabels(e.me, []string{"ui"}, nil), issue.SetStatus(e.me, issue.Closed))
	e.create("L2", "")
	r := e.sync(syncer.Options{}, a)
	if r.Created != 2 || r.Posted != 2 {
		e.t.Fatalf("report = %+v", r)
	}
	want := []string{"F1", "F2", "L1", "L2"}
	if got := titles(f); !reflect.DeepEqual(got, want) {
		e.t.Fatalf("forge = %v", got)
	}
	if got := gbTitles(e); !reflect.DeepEqual(got, want) {
		e.t.Fatalf("git-board = %v", got)
	}
	l1 := onForge(e.t, f, "L1")
	if text(l1.Body) != "lb" || l1.Status != "closed" || !reflect.DeepEqual(l1.Labels, []string{"ui"}) ||
		len(l1.Comments) != 1 || text(l1.Comments[0].Body) != "lc" {
		e.t.Fatalf("L1 on the forge = %+v", l1)
	}
	f1 := e.byTitle("F1")
	if f1.Body != "fb" || !reflect.DeepEqual(f1.Labels, []string{"bug"}) || len(f1.Comments) != 1 || f1.Comments[0].Body != "fc" {
		e.t.Fatalf("F1 in git-board = %+v", f1)
	}
	if e.byTitle("F2").Status != issue.Closed {
		e.t.Fatal("F2 isn't closed")
	}
	e.stable(map[*fake.Forge]forge.Forge{f: a})
	e.stable(map[*fake.Forge]forge.Forge{f: a})
}

func openedOnEachSide(e *env) {
	f, a, _, _ := mirrored(e)
	f.Add(forge.Issue{Author: bob, Title: "F2", Body: "new there"})
	e.create("L2", "new here")
	if r := e.sync(syncer.Options{}, a); r.Created != 1 || r.Posted != 1 {
		e.t.Fatalf("report = %+v", r)
	}
	if e.byTitle("F2").Body != "new there" || text(onForge(e.t, f, "L2").Body) != "new here" {
		e.t.Fatal("not mirrored")
	}
	e.stable(map[*fake.Forge]forge.Forge{f: a})
}

func localEditSurvives(e *env) {
	f, a, fi, li := mirrored(e)
	e.do(fi.ID, issue.SetTitle(e.me, "F2"), issue.EditText(e.me, fi.BodyOp, "fb2"), issue.EditText(e.me, fi.Comments[0].ID, "fc2"),
		issue.SetStatus(e.me, issue.Closed), issue.ChangeLabels(e.me, []string{"ui"}, []string{"bug"}))
	e.do(li.ID, issue.EditText(e.me, li.BodyOp, "lb2"), issue.EditText(e.me, li.Comments[0].ID, "lc2"))
	e.sync(syncer.Options{}, a)
	got := onForge(e.t, f, "F2")
	if got.Body != "fb2" || got.Comments[0].Body != "fc2" || got.Status != "closed" || !reflect.DeepEqual(got.Labels, []string{"ui"}) {
		e.t.Fatalf("F on the forge = %+v", got)
	}
	l := onForge(e.t, f, "L")
	if text(l.Body) != "lb2" || text(l.Comments[0].Body) != "lc2" || !strings.Contains(l.Body, "<!-- git-board issue "+li.ID) {
		e.t.Fatalf("L on the forge = %+v", l)
	}
	if is := e.load(fi.ID); is.Title != "F2" || is.Body != "fb2" || is.Comments[0].Body != "fc2" || is.Status != issue.Closed {
		e.t.Fatalf("F in git-board = %+v", is)
	}
	e.stable(map[*fake.Forge]forge.Forge{f: a})
}

func forgeEditIsPulled(e *env) {
	f, a, fi, li := mirrored(e)
	f.Edit(onForge(e.t, f, "F").Number, func(is *forge.Issue) {
		is.Title, is.Body, is.Status, is.Labels = "F2", "fb2", "closed", []string{"docs"}
		is.Comments[0].Body = "fc2"
	})
	f.Edit(onForge(e.t, f, "L").Number, func(is *forge.Issue) {
		is.Title = "L2"
		is.Comments[0].Body = strings.Replace(is.Comments[0].Body, "lc", "lc2", 1) // header and marker kept
	})
	e.sync(syncer.Options{}, a)
	got := e.load(fi.ID)
	if got.Title != "F2" || got.Body != "fb2" || got.Status != issue.Closed || !reflect.DeepEqual(got.Labels, []string{"docs"}) ||
		got.Comments[0].Body != "fc2" {
		e.t.Fatalf("F in git-board = %+v", got)
	}
	if l := e.load(li.ID); l.Title != "L2" || l.Comments[0].Body != "lc2" || l.Body != "lb" {
		e.t.Fatalf("L in git-board = %+v", l)
	}
	e.stable(map[*fake.Forge]forge.Forge{f: a})
}

func conflictKeepsLater(e *env) {
	f, a, fi, _ := mirrored(e)
	e.do(fi.ID, issue.SetTitle(e.me, "mine"), issue.EditText(e.me, fi.BodyOp, "my body"))
	now := time.Now().Unix()
	f.Edit(onForge(e.t, f, "F").Number, func(is *forge.Issue) {
		is.Title, is.Changed["title"] = "theirs", now+1000 // later than git-board's
		is.Body, is.Changed["body"] = "their body", 1      // earlier
	})
	e.sync(syncer.Options{}, a)
	got := e.load(fi.ID)
	if got.Title != "theirs" || got.Body != "my body" {
		e.t.Fatalf("git-board = %q, %q; want theirs, my body", got.Title, got.Body)
	}
	if on := onForge(e.t, f, "theirs"); on.Body != "my body" {
		e.t.Fatalf("forge body = %q", on.Body)
	}
	e.stable(map[*fake.Forge]forge.Forge{f: a})
}

// crashOnce makes the first write of op land, then fail.
func crashOnce(f *fake.Forge, op string) {
	var done atomic.Bool
	f.After(func(w fake.Write) error {
		if w.Op == op && done.CompareAndSwap(false, true) {
			return fake.ErrCrash
		}
		return nil
	})
}

func crashAfterWrite(e *env) {
	f, a := e.forge("example.com/p")
	l := e.create("L", "lb")
	crashOnce(f, "create-issue")
	if _, err := syncer.Sync(e.ctx, e.s, syncer.Options{}, a); err == nil {
		e.t.Fatal("the crash wasn't reported")
	}
	e.sync(syncer.Options{}, a)
	if got := titles(f); !reflect.DeepEqual(got, []string{"L"}) {
		e.t.Fatalf("forge = %v, want one L", got)
	}
	// a comment, crashing after it is posted
	e.do(l.ID, issue.NewComment(e.me, "lc"))
	crashOnce(f, "create-comment")
	if _, err := syncer.Sync(e.ctx, e.s, syncer.Options{}, a); err == nil {
		e.t.Fatal("the crash wasn't reported")
	}
	e.sync(syncer.Options{}, a)
	if cs := onForge(e.t, f, "L").Comments; len(cs) != 1 || text(cs[0].Body) != "lc" {
		e.t.Fatalf("comments = %+v, want one lc", cs)
	}
	if n := len(e.load(l.ID).Comments); n != 1 {
		e.t.Fatalf("git-board comments = %d", n)
	}
	e.stable(map[*fake.Forge]forge.Forge{f: a})
}

func labelsMerge(e *env) {
	f, a, fi, _ := mirrored(e)
	e.do(fi.ID, issue.ChangeLabels(e.me, []string{"ui"}, nil))
	f.Edit(onForge(e.t, f, "F").Number, func(is *forge.Issue) { is.Labels = append(is.Labels, "docs") })
	e.sync(syncer.Options{}, a)
	want := []string{"bug", "docs", "ui"}
	if got := e.load(fi.ID).Labels; !reflect.DeepEqual(got, want) {
		e.t.Fatalf("git-board = %v", got)
	}
	if got := sorted(onForge(e.t, f, "F").Labels); !reflect.DeepEqual(got, want) {
		e.t.Fatalf("forge = %v", got)
	}
	e.stable(map[*fake.Forge]forge.Forge{f: a})
}

func pushingAloneKeepsLabels(e *env) {
	f, a, fi, _ := mirrored(e)
	e.do(fi.ID, issue.ChangeLabels(e.me, []string{"ui"}, nil))
	f.Edit(onForge(e.t, f, "F").Number, func(is *forge.Issue) { is.Labels = append(is.Labels, "docs") })
	if _, err := syncer.Push(e.ctx, e.s, syncer.Options{}, a); err != nil {
		e.t.Fatal(err)
	}
	if got := sorted(onForge(e.t, f, "F").Labels); !reflect.DeepEqual(got, []string{"bug", "docs", "ui"}) {
		e.t.Fatalf("forge = %v", got)
	}
	if got := e.load(fi.ID).Labels; !reflect.DeepEqual(got, []string{"bug", "ui"}) {
		e.t.Fatalf("git-board took from the push alone: %v", got)
	}
	e.sync(syncer.Options{}, a)
	if got := e.load(fi.ID).Labels; !reflect.DeepEqual(got, []string{"bug", "docs", "ui"}) {
		e.t.Fatalf("git-board = %v", got)
	}
	e.stable(map[*fake.Forge]forge.Forge{f: a})
}

func tokenOwnerActsDirectly(e *env) {
	f, a := e.forge("example.com/p")
	f.Grant(ann)
	p, err := people.Create(e.ctx, e.s, people.NewCreate(e.me, "Ann", ""),
		people.AddAccount(e.me, people.Account{Server: a.Server(), ID: ann.ID, Login: ann.Login}))
	if err != nil {
		e.t.Fatal(err)
	}
	is, err := issue.Create(e.ctx, e.s, issue.NewCreate(p.ID, "A", "by ann"), issue.NewComment(p.ID, "ann says"),
		issue.NewComment(e.me, "me says"))
	if err != nil {
		e.t.Fatal(err)
	}
	e.sync(syncer.Options{}, a)
	got := onForge(e.t, f, "A")
	if got.Author.ID != ann.ID || got.Body != "by ann\n\n<!-- git-board issue "+is.ID+" -->" {
		e.t.Fatalf("issue = %+v; want ann's, with a marker and no header", got)
	}
	if c := got.Comments[0]; c.Author.ID != ann.ID || strings.HasPrefix(c.Body, "*From") {
		e.t.Fatalf("ann's comment = %+v", c)
	}
	if c := got.Comments[1]; c.Author.ID == ann.ID || !strings.HasPrefix(c.Body, "*From me, ") || text(c.Body) != "me says" {
		e.t.Fatalf("my comment = %+v; want the posting account's, with a header", c)
	}
	e.do(is.ID, issue.SetTitle(p.ID, "A2"))
	e.sync(syncer.Options{}, a)
	w := f.Writes()
	if last := w[len(w)-1]; last.Op != "set-title" || last.As.ID != ann.ID {
		e.t.Fatalf("last write = %+v; want ann's set-title", last)
	}
	e.stable(map[*fake.Forge]forge.Forge{f: a})
}

// refsNeverMirrored: a #n sent to another project becomes that project's
// number for the same issue, else a link to the original, else code. And a
// mention goes as code.
func refsNeverMirrored(e *env) {
	fa, a := e.forge("example.com/a")
	fb, b := e.forge("example.com/b")
	fb.Add(forge.Issue{Author: bob, Title: "native"}) // so b's numbers differ from a's
	fa.Add(forge.Issue{Author: ann, Title: "First"})
	fa.Add(forge.Issue{Author: ann, Title: "Second (#1)", Body: "Follows #1, not #9, see #3. cc @ann"})
	fa.Add(forge.Issue{Author: ann, Title: "Third"})
	e.sync(syncer.Options{}, a, b)
	first, third := onForge(e.t, fb, "First").Number, onForge(e.t, fb, "Third").Number
	second := onForge(e.t, fb, "Second (#"+first+")")
	if want := "Follows #" + first + ", not `#9`, see #" + third + ". cc `@ann`"; text(second.Body) != want {
		e.t.Fatalf("b's body = %q, want %q", text(second.Body), want)
	}
	// links are to the original, as the adapter names it
	orig := func(title string) string {
		list, err := a.Issues(e.ctx, 0)
		if err != nil {
			e.t.Fatal(err)
		}
		for _, is := range list {
			if is.Title == title {
				return "[" + a.Key() + "#" + is.Number + "](" + is.URL + ")"
			}
		}
		e.t.Fatalf("no %q on a", title)
		return ""
	}
	if !strings.HasPrefix(second.Body, "*From ann, ") || !strings.Contains(second.Body, "on "+orig("Second (#1)")) {
		e.t.Fatalf("b's header = %q", second.Body)
	}
	if e.byTitle("Second (#1)").Body != "Follows #1, not #9, see #3. cc @ann" {
		e.t.Fatal("git-board's text changed")
	}
	e.stable(map[*fake.Forge]forge.Forge{fa: a, fb: b})

	// Alone on a project without the issues it names, a reference becomes
	// a link to the original.
	fc, c := e.forge("example.com/c")
	e.sync(syncer.Options{Issue: e.byTitle("Second (#1)").ID}, c)
	got := onForge(e.t, fc, "Second (`#1`)")
	if want := "Follows " + orig("First") + ", not `#9`, see " + orig("Third") + ". cc `@ann`"; text(got.Body) != want {
		e.t.Fatalf("c's body = %q, want %q", text(got.Body), want)
	}
}

func deletionsNotPropagated(e *env) {
	f, a, fi, li := mirrored(e)
	before := len(f.All())
	f.Delete(onForge(e.t, f, "F").Number)
	f.Edit(onForge(e.t, f, "L").Number, func(is *forge.Issue) { is.Comments = nil })
	e.do(fi.ID, issue.SetTitle(e.me, "F2"))
	e.do(li.ID, issue.EditText(e.me, li.Comments[0].ID, "lc2"))
	e.sync(syncer.Options{}, a)
	if n := len(f.All()); n != before-1 {
		e.t.Fatalf("forge issues = %d, want %d", n, before-1)
	}
	if cs := onForge(e.t, f, "L").Comments; len(cs) != 0 {
		e.t.Fatalf("a deleted comment came back: %+v", cs)
	}
	if got := e.load(fi.ID); got.Title != "F2" || len(e.load(li.ID).Comments) != 1 {
		e.t.Fatal("git-board lost something")
	}
	e.stable(map[*fake.Forge]forge.Forge{f: a})
}

func oneIssueAlone(e *env) {
	f, a := e.forge("example.com/p")
	x, y := e.create("X", "xb"), e.create("Y", "yb")
	e.sync(syncer.Options{}, a)
	e.do(x.ID, issue.SetTitle(e.me, "X2"))
	e.do(y.ID, issue.SetTitle(e.me, "Y2"))
	for _, n := range []string{onForge(e.t, f, "X").Number, onForge(e.t, f, "Y").Number} {
		f.Edit(n, func(is *forge.Issue) { is.Comments = append(is.Comments, forge.Comment{Author: bob, Body: "hi"}) })
	}
	e.sync(syncer.Options{Issue: x.ID}, a)
	if onForge(e.t, f, "X2").Title != "X2" || len(e.load(x.ID).Comments) != 1 {
		e.t.Fatal("X not synced")
	}
	if onForge(e.t, f, "Y").Title != "Y" || len(e.load(y.ID).Comments) != 0 {
		e.t.Fatal("Y synced too")
	}
	e.sync(syncer.Options{}, a)
	if onForge(e.t, f, "Y2").Title != "Y2" || len(e.load(y.ID).Comments) != 1 {
		e.t.Fatal("Y not synced by the full run")
	}
	e.stable(map[*fake.Forge]forge.Forge{f: a})
}

func theGuard(e *env) {
	fa, a := e.forge("example.com/private")
	fb, b := e.forge("example.com/public")
	fb.SetPublic(true)
	fa.Add(forge.Issue{Author: ann, Title: "secret", Comments: []forge.Comment{{Author: bob, Body: "hush"}}})
	open := e.create("open", "")
	e.sync(syncer.Options{}, a, b)
	if got := titles(fb); !reflect.DeepEqual(got, []string{"open"}) {
		e.t.Fatalf("public = %v, want only open", got)
	}
	if got := titles(fa); !reflect.DeepEqual(got, []string{"open", "secret"}) {
		e.t.Fatalf("private = %v", got)
	}
	// A comment and a label on the private mirror don't go public.
	fa.Edit(onForge(e.t, fa, "open").Number, func(is *forge.Issue) {
		is.Comments = append(is.Comments, forge.Comment{Author: bob, Body: "private words"})
		is.Labels = []string{"internal"}
	})
	e.sync(syncer.Options{}, a, b)
	if got := e.load(open.ID); len(got.Comments) != 1 || !reflect.DeepEqual(got.Labels, []string{"internal"}) {
		e.t.Fatalf("not pulled: %+v", got)
	}
	if got := onForge(e.t, fb, "open"); len(got.Comments) != 0 || len(got.Labels) != 0 {
		e.t.Fatalf("published: %+v", got)
	}
	e.stable(map[*fake.Forge]forge.Forge{fa: a, fb: b})
	// publish-private lets it go.
	e.sync(syncer.Options{PublishPrivate: true}, a, b)
	if got := titles(fb); !reflect.DeepEqual(got, []string{"open", "secret"}) {
		e.t.Fatalf("public = %v", got)
	}
	if got := onForge(e.t, fb, "open"); len(got.Comments) != 1 || text(got.Comments[0].Body) != "private words" {
		e.t.Fatalf("open = %+v", got)
	}
}

func confidentialIssues(e *env) {
	f, a := e.forge("example.com/p")
	c := f.Add(forge.Issue{Author: ann, Title: "hidden"})
	f.SetConfidential(c.Number, true)
	f.Add(forge.Issue{Author: ann, Title: "shown"})
	e.sync(syncer.Options{}, a)
	if got := gbTitles(e); !reflect.DeepEqual(got, []string{"shown"}) {
		e.t.Fatalf("git-board = %v", got)
	}
	if _, err := a.Issue(e.ctx, c.ID); !errors.Is(err, forge.ErrConfidential) {
		e.t.Fatalf("reading a confidential issue: %v, want ErrConfidential", err)
	}
	// One that turns confidential keeps its mirror, which no longer syncs.
	shown := onForge(e.t, f, "shown")
	f.SetConfidential(shown.Number, true)
	e.do(e.byTitle("shown").ID, issue.SetTitle(e.me, "renamed"))
	n := len(f.Writes())
	e.sync(syncer.Options{}, a)
	if len(f.Writes()) != n {
		e.t.Fatal("wrote to a confidential issue")
	}
	if got, _ := f.Get(shown.Number); got.Title != "shown" {
		e.t.Fatalf("title = %q", got.Title)
	}
}

func refusalWarnsOnce(e *env) {
	f, a, fi, _ := mirrored(e)
	var tries atomic.Int32
	f.Before(func(w fake.Write) error {
		if w.Op == "set-title" {
			tries.Add(1)
			return forge.ErrRefused
		}
		return nil
	})
	e.do(fi.ID, issue.SetTitle(e.me, "F2"), issue.ChangeLabels(e.me, []string{"ui"}, nil))
	e.sync(syncer.Options{}, a)
	if len(e.warnings) != 1 || !strings.Contains(e.warnings[0], "F2") {
		e.t.Fatalf("warnings = %q", e.warnings)
	}
	if got := onForge(e.t, f, "F"); !slices.Contains(got.Labels, "ui") {
		e.t.Fatal("the rest didn't go through")
	}
	if e.load(fi.ID).Title != "F2" {
		e.t.Fatal("git-board lost its value")
	}
	e.sync(syncer.Options{}, a)
	if tries.Load() != 1 || len(e.warnings) != 1 {
		e.t.Fatalf("tried %d times, %d warnings; want once", tries.Load(), len(e.warnings))
	}
	e.do(fi.ID, issue.SetTitle(e.me, "F3"))
	e.sync(syncer.Options{}, a)
	if tries.Load() != 2 || len(e.warnings) != 2 {
		e.t.Fatalf("a later change: tried %d times, %d warnings; want 2", tries.Load(), len(e.warnings))
	}
}

func assigneesAreTheForges(e *env) {
	f, a, fi, _ := mirrored(e) // ann and bob have persons, from the pull
	f.LimitAssignees(1)
	e.do(fi.ID, issue.ChangeLabels(e.me, []string{"assignee:ann", "Assignee:bob", "assignee:nobody"}, nil))
	e.sync(syncer.Options{}, a)
	got := onForge(e.t, f, "F")
	if len(got.Assignees) != 1 || got.Assignees[0].ID != ann.ID || slices.ContainsFunc(got.Labels, func(l string) bool {
		return strings.HasPrefix(strings.ToLower(l), "assignee:")
	}) {
		e.t.Fatalf("forge = %+v", got)
	}
	if len(e.warnings) != 2 { // bob not kept, nobody unknown
		e.t.Fatalf("warnings = %q", e.warnings)
	}
	f.LimitAssignees(0)
	f.Edit(got.Number, func(is *forge.Issue) { is.Assignees = nil })
	e.sync(syncer.Options{}, a)
	if got := e.load(fi.ID).Labels; slices.Contains(got, "assignee:ann") || !slices.Contains(got, "Assignee:bob") {
		e.t.Fatalf("labels = %v; want ann's removed, bob's kept", got)
	}
	e.stable(map[*fake.Forge]forge.Forge{f: a})
}
