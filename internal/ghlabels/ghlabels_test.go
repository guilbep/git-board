package ghlabels

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/guilbep/git-board/internal/config"
)

func TestDesiredCoversFieldOptionsAndSprints(t *testing.T) {
	cfg := &config.Config{
		Fields: map[string]config.Field{
			"status":   {Single: true, Options: []string{"backlog", "ready"}},
			"prio":     {Single: true, Options: []string{"p0"}},
			"assignee": {Single: false}, // no Options -- free-form, no label to create
		},
		Sprints: config.SprintsConfig{
			List: []config.Sprint{{Slug: "delivery"}, {Slug: "post-delivery"}},
		},
	}

	got := Desired(cfg)
	want := []string{
		"prio:p0",
		"sprint:delivery",
		"sprint:post-delivery",
		"status:backlog",
		"status:ready",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Desired = %v, want %v", got, want)
	}
}

func TestDesiredEmptyConfigIsEmpty(t *testing.T) {
	if got := Desired(&config.Config{}); len(got) != 0 {
		t.Errorf("Desired(empty config) = %v, want empty", got)
	}
}

func TestMissingFromReturnsOnlyAbsentLabels(t *testing.T) {
	desired := []string{"prio:p0", "prio:p1", "status:ready"}
	existing := []string{"status:ready", "bug", "feature"}

	got := MissingFrom(desired, existing)
	want := []string{"prio:p0", "prio:p1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MissingFrom = %v, want %v", got, want)
	}
}

func TestMissingFromNothingMissingReturnsEmpty(t *testing.T) {
	desired := []string{"prio:p0"}
	existing := []string{"prio:p0", "extra"}

	if got := MissingFrom(desired, existing); len(got) != 0 {
		t.Errorf("MissingFrom = %v, want empty", got)
	}
}

func TestTemplatesReadsEveryLabelsShape(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{
		"a.yaml":     "labels: [\"enhancement\", \"triage\"]\n",
		"b.md":       "---\nname: B\nlabels: bug, needs-triage\n---\nBody\n",
		"c.md":       "---\nlabels:\n  - feat\n---\n",
		"d.md":       "no front matter, no labels\n",
		"e.md":       "--- \nlabels:\n  - specs\n---\n", // co2's specs.md opens with "--- "
		"config.yml": "blank_issues_enabled: false\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got, err := Templates(dir)
	if err != nil {
		t.Fatalf("Templates: %v", err)
	}
	want := []Template{
		{File: "a.yaml", Labels: []string{"enhancement", "triage"}},
		{File: "b.md", Labels: []string{"bug", "needs-triage"}},
		{File: "c.md", Labels: []string{"feat"}},
		{File: "e.md", Labels: []string{"specs"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Templates = %+v, want %+v", got, want)
	}
}
