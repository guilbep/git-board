package config

import (
	"fmt"
	"maps"
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
	Tests      []string `toml:"tests"` // default defaultTests
	Risk       string   `toml:"risk"`  // the role that sets the risk class
	Plan       string   `toml:"plan"`  // the role that approves the plan
	Review     string   `toml:"review"`
	// Signoffs maps a validation status to the role that signs it off.
	Signoffs map[string]string `toml:"signoffs"`
}

// Stages is the [cycle] table with every default filled: what the server
// computes the step with, and what it shows as guessed.
type Stages struct {
	Before     []string `json:"before"` // statuses in no other stage
	Ready      string   `json:"ready"`
	InProgress []string `json:"in_progress"`
	InReview   []string `json:"in_review"`
	Validation []string `json:"validation"`
	Closing    string   `json:"closing"` // the last status option, "" without options
	Plans      string   `json:"plans"`
	Tests      []string `json:"tests"`
	// Guessed: a stage came from the status names, not the file.
	Guessed bool `json:"guessed"`
}

var defaultTests = []string{"*_test.go", "*.test.ts", "*.spec.ts", "test_*.py", "tests/**"}

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
	s.Before = nil
	for _, o := range options {
		if o != s.Closing && o != s.Ready && !slices.Contains(s.InProgress, o) && !slices.Contains(s.InReview, o) && !slices.Contains(s.Validation, o) {
			s.Before = append(s.Before, o)
		}
	}
	s.Plans = k.Plans
	if s.Plans == "" {
		s.Plans = "docs/src/implementation-plans"
	}
	s.Tests = k.Tests
	if s.Tests == nil {
		s.Tests = defaultTests
	}
	return s
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
	for _, g := range []struct{ key, role string }{{"risk", c.Risk}, {"plan", c.Plan}, {"review", c.Review}} {
		if g.role != "" && !roleName.MatchString(g.role) {
			return fmt.Errorf("[cycle] %s: a role name, not %q", g.key, g.role)
		}
	}
	for _, status := range slices.Sorted(maps.Keys(c.Signoffs)) {
		if role := c.Signoffs[status]; !roleName.MatchString(role) {
			return fmt.Errorf("[cycle.signoffs] %s: a role name, not %q", status, role)
		}
	}
	return nil
}
