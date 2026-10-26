package tui

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
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

// Every "ns:value" label is a field, configured or not; only labels without
// a namespace are "other".
func TestParseLabelsSeparatesKnownFromOtherLabels(t *testing.T) {
	t.Parallel()
	labels := []string{
		"status:ready",
		"prio:p1",
		"assignee:alice",
		"assignee:bob",
		"random-flag",
		"unconfigured:value",
	}

	known, other := ParseLabels(labels)

	want := map[string][]string{
		"assignee":     {"alice", "bob"},
		"prio":         {"p1"},
		"status":       {"ready"},
		"unconfigured": {"value"},
	}
	if len(known) != len(want) {
		t.Fatalf("known = %+v, want %d fields", known, len(want))
	}
	for _, f := range known {
		gotVals, ok := want[f.Name]
		if !ok {
			t.Errorf("unexpected known field %q", f.Name)
			continue
		}
		if len(f.Values) != len(gotVals) {
			t.Errorf("field %q values = %v, want %v", f.Name, f.Values, gotVals)
			continue
		}
		for i, v := range gotVals {
			if f.Values[i] != v {
				t.Errorf("field %q values = %v, want %v", f.Name, f.Values, gotVals)
			}
		}
	}

	wantOther := map[string]bool{"random-flag": true}
	if len(other) != len(wantOther) {
		t.Fatalf("other = %v, want %v", other, wantOther)
	}
	for _, l := range other {
		if !wantOther[l] {
			t.Errorf("unexpected other label %q", l)
		}
	}
}

func TestParseLabelsNoLabelsYieldsNothingKnown(t *testing.T) {
	t.Parallel()
	known, other := ParseLabels(nil)
	if len(known) != 0 || len(other) != 0 {
		t.Errorf("known = %v, other = %v, want both empty", known, other)
	}
}

func TestRenderDetailShowsKnownFieldsAndOtherLabels(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	iss := store.Issue{
		ID: "abcdef1234", Title: "widget", Status: "open",
		Labels: []string{"status:ready", "assignee:alice", "assignee:bob", "random-flag", "unconfigured:value"},
	}
	view := renderDetail(iss, cfg, "")
	for _, want := range []string{"Status: ready", "Assignee: alice, bob", "Unconfigured: value", "Other labels:", "random-flag"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}

	// The dogfood case that motivated this: a real issue with no
	// board-relevant labels yet must not show a stray "Other labels:"
	// heading over nothing.
	bare := renderDetail(store.Issue{ID: "x", Title: "y", Status: "open"}, cfg, "")
	if strings.Contains(bare, "Other labels:") {
		t.Errorf("bare issue view has an empty Other labels section:\n%s", bare)
	}
	if !strings.Contains(bare, "(no recognized fields)") {
		t.Errorf("bare issue view = %q, want the no-fields placeholder", bare)
	}
}

func TestFieldNamesAreConfiguredPlusFoundSorted(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	issues := []store.Issue{{Labels: []string{"sprint:s1", "status:ready", "plain"}}, {Labels: []string{"mod:travel"}}}
	got := FieldNames(cfg, issues)
	want := []string{"assignee", "cat", "est", "mod", "prio", "sprint", "status"}
	if !slices.Equal(got, want) {
		t.Errorf("FieldNames = %v, want %v", got, want)
	}
}

func TestValuePickerItemsUsesConfiguredOptions(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	items := valuePickerItems(cfg, "prio")
	if len(items) != 4 {
		t.Fatalf("got %d items, want 4", len(items))
	}
	if items[0].(valueItem).value != "p0" {
		t.Errorf("items[0] = %v, want p0", items[0])
	}
}

func TestValuePickerItemsEmptyForFieldWithoutOptions(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	items := valuePickerItems(cfg, "assignee")
	if len(items) != 0 {
		t.Fatalf("got %d items, want 0 (assignee has no fixed options)", len(items))
	}
}

func TestCatFieldFoundWhenConfigured(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	f, ok := catField(cfg)
	if !ok {
		t.Fatal("catField() ok = false, want true (fixture configures fields.cat)")
	}
	if len(f.Options) == 0 {
		t.Error("catField() options empty")
	}
}

func TestCatFieldMissingWhenNotConfigured(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{Fields: map[string]config.Field{"status": {Single: true}}}
	if _, ok := catField(cfg); ok {
		t.Error("catField() ok = true, want false when cat isn't configured")
	}
}

func TestFilterByFieldNarrowsToMatchingIssues(t *testing.T) {
	t.Parallel()
	b := storetest.New(t)
	bugID := storetest.NewIssue(t, b, "a bug")
	featureID := storetest.NewIssue(t, b, "a feature")
	storetest.NewIssue(t, b, "uncategorized")
	if _, err := b.ApplyLabelChange(bugID, []string{"cat:bug"}, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := b.ApplyLabelChange(featureID, []string{"cat:feature"}, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}

	issues, err := b.ListIssues()
	if err != nil {
		t.Fatalf("ListIssues: %v", err)
	}

	filtered := FilterByField(issues, catFieldName, "bug")
	if len(filtered) != 1 || filtered[0].ID != bugID {
		t.Errorf("filtered = %+v, want only %q", filtered, bugID)
	}
}

func TestApplyFieldSingleCardinalityWritesThroughAndRefreshesLabels(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	if _, err := b.ApplyLabelChange(id, []string{"status:backlog", "prio:p1"}, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}

	labels, applied, err := ApplyField(b, cfg, id, "status", "ready", "", today)
	if err != nil {
		t.Fatalf("ApplyField: %v", err)
	}
	if applied != "status:ready" {
		t.Errorf("applied = %q, want status:ready", applied)
	}

	hasReady, hasBacklog := false, false
	for _, l := range labels {
		switch l {
		case "status:ready":
			hasReady = true
		case "status:backlog":
			hasBacklog = true
		}
	}
	if !hasReady {
		t.Errorf("labels = %v, want status:ready present", labels)
	}
	if hasBacklog {
		t.Errorf("labels = %v, want status:backlog gone (single-cardinality swap)", labels)
	}
}

func TestApplyFieldMultiCardinalityAddsWithoutRemoving(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")
	if _, err := b.ApplyLabelChange(id, []string{"assignee:alice"}, nil); err != nil {
		t.Fatalf("seed: %v", err)
	}

	labels, applied, err := ApplyField(b, cfg, id, "assignee", "+bob", "", today)
	if err != nil {
		t.Fatalf("ApplyField: %v", err)
	}
	if applied != "assignee:bob" {
		t.Errorf("applied = %q, want assignee:bob", applied)
	}

	want := map[string]bool{"assignee:alice": true, "assignee:bob": true}
	if len(labels) != len(want) {
		t.Fatalf("labels = %v, want %v", labels, want)
	}
	for _, l := range labels {
		if !want[l] {
			t.Errorf("unexpected label %q", l)
		}
	}
}

func TestApplyFieldErrorsWithoutWritingOnUnknownSprintAlias(t *testing.T) {
	t.Parallel()
	cfg := mustLoadConfig(t)
	b := storetest.New(t)
	id := storetest.NewIssue(t, b, "x")

	if _, _, err := ApplyField(b, cfg, id, "assignee", "@me", "", today); err == nil {
		t.Fatal("expected error when @me used without a configured user")
	}

	labels, err := b.GetLabels(id)
	if err != nil {
		t.Fatalf("GetLabels: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("labels = %v, want no write on error", labels)
	}
}
