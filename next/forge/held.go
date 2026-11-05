package forge

import "strings"

// ForgeOf is the forge a sync key names ("github.com/o/r"): the table
// whose host starts the key. No table for the host: a synthetic one, the
// kind from the host's first label, so that the type defaults hold for a
// project the file never named, as for one it did.
func (fs Forges) ForgeOf(key string) Forge {
	for _, f := range fs.List {
		if strings.HasPrefix(strings.ToLower(key), strings.ToLower(f.Host)+"/") {
			return f
		}
	}
	kind := "github"
	if !strings.HasPrefix(strings.ToLower(key), "github.com/") {
		kind = "gitlab"
	}
	return Forge{Kind: kind, Host: strings.SplitN(key, "/", 2)[0]}
}

// Target is where a field lives on forge f: its mapping's target, else,
// on GitHub, the Project when inProject says the field is there, else the
// default for its type.
func Target(fields Fields, inProject func(field string) bool, f Forge, field string) string {
	if m, ok := f.Fields[field]; ok && m.To != "" {
		return m.To
	}
	if f.Kind == "github" && inProject != nil && inProject(field) {
		return TargetProject
	}
	typ, _, _ := fields.Field(field)
	return DefaultTarget(f.Kind, field, typ)
}

// DefaultTarget is the target a field of that type uses on a forge of
// that kind when nothing is written: each a free feature. The assignees
// slot is the field named assignee's; another person field lives in the
// labels.
func DefaultTarget(kind, field, fieldType string) string {
	if field == "assignee" && fieldType == typePerson {
		return TargetAssignees
	}
	switch kind {
	case "gitlab":
		switch fieldType {
		case typeIteration:
			return TargetMilestone
		case typeDate:
			return TargetDueDate
		}
	default: // github, and the shape any other adapter would take
		switch fieldType {
		case typeIssue:
			return TargetSubIssues
		case typeText:
			return TargetNone
		}
	}
	return TargetLabel
}

// typeText is a text field's type, as the issue domain spells it.
const typeText = "text"

// Held says whether the sync keeps a label off the forge of key: when its
// field's home there (Target) is not a label. The milestone is the
// forge's own, never ours. A label without a namespace is never held.
func (fs Forges) Held(fields Fields, inProject func(field string) bool, key, label string) bool {
	field, _, ok := strings.Cut(label, ":")
	if !ok {
		return false
	}
	field = strings.ToLower(field) // GitHub's label names ignore case
	if field == "milestone" {
		return true
	}
	switch Target(fields, inProject, fs.ForgeOf(key), field) {
	case TargetLabel, TargetScopedLabel:
		return false
	}
	return true
}
