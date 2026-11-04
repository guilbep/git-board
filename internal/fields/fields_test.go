package fields_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/store/storetest"
)

var today = time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)

func mustLoadConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("../config/testdata/git-board.toml")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

func TestApplySingleCardinalitySwapsLabel(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	if _, err := b.ApplyLabelChange(id, []string{"status:backlog", "prio:p1"}, nil); err != nil {
		t.Fatalf("seed labels: %v", err)
	}

	label, err := fields.Apply(b, cfg, id, "status", "ready", "", today)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if label != "status:ready" {
		t.Errorf("label = %q, want status:ready", label)
	}

	got, err := b.GetLabels(id)
	if err != nil {
		t.Fatalf("GetLabels: %v", err)
	}
	want := map[string]bool{"status:ready": true, "prio:p1": true}
	if len(got) != 2 {
		t.Fatalf("labels = %v", got)
	}
	for _, l := range got {
		if !want[l] {
			t.Errorf("unexpected label %q", l)
		}
	}
}

// The real regression case: git-bug's ChangeLabels errors if you re-add a
// label that's already set, so Apply must short-circuit instead of blindly
// calling ChangeLabels with the target already in the "added" list.
func TestApplySingleCardinalityNoOpWhenAlreadySet(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	if _, err := b.ApplyLabelChange(id, []string{"status:ready"}, nil); err != nil {
		t.Fatalf("seed labels: %v", err)
	}

	label, err := fields.Apply(b, cfg, id, "status", "ready", "", today)
	if err != nil {
		t.Fatalf("Apply on already-set value must not error, got: %v", err)
	}
	if label != "status:ready" {
		t.Errorf("label = %q, want status:ready", label)
	}

	got, err := b.GetLabels(id)
	if err != nil {
		t.Fatalf("GetLabels: %v", err)
	}
	if len(got) != 1 || got[0] != "status:ready" {
		t.Errorf("labels = %v, want unchanged [status:ready]", got)
	}
}

func TestApplyMultiCardinalityAddsWithoutRemoving(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	if _, err := b.ApplyLabelChange(id, []string{"assignee:alice"}, nil); err != nil {
		t.Fatalf("seed labels: %v", err)
	}

	label, err := fields.Apply(b, cfg, id, "assignee", "+bob", "", today)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if label != "assignee:bob" {
		t.Errorf("label = %q, want assignee:bob", label)
	}

	got, err := b.GetLabels(id)
	if err != nil {
		t.Fatalf("GetLabels: %v", err)
	}
	want := map[string]bool{"assignee:alice": true, "assignee:bob": true}
	if len(got) != 2 {
		t.Fatalf("labels = %v", got)
	}
	for _, l := range got {
		if !want[l] {
			t.Errorf("unexpected label %q", l)
		}
	}
}

func TestApplyMultiCardinalityRemove(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	if _, err := b.ApplyLabelChange(id, []string{"assignee:alice", "assignee:bob"}, nil); err != nil {
		t.Fatalf("seed labels: %v", err)
	}

	label, err := fields.Apply(b, cfg, id, "assignee", "-bob", "", today)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if label != "assignee:bob" {
		t.Errorf("label = %q, want assignee:bob", label)
	}

	got, err := b.GetLabels(id)
	if err != nil {
		t.Fatalf("GetLabels: %v", err)
	}
	if len(got) != 1 || got[0] != "assignee:alice" {
		t.Errorf("labels = %v, want [assignee:alice]", got)
	}
}

func TestApplyResolvesSprintAlias(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")

	label, err := fields.Apply(b, cfg, id, "sprint", "@next", "", today)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if label != "sprint:post-delivery" {
		t.Errorf("label = %q, want sprint:post-delivery", label)
	}
}

func TestApplyResolvesMe(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")

	label, err := fields.Apply(b, cfg, id, "assignee", "@me", "guilbep", today)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if label != "assignee:guilbep" {
		t.Errorf("label = %q, want assignee:guilbep", label)
	}
}

// @me with no --me and no one chosen to write as means nobody.
func TestApplyMeWithoutMeConfiguredErrors(t *testing.T) {
	cfg := mustLoadConfig(t)
	dir := storetest.Repo(t, false)
	storetest.Init(t, dir)
	b := storetest.Open(t, dir)
	id := storetest.NewIssue(t, b, "x")
	storetest.Git(t, dir, "config", "--unset", "board.me")

	if _, err := fields.Apply(b, cfg, id, "assignee", "@me", "", today); err == nil {
		t.Error("expected error when @me used without a configured user")
	}
}

// Without a config, assignee still holds several people, and a field that
// already holds several values keeps them.
func TestApplyUnconfiguredMultiValuedFields(t *testing.T) {
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	if _, err := b.ApplyLabelChange(id, []string{"assignee:alice", "team:web", "team:api", "sprint:s1"}, nil); err != nil {
		t.Fatalf("seed labels: %v", err)
	}
	cfg := &config.Config{}
	for _, set := range [][2]string{{"assignee", "bob"}, {"team", "data"}, {"sprint", "s2"}} {
		if _, err := fields.Apply(b, cfg, id, set[0], set[1], "", today); err != nil {
			t.Fatalf("Apply %v: %v", set, err)
		}
	}
	got, err := b.GetLabels(id)
	if err != nil {
		t.Fatal(err)
	}
	slices.Sort(got)
	want := []string{"assignee:alice", "assignee:bob", "sprint:s2", "team:api", "team:data", "team:web"}
	if !slices.Equal(got, want) {
		t.Errorf("labels = %v, want %v", got, want)
	}
}

// Without --me, @me is the person you write as: their name while they
// have no forge account (issue #87), their login once they have one.
func TestApplyMeFallsBackToTheIdentitysName(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	me, _, err := b.Me()
	if err != nil {
		t.Fatal(err)
	}
	label, err := fields.Apply(b, cfg, id, "assignee", "@me", "", today)
	if err != nil || label != "assignee:"+me.Name {
		t.Errorf("Apply @me = %q, %v; want assignee:%s", label, err, me.Name)
	}
	for _, f := range []string{"assignee:@me", "involves:@me"} {
		if issues, err := board.LoadMatching(b, cfg, f, ""); err != nil || len(issues) != 1 {
			t.Errorf("LoadMatching %s = %d issues, %v; want the one", f, len(issues), err)
		}
	}
}

// Without --me, @me is the forge login of the person you write as.
func TestApplyMeFallsBackToTheIdentitysLogin(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	storetest.SetMyLogin(t, b, "guilbep")
	label, err := fields.Apply(b, cfg, id, "assignee", "@me", "", today)
	if err != nil || label != "assignee:guilbep" {
		t.Errorf("Apply @me = %q, %v; want assignee:guilbep", label, err)
	}
	issues, err := board.LoadMatching(b, cfg, "assignee:@me", "")
	if err != nil || len(issues) != 1 {
		t.Errorf("LoadMatching assignee:@me = %d issues, %v; want the one", len(issues), err)
	}
}

// The review's probes: each wrote a label nothing reads, and exited 0.
func TestParseRefusesMalformedAssignments(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"=x", "status", "status=-", "assignee=+", "status= ready", "status=a\nb", "a b=c", "prio:p1=x", "status=+-x"} {
		if a, err := fields.Parse(s); err == nil {
			t.Errorf("Parse(%q) = %+v, want an error", s, a)
		}
	}
	for s, want := range map[string]fields.Assignment{
		"status=ready":   {Field: "status", Value: "ready"},
		"status=":        {Field: "status"},
		"assignee=+@me":  {Field: "assignee", Op: '+', Value: "@me"},
		"label=-urgent":  {Field: "label", Op: '-', Value: "urgent"},
		"status=in prog": {Field: "status", Value: "in prog"},
	} {
		if got, err := fields.Parse(s); err != nil || got != want {
			t.Errorf("Parse(%q) = %+v, %v; want %+v", s, got, err, want)
		}
	}
}

// assignee=+@me adds you, -@me removes you: the sign is split off before
// @me resolves (review: fields.go:108).
func TestApplySignedMeResolves(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	storetest.SetMyLogin(t, b, "guilbep")
	if label, err := fields.Apply(b, cfg, id, "assignee", "+@me", "", today); err != nil || label != "assignee:guilbep" {
		t.Fatalf("Apply +@me = %q, %v; want assignee:guilbep", label, err)
	}
	if _, err := fields.Apply(b, cfg, id, "assignee", "-@me", "", today); err != nil {
		t.Fatalf("Apply -@me: %v", err)
	}
	if got, _ := b.GetLabels(id); len(got) != 0 {
		t.Errorf("labels = %v, want none after -@me", got)
	}
}

// status= clears the field instead of writing "status:"; -value drops a
// single field's value; +value on a single field is refused.
func TestApplyClearsAndRemovesSingleFields(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	if _, err := b.ApplyLabelChange(id, []string{"status:ready", "prio:p1", "assignee:a", "assignee:b"}, nil); err != nil {
		t.Fatal(err)
	}
	if label, err := fields.Apply(b, cfg, id, "status", "", "", today); err != nil || label != "" {
		t.Fatalf("Apply status= = %q, %v", label, err)
	}
	if _, err := fields.Apply(b, cfg, id, "prio", "-p1", "", today); err != nil {
		t.Fatalf("Apply prio=-p1: %v", err)
	}
	if _, err := fields.Apply(b, cfg, id, "assignee", "", "", today); err != nil {
		t.Fatalf("Apply assignee=: %v", err)
	}
	if _, err := fields.Apply(b, cfg, id, "prio", "+p2", "", today); err == nil {
		t.Error("Apply prio=+p2 on a single field: want an error")
	}
	if _, err := fields.Apply(b, cfg, id, "", "x", "", today); err == nil {
		t.Error("Apply with no field name: want an error")
	}
	if got, _ := b.GetLabels(id); len(got) != 0 {
		t.Errorf("labels = %v, want none", got)
	}
}

// label=+name and label=-name add and remove a plain label.
func TestApplyPlainLabels(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	if _, err := fields.Apply(b, cfg, id, "label", "+urgent", "", today); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.GetLabels(id); !slices.Equal(got, []string{"urgent"}) {
		t.Fatalf("labels = %v, want [urgent]", got)
	}
	if _, err := fields.Apply(b, cfg, id, "label", "-urgent", "", today); err != nil {
		t.Fatal(err)
	}
	if got, _ := b.GetLabels(id); len(got) != 0 {
		t.Errorf("labels = %v, want none", got)
	}
	if _, err := fields.Apply(b, cfg, id, "label", "+prio:p1", "", today); err == nil {
		t.Error("label=+prio:p1: want an error naming prio=p1")
	}
}

func TestCheckHoldsToTheConfig(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	inUse := []string{"team:web"}
	for _, s := range []string{"prio=p9", "milestone=v1", "est=lots", "sprint=nope", "status=Ready"} {
		a, _ := fields.Parse(s)
		if err := fields.Check(cfg, a, inUse); !errors.Is(err, fields.ErrUnchecked) {
			t.Errorf("Check(%s) = %v, want ErrUnchecked", s, err)
		}
	}
	for _, s := range []string{"prio=p0", "prio=-p9", "prio=", "est=3.5", "sprint=delivery", "assignee=+bob", "team=data", "due=2026-10-01", "label=+urgent"} {
		a, _ := fields.Parse(s)
		if err := fields.Check(cfg, a, inUse); err != nil {
			t.Errorf("Check(%s) = %v, want nil", s, err)
		}
	}
	a, _ := fields.Parse("due=tomorrow")
	if err := fields.Check(cfg, a, inUse); err == nil {
		t.Error("Check(due=tomorrow): want an error, due is a date")
	}
}
