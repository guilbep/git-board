package config

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/guilbep/git-board/core/fields"
)

// saveTwice is text saved twice through save: the second save changes
// nothing, or the writer isn't byte-stable.
func saveTwice(t *testing.T, save func(string) (string, error)) string {
	t.Helper()
	once, err := save(Default)
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	twice, err := save(once)
	if err != nil {
		t.Fatalf("save twice: %v", err)
	}
	if once != twice {
		t.Errorf("not byte-stable:\nfirst  %q\nsecond %q", once, twice)
	}
	return once
}

func TestSetFieldsKeepsTheOtherLines(t *testing.T) {
	t.Parallel()
	before := "# the workflow, in order\n[fields.status]\nsingle = true # one at a time\noptions = [\"backlog\", \"ready\"]\n\n[fields.est]\nnumeric = true\noptions = [\"1\", \"2\"]\n\n[views.open]\nlayout = \"table\"\n"
	after, err := SetFieldsText(before, map[string]fields.Field{"status": {Single: true, Options: []string{"backlog", "ready", "done"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "# the workflow, in order\n[fields.status]\nsingle = true # one at a time\noptions = [\"backlog\", \"ready\", \"done\"]\n\n[fields.est]\nnumeric = true\noptions = [\"1\", \"2\"]\n\n[views.open]\nlayout = \"table\"\n"
	if after != want {
		t.Errorf("got %q, want %q", after, want)
	}
}

func TestSetFieldsWritesWholeAndRemoves(t *testing.T) {
	t.Parallel()
	before := "# the workflow\n[fields.status]\nsingle = true\noptions = [\"backlog\"]\n\n[fields.status.colors]\nbacklog = \"grey\"\n\n[fields.kind.descriptions]\nbug = \"broken\"\n\n[views.open]\nlayout = \"table\"\n"
	// kind goes: its table, its comment-adjacent subtables, nothing else
	after, err := SetFieldsText(before, map[string]fields.Field{
		"status": {Single: true, Options: []string{"backlog", "ready"}, Colors: map[string]string{"ready": "teal"}, Project: map[string]string{"wont": "Won't"}},
	}, []string{"kind"})
	if err != nil {
		t.Fatal(err)
	}
	want := "# the workflow\n[fields.status]\nsingle = true\noptions = [\"backlog\", \"ready\"]\n\n[fields.status.project]\nwont = \"Won't\"\n\n[fields.status.colors]\nready = \"teal\"\n\n[views.open]\nlayout = \"table\"\n"
	if after != want {
		t.Errorf("got\n%s\nwant\n%s", after, want)
	}
	if cfg, _, err := Check([]byte(after)); err != nil || len(cfg.Fields) != 1 {
		t.Errorf("check: %v, %d fields", err, len(cfg.Fields))
	}
}

func TestMoveFieldTakesTheSubtablesAndTheComment(t *testing.T) {
	t.Parallel()
	before := "# the fields\n\n# the workflow, in order\n[fields.status]\nsingle = true\n\n[fields.status.colors]\nready = \"green\"\n\n# the estimates\n[fields.est]\nnumeric = true\n\n[views.open]\nlayout = \"table\"\n"
	after, err := MoveFieldText(before, "est", "status")
	if err != nil {
		t.Fatal(err)
	}
	want := "# the fields\n\n# the estimates\n[fields.est]\nnumeric = true\n\n# the workflow, in order\n[fields.status]\nsingle = true\n\n[fields.status.colors]\nready = \"green\"\n\n[views.open]\nlayout = \"table\"\n"
	if after != want {
		t.Errorf("got\n%s\nwant\n%s", after, want)
	}
	cfg, _, err := Check([]byte(after))
	if err != nil {
		t.Errorf("check: %v", err)
	} else if got, want := cfg.FieldOrder(), []string{"est", "status"}; !slices.Equal(got, want) {
		t.Errorf("FieldOrder = %v, want %v", got, want)
	}
}

func TestParseFieldOrderIsTheFileOrder(t *testing.T) {
	t.Parallel()
	text := "[fields.status]\nsingle = true\n\n[fields.est]\nnumeric = true\n\n[fields.kind]\n"
	cfg, _, err := parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.FieldOrder(), []string{"status", "est", "kind"}; !slices.Equal(got, want) {
		t.Errorf("FieldOrder = %v, want %v", got, want)
	}
	// one the file lost track of (never seen): sorted
	text = "[fields.status]\n"
	cfg, _, err = parse([]byte(text))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := cfg.FieldOrder(), []string{"status"}; !slices.Equal(got, want) {
		t.Errorf("FieldOrder = %v, want %v", got, want)
	}
}

func TestSetFieldsAddsAFieldAtTheEnd(t *testing.T) {
	t.Parallel()
	before := "[views.open]\nlayout = \"table\"\n"
	after, err := SetFieldsText(before, map[string]fields.Field{"prio": {Single: true, Options: []string{"p0", "p1"}, Descriptions: map[string]string{"p0": "now"}}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	want := "[views.open]\nlayout = \"table\"\n\n[fields.prio]\nsingle = true\noptions = [\"p0\", \"p1\"]\n\n[fields.prio.descriptions]\np0 = \"now\"\n"
	if after != want {
		t.Errorf("got\n%s\nwant\n%s", after, want)
	}
}

func TestSetFieldsOnTheDefaultFile(t *testing.T) {
	t.Parallel()
	after := saveTwice(t, func(text string) (string, error) {
		return SetFieldsText(text, map[string]fields.Field{
			"hold": {Single: true, Options: []string{"blocked", "paused"}, Descriptions: map[string]string{
				"blocked": "waiting for something outside the team, or for another issue",
				"paused":  "deprioritized, or its developer away for a while",
			}},
		}, nil)
	})
	// the default's [fields.hold] holds an inline-free table already: its
	// lines are rewritten in place, the comment above stays
	if !strings.Contains(after, "# On hold, beside the status, never a status: blocked, or paused.\n[fields.hold]") {
		t.Errorf("the comment above [fields.hold] went:\n%s", after)
	}
	// the moscow field's inline project table is untouched here
	if !strings.Contains(after, `project = { wont = "Won't" }`) {
		t.Errorf("the inline project table went:\n%s", after)
	}
}

func TestSetRoles(t *testing.T) {
	t.Parallel()
	before := "# the project's people by role\n[roles]\nlead-dev = \"bob\"\npm = \"claire\"\n\n[views.open]\nlayout = \"table\"\n"
	after, err := SetRolesText(before, map[string]string{"lead-dev": "alice", "reviewer": "dan"})
	if err != nil {
		t.Fatal(err)
	}
	want := "# the project's people by role\n[roles]\nlead-dev = \"alice\"\nreviewer = \"dan\"\n\n[views.open]\nlayout = \"table\"\n"
	if after != want {
		t.Errorf("got %q, want %q", after, want)
	}
	none, err := SetRolesText(before, nil)
	if err != nil {
		t.Fatal(err)
	}
	if none != "[views.open]\nlayout = \"table\"\n" {
		t.Errorf("no roles left the table behind:\n%s", none)
	}
	if _, err := SetRolesText(before, map[string]string{"lead.dev": "x"}); err == nil {
		t.Errorf("a dotted role name was taken")
	}
}

func TestSetLabels(t *testing.T) {
	t.Parallel()
	before := "[labels]\nlist = [\"needs-design\"]\n\n[labels.colors]\nneeds-design = \"amber\"\n\n[views.open]\nlayout = \"table\"\n"
	after, err := SetLabelsText(before, fields.Labels{List: []string{"regression", "security"}, Descriptions: map[string]string{"regression": "came back"}})
	if err != nil {
		t.Fatal(err)
	}
	want := "[labels]\nlist = [\"regression\", \"security\"]\n\n[labels.descriptions]\nregression = \"came back\"\n\n[views.open]\nlayout = \"table\"\n"
	if after != want {
		t.Errorf("got\n%s\nwant\n%s", after, want)
	}
	// an empty list stays explicit when the table is there
	empty, err := SetLabelsText(before, fields.Labels{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(empty, "list = []") {
		t.Errorf("an empty list went quiet:\n%s", empty)
	}
	// no table, nothing to hold: untouched
	untouched, err := SetLabelsText("[views.open]\nlayout = \"table\"\n", fields.Labels{})
	if err != nil {
		t.Fatal(err)
	}
	if untouched != "[views.open]\nlayout = \"table\"\n" {
		t.Errorf("a file without [labels] grew one:\n%s", untouched)
	}
}

func TestSetSprints(t *testing.T) {
	t.Parallel()
	d := func(s string) time.Time { t2, _ := time.Parse("2006-01-02", s); return t2 }
	before := "# sprints are sprint: labels\n[sprints]\nlength_days = 14\n\n[[sprints.list]]\nslug = \"s11\"\ntitle = \"Sprint 11\"\nstart = 2026-09-21\n\n[views.open]\nlayout = \"table\"\n"
	after, err := SetSprintsText(before, fields.Sprints{LengthDays: 14, List: []fields.Sprint{
		{Slug: "s12", Title: "Sprint 12", Start: d("2026-10-05")},
		{Slug: "s13", Title: "Sprint 13", Start: d("2026-10-19")},
	}})
	if err != nil {
		t.Fatal(err)
	}
	want := "# sprints are sprint: labels\n[sprints]\nlength_days = 14\n\n[[sprints.list]]\nslug = \"s12\"\ntitle = \"Sprint 12\"\nstart = 2026-10-05\n\n[[sprints.list]]\nslug = \"s13\"\ntitle = \"Sprint 13\"\nstart = 2026-10-19\n\n[views.open]\nlayout = \"table\"\n"
	if after != want {
		t.Errorf("got\n%s\nwant\n%s", after, want)
	}
	cfg, _, err := Check([]byte(after))
	if err != nil || len(cfg.Sprints.List) != 2 {
		t.Errorf("check: %v, %d sprints", err, len(cfg.Sprints.List))
	}
	none, err := SetSprintsText(before, fields.Sprints{LengthDays: 14})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(none, "sprints.list") {
		t.Errorf("the entries stayed:\n%s", none)
	}
}

func TestSetBranches(t *testing.T) {
	t.Parallel()
	before := "# branch names\n[branches]\nbase = \"main\"\nname = \"{number}/{n}-{slug}\" # the default\n\n[branches.kinds]\nbug = \"bug\"\n\n[views.open]\nlayout = \"table\"\n"
	after, err := SetBranchesText(before, Branches{Base: "trunk", Name: "{kind}/{slug}", Kinds: map[string]string{"bug": "bug", "feat": "feat"}})
	if err != nil {
		t.Fatal(err)
	}
	want := "# branch names\n[branches]\nbase = \"trunk\"\nname = \"{kind}/{slug}\" # the default\n\n[branches.kinds]\nbug = \"bug\"\nfeat = \"feat\"\n\n[views.open]\nlayout = \"table\"\n"
	if after != want {
		t.Errorf("got\n%s\nwant\n%s", after, want)
	}
	none, err := SetBranchesText(before, Branches{})
	if err != nil {
		t.Fatal(err)
	}
	if none != "[views.open]\nlayout = \"table\"\n" {
		t.Errorf("no branches left the table behind:\n%s", none)
	}
	afterDefault := saveTwice(t, func(text string) (string, error) {
		return SetBranchesText(text, Branches{Kinds: map[string]string{"bug": "bug"}})
	})
	if !strings.Contains(afterDefault, "[branches.kinds]\nbug = \"bug\"") {
		t.Errorf("the default's inline kinds didn't come back as a table:\n%s", afterDefault)
	}
}

func TestSetCycle(t *testing.T) {
	t.Parallel()
	before := "[fields.status]\noptions = [\"backlog\"]\n"
	off, err := SetCycleText(before, false, Cycle{})
	if err != nil {
		t.Fatal(err)
	}
	if off != before {
		t.Errorf("off changed the file:\n%s", off)
	}
	on, err := SetCycleText(before, true, Cycle{Ready: "ready", InProgress: []string{"in-progress"}, Risk: "pm", Kinds: map[string]string{"bug": FailingTest}})
	if err != nil {
		t.Fatal(err)
	}
	want := "[fields.status]\noptions = [\"backlog\"]\n\n[cycle]\nin-progress = [\"in-progress\"]\nready = \"ready\"\nrisk = \"pm\"\n\n[cycle.kinds]\nbug = \"failing-test\"\n"
	if on != want {
		t.Errorf("got\n%s\nwant\n%s", on, want)
	}
	if cfg, _, err := Check([]byte(on)); err != nil || !cfg.Cycle.On {
		t.Errorf("check: %v, on %v", err, cfg != nil && cfg.Cycle.On)
	}
	// a suite with its env and tools, byte-stable
	suite := Cycle{Tests: map[string]Suite{"unit": {
		Run:   []string{"go", "test", "./..."},
		Files: []string{"**/*.go"},
		Env:   map[string]string{"CGO": "0"},
		Tools: [][]string{{"go", "version"}},
	}}}
	withSuite, err := SetCycleText(before, true, suite)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(withSuite, "run = [\"go\", \"test\", \"./...\"]") || !strings.Contains(withSuite, "tools = [[\"go\", \"version\"]]") {
		t.Errorf("the suite didn't come out:\n%s", withSuite)
	}
	if !strings.Contains(withSuite, "[cycle.tests.unit.env]\nCGO = \"0\"") {
		t.Errorf("the suite's env:\n%s", withSuite)
	}
	twice, err := SetCycleText(withSuite, true, suite)
	if err != nil {
		t.Fatal(err)
	}
	if twice != withSuite {
		t.Errorf("not byte-stable:\nfirst\n%s\nsecond\n%s", withSuite, twice)
	}
	// a suite dropped from the map goes from the file
	gone, err := SetCycleText(withSuite, true, Cycle{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(gone, "cycle.tests") {
		t.Errorf("the dropped suite stayed:\n%s", gone)
	}
}

func TestSetCycleOnTheDefaultFile(t *testing.T) {
	t.Parallel()
	cfg, _, err := Check([]byte(Default))
	if err != nil {
		t.Fatal(err)
	}
	after := saveTwice(t, func(text string) (string, error) { return SetCycleText(text, true, cfg.Cycle) })
	if !strings.Contains(after, "[cycle]\n") {
		t.Errorf("[cycle] went:\n%s", after)
	}
	if _, warns, err := Check([]byte(after)); err != nil || len(warns) > 0 {
		t.Errorf("check: %v %v", err, warns)
	}
}

func TestSetReposUploads(t *testing.T) {
	t.Parallel()
	before := "[fields.status]\noptions = [\"backlog\"]\n"
	repos, err := SetReposText(before, []string{"epfl-enac/one", "epfl-enac/two"}, "labels")
	if err != nil {
		t.Fatal(err)
	}
	want := "[fields.status]\noptions = [\"backlog\"]\n\n[repos]\nlist = [\"epfl-enac/one\", \"epfl-enac/two\"]\nassignees = \"labels\"\n"
	if repos != want {
		t.Errorf("got\n%s\nwant\n%s", repos, want)
	}
	gone, err := SetReposText(repos, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if gone != before {
		t.Errorf("no repos left the table behind:\n%s", gone)
	}
	ups, err := SetUploadsText(before, Uploads{Prefix: "gb/x", Region: "us-east-1"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ups, "prefix = \"gb/x\"") {
		t.Errorf("uploads:\n%s", ups)
	}
	upsgone, err := SetUploadsText(ups, Uploads{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(upsgone, "uploads") {
		t.Errorf("the defaults left the table behind:\n%s", upsgone)
	}
}

func TestSetRelease(t *testing.T) {
	t.Parallel()
	before := "[fields.status]\noptions = [\"backlog\"]\n"
	r := Release{
		Environments: []string{"dev", "stage", "prod"},
		Checks:       "make ci",
		Images:       []Image{{Name: "board", Context: "."}},
		Chart:        &Chart{Path: "chart", Registry: "registry.epfl.ch/x"},
		Deploy: []Target{
			{Repo: "epfl-enac/app-deploy", Path: "apps/app/{env}", Registry: "registry.epfl.ch/x"},
			{Repo: "epfl-enac/app-deploy", Path: "apps/app/{env}", Registry: "registry.epfl.ch/y"},
		},
	}
	after, err := SetReleaseText(before, r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(after, "environments = [\"dev\", \"stage\", \"prod\"]") ||
		!strings.Contains(after, "images = [{ name = \"board\", context = \".\" }]") ||
		!strings.Contains(after, "[release.chart]\npath = \"chart\"") {
		t.Errorf("release:\n%s", after)
	}
	if strings.Count(after, "[[release.deploy]]") != 2 || !strings.Contains(after, "repo = \"epfl-enac/app-deploy\"") {
		t.Errorf("deploy:\n%s", after)
	}
	twice, err := SetReleaseText(after, r)
	if err != nil {
		t.Fatal(err)
	}
	if twice != after {
		t.Errorf("not byte-stable:\nfirst\n%s\nsecond\n%s", after, twice)
	}
	if cfg, _, err := Check([]byte(after)); err != nil || len(cfg.Release.Environments) != 3 {
		t.Errorf("check: %v", err)
	}
	// one target left, one gone
	r.Deploy = r.Deploy[:1]
	only, err := SetReleaseText(after, r)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(only, "[[release.deploy]]") != 1 {
		t.Errorf("the dropped target stayed:\n%s", only)
	}
	none, err := SetReleaseText(after, Release{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(none, "release") {
		t.Errorf("no release left tables behind:\n%s", none)
	}
}

func TestDeleteAndMoveViews(t *testing.T) {
	t.Parallel()
	before := "# views are the tabs\n[views.open]\nlayout = \"table\"\n\n[views.backlog]\n# the backlog, ready first\nfilter = \"is:open\"\n\n[views.mine]\nlayout = \"board\"\n"
	gone, err := DeleteViewText(before, "backlog")
	if err != nil {
		t.Fatal(err)
	}
	want := "# views are the tabs\n[views.open]\nlayout = \"table\"\n\n[views.mine]\nlayout = \"board\"\n"
	if gone != want {
		t.Errorf("delete:\ngot      %q\nwant     %q", gone, want)
	}
	moved, err := MoveViewText(before, "mine", "open")
	if err != nil {
		t.Fatal(err)
	}
	wantMoved := "# views are the tabs\n[views.mine]\nlayout = \"board\"\n\n[views.open]\nlayout = \"table\"\n\n[views.backlog]\n# the backlog, ready first\nfilter = \"is:open\"\n"
	if moved != wantMoved {
		t.Errorf("move:\ngot\n%s\nwant\n%s", moved, wantMoved)
	}
	last, err := MoveViewText(before, "open", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(last, "[views.open]\nlayout = \"table\"\n") {
		t.Errorf("move to the end:\n%s", last)
	}
	cfg, _, err := Check([]byte(moved))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.ViewNames(); !slices.Equal(got, []string{"mine", "open", "backlog"}) {
		t.Errorf("the file's order is the tabs': %v", got)
	}
	if _, err := MoveViewText(before, "nope", "open"); err == nil {
		t.Errorf("moving a view there isn't")
	}
}

// A key the form cleared goes: single unticked, the options emptied, the
// type unset; a sprint's length and weekday back to their defaults.
func TestSectionWritersClearKeys(t *testing.T) {
	t.Parallel()
	text := "[fields.a]\nsingle = true\noptions = [\"x\"]\ntype = \"select\"\n# kept\n"
	got, err := SetFieldsText(text, map[string]fields.Field{"a": {}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != "[fields.a]\n# kept\n" {
		t.Errorf("fields:\n%q", got)
	}
	text = "[sprints]\nlength_days = 7\nstart_weekday = \"monday\"\n"
	if got, err = SetSprintsText(text, fields.Sprints{}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "length_days") || strings.Contains(got, "start_weekday") {
		t.Errorf("sprints:\n%q", got)
	}
	if got, _ = SetSprintsText("", fields.Sprints{}); strings.Contains(got, "[sprints]") {
		t.Errorf("an empty table made:\n%q", got)
	}
}

// Moving a field the file has no table for is refused, the text untouched.
func TestMoveFieldTextRefusesAMissingField(t *testing.T) {
	t.Parallel()
	text := "top = 1\n\n[fields.a]\nsingle = true\n"
	if _, err := MoveFieldText(text, "nope", "a"); err == nil {
		t.Error("moving a missing field: no error")
	}
	if _, err := MoveFieldText(text, "a", "nope"); err == nil {
		t.Error("moving above a missing field: no error")
	}
}

// Deleting a table keeps the comment over the next one.
func TestDeleteTableKeepsTheNextComment(t *testing.T) {
	t.Parallel()
	got := deleteTable("# a's\n[fields.a]\nsingle = true\n\n# b is the status\n[fields.b]\nsingle = true\n", "fields.a")
	if got != "# b is the status\n[fields.b]\nsingle = true\n" {
		t.Errorf("%q", got)
	}
}

// follow = "" (no environment follows the trunk) stays: absent means dev.
func TestSetReleaseTextKeepsAnEmptyFollow(t *testing.T) {
	t.Parallel()
	none := ""
	got, err := SetReleaseText("", Release{Environments: []string{"dev", "prod"}, Follow: &none})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, `follow = ""`) {
		t.Errorf("%q", got)
	}
}

// The flat kinds' field, with no table, doesn't make the fields' order
// alphabetical: the file's order holds, the kinds after it.
func TestFieldOrderKeepsTheFileWithFlatKinds(t *testing.T) {
	t.Parallel()
	cfg, _, err := Check([]byte("[fields.status]\noptions = [\"a\"]\n\n[fields.prio]\noptions = [\"p0\"]\n\n[branches.kinds]\nbug = \"bug\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.FieldOrder(); !slices.Equal(got, []string{"status", "prio", "kind"}) {
		t.Errorf("%v", got)
	}
}
