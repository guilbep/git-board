# Settings forms M1, handover

The project file's sections are form tabs now. This is what landed, what
the rulings were, and where M2 picks up. The plan is
`docs/prd/settings-forms-plan.md`; branch `feat/settings-forms`.

## What M1 delivers

- **The API.** `GET /api/config/model` answers every section typed,
  parsed fresh from the file (`error` set when it doesn't parse; the forms
  show that, Advanced fixes it). `PUT /api/config/section/<name>?base=<hash>`
  applies one section — `fields`, `labels`, `views`, `sprints`, `roles` —
  through the config writers, checks the whole file, saves on the usual
  hash CAS (`409 changed`), and serves the check and the fresh model,
  live for every request at once. `views` takes `{set, remove, order}`;
  `fields` takes `{set, remove}`; the others take the section's value.
- **The writers** (`internal/config/setsection.go`), each the
  `SetViewText` pattern: only its tables change, every other line and
  comment byte-stable, a new table joins its family (a field after the
  last `[fields.*]`, not the file's end). `setTableKeys` is
  `SetViewText`'s core, refactored out (`own`: the form owns the table —
  roles; a key line it doesn't name goes). `deleteTable`, `MoveViewText`
  (the file's order is the tabs', realized pairwise) are beside it.
- **The tabs.** Settings' sidebar grows "The project file": Fields,
  Labels, Views, Sprints, Roles, then the M2 tabs (Branches, Cycle,
  Release, Repos — placeholder panels), Numbers, Worktrees,
  **Advanced** (the raw editor, untouched). `project-file` redirects to
  `fields`. Each tab: one load (`configmodel.ts`), a local draft, a
  sticky save bar, presets to add from (`configpresets.ts`), the guide's
  own sections folded beside the form (`ConfigRefPanel`), a dot on a
  dirty tab and a beforeunload guard.
- **Labels** gained the `[labels]` forms above its GitHub comparison;
  **Adopt** now saves through the section API instead of staging text in
  the raw draft (`configdraft.ts` is the Advanced tab's alone).

## Rulings taken (keep these)

- **The server is the truth.** A form pre-validates shapes; the check
  after save decides, its warnings show under the form. `noUncheckedIndexedAccess`
  is on in the web build — index carefully.
- **Removal is derived, not tracked.** For `fields` and `views` the body's
  `remove` is the names the model has that the draft lost — delete and
  rename both fall out of it. Don't add a separate removed-list.
- **`ConfigRows`' slot is `#row` with `{row, index, set}`** — slot props
  aren't assignable, so a row input binds through `set(...)`. Objects
  passed as rows keep identity, so `v-model="row.x"` works on them.
- **The model's JSON tags matter**: `encoding/json` matches
  case-insensitively but not across underscores — any new section type
  needs explicit `json` tags or the PUT silently drops keys (this bit
  `SprintsConfig.LengthDays`).
- **json tags went on `Labels`, `Sprint`, `SprintsConfig`** (config.go) —
  they're served and taken back by the forms.
- A rename of a *saved* element is refused by the UI (the name input is
  disabled once saved): the file's tables are named by the element, and
  the forms rename by remove+add of values, never by editing history.

## M2 picks up here

Branches, Cycle (tests/checks with `/api/checks/setup`), Repos,
Attachments, Release. The server route takes new sections in
`sectionWriter` (configsections.go) with a writer in
`internal/config/setsection.go` — the model gains the section in
`configModel`, the tab in `SettingsPage`'s placeholder list. The M3 bits
(the *Start from the default* empty state, the dirty-tab guard's polish,
the docs' per-section addresses) are the plan's.

## Verified

`make ci`'s parts green (`go build/vet/test`, `web-check` typecheck+308
tests, `web-build`); a live round-trip on a scratch repository: model
read, roles/fields/views/sprints saves, the file's comment and untouched
tables byte-identical, `409 changed` on a stale base.
