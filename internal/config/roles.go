package config

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

// Roles are a project's people by role, as the team's roles page names
// them: [roles] lead-dev = "bob", pm = "claire", reviewer = "alice". A role
// is a person field; an issue's own value (lead-dev:dan) overrides the
// project's.
// ponytail: one person per role, as the roles page has it; a list when a
// project shares one

// RolesFor is what an issue with these labels takes from the project: a
// role:login label for each role it doesn't set itself.
func (c *Config) RolesFor(labels []string) []string {
	if c == nil {
		return nil
	}
	var out []string
	for _, role := range slices.Sorted(maps.Keys(c.Roles)) {
		if !slices.ContainsFunc(labels, func(l string) bool { return strings.HasPrefix(l, role+":") }) {
			out = append(out, role+":"+c.Roles[role])
		}
	}
	return out
}

var roleName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// checkRoles refuses a role that isn't a field name, names nobody or
// several, or is a field of another type than person.
func checkRoles(roles map[string]string, fields map[string]Field) error {
	for role, who := range roles {
		switch t := fields[role].Type; {
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
