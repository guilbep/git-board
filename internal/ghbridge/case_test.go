package ghbridge

import (
	"testing"

	"github.com/guilbep/git-board/internal/config"
)

// Values spelled as the forge does (P0, Must; docs/prd/board-template.md)
// still match the Project's options both ways: by their slugs.
func TestProjectValuesInTheForgesSpelling(t *testing.T) {
	t.Parallel()
	cfg, _, err := config.Check([]byte(`[fields.prio]
single = true
options = ["P0", "P1"]

[fields.status]
single = true
options = ["backlog", "in-progress"]
`))
	if err != nil {
		t.Fatal(err)
	}
	for raw, want := range map[string]string{"P0": "P0", "p1": "P1"} {
		if v, ok := toLabel(cfg, "prio", raw); !ok || v != want {
			t.Errorf("toLabel(%q) = %q, %v; want %q", raw, v, ok, want)
		}
	}
	if v, ok := toLabel(cfg, "status", "In progress"); !ok || v != "in-progress" {
		t.Errorf("toLabel(In progress) = %q, %v", v, ok)
	}
	if _, id, ok := toProject(cfg, ProjectField{Options: map[string]string{"P0": "a", "P1": "b"}}, "prio", "P1"); !ok || id != "b" {
		t.Errorf("toProject(P1) = %q, %v", id, ok)
	}
}
