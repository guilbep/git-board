// The forges' tables of .git-board.toml, [forges] and [repos], with
// v1's rules.

package forge

import (
	"cmp"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/guilbep/git-board/next/conf"
)

// Table is a [forges.<name>] table: a forge project the issues sync with.
// Its token is never here.
type Table struct {
	Name    string `toml:"-"`
	Kind    string `toml:"kind"`    // github, gitlab
	Project string `toml:"project"` // owner/repo; GitLab's numeric id
	Host    string `toml:"host"`    // default github.com, gitlab.com
	Login   string `toml:"login"`   // the posting account
	Sync    bool   `toml:"sync"`
	// Fields maps a field to where it lives on this forge: only the
	// overrides of its type's default.
	Fields map[string]FieldMapping `toml:"fields"`
	// Discovered: found in the git remotes, with no table. A GitLab one's
	// Project is its path until the run reads its id; Path keeps the path.
	Discovered bool   `toml:"-"`
	Path       string `toml:"-"`
}

// FieldMapping is one [forges.<name>.fields.<field>].
type FieldMapping struct {
	To     string            `toml:"to"`
	As     string            `toml:"as"`
	Field  string            `toml:"field"`
	Values map[string]string `toml:"values"`
}

// The targets, as `to` spells them.
const (
	TargetLabel       = "label"
	TargetScopedLabel = "scoped-label"
	TargetProject     = "project"
	TargetAssignees   = "assignees"
	TargetMilestone   = "milestone"
	TargetIteration   = "iteration"
	TargetWeight      = "weight"
	TargetDueDate     = "due-date"
	TargetSubIssues   = "sub-issues"
	TargetNone        = "none"
)

// Forges is the [forges] table read.
type Forges struct {
	List           []Table
	PublishPrivate bool // [forges] publish-private
	Discover       bool // no table and no discover = false: from the git remotes
}

// Repos is [repos]. Its assignees, which no git-board read, schema 2
// sets aside (docs/guide/config-schema.md).
type Repos struct {
	List []string `toml:"list"`
}

// Fields is what a mapping's check asks of the issue domain's fields.
type Fields interface {
	// Field says the field is declared, its type (TypeOf's), and whether a
	// value is one of its options.
	Field(name string) (typ string, known func(value string) bool, ok bool)
}

// Tables are the forges' tables on a file.
type Tables struct {
	forges *conf.Handle[map[string]toml.Primitive]
	repos  *conf.Handle[Repos]
}

// Declare declares [forges] and [repos] on f.
func Declare(f *conf.File) Tables {
	return Tables{
		forges: conf.Declare(f, conf.Table[map[string]toml.Primitive]{Name: "forges"}),
		repos:  conf.Declare(f, conf.Table[Repos]{Name: "repos"}),
	}
}

// Repos is [repos].
func (t Tables) Repos() (Repos, []conf.Problem) { return t.repos.Get() }

var (
	forgeName  = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)
	ghProject  = regexp.MustCompile(`^[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+$`)
	glProject  = regexp.MustCompile(`^[0-9]+$`)
	forgeHost  = regexp.MustCompile(`^[A-Za-z0-9.-]+(:[0-9]+)?$`)
	forgeLogin = regexp.MustCompile(`^[A-Za-z0-9_.-]*$`)
)

// Forges is [forges] read and checked, its mappings against fields.
func (t Tables) Forges(fields Fields) (Forges, []conf.Problem) {
	tables, probs := t.forges.Get()
	if len(probs) > 0 {
		return Forges{Discover: true}, probs
	}
	res, err := t.parse(tables)
	if err == nil {
		err = checkMappings(res.List, fields)
	}
	if err != nil {
		return res, []conf.Problem{{Table: "forges", Err: err}}
	}
	return res, nil
}

func (t Tables) parse(tables map[string]toml.Primitive) (Forges, error) {
	res := Forges{Discover: true}
	for _, name := range slices.Sorted(maps.Keys(tables)) {
		if name == "publish-private" || name == "discover" {
			v := map[string]*bool{"publish-private": &res.PublishPrivate, "discover": &res.Discover}[name]
			if err := t.forges.Decode(tables[name], v); err != nil {
				return res, fmt.Errorf("[forges] %s: %w", name, err)
			}
			continue
		}
		if !forgeName.MatchString(name) {
			return res, fmt.Errorf("[forges.%s]: a forge's name is letters, digits, - and _", name)
		}
		f := Table{Name: name}
		if err := t.forges.Decode(tables[name], &f); err != nil {
			return res, fmt.Errorf("[forges.%s]: %w", name, err)
		}
		if err := checkForge(&f); err != nil {
			return res, err
		}
		res.List = append(res.List, f)
	}
	res.Discover = res.Discover && len(res.List) == 0
	return res, nil
}

// checkForge fills the host's default, and refuses a table git-board
// can't sync with.
func checkForge(f *Table) error {
	where := "[forges." + f.Name + "]"
	switch f.Kind {
	case "github":
		f.Host = cmp.Or(f.Host, "github.com")
		if !ghProject.MatchString(f.Project) {
			return fmt.Errorf("%s project: owner/repo, not %q", where, f.Project)
		}
	case "gitlab":
		f.Host = cmp.Or(f.Host, "gitlab.com")
		if !glProject.MatchString(f.Project) {
			return fmt.Errorf("%s project: the project's numeric id, not %q", where, f.Project)
		}
	default:
		return fmt.Errorf("%s kind: github or gitlab, not %q", where, f.Kind)
	}
	if !forgeHost.MatchString(f.Host) {
		return fmt.Errorf("%s host: a host name, not %q", where, f.Host)
	}
	if !forgeLogin.MatchString(f.Login) {
		return fmt.Errorf("%s login: a login, not %q", where, f.Login)
	}
	return nil
}

// checkMappings refuses a mapping the field's type or the forge can't
// carry, naming its table.
func checkMappings(forges []Table, fields Fields) error {
	assignees := map[string]string{} // forge → the field in its one slot
	for _, f := range forges {
		for _, name := range slices.Sorted(maps.Keys(f.Fields)) {
			m := f.Fields[name]
			where := fmt.Sprintf("[forges.%s.fields.%s]", f.Name, name)
			t, known, ok := fields.Field(name)
			if !ok {
				return fmt.Errorf("%s: no [fields.%s] table declares it", where, name)
			}
			if err := checkTarget(where, m.To, t, f.Kind); err != nil {
				return err
			}
			if m.To == TargetAssignees {
				if prev, ok := assignees[f.Name]; ok {
					return fmt.Errorf("%s to: %s has the assignees on this forge already — one slot per forge", where, prev)
				}
				assignees[f.Name] = name
			}
			if m.As != "" {
				return fmt.Errorf("%s as: not applied yet — labels keep the namespaced spelling (status:ready); leave as out until the sync writes it", where)
			}
			if m.To == "" {
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
			for _, v := range slices.Sorted(maps.Keys(m.Values)) {
				if !known(v) {
					return fmt.Errorf("%s values: %q is not an option", where, v)
				}
				if strings.TrimSpace(m.Values[v]) == "" {
					return fmt.Errorf("%s values: %q's name is empty", where, v)
				}
			}
		}
	}
	return nil
}

// The field types a target checks, as the issue domain spells them.
const (
	typeSelect    = "select"
	typeIteration = "iteration"
	typePerson    = "person"
	typeNumber    = "number"
	typeDate      = "date"
	typeIssue     = "issue"
)

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
		if fieldType == typePerson || fieldType == typeIssue {
			return fmt.Errorf("%s to: a %s field doesn't live in a Project column", where, fieldType)
		}
	case TargetScopedLabel:
		if kind != "gitlab" {
			return fmt.Errorf("%s to: scoped-label is GitLab's, not a %s's", where, kind)
		}
		if fieldType != typeSelect {
			return fmt.Errorf("%s to: a %s field doesn't live in a scoped label", where, fieldType)
		}
	case TargetAssignees:
		if fieldType != typePerson {
			return fmt.Errorf("%s to: assignees hold a person, not a %s", where, fieldType)
		}
	case TargetMilestone:
		if fieldType != typeIteration {
			return fmt.Errorf("%s to: a milestone holds a sprint, not a %s", where, fieldType)
		}
	case TargetIteration:
		if kind != "gitlab" {
			return fmt.Errorf("%s to: iteration is GitLab's, not a %s's", where, kind)
		}
		if fieldType != typeIteration {
			return fmt.Errorf("%s to: an iteration holds a sprint, not a %s", where, fieldType)
		}
	case TargetWeight:
		if kind != "gitlab" {
			return fmt.Errorf("%s to: weight is GitLab's, not a %s's", where, kind)
		}
		if fieldType != typeNumber {
			return fmt.Errorf("%s to: weight holds a number, not a %s", where, fieldType)
		}
	case TargetDueDate:
		if fieldType != typeDate {
			return fmt.Errorf("%s to: a due date holds a date, not a %s", where, fieldType)
		}
	case TargetSubIssues:
		if kind != "github" {
			return fmt.Errorf("%s to: sub-issues are GitHub's, not a %s's", where, kind)
		}
		if fieldType != typeIssue {
			return fmt.Errorf("%s to: sub-issues hold an issue reference, not a %s", where, fieldType)
		}
	default:
		return fmt.Errorf("%s to: label, scoped-label, project, assignees, milestone, iteration, weight, due-date, sub-issues or none, not %q", where, to)
	}
	return nil
}
