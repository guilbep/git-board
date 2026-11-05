package tui

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/charmbracelet/bubbles/list"

	"github.com/guilbep/git-board/domain/issues"
)

// catFieldName is the field namespace the quick category filter (kbd "c")
// picks values from.
//
// ponytail: hardcoded to a literal "cat" lookup rather than a "guess the
// type-like field" heuristic — add a heuristic or a config key if a project
// wants the quick filter on a differently-named field.
const catFieldName = "cat"

// KnownField is one field's current value(s) on an issue, ready to render
// as a labeled detail line (e.g. "Assignee: alice, bob").
type KnownField struct {
	Name   string
	Values []string
}

// ParseLabels splits an issue's labels into field values — every
// "ns:value" label, configured or not (grouped by field name, one entry per
// field, field-value order preserved) — and labels without a namespace.
// Pure — no bubbletea — so it's covered directly by tests.
func ParseLabels(labels []string) (known []KnownField, other []string) {
	byField := make(map[string][]string)
	for _, l := range labels {
		ns, val, ok := strings.Cut(l, ":")
		if !ok {
			other = append(other, l)
			continue
		}
		byField[ns] = append(byField[ns], val)
	}

	names := make([]string, 0, len(byField))
	for n := range byField {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		known = append(known, KnownField{Name: n, Values: byField[n]})
	}
	return known, other
}

// DisplayFieldName renders a field namespace (e.g. "status") as a detail
// line label (e.g. "Status").
func DisplayFieldName(name string) string {
	if name == "" {
		return name
	}
	r := []rune(name)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// renderDetail is the pure text body of the detail view: title, id/status,
// recognized fields, unrecognized labels, and a trailing status line (write
// confirmation or error) when one is set.
func renderDetail(issue issues.Issue, cfg *issues.Schema, status string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n%s  (%s)\n\n", issue.ShownTitle(), ref(issue), issue.Status)

	known, other := ParseLabels(issue.Labels)
	for _, f := range known {
		fmt.Fprintf(&b, "%s: %s\n", DisplayFieldName(f.Name), strings.Join(f.Values, ", "))
	}
	if len(known) == 0 {
		b.WriteString("(no recognized fields)\n")
	}

	if len(other) > 0 {
		b.WriteString("\nOther labels:\n")
		for _, l := range other {
			fmt.Fprintf(&b, "  - %s\n", l)
		}
	}

	if status != "" {
		fmt.Fprintf(&b, "\n%s\n", status)
	}
	b.WriteString("\ne: edit field   esc/q: back\n")
	return b.String()
}

// fieldItem adapts a config field name to bubbles/list's DefaultItem, for
// the edit-field picker.
type fieldItem struct{ name string }

func (f fieldItem) Title() string       { return DisplayFieldName(f.name) }
func (f fieldItem) Description() string { return f.name }
func (f fieldItem) FilterValue() string { return f.name }

// valueItem adapts one selectable value (a field option, or a category) to
// bubbles/list's DefaultItem, for the value and category pickers.
type valueItem struct{ value, desc string }

func (v valueItem) Title() string       { return v.value }
func (v valueItem) Description() string { return v.desc }
func (v valueItem) FilterValue() string { return v.value }

// FieldNames returns the fields shown and editable, sorted: cfg's
// configured ones plus every label namespace found on issues, so a new
// "ns:value" label needs no config to get its column.
func FieldNames(cfg *issues.Schema, list_ []issues.Issue) []string {
	seen := make(map[string]bool, len(cfg.Fields))
	for n := range cfg.Fields {
		seen[n] = true
	}
	for _, issue := range list_ {
		for _, l := range issue.Labels {
			if ns, _, ok := strings.Cut(l, ":"); ok {
				seen[ns] = true
			}
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func fieldPickerItems(cfg *issues.Schema, list_ []issues.Issue) []list.Item {
	names := FieldNames(cfg, list_)
	items := make([]list.Item, len(names))
	for i, n := range names {
		items[i] = fieldItem{name: n}
	}
	return items
}

// valuePickerItems returns fieldName's configured Options as picker items,
// or nil if it has none — the caller falls back to free-text input.
func valuePickerItems(cfg *issues.Schema, fieldName string) []list.Item {
	opts := cfg.Fields[fieldName].Options
	items := make([]list.Item, len(opts))
	for i, o := range opts {
		items[i] = valueItem{value: o}
	}
	return items
}

// catField reports the field driving the quick category filter, and whether
// one is configured at all.
func catField(cfg *issues.Schema) (issues.Field, bool) {
	f, ok := cfg.Fields[catFieldName]
	return f, ok && f.Single
}

// catFilterItems builds the category picker's items: a leading "(all)" entry
// that clears the filter, then f's configured options.
func catFilterItems(f issues.Field) []list.Item {
	items := make([]list.Item, 0, len(f.Options)+1)
	items = append(items, valueItem{value: "(all)", desc: "clear filter"})
	for _, o := range f.Options {
		items = append(items, valueItem{value: o})
	}
	return items
}

// FilterByField narrows issues to those carrying "fieldName:value" as a
// label — the pure logic behind the quick category filter.
func FilterByField(list_ []issues.Issue, fieldName, value string) []issues.Issue {
	label := fieldName + ":" + value
	out := make([]issues.Issue, 0, len(list_))
	for _, iss := range list_ {
		for _, l := range iss.Labels {
			if l == label {
				out = append(out, iss)
				break
			}
		}
	}
	return out
}
