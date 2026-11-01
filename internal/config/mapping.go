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
	TargetLabel       = "label"        // the namespaced label, status:ready
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
	// As is how a label spells its value: namespaced (status:ready) or
	// plain (ready). Planned, not applied: nothing writes a plain label
	// yet, so the parse refuses it — leaving it out keeps the file honest
	// until the sync spells labels the mapping's way.
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
// the old homes the file already implies — a GitHub field with a project
// map lived in the Project, and with [github-project] configured the
// fields it always synced still do — else the type's default (DefaultTarget).
func (c *Config) FieldTarget(f Forge, field string) string {
	if m, ok := f.Fields[field]; ok && m.To != "" {
		return m.To
	}
	if f.Kind == "github" {
		if len(c.Fields[field].Project) > 0 {
			return TargetProject
		}
		// [github-project] configured: the fields it always synced live
		// there, declared or not — today's behavior, kept
		if c.GithubProject.Number != 0 && slices.Contains(ProjectFields, field) {
			return TargetProject
		}
	}
	return DefaultTarget(f.Kind, field, c.TypeOf(field, c.Fields[field].Options))
}

// DefaultTarget is the target a field of that type uses on the forge when
// nothing is written: chosen to work on any plan, every default a free
// feature. The type is TypeOf's spelling (select, iteration, person,
// number, date, issue, text). The assignees slot is one per forge, and
// the forges' sync reads and writes it for the field named assignee —
// another person field (reviewer) lives in its labels until a mapping
// says otherwise.
func DefaultTarget(kind, field, fieldType string) string {
	switch kind {
	case "gitlab":
		switch {
		case field == "assignee" && fieldType == TypePerson:
			return TargetAssignees
		case fieldType == TypeIteration:
			return TargetMilestone
		case fieldType == TypeDate:
			return TargetDueDate
		}
		return TargetLabel
	default: // github, and the shape any Gitea/Forgejo adapter would take
		switch {
		case field == "assignee" && fieldType == TypePerson:
			return TargetAssignees
		case fieldType == TypeIssue:
			return TargetSubIssues
		case fieldType == TypeText:
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

// forgeOf is the forge a sync key names ("github.com/o/r"): its host
// starts the key. No table for the host: a synthetic one, the kind from
// the host's first label — the type defaults hold for a project the file
// never named, as for one it did.
func (c *Config) forgeOf(key string) (Forge, bool) {
	for _, f := range c.Forges {
		if strings.HasPrefix(strings.ToLower(key), strings.ToLower(f.Host)+"/") {
			return f, true
		}
	}
	kind := "github"
	if !strings.HasPrefix(strings.ToLower(key), "github.com/") {
		kind = "gitlab"
	}
	return Forge{Kind: kind, Host: strings.SplitN(key, "/", 2)[0]}, true
}

// GitHubForge is the config's GitHub forge: the first declared, else a
// synthetic one for github.com. The Project sync and the type defaults
// name the kind, not the table.
func (c *Config) GitHubForge() Forge {
	for _, f := range c.Forges {
		if f.Kind == "github" {
			return f
		}
	}
	return Forge{Name: "gh", Kind: "github", Host: "github.com"}
}

// checkFieldMappings refuses a mapping the field's type or the forge
// can't carry, with the table's name in the message: the editor names the
// line from it.
func checkFieldMappings(c *Config) error {
	assignees := map[string]string{} // forge name → the field in its one slot
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
			if m.To == TargetAssignees {
				if prev, ok := assignees[f.Name]; ok {
					return fmt.Errorf("%s to: %s has the assignees on this forge already — one slot per forge", where, prev)
				}
				assignees[f.Name] = name
			}
			// as is planned, not applied: nothing writes a plain label
			// yet, so a file carrying one would silently change nothing
			// (and the migration would remove the wrong labels). Refused
			// until the sync spells labels the mapping's way.
			if m.As != "" {
				return fmt.Errorf("%s as: not applied yet — labels keep the namespaced spelling (status:ready); leave as out until the sync writes it", where)
			}
			if m.To == "" { // the details' fit is the target's to say
				if m.Field != "" {
					return fmt.Errorf("%s field: a project's key, and to is empty", where)
				}
				if len(m.Values) > 0 {
					return fmt.Errorf("%s values: renames of a target, and to is empty", where)
				}
				continue
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
