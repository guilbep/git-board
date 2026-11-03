# Fields: one editor — Implementation Plan

> Steps use checkbox (`- [ ]`) syntax. One commit per task; each leaves the app working.

**Goal:** Where a field lives on each forge is edited in the field editor; one Save writes `[fields.<name>]` and `[forges.*.fields.<name>]`; Forge fields leaves the sidebar.

**Architecture:** The server's `fields` section body gains `forges`, applied in the same write after the fields change; removing a field drops its mappings. The web moves `ForgeFieldsSettings.vue`'s row logic (targets, Project field, renames, migration) into a `FieldPlaces.vue` block inside `FieldsSettings.vue`, fed by one combined draft. Sync › <forge> gets a summary line.

**Spec:** `docs/prd/fields-one-editor.md`; the model: `docs/prd/forge-field-mappings.md`.

## Global Constraints

- No new dependency. No change to `config.FieldMapping`, the targets, the defaults or the sync.
- One file write per Save, guarded by `base` as today.
- Old route `#/settings/forge-fields/<forge>` redirects to `#/settings/fields`.
- Go tests `t.Parallel()` first (CLAUDE.md); web unit-tested modules import types relatively.

## Review Focus

1. **Atomic**: a fields change and a forge change in one Save are one write; a bad forge mapping refuses the whole Save and writes nothing. Pinned in Task 1.
2. **Removing a field removes its mappings** on every forge. Pinned in Task 1.
3. **A found forge** (no `[forges.*]` yet): the first place changed writes the found forges' tables, as `forge-fields` does today. Pinned in Task 1.
4. **Migration asks before running**, with the counts, and runs after the write. Pinned in Task 4 (e2e).

---

### Task 1: one write for both tables (Go)

**Files:** `internal/server/configsections.go`; tests in `internal/server/configsections_test.go` (or beside the `forge-fields` tests).

- [ ] Tests: `fields` with `forges.github.set.prio = {to: project, field: Priority}` writes both tables in one save (one hash change); a mismatched mapping (`scoped-label` on GitHub) → 400, file unchanged; `remove: ["prio"]` drops `[forges.*.fields.prio]` on every forge; with the forges only found in the remotes, the found forges' tables are written first.
- [ ] Implement: decode `Forges map[string]struct{ Set map[string]config.FieldMapping; Remove []string }` in the `fields` case; after the fields edits, for each forge (sorted), `resolveFound` once if needed, then `config.SetForgeFieldsText`; for each removed field, remove it from every forge's fields. Keep `forge-fields` working (the old screen until Task 5).
- [ ] `go test ./internal/server/ -run 'Fields|ForgeFields'`. Commit `feat(fields): one save for a field and its places`.

### Task 2: the places, as a pure module (web)

**Files:** create `web/src/fieldplaces.ts` (+ test, added to `npm test`), moving the row logic out of `ForgeFieldsSettings.vue`: targets per forge kind and field type, the default, override detection, the Project columns, renames, the body for the save.

- [ ] Unit tests: the default per type and forge kind; an override vs the default; the body's `set`/`remove` from a draft; renames emptied mean no rename.
- [ ] Commit `refactor(fields): the places' rules in fieldplaces.ts`.

### Task 3: "Where it lives" in the field editor

**Files:** create `web/src/components/FieldPlaces.vue`; modify `FieldsSettings.vue` (its draft carries `forges`; the TOML tab and aside show both tables; the list's chips).

- [ ] The block per the mockup: a row per forge (icon, name, target select with the default marked, details, "Names on …" disclosure); overrides in blue.
- [ ] The draft: `forges` beside `set/remove/order`; dirty when either changes; the save bar counts both.
- [ ] The list: a chip per forge with the place; outlined when not the default.
- [ ] Typecheck, unit tests. Commit `feat(fields): where a field lives, in its editor`.

### Task 4: migration from the editor

**Files:** `FieldPlaces.vue`, `FieldsSettings.vue`; `web/e2e/fields.spec.ts` (new, or `settings.spec.ts`).

- [ ] Changing a target calls `previewFieldMigration` and shows the counts under the block; Save asks ("Save and migrate N issues on GitHub?"), saves, then `runFieldMigration`.
- [ ] e2e: change `prio` to Project › a column on the e2e forge; the counts show; Save writes both tables (read `.git-board.toml`: `boardFile()`), nothing migrates before the confirmation.
- [ ] Commit `feat(fields): changing a place shows and runs its migration`.

### Task 5: Forge fields leaves the sidebar

**Files:** `web/src/settingstabs.ts`, `pages/SettingsPage.vue` (the tab, and its `forge-fields/<forge>` address read in the page), `components/SyncSettings.vue` (the summary), delete `ForgeFieldsSettings.vue` once nothing uses it; the e2e tests that open Forge fields: `settings.spec.ts` (two), `review.spec.ts` (C-22, D-33), `layout.spec.ts` (the written-to panel), `tabs.spec.ts`.

- [ ] The tab goes; `#/settings/forge-fields/:forge` redirects to `#/settings/fields`; Sync › <forge> shows "Where fields live on <forge>: …" and "Change it in Fields".
- [ ] Rewrite those five e2e tests to do the same from Fields (what each checks stays: the found forge's table written, a Project column saved and removed, the kind said once, the written-to panel).
- [ ] `docs/prd/forge-field-mappings.md` UI section: a line pointing here. `docs/guide/configuration.md` Forges: "set in Settings › Fields, on each field".
- [ ] `make ci`. Commit `feat(fields): forge fields edited on each field; the tab goes`.
