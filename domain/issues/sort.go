package issues

import (
	"cmp"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// ErrUsage is a request that can't be run as written: a sort key that
// is no field.
var ErrUsage = errors.New("usage")

// usage is ErrUsage with what the user reads.
type usage struct{ error }

func (u usage) Unwrap() []error { return []error{u.error, ErrUsage} }

// sortKeys are the keys a sort takes besides the fields.
var sortKeys = map[string]func(x, y Issue) int{
	"created": func(x, y Issue) int { return x.CreatedAt.Compare(y.CreatedAt) },
	"updated": func(x, y Issue) int { return x.UpdatedAt.Compare(y.UpdatedAt) },
	"title": func(x, y Issue) int {
		return cmp.Compare(strings.ToLower(x.ShownTitle()), strings.ToLower(y.ShownTitle()))
	},
}

// Sort sorts issues by spec, "key [asc|desc]": created, updated, title or
// a field, stably. A key that is none of them, or another direction, is
// an ErrUsage: a sort that does nothing says so.
func (s *Schema) Sort(issues []Issue, spec string) error {
	if spec == "" {
		return nil
	}
	key, dir, _ := strings.Cut(strings.TrimSpace(spec), " ")
	dir = strings.TrimSpace(dir)
	if dir != "" && dir != "asc" && dir != "desc" {
		return usage{fmt.Errorf("--sort %q: the direction is asc or desc", spec)}
	}
	if by, ok := sortKeys[key]; ok {
		slices.SortStableFunc(issues, func(x, y Issue) int {
			if dir == "desc" {
				return by(y, x)
			}
			return by(x, y)
		})
		return nil
	}
	_, known := s.Fields[key]
	for _, is := range issues {
		known = known || slices.ContainsFunc(is.AllLabels(), func(l string) bool { return strings.HasPrefix(l, key+":") })
	}
	if !known && len(issues) > 0 {
		return usage{fmt.Errorf("--sort %q: %s is no field here; sort by created, updated, title or a field (%s)", spec, key, strings.Join(s.FieldNames(issues), ", "))}
	}
	s.sortByField(issues, key, dir == "desc")
	return nil
}

// FieldValue is the value of a field on an issue, its own or inherited:
// "p1" of prio:p1; "" for none.
func FieldValue(is Issue, field string) string {
	prefix := field + ":"
	for _, l := range is.AllLabels() {
		if v, ok := strings.CutPrefix(l, prefix); ok {
			return v
		}
	}
	return ""
}

// sortByField sorts by a field: numerically for a number field; else in
// the order of its options, values outside them after, by text. Issues
// without the field sort last either way.
func (s *Schema) sortByField(issues []Issue, key string, desc bool) {
	f := s.Fields[key]
	rank := func(v string) int {
		if i := slices.Index(f.Options, v); i >= 0 {
			return i
		}
		return len(f.Options)
	}
	sort.SliceStable(issues, func(i, j int) bool {
		vi, vj := FieldValue(issues[i], key), FieldValue(issues[j], key)
		switch {
		case f.Numeric || f.Type == TypeNumber:
			return lessNumeric(vi, vj, desc)
		case vi == "" || vj == "":
			return vi != "" // a value before none, either way
		case rank(vi) != rank(vj):
			return (rank(vi) < rank(vj)) != desc
		case desc:
			return vi > vj
		}
		return vi < vj
	})
}

// lessNumeric compares two values as numbers; one that isn't sorts last,
// either way.
func lessNumeric(a, b string, desc bool) bool {
	fa, aerr := strconv.ParseFloat(a, 64)
	fb, berr := strconv.ParseFloat(b, 64)
	switch {
	case aerr != nil:
		return false
	case berr != nil:
		return true
	case desc:
		return fa > fb
	}
	return fa < fb
}

// FieldNames are the fields of a list, sorted: the configured ones and
// every namespace on the issues' own labels.
func (s *Schema) FieldNames(issues []Issue) []string {
	seen := make(map[string]bool, len(s.Fields))
	for n := range s.Fields {
		seen[n] = true
	}
	for _, is := range issues {
		for _, l := range is.Labels {
			if ns, _, ok := strings.Cut(l, ":"); ok {
				seen[ns] = true
			}
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}

// FieldValues is a field and its values on an issue.
type FieldValues struct {
	Name   string
	Values []string
}

// ParseLabels splits labels into fields, by name, each with its values in
// the labels' order, and the plain labels.
func ParseLabels(labels []string) (known []FieldValues, other []string) {
	byField := map[string][]string{}
	for _, l := range labels {
		ns, val, ok := strings.Cut(l, ":")
		if !ok {
			other = append(other, l)
			continue
		}
		byField[ns] = append(byField[ns], val)
	}
	names := make([]string, 0, len(byField))
	for n := range byField {
		names = append(names, n)
	}
	slices.Sort(names)
	for _, n := range names {
		known = append(known, FieldValues{n, byField[n]})
	}
	return known, other
}

// DisplayName is a field as a heading shows it: Status for status.
func DisplayName(name string) string {
	if name == "" {
		return name
	}
	r := []rune(name)
	r[0] = unicode.ToUpper(r[0])
	return string(r)
}

// Format joins a field's values for display, a number field's with its
// unit (est: 4h).
func (s *Schema) Format(name string, values []string) string {
	if unit := s.UnitOf(name); unit != "" {
		suffixed := make([]string, len(values))
		for i, v := range values {
			suffixed[i] = v + unit
		}
		return strings.Join(suffixed, ", ")
	}
	return strings.Join(values, ", ")
}

// Bucket is one group of issues, in first-seen order.
type Bucket struct {
	Group  string  `json:"group"`
	Issues []Issue `json:"issues"`
}

// BucketBy groups issues by a field's value; "" is one bucket of all,
// the list itself. No issue is no bucket.
func BucketBy(issues []Issue, group string) []Bucket {
	if group == "" && len(issues) > 0 {
		return []Bucket{{Issues: issues}}
	}
	buckets := map[string][]Issue{}
	var order []string
	for _, is := range issues {
		key := FieldValue(is, group)
		if key == "" {
			key = "(none)"
		}
		if _, seen := buckets[key]; !seen {
			order = append(order, key)
		}
		buckets[key] = append(buckets[key], is)
	}
	out := make([]Bucket, len(order))
	for i, name := range order {
		out[i] = Bucket{Group: name, Issues: buckets[name]}
	}
	return out
}
