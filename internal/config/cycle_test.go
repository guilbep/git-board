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
		Plans: "docs/src/implementation-plans", PlansInPR: true,
		TestFiles: []string{"*_test.go", "*.test.ts", "*.spec.ts", "test_*.py", "tests/**"},
		Criteria:  []string{"Acceptance criteria", "Done when", "Expected"},
		Kinds:     map[string]string{"bug": "failing-test", "feat": "per-criterion", "doc": "none", "chore": "none"},
		Roles: map[string]string{"risk": "pm", "plan": "lead-dev", "review": "reviewer",
			"signoff:ready-for-validation": "lead-dev", "signoff:qa": "pm", "signoff:tests-metier": "metier"},
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
	if !s.Guessed || s.Ready != "ready" || s.Plans != "docs/src/implementation-plans" || len(s.TestFiles) != 5 {
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

func TestCycleProtocol(t *testing.T) {
	t.Parallel()
	cfg, md, err := parse([]byte("[cycle]\ncriteria = [\"Definition of done\"]\n\n[cycle.kinds]\ndoc = \"per-criterion\"\nspike = \"none\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if u := md.Undecoded(); len(u) > 0 {
		t.Errorf("undecoded keys: %v", u)
	}
	s := cfg.Stages()
	if !reflect.DeepEqual(s.Criteria, []string{"Definition of done"}) {
		t.Errorf("criteria = %q", s.Criteria)
	}
	// the file's kinds over the defaults
	for kind, want := range map[string]string{"doc": "per-criterion", "spike": "none", "chore": "none", "bug": "failing-test", "": "per-criterion", "epic": "per-criterion"} {
		if got := s.KindRule(kind); got != want {
			t.Errorf("KindRule(%q) = %q, want %q", kind, got, want)
		}
	}
	// the defaults stay the defaults: a file's kinds never write into them
	if r := (&Config{}).Stages().KindRule("doc"); r != "none" {
		t.Errorf("default doc rule = %q after a file changed it", r)
	}
	_, _, err = parse([]byte("[cycle.kinds]\nbug = \"fails-first\"\n"))
	if err == nil || !strings.Contains(err.Error(), `[cycle.kinds] bug: failing-test, per-criterion, none, not "fails-first"`) {
		t.Errorf("err = %v", err)
	}
	_, _, err = parse([]byte("[cycle]\ncriteria = [\" \"]\n"))
	if err == nil || !strings.Contains(err.Error(), "[cycle] criteria: a heading") {
		t.Errorf("err = %v", err)
	}
}

// plans-in-pr is true unless the file says false, and Settings doesn't
// warn about it as an unknown key.
func TestPlansInPR(t *testing.T) {
	t.Parallel()
	if !(&Config{}).Stages().PlansInPR {
		t.Error("plans-in-pr unset: want true")
	}
	cfg, warnings, err := Check([]byte("[cycle]\nplans-in-pr = false\n"))
	if err != nil || len(warnings) != 0 {
		t.Fatalf("Check: %v %q", err, warnings)
	}
	if cfg.Stages().PlansInPR {
		t.Error("plans-in-pr = false: want false")
	}
}

func TestCycleSuites(t *testing.T) {
	t.Parallel()
	cfg, md, err := parse([]byte(`[cycle]
test-files = ["*_test.go"]
signers = "ci/signers"

[cycle.tests.go]
run = ["go", "test", "./..."]
files = ["**/*.go", "go.mod"]
where = "worktree"
tools = [["go", "version"]]
env = { CGO_ENABLED = "0" }

[cycle.tests.e2e]
run = ["npm", "run", "e2e"]
dir = "web"
where = "ci"

[cycle.checks.vet]
run = ["go", "vet", "./..."]
`))
	if err != nil {
		t.Fatal(err)
	}
	if u := md.Undecoded(); len(u) > 0 {
		t.Errorf("undecoded keys: %v", u)
	}
	c := cfg.Cycle
	want := Suite{Run: []string{"go", "test", "./..."}, Files: []string{"**/*.go", "go.mod"}, Where: "worktree", Tools: [][]string{{"go", "version"}}, Env: map[string]string{"CGO_ENABLED": "0"}}
	if !reflect.DeepEqual(c.Tests["go"], want) || c.Tests["e2e"].Dir != "web" || c.Checks["vet"].Run[1] != "vet" {
		t.Errorf("Cycle = %+v", c)
	}
	if c.SignersFile() != "ci/signers" || (Cycle{}).SignersFile() != ".allowed_signers" {
		t.Errorf("SignersFile = %q", c.SignersFile())
	}
	if s := cfg.Stages(); !reflect.DeepEqual(s.TestFiles, []string{"*_test.go"}) {
		t.Errorf("TestFiles = %v", s.TestFiles)
	}
	// M1's globs under tests: refused, naming the new key
	if _, _, err := parse([]byte("[cycle]\ntests = [\"*.go\"]\n")); err == nil || !strings.Contains(err.Error(), "test-files") {
		t.Error("tests as a list of globs: want an error")
	}
}

func TestCycleSuitesChecked(t *testing.T) {
	t.Parallel()
	for toml, want := range map[string]string{
		"[cycle.checks.vet]\nrun = []\n":                                "[cycle.checks.vet] run: a command",
		"[cycle.tests.go]\nrun = [\"go\"]\nwhere = \"cloud\"\n":         `[cycle.tests.go] where: worktree or ci, not "cloud"`,
		"[cycle.tests.go]\nrun = [\"go\"]\ndir = \"../x\"\n":            `[cycle.tests.go] dir: a directory of the repository, not "../x"`,
		"[cycle.tests.go]\nrun = [\"go\"]\ndir = \"/abs\"\n":            `dir: a directory of the repository`,
		"[cycle.tests.go]\nrun = [\"go\"]\nenv = { \"A B\" = \"1\" }\n": `env: a variable name, not "A B"`,
		"[cycle.tests.go]\nrun = [\"go\"]\ntools = [[]]\n":              "tools: each a command",
		"[cycle.tests.go]\nrun = [\"go\"]\nfiles = [\":(exclude)x\"]\n": "files: a glob",
		"[cycle.tests.\"a b\"]\nrun = [\"go\"]\n":                       "[cycle.tests.a b]: a name of letters",
		"[cycle]\nsigners = \"../keys\"\n":                              "[cycle] signers: a path in the repository",
	} {
		if _, _, err := parse([]byte(toml)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", toml, err, want)
		}
	}
}
