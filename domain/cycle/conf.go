// Package cycle is an issue's steps, experimental (ADR 0018). M2 reads
// its table of .git-board.toml, [cycle], with v1's rules; the cycle
// itself comes with M6.
package cycle

import (
	"errors"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/guilbep/git-board/conf"
	"github.com/guilbep/git-board/domain/releases/proof"
)

// Feature is the cycle's name in [experimental] features.
const Feature = "cycle"

// Config is the [cycle] table: the project's statuses as the loop's
// stages, where plans live, which files are tests, and who holds each
// gate. Every key has a default (Stages).
type Config struct {
	// On says the file has a [cycle] table: the board walks the cycle.
	On         bool     `toml:"-" json:"on"`
	Ready      string   `toml:"ready"`
	InProgress []string `toml:"in-progress"`
	InReview   []string `toml:"in-review"`
	Validation []string `toml:"validation"`
	Plans      string   `toml:"plans"`
	PlansInPR  *bool    `toml:"plans-in-pr"`
	TestFiles  []string `toml:"test-files"`
	Criteria   []string `toml:"criteria"`
	Steps      []string `toml:"steps"`
	// Kinds are step 4's rule by kind: [cycle.kinds], a string or a
	// table's tests.
	Kinds    map[string]string `toml:"-"`
	Risk     string            `toml:"risk"`
	Plan     string            `toml:"plan"`
	Review   string            `toml:"review"`
	Spec     string            `toml:"spec"`
	Signoffs map[string]string `toml:"signoffs"`
	Tests    map[string]Suite  `toml:"tests"`
	Checks   map[string]Suite  `toml:"checks"`
	Signers  string            `toml:"signers"`
	// KindsFile is [cycle.kinds] as the file writes it.
	KindsFile map[string]toml.Primitive `toml:"kinds"`
}

// Suite is a [cycle.tests.<name>] or [cycle.checks.<name>]: proof's.
type Suite = proof.Suite

// Table is the cycle's table on a file.
type Table struct {
	h *conf.Handle[Config]
	f *conf.File
}

// Declare declares [cycle] on f.
func Declare(f *conf.File) Table {
	return Table{conf.Declare(f, conf.Table[Config]{Name: "cycle"}), f}
}

// Get is [cycle] read, checked against the status field's options. Its
// problems break the file only while the cycle is on: the caller says.
func (t Table) Get(statuses []string) (Config, error) {
	c, probs := t.h.Get()
	if len(probs) > 0 {
		return c, probs[0]
	}
	c.On = t.h.Defined()
	// M1's test-file globs were [cycle] tests; the decoder would drop a
	// list there without a word
	if t.f.Type("cycle", "tests") == "Array" {
		return c, errors.New("[cycle] tests: the test-file globs are test-files now; [cycle.tests.<name>] tables are the suites step 4 runs")
	}
	if err := t.readKinds(&c); err != nil {
		return c, err
	}
	if err := check(c); err != nil {
		return c, err
	}
	return c, checkSteps(c, validation(c, statuses))
}

// readKinds reads [cycle.kinds] into Kinds: a string is the rule, a
// table its tests.
func (t Table) readKinds(c *Config) error {
	for _, kind := range slices.Sorted(maps.Keys(c.KindsFile)) {
		var k struct {
			Tests string `toml:"tests"`
		}
		switch t.f.Type("cycle", "kinds", kind) {
		case "String":
			if err := t.h.Decode(c.KindsFile[kind], &k.Tests); err != nil {
				return err
			}
		case "Hash":
			if err := t.h.Decode(c.KindsFile[kind], &k); err != nil {
				return fmt.Errorf("[cycle.kinds.%s] %w", kind, err)
			}
		default:
			return fmt.Errorf("[cycle.kinds] %s: %s, or a [cycle.kinds.%s] table", kind, strings.Join(KindRules, ", "), kind)
		}
		if k.Tests != "" {
			if c.Kinds == nil {
				c.Kinds = map[string]string{}
			}
			c.Kinds[kind] = k.Tests
		}
	}
	return nil
}

var (
	roleName  = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	suiteName = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
	envName   = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

func check(c Config) error {
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

func relative(p string) bool {
	return !path.IsAbs(p) && !slices.Contains(strings.Split(path.Clean(p), "/"), "..")
}

// checkSteps refuses a list the cycle can't walk.
func checkSteps(c Config, validation []string) error {
	if c.Steps == nil {
		return nil
	}
	if len(c.Steps) == 0 {
		return errors.New("[cycle] steps: at least one step")
	}
	for i, id := range c.Steps {
		if slices.Contains(c.Steps[:i], id) {
			return fmt.Errorf("[cycle] steps: %q twice", id)
		}
		b := slices.Index(Builtins, id)
		if _, signoff := c.Signoffs[id]; b < 0 && id != "verify" && !signoff && !slices.Contains(validation, id) {
			return fmt.Errorf("[cycle] steps: %q is no step: %s, verify, a validation status or a [cycle.signoffs] key", id, strings.Join(Builtins, ", "))
		}
		if i > 0 && b >= 0 {
			prev := c.Steps[i-1]
			if p := slices.Index(Builtins, prev); p < 0 || p > b {
				return fmt.Errorf("[cycle] steps: %q after %q: steps keep the cycle's order, %s, then the sign-offs (a review needs the PR that code opens, a sign-off the merge)", id, prev, strings.Join(Builtins, ", "))
			}
		}
	}
	return nil
}

// validation is the validation statuses: the table's, else those the
// status options' words name (v1's guessStages), the last option aside.
func validation(c Config, options []string) []string {
	if c.Validation != nil {
		return c.Validation
	}
	words := []string{"validation", "qa", "uat", "acceptance", "test", "tests", "metier", "verify", "staging"}
	var res []string
	for i, o := range options {
		if i == len(options)-1 {
			break
		}
		ws := strings.FieldsFunc(strings.ToLower(o), func(r rune) bool { return r == '-' || r == '_' || r == ' ' })
		if slices.ContainsFunc(ws, func(w string) bool { return slices.Contains(words, w) }) {
			res = append(res, o)
		}
	}
	return res
}
