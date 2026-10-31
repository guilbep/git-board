package config

import (
	"slices"
	"strings"
	"testing"
	"time"
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
	after, err := SetFieldsText(before, map[string]Field{"status": {Single: true, Options: []string{"backlog", "ready", "done"}}}, nil)
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
	after, err := SetFieldsText(before, map[string]Field{
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

func TestSetFieldsAddsAFieldAtTheEnd(t *testing.T) {
	t.Parallel()
	before := "[views.open]\nlayout = \"table\"\n"
	after, err := SetFieldsText(before, map[string]Field{"prio": {Single: true, Options: []string{"p0", "p1"}, Descriptions: map[string]string{"p0": "now"}}}, nil)
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
		return SetFieldsText(text, map[string]Field{
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
	after, err := SetLabelsText(before, Labels{List: []string{"regression", "security"}, Descriptions: map[string]string{"regression": "came back"}})
	if err != nil {
		t.Fatal(err)
	}
	want := "[labels]\nlist = [\"regression\", \"security\"]\n\n[labels.descriptions]\nregression = \"came back\"\n\n[views.open]\nlayout = \"table\"\n"
	if after != want {
		t.Errorf("got\n%s\nwant\n%s", after, want)
	}
	// an empty list stays explicit when the table is there
	empty, err := SetLabelsText(before, Labels{})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(empty, "list = []") {
		t.Errorf("an empty list went quiet:\n%s", empty)
	}
	// no table, nothing to hold: untouched
	untouched, err := SetLabelsText("[views.open]\nlayout = \"table\"\n", Labels{})
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
	after, err := SetSprintsText(before, SprintsConfig{LengthDays: 14, List: []Sprint{
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
	none, err := SetSprintsText(before, SprintsConfig{LengthDays: 14})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(none, "sprints.list") {
		t.Errorf("the entries stayed:\n%s", none)
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
