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
	// the IT4R kinds, flat, are the kind field, in the file's order; each
	// its branch's prefix, and described
	kinds := []string{"bug", "feat", "polish", "docs", "refactor", "perf", "test", "style", "chore", "design"}
	if k := cfg.Fields["kind"]; !cfg.FlatKinds() || !k.Single || !slices.Equal(k.Options, kinds) {
		t.Errorf("kind = %+v, flat %v; want single, flat, %q", k, cfg.FlatKinds(), kinds)
	}
	for _, k := range kinds {
		if cfg.Branches.Kinds[k] != k || cfg.Fields["kind"].Descriptions[k] == "" {
			t.Errorf("kind %s: branch %q, description %q", k, cfg.Branches.Kinds[k], cfg.Fields["kind"].Descriptions[k])
		}
	}
}

// ENAC's template checks clean, with the forge's spelling and EPFL-ENAC's
// labels byte for byte (docs/prd/board-template.md).
func TestENACTemplateChecksClean(t *testing.T) {
	t.Parallel()
	s, err := Template("enac")
	if err != nil {
		t.Fatal(err)
	}
	cfg, warnings, err := Check([]byte(s))
	if err != nil || len(warnings) > 0 {
		t.Fatalf("Check: %v, warnings %q", err, warnings)
	}
	if p := cfg.Fields["prio"].Options; !slices.Equal(p, []string{"P0", "P1", "P2", "P3"}) {
		t.Errorf("prio = %q", p)
	}
	k := cfg.Fields["kind"]
	if k.Colors["perf"] != "#120608" || k.Descriptions["perf"] != "A code change that improves perforamnce: should prefix branch by perf/" {
		t.Errorf("perf = %q, %q", k.Colors["perf"], k.Descriptions["perf"])
	}
	if _, err := Template("nope"); err == nil {
		t.Error("an unknown template loaded")
	}
}
