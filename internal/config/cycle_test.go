package config

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

var co2Statuses = []string{"backlog", "comite", "ready", "in-progress", "in-review", "ready-for-validation", "qa", "tests-metier", "validated"}

func TestGuessStages(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options []string
		want    Stages
	}{
		{"co2", co2Statuses, Stages{
			Before: []string{"backlog", "comite"}, Ready: "ready",
			InProgress: []string{"in-progress"}, InReview: []string{"in-review"},
			Validation: []string{"ready-for-validation", "qa", "tests-metier"}, Closing: "validated",
		}},
		{"git-board", []string{"draft", "backlog", "ready", "in-progress", "in-review", "done"}, Stages{
			Before: []string{"draft", "backlog"}, Ready: "ready",
			InProgress: []string{"in-progress"}, InReview: []string{"in-review"}, Closing: "done",
		}},
		{"short words", []string{"todo", "doing", "review", "done"}, Stages{
			Ready: "todo", InProgress: []string{"doing"}, InReview: []string{"review"}, Closing: "done",
		}},
		{"two readies", []string{"ready", "ready-to-start", "done"}, Stages{
			Before: []string{"ready-to-start"}, Ready: "ready", Closing: "done",
		}},
		{"none", nil, Stages{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := guessStages(tc.options); !reflect.DeepEqual(got, tc.want) {
				t.Errorf("guessStages(%v)\n got %+v\nwant %+v", tc.options, got, tc.want)
			}
		})
	}
}

const co2Cycle = `
[fields.status]
options = ["backlog", "comite", "ready", "in-progress", "in-review", "ready-for-validation", "qa", "tests-metier", "validated"]
single = true

[fields]
risk = { options = ["low", "normal", "structural"], single = true }

[cycle]
ready = "ready"
in-progress = ["in-progress"]
in-review = ["in-review"]
validation = ["ready-for-validation", "qa", "tests-metier"]
plans = "docs/src/implementation-plans"
risk = "pm"
plan = "lead-dev"
review = "reviewer"

[cycle.signoffs]
ready-for-validation = "lead-dev"
qa = "pm"
tests-metier = "metier"
`

func TestCycleFromFile(t *testing.T) {
	cfg, md, err := parse([]byte(co2Cycle))
	if err != nil {
		t.Fatal(err)
	}
	if u := md.Undecoded(); len(u) > 0 {
		t.Errorf("undecoded keys: %v", u)
	}
	s := cfg.Stages()
	want := Stages{
		Before: []string{"backlog", "comite"}, Ready: "ready",
		InProgress: []string{"in-progress"}, InReview: []string{"in-review"},
		Validation: []string{"ready-for-validation", "qa", "tests-metier"}, Closing: "validated",
		Plans: "docs/src/implementation-plans",
		Tests: []string{"*_test.go", "*.test.ts", "*.spec.ts", "test_*.py", "tests/**"},
	}
	if !reflect.DeepEqual(s, want) {
		t.Errorf("Stages()\n got %+v\nwant %+v", s, want)
	}
	if cfg.Cycle.Signoffs["qa"] != "pm" || cfg.Cycle.Plan != "lead-dev" {
		t.Errorf("Cycle = %+v", cfg.Cycle)
	}
}

func TestStagesGuessed(t *testing.T) {
	cfg := &Config{Fields: map[string]Field{"status": {Options: co2Statuses}}}
	s := cfg.Stages()
	if !s.Guessed || s.Ready != "ready" || s.Plans != "docs/src/implementation-plans" || len(s.Tests) != 5 {
		t.Errorf("Stages() = %+v", s)
	}
	// one key set: it replaces that part of the guess, the rest is guessed
	cfg.Cycle.Validation = []string{"qa"}
	s = cfg.Stages()
	if !s.Guessed || !reflect.DeepEqual(s.Validation, []string{"qa"}) || !reflect.DeepEqual(s.Before, []string{"backlog", "comite", "ready-for-validation", "tests-metier"}) {
		t.Errorf("Stages() with validation set = %+v", s)
	}
	if (&Config{}).Stages().Closing != "" {
		t.Error("no options, no closing status")
	}
	// no file, or no validation status: lists, never null (the settings
	// page joins them)
	for _, c := range []*Config{{}, {Fields: map[string]Field{"status": {Options: []string{"todo", "done"}}}}} {
		if b, _ := json.Marshal(c.Stages()); strings.Contains(string(b), "null") {
			t.Errorf("Stages() = %s", b)
		}
	}
}

func TestStageOf(t *testing.T) {
	s := guessStages(co2Statuses)
	for status, want := range map[string]string{
		"qa": "validation", "validated": "done", "comite": "before", "ready": "ready",
		"in-progress": "in-progress", "in-review": "in-review", "nope": "", "": "",
	} {
		if got := s.StageOf(status); got != want {
			t.Errorf("StageOf(%q) = %q, want %q", status, got, want)
		}
	}
	if got := (Stages{}).StageOf(""); got != "" {
		t.Errorf("no statuses: StageOf(\"\") = %q", got)
	}
}

func TestCycleRolesChecked(t *testing.T) {
	if _, _, err := parse([]byte("[cycle]\nrisk = \"pm\"\n")); err != nil {
		t.Errorf("a role nobody holds is anyone's turn: %v", err)
	}
	_, _, err := parse([]byte("[cycle]\nrisk = \"p m\"\n"))
	if err == nil || !strings.Contains(err.Error(), `[cycle] risk: a role name, not "p m"`) {
		t.Errorf("err = %v", err)
	}
	_, _, err = parse([]byte("[cycle.signoffs]\nqa = \"Pm\"\n"))
	if err == nil || !strings.Contains(err.Error(), `[cycle.signoffs] qa: a role name, not "Pm"`) {
		t.Errorf("err = %v", err)
	}
}
