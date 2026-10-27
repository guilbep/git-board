package config

import (
	"encoding/json"
	"reflect"
	"slices"
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
		// read 2: a cell per validation status, verify dropped
		Steps: append(slices.Clone(DefaultSteps[:6]), stepOf("ready-for-validation"), stepOf("qa"), stepOf("tests-metier")),
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
	cfg, _, err := parse([]byte("[cycle]\nspec = \"pm\"\n"))
	if err != nil || cfg.Stages().Roles["spec"] != "pm" {
		t.Errorf("spec: %v %v", err, cfg.Stages().Roles)
	}
	if _, _, err := parse([]byte("[cycle]\nspec = \"P M\"\n")); err == nil || !strings.Contains(err.Error(), "[cycle] spec: a role name") {
		t.Errorf("spec err = %v", err)
	}
	_, _, err = parse([]byte("[cycle]\nrisk = \"p m\"\n"))
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

// co2Status is co2-calculator's status field: the stages are guessed from it.
const co2Status = "[fields.status]\noptions = [\"backlog\", \"comite\", \"ready\", \"in-progress\", \"in-review\", \"ready-for-validation\", \"qa\", \"tests-metier\", \"validated\"]\nsingle = true\n\n"

func stepIDs(steps []Step) []string {
	ids := []string{}
	for _, s := range steps {
		ids = append(ids, s.ID)
	}
	return ids
}

func TestCycleSteps(t *testing.T) {
	t.Parallel()
	six := []string{"intent", "risk", "plan", "tests", "code", "review"}
	for _, tc := range []struct {
		name, toml string
		want       []string
	}{
		{"no steps", co2Status, append(slices.Clone(six), "verify")},
		{"no file", "", append(slices.Clone(six), "verify")},
		// read 2: every validation status a cell, verify gone
		{"sign-offs", co2Status + "[cycle.signoffs]\nqa = \"pm\"\ntests-metier = \"metier\"\n", append(slices.Clone(six), "ready-for-validation", "qa", "tests-metier")},
		{"a key no status names", co2Status + "[cycle.signoffs]\nstaging = \"pm\"\nqa = \"pm\"\n", append(slices.Clone(six), "ready-for-validation", "qa", "tests-metier", "staging")},
		{"code and review", co2Status + "[cycle]\nsteps = [\"code\", \"review\"]\n", []string{"code", "review"}},
		{"verify with a sign-off", co2Status + "[cycle]\nsteps = [\"review\", \"qa\", \"verify\"]\n\n[cycle.signoffs]\nqa = \"pm\"\n", []string{"review", "qa", "verify"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _, err := parse([]byte(tc.toml))
			if err != nil {
				t.Fatal(err)
			}
			if got := stepIDs(cfg.Stages().Steps); !slices.Equal(got, tc.want) {
				t.Errorf("steps = %v, want %v", got, tc.want)
			}
		})
	}
	cfg, _, err := parse([]byte(co2Status + "[cycle]\nsteps = [\"review\", \"qa\", \"verify\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	want := []Step{{ID: "review", Kind: "review", Name: "Review"}, {ID: "qa", Kind: "signoff", Name: "qa", Status: "qa"}, {ID: "verify", Kind: "signoff", Name: "Verify"}}
	if got := cfg.Stages().Steps; !reflect.DeepEqual(got, want) {
		t.Errorf("Steps = %+v, want %+v", got, want)
	}
}

func TestCycleStepsChecked(t *testing.T) {
	t.Parallel()
	for steps, want := range map[string]string{
		`["review", "code"]`: `[cycle] steps: "code" after "review": steps keep the cycle's order, intent, risk, plan, tests, code, review, then the sign-offs`,
		`["qa", "code"]`:     `[cycle] steps: "code" after "qa"`,
		`["code", "deploy"]`: `[cycle] steps: "deploy" is no step: intent, risk, plan, tests, code, review, verify, a validation status or a [cycle.signoffs] key`,
		`["code", "code"]`:   `[cycle] steps: "code" twice`,
		`[]`:                 `[cycle] steps: at least one step`,
	} {
		if _, _, err := parse([]byte(co2Status + "[cycle]\nsteps = " + steps + "\n")); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", steps, err, want)
		}
	}
	for _, steps := range []string{`["intent", "risk", "code", "verify"]`, `["code", "review", "verify"]`, `["tests-metier", "qa"]`} {
		if _, _, err := parse([]byte(co2Status + "[cycle]\nsteps = " + steps + "\n")); err != nil {
			t.Errorf("%s: %v", steps, err)
		}
	}
}

// read 3: a kind's rule as a string or as its table's tests; a kind's
// steps is M3's, a key git-board doesn't know yet.
func TestCycleKindsTable(t *testing.T) {
	t.Parallel()
	cfg, warnings, err := Check([]byte("[cycle.kinds]\nbug = \"failing-test\"\n\n[cycle.kinds.doc]\ntests = \"per-criterion\"\n\n[cycle.kinds.chore]\nsteps = [\"code\"]\n"))
	if err != nil {
		t.Fatal(err)
	}
	s := cfg.Stages()
	for kind, want := range map[string]string{"bug": "failing-test", "doc": "per-criterion", "chore": "none"} {
		if got := s.KindRule(kind); got != want {
			t.Errorf("KindRule(%q) = %q, want %q", kind, got, want)
		}
	}
	if want := []string{"cycle.kinds.chore.steps: git-board doesn't know this key"}; !slices.Equal(warnings, want) {
		t.Errorf("warnings = %q, want %q", warnings, want)
	}
	for toml, want := range map[string]string{
		"[cycle.kinds.doc]\ntests = \"nope\"\n": `[cycle.kinds] doc: failing-test, per-criterion, none, not "nope"`,
		"[cycle.kinds]\nbug = 3\n":              `[cycle.kinds] bug: failing-test, per-criterion, none`,
	} {
		if _, _, err := parse([]byte(toml)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want %q", toml, err, want)
		}
	}
}

func TestCycleReads(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, toml string
		want       []string
	}{
		{"no file", "", []string{"steps: none, so the default list: intent, risk, plan, tests, code, review, verify"}},
		{"old forms", co2Status + "[cycle.signoffs]\nqa = \"pm\"\n\n[cycle.kinds]\nbug = \"failing-test\"\n", []string{
			"steps: none, so the default list with a sign-off per validation status: intent, risk, plan, tests, code, review, ready-for-validation, qa, tests-metier",
			`[cycle.signoffs] qa = "pm": the sign-off step qa, at status qa, held by the role pm`,
			`[cycle.kinds] bug = "failing-test": [cycle.kinds.bug] tests = "failing-test"`,
		}},
		{"new forms", "[cycle]\nsteps = [\"code\", \"review\"]\n\n[cycle.kinds.bug]\ntests = \"failing-test\"\n", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, _, err := parse([]byte(tc.toml))
			if err != nil {
				t.Fatal(err)
			}
			if got := cfg.Reads(); !slices.Equal(got, tc.want) {
				t.Errorf("Reads()\n got %q\nwant %q", got, tc.want)
			}
		})
	}
}
