package config

import (
	"strings"
	"testing"
)

// WithForge keeps the mappings' subtables: upsertTable's span ends at the
// next header line, a subtable's included, so a re-saved forge table
// leaves [forges.gh.fields] and its values alone.
func TestWithForgeKeepsTheFieldsSubtables(t *testing.T) {
	src := `# mine
[forges.gh]
kind = "github"
project = "o/r"
sync = true

# where each field lives
[forges.gh.fields]
status = { to = "project", field = "Status" }

[forges.gh.fields.status.values]
in-progress = "In Progress"

[fields.status]
options = ["todo", "in-progress"]
`
	f := Forge{Name: "gh", Kind: "github", Project: "o/r", Host: "github.com", Sync: false}
	text := WithForge(src, f)
	for _, want := range []string{`[forges.gh.fields]`, `status = { to = "project", field = "Status" }`, `[forges.gh.fields.status.values]`, `in-progress = "In Progress"`, `# where each field lives`} {
		if !contains(text, want) {
			t.Errorf("WithForge lost %q:\n%s", want, text)
		}
	}
	cfg, _, err := parse([]byte(text))
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, text)
	}
	gh, _ := cfg.Forge("gh")
	if gh.Fields["status"].To != TargetProject || gh.Fields["status"].Field != "Status" {
		t.Errorf("mapping = %+v", gh.Fields["status"])
	}
	if got := cfg.FieldTarget(gh, "status"); got != TargetProject {
		t.Errorf("FieldTarget = %q, want project", got)
	}
	// the values came through as the field's on this forge
	if got := cfg.FieldValues(gh, "status")["in-progress"]; got != "In Progress" {
		t.Errorf("values = %v", cfg.FieldValues(gh, "status"))
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}

func TestSetForgeFields(t *testing.T) {
	src := `# mine
[forges.gh]
kind = "github"
project = "o/r"
sync = true

[fields.status]
options = ["todo", "in-progress"]
`
	text := SetForgeFieldsText(src, "gh", map[string]FieldMapping{
		"status": {To: TargetProject, Field: "Status", Values: map[string]string{"in-progress": "In Progress"}},
	}, nil)
	for _, want := range []string{`status = { to = "project", field = "Status" }`, "[forges.gh.fields.status.values]", `in-progress = "In Progress"`} {
		if !strings.Contains(text, want) {
			t.Errorf("SetForgeFields lost %q:\n%s", want, text)
		}
	}
	cfg, _, err := parse([]byte(text))
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, text)
	}
	gh, _ := cfg.Forge("gh")
	if gh.Fields["status"].To != TargetProject {
		t.Errorf("mapping = %+v", gh.Fields["status"])
	}
	// a removal takes the subtable and the inline key; the forge stays
	text = SetForgeFieldsText(text, "gh", nil, []string{"status"})
	if strings.Contains(text, "forges.gh.fields.status") || strings.Contains(text, `to = "project"`) {
		t.Errorf("the removal left things behind:\n%s", text)
	}
	cfg2, _, err := parse([]byte(text))
	if err != nil {
		t.Fatalf("parse after removal: %v\n%s", err, text)
	}
	gh2, _ := cfg2.Forge("gh")
	if len(gh2.Fields) != 0 {
		t.Errorf("the mapping is still there: %+v", gh2.Fields)
	}
	if !strings.Contains(text, "[forges.gh]") || !strings.Contains(text, "sync = true") {
		t.Errorf("the forge table lost:\n%s", text)
	}
}

// The renames cleared, the values subtable goes with them: a stale one
// would keep the old names applying on the sync, and coming back in the
// editor after a reload.
func TestSetForgeFieldsClearsTheValuesSubtable(t *testing.T) {
	src := `[forges.gh]
kind = "github"
project = "o/r"
sync = true

[fields.status]
options = ["todo", "in-progress"]

[forges.gh.fields]
status = { to = "project", field = "Status" }

[forges.gh.fields.status.values]
in-progress = "In Progress"
`
	text := SetForgeFieldsText(src, "gh", map[string]FieldMapping{
		"status": {To: TargetProject, Field: "Status"}, // the renames cleared
	}, nil)
	if strings.Contains(text, "values") || strings.Contains(text, "In Progress") {
		t.Errorf("the cleared renames left their subtable:\n%s", text)
	}
	cfg, _, err := parse([]byte(text))
	if err != nil {
		t.Fatalf("parse: %v\n%s", err, text)
	}
	gh, _ := cfg.Forge("gh")
	if len(gh.Fields["status"].Values) != 0 {
		t.Errorf("values = %v, want none", gh.Fields["status"].Values)
	}
	if gh.Fields["status"].To != TargetProject || gh.Fields["status"].Field != "Status" {
		t.Errorf("mapping = %+v, want the mapping kept", gh.Fields["status"])
	}
}
