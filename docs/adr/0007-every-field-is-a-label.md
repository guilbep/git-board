# ADR 0007: Every field is a label, `ns:value`

- Status: accepted (recorded 2026-11-04, decided earlier); its forge half
  amended by `docs/prd/forge-field-mappings.md` (2026-11-01).
- Date: before 2026-09-02 inside git-board (`docs/implementation-plan.md`
  › Why this exists: "field values live as namespaced git-bug labels");
  confirmed for the core and the forges on 2026-10-24 (`docs/prd/core.md`
  › The data model, in brief; Answered by guilbep).
- Sources: `docs/spec/data-model.md` › Labels; `docs/guide/configuration.md`
  › Fields, Types; `internal/config/mapping.go`.

## Context

git-board began as a replacement for a GitHub Project of 811 items and
19 fields that drifted outside git (`docs/implementation-plan.md` › Why
this exists). Fields had to travel with the issues, through git and through
every forge, and each project needs its own fields (status workflow,
priority, sprint, estimate, MoSCoW, parent…). Forges disagree on native
fields: GitHub's issue types and Projects, GitLab's milestones, weights,
scoped labels, several of them paid.

## Decision

### 1. In the board, a field value is a label

- Every field besides title, body, status (open/closed) and comments is a
  label `ns:value`: `status:ready`, `prio:p1`, `due:2026-10-30`,
  `parent:#12`, `assignee:guilbep` (`docs/spec/data-model.md` › Labels). A
  label without a colon is a plain label.
- The core has one operation for all of them, `change-labels`, and gives no
  namespace a meaning. Labels merge one by one, so they never conflict
  (`docs/spec/sync.md` › The four cases).
- git-board gives them meaning from `.git-board.toml` (ADR 0011):
  `[fields.<ns>]` adds order (`options`), cardinality (`single`), a type
  (`select`, `iteration`, `person`, `number`, `date`, `issue`, `text`),
  colors and units. Any namespace found on issues is already a field; the
  file only adds what labels can't say (`docs/guide/configuration.md` ›
  Fields).
- A single-value field swaps: setting it removes the namespace's other
  labels (`core/fields/fields.go:1-9`).
- The workflow status (`status:in-progress`) is a field like any other. The
  issue's own `set-status` is open or closed; `git board close` sets the
  last option and closes (`docs/guide/configuration.md` › Fields).

### 2. On a forge, labels by default; native slots by mapping

As decided on 2026-10-24: "The forges get labels, never their own fields",
with one exception, assignees, which always became the forge's own
assignees, since a forge notifies only its own.

As built since 2026-11-01, each forge may map a field elsewhere
(`[forges.<name>.fields.<field>] to = …`, `internal/config/mapping.go:15-27`).
A field with no mapping takes its type's default
(`internal/config/mapping.go:78` `DefaultTarget`):

| Type | GitHub | GitLab |
| --- | --- | --- |
| `person`, field `assignee` | native assignees | native assignees |
| `issue` | sub-issues | label |
| `iteration` | label | milestone |
| `date` | label | due date |
| `text` | none (git-board only) | label |
| anything else | label | label |

On GitHub with `[github-project]`, the Project's fields default to the
Project (`internal/config/mapping.go:58-66`).

The core's forge sync still writes only labels and native assignees. A
label whose field lives elsewhere on that forge is **held**: never sent,
never pulled, never removed there (`internal/config/config.go:511`
`Held`, passed as `syncer.Options.Held`). `milestone:` is always held.

## Alternatives considered

Partly recorded: the PRD weighed forge fields and chose labels
(2026-10-24); the mapping PRD then made the forge side configurable.
Scored on: one model for every forge (30), fields travel through git with
no schema change (30), forge users see the values natively (20), cost
(20).

| Option | Score | Why not |
| --- | --- | --- |
| **Labels in the board, labels on forges by default, mapped slots where configured (this decision)** | 80 | |
| Labels everywhere, assignees only native (the 2026-10-24 form) | 70 | Status in a Project column, or dates as GitLab due dates, couldn't be chosen |
| Typed fields in the data model (a `set-field` operation per field) | 50 | A schema change for every new field; each forge's field model to mirror; conflicts per field type |
| The forges' own fields as the source (GitHub issue types, Projects) | 25 | Not in git, differs per forge, often paid; the drift git-board was made to end |

## Consequences

Good:

- A new field needs no code and no migration: write a label.
- Fields sync through git and through any forge that has labels.
- Set merges per label: two clones adding different labels both keep
  them.

Bad, and accepted:

- A single-value field can hold two values after a merge between clones
  (each swapped offline); git-board's lint reports it.
- Values aren't validated by the store: a value outside `options` "still
  works; `git board lint` reports it".
- Forge users see `status:ready` as a label, unless mapped.
- Label names are case-insensitive on GitHub: `Assignee:` is read as
  `assignee:` (`core/syncer/pull.go:88`).

## Left open

- **Drift: the decision's text.** `docs/spec/decisions.md` and
  `docs/prd/core.md` still say "the forges get labels, never their own
  fields". The code maps fields to native slots per forge
  (`internal/config/mapping.go`). Those lines need the amendment.
- **Mapped but not written.** `milestone`, `sub-issues` and `due-date` are
  accepted targets, and defaults for some types, but the forge interface
  has no setter for them (`core/forge/forge.go:85-126`). A GitLab date
  field or a GitHub `issue` field is therefore held off that forge and
  lives only in git-board (`forge-field-mappings.md` › Milestones 3 and 5).
- `as = "plain"` (a value spelled without its namespace on a forge) is
  refused until sync writes it.
- `docs/guide/configuration.md` › Types says an issue field names "its
  git-bug id"; it is the core's id now (ADR 0003).
