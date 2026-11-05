package cycle

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestGuessStages(t *testing.T) {
	for _, tc := range []struct {
		name    string
		options []string
		want    Stages
	}{
		{"co2", co2Options, Stages{
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

func TestStagesGuessed(t *testing.T) {
	s := Config{}.Stages(co2Options)
	if !s.Guessed || s.Ready != "ready" || s.Plans != "docs/src/implementation-plans" || len(s.TestFiles) != 5 {
		t.Errorf("Stages() = %+v", s)
	}
	// one key set: it replaces that part of the guess, the rest is guessed
	s = Config{Validation: []string{"qa"}}.Stages(co2Options)
	if !s.Guessed || !reflect.DeepEqual(s.Validation, []string{"qa"}) || !reflect.DeepEqual(s.Before, []string{"backlog", "comite", "ready-for-validation", "tests-metier"}) {
		t.Errorf("Stages() with validation set = %+v", s)
	}
	if (Config{}).Stages(nil).Closing != "" {
		t.Error("no options, no closing status")
	}
	// no file, or no validation status: lists, never null (the settings
	// page joins them)
	for _, options := range [][]string{nil, {"todo", "done"}} {
		if b, _ := json.Marshal(Config{}.Stages(options)); strings.Contains(string(b), "null") {
			t.Errorf("Stages() = %s", b)
		}
	}
}

func TestStageOf(t *testing.T) {
	s := guessStages(co2Options)
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
