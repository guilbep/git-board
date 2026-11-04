package config

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/guilbep/git-board/core/cycle"
	"github.com/guilbep/git-board/core/fields"
	"github.com/guilbep/git-board/core/proof"
)

// cycleTable is [cycle] as the file writes it: the core's table
// (core/cycle), and [cycle.kinds], a kind's rule as a string or as its
// table's tests, which readKinds reads into Kinds.
type cycleTable struct {
	cycle.Config
	KindsFile map[string]toml.Primitive `toml:"kinds"`
	// shortKinds are the kinds the file gave as a string, for Reads.
	shortKinds []string
}

// AtBase is the proof rules as the base branch has them committed: its
// [cycle] tests and checks, and its signers file (ADR 0001, section 5).
// No .git-board.toml there is no checks; no signers file, nil signers.
func AtBase(ctx context.Context, dir, base string) (proof.Base, error) {
	data, err := proof.Show(ctx, dir, base, ".git-board.toml")
	if err != nil {
		return proof.Base{}, err
	}
	cfg := &Config{}
	if data != nil {
		if cfg, _, err = Check(data); err != nil {
			return proof.Base{}, fmt.Errorf(".git-board.toml on %s: %w", base, err)
		}
	}
	b := proof.Base{Branch: base, Tests: cfg.Cycle.Tests, Checks: cfg.Cycle.Checks, SignersFile: cfg.Cycle.SignersFile()}
	b.Signers, err = proof.Show(ctx, dir, base, b.SignersFile)
	return b, err
}

// Stages is the cycle's stages: the [cycle] table's keys, each one left
// out guessed from the status field's options.
func (c *Config) Stages() cycle.Stages {
	return c.Cycle.Stages(c.Fields["status"].Options)
}

// Reads is what the file's old forms were read as, one line each, for
// lint: the default list (named), [cycle.signoffs] as cells, a kind's
// string as its tests.
func (c *Config) Reads() []string {
	var out []string
	if c.Cycle.Steps == nil {
		with := ""
		if len(c.Cycle.Signoffs) > 0 {
			with = " with a sign-off per validation status"
		}
		var ids []string
		for _, s := range c.Stages().Steps {
			ids = append(ids, s.ID)
		}
		out = append(out, fmt.Sprintf("steps: none, so the default list%s: %s", with, strings.Join(ids, ", ")))
	}
	for _, id := range slices.Sorted(maps.Keys(c.Cycle.Signoffs)) {
		role := c.Cycle.Signoffs[id]
		out = append(out, fmt.Sprintf("[cycle.signoffs] %s = %q: the sign-off step %s, at status %s, held by the role %s", id, role, id, id, role))
	}
	for _, kind := range c.shortKinds {
		r := c.Cycle.Kinds[kind]
		out = append(out, fmt.Sprintf("[cycle.kinds] %s = %q: [cycle.kinds.%s] tests = %q", kind, r, kind, r))
	}
	return out
}

// readKinds reads [cycle.kinds] into Kinds: a kind's string is its rule,
// its table's tests too; a table without tests keeps the default. Only
// tests is decoded, so a kind's steps (M3's) is a key Check warns about.
func readKinds(md toml.MetaData, c *cycleTable) error {
	for _, kind := range slices.Sorted(maps.Keys(c.KindsFile)) {
		var t struct {
			Tests string `toml:"tests"`
		}
		switch md.Type("cycle", "kinds", kind) {
		case "String":
			if err := md.PrimitiveDecode(c.KindsFile[kind], &t.Tests); err != nil {
				return err
			}
			c.shortKinds = append(c.shortKinds, kind)
		case "Hash":
			if err := md.PrimitiveDecode(c.KindsFile[kind], &t); err != nil {
				return fmt.Errorf("[cycle.kinds.%s] %w", kind, err)
			}
		default:
			return fmt.Errorf("[cycle.kinds] %s: %s, or a [cycle.kinds.%s] table", kind, strings.Join(cycle.KindRules, ", "), kind)
		}
		if t.Tests != "" {
			if c.Kinds == nil {
				c.Kinds = map[string]string{}
			}
			c.Kinds[kind] = t.Tests
		}
	}
	return nil
}

// checkSteps refuses a list the cycle can't walk: an id no step has, one
// listed twice, a built-in out of the cycle's order or after a sign-off.
// A known id is a built-in, verify, a validation status or a
// [cycle.signoffs] key: what read 2 reads, so a project can write it out.
func checkSteps(c cycle.Config, validation []string) error {
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
		b := slices.Index(cycle.Builtins, id)
		if _, signoff := c.Signoffs[id]; b < 0 && id != "verify" && !signoff && !slices.Contains(validation, id) {
			return fmt.Errorf("[cycle] steps: %q is no step: %s, verify, a validation status or a [cycle.signoffs] key", id, strings.Join(cycle.Builtins, ", "))
		}
		// pairwise is enough: a built-in follows a built-in before it
		if i > 0 && b >= 0 {
			prev := c.Steps[i-1]
			if p := slices.Index(cycle.Builtins, prev); p < 0 || p > b {
				return fmt.Errorf("[cycle] steps: %q after %q: steps keep the cycle's order, %s, then the sign-offs (a review needs the PR that code opens, a sign-off the merge)", id, prev, strings.Join(cycle.Builtins, ", "))
			}
		}
	}
	return nil
}

// checkCycleTable reads [cycle.kinds] into c and checks the table: what
// parse refuses while the cycle is on.
func checkCycleTable(md toml.MetaData, c *cycleTable, defs map[string]fields.Field) error {
	// M1's test-file globs were [cycle] tests; the name is the suites' now,
	// and the decoder would drop a list there without a word
	if md.Type("cycle", "tests") == "Array" {
		return errors.New("[cycle] tests: the test-file globs are test-files now; [cycle.tests.<name>] tables are the suites step 4 runs")
	}
	if err := readKinds(md, c); err != nil {
		return err
	}
	if err := checkCycle(c.Config); err != nil {
		return err
	}
	return checkSteps(c.Config, c.Stages(defs["status"].Options).Validation)
}

// checkCycle refuses a gate's role that couldn't be a role name. One that
// [roles] doesn't name is fine: that gate is anyone's.
func checkCycle(c cycle.Config) error {
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
		if r := c.Kinds[kind]; !slices.Contains(cycle.KindRules, r) {
			return fmt.Errorf("[cycle.kinds] %s: %s, not %q", kind, strings.Join(cycle.KindRules, ", "), r)
		}
	}
	for _, status := range slices.Sorted(maps.Keys(c.Signoffs)) {
		if role := c.Signoffs[status]; !roleName.MatchString(role) {
			return fmt.Errorf("[cycle.signoffs] %s: a role name, not %q", status, role)
		}
	}
	for _, t := range []struct {
		table  string
		suites map[string]proof.Suite
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
func checkSuite(s proof.Suite) error {
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
