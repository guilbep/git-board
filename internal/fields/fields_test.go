package fields_test

import (
	"slices"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
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
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "x")
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
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "x")
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
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "x")
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
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "x")
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
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "x")

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
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "x")

	label, err := fields.Apply(b, cfg, id, "assignee", "@me", "guilbep", today)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if label != "assignee:guilbep" {
		t.Errorf("label = %q, want assignee:guilbep", label)
	}
}

func TestApplyMeWithoutMeConfiguredErrors(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "x")

	if _, err := fields.Apply(b, cfg, id, "assignee", "@me", "", today); err == nil {
		t.Error("expected error when @me used without a configured user")
	}
}

// Without a config, assignee still holds several people, and a field that
// already holds several values keeps them.
func TestApplyUnconfiguredMultiValuedFields(t *testing.T) {
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "x")
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
