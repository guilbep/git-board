package ghbridge_test

import (
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghbridge"
	"github.com/guilbep/git-board/internal/gitbugio"
)

func TestDecide(t *testing.T) {
	cases := []struct {
		name                 string
		project, local, base string
		hasBase              bool
		want                 ghbridge.Decision
	}{
		{"agreed", "ready", "ready", "backlog", true, ghbridge.Agreed},
		{"both empty", "", "", "", false, ghbridge.Agreed},
		{"project changed", "qa", "ready", "ready", true, ghbridge.TakeProject},
		{"project cleared", "", "ready", "ready", true, ghbridge.TakeProject},
		{"local changed", "ready", "in-review", "ready", true, ghbridge.TakeLocal},
		{"local cleared", "ready", "", "ready", true, ghbridge.TakeLocal},
		{"both changed", "qa", "in-review", "ready", true, ghbridge.Held},
		{"first sync, project set", "qa", "ready", "", false, ghbridge.TakeProject},
		// a field never set on the Project can't be told from one a
		// teammate cleared
		{"first sync, project empty", "", "ready", "", false, ghbridge.Held},
	}
	for _, c := range cases {
		if got := ghbridge.Decide(c.project, c.local, c.base, c.hasBase); got != c.want {
			t.Errorf("%s: Decide(%q, %q, %q, %v) = %v, want %v", c.name, c.project, c.local, c.base, c.hasBase, got, c.want)
		}
	}
}

const testURL = "https://github.com/o/r/issues/"

func syncConfig() *config.Config {
	return &config.Config{
		Fields: map[string]config.Field{
			"status": {Single: true, Options: []string{"backlog", "comite", "ready", "in-review", "qa"},
				Project: map[string]string{"qa": "QA Validation"}},
			"prio": {Single: true, Options: []string{"p0", "p1", "p2"}},
			"est":  {Single: true, Options: []string{"1", "2", "4"}, Numeric: true},
		},
		Sprints: config.SprintsConfig{List: []config.Sprint{
			{Slug: "sprint-4", Title: "Sprint 4", Start: time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)},
		}},
		GithubProject: config.GithubProject{Owner: "o", Number: 1, Write: true},
	}
}

func syncMeta() ghbridge.ProjectMeta {
	return ghbridge.ProjectMeta{ID: "P", Fields: map[string]ghbridge.ProjectField{
		"Status": {ID: "F-status", Options: map[string]string{
			"Backlog": "o-backlog", "Comité Spécialisé": "o-comite", "Ready": "o-ready",
			"In review": "o-review", "QA Validation": "o-qa"}},
		"Priority":   {ID: "F-prio", Options: map[string]string{"P0": "o-p0", "P1": "o-p1", "P2": "o-p2"}},
		"Est. hours": {ID: "F-est"},
		"Sprint":     {ID: "F-sprint", Options: map[string]string{"Sprint 4": "i-4"}},
	}}
}

func item(n int, status string) ghbridge.Item {
	return ghbridge.Item{ID: "item-" + status, Status: status,
		Content: ghbridge.Content{Number: n, Type: "Issue", URL: testURL + "1"}}
}

func oneBug(labels ...string) []gitbugio.Issue {
	return []gitbugio.Issue{{ID: "bug1", GithubURL: testURL + "1", Labels: labels}}
}

func plan(it ghbridge.Item, bugs []gitbugio.Issue, bases map[string]string) ghbridge.SyncPlan {
	all := map[string]map[string]string{}
	if bases != nil {
		all["bug1"] = bases
	}
	return ghbridge.BuildSyncPlan([]ghbridge.Item{it}, bugs, all, syncMeta(), syncConfig())
}

func TestSyncFirstRunImports(t *testing.T) {
	p := plan(item(1, "Ready"), oneBug(), nil)
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
	it := item(1, "Ready")
	it.Category = "FEAT"
	cfg := syncConfig()
	cfg.Branches.Kinds = map[string]string{"bug": "fix", "feat": "feat"}
	p := ghbridge.BuildSyncPlan([]ghbridge.Item{it}, oneBug(), nil, syncMeta(), cfg)
	if got := p.Changes[0].Added; !slices.Contains(got, "feat") || slices.Contains(got, "FEAT") {
		t.Errorf("added = %v, want feat and no FEAT", got)
	}
}

func TestSyncLocalChangeGoesToProject(t *testing.T) {
	p := plan(item(1, "Ready"), oneBug("status:in-review"), map[string]string{"status": "ready"})
	want := []ghbridge.ProjectEdit{{BugID: "bug1", ItemID: "item-Ready", Number: 1, Field: "status",
		Value: "in-review", FieldID: "F-status", Kind: ghbridge.EditOption, Arg: "o-review"}}
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
	// "QA Validation" is the Project's name of the status qa
	p := plan(item(1, "QA Validation"), oneBug("status:ready"), map[string]string{"status": "ready"})
	c := p.Changes[0]
	if !slices.Equal(c.Added, []string{"status:qa"}) || !slices.Equal(c.Removed, []string{"status:ready"}) {
		t.Errorf("added %v removed %v, want status:qa for status:ready", c.Added, c.Removed)
	}
	if len(p.Edits) != 0 {
		t.Errorf("edits = %v", p.Edits)
	}
}

func TestSyncConflictIsHeld(t *testing.T) {
	p := plan(item(1, "QA Validation"), oneBug("status:in-review"), map[string]string{"status": "ready"})
	want := []ghbridge.Conflict{{BugID: "bug1", Number: 1, URL: testURL + "1", Field: "status",
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
	p := plan(item(1, ""), oneBug("status:ready"), nil)
	if len(p.Conflicts) != 1 || len(p.Edits) != 0 {
		t.Errorf("conflicts %v edits %v, want the field held", p.Conflicts, p.Edits)
	}
}

func TestSyncUnmappableValueFreezesField(t *testing.T) {
	// the Project's value has no option: not read as "cleared"
	p := plan(item(1, "Brand new status"), oneBug("status:in-review"), map[string]string{"status": "ready"})
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
	p = plan(item(1, "Ready"), oneBug("status:backlog-ish"), map[string]string{"status": "ready"})
	if len(p.Edits) != 0 || len(p.Changes[0].Warnings) == 0 {
		t.Errorf("edits %v warnings %v", p.Edits, p.Changes[0].Warnings)
	}
}

func TestSyncClearsBothWays(t *testing.T) {
	p := plan(item(1, ""), oneBug("status:ready"), map[string]string{"status": "ready"})
	if !slices.Equal(p.Changes[0].Removed, []string{"status:ready"}) {
		t.Errorf("Project cleared: removed %v", p.Changes[0].Removed)
	}
	if got, ok := p.Bases["bug1"]["status"]; !ok || got != "" {
		t.Errorf("base = %q, %v, want recorded empty", got, ok)
	}

	p = plan(item(1, "Ready"), oneBug(), map[string]string{"status": "ready"})
	if len(p.Edits) != 1 || p.Edits[0].Kind != ghbridge.EditClear {
		t.Errorf("label removed: edits %+v, want one clear", p.Edits)
	}
}

func TestSyncNumbersAndSprints(t *testing.T) {
	four, two := 4.0, 2.0
	it := item(1, "Ready")
	it.EstHours = &four
	it.Sprint = &ghbridge.Sprint{Title: "Sprint 4"}
	p := plan(it, oneBug("status:ready"), nil)
	if a := p.Changes[0].Added; !slices.Contains(a, "est:4") || !slices.Contains(a, "sprint:sprint-4") {
		t.Errorf("added %v", a)
	}

	it.EstHours = &two
	it.Sprint = nil
	p = plan(it, oneBug("status:ready", "est:1", "sprint:sprint-4"), map[string]string{"status": "ready", "est": "2", "sprint": ""})
	kinds := map[string]ghbridge.ProjectEdit{}
	for _, e := range p.Edits {
		kinds[e.Field] = e
	}
	if e := kinds["est"]; e.Kind != ghbridge.EditNumber || e.Arg != "1" {
		t.Errorf("est edit %+v", e)
	}
	if e := kinds["sprint"]; e.Kind != ghbridge.EditIteration || e.Arg != "i-4" {
		t.Errorf("sprint edit %+v", e)
	}
}

func TestSlugTransliteratesAccents(t *testing.T) {
	cfg := syncConfig()
	cfg.Fields["status"] = config.Field{Single: true, Options: []string{"comite-specialise"}}
	p := ghbridge.BuildSyncPlan([]ghbridge.Item{item(1, "Comité Spécialisé")}, oneBug(), nil, syncMeta(), cfg)
	if !slices.Contains(p.Changes[0].Added, "status:comite-specialise") {
		t.Errorf("added %v warnings %v", p.Changes[0].Added, p.Changes[0].Warnings)
	}
}

func assigneeEdit(p ghbridge.SyncPlan) (ghbridge.ProjectEdit, bool) {
	for _, e := range p.Edits {
		if e.Field == "assignee" {
			return e, true
		}
	}
	return ghbridge.ProjectEdit{}, false
}

func TestSyncAssigneesMergeBothWays(t *testing.T) {
	it := item(1, "Ready")
	it.Assignees = []string{"alice", "bob"}
	p := plan(it, oneBug("status:ready", "assignee:alice", "assignee:carol"), map[string]string{"status": "ready", "assignee": "alice"})
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
	p = plan(it, oneBug("status:ready", "assignee:alice", "assignee:bob"), map[string]string{"status": "ready", "assignee": "alice,bob"})
	if c := p.Changes[0]; !slices.Equal(c.Removed, []string{"assignee:alice"}) {
		t.Errorf("removed %v, want alice", c.Removed)
	}
}

// fakeSide records every write of ApplySync, in order.
type fakeSide struct {
	calls []string
	bases map[string]map[string]string
}

func (f *fakeSide) ApplyLabelChange(id string, added, removed []string) ([]string, error) {
	f.calls = append(f.calls, "labels")
	return nil, nil
}

func (f *fakeSide) Edit(projectID string, e ghbridge.ProjectEdit) error {
	f.calls = append(f.calls, "edit:"+e.Field)
	return nil
}

func (f *fakeSide) SetProjectBases(id, prefix string, values map[string]string) error {
	f.calls = append(f.calls, "bases")
	if f.bases == nil {
		f.bases = map[string]map[string]string{}
	}
	f.bases[id] = values
	return nil
}

func TestApplySyncWritesProjectThenLabelsThenBases(t *testing.T) {
	it := item(1, "QA Validation")
	it.Priority = "P1"
	p := plan(it, oneBug("status:ready", "prio:p2"), map[string]string{"status": "ready", "prio": "p1"})
	var f fakeSide
	if _, err := ghbridge.ApplySync(&f, &f, &f, p, syncConfig(), syncMeta()); err != nil {
		t.Fatal(err)
	}
	if want := []string{"edit:prio", "labels", "bases"}; !slices.Equal(f.calls, want) {
		t.Errorf("calls %v, want %v", f.calls, want)
	}
}

func TestApplySyncNeverWritesProjectUnlessEnabled(t *testing.T) {
	cfg := syncConfig()
	cfg.GithubProject.Write = false
	p := ghbridge.BuildSyncPlan([]ghbridge.Item{item(1, "Ready")}, oneBug("status:in-review"),
		map[string]map[string]string{"bug1": {"status": "ready"}}, syncMeta(), cfg)
	var f fakeSide
	skipped, err := ghbridge.ApplySync(&f, &f, &f, p, cfg, syncMeta())
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

type failingEdit struct{ fakeSide }

func (f *failingEdit) Edit(string, ghbridge.ProjectEdit) error { return errors.New("gh: boom") }

func TestApplySyncStopsAnIssueOnFailedEdit(t *testing.T) {
	p := plan(item(1, "Ready"), oneBug("status:in-review"), map[string]string{"status": "ready"})
	var f failingEdit
	if _, err := ghbridge.ApplySync(&f, &f, &f, p, syncConfig(), syncMeta()); err == nil {
		t.Fatal("no error")
	}
	if slices.Contains(f.calls, "bases") {
		t.Errorf("base recorded after a failed edit: %v", f.calls)
	}
}

func TestResolveKeepsOneSide(t *testing.T) {
	it := item(1, "QA Validation")
	bugs := oneBug("status:in-review")
	bases := map[string]string{"status": "ready"}
	c := plan(it, bugs, bases).Conflicts[0]

	ours, err := ghbridge.Resolve(c, true, it, bugs[0], syncMeta(), syncConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(ours.Edits) != 1 || ours.Edits[0].Arg != "o-review" || len(ours.Changes[0].Added) != 0 {
		t.Errorf("ours: edits %+v change %+v", ours.Edits, ours.Changes)
	}
	if got := ours.Bases["bug1"]["status"]; got != "in-review" {
		t.Errorf("ours base %q", got)
	}

	theirs, err := ghbridge.Resolve(c, false, it, bugs[0], syncMeta(), syncConfig())
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
	meta := syncMeta()
	meta.Fields["Sprint"].Options["Hypercare"] = "i-h"
	it := item(1, "Ready")
	it.Sprint = &ghbridge.Sprint{Title: "Hypercare"}
	p := ghbridge.BuildSyncPlan([]ghbridge.Item{it}, oneBug("status:ready"), nil, meta, syncConfig())
	if a := p.Changes[0].Added; !slices.Contains(a, "sprint:hypercare") || len(p.Changes[0].Warnings) != 0 {
		t.Errorf("added %v warnings %v", a, p.Changes[0].Warnings)
	}

	// and back: a local sprint named after an iteration is written to it
	it.Sprint = nil
	bases := map[string]map[string]string{"bug1": {"status": "ready", "sprint": ""}}
	p = ghbridge.BuildSyncPlan([]ghbridge.Item{it}, oneBug("status:ready", "sprint:hypercare"), bases, meta, syncConfig())
	if len(p.Edits) != 1 || p.Edits[0].Kind != ghbridge.EditIteration || p.Edits[0].Arg != "i-h" {
		t.Errorf("edits %+v, want the Hypercare iteration", p.Edits)
	}
}

func TestSyncNumericFieldTakesAnyNumber(t *testing.T) {
	v := 3.5
	it := item(1, "Ready")
	it.EstHours = &v
	p := plan(it, oneBug("status:ready"), nil)
	if a := p.Changes[0].Added; !slices.Contains(a, "est:3.5") || len(p.Changes[0].Warnings) != 0 {
		t.Errorf("added %v warnings %v", a, p.Changes[0].Warnings)
	}
}

// A Project value with nothing to slug ("-") isn't read as "cleared".
func TestSyncValueWithAnEmptySlugIsLeftAlone(t *testing.T) {
	it := item(1, "Ready")
	it.Sprint = &ghbridge.Sprint{Title: "-"}
	p := plan(it, oneBug("status:ready", "sprint:sprint-4"), map[string]string{"status": "ready", "sprint": "sprint-4"})
	if len(p.Changes[0].Removed) != 0 || len(p.Changes[0].Warnings) == 0 {
		t.Errorf("removed %v warnings %v, want the sprint left alone with a warning", p.Changes[0].Removed, p.Changes[0].Warnings)
	}
}

// Since they agreed on alice and bob, git-board removed bob and added
// carol: both reach the GitHub issue, the labels stay.
func TestSyncAssigneesWriteBackLocalChanges(t *testing.T) {
	it := item(1, "Ready")
	it.Assignees = []string{"alice", "bob"}
	p := plan(it, oneBug("status:ready", "assignee:alice", "assignee:carol"), map[string]string{"status": "ready", "assignee": "alice,bob"})
	e, ok := assigneeEdit(p)
	if !ok || e.Kind != ghbridge.EditAssignees || e.URL != testURL+"1" ||
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

// Without write = true assignees only come from GitHub, as before.
func TestSyncAssigneesStayLocalWithoutWrite(t *testing.T) {
	cfg := syncConfig()
	cfg.GithubProject.Write = false
	it := item(1, "Ready")
	it.Assignees = []string{"alice", "bob"}
	p := ghbridge.BuildSyncPlan([]ghbridge.Item{it}, oneBug("status:ready", "assignee:alice", "assignee:carol"),
		map[string]map[string]string{"bug1": {"status": "ready", "assignee": "alice,bob"}}, syncMeta(), cfg)
	if e, ok := assigneeEdit(p); ok {
		t.Errorf("edit %+v without write", e)
	}
	if got, changed := p.Bases["bug1"]["assignee"]; changed {
		t.Errorf("base changed to %q, want GitHub's alice,bob kept", got)
	}
}

// GitHub silently drops an assignee without access to the repository: such
// a login stays in git-board, with a warning, and isn't recorded as agreed
// (else the next sync would read GitHub's "no" as a removal).
func TestSyncAssigneesSkipWhoCantBeAssigned(t *testing.T) {
	meta := syncMeta()
	meta.Assignable = map[string][]string{"o/r": {"alice", "bob"}}
	it := item(1, "Ready")
	it.Assignees = []string{"alice"}
	p := ghbridge.BuildSyncPlan([]ghbridge.Item{it}, oneBug("status:ready", "assignee:alice", "assignee:bob", "assignee:dave"),
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
	it := item(1, "Ready")
	it.Category = "FEAT"
	cfg := syncConfig()
	cfg.Branches.Kinds = map[string]string{"bug": "fix", "feat": "feat"}
	p := ghbridge.BuildSyncPlan([]ghbridge.Item{it}, oneBug("FEAT", "bug"), nil, syncMeta(), cfg)
	c := p.Changes[0]
	if !slices.Contains(c.Removed, "FEAT") || slices.Contains(c.Added, "feat") {
		t.Errorf("added %v removed %v, want FEAT removed and no feat beside bug", c.Added, c.Removed)
	}
}
