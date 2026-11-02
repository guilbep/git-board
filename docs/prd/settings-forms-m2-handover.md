# Settings forms M2+M3, handover

The project file is fully form-driven now: every section of
`.git-board.toml` has its settings tab, presets included, and nothing
needs the raw file except the unusual. The plan is
`docs/prd/settings-forms-plan.md` (all boxes checked); branch
`feat/settings-forms`. M1's rulings hold (`settings-forms-m1-handover.md`);
this adds what M2 and M3 took and the rulings they took.

## What M2 delivers

- **The writers** (`internal/config/setsection.go`):
  `SetBranchesText` (base, name, `[branches.kinds]` — the default file's
  inline kinds come back as the table form, same meaning),
  `SetCycleText(text, on, cycle)` — off deletes every `[cycle.*]` table,
  on writes the keys, the kinds as string rules (a table form's rule is
  the same `tests` value; a kind's future `steps` would be lost, the
  check warns), and the suites with their `[cycle.tests.<name>.env]`
  subtables and `tools` argv-of-argv — `SetReposText` (own table: list
  plus the preserved `assignees` setting), `SetUploadsText`,
  `SetReleaseText` (the chart subtable, images as an inline array,
  `[[release.deploy]]` replaced whole in the form's order).
- **The API**: `sectionWriter` takes `branches`, `cycle` (`{on, cycle}`),
  `repos` (`{list, assignees}`), `uploads`, `release`; `configModel`
  serves them with `cycle.on` set by whether the file has the table.
- **The tabs**: Branches (base, name template, kinds with the kind
  field's options as a datalist), Cycle (on/off toggle, stages, gates,
  kinds' rules, suites, and **Propose from the project** — GET
  `/api/checks/setup`'s proposal lands in the draft, saving is the
  section's own CAS), Release (environments with their roles spelled
  out, images, chart, deploy, binaries folded), Repos, Attachments.

## What M3 delivers

- **The empty state**: Fields shows *Start from the default* when the
  file doesn't exist; it writes the whole default through the raw
  `PUT /api/config` (the ruling: the raw route, exactly what it is for)
  and reads the model back.
- **The last presets**: branch kinds, release environments
  (dev/prod, dev/stage/prod) beside the M1 ones.
- **The docs' addresses**: every `## section` of
  `docs/guide/configuration.md` carries "*These are the forms of
  Settings › <Tab>; this section is the reference.*" under its heading
  (headings unchanged: the reference panel titles itself from them), and
  the prose's stale "Settings › Project file" mentions point at Cycle
  and Advanced.
- **The guards**: leaving Settings with a draft asks — the router's
  `onBeforeRouteLeave` beside the `beforeunload`; the preset menu closes
  on Escape and returns the focus.

## Rulings taken in M2/M3 (keep these)

- **`cycle` is on/off plus a value.** The empty `[cycle]` table alone
  turns the cycle on, so the save body is `{on, cycle}` and `on: false`
  means *delete the family*, never "save an empty table".
- **A suite's `run` is an argv**, so the form's one input splits on
  whitespace (`words` in CycleSettings); `where` is a select of
  worktree/ci. `env` edits later (M3+ if wanted); the writer preserves
  what the file has only while the form doesn't touch the suite —
  editing a suite rewrites its table, so its env rows are re-sent as
  parsed. Suites the draft drops are deleted (`subTables` finds them).
- **`tools = [["go", "version"]]` needed `keyLine` to match one nested
  bracket level** (`\[(?:[^\[\]]|\[[^\[\]]*\])*\]`); the flat `\[[^\]]*\]`
  truncated the value and rewrote it broken (a stray `]` per save).
- **`deleteTable` keeps the file's trailing newline** and no longer
  removes the blank line above a table — the blank above becomes the
  separator that stays; a trailing table's deletion keeps the file's
  final newline. It also matches `[[array.tables]]` now.
- **Empty means gone, everywhere**: every writer maps `""`/`[]` values
  to "remove the line" and an all-empty section to "delete the table"
  — the forms send `undefined` for a cleared map, and the writers take
  it. Watch the trap: `tomlArray(nil)` is `""`, not `"[]"` — an empty
  check must accept both.
- **`release.follow` is tri-state** (`*string`): absent = the first
  environment when "dev" is listed; the form sends `follow: null` for
  "follow the default", a name for explicit, and the writer writes the
  line only when set.
- **Repos' `assignees` is preserved, not edited** — the Sync owns how
  forges hold assignees; the Repos tab keeps whatever the file says and
  the writer re-emits it when it isn't `native`.

## Verified

`go build/vet/test` (config + the full server suite), `web-check`
(typecheck + 308 node tests), `web-build`, gofmt — all green. Live
round-trip on a scratch repository: branches/cycle/release/repos/uploads
saves each byte-stable on a second save, cycle-off deletes the family,
`409 changed` on a stale base, `/api/checks/setup` answers (empty for a
repository with nothing to propose).

## Known small gaps (deliberate)

- A suite's `env` and `files` have no rows in the Cycle tab yet; the
  reference panel documents them, Advanced edits them, and a save that
  doesn't touch a suite leaves its table untouched.
- `[cycle.kinds]` table-form kinds (a kind's own tests table) are written
  back as string rules when the kinds are edited — same rule, different
  byte form. `steps` per kind (the plan's M3 idea) doesn't exist yet.
- The Overview's project-file card lists sections; it could grow each
  section's one-line state (n fields, m views) when someone wants it.
