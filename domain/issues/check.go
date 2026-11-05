package issues

import (
	"bytes"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/guilbep/git-board/conf"
)

// The issue domain's rules on its tables, v1's (internal/config), with
// its messages: a file v1 refuses, v2 refuses, saying the same. v1 walks
// its maps in Go's random order; here they go by name, so of two wrongs
// the first by name is the one told.

// Palette is the colors a field's value or a label can take; the web
// draws each as a tinted chip.
var Palette = []string{"grey", "blue", "purple", "amber", "orange", "green", "red", "teal", "pink"}

// Types are the types a field can take.
var Types = []string{TypeSelect, TypeIteration, TypePerson, TypeNumber, TypeDate, TypeIssue, TypeText}

var (
	hexColor     = regexp.MustCompile(`^#[0-9a-f]{6}$`)
	colorAliases = map[string]string{"gray": "grey", "yellow": "amber", "violet": "purple"}
	roleName     = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
)

// NormColor is a color in Palette's words: lower case, yellow is amber;
// a #rrggbb lower case.
func NormColor(color string) string {
	c := strings.ToLower(strings.TrimSpace(color))
	if a, ok := colorAliases[c]; ok {
		return a
	}
	return c
}

func isColor(c string) bool { return slices.Contains(Palette, c) || hexColor.MatchString(c) }

// check is the schema's problems, in v1's order: the first one is the one
// v1 tells. The colors are put in Palette's words on the way.
func (s *Schema) check(text []byte) []conf.Problem {
	for _, c := range []struct {
		table string
		check func() error
	}{
		{"fields", s.checkColors},
		{"fields", s.checkTypes},
		{"roles", s.checkRoles},
		{"fields", s.checkDescriptions},
		{"branches", func() error { return checkKindWords(text, s.Kinds) }},
		{"labels", s.checkLabels},
		{"fields", s.checkAliases},
	} {
		if err := c.check(); err != nil {
			return []conf.Problem{{Table: c.table, Err: err}}
		}
	}
	return nil
}

func (s *Schema) names() []string { return slices.Sorted(maps.Keys(s.Fields)) }

func (s *Schema) checkColors() error {
	for _, name := range s.names() {
		f := s.Fields[name]
		for _, v := range slices.Sorted(maps.Keys(f.Colors)) {
			color := f.Colors[v]
			c := NormColor(color)
			if !isColor(c) {
				return fmt.Errorf("fields.%s.colors: %q for %q isn't a color: %s, or #rrggbb", name, color, v, strings.Join(Palette, ", "))
			}
			f.Colors[v] = c
		}
	}
	return nil
}

func (s *Schema) checkTypes() error {
	for _, name := range s.names() {
		if t := s.Fields[name].Type; t != "" && !slices.Contains(Types, t) {
			return fmt.Errorf("fields.%s: type %q isn't one of %s", name, t, strings.Join(Types, ", "))
		}
	}
	return nil
}

// checkRoles refuses a role that isn't a field name, names nobody or
// several, or is a field of another type than person.
func (s *Schema) checkRoles() error {
	for _, role := range slices.Sorted(maps.Keys(s.Roles)) {
		who := s.Roles[role]
		switch t := s.Fields[role].Type; {
		case !roleName.MatchString(role):
			return fmt.Errorf("roles.%s: a role is a field name: lower-case letters, digits and -", role)
		case strings.TrimSpace(who) == "" || strings.ContainsAny(who, " ,:"):
			return fmt.Errorf("roles.%s: one login, not %q", role, who)
		case strings.HasPrefix(who, "@"):
			return fmt.Errorf("roles.%s: a login without @: %q", role, strings.TrimPrefix(who, "@"))
		case t != "" && t != TypePerson:
			return fmt.Errorf("roles.%s: fields.%s is a %s; a role is a person", role, role, t)
		}
	}
	return nil
}

func (s *Schema) checkDescriptions() error {
	for _, name := range s.names() {
		f := s.Fields[name]
		for _, v := range slices.Sorted(maps.Keys(f.Descriptions)) {
			if n := utf8.RuneCountInString(f.Descriptions[v]); n > 100 {
				return fmt.Errorf("fields.%s.descriptions: %q's is %d characters, GitHub takes 100 at most", name, v, n)
			}
		}
	}
	return nil
}

// checkKindWords: a branch starts with its kind's label, so a kind's
// word is the kind itself; bug = "fix" is refused at its line.
func checkKindWords(data []byte, kinds map[string]string) error {
	for _, k := range slices.Sorted(maps.Keys(kinds)) {
		w := kinds[k]
		if w == k {
			continue
		}
		at := regexp.MustCompile(`(?m)^.*\b` + regexp.QuoteMeta(k) + `\s*=\s*"` + regexp.QuoteMeta(w) + `"`).FindIndex(data)
		line := ""
		if at != nil {
			line = fmt.Sprintf("line %d: ", bytes.Count(data[:at[0]], []byte("\n"))+1)
		}
		return fmt.Errorf(`%s[branches] kinds: %s = %q: a branch starts with its kind's label, write %s = %q (its branches start %s/)`, line, k, w, k, k, k)
	}
	return nil
}

// checkLabels: a listed label is plain, not a field's value nor a kind;
// the colors and descriptions are the list's own.
func (s *Schema) checkLabels() error {
	l := s.Labels
	for _, name := range l.List {
		switch {
		case strings.TrimSpace(name) == "":
			return errors.New("[labels] list: an empty label")
		case strings.Contains(name, ":"):
			return fmt.Errorf("[labels] list: %q is a field's value (name:value): give that field the option instead", name)
		case s.Kinds[name] != "":
			return fmt.Errorf("[labels] list: %q is a kind, in [branches] kinds already", name)
		}
	}
	for _, name := range slices.Sorted(maps.Keys(l.Colors)) {
		color := l.Colors[name]
		if !slices.Contains(l.List, name) {
			return fmt.Errorf("[labels] colors: %q isn't in the list", name)
		}
		c := NormColor(color)
		if !isColor(c) {
			return fmt.Errorf("[labels] colors: %q for %q isn't a color: %s, or #rrggbb", color, name, strings.Join(Palette, ", "))
		}
		l.Colors[name] = c
	}
	for _, name := range slices.Sorted(maps.Keys(l.Descriptions)) {
		if !slices.Contains(l.List, name) {
			return fmt.Errorf("[labels] descriptions: %q isn't in the list", name)
		}
		if n := utf8.RuneCountInString(l.Descriptions[name]); n > 100 {
			return fmt.Errorf("[labels] descriptions: %q's is %d characters, GitHub takes 100 at most", name, n)
		}
	}
	return nil
}

// checkAliases refuses an alias that shadows an option or folds into
// nothing.
func (s *Schema) checkAliases() error {
	for _, name := range s.names() {
		f := s.Fields[name]
		for _, old := range slices.Sorted(maps.Keys(f.Aliases)) {
			cur := f.Aliases[old]
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

// Field is a field as the forges' mappings check it: its type, whether a
// value is one of its options, and whether a table declares it.
func (s *Schema) Field(name string) (string, func(string) bool, bool) {
	f, ok := s.Fields[name]
	return s.TypeOf(name, f.Options), f.KnownValue, ok
}
