package cycle

import (
	"maps"
	"slices"
	"strings"
)

// The [cycle] table's stages and steps, every default filled: v1's
// core/cycle/config.go, owned.

// SignersFile is the signers file's path, its default filled.
func (c Config) SignersFile() string {
	if c.Signers == "" {
		return ".allowed_signers"
	}
	return c.Signers
}

// Stages is the [cycle] table with every default filled: what the server
// computes the step with, and what it shows as guessed.
type Stages struct {
	Before     []string          `json:"before"` // statuses in no other stage
	Ready      string            `json:"ready"`
	InProgress []string          `json:"in_progress"`
	InReview   []string          `json:"in_review"`
	Validation []string          `json:"validation"`
	Closing    string            `json:"closing"` // the last status option, "" without options
	Plans      string            `json:"plans"`
	PlansInPR  bool              `json:"plans_in_pr"`
	TestFiles  []string          `json:"test_files"`
	Criteria   []string          `json:"criteria"`
	Kinds      map[string]string `json:"kinds"`
	// Guessed: a stage came from the status names, not the file.
	Guessed bool `json:"guessed"`
	// Roles is the role holding each gate, keyed as the evidence's people
	// are: risk, plan, review, signoff:<status>. The page names it.
	Roles map[string]string `json:"roles,omitempty"`
	// Steps are the rail's cells, in order.
	Steps []Step `json:"steps"`
}

// Step is one cell of the rail. Status is a sign-off's validation status;
// verify has none: it holds every validation status no listed sign-off
// has, all of them when it is alone, as today.
type Step struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"` // intent, risk, plan, tests, code, review, signoff
	Name   string `json:"name"` // "Review"; a sign-off's is its id until M2
	Status string `json:"status,omitempty"`
}

// DefaultSteps are dev-cycle.md's seven, a project's with no steps key.
var DefaultSteps = []Step{
	{ID: "intent", Kind: "intent", Name: "Intent"}, {ID: "risk", Kind: "risk", Name: "Risk"},
	{ID: "plan", Kind: "plan", Name: "Plan"}, {ID: "tests", Kind: "tests", Name: "Tests"},
	{ID: "code", Kind: "code", Name: "Code"}, {ID: "review", Kind: "review", Name: "Review"},
	{ID: "verify", Kind: "signoff", Name: "Verify"},
}

// Builtins are the steps of a kind of their own, in the cycle's order.
var Builtins = []string{"intent", "risk", "plan", "tests", "code", "review"}

// stepOf is the step an id of the list names: a built-in or verify, else
// a sign-off at the status of that name.
func stepOf(id string) Step {
	if i := slices.IndexFunc(DefaultSteps, func(s Step) bool { return s.ID == id }); i >= 0 {
		return DefaultSteps[i]
	}
	return Step{ID: id, Kind: "signoff", Name: id, Status: id}
}

// defaultSteps is the list a file with no steps key walks: the seven, or,
// when [cycle.signoffs] has entries, the six then a sign-off per
// validation status, and a key no status names last (read 2).
func defaultSteps(validation []string, signoffs map[string]string) []Step {
	if len(signoffs) == 0 {
		return slices.Clone(DefaultSteps)
	}
	steps := slices.Clone(DefaultSteps[:len(Builtins)])
	for _, id := range slices.Concat(validation, slices.Sorted(maps.Keys(signoffs))) {
		if !slices.ContainsFunc(steps, func(s Step) bool { return s.ID == id }) {
			steps = append(steps, stepOf(id))
		}
	}
	return steps
}

var defaultTests = []string{"*_test.go", "*.test.ts", "*.spec.ts", "test_*.py", "tests/**"}

var defaultCriteria = []string{"Acceptance criteria", "Done when", "Expected"}

// Step 4's rules, by an issue's kind: a test that fails before the fix,
// the touched suites green with a test per criterion, or no tests.
const (
	FailingTest  = "failing-test"
	PerCriterion = "per-criterion"
	NoTests      = "none"
)

// KindRules are the values [cycle.kinds] takes.
var KindRules = []string{FailingTest, PerCriterion, NoTests}

var defaultKinds = map[string]string{"bug": FailingTest, "feat": PerCriterion, "doc": NoTests, "chore": NoTests}

// The words of a status name that guess its stage, checked in this order:
// "ready-for-validation" is a validation status, not a ready one.
var stageWords = []struct {
	stage string
	words []string
}{
	{"validation", []string{"validation", "qa", "uat", "acceptance", "test", "tests", "metier", "verify", "staging"}},
	{"in-review", []string{"review", "reviewing", "pr"}},
	{"in-progress", []string{"progress", "doing", "wip", "dev", "development", "implementing", "coding"}},
	{"ready", []string{"ready", "todo", "planned", "selected"}},
}

// guessStages reads the stages from the status names, like Colors reads
// colors from them. The last option closes the issue.
// ponytail: English words and co2's "metier"; [cycle] names the rest
func guessStages(options []string) Stages {
	var s Stages
	if len(options) == 0 {
		return s
	}
	s.Closing = options[len(options)-1]
	for _, o := range options[:len(options)-1] {
		words := strings.FieldsFunc(strings.ToLower(o), func(r rune) bool { return r == '-' || r == '_' || r == ' ' })
		stage := "before"
		for _, sw := range stageWords {
			if slices.ContainsFunc(words, func(w string) bool { return slices.Contains(sw.words, w) }) {
				stage = sw.stage
				break
			}
		}
		switch {
		case stage == "validation":
			s.Validation = append(s.Validation, o)
		case stage == "in-review":
			s.InReview = append(s.InReview, o)
		case stage == "in-progress":
			s.InProgress = append(s.InProgress, o)
		case stage == "ready" && s.Ready == "":
			s.Ready = o
		default:
			s.Before = append(s.Before, o)
		}
	}
	return s
}

// Stages is the cycle's stages: the table's keys, each one left out
// guessed from the status field's options.
func (k Config) Stages(options []string) Stages {
	s := guessStages(options)
	s.Guessed = k.Ready == "" || k.InProgress == nil || k.InReview == nil || k.Validation == nil
	if k.Ready != "" {
		s.Ready = k.Ready
	}
	if k.InProgress != nil {
		s.InProgress = k.InProgress
	}
	if k.InReview != nil {
		s.InReview = k.InReview
	}
	if k.Validation != nil {
		s.Validation = k.Validation
	}
	// never null in JSON: the settings page lists them, even with no status
	for _, l := range []*[]string{&s.InProgress, &s.InReview, &s.Validation} {
		if *l == nil {
			*l = []string{}
		}
	}
	s.Before = []string{}
	for _, o := range options {
		if o != s.Closing && o != s.Ready && !slices.Contains(s.InProgress, o) && !slices.Contains(s.InReview, o) && !slices.Contains(s.Validation, o) {
			s.Before = append(s.Before, o)
		}
	}
	s.Plans = k.Plans
	if s.Plans == "" {
		s.Plans = "docs/src/implementation-plans"
	}
	s.PlansInPR = k.PlansInPR == nil || *k.PlansInPR
	s.TestFiles = k.TestFiles
	if s.TestFiles == nil {
		s.TestFiles = defaultTests
	}
	s.Criteria = k.Criteria
	if s.Criteria == nil {
		s.Criteria = defaultCriteria
	}
	// the file's kinds over the defaults: a project names only what differs
	s.Kinds = maps.Clone(defaultKinds)
	maps.Copy(s.Kinds, k.Kinds)
	roles := map[string]string{"spec": k.Spec, "risk": k.Risk, "plan": k.Plan, "review": k.Review}
	for status, role := range k.Signoffs {
		roles["signoff:"+status] = role
	}
	maps.DeleteFunc(roles, func(_, role string) bool { return role == "" })
	if len(roles) > 0 {
		s.Roles = roles
	}
	s.Steps = defaultSteps(s.Validation, k.Signoffs)
	if k.Steps != nil {
		s.Steps = []Step{}
		for _, id := range k.Steps {
			s.Steps = append(s.Steps, stepOf(id))
		}
	}
	return s
}

// KindRule is step 4's rule for an issue of this kind: per-criterion for
// no kind, or one the table doesn't list (the rule before [cycle.kinds]).
func (s Stages) KindRule(kind string) string {
	if r, ok := s.Kinds[kind]; ok {
		return r
	}
	return PerCriterion
}

// StageOf is a status's stage: "before", "ready", "in-progress",
// "in-review", "validation", "done", or "" for a status no option names.
func (s Stages) StageOf(status string) string {
	switch {
	case status == "":
		return ""
	case status == s.Closing:
		return "done"
	case status == s.Ready:
		return "ready"
	case slices.Contains(s.InProgress, status):
		return "in-progress"
	case slices.Contains(s.InReview, status):
		return "in-review"
	case slices.Contains(s.Validation, status):
		return "validation"
	case slices.Contains(s.Before, status):
		return "before"
	}
	return ""
}
