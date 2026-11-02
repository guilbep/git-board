# git-board · Fields settings: list + editor PRD

Nov 1, 2026 · @Pierre Guilbert

Settings › Fields moves from a long stack of field cards to a **field list on the left and one field's editor on the right**. Every field stays visible at once, and each field gets its own address.

## Problem

The current page stacks one card per `[fields.<name>]` table. It breaks down in two ways.

**It doesn't scale.** At 10 fields the page is several screens long. You can't see which fields exist, find the one you want, or compare two of them without scrolling back and forth. Field order, which sets the order fields appear on an issue, is invisible.

**Each card asks the wrong questions in the wrong order.** The old card puts name, type, "single", "numeric" and unit before the values, then shows colors, descriptions and GitHub Project names as three separate lists matched by index. Specifically:

- The Type dropdown shows "guessed", which tells the user nothing.
- "numeric" repeats what the values already show.
- Up/down buttons on every row of a numeric field, which sorts itself anyway.
- Count badges (5, 0, 0, 0) that carry no decision.
- The form and `.git-board.toml` can drift: one screenshot showed `1, 2, 4, 8, 16` while the file held `0.5, 1, 2, 4, 8`.

## Goals and non-goals

Goals:

1. See every field, and what kind it is, on one screen without scrolling, up to at least 20 fields.
2. Open, edit and save any one field in two clicks or fewer.
3. Set field order by dragging.
4. Make the values the first thing you edit; infer everything the values already say.
5. Keep the form and `.git-board.toml` in sync, and let people edit either one.

Non-goals:

- Editing the values on individual issues. This page defines fields only.
- Changing the TOML schema beyond what the form needs (see Data model).
- Bulk editing several fields at once.
- Syncing with GitHub Project. The GitHub name column only stores a mapping; Sync does the syncing.

## Users and scenarios

The main user is the one person on each team who manages ordering and priorities and owns the project file. Other team members mostly read this page to see what a field means.

| Scenario | Today | With this design |
| --- | --- | --- |
| "Which fields do we have?" | Scroll through every card | Read the list |
| Add the value `16` to `est` | Scroll to the card, click Add an option, type | Click `est`, type `16`, press Enter |
| Move `priority` above `est` on issues | Not possible in the form | Drag the row |
| Give `review` an orange color | Find the Colors list, match it by index | Open the per-value table, pick the color on the `review` row |
| Check what the file says | Open the file elsewhere | Switch the editor to File |
| Switch fields with unsaved edits | Edits silently stay in the card | A dot on the row; leaving asks to save or discard |

## Solution overview

The page becomes three columns: the existing Settings menu, a list of every field, and the editor for the selected field. Fields sits under **Project file** in the Settings menu (open question below).

[embedded content: Settings › Fields layout · three columns at 1280 px]

Clicking a row opens that field on the right. Header tabs were ruled out because 10 field names don't fit in one row and leave no room for a summary. Collapsible cards were ruled out because they still scroll. To make room, the always-visible TOML panel from the first mockup becomes a Form | File switch inside the editor.

## Requirements

P0 = needed to ship, P1 = soon after, P2 = later.

### Field list (middle column)

| ID | Requirement | Priority |
| --- | --- | --- |
| L1 | One row per field, in file order: drag handle, name (monospace), one-line summary such as `numbers · one · h`, and a preview of the values (`todo → doing → review → done`; truncated with … past the row width) | P0 |
| L2 | Clicking a row opens that field in the editor and highlights the row (`aria-current`) | P0 |
| L3 | Drag a row (or press Alt+↑/↓ on a focused row) to reorder fields; the new order is written to the file | P0 |
| L4 | A dot on rows with unsaved edits | P0 |
| L5 | "+ Add a field" at the bottom: adds a row named `new_field`, opens it, focuses the name | P0 |
| L6 | A filter box above the list, shown once there are more than 12 fields | P1 |
| L7 | A warning icon on rows whose TOML failed to parse, with the error on hover | P1 |

### Field editor (right column)

| ID | Requirement | Priority |
| --- | --- | --- |
| E1 | Header: `fields.` + an editable name, the inferred summary, and the save state (Saved / Unsaved changes) | P0 |
| E2 | Values as chips in one input: Enter adds a value; pasting a comma- or newline-separated list adds them all; × removes one | P0 |
| E3 | When every value parses as a number: show "All values are numbers", a Unit input, and a "Treat them as labels instead" link. No Type dropdown, no numeric checkbox | P0 |
| E4 | Label values reorder by dragging chips (left = first in the workflow); number values sort by value and can't be dragged | P0 |
| E5 | "An issue can have": one value / several values / any text, no list | P0 |
| E6 | Per-value details: a collapsed toggle with a one-line summary; expanded, a table with one row per value and columns Color, Description, GitHub Project name | P0 |
| E7 | Save and Discard in the footer; Remove field… in the footer, with a confirm that names how many issues use the field | P0 |
| E8 | Renaming a field warns that issues keep the old key until migrated | P1 |

### Form | File switch

| ID | Requirement | Priority |
| --- | --- | --- |
| F1 | A Form \| File switch at the top of the editor. File shows that field's TOML table, editable | P0 |
| F2 | Switching back to Form re-parses the TOML; a parse error keeps you in File and shows the error and line | P0 |
| F3 | If the file changes on disk (e.g. after a pull) while the form has unsaved edits, show a banner offering to reload or keep editing | P1 |
| F4 | A "View the whole file" link opens all of `.git-board.toml` read-only | P2 |

### Saving and addresses

| ID | Requirement | Priority |
| --- | --- | --- |
| S1 | Each field has its own address: `#/settings/fields/<name>`. `#/settings/fields` opens the first field | P0 |
| S2 | Leaving a field with unsaved edits asks: Save, Discard, or Stay | P0 |
| S3 | Saving rewrites only that field's table and leaves comments and formatting elsewhere in the file alone | P0 |
| S4 | Saving validates first: no duplicate values, no empty field name, no duplicate field name | P0 |

### Layout, small screens, accessibility

| ID | Requirement | Priority |
| --- | --- | --- |
| R1 | At 1280 px and up: Settings menu ~220 px, field list ~280 px, editor takes the rest (~650 px) | P0 |
| R2 | Under 960 px the Settings menu collapses into a dropdown above the page | P0 |
| R3 | Under 640 px the list and the editor are separate screens, with a Back link in the editor | P0 |
| R4 | The list is a real list of links; every control is keyboard-reachable; buttons and rows are at least 44 px tall; text contrast is at least 4.5:1 | P0 |

## Data model

The form reads and writes `[fields.<name>]` tables in `.git-board.toml`; the file is the source of truth. Checked against the parser (`internal/config/config.go:27`, `Field` struct) — the real keys are:

| Key | Type | Set by the form when | Status |
| --- | --- | --- | --- |
| `options` | array of strings | Always, except for `type = "text"` | Exists |
| `single` | bool | One value: `true`; several: `false` | Exists |
| `numeric` | bool | Inferred from the values (every value a number), unless the user picks labels | Exists |
| `unit` | string | The Unit input has text | Exists |
| `type` | string | One of `select, iteration, person, number, date, issue, text`; empty = inferred by `TypeOf` | Exists |
| `colors` | inline table, value → color | A value has a color | Exists |
| `descriptions` | inline table, value → string | A value has a description | Exists |
| `project` | inline table, value → GitHub Project option name | The GitHub name differs from the value | Exists (not `github`) |

Notes from the parser:

- **Free text already exists**: `type = "text"`. No `free_text` key.
- **`numeric` is sparse in the file**: the server writes `numeric = true` only when true (`SetFieldsText`), and `TypeOf` infers numbers from the values anyway — E3's "infer everything the values say" matches the file's own convention.
- **Field order needs no new file key.** The file's `[fields.*]` table order is recoverable server-side (`keysInOrder(md, r.Fields, "fields")`, the exact pattern views already use for `view_order`, exposed as JSON `field_order`). Go's map loses the order, so the API must carry it — but the file stays as-is, order = the tables' order in the file.
- **S3 is already true today**: `SetFieldsText` edits the file textually — only the touched field's tables change, comments and formatting elsewhere stay byte-for-byte.
- **S4 (duplicate names/values, empty name) shipped** in `541f598`.

Example of what the form writes for `est`:

```toml
[fields.est]
single  = true
numeric = true
unit    = "h"
options = ["0.5", "1", "2", "4", "8"]
```

## Success metrics and acceptance

The page succeeds if the person who owns the project file stops editing `.git-board.toml` by hand for routine changes. Checks to sign off on:

- [ ] With 20 fields at 1280×800, every field row is visible without scrolling the page.
- [ ] Adding one value to an existing field takes 2 clicks plus typing.
- [ ] Dragging a field row changes the order fields appear on an issue, after a reload.
- [ ] Typing `0.5, 1, 2, 4, 8` sets `numeric = true` and sorts the chips, with no other input.
- [ ] Editing in File and switching back to Form shows the same values; a bad edit shows the error and line.
- [ ] Saving one field leaves the rest of the file byte-for-byte unchanged, comments included.
- [ ] Leaving a field with unsaved edits always asks first.
- [ ] Opening `#/settings/fields/est` directly opens `est`.
- [ ] The whole page works by keyboard, including reordering fields and values.
- [ ] At 375 px wide, the list and the editor work as separate screens.

## Open questions

Answered against the code (milestone 1, Nov 1 2026):

- [x] **Where Fields lives in Settings.** Under Project file, as today's tab — decided Nov 1 2026. The GitHub Project mapping (`project`) stays a per-field column here; the Forges canvas's "3 · GitHub Project · fields" is this column, not a separate tab.
- [x] **Real key names.** All exist: `unit`, `colors`, `descriptions`, `project` (not `github`), `type` with `text` for free text. The Data model table above is corrected and locked.
- [x] **What "guessed" means.** `type` empty = inferred by `TypeOf`: explicit type → numeric → role → name (sprint, assignee, due…) → what the values parse as → select. The editor should *show* the inference ("numbers", "dates", "forge logins") instead of a dead "guessed" entry.
- [x] **The 1, 2, 4, 8, 16 vs 0.5, 1, 2, 4, 8 mismatch.** The form reads the file (the draft builds from the server-parsed model, and inputs bind `:value` since `ec93614`). The screenshot predates that binding fix — inputs drifted from the file. F2 (Form | File) already shipped at card granularity (`2225a891`); F3 is new, but the server already answers 409 `changed` on a stale hash, which covers the concurrent-save case.
- [x] **Field order today.** None anywhere: `Fields` is a Go map (JSON marshals it alphabetically), the TUI sorts, the web's field line uses a hardcoded preferred list. L3 is therefore two ends: expose `field_order` in the API (views' `view_order` pattern, no file change), and make an issue-side consumer read it. One server caveat: `SetFieldsText` writes set tables in sorted order — reordering needs an order-aware writer (delete + re-append in order, or a sorted-set of the remaining).
- [x] **Renames.** The mechanics exist (remove + set in one section save; the TOML switch already renames this way) and issues keep their `old:value` labels. The lint already reports unknown namespaces, and the page shows every namespace found on issues. Answer: rename leaves issues untouched, E8's warning + lint cover migration; a rename-issues action is deferred.
- [x] **Usage counts.** No endpoint today. Cheap: a small `GET /api/fields/usage` counting issues per `field:` namespace in one pass over the store (the client-side alternative needs issues the settings route doesn't load). E7's Remove confirm and L4-style badges can both use it.

## Milestones

No dates set yet. Each step is shippable on its own.

1. **Answer the open questions** — done (above); the Data model table is locked.
2. **Layout and list:** three columns, the field list (L1–L5), per-field addresses (S1), and the existing editor dropped into the right column unchanged.
3. **New editor:** chip values, numbers inferred, one/several/any text, the per-value table (E1–E7), plus saving with validation (S2–S4).
4. **Form | File switch** (F1–F2) and small-screen layouts (R2–R3).
5. **P1 follow-ups:** list filter, parse-error rows, on-disk change banner, rename warning.
