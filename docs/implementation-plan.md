# Implementation plan / status

Living doc — update as milestones land. See the PRD (in the original design
conversation, not checked in here yet) for full context; this tracks state.

## Done

- [x] M1 (fields, `set`/`get`/`ls`/`view`, filter grammar, config):
  `.git-board.toml` parsing, `@current`/`@next`/`@previous`/`@me`
  resolution, single/multi-cardinality label swap logic, filter grammar.
- [x] Ported Python prototype → Go, **importing git-bug as a Go library**
  (`cache.RepoCache`, `ChangeLabels`, `Commit`) instead of shelling out to
  the `git-bug` binary — no CLI dependency, in-process. First draft wrapped
  the CLI; abandoned mid-build once that was clearly not the goal.
- [x] Verified against git-bug's actual source: no `bug ls` subcommand
  exists (`git bug bug --format json` lists). `ChangeLabels` errors on an
  all-no-op call — callers must pre-filter against current labels.
  Pinned `git-bug@v0.10.1`.
- [x] Tests: real git-bug repo caches via `t.TempDir()` + git-bug's own
  test helpers, no mocks, no `git-bug` binary required to test.
- [x] README: install (incl. `GOPRIVATE`), dev/test commands.

## Not done (M2+ by design)

- [ ] `lint` (double singles, unknown options, open w/o prio, orphan
  sprints, unknown assignees, dead options).
- [ ] `move`, `close`.
- [ ] `sprint ls|current|new|report`.
- [ ] `html` (static site for PM/non-dev browsing, cron-published).
- [ ] `sync` (multi-repo `git bug` pull/push).
- [ ] `import-github` (bridges GitHub Project 40 into the label model —
  needed before real dogfooding on co2-calculator's actual 811 items).
- [ ] Numeric-aware `--sort` (currently lexical; fine for the one shipped
  sort `prio asc`, marked `ponytail:` in `cmd.go`).

## Dogfood feedback (first real try, 2026-09-02)

Tried against a repo with git-bug initialized. Immediate finding: **the CLI
output is unusable for actually browsing a board** — plain table dump,
no scroll, no search/incremental filter. This was expected (M1 never built
a TUI, just `internal/*` split out so one *could* be added later) but it's
now a confirmed real blocker to adoption, not a hypothetical one.

### Open decision: TUI (bubbletea) vs. jump straight to a web UI (Quasar/Vue)

Two ways to fix "no scroll, no search":

1. **Build the planned `bubbletea` TUI.** Reuses 100% of `internal/*`
   as-is (that's exactly why it's split out). No new server, no new
   deploy target, stays inside the git-native/no-vendor architecture the
   whole project is built on. Scope: a `git-board` (no args) or
   `git-board tui` command with scrollable list, `/` incremental search
   over the filter grammar already implemented, `enter` to `get`, maybe
   `s`/`p` quick-set keys for status/prio. Order of magnitude: similar
   size to M1 itself.
2. **Skip to a Quasar+Vue web UI**, matching co2-calculator's own
   frontend stack. Bigger commitment: needs a backend API (the PRD's own
   §8 sketches "FastAPI + git-bug GraphQL bridge"), hosting, auth — i.e.
   re-introduces the server this whole project was designed to avoid.
   The PRD explicitly frames this as the **v2 last resort**, "not before"
   CLI+cron-HTML adoption is tried and fails for non-dev users — it
   wasn't meant to be the first response to the CLI feeling clunky on
   day one.

**Recommendation carried into this doc, not yet acted on:** try the TUI
first — it's the cheap, architecturally-consistent fix for the actual
complaint (scroll/search), and only escalate to a real web app if that
still doesn't work for non-dev adoption, per the PRD's own escalation
criteria. Revisit this note once the TUI's been tried; don't build both.

## Open questions (from the PRD, unresolved — don't re-litigate, just track)

- git-bug pre-1.0, CLI/library surface has already diverged from
  assumptions once — recheck compatibility before bumping past `v0.10.1`.
- Sub-issues / `parent:` label: decide at M2 whether co2-calculator's
  linked-PR items need it before v2.

## Status update (2026-09-02): web UI landed

Despite the TUI-first recommendation above, the web UI was built next
(explicit ask, not a re-litigation of that note) — **Go backend, not the
PRD's old FastAPI sketch**, since that assumed a Python implementation that
no longer exists after the Go rewrite.

- `internal/board` (new): `loadMatchingIssues`/`fieldValue`/sort/`bucketBy`
  pulled out of `cmd/git-board`'s `package main` into an importable package
  — it had exactly one caller before, now it has three (CLI, TUI, web API).
  No behavior change for the CLI; `go test ./...` stayed green throughout.
- `cmd/git-board-server` (new): stdlib `net/http` + Go 1.22 method-pattern
  `ServeMux`, no router framework. `GET /api/views`, `GET
  /api/config/fields`, `GET /api/issues` (filter+sort+group — group is
  applied server-side via `board.BucketBy`, unlike the CLI's own `--json`
  which ignores it), `GET`/`PATCH /api/issues/{id}` (PATCH goes through
  `fields.Apply`, same swap semantics as `set`). Tested with
  `httptest.Server` + `gitbugiotest` real-repo fixtures, no mocks — plus a
  manual end-to-end smoke run (built binary, real git-bug repo, `curl`
  against every endpoint including the embedded static UI and a CORS
  preflight) before calling it done.
- **No auth in v1**, matching git-bug's own `webui` precedent (trusted
  internal network). Loud caveat in the README, not a silent gap.
- `web/` (continued the existing Quasar/Vue scaffold, not restarted): one
  board page — view/filter picker, a table per group (Quasar's `QTable` has
  no built-in `group-by` row-grouping like Vuetify does, so grouping is
  server-side buckets rendered as one table per bucket instead), click a
  row for a detail dialog with per-field inputs (`QSelect` for
  single-cardinality fields, chip input diffed into add/remove `PATCH`
  calls for multi-cardinality `assignee`). All HTTP through
  `web/src/api/board.ts`. No routing library added; the generated
  vue-router scaffold was trimmed to one route instead of ripped out
  (smaller diff, still effectively one page). No test framework shipped
  with the scaffold (no Vitest) — not added for this MVP; `vue-tsc`
  type-checks clean and `npm run build` produces a real `web/dist`.
- Production is one binary: `web/embed.go` (`//go:embed all:dist`) embeds
  `web/dist`, and `cmd/git-board-server` serves `dist/spa` (Quasar's
  default nested output — a first attempt flattened `distDir` to match a
  simpler `fs.Sub` target, but `quasar build` recreates its dist dir from
  scratch every run and took the tracked `web/dist/.gitkeep` placeholder
  with it; keeping Quasar's default means the build only ever touches
  `dist/spa`, leaving `.gitkeep` in the untouched parent). `web/dist/.gitkeep`
  is tracked (via a `.gitignore` carve-out) so `go build ./...` succeeds
  even before the frontend has ever been built — verified with
  `npm run build && git status` showing no changes to `web/dist/.gitkeep`.
- Genuinely MVP-missing, deliberately deferred: no auth (see above), no
  issue creation from the UI (the CLI itself has no `new` command either —
  git-bug's own tooling owns that), no bulk edit, no optimistic UI (each
  `PATCH` round-trips before the row updates), clearing a single-cardinality
  field to empty isn't wired up in the dialog (only picking one of the
  configured options is).

## Status update (2026-09-02): TUI editing, on `feat/tui-editing`

Follow-up on the "Dogfood feedback" section above. Re-diagnosed: the detail
view and "nothing to edit/see" complaints were mostly because the real
issues dogfooded against had **zero board-relevant labels yet** (a separate
bridge command — `import-github`, tracked above under "Not done" — is being
built elsewhere to populate those from GitHub Projects). Rather than wait for
that, this closes the three concrete TUI gaps against fixtures with
realistic populated labels, so the behavior is properly testable now:

- **Nicer detail view**: labels are parsed against `cfg.Fields` and rendered
  as `Field: value(s)` lines (multi-cardinality joined, e.g.
  `Assignee: alice, bob`), sorted by field name; anything not matching a
  configured field namespace is listed under "Other labels" instead of
  silently dropped. Pure logic in `ParseLabels` (`internal/tui/detail.go`),
  independent of bubbletea's `Update`/`View`.
- **Inline field editing**: `e` from the detail view opens a field picker
  (`bubbles/list`, config-driven, not a hardcoded field list) → a value
  picker over that field's configured `Options`, or a free-text
  `bubbles/textinput` for fields with none (only `assignee` in the current
  fixture — same `+value`/`-value` semantics `fields.Apply` already defines).
  The write goes through `fields.Apply` (`ApplyField` in `detail.go`
  wraps it and re-reads labels from the backend afterward, rather than
  re-deriving the single/multi-cardinality swap logic client-side) and shows
  a confirmation or error status line — a write failure doesn't crash the
  TUI, it's shown inline and the user can retry or back out (`esc`).
- **Quick category filter**: `c` from the list opens a picker over the
  configured `cat` field's options (plus a `(all)` entry to clear) and
  narrows the visible list via `FilterByField`; layers on top of
  `bubbles/list`'s own `/` text filter rather than replacing it.
- **Wiring**: `tui.Run`/`tui.New` now take `backend fields.Backend, cfg
  *config.Config, me string, today time.Time` in addition to the existing
  `issues []gitbugio.Issue, title string` — the `fields.Backend` interface
  (not the concrete `*gitbugio.Backend`) so tests can drive it through
  `gitbugiotest` fixtures like the rest of the repo. `cmd/git-board`'s
  `runTui` now opens the backend itself (previously `loadMatchingIssues`
  opened-and-closed it before `tui.Run` even started) and keeps it open for
  the life of the TUI session so edits can write through it.
- Testdata: `internal/config/testdata/git-board.toml` (the shared fixture
  already used by `fields`/`filter`/server tests) gained `[fields.cat]`
  (`bug`/`feature`/`chore`) so the category filter has something realistic
  to filter on — no other fixture change needed since `status`/`prio`/`est`/
  `assignee` already covered the single/multi-cardinality and options/
  free-text cases.
- Tests: known-vs-unrecognized label parsing, field/value picker
  construction from config (including the free-text fallback when a field
  has no `Options`), single-cardinality edit (old label gone, new one
  present, same assertion style as `fields_test.go`), multi-cardinality
  edit, category quick-filter narrowing — all via real `gitbugiotest`
  fixtures, no mocks. `go build ./...`, `go vet ./...`, `gofmt -l .`, `go
  test ./...` all clean.
- Not done here, still tracked above: `import-github` bridge itself (a
  parallel effort), numeric-aware sort, `lint`/`move`/`close`/`sprint`
  subcommands.
