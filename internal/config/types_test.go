package config

import (
	"strings"
	"testing"
)

func TestTypeOfAField(t *testing.T) {
	cfg, _, err := Check([]byte(`
[fields.status]
options = ["backlog", "done"]

[fields.est]
numeric = true

[fields.points]
type = "number"
unit = "pt"

[fields.module]
type = "text"

[[sprints.list]]
slug = "s1"
start = 2026-10-05
`))
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name   string
		values []string
		want   string
	}{
		{"status", nil, TypeSelect},                  // options
		{"est", nil, TypeNumber},                     // numeric = true
		{"points", nil, TypeNumber},                  // its own type
		{"module", []string{"2026-10-05"}, TypeText}, // its own type wins over its values
		{"sprint", nil, TypeIteration},               // the name
		{"assignee", nil, TypePerson},                // the name
		{"reviewer", nil, TypePerson},
		{"due", nil, TypeDate},                                    // the name
		{"start", []string{"2026-10-05", "2026-11-02"}, TypeDate}, // its values
		{"parent", nil, TypeIssue},                                // the name
		{"blocked-by", []string{"#12"}, TypeIssue},
		{"duplicate", []string{"#12", "9dcbc62"}, TypeIssue}, // forge numbers and git-bug ids
		{"cat", []string{"feat", "bug"}, TypeSelect},         // anything else
	} {
		if got := cfg.TypeOf(c.name, c.values); got != c.want {
			t.Errorf("TypeOf(%s, %v) = %s, want %s", c.name, c.values, got, c.want)
		}
	}
	if cfg.UnitOf("est") != "h" || cfg.UnitOf("points") != "pt" || cfg.UnitOf("status") != "" {
		t.Errorf("units: est %q points %q status %q", cfg.UnitOf("est"), cfg.UnitOf("points"), cfg.UnitOf("status"))
	}
}

func TestAnUnknownTypeIsRefused(t *testing.T) {
	_, _, err := Check([]byte("[fields.due]\ntype = \"calendar\"\n"))
	if err == nil || !strings.Contains(err.Error(), "iteration") {
		t.Errorf("err %v, want the list of types", err)
	}
}
