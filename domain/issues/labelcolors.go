package issues

import (
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// The colors a forge's label gets for a value, as v1's server gave them
// (internal/config's Colors, LabelColors and Hex), and the web paints
// them (web/src/valuecolors.ts): git board lint --create-labels.

// PaletteHex is each Palette color as its dot is drawn, the --gb-<name>-dot
// of web/src/css/tokens/values.css (TestPaletteHexMatchesCSS): what a
// forge's label gets for it.
var PaletteHex = map[string]string{"grey": "8a8a8a", "blue": "3a7bc8", "purple": "8e44ad", "amber": "d99100", "orange": "e06a00",
	"green": "4f9a1a", "red": "e00000", "teal": "00897f", "pink": "d63a8e"}

// Hex is a color as a forge writes it, without #: a #rrggbb as is, a
// Palette name its dot's; "" for none.
func Hex(color string) string {
	c := NormColor(color)
	if hexColor.MatchString(c) {
		return c[1:]
	}
	return PaletteHex[c]
}

// keywordColors colors a value by a word in it, without any config: the
// first rule with a word of the value wins (so ready-for-validation is
// pink, not ready's teal).
var keywordColors = []struct {
	color string
	words []string
}{
	{"green", []string{"done", "validated", "closed", "shipped", "released", "merged", "resolved", "complete", "completed", "accepted", "approved", "fixed"}},
	{"red", []string{"blocked", "bug", "fix", "urgent", "critical", "p0", "incident", "broken"}},
	{"orange", []string{"p1", "high", "test", "tests", "testing"}},
	{"purple", []string{"review", "reviewing", "qa"}},
	{"pink", []string{"validation", "approval", "sign-off", "signoff"}},
	{"amber", []string{"waiting", "wait", "hold", "pending", "p2", "medium", "comite", "comité", "committee", "triage", "paused"}},
	{"blue", []string{"progress", "doing", "wip", "active", "started", "dev", "development", "ongoing"}},
	{"teal", []string{"ready", "feat", "feature", "enhancement", "planned", "next"}},
	{"grey", []string{"backlog", "todo", "new", "icebox", "someday", "low", "p3", "p4", "draft", "idea", "later", "wontfix"}},
}

// spread are the colors values with no rule take turns with: not the ones
// that say something (grey inactive, red wrong, green done, amber and
// orange wait).
var spread = []string{"blue", "purple", "teal", "pink"}

// Colors is the color of each of a field's values: the file's, else a
// keyword's, else the next of spread in the values' sorted order. Only
// select and iteration fields take colors of their own: people, numbers,
// dates, issues and text only those the file sets.
func (s *Schema) Colors(field string, values []string) map[string]string {
	res := map[string]string{}
	if t := s.TypeOf(field, values); t != TypeSelect && t != TypeIteration {
		for _, v := range values {
			if color := s.ownColor(field, v); color != "" {
				res[v] = color
			}
		}
		return res
	}
	var rest []string
	for _, v := range values {
		if color := s.color(field, v); color != "" {
			res[v] = color
		} else if _, err := strconv.ParseFloat(v, 64); err != nil {
			rest = append(rest, v)
		}
	}
	sort.Strings(rest)
	rest = slices.Compact(rest)
	for i, v := range rest {
		res[v] = spread[i%len(spread)]
	}
	return res
}

// ownColor is a value's color from [fields.<name>.colors], "" for none.
func (s *Schema) ownColor(field, value string) string {
	for v, color := range s.Fields[field].Colors {
		if strings.EqualFold(v, value) {
			return NormColor(color)
		}
	}
	return ""
}

// color is a value's configured or keyword color, "" for none.
func (s *Schema) color(field, value string) string {
	if color := s.ownColor(field, value); color != "" {
		return color
	}
	if _, err := strconv.ParseFloat(value, 64); err == nil {
		return ""
	}
	words := strings.FieldsFunc(strings.ToLower(value), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
	for _, rule := range keywordColors {
		for _, w := range words {
			if slices.Contains(rule.words, w) {
				return rule.color
			}
		}
	}
	return ""
}

// LabelColors are [labels] list's colors: the file's, else the palette's
// in turn.
func (s *Schema) LabelColors() map[string]string {
	res := map[string]string{}
	i := 0
	for _, l := range s.Labels.List {
		if c := s.Labels.Colors[l]; c != "" {
			res[l] = NormColor(c)
			continue
		}
		res[l] = Palette[i%len(Palette)]
		i++
	}
	return res
}

// Label is the label a field's value is stored as: bug for a flat kind,
// else field:value.
func (s *Schema) Label(field, value string) string {
	if _, ok := s.Kinds[value]; ok && field == "kind" && s.FlatKinds() {
		return value
	}
	return field + ":" + value
}
