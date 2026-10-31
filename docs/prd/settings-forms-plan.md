# Project file as forms: one settings tab per section, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Settings › Project file stops being a TOML editor with a
reference panel. Each section of `.git-board.toml` becomes its own tab in
the settings' left sidebar, with forms to add, change and remove its
elements, and presets to start from. The user never edits the file in the
interface (an *Advanced* tab keeps the raw editor as an escape hatch);
git-board holds the file ↔ API ↔ frontend path: the server patches one
table of the file per form save, keeping every other line and comment, the
way the board's *Save view* already does.

**Architecture:** One new server surface, `GET /api/config/model` (every
section as typed JSON, from the config the server already holds, plus the
file's hash) and `PUT /api/config/{section}` (one section's typed value;
the server rewrites that table in the text with a new family of
`internal/config` writers beside `SetViewText`, checks the whole file,
saves on a hash compare-and-swap, and serves the fresh model). The
frontend gets a shared module `web/src/configmodel.ts` (load, per-section
save, dirty and conflict state) and one component per tab, built on the
form patterns of `SyncSettings.vue`. `settingstabs.ts` grows the tabs;
`project-file` redirects to `fields`. The raw `ConfigEditor` stays, as the
group's *Advanced* tab. Core is untouched.

**Tech Stack:** Vue 3, `node:test`; Go (`net/http`, `internal/config`).

**Spec:** `docs/guide/configuration.md` (each section's keys and meaning —
the forms' field lists come from its tables); `internal/config/default.git-board.toml`
(the presets); `internal/config/setview.go` (the writer pattern); `docs/prd/settings-forms-plan.md`
(this file). Where the docs and the code disagree, the code wins and the
docs get a line in the milestone.

**Base:** main at `3f31324`, branch `feat/settings-forms`. This plan goes
first, at `docs/prd/settings-forms-plan.md` (Task 0), as the branch's
first commit: no pull request of its own, it lands with the code.

## Decisions taken (with the user)

1. **The raw editor survives as a hidden advanced tab**, in the same
   sidebar group, last: forms are the way; *Advanced* is the escape hatch.
2. **No duplication.** Sections that already have a form page keep it and
   keep their tab: *Sync* (forges, GitHub Project), *Numbers*, *Worktrees*.
   The new tabs cover the sections that had no form. Cross-links between
   related tabs (fields ↔ labels, branches ↔ cycle, sync ↔ forges).
3. **Milestones**, this doc first, then M1–M3 below.

## What exists today, found in the code

**The page.** `SettingsPage.vue:85-92` renders `ConfigEditor.vue` in the
`project-file` tab; `settingstabs.ts` holds the tab list and the `moved`
map that redirects old addresses. `ConfigEditor.vue` loads `GET /api/config`
(text, hash, `default`), checks drafts through `POST /api/config/check`
(300 ms debounce), shows a line diff, saves through `PUT /api/config`
(`{text, base}`; the server re-checks, compares the hash, saves, applies).
`web/src/configdraft.ts` is the shared draft text; `web/src/configsnippets.ts`
appends a View/Sprint/Field snippet.

**The writer pattern to copy.** `internal/config/setview.go`: `SetViewText`
splits the text into lines, finds the table's span (`tableSpan`), rewrites
its keys in place keeping trailing comments (`keyLine`), adds missing keys
after the last one, appends the table when absent, and touches nothing
else. `setSubTable` (same file) writes a whole `[parent.sub]` table and is
already used for `[fields.<name>.colors|descriptions]` (`SetFieldTable`)
and `[labels.colors|descriptions]` (`SetLabelsTable`) — called from
`internal/server/labels.go` for Adopt and Send today. `tomlString`,
`tomlArray`, `tomlTable` and `unquote` are there.

**The save.** `internal/server/configfile.go`: `readConfigFile` (text,
exists, sha256 hash), `checkConfig` (parse, warnings, summary),
`handleConfigSave` (CAS on hash → `409 changed`), `config.Save`,
`s.setConfig(cfg)` (live for every request at once, live events fire).

**Forms to reuse.** `SyncSettings.vue` (a form per forge, add/remove,
save bar, check-then-apply), `NumbersSettings.vue`, `WorktreesSettings.vue`
(preview-then-write), `LabelsSettings.vue` (rows with state per repo,
adopt/send/delete). The settings shell (`SettingsPage.vue`) takes any tab
id from `settingstabs.ts` and a `v-if` per tab — adding tabs is cheap.

**The coupling to redo.** `LabelsSettings.vue:184-185` Adopt writes the
adopted color/description into `configText` (the raw draft) and pushes to
`#/settings/project-file` for a human to review and save. With the raw
editor out of the main path, Adopt calls `PUT /api/config/fields` directly
and reports the save itself; `configdraft.ts` and the `configText` import
in `ConfigEditor` collapse into the editor alone.

**Presets the server already proposes.** `GET/POST /api/checks/setup`
(`internal/server/checks.go`) proposes `[cycle.checks.*]` from lefthook,
the Makefile and package.json; the Cycle tab reuses it instead of
re-inventing proposal. `config.Default` (`internal/config/default.go`)
is the default file the presets are drawn from.

## The tabs

`settingstabs.ts`, with groups as `SettingsPage.vue` already renders them
(`set-group` headings):

| Group | Tab | Section(s) of the file | Form |
| --- | --- | --- | --- |
| — | Overview | — | unchanged (its project-file card lists the sections' counts) |
| Where your issues go | Sync | `[forges.*]`, `[github-project]` | unchanged |
| The project file | Fields | `[fields.<name>]` (+ `.colors`, `.descriptions`, `.project`) | one card per field: name, type (the 7 types), options (add/reorder/remove), single, numeric, unit; colors and descriptions as rows; *Add a field* with a preset menu (status, prio, est, kind, hold, moscow, due, points, parent, assignee…) |
| | Labels | `[labels]` | the `list` as an editable tag list, colors and descriptions as rows; below, the existing GitHub comparison from `LabelsSettings.vue` (merged: one Labels tab, the `[labels]` forms on top) |
| | Views | `[views.<name>]` | one card per view: name, layout, filter (with the filter syntax hint), group, sort, columns, lanes; *Add a view* with presets (open, backlog, board, mine, current-sprint); delete with a confirm saying what the address `#/…?view=` breaks |
| | Sprints | `[sprints]`, `[[sprints.list]]` | length and weekday; the list as rows (slug, title, start date); *+ Sprint* computes the next start from `length_days`, as the board's button does |
| | Roles | `[roles]` | rows: role name → login, with the role-as-person-field note |
| | Branches | `[branches]` | base, name pattern (with the `{kind} {number} {n} {slug}` legend), kinds as rows (kind → branch word) |
| | Cycle | `[cycle]` (+ `.kinds`, `.signoffs`, `.tests.*`, `.checks.*`) | on/off first (an empty `[cycle]` is enough); guessed-stages banner (what `settings.cycle.guessed` says today); the keys as selects/inputs; steps as an ordered list; kinds and signoffs as rows; tests and checks as cards (run argv, files, dir, where, env) with *Propose from the project* (`/api/checks/setup`) |
| | Release | `[release]`, `[[release.deploy]]` | environments (ordered list), follow, checks, images, chart, deploys; the *Enable binary release* helper stays on the Releases page |
| | Repos | `[repos]` | the list, plus `assignees` (read-only note: not read any more) |
| | Attachments | `[uploads]` | the tab that today only reports state becomes the form: prefix (endpoint, public_url, region are defaults — shown, editable, with their default noted); the env-var note stays under it |
| | Numbers | `refs/board/seq` | unchanged (its own tab already) |
| | Worktrees | `wtx.toml` | unchanged (not this file) |
| | Advanced | the whole file | the existing `ConfigEditor.vue`, unchanged, labelled *for the rare case the forms don't cover* |
| — | You | — | unchanged |

`project-file` joins the `moved` map → `fields`. Old links (the board's
*Save view* bar, `LabelsSettings`, `ReleasesPage`) are re-pointed in M1.

**Where the docs' eighteen headings go** (`docs/guide/configuration.md` is
the spec's own table of contents; every heading must land somewhere):

| Docs heading | Tab | How |
| --- | --- | --- |
| Overview | Fields | the group's intro lines; the full text stays the Advanced tab's opening reference |
| Fields | Fields | the tab itself |
| Types | Fields | `type` is a per-field property: a select of the 7 types on each field card, `Guessed from` as the option's hint; the types table is the tab's reference |
| Colors | Fields, Labels | `[fields.<n>.colors]` and `[labels.colors]` are rows on their tab's cards, edited with the palette swatches (grey…pink, or a hex) the old reference panel showed |
| Labels | Labels | the tab itself |
| Views | Views | the tab itself |
| Roles | Roles | the tab itself |
| Filters | Views | no file section exists — it is the filter *syntax*. Its doc renders beside the filter inputs of the Views tab (and the board's filter box keeps its own hints); a view's `filter` input links to it |
| Sprints | Sprints | the tab itself |
| GitHub Project | Sync | already a form there; the model serves it read-only for context |
| Forges | Sync | already a form there |
| Attachments | Attachments | the tab itself |
| Branches | Branches | the tab itself |
| Cycle | Cycle | the tab itself |
| Worktrees | Worktrees | unchanged (its own tab, not this file) |
| Numbers | Numbers | unchanged (its own tab) |
| Release | Release | the tab itself |
| Repos | Repos | the tab itself |

## The reference, per tab

The old page's right-hand panel (the docs section under the cursor,
rendered, its TOML examples colored) survives per tab, not per cursor:

- **`/api/config/docs` is unchanged** — the server still cuts
  `docs/guide/configuration.md` at its `##` headings; the ids are the
  headings' first words (`fields`, `types`, `colors`, `labels`, `views`,
  `roles`, `filters`, `sprints`, `github-project`, `forges`, `attachments`,
  `branches`, `cycle`, `worktrees`, `numbers`, `release`, `repos`).
- Each tab knows its sections and renders them in a **Reference panel** —
  folded by default, a toggle in the tab's head, beside the form on wide
  screens and under it on narrow (the `@container` pattern the old editor
  used). The rendering pipeline is the old one, lifted into the shared
  `configforms/` bits: `gb-md`, tables labelled per column when narrow,
  `tomlTokens` over `pre > code.language-toml`.
- A tab maps to more than one section when its form spans them: Fields →
  `fields`, `types`, `colors`; Labels → `labels`, `colors`; Views →
  `views`, `filters`; Cycle → `cycle`; the rest map one to one.
- The **Advanced tab keeps the cursor-following panel as is** —
  `docSectionAt`, the nav chips, all of it; nothing there is lost.
- The color inputs (Fields', Labels' color rows) reuse the old panel's
  swatch strip as a picker: a palette name or a `#rrggbb` input, the same
  nine the file allows.

## The API

```
GET /api/config/model
  → { hash, exists,
      fields:  { <name>: { options[], single, numeric, type, unit,
                           project{}, colors{}, descriptions{} } },
      labels:  { list[], colors{}, descriptions{} },
      views:   { <name>: ViewConfig },        // what /api/views returns today
      sprints: { length_days, start_weekday, list[{slug,title,start}] },
      roles:   { <role>: login },
      branches:{ base, name, kinds{} },
      cycle:   { ...the [cycle] keys, kinds{}, signoffs{},
                 tests{<suite>:{run,files,dir,where,env,tools}},
                 checks{...} },
      release: { ..., deploy[] },
      repos:   { list[], assignees },
      uploads: { prefix, endpoint, public_url, region } }
```

Built from the config the server already holds (`s.config()`), not from a
re-read — one source of truth, already applied. The hash comes from
`readConfigFile` so a save can CAS without a second read.

```
PUT /api/config/{section}      section: fields|labels|views|sprints|roles|
                                         branches|cycle|release|repos|uploads
  body: that section's typed value (the same shape the model holds)
  → a ConfigCheck (ok, warnings, summary) + the fresh model of that section
```

One route, one decode per section into the section's Go type, then:

1. read the file text (`readConfigFile`);
2. apply the section's writer (`internal/config`): `SetFieldsText`,
   `SetLabelsListText`, `SetViewText`/`DeleteViewText`, `SetSprintsText`,
   `SetRolesText`, `SetBranchesText`, `SetCycleText`, `SetReleaseText`,
   `SetReposText`, `SetUploadsText` — each the `SetViewText` pattern:
   rewrite or append only its tables, every other line, comment and order
   untouched; a section emptied of a table removes the table (and one
   leading blank line, as `setSubTable` does);
3. `checkConfig` the new text — not ok → `400` with the check (the form
   shows it; the file is untouched);
4. hash CAS → `409 changed` as today;
5. `config.Save`, `s.setConfig`, return check + model.

New config-package writers get table tests in `internal/config`
(`testdata` round-trips: a file with comments in and around every table,
saved twice, byte-stable the second time). Server tests follow
`configfile_test.go`: save each section, read the model back, the other
sections byte-identical in the text.

`GET /api/config/check` and `PUT /api/config` stay: the Advanced tab and
nothing else use them.

## The frontend

**`web/src/configmodel.ts`** (new, pure where it can be): the model ref,
`load()`, `saveSection(name, value)` (returns the check; raises `changed`
as a typed error), and `dirty` tracking per tab (draft vs model). A tab
that is dirty says so in its tab row (a dot), and leaving the settings
page with a dirty tab asks, as the composer does.

**Shared bits** (`web/src/components/configforms/`, small): `SectionCard`
(title, hint, slot, remove), `RowList` (ordered rows with add/remove/up/
down — the options, labels list, environments, sprints), `SaveBar` (the
sticky bar every tab ends with: state chip as `ConfigEditor`'s, Discard,
Save), `PresetMenu` (a button opening the preset list, each with a
one-line description).

**One component per tab**, `FieldsSettings.vue`, `LabelsSettings.vue`
(extended), `ViewsSettings.vue`, `SprintsSettings.vue`, `RolesSettings.vue`,
`BranchesSettings.vue`, `CycleSettings.vue`, `ReleaseSettings.vue`,
`ReposSettings.vue`, `AttachmentsSettings.vue` — each: load the model,
draft locally, validate what it can locally (a field name's shape, a
branch pattern's placeholders, a date), Save through `saveSection`, show
the server's warnings under the form, and embed its Reference panel (see
"The reference, per tab" above — the sections of
`docs/guide/configuration.md` it owns, folded until asked for).

**Presets** (`web/src/configpresets.ts`, pure, tested): the default file's
tables, each as `{ label, why, value }` — the ten field presets with the
default file's options and descriptions, the four view presets, the
sprint defaults, the branch kinds, the cycle's default steps and kinds.
The preset menu *adds and selects*, never overwrites silently; a name
that exists appends `-2` as the board's *Save as new view* does.

**Testing:** `node:test` beside the components where logic is pure
(`configpresets.test.ts`, `configmodel.test.ts` with a stubbed fetch),
tab tests in `settingstabs.test.ts` (the new list, the redirects); Go
tests as above. `make ci` green per milestone.

## Milestones

### M1 — the plumbing and the board's core sections

- [x] Task 0: this plan, first commit on `feat/settings-forms`.
- [x] `internal/config`: `SetFieldsText`, `SetRolesText`, `SetLabelsListText`,
  `SetSprintsText`, `DeleteViewText` + tests (round-trip, comments kept,
  append/remove/rename).
- [x] `internal/server/configsections.go`: `GET /api/config/model`,
  `PUT /api/config/{section}` for fields, labels, views, sprints, roles;
  tests.
- [x] `web/src/configmodel.ts` + tests; `settingstabs.ts` the new tab list
  and redirects; `SettingsPage.vue` renders the group; the shared
  `configforms/` bits including the **Reference panel** (folded, the
  `/api/config/docs` sections rendered as the old editor did).
- [x] `FieldsSettings.vue` (with the types select and the palette-picker
  color rows), `RolesSettings.vue`, `SprintsSettings.vue`,
  `ViewsSettings.vue` (with the filter-syntax reference beside the filter
  input), `LabelsSettings.vue` extended with the `[labels]` forms; Adopt
  switches to `PUT /api/config/fields`; `configdraft.ts` retired from
  everything but the Advanced tab.
- [x] Old links re-pointed (`ReleasesPage`, `LabelPicker`, `LabelsSettings`,
  the board's Save-view bar); Overview's project-file card lists sections.
- [x] `make ci` green; a handover note.

### M2 — the work: branches, cycle, repos, attachments, release

- [ ] Writers: `SetBranchesText`, `SetCycleText` (keys, kinds, signoffs,
  tests, checks), `SetReposText`, `SetUploadsText`, `SetReleaseText` + tests.
- [ ] The API grows those sections; tests.
- [ ] `BranchesSettings.vue`, `CycleSettings.vue` (with *Propose from the
  project* through `/api/checks/setup`), `ReposSettings.vue`,
  `AttachmentsSettings.vue`, `ReleaseSettings.vue`.
- [ ] `make ci` green; handover.

### M3 — presets, polish, docs

- [ ] `configpresets.ts` filled out and wired into every *Add* menu; the
  *No project file yet* state of the old editor becomes the Fields tab's
  empty state: *Start from the default* writes the whole default file
  through one save (a `PUT /api/config/fields`-like whole-default route or
  the existing `PUT /api/config` — decided when built, default: the raw
  route, it is exactly what it is for).
- [ ] Dirty-tab guard, empty states, narrow layout (the `@container`
  patterns `ConfigEditor` already uses), keyboard pass, contrast pass
  (EPFL tokens only, no raw hex/px — same rules as the rest of the app).
- [ ] `docs/guide/configuration.md`: each section's heading gains its
  settings address ("Settings › Fields"); the Overview card's words.
- [ ] `make ci` green; final review and handover.

## Risks and rulings

- **Comments are the contract.** Every writer keeps lines it doesn't own,
  byte for byte; the round-trip tests hold that line. A writer that cannot
  express a change without reordering the file refuses (`400`), it never
  rewrites.
- **Two writers, one table.** `views` is written by the board's *Save
  view* (`handleSaveView`) and by the new tab; both go through
  `SetViewText`/`DeleteViewText`, so no drift.
- **The GitHub Project and forges stay in Sync.** Their forms live there;
  the model serves them read-only for context, and the tabs that mention
  them link to `#/settings/sync`.
- **The guessed cycle.** The Cycle tab shows the guess (as the editor's
  banner does today) and writes the explicit form only for what the user
  changes — writing the whole guessed table back would freeze the guess.
  The save body carries only the keys the form touched.
- **`[cycle]` on the base branch.** The checks' gate reads the file from
  the base; the tab edits the working tree, and says so, as
  `/api/checks/setup` already does.
- **Validation belongs to the server.** The forms pre-validate shapes
  (names, dates, patterns) for fast feedback; the check after save is the
  truth, and its warnings show under the form.
