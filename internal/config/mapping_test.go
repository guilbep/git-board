package config

import (
	"os"
	"strings"
	"testing"
)

const mappingConfig = `
[fields.status]
single = true
options = ["draft", "backlog", "ready", "in-progress", "in-review", "done"]

[fields.est]
numeric = true
options = ["0.5", "1", "2", "4", "8"]
unit = "h"

[fields.moscow]
single = true
options = ["musthave", "shouldhave", "couldhave", "wonthave"]
aliases = { could = "couldhave" }

[fields.assignee]
type = "person"

[fields.parent]
type = "issue"

[forges.gh]
kind = "github"
project = "o/r"
sync = true

[forges.gh.fields]
status = { to = "project", field = "Status" }
moscow = { to = "label" }

[forges.gh.fields.status.values]
in-progress = "In Progress"

[forges.gl]
kind = "gitlab"
project = "7"
`

func parseMapping(t *testing.T, text string) (*Config, error) {
	t.Helper()
	path := t.TempDir() + "/" + FileName
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return Load(path)
}

func TestFieldTargetTakesTheMappingThenTheDefault(t *testing.T) {
	t.Parallel()
	cfg, err := parseMapping(t, mappingConfig)
	if err != nil {
		t.Fatal(err)
	}
	gh, _ := cfg.Forge("gh")
	gl, _ := cfg.Forge("gl")
	for _, want := range []struct {
		f    Forge
		name string
		to   string
	}{
		{gh, "status", TargetProject},     // the mapping's
		{gh, "est", TargetLabel},          // a number's GitHub default
		{gh, "assignee", TargetAssignees}, // a person's
		{gh, "parent", TargetSubIssues},   // an issue's
		{gl, "status", TargetLabel},       // no mapping, the select default
		{gl, "sprint", TargetMilestone},   // an iteration's GitLab default
	} {
		if got := cfg.FieldTarget(want.f, want.name); got != want.to {
			t.Errorf("FieldTarget(%s, %s) = %q, want %q", want.f.Name, want.name, got, want.to)
		}
	}
}

func TestTheOldProjectKeyReadsAsTheValues(t *testing.T) {
	t.Parallel()
	src := `[fields.size]
numeric = true
options = ["1", "2"]

[fields.size.project]
"1" = "Small"
"2" = "Large"

[forges.gh]
kind = "github"
project = "o/r"
`
	cfg, err := parseMapping(t, src)
	if err != nil {
		t.Fatal(err)
	}
	gh, _ := cfg.Forge("gh")
	// the old map implies the Project target, and reads as the values
	if got := cfg.FieldTarget(gh, "size"); got != TargetProject {
		t.Errorf("FieldTarget = %q, want project", got)
	}
	if cfg.FieldValues(gh, "size")["2"] != "Large" {
		t.Errorf("values = %v", cfg.FieldValues(gh, "size"))
	}
	// a mapping's values win once written
	WithMapping := *cfg
	WithMapping.Forges[0].Fields = map[string]FieldMapping{"size": {To: TargetProject, Field: "Size", Values: map[string]string{"1": "S"}}}
	if got := WithMapping.FieldValues(WithMapping.Forges[0], "size")["1"]; got != "S" {
		t.Errorf("values = %v", WithMapping.FieldValues(WithMapping.Forges[0], "size"))
	}
}

func TestAliasesFoldAnOldValue(t *testing.T) {
	t.Parallel()
	cfg, err := parseMapping(t, mappingConfig)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Fields["moscow"].AliasOf("could"); got != "couldhave" {
		t.Errorf("AliasOf(could) = %q", got)
	}
	if got := cfg.Fields["moscow"].AliasOf("musthave"); got != "musthave" {
		t.Errorf("an option is its own value: %q", got)
	}
	for _, bad := range []string{
		`[fields.x]
options = ["a", "b"]
aliases = { b = "c" }
`,
		`[fields.x]
options = ["a", "b"]
aliases = { a = "b" }
`,
	} {
		if _, err := parseMapping(t, bad); err == nil {
			t.Errorf("parsed:\n%s\nwant a refusal", bad)
		} else if !strings.Contains(err.Error(), "aliases") {
			t.Errorf("%s: %v", bad, err)
		}
	}
}

func TestAMappingThatDoesntFitIsRefused(t *testing.T) {
	t.Parallel()
	for _, bad := range []struct{ text, want string }{
		{`[fields.x]
options = ["a"]
[forges.gh]
kind = "github"
project = "o/r"
[forges.gh.fields.x]
to = "weight"
`, "weight is GitLab's"},
		{`[fields.x]
type = "person"
[forges.gh]
kind = "github"
project = "o/r"
[forges.gh.fields.x]
to = "project"
field = "X"
`, "doesn't live in a Project column"},
		{`[fields.x]
options = ["a"]
[forges.gh]
kind = "github"
project = "o/r"
[forges.gh.fields.x]
to = "scoped-label"
`, "scoped-label is GitLab's"},
		{`[fields.x]
type = "person"
[forges.gl]
kind = "gitlab"
project = "7"
[forges.gl.fields.x]
to = "sub-issues"
`, "sub-issues are GitHub's"},
		{`[fields.x]
options = ["a"]
[forges.gh]
kind = "github"
project = "o/r"
[forges.gh.fields.x]
to = "label"
as = "plain"
`, "as: not applied yet"},
		{`[fields.x]
single = true
options = ["a"]
[forges.gl]
kind = "gitlab"
project = "7"
[forges.gl.fields.x]
as = "maybe"
`, "as: not applied yet"},
		{`[fields.x]
single = true
options = ["a"]
[forges.gh]
kind = "github"
project = "o/r"
[forges.gh.fields.x]
to = "label"
field = "X"
`, "a project's key"},
		{`[fields.x]
single = true
options = ["a", "b"]
[forges.gh]
kind = "github"
project = "o/r"
[forges.gh.fields.x]
to = "project"
field = "X"

[forges.gh.fields.x.values]
c = "C"
`, "not an option"},
		{`[fields.x]
options = ["a"]
[forges.gh]
kind = "github"
project = "o/r"
[forges.gh.fields.y]
to = "label"
`, "no [fields.y]"},
		// the details fit their target, whatever to says: an empty one
		// carries no field key and no renames
		{`[fields.x]
options = ["a"]
[forges.gh]
kind = "github"
project = "o/r"
[forges.gh.fields.x]
field = "X"
`, "field: a project's key, and to is empty"},
		{`[fields.x]
options = ["a"]
[forges.gh]
kind = "github"
project = "o/r"
[forges.gh.fields.x.values]
a = "A"
`, "values: renames of a target, and to is empty"},
	} {
		_, err := parseMapping(t, bad.text)
		if err == nil {
			t.Errorf("parsed:\n%s\nwant a refusal naming %q", bad.text, bad.want)
		} else if !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%s: %v, want it to name %q", bad.text, err, bad.want)
		}
	}
}
