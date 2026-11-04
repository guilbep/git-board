package ghbridge_test

import (
	"os"
	"slices"
	"strconv"
	"testing"

	"github.com/guilbep/git-board/core/fields"
	"github.com/guilbep/git-board/core/model"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/ghbridge"
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

// bug is a small helper to build a model.Issue fixture keyed by the
// GitHub issue number, mirroring the fixture's real content.url shape.
func bug(id string, number int, labels ...string) model.Issue {
	return model.Issue{
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

// The assignees live where their field's mapping says: with the assignee
// field on a label, the Project's assignees move nothing — default (no
// mapping) they merge as ever.
// bridge pull reads the Project the way bridge sync does: option aliases,
// any number for a numeric field, sprints outside the list by their title.
// An earlier bridge wrote Category verbatim (FEAT): a sync would send it
// as feat, beside a bug label, or create FEAT on the forge. It goes, and
// the kind is filled only where there is none.
// A label GitHub has stays, whatever its case: the forge resolves FEAT to
// its feat, and a sync sends the whole set.
// A field the mappings send to the Project syncs there by the mapping's
// column and value names, extractor or no: risk is nobody's known field,
// the item's raw value comes as gh lists it, by column name.

// A field the mappings send to the Project syncs there by the mapping's
// column and value names, extractor or no: risk is nobody's known field,
// the item's raw value comes as gh lists it, by column name.
func TestMappingSendsAnyFieldToTheProject(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		Forges: []config.Forge{{Name: "gh", Kind: "github", Host: "github.com",
			Fields: map[string]config.FieldMapping{
				"risk": {To: "project", Field: "Risk", Values: map[string]string{"structural": "Structural"}},
			}}},
		Schema: fields.Schema{Fields: map[string]fields.Field{
			"risk": {Single: true, Options: []string{"normal", "structural"}},
		}},
	}
	meta := ghbridge.ProjectMeta{ID: "P",
		Fields: map[string]ghbridge.ProjectField{"Risk": {ID: "F-risk", Options: map[string]string{"Structural": "o-structural", "Normal": "o-normal"}}}}
	item := func(risk string) []ghbridge.Item {
		return []ghbridge.Item{{Extra: map[string]string{"Risk": risk},
			Content: ghbridge.Content{Number: 9, Type: "Issue", URL: bug("b9", 9).GithubURL}}}
	}

	plan := ghbridge.BuildSyncPlan(item("Structural"), []model.Issue{bug("b9", 9)}, nil, meta, cfg)
	if len(plan.Changes) != 1 || !slices.Contains(plan.Changes[0].Added, "risk:structural") {
		t.Errorf("changes = %+v, want risk:structural — the Project's Structural read back", plan.Changes)
	}
	// and the other way: git-board's value written by the mapping's names.
	// The Project still reads Normal, the base agrees, the issue moved to
	// structural: the edit takes the mapping's rename and option id.
	plan = ghbridge.BuildSyncPlan(item("Normal"),
		[]model.Issue{bug("b9", 9, "risk:structural")}, map[string]map[string]string{"b9": {"risk": "normal"}}, meta, cfg)
	if len(plan.Edits) != 1 || plan.Edits[0].Kind != ghbridge.EditOption || plan.Edits[0].Arg != "o-structural" {
		t.Errorf("edits = %+v, want risk:normal → Structural", plan.Edits)
	}
}
