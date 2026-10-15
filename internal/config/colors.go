package config

import (
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Palette is the colors a field value can take ([fields.<name>.colors]);
// the web draws each as a tinted chip (web/src/css/gitboard.css).
var Palette = []string{"grey", "blue", "purple", "amber", "orange", "green", "red", "teal", "pink"}

var colorAliases = map[string]string{"gray": "grey", "yellow": "amber", "violet": "purple"}

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

// Colors is the color of each of a field's values: the config's, else a
// keyword's, else the next of spread in the values' sorted order. Only
// select and iteration fields take colors of their own: people, numbers,
// dates (colored by the server against today), issues and text stay plain.
//
// ponytail: turns in sorted order spread colors evenly (a hash clumps a
// handful of values); a new value can shift the turn colors of the values
// after it. Pin one in [fields.<name>.colors] to keep it.
func (c *Config) Colors(field string, values []string) map[string]string {
	res := map[string]string{}
	if t := c.TypeOf(field, values); t != TypeSelect && t != TypeIteration {
		for _, v := range values {
			if color := c.configured(field, v); color != "" {
				res[v] = color
			}
		}
		return res
	}
	var rest []string
	for _, v := range values {
		if color := c.color(field, v); color != "" {
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

// configured is a value's color from [fields.<name>.colors], "" for none.
func (c *Config) configured(field, value string) string {
	for v, color := range c.Fields[field].Colors {
		if strings.EqualFold(v, value) {
			return normColor(color)
		}
	}
	return ""
}

// color is a value's configured or keyword color, "" for none.
func (c *Config) color(field, value string) string {
	if color := c.configured(field, value); color != "" {
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

// checkColors puts each field's colors in Palette's words, or says which
// isn't one.
func checkColors(fields map[string]Field) error {
	for name, f := range fields {
		for v, color := range f.Colors {
			c := normColor(color)
			if !slices.Contains(Palette, c) {
				return fmt.Errorf("fields.%s.colors: %q for %q isn't a color: %s", name, color, v, strings.Join(Palette, ", "))
			}
			f.Colors[v] = c
		}
	}
	return nil
}

// normColor is a color in Palette's words: lower case, yellow is amber.
func normColor(color string) string {
	c := strings.ToLower(strings.TrimSpace(color))
	if a, ok := colorAliases[c]; ok {
		return a
	}
	return c
}
