package config

import (
	"slices"
	"testing"
)

// The default must check clean, and its statuses must give the whole cycle,
// the sign-off step included.
func TestDefaultChecksClean(t *testing.T) {
	cfg, warnings, err := Check([]byte(Default))
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if len(warnings) > 0 {
		t.Errorf("warnings = %q, want none", warnings)
	}
	s := cfg.Stages()
	if s.Ready != "ready" || !slices.Equal(s.InProgress, []string{"in-progress"}) || !slices.Equal(s.InReview, []string{"in-review"}) ||
		!slices.Equal(s.Validation, []string{"ready-for-validation"}) || s.Closing != "validated" {
		t.Errorf("stages = %+v", s)
	}
	// the org's flat kinds are the kind field, in the file's order
	kinds := []string{"bug", "feat", "docs", "refactor", "perf", "polish", "style", "test"}
	if k := cfg.Fields["kind"]; !cfg.FlatKinds() || !k.Single || !slices.Equal(k.Options, kinds) {
		t.Errorf("kind = %+v, flat %v; want single, flat, %q", k, cfg.FlatKinds(), kinds)
	}
}
