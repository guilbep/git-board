package config

import (
	"fmt"
	"maps"
	"slices"
	"strings"
)

// A field's per-forge mapping: where it is written and read on one forge
// (docs/prd/forge-field-mappings.md). Only the overrides are written
// down — a field with no mapping follows its type's default (Default),
// so a new project syncs with no mapping written.

// The targets, as `to` spells them. One per forge slot; a field is read
// back from its target alone.
const (
	TargetLabel       = "label"        // the namespaced label, status:ready (or plain, ready)
	TargetScopedLabel = "scoped-label" // GitLab's status::ready, paid tier
	TargetProject     = "project"      // a GitHub Project field ([github-project])
	TargetAssignees   = "assignees"    // the forge's native assignees
	TargetMilestone   = "milestone"    // the forge's milestone, a sprint's home on GitLab
	TargetIteration   = "iteration"    // GitLab iterations, paid tier
	TargetWeight      = "weight"       // GitLab's weight, a number's home, paid tier
	TargetDueDate     = "due-date"     // the forge's due date
	TargetSubIssues   = "sub-issues"   // GitHub's sub-issues, an issue field's home
	TargetNone        = "none"         // git-board only, never written
)

// FieldMapping is one [forges.<name>.fields.<field>] table: only what
// differs from the type's default.
type FieldMapping struct {
	// To is the target: label, scoped-label, project, assignees,
	// milestone, iteration, weight, due-date, sub-issues, none.
	To string `toml:"to" json:"to"`
	// As is how a label spells its value: namespaced (the default,
	// status:ready) or plain (ready). Labels only; plain is refused when
	// two fields share a value, the label can't be traced back then.
	As string `toml:"as" json:"as,omitempty"`
	// Field is which GitHub Project field to use, for project alone; the
	// default is the git-board field's name.
	Field string `toml:"field" json:"field,omitempty"`
	// Values rename the values on this forge: the git-board value to the
	// name it carries there (in-progress = "In Progress"). Replaces the
	// old [fields.X] project map.
	Values map[string]string `toml:"values" json:"values,omitempty"`
}

// FieldTarget is where field lives on the forge: its mapping's `to`, else
// the type's default (Default), else the old [fields.X] project map's
// reading — a GitHub field with renames there lived in the Project
// already, and keeps doing so until the mapping is written.
func (c *Config) FieldTarget(f Forge, field string) string {
	if m, ok := f.Fields[field]; ok && m.To != "" {
		return m.To
	}
	if f.Kind == "github" {
		if pf, ok := c.Fields[field]; ok && len(pf.Project) > 0 {
			return TargetProject
		}
	}
	return DefaultTarget(f.Kind, c.TypeOf(field, c.Fields[field].Options))
}

// DefaultTarget is the target a field of that type uses on the forge when
// nothing is written: chosen to work on any plan, every default a free
// feature. The type is TypeOf's spelling (select, iteration, person,
// number, date, issue, text).
func DefaultTarget(kind, fieldType string) string {
	switch kind {
	case "gitlab":
		switch fieldType {
		case TypePerson:
			return TargetAssignees
		case TypeIteration:
			return TargetMilestone
		case TypeDate:
			return TargetDueDate
		}
		return TargetLabel
	default: // github, and the shape any Gitea/Forgejo adapter would take
		switch fieldType {
		case TypePerson:
			return TargetAssignees
		case TypeIssue:
			return TargetSubIssues
		case TypeText:
			return TargetNone
		}
		return TargetLabel
	}
}

// FieldValues is the field's value renames on the forge: the mapping's,
// else the old [fields.X] project map when the forge is a GitHub one —
// read as [forges.gh.fields.<field>.values] since the mappings exist;
// lint suggests moving it.
func (c *Config) FieldValues(f Forge, field string) map[string]string {
	if m, ok := f.Fields[field]; ok && len(m.Values) > 0 {
		return m.Values
	}
	if f.Kind == "github" {
		return c.Fields[field].Project
	}
	return nil
}

// FieldLabel is the label a field's value carries on the forge: plain
// (ready) when the mapping says so, namespaced (status:ready) otherwise —
// the file's own spelling, since the values in git-bug stay namespaced
// labels either way.
func (c *Config) FieldLabel(f Forge, field, value string) string {
	if m, ok := f.Fields[field]; ok && m.To == TargetLabel && m.As == "plain" {
		return value
	}
	return c.Label(field, value)
}

// forgeOf is the forge a sync key names ("github.com/o/r"): its host
// starts the key.
func (c *Config) forgeOf(key string) (Forge, bool) {
	for _, f := range c.Forges {
		if strings.HasPrefix(strings.ToLower(key), strings.ToLower(f.Host)+"/") {
			return f, true
		}
	}
	return Forge{}, false
}

// checkFieldMappings refuses a mapping the field's type or the forge
// can't carry, with the table's name in the message: the editor names the
// line from it.
func checkFieldMappings(c *Config) error {
	for _, f := range c.Forges {
		for _, name := range slices.Sorted(maps.Keys(f.Fields)) {
			m := f.Fields[name]
			where := fmt.Sprintf("[forges.%s.fields.%s]", f.Name, name)
			field, ok := c.Fields[name]
			if !ok {
				return fmt.Errorf("%s: no [fields.%s] table declares it", where, name)
			}
			t := c.TypeOf(name, field.Options)
			if err := checkTarget(where, m.To, t, f.Kind); err != nil {
				return err
			}
			if m.To == "" { // details alone are nothing to validate further
				continue
			}
			if m.As != "" && m.As != "namespaced" && m.As != "plain" {
				return fmt.Errorf("%s as: namespaced or plain, not %q", where, m.As)
			}
			if m.As != "" && m.To != TargetLabel {
				return fmt.Errorf("%s as: a label's spelling, not a %s's", where, m.To)
			}
			if m.Field != "" && m.To != TargetProject {
				return fmt.Errorf("%s field: a project's key, not a %s's", where, m.To)
			}
			if m.Field == "" && m.To == TargetProject {
				return fmt.Errorf("%s field: which Project field, not the default (the git-board name)", where)
			}
			if len(m.Values) > 0 && m.To != TargetProject && m.To != TargetScopedLabel {
				return fmt.Errorf("%s values: renames on a %s, not a %s's", where, m.To, TargetLabel)
			}
			for v, on := range m.Values {
				if !field.KnownValue(v) {
					return fmt.Errorf("%s values: %q is not an option", where, v)
				}
				if strings.TrimSpace(on) == "" {
					return fmt.Errorf("%s values: %q's name is empty", where, v)
				}
			}
			if m.As == "plain" {
				if !field.Single {
					return fmt.Errorf("%s as: plain labels fit a one-value field; %s takes several", where, name)
				}
				for other, of := range c.Fields {
					if other == name {
						continue
					}
					for _, v := range of.Options {
						if slices.ContainsFunc(field.Options, func(o string) bool { return strings.EqualFold(o, v) }) {
							return fmt.Errorf("%s as: plain refused, %s's values share %q", where, other, v)
						}
					}
				}
			}
		}
	}
	return nil
}

// checkTarget refuses a target the type can't live in, or the forge
// doesn't offer.
func checkTarget(where, to, fieldType, kind string) error {
	switch to {
	case "", TargetLabel, TargetNone:
		return nil
	case TargetProject:
		if kind != "github" {
			return fmt.Errorf("%s to: project is GitHub's, not a %s's", where, kind)
		}
		switch fieldType {
		case TypePerson, TypeIssue:
			return fmt.Errorf("%s to: a %s field doesn't live in a Project column", where, fieldType)
		}
	case TargetScopedLabel:
		if kind != "gitlab" {
			return fmt.Errorf("%s to: scoped-label is GitLab's, not a %s's", where, kind)
		}
		if fieldType != TypeSelect {
			return fmt.Errorf("%s to: a %s field doesn't live in a scoped label", where, fieldType)
		}
	case TargetAssignees:
		if fieldType != TypePerson {
			return fmt.Errorf("%s to: assignees hold a person, not a %s", where, fieldType)
		}
	case TargetMilestone:
		if fieldType != TypeIteration {
			return fmt.Errorf("%s to: a milestone holds a sprint, not a %s", where, fieldType)
		}
	case TargetIteration:
		if kind != "gitlab" {
			return fmt.Errorf("%s to: iteration is GitLab's, not a %s's", where, kind)
		}
		if fieldType != TypeIteration {
			return fmt.Errorf("%s to: an iteration holds a sprint, not a %s", where, fieldType)
		}
	case TargetWeight:
		if kind != "gitlab" {
			return fmt.Errorf("%s to: weight is GitLab's, not a %s's", where, kind)
		}
		if fieldType != TypeNumber {
			return fmt.Errorf("%s to: weight holds a number, not a %s", where, fieldType)
		}
	case TargetDueDate:
		if fieldType != TypeDate {
			return fmt.Errorf("%s to: a due date holds a date, not a %s", where, fieldType)
		}
	case TargetSubIssues:
		if kind != "github" {
			return fmt.Errorf("%s to: sub-issues are GitHub's, not a %s's", where, kind)
		}
		if fieldType != TypeIssue {
			return fmt.Errorf("%s to: sub-issues hold an issue reference, not a %s", where, fieldType)
		}
	default:
		return fmt.Errorf("%s to: label, scoped-label, project, assignees, milestone, iteration, weight, due-date, sub-issues or none, not %q", where, to)
	}
	return nil
}

// checkAliases refuses an alias that shadows an option or folds into
// nothing: could = "couldhave" reads moscow:could as couldhave, so
// couldhave must be an option and could must not be one.
func checkAliases(fields map[string]Field) error {
	for name, f := range fields {
		for old, cur := range f.Aliases {
			if strings.TrimSpace(old) == "" || strings.TrimSpace(cur) == "" {
				return fmt.Errorf("fields.%s.aliases: an empty name", name)
			}
			if slices.Contains(f.Options, old) {
				return fmt.Errorf("fields.%s.aliases: %q is an option already; an alias folds a retired value into a current one", name, old)
			}
			if !slices.Contains(f.Options, cur) {
				return fmt.Errorf("fields.%s.aliases: %q is not an option", name, cur)
			}
		}
	}
	return nil
}
