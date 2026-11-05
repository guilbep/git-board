package issues

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The field assignments of set and new (ADR 0007), as v1's core/fields
// reads them: field=value sets, +value adds, -value removes, field=
// clears; label=+name and label=-name a plain label.

// PlainLabel is the field name that writes a plain label.
const PlainLabel = "label"

// ErrInvalid is a malformed assignment: one that would write a label
// nothing reads.
var ErrInvalid = errors.New("invalid assignment")

// ErrUnchecked is an assignment .git-board.toml doesn't allow.
var ErrUnchecked = errors.New("not allowed by .git-board.toml")

// Assignment is one field=value; Op is '+', '-' or 0 (set).
type Assignment struct {
	Field string
	Op    byte
	Value string
}

// String is the assignment as written.
func (a Assignment) String() string {
	op := ""
	if a.Op != 0 {
		op = string(a.Op)
	}
	return a.Field + "=" + op + a.Value
}

// Clears says the assignment empties the field.
func (a Assignment) Clears() bool { return a.Op == 0 && a.Value == "" }

// Result is what an assignment changes: the label it names and the
// labels it adds and removes.
type Result struct {
	Label   string
	Added   []string
	Removed []string
}

var fieldName = regexp.MustCompile(`^[^\s:=+\-@#][^\s:=]*$`)

// ParseAssignment reads field=value.
func ParseAssignment(s string) (Assignment, error) {
	name, value, ok := strings.Cut(s, "=")
	if !ok {
		return Assignment{}, fmt.Errorf("%w %q: expected field=value", ErrInvalid, s)
	}
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

// resolve is the assignment with its value resolved: @me who writes,
// @current, @next and @previous sprints, an alias its current name.
func (a Assignment) resolve(s *Schema, me string, today time.Time) (Assignment, error) {
	if a.Value == "" {
		return a, nil
	}
	v := a.Value
	switch {
	case v == "@current" || v == "@next" || v == "@previous":
		sprint := s.Sprints.Resolve(v[1:], today)
		if sprint == nil {
			return a, fmt.Errorf("no sprint resolves to %s", v)
		}
		v = sprint.Slug
	case v == "@me":
		if me == "" {
			return a, ErrNoMe
		}
		v = me
	}
	if f, ok := s.Fields[a.Field]; ok {
		v = f.AliasOf(v)
	}
	a.Value = v
	return a, nil
}

// Check refuses an assignment the config doesn't allow; inUse are the
// labels on the board. Removing and clearing are never refused.
func (s *Schema) Check(a Assignment, inUse []string) error {
	if a.Op == '-' || a.Clears() {
		return nil
	}
	if a.Field == PlainLabel && !s.configured(PlainLabel) {
		if s.Labels.Defined && !slices.Contains(s.Labels.List, a.Value) {
			return fmt.Errorf("%w: label %q isn't in [labels] list (%s)", ErrUnchecked, a.Value, orNone(s.Labels.List))
		}
		return nil
	}
	if !s.known(a.Field, inUse) {
		return fmt.Errorf("%w: unknown field %q, not configured and on no issue (fields: %s)", ErrUnchecked, a.Field, orNone(s.FieldOrder()))
	}
	f := s.Fields[a.Field]
	if len(f.Options) > 0 {
		if !f.KnownValue(a.Value) {
			return fmt.Errorf("%w: %s %q isn't one of its options (%s)", ErrUnchecked, a.Field, a.Value, strings.Join(f.Options, ", "))
		}
		return nil
	}
	switch s.TypeOf(a.Field, nil) {
	case TypeNumber:
		if _, err := strconv.ParseFloat(a.Value, 64); err != nil {
			return fmt.Errorf("%w: %s is a number, not %q", ErrUnchecked, a.Field, a.Value)
		}
	case TypeDate:
		if _, err := time.Parse(time.DateOnly, a.Value); err != nil || !dateValue.MatchString(a.Value) {
			return fmt.Errorf("%w: %s is a date (YYYY-MM-DD), not %q", ErrUnchecked, a.Field, a.Value)
		}
	case TypeIteration:
		var slugs []string
		for _, sp := range s.Sprints.List {
			slugs = append(slugs, sp.Slug)
		}
		if len(slugs) > 0 && !slices.Contains(slugs, a.Value) {
			return fmt.Errorf("%w: %s %q isn't a sprint (%s)", ErrUnchecked, a.Field, a.Value, strings.Join(slugs, ", "))
		}
	}
	return nil
}

func (s *Schema) configured(name string) bool {
	_, ok := s.Fields[name]
	return ok
}

func (s *Schema) known(name string, inUse []string) bool {
	if s.configured(name) || s.Roles[name] != "" || s.TypeOf(name, nil) != TypeSelect {
		return true
	}
	return slices.ContainsFunc(inUse, func(l string) bool { return strings.HasPrefix(l, name+":") })
}

func orNone(list []string) string {
	if len(list) == 0 {
		return "none"
	}
	return strings.Join(list, ", ")
}

// Change is what an assignment does to current labels.
func (s *Schema) Change(current []string, a Assignment) (Result, error) {
	if a.Field == PlainLabel && !s.configured(PlainLabel) {
		switch {
		case a.Value == "":
			return Result{}, fmt.Errorf("%w %q: name the label, label=+name or label=-name", ErrInvalid, a)
		case strings.Contains(a.Value, ":"):
			return Result{}, fmt.Errorf("%w %q: %s is a field's value, write %s", ErrInvalid, a, a.Value, strings.Replace(a.Value, ":", "=", 1))
		}
		r := Result{Label: a.Value}
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
	single := s.single(a.Field, current)
	if single && a.Op == '+' {
		return Result{}, fmt.Errorf("%w %q: %s holds one value, write %s=%s", ErrInvalid, a, a.Field, a.Field, a.Value)
	}
	label := a.Field + ":" + a.Value
	r := Result{Label: label}
	has := slices.Contains(current, label)
	switch {
	case a.Op == '-':
		if has {
			r.Removed = []string{label}
		}
	case single:
		for _, l := range current {
			if strings.HasPrefix(l, a.Field+":") && l != label {
				r.Removed = append(r.Removed, l)
			}
		}
		if !has {
			r.Added = []string{label}
		}
	case !has:
		r.Added = []string{label}
	}
	return r, nil
}

// single says a field holds one value: its config, else a field that
// isn't a person's and holds at most one value now.
func (s *Schema) single(field string, current []string) bool {
	if f, known := s.Fields[field]; known {
		return f.Single
	}
	n := 0
	for _, l := range current {
		if strings.HasPrefix(l, field+":") {
			n++
		}
	}
	return s.TypeOf(field, nil) != TypePerson && n < 2
}

// Fold is labels with r applied.
func Fold(labels []string, r Result) []string {
	res := slices.DeleteFunc(slices.Clone(labels), func(l string) bool { return slices.Contains(r.Removed, l) })
	for _, l := range r.Added {
		if !slices.Contains(res, l) {
			res = append(res, l)
		}
	}
	return res
}

// Describe says what an assignment did, as set prints it.
func Describe(a Assignment, r Result) string {
	switch {
	case a.Clears():
		return a.Field + " cleared"
	case a.Op == '-':
		return "-" + r.Label
	}
	return r.Label
}
