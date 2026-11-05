package issues

import (
	"maps"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/guilbep/git-board/next/conf"
)

// Field describes one field, a label namespace (ADR 0007): status, prio.
type Field struct {
	Single  bool              `toml:"single"`
	Options []string          `toml:"options"`
	Numeric bool              `toml:"numeric"`
	Type    string            `toml:"type"` // Type*; empty: guessed (TypeOf)
	Unit    string            `toml:"unit"` // after a number field's values: "h"
	Aliases map[string]string `toml:"aliases"`
	// Project names the GitHub Project's option of a value; Colors a
	// value's color (Palette, or #rrggbb); Descriptions what a value
	// means, 100 characters at most, GitHub's.
	Project      map[string]string `toml:"project"`
	Colors       map[string]string `toml:"colors"`
	Descriptions map[string]string `toml:"descriptions"`
	// Flat: the kind field of plain labels (bug, not kind:bug), from
	// [branches] kinds.
	Flat bool `toml:"-"`
}

// A field's type says what its values are.
const (
	TypeSelect    = "select"
	TypeIteration = "iteration"
	TypePerson    = "person"
	TypeNumber    = "number"
	TypeDate      = "date"
	TypeIssue     = "issue"
	TypeText      = "text"
)

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

// Schema is what a board's fields are: the issue domain's tables of
// .git-board.toml, [fields], [sprints], [roles] and [branches] kinds.
type Schema struct {
	Fields  map[string]Field
	Sprints Sprints
	// Roles are the project's people by role (lead-dev = "bob"): a person
	// field an issue inherits when it sets none (inherited).
	Roles map[string]string
	// Kinds are [branches] kinds: plain labels read as kind:<label> while
	// [fields.kind] has no options of its own.
	Kinds map[string]string
	// Labels is [labels]: the plain labels the project allows.
	Labels Labels
	// Order is [fields]' names as the file writes them.
	Order []string
}

// Labels is the [labels] table.
type Labels struct {
	List         []string          `toml:"list"`
	Colors       map[string]string `toml:"colors"`       // a palette name or #rrggbb
	Descriptions map[string]string `toml:"descriptions"` // GitHub's, 100 characters at most
	Defined      bool              `toml:"-"`            // the file has the table, an empty list too
}

// FieldOrder is the fields' names as the file lists them; sorted
// without an order.
func (s *Schema) FieldOrder() []string {
	if len(s.Order) == len(s.Fields) {
		return s.Order
	}
	return slices.Sorted(maps.Keys(s.Fields))
}

// AliasOf is a value's current name on the field.
func (f Field) AliasOf(value string) string {
	if cur, ok := f.Aliases[value]; ok {
		return cur
	}
	return value
}

// KnownValue says value is one of the options, or any with none; a
// number field takes any number.
func (f Field) KnownValue(value string) bool {
	if _, err := strconv.ParseFloat(value, 64); err == nil && (f.Numeric || f.Type == TypeNumber) {
		return true
	}
	return len(f.Options) == 0 || slices.Contains(f.Options, value)
}

// Tables are the issue domain's tables on a file.
type Tables struct {
	file     *conf.File
	fields   *conf.Handle[map[string]Field]
	sprints  *conf.Handle[Sprints]
	roles    *conf.Handle[map[string]string]
	branches *conf.Handle[Branches]
	views    *conf.Handle[map[string]View]
	labels   *conf.Handle[Labels]
}

// View is a named list of [views]: its filter, grouping and sort.
type View struct {
	Layout  string   `toml:"layout"`
	Filter  string   `toml:"filter"`
	Group   string   `toml:"group"`
	Sort    string   `toml:"sort"`
	Columns []string `toml:"columns"`
	Lanes   []string `toml:"lanes"`
	// Limits caps a lane's cards (work in progress): in-progress = 5.
	Limits map[string]int `toml:"limits"`
}

// Views are [views], by name.
func (t Tables) Views() (map[string]View, []conf.Problem) { return t.views.Get() }

// Branches is [branches]: the kinds are the issue domain's; the base and
// the name, the branch an issue suggests, the tools'.
type Branches struct {
	Base  string            `toml:"base"`
	Name  string            `toml:"name"`
	Kinds map[string]string `toml:"kinds"`
}

// Branches is [branches], for the tools.
func (t Tables) Branches() (Branches, []conf.Problem) { return t.branches.Get() }

// Declare declares the issue domain's tables on f.
func Declare(f *conf.File) Tables {
	return Tables{
		file:     f,
		fields:   conf.Declare(f, conf.Table[map[string]Field]{Name: "fields"}),
		sprints:  conf.Declare(f, conf.Table[Sprints]{Name: "sprints"}),
		roles:    conf.Declare(f, conf.Table[map[string]string]{Name: "roles"}),
		branches: conf.Declare(f, conf.Table[Branches]{Name: "branches"}),
		views:    conf.Declare(f, conf.Table[map[string]View]{Name: "views"}),
		labels:   conf.Declare(f, conf.Table[Labels]{Name: "labels"}),
	}
}

// Schema is the tables read. A table that doesn't decode is left at its
// default, and named in the problems.
func (t Tables) Schema() (*Schema, []conf.Problem) {
	var probs []conf.Problem
	s := &Schema{}
	var p []conf.Problem
	s.Fields, p = t.fields.Get()
	probs = append(probs, p...)
	s.Sprints, p = t.sprints.Get()
	probs = append(probs, p...)
	s.Roles, p = t.roles.Get()
	probs = append(probs, p...)
	b, p := t.branches.Get()
	probs = append(probs, p...)
	s.Kinds = b.Kinds
	s.Labels, p = t.labels.Get()
	probs = append(probs, p...)
	s.Labels.Defined = t.labels.Defined()
	s.Order = t.fields.Keys()
	if len(probs) == 0 {
		probs = s.check(t.file.Text())
	}
	// the kinds are plain labels unless [fields.kind] has options of its own
	if len(s.Kinds) > 0 && len(s.Fields["kind"].Options) == 0 {
		if s.Fields == nil {
			s.Fields = map[string]Field{}
		}
		order := t.branches.Keys("kinds")
		for _, k := range slices.Sorted(maps.Keys(s.Kinds)) {
			if !slices.Contains(order, k) {
				order = append(order, k)
			}
		}
		f := s.Fields["kind"]
		f.Single, f.Options, f.Flat = true, order, true
		s.Fields["kind"] = f
	}
	// the fields the file has no table for (the flat kinds') after the
	// file's, by name
	for _, k := range slices.Sorted(maps.Keys(s.Fields)) {
		if !slices.Contains(s.Order, k) {
			s.Order = append(s.Order, k)
		}
	}
	return s, probs
}

// TypeOf is a field's type: its own; else numeric, a number; else a role,
// a person; else what its name says; else what all its values are; else
// select.
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

// UnitOf is a number field's unit: its own, else h for est.
func (s *Schema) UnitOf(name string) string {
	if u := s.Fields[name].Unit; u != "" {
		return u
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

// FlatKinds says the kinds are plain labels: bug is read kind:bug.
func (s *Schema) FlatKinds() bool { return s.Fields["kind"].Flat }

// RolesFor are the role labels an issue inherits: each role it doesn't
// set itself, as role:<login>, by role.
func (s *Schema) RolesFor(labels []string) []string {
	var out []string
	for _, role := range slices.Sorted(maps.Keys(s.Roles)) {
		if !slices.ContainsFunc(labels, func(l string) bool { return strings.HasPrefix(l, role+":") }) {
			out = append(out, role+":"+s.Roles[role])
		}
	}
	return out
}

// Shown are stored labels as git-board reads them: a flat kind's bug is
// kind:bug; Assignee:bob is assignee:bob; each once.
func (s *Schema) Shown(labels []string) []string {
	if !slices.ContainsFunc(labels, s.rewritten) { // most lists: labels as stored
		return labels
	}
	res := make([]string, 0, len(labels))
	for _, l := range labels {
		if k := strings.TrimPrefix(l, "kind:"); s.FlatKinds() {
			if _, ok := s.Kinds[k]; ok {
				l = "kind:" + k
			}
		}
		const p = "assignee:"
		if len(l) > len(p) && strings.EqualFold(l[:len(p)], p) {
			l = p + l[len(p):]
		}
		if !slices.Contains(res, l) {
			res = append(res, l)
		}
	}
	return res
}

// rewritten says Shown changes l: a flat kind, an assignee in another
// case.
func (s *Schema) rewritten(l string) bool {
	if _, ok := s.Kinds[strings.TrimPrefix(l, "kind:")]; ok && s.FlatKinds() {
		return true
	}
	const p = "assignee:"
	return len(l) > len(p) && strings.EqualFold(l[:len(p)], p) && l[:len(p)] != p
}

// Sprint is one entry of [[sprints.list]].
type Sprint struct {
	Slug  string    `toml:"slug"`
	Title string    `toml:"title"`
	Start time.Time `toml:"start"`
}

// Sprints is the sprint calendar, [sprints]. Its start_weekday, which no
// git-board read, schema 2 sets aside (docs/guide/config-schema.md).
type Sprints struct {
	LengthDays int      `toml:"length_days"`
	List       []Sprint `toml:"list"`
}

func dateOnly(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// Resolve is the sprint "current", "next" or "previous" names today: the
// current is the last one started on or before today, unless that is the
// last one and it has ended. nil when there is none.
func (sp Sprints) Resolve(which string, today time.Time) *Sprint {
	today = dateOnly(today)
	ordered := slices.Clone(sp.List)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Start.Before(ordered[j].Start) })
	idx := -1
	for i, s := range ordered {
		if dateOnly(s.Start).After(today) {
			break
		}
		idx = i
	}
	if idx >= 0 && idx == len(ordered)-1 && !today.Before(dateOnly(ordered[idx].Start).AddDate(0, 0, lastSprintDays(sp, ordered))) {
		switch which {
		case "previous":
			return &ordered[idx]
		case "current":
			return nil
		}
	}
	switch which {
	case "current":
		if idx >= 0 {
			return &ordered[idx]
		}
	case "next":
		if idx+1 < len(ordered) {
			return &ordered[idx+1]
		}
	case "previous":
		if idx > 0 {
			return &ordered[idx-1]
		}
	}
	return nil
}

// lastSprintDays is how long the last sprint lasts: length_days, else as
// long as the one before it, else 14 days.
func lastSprintDays(sp Sprints, ordered []Sprint) int {
	if sp.LengthDays > 0 {
		return sp.LengthDays
	}
	if n := len(ordered); n >= 2 {
		if d := int(dateOnly(ordered[n-1].Start).Sub(dateOnly(ordered[n-2].Start)).Hours() / 24); d > 0 {
			return d
		}
	}
	return 14
}
