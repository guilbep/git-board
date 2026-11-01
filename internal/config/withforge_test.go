package config

import "testing"

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
