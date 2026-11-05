// Package ghproject is the GitHub Project bridge: a Project's fields
// (single select, iteration, number, date, text) kept in step with the
// board's fields, which are labels (ADR 0007), both ways. It is the sync
// engine's third mapper, after the issues and the releases (design.md,
// Sync). v1's internal/ghbridge, copied and owned; v1 read the Project
// with the gh CLI, v2 through GitHub's GraphQL API (forge/github's
// ProjectV2), with the forge's token (ADR 0010).
//
// Each field of each issue is decided from three values: the Project's,
// the label's, and the one both last agreed on (the base, recorded in the
// issue's metadata, under git-board:<owner>/<number>:<field>). Only the
// side that changed since is taken; when both changed differently, the
// field is held, left alone on both sides, until someone resolves it.
package ghproject

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/forge"
	"github.com/guilbep/git-board/next/forge/github"
)

// Config is what the bridge reads of .git-board.toml: [github-project],
// the fields, and the GitHub forge's mappings.
type Config struct {
	Owner  string
	Number int
	// Write lets a sync write git-board's changes to the Project. Off by
	// default: the Project is only read.
	Write  bool
	Schema *issues.Schema
	// Forge is the GitHub forge's table, its fields' mappings: the first
	// GitHub one, else github.com's with none.
	Forge forge.Table
}

// ProjectFields are the fields the bridge keeps in step with its Project
// once [github-project] names one, declared or not: the Project's own
// columns.
var ProjectFields = []string{"status", "prio", "mod", "est", "sprint", "moscow", "milestone"}

// Configured says [github-project] names a Project.
func (c *Config) Configured() bool { return c.Owner != "" && c.Number != 0 }

// Name is the Project as people name it: owner/number.
func (c *Config) Name() string { return fmt.Sprintf("%s/%d", c.Owner, c.Number) }

// field is a field's declaration; the zero one when the file has none.
func (c *Config) field(name string) issues.Field {
	if c.Schema == nil {
		return issues.Field{}
	}
	return c.Schema.Fields[name]
}

// InProject says the field's home on GitHub is the Project without a
// mapping saying so: its [fields.X] project map, or [github-project]
// configured and one of its columns.
func (c *Config) InProject(name string) bool {
	return len(c.field(name).Project) > 0 || c.Number != 0 && slices.Contains(ProjectFields, name)
}

// Target is where the field lives on GitHub (forge.Target).
func (c *Config) Target(name string) string {
	schema := c.Schema
	if schema == nil {
		schema = &issues.Schema{}
	}
	return forge.Target(schema, c.InProject, c.Forge, name)
}

// values is the field's value renames on GitHub: the mapping's, else its
// [fields.X] project map.
func (c *Config) values(name string) map[string]string {
	if m, ok := c.Forge.Fields[name]; ok && len(m.Values) > 0 {
		return m.Values
	}
	return c.field(name).Project
}

// order is the fields' names as the file lists them.
func (c *Config) order() []string {
	if c.Schema == nil {
		return nil
	}
	return c.Schema.FieldOrder()
}

// kinds are [branches] kinds.
func (c *Config) kinds() map[string]string {
	if c.Schema == nil {
		return nil
	}
	return c.Schema.Kinds
}

// label is the label of a field's value: a flat kind is its plain label.
func (c *Config) label(field, value string) string {
	if _, ok := c.kinds()[value]; ok && field == "kind" && c.Schema.FlatKinds() {
		return value
	}
	return field + ":" + value
}

// kindOf is the kind the labels give: kind:<k>, else a [branches] kind.
func (c *Config) kindOf(labels []string) string {
	for _, l := range labels {
		if v, ok := strings.CutPrefix(l, "kind:"); ok {
			return v
		}
	}
	for _, l := range slices.Sorted(slices.Values(labels)) {
		if _, ok := c.kinds()[l]; ok {
			return l
		}
	}
	return ""
}

// Decision is what syncing one field does.
type Decision int

const (
	// Agreed: both sides have the same value.
	Agreed Decision = iota
	// TakeProject: only the Project changed; the label follows.
	TakeProject
	// TakeLocal: only git-board changed; the Project follows.
	TakeLocal
	// Held: both changed differently; a person decides.
	Held
)

func (d Decision) String() string {
	return [...]string{"agreed", "take project", "take local", "held"}[d]
}

// Decide decides one field from the Project's value, git-board's, and the
// one both last agreed on. "" is a value: an empty field, a missing label.
func Decide(project, local, base string, hasBase bool) Decision {
	switch {
	// first, so a run stopped halfway is recorded by the next one
	case project == local:
		return Agreed
	case !hasBase:
		// a field never set on the Project can't be told from one a
		// teammate cleared
		if project == "" {
			return Held
		}
		return TakeProject
	case local == base:
		return TakeProject
	case project == base:
		return TakeLocal
	default:
		return Held
	}
}

// Item is one row of the Project.
type Item = github.ProjectItem

// Meta is what writing to a Project takes: its id, and its fields by
// name; and its title, to show it answers.
type Meta struct {
	ID     string
	Title  string
	Fields map[string]Field
	// Assignable is who can be assigned, by repository ("owner/name"):
	// GitHub silently drops anyone else. nil when not looked up.
	Assignable map[string][]string
}

// Field is a Project field: its id, its type, and its options or
// iterations, by name, to their id.
type Field struct {
	ID      string
	Type    string // SINGLE_SELECT, ITERATION, NUMBER, DATE, TEXT…
	Options map[string]string
}

// MetaOf is a Project's meta as GitHub answers it.
func MetaOf(m github.ProjectMeta) Meta {
	out := Meta{ID: m.ID, Title: m.Title, Fields: map[string]Field{}}
	for _, f := range m.Fields {
		pf := Field{ID: f.ID, Type: f.DataType, Options: map[string]string{}}
		for _, c := range f.Choices {
			pf.Options[c.Name] = c.ID
		}
		out.Fields[f.Name] = pf
	}
	return out
}

// field is the Project's column of that name, in any case.
func (m Meta) field(column string) (Field, bool) {
	if f, ok := m.Fields[column]; ok {
		return f, true
	}
	for _, name := range slices.Sorted(maps.Keys(m.Fields)) {
		if strings.EqualFold(name, column) {
			return m.Fields[name], true
		}
	}
	return Field{}, false
}

// valueOf is the item's value of a column, in any case.
func valueOf(it Item, column string) string {
	if v, ok := it.Values[column]; ok {
		return v
	}
	for _, name := range slices.Sorted(maps.Keys(it.Values)) {
		if strings.EqualFold(name, column) {
			return it.Values[name]
		}
	}
	return ""
}

// The kinds of an Edit: a Project field's (github.Value*), or the
// assignees, the GitHub issue's own.
const (
	EditOption    = github.ValueOption
	EditNumber    = github.ValueNumber
	EditIteration = github.ValueIteration
	EditDate      = github.ValueDate
	EditText      = github.ValueText
	EditClear     = github.ValueClear
	EditAssignees = "assignees"
)

// Edit is one field written to one Project item.
type Edit struct {
	IssueID string
	ItemID  string
	Number  int
	Field   string // the git-board field
	Value   string // its value, "" to clear
	FieldID string
	Kind    string
	Arg     string // the option or iteration id, the number, date or text

	URL         string   // EditAssignees: the issue
	Add, Remove []string // EditAssignees: the logins
}

// Conflict is a field both sides changed differently since they last
// agreed.
type Conflict struct {
	IssueID string `json:"issue"`
	Number  int    `json:"number"`
	URL     string `json:"url"`
	Title   string `json:"title"`
	Field   string `json:"field"`
	Base    string `json:"base"`
	HasBase bool   `json:"has_base"`
	Local   string `json:"local"`   // git-board's value
	Project string `json:"project"` // the Project's
}

// FieldResult is one field's planned label change for one issue.
type FieldResult struct {
	Field   string
	Value   string // the value that was (or would be) set
	Added   []string
	Removed []string
}

// Change is the planned label changes of one matched item and issue.
type Change struct {
	IssueID  string
	URL      string
	Number   int
	Title    string
	Fields   []FieldResult
	Added    []string // every field's Added
	Removed  []string // every field's Removed
	Warnings []string // a value with no option, several labels of a field…
}

// Unmatched is an item that couldn't be matched: not an issue (a pull
// request or a draft added to the Project), or an issue no issue here is
// linked to.
type Unmatched struct {
	Number     int    `json:"number"`
	URL        string `json:"url"`
	Title      string `json:"title"`
	Reason     string `json:"reason"`
	NotAnIssue bool   `json:"not_an_issue"`
}

// Plan is what a sync would do.
type Plan struct {
	Changes   []Change // one per matched issue
	Edits     []Edit
	Conflicts []Conflict
	// Bases are the values to record per issue once applied: the fields
	// both sides agree on afterwards, where that changed.
	Bases     map[string]map[string]string
	Unmatched []Unmatched
}

// Issue is the board's side of one issue: its id, its GitHub issue, its
// labels as git-board reads them, and what it last agreed on with the
// Project.
type Issue struct {
	ID     string
	URL    string
	Labels []string
}

// synced is one field the sync keeps in step, by the Project's column.
type synced struct {
	name, column string
	value        func(Item) string
	readOnly     bool
}

// known are the Project columns the known fields use when no mapping
// names one.
var known = map[string]string{
	"status": "Status", "prio": "Priority", "mod": "Module", "est": "Est. hours",
	"sprint": "Sprint", "moscow": "MoSCoW", "milestone": "Milestone",
}

// readOnly are the synced fields only GitHub writes: the Project shows the
// issue's milestone, which a Project edit can't set. The Project's value
// always wins.
// ponytail: the title alone, as a slug; its due date and a write back
// (the issue's milestone, by title) when a project needs them.
var readOnly = map[string]bool{"milestone": true}

// Columns is each field the Project holds by its column there.
func (c *Config) Columns() map[string]string {
	res := map[string]string{}
	for _, f := range c.synced() {
		res[f.column] = f.name
	}
	return res
}

// Column is the Project's column of a field whose home is the Project, ""
// for another.
func (c *Config) Column(name string) string {
	for _, f := range c.synced() {
		if f.name == name {
			return f.column
		}
	}
	return ""
}

// synced is the fields whose GitHub home is the Project, in the file's
// order then the Project's own columns, each with the column the mapping
// names, else the known one, else the field's own, capitalized.
func (c *Config) synced() []synced {
	res := []synced{}
	seen := map[string]bool{}
	add := func(name string) {
		if seen[name] || c.Target(name) != forge.TargetProject {
			return
		}
		seen[name] = true
		column := c.Forge.Fields[name].Field
		col, isKnown := known[name]
		switch {
		case column == "" && isKnown:
			column = col
		case column == "":
			column = titleCase(name)
		case isKnown && strings.EqualFold(column, col):
			column = col
		case !isKnown:
			column = titleCase(column)
		}
		value := func(it Item) string { return valueOf(it, column) }
		switch name {
		case "mod":
			value = func(it Item) string { return normalizeModule(valueOf(it, column)) }
		case "milestone":
			value = func(it Item) string { return it.Milestone }
		}
		res = append(res, synced{name: name, column: column, value: value, readOnly: readOnly[name]})
	}
	for _, name := range c.order() {
		add(name)
	}
	for _, name := range ProjectFields {
		add(name)
	}
	return res
}

// titleCase is a column's name with a capital, as the Project's own are.
func titleCase(name string) string {
	if name == "" {
		return name
	}
	r := []rune(name)
	return string(unicode.ToUpper(r[0])) + string(r[1:])
}

// BasePrefix is where a Project's bases are recorded on issues.
func (c *Config) BasePrefix() string {
	return fmt.Sprintf("git-board:%s/%d:", strings.ToLower(c.Owner), c.Number)
}

// BuildPlan decides every synced field of every matched item, without
// writing anything. bases are the recorded bases, by issue id then field.
func BuildPlan(items []Item, list []Issue, bases map[string]map[string]string, meta Meta, c *Config) Plan {
	byURL := make(map[string]Issue, len(list))
	for _, is := range list {
		if is.URL != "" {
			byURL[is.URL] = is
		}
	}
	plan := Plan{Bases: map[string]map[string]string{}}
	for _, it := range items {
		if it.Type != "Issue" {
			plan.Unmatched = append(plan.Unmatched, Unmatched{Number: it.Number, URL: it.URL, Title: it.Title,
				Reason: fmt.Sprintf("not an issue (content type %s), excluded", it.Type), NotAnIssue: true})
			continue
		}
		is, ok := byURL[it.URL]
		if !ok {
			plan.Unmatched = append(plan.Unmatched, Unmatched{Number: it.Number, URL: it.URL, Title: it.Title,
				Reason: "no matching issue here (not pulled from the forge yet? git board sync)"})
			continue
		}
		syncIssue(&plan, it, is, bases[is.ID], meta, c)
	}
	return plan
}

func syncIssue(plan *Plan, it Item, is Issue, bases map[string]string, meta Meta, c *Config) {
	change := Change{IssueID: is.ID, URL: is.URL, Number: it.Number, Title: it.Title}
	newBases := map[string]string{}
	record := func(field, value string) {
		if old, ok := bases[field]; !ok || old != value {
			newBases[field] = value
		}
	}
	setLabel := func(field, value string) {
		res := labelChange(is.Labels, field, value)
		change.Fields = append(change.Fields, res)
		change.Added = append(change.Added, res.Added...)
		change.Removed = append(change.Removed, res.Removed...)
	}

	for _, f := range c.synced() {
		pf, onProject := meta.field(f.column)
		if !onProject {
			continue
		}
		raw := f.value(it)
		p, ok := c.toLabel(pf, f.name, raw)
		if !ok {
			change.Warnings = append(change.Warnings, fmt.Sprintf("%s value %q from the Project has no matching option, field left alone", f.name, raw))
			continue
		}
		l, ok := localValue(is.Labels, f.name)
		if !ok {
			change.Warnings = append(change.Warnings, fmt.Sprintf("several %s labels, field left alone", f.name))
			continue
		}
		base, hasBase := bases[f.name]

		d := Decide(p, l, base, hasBase)
		if f.readOnly && d != Agreed {
			d = TakeProject
		}
		switch d {
		case Agreed:
			record(f.name, p)
		case TakeProject:
			setLabel(f.name, p)
			record(f.name, p)
		case TakeLocal:
			kind, arg, ok := c.toProject(pf, f.name, l)
			if !ok {
				change.Warnings = append(change.Warnings, fmt.Sprintf("%s value %q has no option on the Project, not written", f.name, l))
				continue
			}
			plan.Edits = append(plan.Edits, Edit{IssueID: is.ID, ItemID: it.ID, Number: it.Number,
				Field: f.name, Value: l, FieldID: pf.ID, Kind: kind, Arg: arg})
			record(f.name, l)
		case Held:
			plan.Conflicts = append(plan.Conflicts, Conflict{IssueID: is.ID, Number: it.Number, URL: it.URL,
				Title: it.Title, Field: f.name, Base: base, HasBase: hasBase, Local: l, Project: p})
		}
	}

	// Assignees merge as a set, person by person, so they never conflict,
	// where their field's home is the native assignees: a mapping moving
	// them to a label, or nowhere, leaves both sides alone.
	if c.Target("assignee") == forge.TargetAssignees {
		assignable, checked := meta.Assignable[repoOf(it.URL)]
		a := syncAssignees(it, is.Labels, bases["assignee"], c.Write)
		if checked {
			var refused []string
			a.ghAdd = slices.DeleteFunc(a.ghAdd, func(login string) bool {
				if slices.Contains(assignable, login) {
					return false
				}
				refused = append(refused, login)
				return true
			})
			for _, login := range refused {
				change.Warnings = append(change.Warnings, fmt.Sprintf("%s can't be assigned on GitHub (no access to %s), kept in git-board only", login, repoOf(it.URL)))
				a.base = strings.Join(slices.DeleteFunc(strings.Split(a.base, ","), func(s string) bool { return s == login }), ",")
			}
		}
		if len(a.added)+len(a.removed) > 0 {
			change.Fields = append(change.Fields, FieldResult{Field: "assignee", Added: a.added, Removed: a.removed})
			change.Added = append(change.Added, a.added...)
			change.Removed = append(change.Removed, a.removed...)
		}
		if len(a.ghAdd)+len(a.ghRemove) > 0 {
			plan.Edits = append(plan.Edits, Edit{IssueID: is.ID, ItemID: it.ID, Number: it.Number,
				Field: "assignee", Value: a.base, Kind: EditAssignees, URL: it.URL, Add: a.ghAdd, Remove: a.ghRemove})
		}
		record("assignee", a.base)
	}

	res, warning := c.planKind(is.Labels, it.Labels, valueOf(it, "Category"))
	if warning != "" {
		change.Warnings = append(change.Warnings, warning)
	}
	if res.Field != "" {
		change.Fields = append(change.Fields, res)
		change.Added = append(change.Added, res.Added...)
		change.Removed = append(change.Removed, res.Removed...)
	}

	plan.Changes = append(plan.Changes, change)
	if len(newBases) > 0 {
		plan.Bases[is.ID] = newBases
	}
}

// planKind fills an issue's kind from the Project's Category, the same
// vocabulary as the kind labels: FEAT is feat, matched to [branches]
// kinds without case and written as kinds spells it. A kind already on
// the issue wins, and so does one GitHub has that isn't pulled yet. It
// removes the bare label an earlier bridge wrote, Category as is (FEAT).
// A label GitHub has stays, whatever its case.
func (c *Config) planKind(current, onForge []string, category string) (FieldResult, string) {
	if category == "" {
		return FieldResult{}, ""
	}
	res := FieldResult{Field: "kind"}
	there := slices.ContainsFunc(onForge, func(l string) bool { return strings.EqualFold(l, category) })
	// ponytail: only the current Category's bare label; one left by an
	// earlier value stays (none on co2)
	if _, isKind := c.kinds()[category]; !isKind && !there && slices.Contains(current, category) {
		res.Removed = []string{category}
	}
	var warning string
	if c.kindOf(current) == "" && c.kindOf(onForge) == "" {
		warning = fmt.Sprintf("category value %q is none of [branches] kinds, skipped", category)
		for _, k := range slices.Sorted(maps.Keys(c.kinds())) {
			if strings.EqualFold(k, category) {
				res.Value, res.Added, warning = k, []string{c.label("kind", k)}, ""
				break
			}
		}
	}
	if len(res.Added)+len(res.Removed) == 0 {
		return FieldResult{}, warning
	}
	return res, warning
}

// genericRE matches "generic" in any case, around spaces: the Module's
// duplicate "Generic" options are one.
var genericRE = regexp.MustCompile(`(?i)^\s*generic\s*$`)

func normalizeModule(value string) string {
	if genericRE.MatchString(value) {
		return "Generic"
	}
	return strings.TrimSpace(value)
}

// labelChange sets a single field's label to value, or removes it for "".
func labelChange(labels []string, field, value string) FieldResult {
	res := FieldResult{Field: field, Value: value}
	if value == "" {
		for _, l := range labels {
			if strings.HasPrefix(l, field+":") {
				res.Removed = append(res.Removed, l)
			}
		}
		return res
	}
	label := field + ":" + value
	for _, l := range labels {
		if strings.HasPrefix(l, field+":") && l != label {
			res.Removed = append(res.Removed, l)
		}
	}
	if !slices.Contains(labels, label) {
		res.Added = []string{label}
	}
	return res
}

// Resolve turns a held conflict into a plan keeping one side: git-board's
// value written to the Project (ours), or the Project's set on the label
// (theirs). Both then agree on it.
func Resolve(k Conflict, ours bool, it Item, is Issue, meta Meta, c *Config) (Plan, error) {
	change := Change{IssueID: is.ID, URL: is.URL, Number: k.Number, Title: k.Title}
	plan := Plan{Bases: map[string]map[string]string{}}
	value := k.Project
	if ours {
		value = k.Local
		var pf Field
		for _, f := range c.synced() {
			if f.name == k.Field {
				pf, _ = meta.field(f.column)
			}
		}
		kind, arg, ok := c.toProject(pf, k.Field, k.Local)
		if !ok {
			return Plan{}, fmt.Errorf("%s value %q %w", k.Field, k.Local, ErrNoOption)
		}
		plan.Edits = []Edit{{IssueID: is.ID, ItemID: it.ID, Number: k.Number, Field: k.Field,
			Value: k.Local, FieldID: pf.ID, Kind: kind, Arg: arg}}
	} else {
		res := labelChange(is.Labels, k.Field, k.Project)
		change.Fields, change.Added, change.Removed = []FieldResult{res}, res.Added, res.Removed
	}
	plan.Changes = []Change{change}
	plan.Bases[is.ID] = map[string]string{k.Field: value}
	return plan, nil
}

// repoOf is "owner/name" of an issue's URL.
func repoOf(issueURL string) string {
	parts := strings.Split(strings.TrimPrefix(issueURL, "https://github.com/"), "/")
	if len(parts) < 2 {
		return ""
	}
	return parts[0] + "/" + parts[1]
}

// assigneeSync is how one issue's assignees merge.
type assigneeSync struct {
	added, removed  []string // labels
	ghAdd, ghRemove []string // logins, on the GitHub issue
	base            string   // what both then hold, recorded as the base
}

// syncAssignees merges an issue's assignees person by person against
// base, what both sides last agreed on: a change on GitHub reaches the
// labels, a change in git-board reaches the issue, with write; else
// assignees only come from GitHub and base is GitHub's. With no base yet,
// both sides' assignees are kept.
func syncAssignees(it Item, labels []string, base string, write bool) assigneeSync {
	project := slices.Sorted(slices.Values(it.Assignees))
	var before []string
	if base != "" {
		before = strings.Split(base, ",")
	}
	var local []string
	for _, l := range labels {
		if a, ok := strings.CutPrefix(l, "assignee:"); ok {
			local = append(local, a)
		}
	}

	var res assigneeSync
	for _, a := range project {
		if !slices.Contains(before, a) && !slices.Contains(local, a) {
			res.added = append(res.added, "assignee:"+a)
		}
	}
	for _, a := range before {
		if !slices.Contains(project, a) && slices.Contains(local, a) {
			res.removed = append(res.removed, "assignee:"+a)
		}
	}
	if !write {
		res.base = strings.Join(project, ",")
		return res
	}
	for _, a := range local {
		if !slices.Contains(before, a) && !slices.Contains(project, a) {
			res.ghAdd = append(res.ghAdd, a)
		}
	}
	for _, a := range before {
		if slices.Contains(project, a) && !slices.Contains(local, a) {
			res.ghRemove = append(res.ghRemove, a)
		}
	}
	agreed := slices.Concat(project, res.ghAdd)
	agreed = slices.DeleteFunc(agreed, func(a string) bool { return slices.Contains(res.ghRemove, a) })
	slices.Sort(agreed)
	res.base = strings.Join(agreed, ",")
	return res
}

// localValue is git-board's value of a single field, "" without a label;
// false with several.
func localValue(labels []string, field string) (string, bool) {
	var values []string
	for _, l := range labels {
		if v, ok := strings.CutPrefix(l, field+":"); ok {
			values = append(values, v)
		}
	}
	switch len(values) {
	case 0:
		return "", true
	case 1:
		return values[0], true
	}
	return "", false
}

// isNumber and isIteration say how a field's values are written on the
// Project: est and sprint as v1 knew them, any other by its column's type.
func isNumber(pf Field, field string) bool {
	return field == "est" || pf.Type == "NUMBER"
}

func isIteration(pf Field, field string) bool {
	return field == "sprint" || pf.Type == "ITERATION"
}

var dateRE = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// toLabel turns the Project's raw value of a field into git-board's
// value; false when no option matches, or nothing is left of it as a slug.
func (c *Config) toLabel(pf Field, field, raw string) (string, bool) {
	if raw == "" {
		return "", true
	}
	f := c.field(field)
	switch {
	case isIteration(pf, field):
		value := c.sprintSlug(raw)
		return value, value != ""
	case isNumber(pf, field):
		return raw, f.KnownValue(raw)
	case pf.Type == "DATE":
		return raw, dateRE.MatchString(raw) && f.KnownValue(raw)
	case pf.Type == "TEXT":
		value := strings.TrimSpace(raw)
		return value, value != "" && !strings.Contains(value, "\n") && f.KnownValue(value)
	}
	for value, name := range c.values(field) {
		if name == raw {
			return value, true
		}
	}
	// "-" slugs to nothing: not a value, and not "cleared" either
	value := Slug(raw)
	// an option spelled as the forge does (P0, Must) is the one its slug is
	for _, o := range f.Options {
		if Slug(o) == value {
			return o, value != ""
		}
	}
	return value, value != "" && f.KnownValue(value)
}

// sprintSlug is the label value of an iteration: its [[sprints.list]]
// slug when listed, else its title as a slug, so a new iteration needs no
// config.
func (c *Config) sprintSlug(title string) string {
	if c.Schema != nil {
		for _, s := range c.Schema.Sprints.List {
			if strings.EqualFold(strings.TrimSpace(s.Title), strings.TrimSpace(title)) {
				return s.Slug
			}
		}
	}
	return Slug(title)
}

// toProject turns git-board's value of a field into a Project edit; false
// when the Project has no option for it.
func (c *Config) toProject(pf Field, field, value string) (kind, arg string, ok bool) {
	if value == "" {
		return EditClear, "", true
	}
	switch {
	case isNumber(pf, field):
		if _, err := strconv.ParseFloat(value, 64); err != nil {
			return "", "", false
		}
		return EditNumber, value, true
	case isIteration(pf, field):
		for _, title := range slices.Sorted(maps.Keys(pf.Options)) {
			if c.sprintSlug(title) == value {
				return EditIteration, pf.Options[title], true
			}
		}
		return "", "", false
	case pf.Type == "DATE":
		return EditDate, value, dateRE.MatchString(value)
	case pf.Type == "TEXT":
		return EditText, value, true
	}
	if name, ok := c.values(field)[value]; ok {
		id, ok := pf.Options[name]
		return EditOption, id, ok
	}
	// sorted, so duplicate names pick the same option every run
	for _, name := range slices.Sorted(maps.Keys(pf.Options)) {
		if Slug(normalizeModule(name)) == Slug(value) {
			return EditOption, pf.Options[name], true
		}
	}
	return "", "", false
}

var slugNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// Slug is a value as a label carries it: lower case, accents dropped,
// runs of anything else one dash.
func Slug(value string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(strings.TrimSpace(value))) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return strings.Trim(slugNonAlnum.ReplaceAllString(b.String(), "-"), "-")
}

// LabelsOnly is the plan's git-board side: its label changes, no Project
// edit. Each edit left out takes its base with it: what isn't sent isn't
// agreed.
func (p Plan) LabelsOnly() Plan {
	q := p
	q.Edits, q.Bases = nil, maps.Clone(p.Bases)
	for _, e := range p.Edits {
		q.Bases[e.IssueID] = without(q.Bases[e.IssueID], e.Field)
	}
	return q
}

// ProjectOnly is the plan's Project side: its edits, no label change.
// Each label change left out takes its base with it.
func (p Plan) ProjectOnly() Plan {
	q := p
	q.Changes, q.Bases = make([]Change, len(p.Changes)), maps.Clone(p.Bases)
	for i, c := range p.Changes {
		for _, f := range c.Fields {
			q.Bases[c.IssueID] = without(q.Bases[c.IssueID], f.Field)
		}
		c.Fields, c.Added, c.Removed = nil, nil, nil
		q.Changes[i] = c
	}
	return q
}

func without(m map[string]string, key string) map[string]string {
	res := make(map[string]string, len(m))
	for k, v := range m {
		if k != key {
			res[k] = v
		}
	}
	return res
}
