package issues

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/guilbep/git-board/next/domain/people"
)

// Described is a field as a page offers it: the configured one, its type
// and unit resolved, and what to pick from (v1's /api/config/fields).
type Described struct {
	Type     string            `json:"type"`
	Single   bool              `json:"single"`
	Numeric  bool              `json:"numeric"`
	Options  []string          `json:"options"`
	Unit     string            `json:"unit,omitempty"`
	Current  string            `json:"current,omitempty"`  // an iteration's sprint today
	Inherits string            `json:"inherits,omitempty"` // a role's project value ([roles])
	People   []string          `json:"people,omitempty"`   // who a person field offers
	Colors   map[string]string `json:"colors,omitempty"`   // a value's color, as configured
}

// multiByName are the issue fields that hold several values.
var multiByName = map[string]bool{"blocks": true, "blocked-by": true, "depends-on": true}

// Fields are the board's fields with their types: the configured ones,
// and those the issues' labels show whose type isn't a plain select (a
// date, a person, an issue). An iteration takes [[sprints.list]]'s slugs,
// by start, unless it has options, and names today's; a role is a person
// field with the project's value; a person field offers the people.
func (s *Service) Fields(ctx context.Context, today time.Time) (map[string]Described, error) {
	ls, err := s.Labels(ctx)
	if err != nil {
		return nil, err
	}
	values := map[string][]string{}
	for _, l := range ls {
		if l.Field != "" {
			values[l.Field] = append(values[l.Field], l.Value)
		}
	}
	res := map[string]Described{}
	for name, f := range s.Schema.Fields {
		res[name] = Described{Type: s.Schema.TypeOf(name, values[name]), Unit: s.Schema.UnitOf(name),
			Single: f.Single, Numeric: f.Numeric, Options: f.Options, Colors: f.Colors}
	}
	for name, vs := range values {
		if _, ok := res[name]; ok {
			continue
		}
		t := s.Schema.TypeOf(name, vs)
		if t == TypeSelect {
			continue
		}
		single := t != TypePerson && !(t == TypeIssue && multiByName[name])
		res[name] = Described{Type: t, Unit: s.Schema.UnitOf(name), Single: single, Numeric: t == TypeNumber}
	}
	if sp := s.Schema.Sprints; len(sp.List) > 0 {
		ordered := slices.Clone(sp.List)
		sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Start.Before(ordered[j].Start) })
		current := ""
		if c := sp.Resolve("current", today); c != nil {
			current = c.Slug
		}
		if _, ok := res["sprint"]; !ok && s.Schema.TypeOf("sprint", nil) == TypeIteration {
			res["sprint"] = Described{Type: TypeIteration, Single: true}
		}
		for name, f := range res {
			if f.Type != TypeIteration {
				continue
			}
			if len(f.Options) == 0 {
				for _, w := range ordered {
					f.Options = append(f.Options, w.Slug)
				}
			}
			f.Current = current
			res[name] = f
		}
	}
	for role, who := range s.Schema.Roles {
		f := res[role]
		f.Type, f.Single, f.Inherits = TypePerson, false, who
		res[role] = f
	}
	var names []string
	if dir, err := people.Read(ctx, s.Board, s.Avatars); err == nil {
		for id, p := range dir.ByID {
			n := p.Name
			if ls := dir.Logins(id); len(ls) > 0 {
				n = ls[0]
			}
			if n = strings.TrimSpace(n); n != "" && !slices.Contains(names, n) {
				names = append(names, n)
			}
		}
	}
	slices.Sort(names)
	for name, f := range res {
		if f.Options == nil {
			f.Options = []string{}
		}
		if f.Type == TypePerson {
			f.People = names
		}
		res[name] = f
	}
	return res, nil
}
