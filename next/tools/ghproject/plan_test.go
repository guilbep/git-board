package ghproject

// v1's internal/ghbridge tests (sync_test.go, case_test.go,
// ghbridge_test.go), ported: the same cases over v2's types.

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/forge"
	"github.com/guilbep/git-board/next/forge/github"
)

func TestDecide(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name                 string
		project, local, base string
		hasBase              bool
		want                 Decision
	}{
		{"agreed", "ready", "ready", "backlog", true, Agreed},
		{"both empty", "", "", "", false, Agreed},
		{"project changed", "qa", "ready", "ready", true, TakeProject},
		{"project cleared", "", "ready", "ready", true, TakeProject},
		{"local changed", "ready", "in-review", "ready", true, TakeLocal},
		{"local cleared", "ready", "", "ready", true, TakeLocal},
		{"both changed", "qa", "in-review", "ready", true, Held},
		{"first sync, project set", "qa", "ready", "", false, TakeProject},
		// a field never set on the Project can't be told from one a
		// teammate cleared
		{"first sync, project empty", "", "ready", "", false, Held},
	}
	for _, c := range cases {
		if got := Decide(c.project, c.local, c.base, c.hasBase); got != c.want {
			t.Errorf("%s: Decide(%q, %q, %q, %v) = %v, want %v", c.name, c.project, c.local, c.base, c.hasBase, got, c.want)
		}
	}
}

const testURL = "https://github.com/o/r/issues/"

var ghTable = forge.Table{Name: "gh", Kind: "github", Host: "github.com"}

func syncConfig() *Config {
	return &Config{Owner: "o", Number: 1, Write: true, Forge: ghTable,
		Schema: &issues.Schema{Fields: map[string]issues.Field{
			"status": {Single: true, Options: []string{"backlog", "comite", "ready", "in-review", "qa"},
				Project: map[string]string{"qa": "QA Validation"}},
			"prio": {Single: true, Options: []string{"p0", "p1", "p2"}},
			"est":  {Single: true, Options: []string{"1", "2", "4"}, Numeric: true},
		}, Sprints: issues.Sprints{List: []issues.Sprint{
			{Slug: "sprint-4", Title: "Sprint 4", Start: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)},
		}}},
	}
}

// withKinds sets [branches] kinds, the kind field flat while it has no
// options, as the issue domain reads the file.
func withKinds(c *Config, kinds map[string]string) {
	c.Schema.Kinds = kinds
	c.Schema.Fields["kind"] = issues.Field{Single: true, Flat: true}
}

func syncMeta() Meta {
	return Meta{ID: "P", Fields: map[string]Field{
		"Status": {ID: "F-status", Type: "SINGLE_SELECT", Options: map[string]string{
			"Backlog": "o-backlog", "Comité Spécialisé": "o-comite", "Ready": "o-ready",
			"In review": "o-review", "QA Validation": "o-qa"}},
		"Priority":   {ID: "F-prio", Type: "SINGLE_SELECT", Options: map[string]string{"P0": "o-p0", "P1": "o-p1", "P2": "o-p2"}},
		"Est. hours": {ID: "F-est", Type: "NUMBER"},
		"Sprint":     {ID: "F-sprint", Type: "ITERATION", Options: map[string]string{"Sprint 4": "i-4"}},
	}}
}

func item(n int, status string) Item {
	it := Item{ID: "item-" + status, Type: "Issue", Number: n, URL: testURL + "1", Values: map[string]string{}}
	if status != "" {
		it.Values["Status"] = status
	}
	return it
}

func set(it Item, column, value string) Item {
	it.Values = copyOf(it.Values)
	if value == "" {
		delete(it.Values, column)
	} else {
		it.Values[column] = value
	}
	return it
}

func copyOf(m map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range m {
		out[k] = v
	}
	return out
}

func oneIssue(labels ...string) []Issue {
	return []Issue{{ID: "bug1", URL: testURL + "1", Labels: labels}}
}

func plan(it Item, list []Issue, bases map[string]string) Plan {
	all := map[string]map[string]string{}
	if bases != nil {
		all["bug1"] = bases
	}
	return BuildPlan([]Item{it}, list, all, syncMeta(), syncConfig())
}

func TestSyncFirstRunImports(t *testing.T) {
	t.Parallel()
	p := plan(item(1, "Ready"), oneIssue(), nil)
	if got := p.Changes[0].Added; !slices.Contains(got, "status:ready") {
		t.Errorf("added = %v, want status:ready", got)
	}
	if len(p.Edits) != 0 || len(p.Conflicts) != 0 {
		t.Errorf("edits = %v, conflicts = %v, want none", p.Edits, p.Conflicts)
	}
	if got := p.Bases["bug1"]["status"]; got != "ready" {
		t.Errorf("base = %q, want ready", got)
	}
}

func TestSyncCategoryFillsAMissingKind(t *testing.T) {
	t.Parallel()
	it := set(item(1, "Ready"), "Category", "FEAT")
	cfg := syncConfig()
	withKinds(cfg, map[string]string{"bug": "fix", "feat": "feat"})
	p := BuildPlan([]Item{it}, oneIssue(), nil, syncMeta(), cfg)
	if got := p.Changes[0].Added; !slices.Contains(got, "feat") || slices.Contains(got, "FEAT") {
		t.Errorf("added = %v, want feat and no FEAT", got)
	}
	// with [fields.kind] options the kinds are kind:* labels
	cfg.Schema.Fields["kind"] = issues.Field{Single: true, Options: []string{"bug", "feat"}}
	p = BuildPlan([]Item{it}, oneIssue(), nil, syncMeta(), cfg)
	if got := p.Changes[0].Added; !slices.Contains(got, "kind:feat") || slices.Contains(got, "feat") {
		t.Errorf("kind:* form: added = %v, want kind:feat", got)
	}
}

func TestSyncLocalChangeGoesToProject(t *testing.T) {
	t.Parallel()
	p := plan(item(1, "Ready"), oneIssue("status:in-review"), map[string]string{"status": "ready"})
	want := []Edit{{IssueID: "bug1", ItemID: "item-Ready", Number: 1, Field: "status",
		Value: "in-review", FieldID: "F-status", Kind: EditOption, Arg: "o-review"}}
	if !reflect.DeepEqual(p.Edits, want) {
		t.Errorf("edits = %+v, want %+v", p.Edits, want)
	}
	if len(p.Changes[0].Added)+len(p.Changes[0].Removed) != 0 {
		t.Errorf("labels changed: %+v", p.Changes[0])
	}
	if got := p.Bases["bug1"]["status"]; got != "in-review" {
		t.Errorf("base = %q, want in-review", got)
	}
}

func TestSyncProjectChangeGoesToLabels(t *testing.T) {
	t.Parallel()
	// "QA Validation" is the Project's name of the status qa
	p := plan(item(1, "QA Validation"), oneIssue("status:ready"), map[string]string{"status": "ready"})
	c := p.Changes[0]
	if !slices.Equal(c.Added, []string{"status:qa"}) || !slices.Equal(c.Removed, []string{"status:ready"}) {
		t.Errorf("added %v removed %v, want status:qa for status:ready", c.Added, c.Removed)
	}
	if len(p.Edits) != 0 {
		t.Errorf("edits = %v", p.Edits)
	}
}

func TestSyncConflictIsHeld(t *testing.T) {
	t.Parallel()
	p := plan(item(1, "QA Validation"), oneIssue("status:in-review"), map[string]string{"status": "ready"})
	want := []Conflict{{IssueID: "bug1", Number: 1, URL: testURL + "1", Field: "status",
		Base: "ready", HasBase: true, Local: "in-review", Project: "qa"}}
	if !reflect.DeepEqual(p.Conflicts, want) {
		t.Errorf("conflicts = %+v, want %+v", p.Conflicts, want)
	}
	if len(p.Edits) != 0 || len(p.Changes[0].Added)+len(p.Changes[0].Removed) != 0 {
		t.Errorf("a side was written: edits %v, change %+v", p.Edits, p.Changes[0])
	}
	if _, ok := p.Bases["bug1"]["status"]; ok {
		t.Error("base recorded for a conflict")
	}
}

func TestSyncFirstRunEmptyProjectIsHeld(t *testing.T) {
	t.Parallel()
	p := plan(item(1, ""), oneIssue("status:ready"), nil)
	if len(p.Conflicts) != 1 || len(p.Edits) != 0 {
		t.Errorf("conflicts %v edits %v, want the field held", p.Conflicts, p.Edits)
	}
}

func TestSyncUnmappableValueFreezesField(t *testing.T) {
	t.Parallel()
	// the Project's value has no option: not read as "cleared"
	p := plan(item(1, "Brand new status"), oneIssue("status:in-review"), map[string]string{"status": "ready"})
	if len(p.Edits) != 0 || len(p.Conflicts) != 0 || len(p.Changes[0].Added)+len(p.Changes[0].Removed) != 0 {
		t.Errorf("field not frozen: %+v", p)
	}
	if len(p.Changes[0].Warnings) == 0 {
		t.Error("no warning")
	}
	if _, ok := p.Bases["bug1"]["status"]; ok {
		t.Error("base recorded for an unmappable value")
	}

	// a local value with no Project option isn't written
	p = plan(item(1, "Ready"), oneIssue("status:backlog-ish"), map[string]string{"status": "ready"})
	if len(p.Edits) != 0 || len(p.Changes[0].Warnings) == 0 {
		t.Errorf("edits %v warnings %v", p.Edits, p.Changes[0].Warnings)
	}
}

func TestSyncClearsBothWays(t *testing.T) {
	t.Parallel()
	p := plan(item(1, ""), oneIssue("status:ready"), map[string]string{"status": "ready"})
	if !slices.Equal(p.Changes[0].Removed, []string{"status:ready"}) {
		t.Errorf("Project cleared: removed %v", p.Changes[0].Removed)
	}
	if got, ok := p.Bases["bug1"]["status"]; !ok || got != "" {
		t.Errorf("base = %q, %v, want recorded empty", got, ok)
	}

	p = plan(item(1, "Ready"), oneIssue(), map[string]string{"status": "ready"})
	if len(p.Edits) != 1 || p.Edits[0].Kind != EditClear {
		t.Errorf("label removed: edits %+v, want one clear", p.Edits)
	}
}

func TestSyncNumbersAndSprints(t *testing.T) {
	t.Parallel()
	it := set(set(item(1, "Ready"), "Est. hours", "4"), "Sprint", "Sprint 4")
	p := plan(it, oneIssue("status:ready"), nil)
	if a := p.Changes[0].Added; !slices.Contains(a, "est:4") || !slices.Contains(a, "sprint:sprint-4") {
		t.Errorf("added %v", a)
	}

	it = set(set(it, "Est. hours", "2"), "Sprint", "")
	p = plan(it, oneIssue("status:ready", "est:1", "sprint:sprint-4"), map[string]string{"status": "ready", "est": "2", "sprint": ""})
	kinds := map[string]Edit{}
	for _, e := range p.Edits {
		kinds[e.Field] = e
	}
	if e := kinds["est"]; e.Kind != EditNumber || e.Arg != "1" {
		t.Errorf("est edit %+v", e)
	}
	if e := kinds["sprint"]; e.Kind != EditIteration || e.Arg != "i-4" {
		t.Errorf("sprint edit %+v", e)
	}
}

func TestSlugTransliteratesAccents(t *testing.T) {
	t.Parallel()
	cfg := syncConfig()
	cfg.Schema.Fields["status"] = issues.Field{Single: true, Options: []string{"comite-specialise"}}
	p := BuildPlan([]Item{item(1, "Comité Spécialisé")}, oneIssue(), nil, syncMeta(), cfg)
	if !slices.Contains(p.Changes[0].Added, "status:comite-specialise") {
		t.Errorf("added %v warnings %v", p.Changes[0].Added, p.Changes[0].Warnings)
	}
}

func assigneeEdit(p Plan) (Edit, bool) {
	for _, e := range p.Edits {
		if e.Field == "assignee" {
			return e, true
		}
	}
	return Edit{}, false
}

func TestSyncAssigneesMergeBothWays(t *testing.T) {
	t.Parallel()
	it := item(1, "Ready")
	it.Assignees = []string{"alice", "bob"}
	p := plan(it, oneIssue("status:ready", "assignee:alice", "assignee:carol"), map[string]string{"status": "ready", "assignee": "alice"})
	c := p.Changes[0]
	if !slices.Equal(c.Added, []string{"assignee:bob"}) || len(c.Removed) != 0 {
		t.Errorf("added %v removed %v, want bob added and carol kept", c.Added, c.Removed)
	}
	if e, ok := assigneeEdit(p); !ok || !slices.Equal(e.Add, []string{"carol"}) || len(e.Remove) != 0 {
		t.Errorf("edit %+v, want carol added on GitHub", e)
	}
	if got := p.Bases["bug1"]["assignee"]; got != "alice,bob,carol" {
		t.Errorf("base %q", got)
	}

	it.Assignees = []string{"bob"}
	p = plan(it, oneIssue("status:ready", "assignee:alice", "assignee:bob"), map[string]string{"status": "ready", "assignee": "alice,bob"})
	if c := p.Changes[0]; !slices.Equal(c.Removed, []string{"assignee:alice"}) {
		t.Errorf("removed %v, want alice", c.Removed)
	}
}

// fakeSide records every write of Apply, in order: the board and the
// Project.
type fakeSide struct {
	calls []string
	bases map[string]map[string]string
	fail  bool
}

func (f *fakeSide) Issues(context.Context) ([]Issue, error) { return nil, nil }
func (f *fakeSide) Bases(context.Context, string, string) (map[string]string, error) {
	return nil, nil
}

func (f *fakeSide) ChangeLabels(_ context.Context, id string, add, remove []string) error {
	f.calls = append(f.calls, "labels")
	return nil
}

func (f *fakeSide) SetBases(_ context.Context, id, prefix string, values map[string]string) error {
	f.calls = append(f.calls, "bases")
	if f.bases == nil {
		f.bases = map[string]map[string]string{}
	}
	f.bases[id] = values
	return nil
}

func (f *fakeSide) Meta(context.Context) (github.ProjectMeta, error) {
	return github.ProjectMeta{}, nil
}
func (f *fakeSide) Items(context.Context, int) ([]Item, bool, error)     { return nil, false, nil }
func (f *fakeSide) Assignable(context.Context, string) ([]string, error) { return nil, nil }

func (f *fakeSide) Set(_ context.Context, _ string, v github.ProjectValue) error {
	if f.fail {
		return errors.New("github: boom")
	}
	f.calls = append(f.calls, "edit:"+v.Field)
	return nil
}

func (f *fakeSide) Assign(context.Context, string, int, []string, []string) error {
	f.calls = append(f.calls, "assign")
	return nil
}

func TestApplyWritesProjectThenLabelsThenBases(t *testing.T) {
	t.Parallel()
	it := set(item(1, "QA Validation"), "Priority", "P1")
	p := plan(it, oneIssue("status:ready", "prio:p2"), map[string]string{"status": "ready", "prio": "p1"})
	var f fakeSide
	if _, err := Apply(t.Context(), &f, &f, p, syncConfig(), syncMeta()); err != nil {
		t.Fatal(err)
	}
	if want := []string{"edit:F-prio", "labels", "bases"}; !slices.Equal(f.calls, want) {
		t.Errorf("calls %v, want %v", f.calls, want)
	}
}

func TestApplyNeverWritesProjectUnlessEnabled(t *testing.T) {
	t.Parallel()
	cfg := syncConfig()
	cfg.Write = false
	p := BuildPlan([]Item{item(1, "Ready")}, oneIssue("status:in-review"),
		map[string]map[string]string{"bug1": {"status": "ready"}}, syncMeta(), cfg)
	var f fakeSide
	skipped, err := Apply(t.Context(), &f, &f, p, cfg, syncMeta())
	if err != nil {
		t.Fatal(err)
	}
	if slices.ContainsFunc(f.calls, func(c string) bool { return c != "labels" && c != "bases" }) {
		t.Errorf("wrote to the Project: %v", f.calls)
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
	if _, ok := f.bases["bug1"]["status"]; ok {
		t.Error("base recorded for a change not written")
	}
}

func TestApplyStopsAnIssueOnFailedEdit(t *testing.T) {
	t.Parallel()
	p := plan(item(1, "Ready"), oneIssue("status:in-review"), map[string]string{"status": "ready"})
	f := fakeSide{fail: true}
	if _, err := Apply(t.Context(), &f, &f, p, syncConfig(), syncMeta()); err == nil {
		t.Fatal("no error")
	}
	if slices.Contains(f.calls, "bases") {
		t.Errorf("base recorded after a failed edit: %v", f.calls)
	}
}

func TestResolveKeepsOneSide(t *testing.T) {
	t.Parallel()
	it := item(1, "QA Validation")
	list := oneIssue("status:in-review")
	bases := map[string]string{"status": "ready"}
	c := plan(it, list, bases).Conflicts[0]

	ours, err := Resolve(c, true, it, list[0], syncMeta(), syncConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(ours.Edits) != 1 || ours.Edits[0].Arg != "o-review" || len(ours.Changes[0].Added) != 0 {
		t.Errorf("ours: edits %+v change %+v", ours.Edits, ours.Changes)
	}
	if got := ours.Bases["bug1"]["status"]; got != "in-review" {
		t.Errorf("ours base %q", got)
	}

	theirs, err := Resolve(c, false, it, list[0], syncMeta(), syncConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(theirs.Edits) != 0 || !slices.Equal(theirs.Changes[0].Added, []string{"status:qa"}) {
		t.Errorf("theirs: edits %+v change %+v", theirs.Edits, theirs.Changes)
	}
	if got := theirs.Bases["bug1"]["status"]; got != "qa" {
		t.Errorf("theirs base %q", got)
	}
}

func TestSyncSprintsOutsideTheListComeFromTheirTitle(t *testing.T) {
	t.Parallel()
	meta := syncMeta()
	meta.Fields["Sprint"].Options["Hypercare"] = "i-h"
	it := set(item(1, "Ready"), "Sprint", "Hypercare")
	p := BuildPlan([]Item{it}, oneIssue("status:ready"), nil, meta, syncConfig())
	if a := p.Changes[0].Added; !slices.Contains(a, "sprint:hypercare") || len(p.Changes[0].Warnings) != 0 {
		t.Errorf("added %v warnings %v", a, p.Changes[0].Warnings)
	}

	// and back: a local sprint named after an iteration is written to it
	it = set(it, "Sprint", "")
	bases := map[string]map[string]string{"bug1": {"status": "ready", "sprint": ""}}
	p = BuildPlan([]Item{it}, oneIssue("status:ready", "sprint:hypercare"), bases, meta, syncConfig())
	if len(p.Edits) != 1 || p.Edits[0].Kind != EditIteration || p.Edits[0].Arg != "i-h" {
		t.Errorf("edits %+v, want the Hypercare iteration", p.Edits)
	}
}

func TestSyncNumericFieldTakesAnyNumber(t *testing.T) {
	t.Parallel()
	p := plan(set(item(1, "Ready"), "Est. hours", "3.5"), oneIssue("status:ready"), nil)
	if a := p.Changes[0].Added; !slices.Contains(a, "est:3.5") || len(p.Changes[0].Warnings) != 0 {
		t.Errorf("added %v warnings %v", a, p.Changes[0].Warnings)
	}
}

// A Project value with nothing to slug ("-") isn't read as "cleared".
func TestSyncValueWithAnEmptySlugIsLeftAlone(t *testing.T) {
	t.Parallel()
	it := set(item(1, "Ready"), "Sprint", "-")
	p := plan(it, oneIssue("status:ready", "sprint:sprint-4"), map[string]string{"status": "ready", "sprint": "sprint-4"})
	if len(p.Changes[0].Removed) != 0 || len(p.Changes[0].Warnings) == 0 {
		t.Errorf("removed %v warnings %v, want the sprint left alone with a warning", p.Changes[0].Removed, p.Changes[0].Warnings)
	}
}

// Since they agreed on alice and bob, git-board removed bob and added
// carol: both reach the GitHub issue, the labels stay.
func TestSyncAssigneesWriteBackLocalChanges(t *testing.T) {
	t.Parallel()
	it := item(1, "Ready")
	it.Assignees = []string{"alice", "bob"}
	p := plan(it, oneIssue("status:ready", "assignee:alice", "assignee:carol"), map[string]string{"status": "ready", "assignee": "alice,bob"})
	e, ok := assigneeEdit(p)
	if !ok || e.Kind != EditAssignees || e.URL != testURL+"1" ||
		!slices.Equal(e.Add, []string{"carol"}) || !slices.Equal(e.Remove, []string{"bob"}) {
		t.Fatalf("edit %+v, want +carol -bob on the issue", e)
	}
	if c := p.Changes[0]; len(c.Added)+len(c.Removed) != 0 {
		t.Errorf("labels changed: +%v -%v", c.Added, c.Removed)
	}
	if got := p.Bases["bug1"]["assignee"]; got != "alice,carol" {
		t.Errorf("base %q, want alice,carol", got)
	}
}

// Without write = true assignees only come from GitHub.
func TestSyncAssigneesStayLocalWithoutWrite(t *testing.T) {
	t.Parallel()
	cfg := syncConfig()
	cfg.Write = false
	it := item(1, "Ready")
	it.Assignees = []string{"alice", "bob"}
	p := BuildPlan([]Item{it}, oneIssue("status:ready", "assignee:alice", "assignee:carol"),
		map[string]map[string]string{"bug1": {"status": "ready", "assignee": "alice,bob"}}, syncMeta(), cfg)
	if e, ok := assigneeEdit(p); ok {
		t.Errorf("edit %+v without write", e)
	}
	if got, changed := p.Bases["bug1"]["assignee"]; changed {
		t.Errorf("base changed to %q, want GitHub's alice,bob kept", got)
	}
}

// GitHub silently drops an assignee without access to the repository:
// such a login stays in git-board, with a warning, and isn't recorded as
// agreed.
func TestSyncAssigneesSkipWhoCantBeAssigned(t *testing.T) {
	t.Parallel()
	meta := syncMeta()
	meta.Assignable = map[string][]string{"o/r": {"alice", "bob"}}
	it := item(1, "Ready")
	it.Assignees = []string{"alice"}
	p := BuildPlan([]Item{it}, oneIssue("status:ready", "assignee:alice", "assignee:bob", "assignee:dave"),
		map[string]map[string]string{"bug1": {"status": "ready", "assignee": "alice"}}, meta, syncConfig())
	if e, ok := assigneeEdit(p); !ok || !slices.Equal(e.Add, []string{"bob"}) {
		t.Errorf("edit %+v, want only bob added", e)
	}
	if len(p.Changes[0].Warnings) != 1 || !strings.Contains(p.Changes[0].Warnings[0], "dave") {
		t.Errorf("warnings %v, want one about dave", p.Changes[0].Warnings)
	}
	if got := p.Bases["bug1"]["assignee"]; got != "alice,bob" {
		t.Errorf("base %q, want alice,bob", got)
	}
}

func TestSyncCategoryRemovesItsBareLabel(t *testing.T) {
	t.Parallel()
	it := set(item(1, "Ready"), "Category", "FEAT")
	cfg := syncConfig()
	withKinds(cfg, map[string]string{"bug": "fix", "feat": "feat"})
	p := BuildPlan([]Item{it}, oneIssue("FEAT", "bug"), nil, syncMeta(), cfg)
	c := p.Changes[0]
	if !slices.Contains(c.Removed, "FEAT") || slices.Contains(c.Added, "feat") {
		t.Errorf("added %v removed %v, want FEAT removed and no feat beside bug", c.Added, c.Removed)
	}
}

// MoSCoW is kept in step as Priority is; Won't, which slugs to won-t, is
// named in [fields.moscow] project.
func TestSyncMoSCoW(t *testing.T) {
	t.Parallel()
	cfg := syncConfig()
	cfg.Schema.Fields["moscow"] = issues.Field{Single: true, Options: []string{"must", "should", "could", "wont"}, Project: map[string]string{"wont": "Won't"}}
	meta := syncMeta()
	meta.Fields["MoSCoW"] = Field{ID: "F-moscow", Type: "SINGLE_SELECT", Options: map[string]string{"Must": "o-must", "Should": "o-should", "Could": "o-could", "Won't": "o-wont"}}
	it := set(item(1, "Ready"), "MoSCoW", "Won't")
	p := BuildPlan([]Item{it}, oneIssue(), nil, meta, cfg)
	if got := p.Changes[0].Added; !slices.Contains(got, "moscow:wont") {
		t.Errorf("added = %v, want moscow:wont", got)
	}
	it = set(it, "MoSCoW", "Should")
	p = BuildPlan([]Item{it}, oneIssue("moscow:must"), map[string]map[string]string{"bug1": {"moscow": "should"}}, meta, cfg)
	if !slices.ContainsFunc(p.Edits, func(e Edit) bool { return e.Field == "moscow" && e.Value == "must" }) {
		t.Errorf("edits = %+v, want moscow set to must on the Project", p.Edits)
	}
}

// The milestone, a deliverable, comes from the Project and is never
// written back: GitHub's issue holds it, which a Project edit can't set.
func TestSyncMilestoneReadOnly(t *testing.T) {
	t.Parallel()
	meta := syncMeta()
	meta.Fields["Milestone"] = Field{ID: "F-milestone", Type: "MILESTONE"}
	it := item(1, "Ready")
	it.Milestone = "V3.1 – Pilot"
	p := BuildPlan([]Item{it}, oneIssue(), nil, meta, syncConfig())
	if got := p.Changes[0].Added; !slices.Contains(got, "milestone:v3-1-pilot") {
		t.Errorf("added = %v, want milestone:v3-1-pilot", got)
	}
	// changed here only: the Project's comes back, nothing is written
	p = BuildPlan([]Item{it}, oneIssue("milestone:v4"), map[string]map[string]string{"bug1": {"milestone": "v3-1-pilot"}}, meta, syncConfig())
	if len(p.Edits) != 0 || len(p.Conflicts) != 0 || !slices.Contains(p.Changes[0].Added, "milestone:v3-1-pilot") {
		t.Errorf("edits %+v, conflicts %+v, added %v; want the Project's milestone back", p.Edits, p.Conflicts, p.Changes[0].Added)
	}
}

// One side of a plan drops the other side's writes, and with each the
// base it would have recorded: what isn't sent isn't agreed.
func TestPlanOneSide(t *testing.T) {
	t.Parallel()
	plan := Plan{
		Changes: []Change{{IssueID: "b", Number: 1, Fields: []FieldResult{{Field: "status", Added: []string{"status:qa"}}}, Added: []string{"status:qa"}}},
		Edits:   []Edit{{IssueID: "b", Field: "prio", Value: "high"}},
		Bases:   map[string]map[string]string{"b": {"status": "qa", "prio": "high", "est": "3"}},
	}

	labels := plan.LabelsOnly()
	if len(labels.Edits) != 0 || !reflect.DeepEqual(labels.Bases["b"], map[string]string{"status": "qa", "est": "3"}) ||
		!slices.Equal(labels.Changes[0].Added, []string{"status:qa"}) {
		t.Errorf("labels only: %+v", labels)
	}

	project := plan.ProjectOnly()
	if len(project.Changes[0].Added) != 0 || len(project.Changes[0].Fields) != 0 || project.Changes[0].Number != 1 ||
		len(project.Edits) != 1 || !reflect.DeepEqual(project.Bases["b"], map[string]string{"prio": "high", "est": "3"}) {
		t.Errorf("project only: %+v", project)
	}

	if len(plan.Bases["b"]) != 3 || len(plan.Changes[0].Added) != 1 {
		t.Errorf("the plan changed: %+v", plan)
	}
}

// Two repositories of one Project can both have an issue #1 in conflict:
// the number alone is refused, owner/repo#1 picks one.
func TestResolveRefusesAnAmbiguousNumber(t *testing.T) {
	t.Parallel()
	in := func(repo, id string) (Item, Issue) {
		u := "https://github.com/" + repo + "/issues/1"
		return Item{ID: "item-" + repo, Type: "Issue", Number: 1, URL: u, Values: map[string]string{"Status": "QA Validation"}},
			Issue{ID: id, URL: u, Labels: []string{"status:in-review"}}
	}
	i1, b1 := in("o/r", "bug1")
	i2, b2 := in("o/s", "bug2")
	s := State{Remote: Remote{Items: []Item{i1, i2}, Meta: syncMeta()}, Issues: []Issue{b1, b2},
		Bases: map[string]map[string]string{"bug1": {"status": "ready"}, "bug2": {"status": "ready"}}}
	cfg := syncConfig()

	var side fakeSide
	if _, err := ResolveIn(t.Context(), &side, &side, cfg, s, "", 1, "status", false); err == nil || !strings.Contains(err.Error(), "o/s#1") {
		t.Fatalf("err = %v, want the two to choose from", err)
	}
	if len(side.calls) != 0 {
		t.Fatalf("wrote %v", side.calls)
	}
	if _, err := ResolveIn(t.Context(), &side, &side, cfg, s, "o/s", 1, "status", false); err != nil {
		t.Fatal(err)
	}
	if _, ok := side.bases["bug2"]; !ok || len(side.bases) != 1 {
		t.Errorf("bases written %v, want bug2's alone", side.bases)
	}
	if _, err := ResolveIn(t.Context(), &side, &side, cfg, s, "o/r", 1, "prio", false); !errors.Is(err, ErrNotConflict) {
		t.Errorf("a field in no conflict: %v", err)
	}
	cfg.Write = false
	if _, err := ResolveIn(t.Context(), &side, &side, cfg, s, "o/r", 1, "status", true); !errors.Is(err, ErrNoWrite) {
		t.Errorf("ours without write: %v", err)
	}
}

// Values spelled as the forge does (P0, Must) still match the Project's
// options both ways: by their slugs.
func TestProjectValuesInTheForgesSpelling(t *testing.T) {
	t.Parallel()
	cfg := &Config{Forge: ghTable, Schema: &issues.Schema{Fields: map[string]issues.Field{
		"prio":   {Single: true, Options: []string{"P0", "P1"}},
		"status": {Single: true, Options: []string{"backlog", "in-progress"}},
	}}}
	sel := Field{Type: "SINGLE_SELECT"}
	for raw, want := range map[string]string{"P0": "P0", "p1": "P1"} {
		if v, ok := cfg.toLabel(sel, "prio", raw); !ok || v != want {
			t.Errorf("toLabel(%q) = %q, %v; want %q", raw, v, ok, want)
		}
	}
	if v, ok := cfg.toLabel(sel, "status", "In progress"); !ok || v != "in-progress" {
		t.Errorf("toLabel(In progress) = %q, %v", v, ok)
	}
	if _, id, ok := cfg.toProject(Field{Type: "SINGLE_SELECT", Options: map[string]string{"P0": "a", "P1": "b"}}, "prio", "P1"); !ok || id != "b" {
		t.Errorf("toProject(P1) = %q, %v", id, ok)
	}
}

func TestMappingSendsAnyFieldToTheProject(t *testing.T) {
	t.Parallel()
	gh := ghTable
	gh.Fields = map[string]forge.FieldMapping{"risk": {To: "project", Field: "Risk", Values: map[string]string{"structural": "Structural"}}}
	cfg := &Config{Forge: gh, Schema: &issues.Schema{Fields: map[string]issues.Field{
		"risk": {Single: true, Options: []string{"normal", "structural"}},
	}}}
	meta := Meta{ID: "P", Fields: map[string]Field{"Risk": {ID: "F-risk", Type: "SINGLE_SELECT", Options: map[string]string{"Structural": "o-structural", "Normal": "o-normal"}}}}
	u := "https://github.com/o/r/issues/9"
	items := func(risk string) []Item {
		return []Item{{Type: "Issue", Number: 9, URL: u, Values: map[string]string{"Risk": risk}}}
	}

	p := BuildPlan(items("Structural"), []Issue{{ID: "b9", URL: u}}, nil, meta, cfg)
	if len(p.Changes) != 1 || !slices.Contains(p.Changes[0].Added, "risk:structural") {
		t.Errorf("changes = %+v, want risk:structural, the Project's Structural read back", p.Changes)
	}
	p = BuildPlan(items("Normal"), []Issue{{ID: "b9", URL: u, Labels: []string{"risk:structural"}}}, map[string]map[string]string{"b9": {"risk": "normal"}}, meta, cfg)
	if len(p.Edits) != 1 || p.Edits[0].Kind != EditOption || p.Edits[0].Arg != "o-structural" {
		t.Errorf("edits = %+v, want risk:normal → Structural", p.Edits)
	}
}

// New in v2: a date and a text column, by their types.
func TestSyncDateAndTextColumns(t *testing.T) {
	t.Parallel()
	gh := ghTable
	gh.Fields = map[string]forge.FieldMapping{"due": {To: "project", Field: "Due"}, "note": {To: "project", Field: "Note"}}
	cfg := &Config{Forge: gh, Write: true, Schema: &issues.Schema{Fields: map[string]issues.Field{"due": {Single: true}, "note": {Single: true}}}}
	meta := Meta{ID: "P", Fields: map[string]Field{"Due": {ID: "F-due", Type: "DATE"}, "Note": {ID: "F-note", Type: "TEXT"}}}
	u := "https://github.com/o/r/issues/9"
	it := Item{Type: "Issue", Number: 9, URL: u, Values: map[string]string{"Due": "2026-12-01", "Note": "  keep it short "}}
	p := BuildPlan([]Item{it}, []Issue{{ID: "b9", URL: u}}, nil, meta, cfg)
	if a := p.Changes[0].Added; !slices.Contains(a, "due:2026-12-01") || !slices.Contains(a, "note:keep it short") {
		t.Errorf("added %v warnings %v", a, p.Changes[0].Warnings)
	}
	p = BuildPlan([]Item{it}, []Issue{{ID: "b9", URL: u, Labels: []string{"due:2027-01-15", "note:keep it short"}}},
		map[string]map[string]string{"b9": {"due": "2026-12-01", "note": "keep it short"}}, meta, cfg)
	if len(p.Edits) != 1 || p.Edits[0].Kind != EditDate || p.Edits[0].Arg != "2027-01-15" {
		t.Errorf("edits %+v, want the date written", p.Edits)
	}
	p = BuildPlan([]Item{set(it, "Due", "next week")}, []Issue{{ID: "b9", URL: u}}, nil, meta, cfg)
	if len(p.Changes[0].Warnings) == 0 {
		t.Errorf("a date that isn't one: %+v", p.Changes[0])
	}
}
