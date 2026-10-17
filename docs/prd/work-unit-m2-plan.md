# Work unit M2: roles, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A project names its people by role in `[roles]` (lead dev, PM, code
reviewer); every issue takes them unless it sets its own, and filters,
boards and facets see the inherited value.

**Architecture:** `config.RolesFor(labels)` is the one rule: the
`role:person` labels an issue takes from `[roles]` for the roles it doesn't
set. `internal/board` puts them on each issue it loads as
`gitbugio.Issue.Inherited`; filtering, grouping, sorting and facets read
`Issue.AllLabels()` (own + inherited); edits keep writing own labels only.
The web side panel gets each role's project value from
`/api/config/fields` (`inherits`) and shows it greyed when the issue has none.

**Tech Stack:** Go, BurntSushi/toml, Vue 3 + Quasar.

**Spec:** `docs/prd/work-unit.md`, section M2.

## Global Constraints

- `[roles]` maps a role to one login: `lead-dev = "bob"`. A role is a
  `person` field of the same name; an issue's own label (`lead-dev:dan`)
  overrides the project's.
- A role name is a field name: `^[a-z][a-z0-9-]*$`. A role declared in
  `[fields]` with a type other than `person` is a config error.
- Inherited values are never written to git-bug: setting a role on an issue
  writes its own label; removing it falls back to the project's.
- `assignee` stays a plain per-issue person field (the contributing
  developer); it has no project value unless `[roles]` names one.
- No new dependency.

## Review Focus

1. **An issue that sets the role itself** is not matched by the project's
   value: `-lead-dev:bob` finds it, `lead-dev:bob` doesn't. (Task 2:
   `TestRolesInherit`.)
2. **`no:lead-dev`** finds nothing while the project names a lead dev.
   (Task 2.)
3. **A config without `[roles]`** changes nothing: no inherited labels, no
   new facets. (Task 1: `RolesFor` on a nil/empty config.)
4. **A role declared as another type** (`[fields.pm] type = "select"`) fails
   loading with a message naming it. (Task 1.)
5. **The side panel** shows the inherited value greyed and still lets you
   add an own value. (Task 3, checked in the app.)

---

### Task 1: `[roles]` in the config

**Files:**
- Create: `internal/config/roles.go`, `internal/config/roles_test.go`
- Modify: `internal/config/config.go` (`raw`, `Config`, `parse`), `internal/config/types.go` (`TypeOf`)

**Interfaces:**
- Produces: `Config.Roles map[string]string`; `func (c *Config) RolesFor(labels []string) []string`;
  `TypeOf` answers `person` for a role.

- [ ] **Step 1: Write the failing test** — `internal/config/roles_test.go`:

```go
package config

import (
	"slices"
	"strings"
	"testing"
)

// A project names its people by role; an issue takes each role it doesn't
// set itself.
func TestRoles(t *testing.T) {
	cfg, _, err := parse([]byte("[roles]\nlead-dev = \"bob\"\npm = \"claire\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.RolesFor([]string{"status:ready", "lead-dev:dan"}); !slices.Equal(got, []string{"pm:claire"}) {
		t.Errorf("RolesFor = %v, want pm:claire alone", got)
	}
	if got := cfg.RolesFor(nil); !slices.Equal(got, []string{"lead-dev:bob", "pm:claire"}) {
		t.Errorf("RolesFor(nil) = %v", got)
	}
	if cfg.TypeOf("pm", nil) != TypePerson {
		t.Errorf("pm is a %s", cfg.TypeOf("pm", nil))
	}
	var none *Config
	if got := none.RolesFor(nil); got != nil {
		t.Errorf("no config: %v", got)
	}
	if got := (&Config{}).RolesFor(nil); got != nil {
		t.Errorf("no [roles]: %v", got)
	}

	for text, want := range map[string]string{
		"[roles]\nPM = \"x\"\n":                                "roles.PM",
		"[roles]\npm = \"\"\n":                                 "roles.pm",
		"[roles]\npm = \"a, b\"\n":                             "roles.pm",
		"[fields.pm]\ntype = \"select\"\n[roles]\npm = \"x\"\n": "a person",
	} {
		if _, _, err := parse([]byte(text)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want an error naming %q", text, err, want)
		}
	}
}
```

- [ ] **Step 2: Run it — FAIL:** `go test ./internal/config/ -run TestRoles` → `cfg.RolesFor undefined`.

- [ ] **Step 3: Implement** — `internal/config/roles.go`:

```go
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
		case t != "" && t != TypePerson:
			return fmt.Errorf("roles.%s: fields.%s is a %s; a role is a person", role, role, t)
		}
	}
	return nil
}
```

`config.go`: add to `raw` `Roles map[string]string \`toml:"roles"\``; to `Config`
(after `Branches`):

```go
	// Roles is the [roles] table: the project's people by role (roles.go).
	Roles map[string]string
```

in `parse`, after `checkTypes`: `if err := checkRoles(r.Roles, r.Fields); err != nil { return nil, md, err }`,
and `Roles: r.Roles,` in the returned `Config`.

`types.go`, `TypeOf`: after `case f.Numeric:`, add
`case c.Roles[name] != "": return TypePerson` and say so in its comment
("else a role ([roles]), a person").

- [ ] **Step 4: Run it — PASS:** `go test ./internal/config/`.
- [ ] **Step 5: Commit:** `feat(roles): [roles], a project's people by role`.

---

### Task 2: issues inherit their roles: filters, boards, facets

**Files:**
- Modify: `internal/gitbugio/gitbugio.go` (`Issue.Inherited`, `AllLabels`)
- Modify: `internal/board/board.go` (`Match`, `FieldValue`, new `withRoles`), `internal/board/facets.go` (`Facets`, `facetKeys`, `valuesOf`)
- Modify: `internal/filter/filter.go` (`Matches`, `matchOne`)
- Test: `internal/board/roles_test.go`

**Interfaces:**
- Consumes: `(*config.Config).RolesFor` (Task 1).
- Produces: `Issue.Inherited []string` (json `inherited`); `func (is Issue) AllLabels() []string`.

- [ ] **Step 1: Write the failing test** — `internal/board/roles_test.go`:

```go
package board_test

import (
	"slices"
	"testing"

	"github.com/guilbep/git-board/internal/board"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio/gitbugiotest"
)

// Issues take the project's roles unless they set their own: filters,
// groups and facets see the inherited value, the PM's board pm:@me too.
func TestRolesInherit(t *testing.T) {
	b := gitbugiotest.New(t)
	b.NewIssue("inherits", "", nil)
	b.NewIssue("overrides", "", []string{"lead-dev:dan"})
	cfg := &config.Config{Roles: map[string]string{"lead-dev": "bob", "pm": "claire"}}
	titles := func(filter, me string) []string {
		t.Helper()
		issues, err := board.LoadMatching(b, cfg, filter, me)
		if err != nil {
			t.Fatalf("%q: %v", filter, err)
		}
		var out []string
		for _, is := range issues {
			out = append(out, is.Title)
		}
		slices.Sort(out)
		return out
	}
	for filter, want := range map[string][]string{
		"lead-dev:@me":  {"inherits"},
		"lead-dev:dan":  {"overrides"},
		"-lead-dev:bob": {"overrides"},
		"pm:@claire":    nil,
		"no:lead-dev":   nil,
	} {
		if got := titles(filter, "bob"); !slices.Equal(got, want) {
			t.Errorf("%q: %v, want %v", filter, got, want)
		}
	}
	if got := titles("pm:@me", "claire"); !slices.Equal(got, []string{"inherits", "overrides"}) {
		t.Errorf("the PM's board, pm:@me: %v", got)
	}

	all, _ := board.LoadMatching(b, cfg, "", "bob")
	groups := map[string]int{}
	for _, bk := range board.BucketBy(all, "lead-dev") {
		groups[bk.Group] = len(bk.Issues)
	}
	if groups["bob"] != 1 || groups["dan"] != 1 {
		t.Errorf("grouped by lead-dev: %v", groups)
	}

	facets, err := board.Facets(b, cfg, "", "bob")
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range facets {
		if f.Key == "lead-dev" {
			counts := map[string]int{}
			for _, o := range f.Options {
				counts[o.Value] = o.Count
			}
			if counts["bob"] != 1 || counts["dan"] != 1 {
				t.Errorf("lead-dev facet: %v", counts)
			}
			return
		}
	}
	t.Error("no lead-dev facet")
}
```

(`pm:@claire` is not `@me`: it's the literal value "@claire", matching
nothing; it guards against a resolver treating any `@x` as a login.)

- [ ] **Step 2: Run it — FAIL:** `go test ./internal/board/ -run TestRolesInherit`
→ `lead-dev:@me` finds nothing.

- [ ] **Step 3: Implement**

`gitbugio.go`, `Issue`, after `Labels`:

```go
	// Inherited are the labels it takes from its project ([roles]) for the
	// roles it doesn't set itself; internal/board sets them.
	Inherited []string `json:"inherited,omitempty"`
```

and a method beside `Headline`:

```go
// AllLabels are its own labels and those it inherits: what filters,
// groups and facets see. Edits change its own labels only.
func (is Issue) AllLabels() []string { return append(slices.Clip(is.Labels), is.Inherited...) }
```

`board.go`: add

```go
// withRoles gives each issue the roles it takes from the project.
func withRoles(cfg *config.Config, issues []gitbugio.Issue) {
	for i := range issues {
		issues[i].Inherited = cfg.RolesFor(issues[i].Labels)
	}
}
```

call `withRoles(cfg, issues)` at the top of `Match` (after `prepare`),
and in `FieldValue` range over `issue.AllLabels()`.

`facets.go`: in `Facets`, `withRoles(cfg, issues)` right after
`b.ListIssues()`; in `facetKeys` and `valuesOf`, range over
`issue.AllLabels()`.

`filter.go`, `Matches`: build `labels` from `issue.AllLabels()`; in
`matchOne`, the `KindLabel` and `KindNo` loops range over the `labels` map
(`for l := range labels`) instead of `issue.Labels`.

- [ ] **Step 4: Run — PASS:** `go test ./internal/...`.
- [ ] **Step 5: Commit:** `feat(roles): issues take the project's roles; filters, boards and facets see them`.

---

### Task 3: the side panel shows the inherited role

**Files:**
- Modify: `cmd/git-board-server/server.go` (`fieldResponse.Inherits`, `handleFields`)
- Modify: `web/src/api/board.ts` (`FieldConfig.inherits`), `web/src/components/IssueSide.vue`
- Test: `cmd/git-board-server/roles_test.go`

- [ ] **Step 1: Write the failing test** — `cmd/git-board-server/roles_test.go`:

```go
package main

import (
	"testing"

	"github.com/guilbep/git-board/internal/config"
)

// The page learns each role's project value with its field, even before
// any issue sets the role.
func TestFieldsCarryRoles(t *testing.T) {
	ts, s := newIssueServer(t)
	s.cfg = &config.Config{Roles: map[string]string{"lead-dev": "bob"}}
	var fields map[string]struct {
		Type     string `json:"type"`
		Inherits string `json:"inherits"`
	}
	call(t, "GET", ts.URL+"/api/config/fields", nil, &fields)
	if f := fields["lead-dev"]; f.Type != config.TypePerson || f.Inherits != "bob" {
		t.Errorf("lead-dev: %+v", f)
	}
}
```

(If `s.config()` reads another field than `s.cfg`, set that one.)

- [ ] **Step 2: Run — FAIL:** `go test ./cmd/git-board-server/ -run TestFieldsCarryRoles`.

- [ ] **Step 3: Implement**

`fieldResponse`: add `Inherits string \`json:"inherits,omitempty"\` // a role's project value ([roles])`.
In `handleFields`, before `writeJSON`:

```go
	// a role is a person field with the project's value, even unused yet
	for role, who := range cfg.Roles {
		f := res[role]
		f.Type, f.Single, f.Inherits = config.TypePerson, false, who
		res[role] = f
	}
```

`board.ts`, `FieldConfig`: `inherits?: string; // a role's project value ([roles]), shown when the issue sets none`.

`IssueSide.vue`, in the multi-value branch, before `<form class="gb-addrow"`:

```html
            <p v-if="!values(name).length && fields[name]?.inherits" class="gb-inherited">{{ fields[name]!.inherits }} <span>· project</span></p>
```

and in its `<style scoped>`: `.gb-inherited { margin: 0 0 .25rem; color: var(--fg-muted); font-style: italic; }`.
Check the side lists role fields: `props.names` comes from the fields
response's keys (follow `names` from `IssuePage.vue`/`IndexPage.vue`); if it
doesn't, add the roles there.

- [ ] **Step 4: Run — PASS:** the Go test; `cd web && npm run typecheck && npm test`.
- [ ] **Step 5: See it** in the playground (local bare mirror as origin) with
  `[roles] lead-dev = "guilbep"`: an issue's side shows "guilbep · project"
  under Lead dev.
- [ ] **Step 6: Commit:** `feat(roles): the side panel shows a role the issue takes from the project`.

---

### Task 4: docs

**Files:**
- Modify: `docs/configuration.md` (a `## Roles` section after `## Views`; the Boards per role example's `pm` view gets `filter = "is:open pm:@me"`)
- Modify: `cmd/git-board-server/configfile_test.go` (sections list: `roles` after `views`)
- Modify: `web/src/configsnippets.ts` (`[roles]` → `roles`), `web/src/configsnippets.test.ts`

- [ ] **Step 1:** in `configsnippets.test.ts`, add `assert.equal(at('[roles]'), 'roles');`
  beside the `[repos]` assertion (adapt to how `at` builds its text); run
  `cd web && npm test` — FAIL.
- [ ] **Step 2:** `configsnippets.ts`: `if (header.startsWith('[roles]')) return 'roles';`
  beside `[repos]`; `npm test` — PASS.
- [ ] **Step 3:** the docs section:

````markdown
## Roles

A project's people by role, as the team's roles page names them: the lead
developer, the project manager, the code reviewer. Every issue takes them,
unless it sets its own.

```toml
[roles]
lead-dev = "bob"
pm = "claire"
reviewer = "alice"
```

A role is a `person` field of the same name: `lead-dev:dan` on an issue
overrides the project's `bob` for that issue, and removing it gives `bob`
back. Filters, boards and the filters panel see the inherited value:
`lead-dev:@me` finds the issues that take you as lead dev, and
`no:lead-dev` finds none while the project names one. The side panel shows
an inherited value greyed, "bob · project". `assignee` is the contributing
developer, set per issue.

| Key | What it is |
| --- | --- |
| a role (`lead-dev`, `pm`, `reviewer`, …) | One login. The name is a field's: lower-case letters, digits and `-`; declared in `[fields]`, it is a `person`. |
````

and in "Boards per role", the `[views.pm]` example gains `filter = "is:open pm:@me"`.
- [ ] **Step 4:** the sections test expects `…,views,roles,filters,…`; run
  `go test ./...` — PASS.
- [ ] **Step 5: Commit:** `docs: roles, a project's people by role`.
