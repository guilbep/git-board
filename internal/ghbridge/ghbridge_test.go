package ghbridge_test

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghbridge"
	"github.com/guilbep/git-board/internal/gitbugio"
)

const fixture = "testdata/project-items.json"

func mustParseFixture(t *testing.T) []ghbridge.Item {
	t.Helper()
	data, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	items, err := ghbridge.ParseItems(data)
	if err != nil {
		t.Fatalf("ParseItems: %v", err)
	}
	return items
}

func mustLoadConfig(t *testing.T) *config.Config {
	t.Helper()
	cfg, err := config.Load("testdata/git-board.toml")
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}
	return cfg
}

// bug is a small helper to build a gitbugio.Issue fixture keyed by the
// GitHub issue number, mirroring the fixture's real content.url shape.
func bug(id string, number int, labels ...string) gitbugio.Issue {
	return gitbugio.Issue{
		ID:        id,
		GithubURL: "https://github.com/EPFL-ENAC/co2-calculator/issues/" + strconv.Itoa(number),
		Labels:    labels,
	}
}

func TestParseItemsMatchesRealShape(t *testing.T) {
	items := mustParseFixture(t)
	if len(items) != 8 {
		t.Fatalf("len(items) = %d, want 8", len(items))
	}
	first := items[0]
	if first.Content.Number != 2605 || first.Content.Type != "Issue" {
		t.Errorf("first item content = %+v", first.Content)
	}
	if first.Status != "QA Validation" || first.Priority != "P0" || first.Module != "BackOffice" {
		t.Errorf("first item fields = status=%q prio=%q module=%q", first.Status, first.Priority, first.Module)
	}
	if first.EstHours == nil || *first.EstHours != 2 {
		t.Errorf("first item EstHours = %v", first.EstHours)
	}
	if first.Sprint == nil || first.Sprint.Title != "Delivery" {
		t.Errorf("first item Sprint = %+v", first.Sprint)
	}
	if len(first.Assignees) != 2 {
		t.Errorf("first item Assignees = %v", first.Assignees)
	}
}

func changeFor(t *testing.T, plan ghbridge.Plan, number int) ghbridge.IssueChange {
	t.Helper()
	for _, c := range plan.Changes {
		if c.Number == number {
			return c
		}
	}
	t.Fatalf("no change for issue #%d in plan (matched: %d)", number, len(plan.Changes))
	return ghbridge.IssueChange{}
}

func TestBuildPlanMapsFields(t *testing.T) {
	items := mustParseFixture(t)
	cfg := mustLoadConfig(t)
	bugs := []gitbugio.Issue{bug("bug2605", 2605)}

	plan := ghbridge.BuildPlan(items, bugs, cfg)
	change := changeFor(t, plan, 2605)

	wantAdded := map[string]bool{
		"prio:p0":            true,
		"est:2":              true,
		"assignee:BenBotros": true,
		"assignee:guilbep":   true,
	}
	got := map[string]bool{}
	for _, l := range change.Added {
		got[l] = true
	}
	for l := range wantAdded {
		if !got[l] {
			t.Errorf("missing expected label %q in Added = %v", l, change.Added)
		}
	}
	// status "QA Validation" slugifies to "qa-validation", not a configured
	// [fields.status] option in the sample config, so it should warn+skip
	// rather than silently write an unconfigured label.
	if len(change.Warnings) != 1 {
		t.Fatalf("Warnings = %v, want exactly 1 (status)", change.Warnings)
	}
	// module isn't configured with Options in the sample config, so it
	// passes through unvalidated.
	if !got["mod:backoffice"] {
		t.Errorf("mod:backoffice missing from Added = %v", change.Added)
	}
	// Category FEAT is the kind feat, as [branches] kinds spells it
	if !got["feat"] || got["FEAT"] {
		t.Errorf("Added = %v, want feat and no FEAT", change.Added)
	}
}

func TestBuildPlanCategoryLeavesAKindAlone(t *testing.T) {
	plan := ghbridge.BuildPlan(mustParseFixture(t), []gitbugio.Issue{bug("bug2605", 2605, "bug")}, mustLoadConfig(t))
	change := changeFor(t, plan, 2605)
	for _, l := range change.Added {
		if l == "feat" {
			t.Errorf("Added = %v: Category overrode the label bug", change.Added)
		}
	}
}

func TestBuildPlanCategoryNotAKindWarns(t *testing.T) {
	items := mustParseFixture(t)
	for i := range items {
		if items[i].Category != "" {
			items[i].Category = "FIX"
		}
	}
	plan := ghbridge.BuildPlan(items, []gitbugio.Issue{bug("bug2605", 2605)}, mustLoadConfig(t))
	change := changeFor(t, plan, 2605)
	for _, l := range change.Added {
		if strings.EqualFold(l, "fix") {
			t.Errorf("Added = %v, want no fix: it isn't one of the kinds", change.Added)
		}
	}
	if len(change.Warnings) != 2 {
		t.Errorf("Warnings = %v, want status's and category's", change.Warnings)
	}
}

func TestBuildPlanMergesDuplicateGenericModuleOptions(t *testing.T) {
	items := mustParseFixture(t)
	cfg := mustLoadConfig(t)
	bugs := []gitbugio.Issue{
		bug("bugGeneric1", 2610),
		bug("bugGeneric2", 2611),
	}

	plan := ghbridge.BuildPlan(items, bugs, cfg)

	for _, number := range []int{2610, 2611} {
		change := changeFor(t, plan, number)
		found := false
		for _, l := range change.Added {
			if l == "mod:generic" {
				found = true
			}
		}
		if !found {
			t.Errorf("issue #%d: want mod:generic in Added, got %v", number, change.Added)
		}
	}
}

func TestBuildPlanSkipsEmptyFieldsWithoutWarning(t *testing.T) {
	items := mustParseFixture(t)
	cfg := mustLoadConfig(t)
	bugs := []gitbugio.Issue{bug("bugEmpty", 2620)}

	plan := ghbridge.BuildPlan(items, bugs, cfg)
	change := changeFor(t, plan, 2620)

	if len(change.Added) != 1 || change.Added[0] != "assignee:BenBotros" {
		t.Errorf("Added = %v, want just [assignee:BenBotros] (status/prio empty)", change.Added)
	}
	if len(change.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none for empty (not invalid) values", change.Warnings)
	}
}

func TestBuildPlanWarnsOnUnmatchedConfiguredOptionValue(t *testing.T) {
	items := mustParseFixture(t)
	cfg := mustLoadConfig(t)
	bugs := []gitbugio.Issue{bug("bugWeird", 2621)}

	plan := ghbridge.BuildPlan(items, bugs, cfg)
	change := changeFor(t, plan, 2621)

	if len(change.Warnings) != 2 {
		t.Fatalf("Warnings = %v, want 2 (status P9-alike + priority)", change.Warnings)
	}
	for _, l := range change.Added {
		if l == "status:some-new-status" || l == "prio:p9" {
			t.Errorf("unconfigured value %q should have been skipped, not written", l)
		}
	}
}

func TestBuildPlanReportsUnmatchedIssueAndExcludesPRs(t *testing.T) {
	items := mustParseFixture(t)
	cfg := mustLoadConfig(t)
	// No bug for #2650 (not bridged) and none for the PR (#2633, excluded
	// before matching is even attempted).
	bugs := []gitbugio.Issue{bug("bug2605", 2605)}

	plan := ghbridge.BuildPlan(items, bugs, cfg)

	reasons := map[int]string{}
	for _, u := range plan.Unmatched {
		reasons[u.Number] = u.Reason
	}
	if _, ok := reasons[2650]; !ok {
		t.Errorf("expected #2650 (not bridged) in Unmatched, got %+v", plan.Unmatched)
	}
	if _, ok := reasons[2633]; !ok {
		t.Errorf("expected #2633 (PullRequest content type) in Unmatched, got %+v", plan.Unmatched)
	}
}

func TestBuildPlanIsPureNoMutationOfInputs(t *testing.T) {
	items := mustParseFixture(t)
	cfg := mustLoadConfig(t)
	b := bug("bug2605", 2605)
	bugsBefore := append([]string(nil), b.Labels...)

	_ = ghbridge.BuildPlan(items, []gitbugio.Issue{b}, cfg)

	if len(b.Labels) != len(bugsBefore) {
		t.Errorf("BuildPlan mutated the input Issue's Labels: %v vs %v", b.Labels, bugsBefore)
	}
}

// bridge pull reads the Project the way bridge sync does: option aliases,
// any number for a numeric field, sprints outside the list by their title.
func TestBuildPlanUsesTheSyncMapping(t *testing.T) {
	cfg := &config.Config{Fields: map[string]config.Field{
		"status": {Single: true, Options: []string{"qa"}, Project: map[string]string{"qa": "QA Validation"}},
		"est":    {Single: true, Options: []string{"1", "2"}, Numeric: true},
	}}
	est := 3.5
	items := []ghbridge.Item{{Status: "QA Validation", EstHours: &est, Sprint: &ghbridge.Sprint{Title: "Hypercare"},
		Content: ghbridge.Content{Number: 7, Type: "Issue", URL: bug("b7", 7).GithubURL}}}

	change := changeFor(t, ghbridge.BuildPlan(items, []gitbugio.Issue{bug("b7", 7)}, cfg), 7)
	got := map[string]bool{}
	for _, l := range change.Added {
		got[l] = true
	}
	for _, want := range []string{"status:qa", "est:3.5", "sprint:hypercare"} {
		if !got[want] {
			t.Errorf("%s missing from Added = %v", want, change.Added)
		}
	}
	if len(change.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", change.Warnings)
	}
}
