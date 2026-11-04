package fields_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/datastore/memstore"
	"github.com/guilbep/git-board/core/fields"
	"github.com/guilbep/git-board/core/issue"
)

var today = time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC)

// co2 is internal/config/testdata/git-board.toml's schema.
var co2 = &fields.Schema{
	Fields: map[string]fields.Field{
		"status":   {Single: true, Options: []string{"backlog", "comite", "ready", "in-progress", "in-review", "ready-for-validation", "qa", "tests-metier", "validated"}},
		"prio":     {Single: true, Options: []string{"p0", "p1", "p2", "p3"}},
		"est":      {Single: true, Options: []string{"0.5", "1", "2", "3", "4", "6", "8", "16"}, Numeric: true},
		"assignee": {Single: false},
		"cat":      {Single: true, Options: []string{"bug", "feature", "chore"}},
	},
	Order: []string{"status", "prio", "est", "assignee", "cat"},
	Sprints: fields.Sprints{LengthDays: 14, StartWeekday: "monday", List: []fields.Sprint{
		{Slug: "delivery", Title: "Delivery", Start: time.Date(2026, 8, 24, 0, 0, 0, 0, time.UTC)},
		{Slug: "post-delivery", Title: "Post-delivery", Start: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC)},
	}},
}

// issues is a Backend on the core's own issues, kept in memory.
type issues struct{ ds *memstore.Store }

func (b issues) GetLabels(id string) ([]string, error) {
	is, err := issue.Load(b.ds, id)
	if err != nil {
		return nil, err
	}
	return is.Labels, nil
}

// ApplyLabelChange writes what changes, as internal/store's does.
func (b issues) ApplyLabelChange(id string, added, removed []string) ([]string, error) {
	is, err := issue.Load(b.ds, id)
	if err != nil {
		return nil, err
	}
	added = slices.DeleteFunc(slices.Clone(added), func(l string) bool { return slices.Contains(is.Labels, l) })
	removed = slices.DeleteFunc(slices.Clone(removed), func(l string) bool { return !slices.Contains(is.Labels, l) })
	if len(added) == 0 && len(removed) == 0 {
		return is.Labels, nil
	}
	if is, err = issue.Do(context.Background(), b.ds, id, issue.ChangeLabels(author, added, removed)); err != nil {
		return nil, err
	}
	return is.Labels, nil
}

const author = "0123456789abcdef0123456789abcdef01234567"

// newIssue is an issue titled x on a board of its own.
func newIssue(t *testing.T) (issues, string) {
	t.Helper()
	b := issues{memstore.New("")}
	is, err := issue.Create(context.Background(), b.ds, issue.NewCreate(author, "x", ""))
	if err != nil {
		t.Fatal(err)
	}
	return b, is.ID
}

func TestApplySingleCardinalitySwapsLabel(t *testing.T) {
	cfg := co2
	b, id := newIssue(t)
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
	cfg := co2
	b, id := newIssue(t)
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
	cfg := co2
	b, id := newIssue(t)
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
	cfg := co2
	b, id := newIssue(t)
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
	cfg := co2
	b, id := newIssue(t)

	label, err := fields.Apply(b, cfg, id, "sprint", "@next", "", today)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if label != "sprint:post-delivery" {
		t.Errorf("label = %q, want sprint:post-delivery", label)
	}
}

func TestApplyResolvesMe(t *testing.T) {
	cfg := co2
	b, id := newIssue(t)

	label, err := fields.Apply(b, cfg, id, "assignee", "@me", "guilbep", today)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if label != "assignee:guilbep" {
		t.Errorf("label = %q, want assignee:guilbep", label)
	}
}

// Without a config, assignee still holds several people, and a field that
// already holds several values keeps them.
func TestApplyUnconfiguredMultiValuedFields(t *testing.T) {
	b, id := newIssue(t)
	if _, err := b.ApplyLabelChange(id, []string{"assignee:alice", "team:web", "team:api", "sprint:s1"}, nil); err != nil {
		t.Fatalf("seed labels: %v", err)
	}
	cfg := &fields.Schema{}
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

// status= clears the field instead of writing "status:"; -value drops a
// single field's value; +value on a single field is refused.
func TestApplyClearsAndRemovesSingleFields(t *testing.T) {
	t.Parallel()
	cfg := co2
	b, id := newIssue(t)
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
	cfg := co2
	b, id := newIssue(t)
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
	cfg := co2
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
