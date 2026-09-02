package ghbridge_test

import (
	"testing"

	"github.com/guilbep/git-board/internal/ghbridge"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// TestApplyPlanIsIdempotent exercises the real git-bug backend (via
// gitbugiotest, no mocks): running BuildPlan+ApplyPlan twice against the
// same repo must not double up or conflict labels the second time, since
// git-bug's ChangeLabels errors on an all-no-op call.
func TestApplyPlanIsIdempotent(t *testing.T) {
	items := mustParseFixture(t)
	cfg := mustLoadConfig(t)

	b := gitbugiotest.New(t)
	id := gitbugiotest.NewImportedIssue(t, b, "cannot change equipment_class",
		"https://github.com/EPFL-ENAC/co2-calculator/issues/2605")

	run := func() []string {
		issues, err := b.ListIssues()
		if err != nil {
			t.Fatalf("ListIssues: %v", err)
		}
		plan := ghbridge.BuildPlan(items, issues, cfg)
		if err := ghbridge.ApplyPlan(b, plan); err != nil {
			t.Fatalf("ApplyPlan: %v", err)
		}
		labels, err := b.GetLabels(id)
		if err != nil {
			t.Fatalf("GetLabels: %v", err)
		}
		return labels
	}

	first := run()
	if len(first) == 0 {
		t.Fatal("first ApplyPlan wrote no labels")
	}

	// Must not error (ApplyPlan would be resubmitting already-set labels,
	// which git-bug's ChangeLabels rejects as an all-no-op call unless
	// ApplyPlan skips no-op changes) and must produce the same label set.
	second := run()
	if len(first) != len(second) {
		t.Fatalf("label count changed across idempotent re-run: %v vs %v", first, second)
	}
	want := map[string]bool{}
	for _, l := range first {
		want[l] = true
	}
	for _, l := range second {
		if !want[l] {
			t.Errorf("unexpected label %q after re-run", l)
		}
	}
}

// TestApplyPlanReconcilesAssigneesOnRerun covers the multi-cardinality
// path specifically: an assignee removed on GitHub between two pulls must
// be removed locally too, not just accumulate.
func TestApplyPlanReconcilesAssigneesOnRerun(t *testing.T) {
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewImportedIssue(t, b, "x", "https://github.com/o/r/issues/1")

	pull := func(assignees []string) []string {
		item := ghbridge.Item{
			Assignees: assignees,
			Content:   ghbridge.Content{Number: 1, Type: "Issue", URL: "https://github.com/o/r/issues/1"},
		}
		issues, err := b.ListIssues()
		if err != nil {
			t.Fatalf("ListIssues: %v", err)
		}
		plan := ghbridge.BuildPlan([]ghbridge.Item{item}, issues, cfg)
		if err := ghbridge.ApplyPlan(b, plan); err != nil {
			t.Fatalf("ApplyPlan: %v", err)
		}
		labels, err := b.GetLabels(id)
		if err != nil {
			t.Fatalf("GetLabels: %v", err)
		}
		return labels
	}

	labels := pull([]string{"alice", "bob"})
	if !contains(labels, "assignee:alice") || !contains(labels, "assignee:bob") {
		t.Fatalf("labels after first pull = %v", labels)
	}

	labels = pull([]string{"alice"})
	if !contains(labels, "assignee:alice") {
		t.Errorf("assignee:alice should remain, got %v", labels)
	}
	if contains(labels, "assignee:bob") {
		t.Errorf("assignee:bob should have been removed, got %v", labels)
	}
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
