package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/forge/fake"
)

// cycleBoard is a repository on dev with an origin (a bare repository),
// the cycle on, statuses through review and one validation, and a person
// writing, with origin's PRs on a fake forge.
func cycleBoard(t *testing.T) (dir string, b *Board, f *fake.Forge) {
	t.Helper()
	dir = t.TempDir()
	origin := filepath.Join(t.TempDir(), "origin.git")
	gitT(t, dir, "init", "-q", "-b", "dev")
	gitT(t, dir, "init", "-q", "--bare", origin)
	gitT(t, dir, "remote", "add", "origin", origin)
	toml := `[fields.status]
single = true
options = ["backlog", "ready", "in-progress", "in-review", "qa", "validated"]

[branches]
base = "dev"

[experimental]
features = ["cycle"]
`
	if err := os.WriteFile(filepath.Join(dir, ".git-board.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, dir, "add", ".")
	gitT(t, dir, "commit", "-q", "-m", "start")
	gitT(t, dir, "push", "-q", "origin", "dev")
	b, err := Open(t.Context(), dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { b.Close() })
	me, err := b.People.New(t.Context(), "dev", "dev@example.com")
	if err != nil {
		t.Fatal(err)
	}
	b.Issues.Me = me.ID
	f = fake.New("github.com/o/r")
	b.PRs = NewPullCache()
	b.PRs.Open = func(context.Context, *Board) (forge.Puller, error) { return f, nil }
	return dir, b, f
}

// An issue walks the cycle through route 50's actions: the risk, the
// start, its branch's PR opened and edited on the forge, ready for
// review behind the gate, a review, the merge, Verify's check, run and
// sign-off, which closes it. Each refusal says whose turn it is.
func TestCycleActs(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	dir, b, f := cycleBoard(t)
	is, err := b.Issues.New(ctx, issues.NewReq{Title: "Save works", Body: "Saving.\n\n## Done when\n- [ ] it saves\n", Assign: []string{"status=ready"}})
	if err != nil {
		t.Fatal(err)
	}
	id := is.ID
	act := func(a CycleAct) CycleView {
		t.Helper()
		a.Name = id
		v, err := b.Act(ctx, a)
		if err != nil {
			t.Fatalf("%s: %v", a.Action, err)
		}
		return v
	}
	refused := func(a CycleAct, code string) {
		t.Helper()
		a.Name = id
		_, err := b.Act(ctx, a)
		var e *Error
		if !errors.As(err, &e) || Code(err) != code {
			t.Fatalf("%s: %v (%s), want %s", a.Action, err, Code(err), code)
		}
	}

	v, err := b.Cycle(ctx, id, "")
	if err != nil || v.Step != 2 || !slices.Equal(v.Actions, []string{"set-risk"}) || v.ID != id || v.Version == "" {
		t.Fatalf("at Risk: %+v, %v", v, err)
	}
	refused(CycleAct{Action: "set-risk", Risk: "huge"}, "invalid")
	refused(CycleAct{Action: "approve-plan"}, "invalid")
	refused(CycleAct{Action: "nope"}, "not-found")
	refused(CycleAct{Action: "verify", Output: "ok"}, "invalid")
	v = act(CycleAct{Action: "set-risk", Risk: "low"})
	if v.Step != 3 || v.Risk != "low" || !slices.Equal(v.Actions, []string{"start"}) {
		t.Fatalf("low risk: %+v", v.Cycle)
	}
	v = act(CycleAct{Action: "start"})
	if v.Step != 4 || v.Evidence.Status != "in-progress" {
		t.Fatalf("started: %+v", v.Cycle)
	}
	refused(CycleAct{Action: "ready-for-review"}, "forbidden")

	// a branch on origin, with its test: Code, and its PR on the forge
	branch := "feat/" + id[:7] + "-save"
	gitT(t, dir, "switch", "-q", "-c", branch)
	commitFile(t, dir, "save_test.go", "package save\n")
	head := commitFile(t, dir, "save.go", "package save\n")
	gitT(t, dir, "push", "-q", "origin", branch)
	gitT(t, dir, "fetch", "-q", "origin")
	gitT(t, dir, "switch", "-q", "dev")
	refused(CycleAct{Action: "open-pull", Branch: "elsewhere"}, "invalid")
	v = act(CycleAct{Action: "open-pull", Branch: branch})
	if v.Step != 5 || v.Evidence.PR == nil || v.Evidence.PR.Number != 1 || v.Evidence.PR.Title != "Save works" || v.Evidence.PR.Body != "Refs: "+id[:7] {
		t.Fatalf("opened: %+v, PR %+v", v.Cycle, v.Evidence.PR)
	}
	f.SetPR(1, func(p *forge.PR) { p.Head, p.Mergeable, p.MergeState = head, "MERGEABLE", "CLEAN" })
	v = act(CycleAct{Action: "edit-pull", Number: 1, Title: "Save, at last", Body: "Refs: it"})
	if v.Evidence.PR.Title != "Save, at last" {
		t.Fatalf("edited: %+v", v.Evidence.PR)
	}
	reads := f.PRReads()
	if _, err := b.Cycle(ctx, id, ""); err != nil || f.PRReads() != reads {
		t.Fatalf("a read within PullsEvery asks the forge again: %d reads, then %d (%v)", reads, f.PRReads(), err)
	}
	if v = act(CycleAct{Action: "refresh"}); f.PRReads() == reads || v.Evidence.PR.Head != head || !v.Evidence.PR.CanMerge {
		t.Fatalf("refresh: %d reads, PR %+v", f.PRReads(), v.Evidence.PR)
	}

	v = act(CycleAct{Action: "ready-for-review"})
	if v.Step != 6 || v.Evidence.Status != "in-review" || !slices.Equal(v.Actions, []string{"review"}) {
		t.Fatalf("in review: %+v", v.Cycle)
	}
	refused(CycleAct{Action: "review", State: "APPROVE"}, "invalid")
	act(CycleAct{Action: "review", State: "APPROVE", Body: "Reads well: the save is one call."})
	act(CycleAct{Action: "review", Path: "save.go", Line: 1, Side: "new", Body: "A doc comment?"})
	th, err := b.Issues.Show(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	var approval, comment *issues.Event
	for i, e := range th.Events {
		switch {
		case e.Meta["review:state"] == "APPROVE":
			approval = &th.Events[i]
		case e.Meta["review:path"] == "save.go":
			comment = &th.Events[i]
		}
	}
	if approval == nil || approval.Meta["review:commit"] != head || approval.Meta["review:pr"] != "github.com/o/r#1" || comment == nil {
		t.Fatalf("the review's comments: %+v, %+v", approval, comment)
	}
	act(CycleAct{Action: "review", ReplyTo: comment.ID, Body: "Added."})
	act(CycleAct{Action: "resolve", Comment: comment.ID, Resolved: true})
	if meta, _ := b.Issues.CommentMetaOf(ctx, id, "x"); meta != nil {
		t.Fatalf("no such comment: %v", meta)
	}

	refused(CycleAct{Action: "merge", Number: 1}, "invalid")
	v = act(CycleAct{Action: "merge", Number: 1, SHA: head})
	if v.Evidence.PR.State != "MERGED" || !slices.Equal(v.Actions, []string{"to-validation"}) {
		t.Fatalf("merged: %+v, %+v", v.Cycle, v.Evidence.PR)
	}
	v = act(CycleAct{Action: "to-validation"})
	if v.Step != 7 || !slices.Equal(v.Actions, []string{"verify", "check", "signoff"}) {
		t.Fatalf("at Verify: %+v", v.Cycle)
	}
	zero := 0
	act(CycleAct{Action: "check", Index: &zero})
	if th, _ = b.Issues.Show(ctx, id); !strings.Contains(th.Events[0].Body, "- [x] it saves") {
		t.Fatalf("the criterion isn't ticked: %q", th.Events[0].Body)
	}
	refused(CycleAct{Action: "signoff"}, "invalid")
	act(CycleAct{Action: "verify", Output: "ok\n```\n"})
	v = act(CycleAct{Action: "signoff"})
	if !v.Done || v.Evidence.Status != "validated" || !v.Evidence.Closed {
		t.Fatalf("signed off: %+v", v.Cycle)
	}
}

// The Intent step's actions: accepted without criteria, and the spec
// role's verdict, on the description's version, moving it to ready.
func TestCycleIntent(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	_, b, _ := cycleBoard(t)
	bare, err := b.Issues.New(ctx, issues.NewReq{Title: "Old one", Assign: []string{"status=backlog"}})
	if err != nil {
		t.Fatal(err)
	}
	v, err := b.Act(ctx, CycleAct{Name: bare.ID, Action: "no-criteria"})
	if err != nil || !v.Evidence.NoCriteria {
		t.Fatalf("no-criteria: %+v, %v", v.Cycle, err)
	}
	v, err = b.Act(ctx, CycleAct{Name: bare.ID, Action: "move-ready"})
	if err != nil || v.Evidence.Status != "ready" {
		t.Fatalf("move-ready: %+v, %v", v.Cycle, err)
	}

	// the description's author doesn't approve it
	is, err := b.Issues.New(ctx, issues.NewReq{Title: "Spec", Body: "## Done when\n- one\n", Assign: []string{"status=backlog"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Act(ctx, CycleAct{Name: is.ID, Action: "spec-verdict", Verdict: "approve", On: "body@v1"}); Code(err) != "forbidden" {
		t.Fatalf("the author's own verdict: %v", err)
	}
	if _, err := b.Act(ctx, CycleAct{Name: is.ID, Action: "spec-verdict", Verdict: "approve", On: "body@v2"}); Code(err) != "changed" {
		t.Fatalf("a verdict on another version: %v", err)
	}
	if v, err = b.Act(ctx, CycleAct{Name: is.ID, Action: "spec-verdict", Verdict: "request-changes", On: "body@v1", Body: "Say which save."}); err == nil || Code(err) != "forbidden" {
		t.Fatalf("request-changes by the author: %+v, %v", v.Cycle, err)
	}
}

// Off, every action refuses as experimental.
func TestCycleActOff(t *testing.T) {
	t.Parallel()
	_, b := co2Board(t)
	if _, err := b.Act(t.Context(), CycleAct{Name: "x", Action: "start"}); !errors.Is(err, ErrCycleOff) {
		t.Fatalf("off: %v", err)
	}
}
