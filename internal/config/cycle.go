package config

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"
)

// Cycle is the [cycle] table: the project's statuses as the loop's stages,
// where plans live, which files are tests, and who holds each gate. Every
// key has a default (Stages); a role left out, or one [roles] doesn't
// name, makes that gate anyone's.
type Cycle struct {
	Ready      string   `toml:"ready"`
	InProgress []string `toml:"in-progress"`
	InReview   []string `toml:"in-review"`
	Validation []string `toml:"validation"`
	Plans      string   `toml:"plans"` // default "docs/src/implementation-plans"
	// PlansInPR: the draft PR opened at step 3 holds the plan; false
	// keeps plans out of PRs, the PR opens at step 5. Default true.
	PlansInPR *bool    `toml:"plans-in-pr"`
	TestFiles []string `toml:"test-files"` // default defaultTests
	// Criteria are the headings a description's acceptance criteria are
	// listed under; default defaultCriteria.
	Criteria []string `toml:"criteria"`
	// Kinds maps an issue's kind to step 4's rule (KindRules), over
	// defaultKinds.
	Kinds  map[string]string `toml:"kinds"`
	Risk   string            `toml:"risk"` // the role that sets the risk class
	Plan   string            `toml:"plan"` // the role that approves the plan
	Review string            `toml:"review"`
	Spec   string            `toml:"spec"` // the role that gives the description's verdict
	// Signoffs maps a validation status to the role that signs it off.
	Signoffs map[string]string `toml:"signoffs"`
	// Tests are the suites step 4 runs, Checks the checks that gate Mark
	// ready for review (docs/adr/0001-check-results-are-signed-git-notes.md).
	Tests  map[string]Suite `toml:"tests"`
	Checks map[string]Suite `toml:"checks"`
	// Signers is the OpenSSH allowed_signers file a result's signature is
	// verified against, read from the base branch; default ".allowed_signers".
	Signers string `toml:"signers"`
}

// Suite is a [cycle.tests.<name>] or [cycle.checks.<name>] table: what runs,
// and what it reads.
type Suite struct {
	Run []string `toml:"run" json:"run"` // an argv, never a shell line
	// Files are the git glob pathspecs of everything it reads (its content
	// key); none is the whole tree.
	Files []string `toml:"files" json:"files,omitempty"`
	Where string   `toml:"where" json:"where,omitempty"` // "worktree" (default) or "ci"
	Dir   string   `toml:"dir" json:"dir,omitempty"`     // where it runs, from the checkout's root
	// Env is every variable it runs with beside PATH, HOME and the tool caches.
	Env map[string]string `toml:"env" json:"env,omitempty"`
	// Tools are version commands: their output names the toolchain.
	Tools [][]string `toml:"tools" json:"tools,omitempty"`
}

// SignersFile is the signers file's path, its default filled.
func (c Cycle) SignersFile() string {
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

// Stages is the cycle's stages: the file's [cycle] keys, each one left out
// guessed from the status options.
func (c *Config) Stages() Stages {
	options := c.Fields["status"].Options
	s := guessStages(options)
	k := c.Cycle
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

// checkCycle refuses a gate's role that couldn't be a role name. One that
// [roles] doesn't name is fine: that gate is anyone's.
func checkCycle(c Cycle) error {
	for _, g := range []struct{ key, role string }{{"spec", c.Spec}, {"risk", c.Risk}, {"plan", c.Plan}, {"review", c.Review}} {
		if g.role != "" && !roleName.MatchString(g.role) {
			return fmt.Errorf("[cycle] %s: a role name, not %q", g.key, g.role)
		}
	}
	for _, h := range c.Criteria {
		if strings.TrimSpace(h) == "" {
			return fmt.Errorf("[cycle] criteria: a heading, not %q", h)
		}
	}
	for _, kind := range slices.Sorted(maps.Keys(c.Kinds)) {
		if r := c.Kinds[kind]; !slices.Contains(KindRules, r) {
			return fmt.Errorf("[cycle.kinds] %s: %s, not %q", kind, strings.Join(KindRules, ", "), r)
		}
	}
	for _, status := range slices.Sorted(maps.Keys(c.Signoffs)) {
		if role := c.Signoffs[status]; !roleName.MatchString(role) {
			return fmt.Errorf("[cycle.signoffs] %s: a role name, not %q", status, role)
		}
	}
	for _, t := range []struct {
		table  string
		suites map[string]Suite
	}{{"tests", c.Tests}, {"checks", c.Checks}} {
		for _, name := range slices.Sorted(maps.Keys(t.suites)) {
			if err := checkSuite(t.suites[name]); err != nil {
				return fmt.Errorf("[cycle.%s.%s] %w", t.table, name, err)
			}
			if !suiteName.MatchString(name) {
				return fmt.Errorf("[cycle.%s.%s]: a name of letters, digits, '.', '_' and '-'", t.table, name)
			}
		}
	}
	if c.Signers != "" && !relative(c.Signers) {
		return fmt.Errorf("[cycle] signers: a path in the repository, not %q", c.Signers)
	}
	return nil
}

// A suite's name goes into a result line, which holds no space.
var suiteName = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// checkSuite refuses a suite that couldn't run as the toml says.
func checkSuite(s Suite) error {
	if len(s.Run) == 0 || s.Run[0] == "" {
		return errors.New("run: a command, as a list of words")
	}
	if s.Where != "" && s.Where != "worktree" && s.Where != "ci" {
		return fmt.Errorf("where: worktree or ci, not %q", s.Where)
	}
	if s.Dir != "" && !relative(s.Dir) {
		return fmt.Errorf("dir: a directory of the repository, not %q", s.Dir)
	}
	for _, k := range slices.Sorted(maps.Keys(s.Env)) {
		if !envName.MatchString(k) {
			return fmt.Errorf("env: a variable name, not %q", k)
		}
		if strings.ContainsAny(s.Env[k], "\n") {
			return fmt.Errorf("env: %s holds a newline", k)
		}
	}
	for _, t := range s.Tools {
		if len(t) == 0 || t[0] == "" {
			return errors.New("tools: each a command, as a list of words")
		}
	}
	for _, f := range s.Files {
		if strings.TrimSpace(f) == "" || strings.HasPrefix(f, ":") {
			return fmt.Errorf("files: a glob, not %q", f)
		}
	}
	return nil
}

// relative is a path inside the repository: not absolute, no "..".
func relative(p string) bool {
	return !path.IsAbs(p) && !slices.Contains(strings.Split(path.Clean(p), "/"), "..")
}
