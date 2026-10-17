package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

// A field's type says what its values are, and so how git-board sorts,
// filters, shows and edits them: together the fields and their types are
// the project's ontology (docs/configuration.md, Types). The values stay
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
func (c *Config) TypeOf(name string, values []string) string {
	f := c.Fields[name]
	switch {
	case f.Type != "":
		return f.Type
	case f.Numeric:
		return TypeNumber
	case c.Roles[name] != "":
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
func (c *Config) UnitOf(name string) string {
	f := c.Fields[name]
	if f.Unit != "" {
		return f.Unit
	}
	if name == "est" && c.TypeOf(name, nil) == TypeNumber {
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

// checkTypes refuses a type git-board doesn't know.
func checkTypes(fields map[string]Field) error {
	for name, f := range fields {
		if f.Type != "" && !slices.Contains(Types, f.Type) {
			return fmt.Errorf("fields.%s: type %q isn't one of %s", name, f.Type, strings.Join(Types, ", "))
		}
	}
	return nil
}
