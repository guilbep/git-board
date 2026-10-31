package config

import (
	"os"
	"path/filepath"
	"regexp"
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

// A GitHub label's #rrggbb is a color too, kept as is; GitHub gets a
// Palette name as its dot's hex.
func TestHexColors(t *testing.T) {
	t.Parallel()
	cfg, _, err := parse([]byte("[fields.kind.colors]\nbug = \"#EBCF04\"\nfeat = \"teal\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Colors("kind", []string{"bug"})["bug"]; got != "#ebcf04" {
		t.Errorf("bug = %q, want #ebcf04", got)
	}
	if Hex("#ebcf04") != "ebcf04" || Hex("teal") != "00897f" || Hex("") != "" {
		t.Errorf("Hex: %q %q %q", Hex("#ebcf04"), Hex("teal"), Hex(""))
	}
	for _, bad := range []string{"#zz0000", "ebcf04", "#ebcf0"} {
		if _, _, err := parse([]byte("[fields.kind.colors]\nbug = \"" + bad + "\"\n")); err == nil {
			t.Errorf("%s: no error", bad)
		}
	}
}

// PaletteHex is the web's dots, not a third set of colors.
func TestPaletteHexMatchesCSS(t *testing.T) {
	t.Parallel()
	css, err := os.ReadFile("../../web/src/css/tokens/values.css")
	if err != nil {
		t.Fatal(err)
	}
	dots := map[string]string{}
	for _, m := range regexp.MustCompile(`--gb-([a-z]+)-dot:\s*#([0-9A-Fa-f]{6})`).FindAllStringSubmatch(string(css), -1) {
		dots[m[1]] = strings.ToLower(m[2])
	}
	for _, c := range Palette {
		if PaletteHex[c] == "" || PaletteHex[c] != dots[c] {
			t.Errorf("%s: PaletteHex %q, tokens/values.css --gb-%s-dot %q", c, PaletteHex[c], c, dots[c])
		}
	}
	if len(PaletteHex) != len(Palette) {
		t.Errorf("PaletteHex has %d colors, Palette %d", len(PaletteHex), len(Palette))
	}
}
