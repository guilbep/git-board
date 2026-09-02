package tui

import (
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
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

func TestParseLabelsSeparatesKnownFromOtherLabels(t *testing.T) {
	cfg := mustLoadConfig(t)
	labels := []string{
		"status:ready",
		"prio:p1",
		"assignee:alice",
		"assignee:bob",
		"random-flag",
		"unconfigured:value",
	}

	known, other := ParseLabels(labels, cfg.Fields)

	want := map[string][]string{
		"assignee": {"alice", "bob"},
		"prio":     {"p1"},
		"status":   {"ready"},
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

	wantOther := map[string]bool{"random-flag": true, "unconfigured:value": true}
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
	cfg := mustLoadConfig(t)
	known, other := ParseLabels(nil, cfg.Fields)
	if len(known) != 0 || len(other) != 0 {
		t.Errorf("known = %v, other = %v, want both empty", known, other)
	}
}

func TestRenderDetailShowsKnownFieldsAndOtherLabels(t *testing.T) {
	cfg := mustLoadConfig(t)
	iss := gitbugio.Issue{
		ID: "abcdef1234", Title: "widget", Status: "open",
		Labels: []string{"status:ready", "assignee:alice", "assignee:bob", "random-flag", "unconfigured:value"},
	}
	view := renderDetail(iss, cfg, "")
	for _, want := range []string{"Status: ready", "Assignee: alice, bob", "Other labels:", "random-flag", "unconfigured:value"} {
		if !strings.Contains(view, want) {
			t.Errorf("view missing %q:\n%s", want, view)
		}
	}

	// The dogfood case that motivated this: a real issue with no
	// board-relevant labels yet must not show a stray "Other labels:"
	// heading over nothing.
	bare := renderDetail(gitbugio.Issue{ID: "x", Title: "y", Status: "open"}, cfg, "")
	if strings.Contains(bare, "Other labels:") {
		t.Errorf("bare issue view has an empty Other labels section:\n%s", bare)
	}
	if !strings.Contains(bare, "(no recognized fields)") {
		t.Errorf("bare issue view = %q, want the no-fields placeholder", bare)
	}
}

func TestFieldNamesSorted(t *testing.T) {
	cfg := mustLoadConfig(t)
	names := FieldNames(cfg)
	for i := 1; i < len(names); i++ {
		if names[i-1] >= names[i] {
			t.Fatalf("FieldNames() not sorted: %v", names)
		}
	}
	want := map[string]bool{"status": true, "prio": true, "est": true, "assignee": true, "cat": true}
	if len(names) != len(want) {
		t.Fatalf("names = %v, want %d entries", names, len(want))
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected field name %q", n)
		}
	}
}

func TestValuePickerItemsUsesConfiguredOptions(t *testing.T) {
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
	cfg := mustLoadConfig(t)
	items := valuePickerItems(cfg, "assignee")
	if len(items) != 0 {
		t.Fatalf("got %d items, want 0 (assignee has no fixed options)", len(items))
	}
}

func TestCatFieldFoundWhenConfigured(t *testing.T) {
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
	cfg := &config.Config{Fields: map[string]config.Field{"status": {Single: true}}}
	if _, ok := catField(cfg); ok {
		t.Error("catField() ok = true, want false when cat isn't configured")
	}
}

func TestFilterByFieldNarrowsToMatchingIssues(t *testing.T) {
	b := gitbugiotest.New(t)
	bugID := gitbugiotest.NewIssue(t, b, "a bug")
	featureID := gitbugiotest.NewIssue(t, b, "a feature")
	gitbugiotest.NewIssue(t, b, "uncategorized")
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
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "x")
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
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "x")
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
	cfg := mustLoadConfig(t)
	b := gitbugiotest.New(t)
	id := gitbugiotest.NewIssue(t, b, "x")

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
