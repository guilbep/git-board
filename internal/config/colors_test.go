package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestColorConfigBeatsKeywordBeatsTurns(t *testing.T) {
	cfg := &Config{Fields: map[string]Field{"status": {Colors: map[string]string{"validated": "pink", "In Review": "yellow"}}}}
	for _, c := range []struct{ field, value, want string }{
		{"status", "validated", "pink"},            // config wins over the keyword's green
		{"status", "in review", "amber"},           // keys match case-insensitively; yellow is amber
		{"status", "in-progress", "blue"},          // keyword default
		{"status", "ready-for-validation", "pink"}, // not in-review's purple: a board shows both
		{"status", "in-review", "purple"},
		{"status", "tests-metier", "orange"},
		{"status", "comite", "amber"},
		{"status", "backlog", "grey"},
		{"status", "done", "green"},
		{"prio", "p0", "red"},
		{"prio", "p1", "orange"},
		{"prio", "p3", "grey"},
		{"cat", "fix", "red"},
		{"est", "3.5", ""}, // numbers stay plain
		{"assignee", "guilbep", ""},
	} {
		if got := cfg.Colors(c.field, []string{c.value})[c.value]; got != c.want {
			t.Errorf("Colors(%s, %s) = %q, want %q", c.field, c.value, got, c.want)
		}
	}
	// no rule: the four spread colors in turn, in sorted order, whatever the input order
	mods := []string{"travel", "buildings", "results", "headcount", "purchases", "generic"}
	got := cfg.Colors("mod", mods)
	want := map[string]string{"buildings": "blue", "generic": "purple", "headcount": "teal", "purchases": "pink", "results": "blue", "travel": "purple"}
	for v, c := range want {
		if got[v] != c {
			t.Errorf("mod %s = %q, want %q (all %v)", v, got[v], c, got)
		}
	}
}

func TestLoadRefusesAnUnknownColor(t *testing.T) {
	p := filepath.Join(t.TempDir(), FileName)
	_ = os.WriteFile(p, []byte("[fields.status.colors]\nvalidated = \"green\"\n\"in review\" = \"chartreuse\"\n"), 0o644)
	_, err := Load(p)
	if err == nil || !strings.Contains(err.Error(), "chartreuse") || !strings.Contains(err.Error(), "purple") {
		t.Fatalf("Load: %v, want an error naming the color and the palette", err)
	}
	_ = os.WriteFile(p, []byte("[fields.status.colors]\nvalidated = \"green\"\n"), 0o644)
	cfg, err := Load(p)
	if err != nil || cfg.Colors("status", []string{"validated"})["validated"] != "green" {
		t.Fatalf("Load: %v, %v", err, cfg)
	}
}
