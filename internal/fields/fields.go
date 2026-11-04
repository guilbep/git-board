// Package fields applies field=value assignments as namespaced labels,
// via internal/store. Single-cardinality fields (the default) swap: any
// existing "field:*" label is removed before the new one is added.
// Multi-cardinality fields (assignee) add by default; a leading "-"
// removes instead (e.g. assignee=-guilbep). An empty value (status=)
// clears the field; label=+bug and label=-bug add and remove a plain
// label, as the filter's label:bug reads one.
package fields

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/store"
)

// Backend is the subset of store.Backend that field application needs.
type Backend interface {
	GetLabels(idPrefix string) ([]string, error)
	ApplyLabelChange(idPrefix string, added, removed []string) ([]string, error)
}

var _ Backend = (*store.Backend)(nil)

// PlainLabel is the field name that writes a plain label (bug, not
// kind:bug): label=+bug, label=-bug, as the filter's label:bug.
const PlainLabel = "label"

// ErrInvalid is a malformed assignment: one that would write a label
// nothing reads.
var ErrInvalid = errors.New("invalid assignment")

// Assignment is one field=value. Op is '+' (add), '-' (remove) or 0
// (set); a set with an empty Value clears the field.
type Assignment struct {
	Field string
	Op    byte
	Value string
}

// String is the assignment as it is written: prio=p1, assignee=+bob.
func (a Assignment) String() string {
	op := ""
	if a.Op != 0 {
		op = string(a.Op)
	}
	return a.Field + "=" + op + a.Value
}

// Clears tells the assignment empties the field (status=).
func (a Assignment) Clears() bool { return a.Op == 0 && a.Value == "" }

// Result is what an assignment changes: the label it names (status:ready;
// "" for a clear) and the labels it adds and removes.
type Result struct {
	Label   string
	Added   []string
	Removed []string
}

// fieldName: no space, colon or equals sign, and no sign first.
var fieldName = regexp.MustCompile(`^[^\s:=+\-@#][^\s:=]*$`)

// Parse reads one field=value as the command line writes it. A nameless,
// spaced or signed-but-empty assignment is refused: it would write a label
// nothing reads (":x", "status: ready", "assignee:").
func Parse(s string) (Assignment, error) {
	name, value, ok := strings.Cut(s, "=")
	if !ok {
		return Assignment{}, fmt.Errorf("%w %q: expected field=value", ErrInvalid, s)
	}
	return parse(name, value)
}

func parse(name, value string) (Assignment, error) {
	a := Assignment{Field: name, Value: value}
	if !fieldName.MatchString(name) {
		return a, fmt.Errorf("%w %q: a field name, without spaces or ':', comes before the '='", ErrInvalid, a)
	}
	if strings.HasPrefix(value, "+") || strings.HasPrefix(value, "-") {
		a.Op, a.Value = value[0], value[1:]
		if a.Value == "" {
			return a, fmt.Errorf("%w %q: %c needs a value (%s=%cvalue)", ErrInvalid, a, a.Op, name, a.Op)
		}
	}
	switch {
	case strings.TrimSpace(a.Value) != a.Value:
		return a, fmt.Errorf("%w %q: the value starts or ends with a space", ErrInvalid, a)
	case strings.ContainsAny(a.Value, "\n\r\t"):
		return a, fmt.Errorf("%w %q: the value holds a line break or a tab", ErrInvalid, a)
	case strings.HasPrefix(a.Value, "+") || strings.HasPrefix(a.Value, "-"):
		return a, fmt.Errorf("%w %q: one sign at most", ErrInvalid, a)
	}
	return a, nil
}

// Resolve replaces the value's aliases: @me (me, else the forge login of
// the person you write as, else their name, from b when it can tell), @current/@next/@previous
// (the sprint), and a configured alias (could → couldhave). The sign
// stays: +@me adds you.
func (a Assignment) Resolve(b any, cfg *config.Config, me string, today time.Time) (Assignment, error) {
	if a.Value == "" {
		return a, nil
	}
	if me == "" && a.Value == "@me" {
		if l, ok := b.(interface{ MyName() string }); ok {
			me = l.MyName() // @me without --me: your login, else your name
		}
	}
	v, err := resolveAssignedValue(cfg, a.Value, me, today)
	if err != nil {
		return a, err
	}
	if f, ok := cfg.Fields[a.Field]; ok {
		v = f.AliasOf(v)
	}
	a.Value = v
	return a, nil
}

// ErrUnchecked is what Check returns: the assignment is well formed, but
// the config doesn't allow it. The command line writes it anyway with
// --force.
var ErrUnchecked = errors.New("not allowed by .git-board.toml")

var dateValue = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// Check holds a resolved assignment to the config: a field it knows (one
// configured, one its type names, or one in use on some issue: inUse are
// the labels in use), a value among the field's options, a number for a
// number, a date for a date, a sprint's slug for a sprint, a plain label
// from [labels] when the project lists them. A removal or a clear is never
// held back: it's how a wrong value goes.
func Check(cfg *config.Config, a Assignment, inUse []string) error {
	if a.Op == '-' || a.Clears() {
		return nil
	}
	if a.Field == PlainLabel && !configured(cfg, PlainLabel) {
		if cfg.Labels.Defined && !slices.Contains(cfg.Labels.List, a.Value) {
			return fmt.Errorf("%w: label %q isn't in [labels] list (%s)", ErrUnchecked, a.Value, orNone(cfg.Labels.List))
		}
		return nil
	}
	if !known(cfg, a.Field, inUse) {
		return fmt.Errorf("%w: unknown field %q, not configured and on no issue (fields: %s)", ErrUnchecked, a.Field, orNone(cfg.FieldOrder()))
	}
	f := cfg.Fields[a.Field]
	if len(f.Options) > 0 {
		if !f.KnownValue(a.Value) {
			return fmt.Errorf("%w: %s %q isn't one of its options (%s)", ErrUnchecked, a.Field, a.Value, strings.Join(f.Options, ", "))
		}
		return nil
	}
	switch cfg.TypeOf(a.Field, nil) {
	case config.TypeNumber:
		if _, err := strconv.ParseFloat(a.Value, 64); err != nil {
			return fmt.Errorf("%w: %s is a number, not %q", ErrUnchecked, a.Field, a.Value)
		}
	case config.TypeDate:
		if _, err := time.Parse(time.DateOnly, a.Value); err != nil || !dateValue.MatchString(a.Value) {
			return fmt.Errorf("%w: %s is a date (YYYY-MM-DD), not %q", ErrUnchecked, a.Field, a.Value)
		}
	case config.TypeIteration:
		if slugs := sprintSlugs(cfg); len(slugs) > 0 && !slices.Contains(slugs, a.Value) {
			return fmt.Errorf("%w: %s %q isn't a sprint (%s)", ErrUnchecked, a.Field, a.Value, strings.Join(slugs, ", "))
		}
	}
	return nil
}

func configured(cfg *config.Config, name string) bool {
	_, ok := cfg.Fields[name]
	return ok
}

// known: configured, a role, a name whose type is known (sprint,
// assignee, due…), the flat kinds, or a namespace some issue uses.
func known(cfg *config.Config, name string, inUse []string) bool {
	if configured(cfg, name) || cfg.Roles[name] != "" || cfg.TypeOf(name, nil) != config.TypeSelect {
		return true
	}
	if name == "kind" && cfg.FlatKinds() {
		return true
	}
	return slices.ContainsFunc(inUse, func(l string) bool { return strings.HasPrefix(l, name+":") })
}

func sprintSlugs(cfg *config.Config) []string {
	var res []string
	for _, s := range cfg.Sprints.List {
		res = append(res, s.Slug)
	}
	return res
}

func orNone(list []string) string {
	if len(list) == 0 {
		return "none"
	}
	return strings.Join(list, ", ")
}

// Change is what a resolved assignment does to labels current, without
// writing: a single field swaps, or clears with an empty value, or drops
// its value with -value (+value is refused: it holds one); a multi field
// adds (value, +value) or removes (-value), and clears with an empty value;
// label=±name adds or removes a plain label.
func Change(cfg *config.Config, current []string, a Assignment) (Result, error) {
	if a.Field == PlainLabel && !configured(cfg, PlainLabel) {
		switch {
		case a.Value == "":
			return Result{}, fmt.Errorf("%w %q: name the label, label=+name or label=-name", ErrInvalid, a)
		case strings.Contains(a.Value, ":"):
			return Result{}, fmt.Errorf("%w %q: %s is a field's value, write %s", ErrInvalid, a, a.Value, strings.Replace(a.Value, ":", "=", 1))
		}
		var r Result
		r.Label = a.Value
		has := slices.Contains(current, a.Value)
		if a.Op == '-' && has {
			r.Removed = []string{a.Value}
		} else if a.Op != '-' && !has {
			r.Added = []string{a.Value}
		}
		return r, nil
	}
	if a.Clears() {
		var r Result
		for _, l := range current {
			if strings.HasPrefix(l, a.Field+":") {
				r.Removed = append(r.Removed, l)
			}
		}
		return r, nil
	}
	single := Single(cfg, a.Field, current)
	if single && a.Op == '+' {
		return Result{}, fmt.Errorf("%w %q: %s holds one value, write %s=%s", ErrInvalid, a, a.Field, a.Field, a.Value)
	}
	if single && a.Op == '-' {
		l := a.Field + ":" + a.Value
		r := Result{Label: l}
		if slices.Contains(current, l) {
			r.Removed = []string{l}
		}
		return r, nil
	}
	value := a.Value
	if a.Op == '-' {
		value = "-" + value
	}
	label, added, removed := Diff(current, a.Field, value, single)
	return Result{Label: label, Added: added, Removed: removed}, nil
}

// Fold applies a change to labels, as the write will leave them.
func Fold(labels []string, r Result) []string {
	res := slices.DeleteFunc(slices.Clone(labels), func(l string) bool { return slices.Contains(r.Removed, l) })
	for _, l := range r.Added {
		if !slices.Contains(res, l) {
			res = append(res, l)
		}
	}
	return res
}

// Apply sets fieldName to value on the issue identified by idPrefix.
// Returns the resulting label (e.g. "status:ready"; "" when value is
// empty: the field is cleared). The form is checked (Parse); the options
// are not: the web and the TUI pick among them, and the command line
// calls Check first.
func Apply(b Backend, cfg *config.Config, idPrefix, fieldName, value, me string, today time.Time) (string, error) {
	a, err := parse(fieldName, value)
	if err != nil {
		return "", err
	}
	if a, err = a.Resolve(b, cfg, me, today); err != nil {
		return "", err
	}
	current, err := b.GetLabels(idPrefix)
	if err != nil {
		return "", err
	}
	r, err := Change(cfg, current, a)
	if err != nil {
		return "", err
	}
	if _, err := b.ApplyLabelChange(idPrefix, r.Added, r.Removed); err != nil {
		return "", err
	}
	return r.Label, nil
}

// Single tells whether fieldName holds one value on an issue with labels
// current: as configured; else, not configured, one value, but for
// assignee (people) and a field the issue already holds several values of.
// The web side panel decides the same way (web/src/components/IssueDetail.vue).
func Single(cfg *config.Config, fieldName string, current []string) bool {
	if f, known := cfg.Fields[fieldName]; known {
		return f.Single
	}
	n := 0
	for _, l := range current {
		if strings.HasPrefix(l, fieldName+":") {
			n++
		}
	}
	return cfg.TypeOf(fieldName, nil) != config.TypePerson && n < 2
}

// Diff computes the label add/remove needed to set fieldName to a resolved
// value against current labels, without writing anything. Apply uses it for
// the actual write; internal/ghbridge uses it to preview a bridge pull
// (dry-run) without touching the backend. Same single/multi-cardinality
// swap semantics described in the package doc.
func Diff(current []string, fieldName, resolved string, single bool) (label string, added, removed []string) {
	currentSet := make(map[string]bool, len(current))
	for _, l := range current {
		currentSet[l] = true
	}

	if single {
		newLabel := fieldName + ":" + resolved
		for _, l := range current {
			if strings.HasPrefix(l, fieldName+":") && l != newLabel {
				removed = append(removed, l)
			}
		}
		if !currentSet[newLabel] {
			added = []string{newLabel}
		}
		return newLabel, added, removed
	}

	remove := strings.HasPrefix(resolved, "-")
	target := strings.TrimLeft(resolved, "+-")
	newLabel := fieldName + ":" + target
	switch {
	case remove && currentSet[newLabel]:
		removed = []string{newLabel}
	case !remove && !currentSet[newLabel]:
		added = []string{newLabel}
	}
	return newLabel, added, removed
}

func resolveAssignedValue(cfg *config.Config, value, me string, today time.Time) (string, error) {
	switch {
	case value == "@current" || value == "@next" || value == "@previous":
		sprint, err := config.ResolveSprint(cfg.Sprints, value[1:], today)
		if err != nil {
			return "", err
		}
		if sprint == nil {
			return "", fmt.Errorf("no sprint resolves to %s", value)
		}
		return sprint.Slug, nil
	case value == "@me":
		if me == "" {
			return "", fmt.Errorf("@me is the person you write as: choose one (git board me --new, Settings › You), or pass --me / set GIT_BOARD_ME")
		}
		return me, nil
	default:
		return value, nil
	}
}
