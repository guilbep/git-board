package issues

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// From v1's internal/config/colors_test.go.
func TestColorConfigBeatsKeywordBeatsTurns(t *testing.T) {
	t.Parallel()
	s := &Schema{Fields: map[string]Field{"status": {Colors: map[string]string{"validated": "pink", "In Review": "yellow"}}}}
	for _, c := range []struct{ field, value, want string }{
		{"status", "validated", "pink"},            // the file's wins over the keyword's green
		{"status", "in review", "amber"},           // keys match without case; yellow is amber
		{"status", "in-progress", "blue"},          // a keyword's
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
		if got := s.Colors(c.field, []string{c.value})[c.value]; got != c.want {
			t.Errorf("Colors(%s, %s) = %q, want %q", c.field, c.value, got, c.want)
		}
	}
	// no rule: the four spread colors in turn, in sorted order, whatever the input order
	got := s.Colors("mod", []string{"travel", "buildings", "results", "headcount", "purchases", "generic"})
	want := map[string]string{"buildings": "blue", "generic": "purple", "headcount": "teal", "purchases": "pink", "results": "blue", "travel": "purple"}
	for v, c := range want {
		if got[v] != c {
			t.Errorf("mod %s = %q, want %q (all %v)", v, got[v], c, got)
		}
	}
}

// A forge's #rrggbb is a color too, kept as is; a forge gets a Palette
// name as its dot's hex.
func TestHex(t *testing.T) {
	t.Parallel()
	if Hex("#ebcf04") != "ebcf04" || Hex("#EBCF04") != "ebcf04" || Hex("teal") != "00897f" || Hex("Yellow") != "d99100" || Hex("") != "" {
		t.Errorf("Hex: %q %q %q %q %q", Hex("#ebcf04"), Hex("#EBCF04"), Hex("teal"), Hex("Yellow"), Hex(""))
	}
}

// [labels] list's colors: the file's, else the palette's in turn; a flat
// kind is stored as its plain label.
func TestLabelColorsAndLabel(t *testing.T) {
	t.Parallel()
	s := &Schema{Labels: Labels{List: []string{"a", "b", "c"}, Colors: map[string]string{"b": "red"}},
		Kinds: map[string]string{"bug": "fix"}, Fields: map[string]Field{"kind": {Options: []string{"bug"}, Flat: true}}}
	if got := s.LabelColors(); got["a"] != "grey" || got["b"] != "red" || got["c"] != "blue" {
		t.Errorf("LabelColors = %v", got)
	}
	if s.Label("kind", "bug") != "bug" || s.Label("kind", "feat") != "kind:feat" || s.Label("prio", "p0") != "prio:p0" {
		t.Errorf("Label: %q %q %q", s.Label("kind", "bug"), s.Label("kind", "feat"), s.Label("prio", "p0"))
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
