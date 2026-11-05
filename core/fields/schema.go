package fields

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"time"
)

// Schema is what a board's fields are: their definitions and types, the
// sprint calendar, the project's roles and its plain labels. The
// application reads it from .git-board.toml (internal/config embeds it);
// a tool can write one by hand. Filters, assignments and the inbox read
// it.
type Schema struct {
	Fields  map[string]Field
	Sprints Sprints
	// Roles are the project's people by role: lead-dev = "bob". A role is
	// a person field; an issue's own value (lead-dev:dan) overrides the
	// project's.
	Roles  map[string]string
	Labels Labels
	// Order is Fields' names as written, the order a list of fields
	// takes (FieldOrder).
	Order []string
}

// FieldOrder is the fields' names as the file lists them: the fields
// page's list order, the order an issue's fields line takes. No Order:
// sorted.
func (s *Schema) FieldOrder() []string {
	if len(s.Order) == len(s.Fields) {
		return s.Order
	}
	return slices.Sorted(maps.Keys(s.Fields))
}

// Field describes one namespaced-label field (e.g. "status", "prio").
type Field struct {
	Single  bool     `toml:"single" json:"single"`
	Options []string `toml:"options" json:"options"`
	Numeric bool     `toml:"numeric" json:"numeric"`
	// Project names the GitHub Project's option for a value, where the
	// slug of the Project's name isn't the value: comite = "Comité
	// Spécialisé".
	Project map[string]string `toml:"project" json:"project,omitempty"`
	// Colors picks a value's color from a palette, or a #rrggbb: "in
	// review" = "purple" (internal/config's Palette). Values without one
	// get a default there.
	Colors map[string]string `toml:"colors" json:"colors,omitempty"`
	// Descriptions say what a value means, as its GitHub label's
	// description: 100 characters at most, GitHub's limit.
	Descriptions map[string]string `toml:"descriptions" json:"descriptions,omitempty"`
	// Type is what the values are (Type*); empty: guessed (TypeOf).
	Type string `toml:"type" json:"type,omitempty"`
	// Unit follows a number field's values: "h" for hours.
	Unit string `toml:"unit" json:"unit,omitempty"`
	// Aliases fold a value's old names into the current one:
	// could = "couldhave" reads moscow:could as moscow:couldhave, and
	// Adopt rewrites the labels. The keys are not options.
	Aliases map[string]string `toml:"aliases" json:"aliases,omitempty"`
	// Flat: the kind field of plain labels (bug, not kind:bug), from
	// .git-board.toml's [branches] kinds; internal/config sets it.
	Flat bool `toml:"-" json:"-"`
}

// AliasOf is value's current name on the field: couldhave for could,
// value itself when no alias (or option) holds it. Reading a forge or an
// issue goes through here, so an old label shows as its current value.
func (f Field) AliasOf(value string) string {
	if cur, ok := f.Aliases[value]; ok {
		return cur
	}
	return value
}

// KnownValue reports whether value is one of Options — or true if Options is
// empty (a free-form field, nothing configured to validate against), or the
// field is numeric and value a number (its options are then the usual
// picks, est 3.5 is still an estimate). Check holds an assignment to it; a
// forge's pull and the lint warn on the others.
func (f Field) KnownValue(value string) bool {
	if _, err := strconv.ParseFloat(value, 64); err == nil && (f.Numeric || f.Type == TypeNumber) {
		return true
	}
	return len(f.Options) == 0 || slices.Contains(f.Options, value)
}

// A field's type says what its values are, and so how git-board sorts,
// filters, shows and edits them: together the fields and their types are
// the project's ontology (docs/guide/configuration.md, Types). The values stay
// labels, ns:value, whatever the type.
const (
	TypeSelect    = "select"    // ordered values (options), colors, board columns
	TypeIteration = "iteration" // spans of dates: [[sprints.list]], @current
	TypePerson    = "person"    // forge logins, @me
	TypeNumber    = "number"    // numbers, with a unit
	TypeDate      = "date"      // YYYY-MM-DD: overdue, soon
	TypeIssue     = "issue"     // another issue: #12 or a git-bug id
	TypeText      = "text"      // free text
)

// Types are the types a field can take.
var Types = []string{TypeSelect, TypeIteration, TypePerson, TypeNumber, TypeDate, TypeIssue, TypeText}

var (
	dateValue  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	issueValue = regexp.MustCompile(`^(#\d+|[0-9a-f]{7,64})$`)
	byName     = map[string]string{
		"sprint": TypeIteration, "iteration": TypeIteration,
		"assignee": TypePerson, "assignees": TypePerson, "reviewer": TypePerson, "owner": TypePerson,
		"due": TypeDate, "deadline": TypeDate,
		"parent": TypeIssue, "blocked-by": TypeIssue, "blocks": TypeIssue, "depends-on": TypeIssue, "duplicate-of": TypeIssue,
	}
)

// TypeOf is a field's type: its own `type`; else `numeric = true` (a
// number); else a role ([roles]), a person; else what its name says (sprint, assignee, due, parent…);
// else what all its values are (dates, issue references); else select.
func (s *Schema) TypeOf(name string, values []string) string {
	f := s.Fields[name]
	switch {
	case f.Type != "":
		return f.Type
	case f.Numeric:
		return TypeNumber
	case s.Roles[name] != "":
		return TypePerson
	case byName[name] != "":
		return byName[name]
	case len(values) > 0 && all(values, dateValue):
		return TypeDate
	case len(values) > 0 && all(values, issueValue):
		return TypeIssue
	}
	return TypeSelect
}

// UnitOf is a number field's unit: its own, else h for est (the hours it
// always showed).
func (s *Schema) UnitOf(name string) string {
	f := s.Fields[name]
	if f.Unit != "" {
		return f.Unit
	}
	if name == "est" && s.TypeOf(name, nil) == TypeNumber {
		return "h"
	}
	return ""
}

func all(values []string, re *regexp.Regexp) bool {
	for _, v := range values {
		if !re.MatchString(v) {
			return false
		}
	}
	return true
}

// Labels is the [labels] table: the plain labels the project allows
// besides its fields (docs/design/issue-head, open point 7), yes-or-no
// flags that can stack and that no field holds (needs-design, regression).
// The issue's head offers them as checkboxes, never free text; Settings ›
// Labels sends them to GitHub with their colors and descriptions; lint
// reports the plain labels outside them. No table, no labels to pick.
type Labels struct {
	List         []string          `toml:"list" json:"list"`
	Colors       map[string]string `toml:"colors" json:"colors"`             // a palette name or #rrggbb; else the palette in turn
	Descriptions map[string]string `toml:"descriptions" json:"descriptions"` // GitHub's, 100 characters at most
	// Defined: the file has a [labels] table, an empty list too (no
	// label allowed), which lint then holds the issues to
	Defined bool `toml:"-" json:"-"`
}

// Sprint is one entry in [[sprints.list]].
type Sprint struct {
	Slug  string    `toml:"slug" json:"slug"`
	Title string    `toml:"title" json:"title"`
	Start time.Time `toml:"start" json:"start"`
}

// Sprints is the sprint calendar, .git-board.toml's [sprints] table.
type Sprints struct {
	LengthDays   int      `toml:"length_days" json:"length_days"`
	StartWeekday string   `toml:"start_weekday" json:"start_weekday"`
	List         []Sprint `toml:"list" json:"list"`
}

// dateOnly strips time-of-day/location so sprint boundaries compare by
// calendar day regardless of the TOML decoder's local timezone.
func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// sortedSprints returns a copy of list ordered by start date — shared by
// Resolve (current/next/previous neighbour lookup) and Windows
// (end-date computation), so the ordering rule lives in one place.
func sortedSprints(list []Sprint) []Sprint {
	ordered := make([]Sprint, len(list))
	copy(ordered, list)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Start.Before(ordered[j].Start) })
	return ordered
}

// Resolve resolves "current" / "next" / "previous" against the sprint
// calendar. The "current" sprint is the last one whose start date is on or
// before today, unless that is the last sprint and it has ended
// (Windows' end): then there is none, and "previous" is it.
// next/previous are its neighbours in start-date order.
func (sprints Sprints) Resolve(which string, today time.Time) (*Sprint, error) {
	today = dateOnly(today)
	ordered := sortedSprints(sprints.List)

	idx := -1
	for i, s := range ordered {
		if !dateOnly(s.Start).After(today) {
			idx = i
		} else {
			break
		}
	}

	// the last sprint ends: past it there is no current one, and the
	// previous is the one that ended
	if idx >= 0 && idx == len(ordered)-1 && !today.Before(dateOnly(ordered[idx].Start).AddDate(0, 0, lastSprintDays(sprints, ordered))) {
		if which == "previous" {
			return &ordered[idx], nil
		}
		if which == "current" {
			return nil, nil
		}
	}

	switch which {
	case "current":
		if idx < 0 {
			return nil, nil
		}
		return &ordered[idx], nil
	case "next":
		nxt := idx + 1
		if nxt >= len(ordered) {
			return nil, nil
		}
		return &ordered[nxt], nil
	case "previous":
		if idx <= 0 {
			return nil, nil
		}
		return &ordered[idx-1], nil
	default:
		return nil, fmt.Errorf("unknown sprint alias: %q", which)
	}
}

// defaultSprintDays is the default file's length_days.
const defaultSprintDays = 14

// lastSprintDays is how long the last sprint of ordered lasts: length_days,
// else as long as the one before it, else defaultSprintDays. Never 0: a
// sprint that ended the day it started stayed @current for ever.
func lastSprintDays(sprints Sprints, ordered []Sprint) int {
	if sprints.LengthDays > 0 {
		return sprints.LengthDays
	}
	if n := len(ordered); n >= 2 {
		if d := int(dateOnly(ordered[n-1].Start).Sub(dateOnly(ordered[n-2].Start)).Hours() / 24); d > 0 {
			return d
		}
	}
	return defaultSprintDays
}

// SprintWindow is a configured sprint plus its computed end date.
type SprintWindow struct {
	Sprint
	End time.Time
}

// Windows returns every configured sprint, ordered by start date (the
// same ordering Resolve uses — see sortedSprints), with End computed
// as the next sprint's start date, or Start+LengthDays for the last one
// (lastSprintDays without it).
// This window math isn't in Resolve itself (which only needs
// start-date ordering to resolve @current/@next/@previous), so it's new
// here rather than duplicated from anywhere.
func (sprints Sprints) Windows() []SprintWindow {
	ordered := sortedSprints(sprints.List)
	windows := make([]SprintWindow, len(ordered))
	for i, s := range ordered {
		end := dateOnly(s.Start).AddDate(0, 0, lastSprintDays(sprints, ordered))
		if i+1 < len(ordered) {
			end = dateOnly(ordered[i+1].Start)
		}
		windows[i] = SprintWindow{Sprint: s, End: end}
	}
	return windows
}
