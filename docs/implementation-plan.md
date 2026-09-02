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

## Status update (2026-09-02): `git-board bridge` (PRD §11 migration bridge)

`git-board bridge configure`/`bridge pull` bridges GitHub Project 40's
custom field values into the label model, for git-bug issues already
imported via `git-bug bridge pull` (that import step is out of scope here —
this only maps fields onto bugs that already exist locally). Shaped as a
`bridge` subcommand namespace mirroring git-bug's own `bridge` UX
(`configure` persists owner/project, `pull` does the sync), not a one-shot
migration script — Project 40's fields keep changing as work progresses, so
this is meant to run repeatedly.

- **Real shape verified**, not guessed: ran `gh project item-list 40
  --owner EPFL-ENAC --format json` against the actual project before
  writing the parser. Top-level item keys: `assignees`, `content`
  (`body`/`number`/`repository`/`title`/`type`/`url`), `id`, `labels`,
  `linked pull requests`, `priority`, `repository`, `sprint`
  (`duration`/`iterationId`/`startDate`/`title`), `status`, `module`,
  `category`, `est. hours` (JSON number, int or float), `title`.
  `content.type` was `"Issue"` on every row fetched live; `"PullRequest"`/
  `"DraftIssue"` are documented GitHub Project item content types, handled
  defensively (excluded, reported as unmatched) with a synthetic fixture
  row since none turned up live in the sample pulled. `milestone`/`size`
  never appeared with a value, consistent with the PRD calling them
  dead — not decoded at all, so any value present is silently ignored by
  omission (not mapped to a label).
- **Matching by GitHub URL, not title**: `git-bug`'s own GitHub bridge
  stamps the issue's URL under a `"github-url"` Create-operation metadata
  key on import (`bridge/github/github.go`'s unexported
  `metaKeyGithubUrl`, set via `Bugs().NewRaw`'s metadata map). Surfaced as
  `gitbugio.Issue.GithubURL`; `ghbridge.BuildPlan` looks bugs up by exact
  URL match against the project item's `content.url`. A bug not bridged in
  from GitHub carries no such metadata and simply never matches.
- **Module dedup**: gh's JSON only exposes an option's display name, not
  its id, so two *identically-named* duplicate options already collapse in
  the JSON with no code needed. To also cover a casing/whitespace variant
  (`"Generic"` / `" generic "` / `"GENERIC"`), `normalizeModule` folds any
  case-insensitive match of "generic" to the canonical `"Generic"` before
  slugifying to `mod:generic` — covered by a test with two differently-cased
  fixture rows.
- **Unmatched-value handling**: a non-empty field value that doesn't
  slugify to one of a configured `[fields.<name>].options` list is skipped
  with a warning (printed per-issue and rolled into the summary line), not
  silently dropped — matches the PRD's own acknowledgment of real drift
  (documented example: dozens of empty-status/empty-priority items). An
  *empty* source value is treated as "leave untouched", not a warning —
  expected, not an error.
  **Verified against the real 100-item sample pulled live from Project 40**
  (fields decoded through the actual `BuildPlan`, not a guess): 6 items
  warn on `status value "qa-validation" has no matching [fields.status]
  option` — the board's "QA Validation" status slugifies to
  `qa-validation`, but `.git-board.toml`'s `[fields.status].options` has
  `qa`, not `qa-validation`. 47 items warn on sprint titles
  ("Simulator Sprint 2", "Simulator Sprint 3 (QA & Fixes)") with no
  matching `[[sprints.list]]` entry — expected, since the shipped sample
  config only defines two placeholder sprints. 1 item warns on
  `est value "0.1"` (not in the sample `[fields.est].options` list).
  **Action for whoever runs the first real `--apply`**: reconcile
  `.git-board.toml`'s `[fields.status]`/`[fields.est]` options and
  `[[sprints.list]]` entries against the board's actual current values
  first, or these will warn-and-skip on every run rather than just once.
- **Idempotency**: `internal/fields.Diff` was extracted (pure, no I/O) out
  of the existing `fields.Apply` so both `set` and `ghbridge.BuildPlan`
  share the exact same single/multi-cardinality swap-diff logic — `Apply`
  itself is now a thin wrapper over `Diff` + the write, no behavior change
  (all existing `internal/fields` tests still pass unmodified). Assignees
  (multi) reconcile the full set (add *and* remove) each run so an
  unassign on GitHub is mirrored, not just accumulated. Verified with a
  real-repo (`gitbugiotest`) test that runs `BuildPlan`+`ApplyPlan` twice
  and asserts an identical label set the second time, plus a CLI-level
  `bridge pull --apply` run-twice test.
- **Tests**: `internal/ghbridge` — pure `BuildPlan` tests against a fixture
  JSON (`testdata/project-items.json`, based on the real captured shape
  plus a few documented synthetic rows for dedup/unmatched/PR-exclusion
  cases) and plain `gitbugio.Issue` literals, no real repo needed for
  those; `ApplyPlan` idempotency/assignee-reconciliation tests via
  `gitbugiotest` (real git-bug repo cache, no mocks). `cmd/git-board` —
  CLI-level tests for `bridge configure` (config file written, rest of the
  file untouched) and `bridge pull` (dry-run writes nothing, `--apply`
  writes and is idempotent on rerun, `--apply`+`--dry-run` rejected,
  missing owner/project rejected) via a `fetchGithubItems` seam that swaps
  in the fixture instead of shelling out to the real `gh` binary — no
  network or `gh` needed to test.
- `internal/config`: new `[github-project]` table (`owner`/`number`),
  written by `SetGithubProject` via targeted find-the-table-or-append text
  surgery rather than a full TOML decode+re-encode round trip (which would
  drop every comment and reorder the rest of the file) — every other table
  is left byte-for-byte alone. Round-trip-tested against the real sample
  `.git-board.toml` fixture, including a second-call-overwrites-the-first
  idempotency check.
- **Not done / manual**: the GitHub→git-bug issue import itself
  (`git-bug bridge pull`) is a separate, already-existing step, out of
  scope here. Not run against the real EPFL-ENAC/co2-calculator project
  with `--apply` — that first real run is for a human to trigger. The
  `[fields.*]`/`[[sprints.list]]` drift noted above should be reconciled
  before that first `--apply`. `--limit`'s default (2000) is untested at
  that size — `gh project item-list` hit a secondary GraphQL rate limit
  during this work at `--limit 900`/`1000` (shared across all agents/tools
  per its own error message, so may just have been contention, not a hard
  ceiling on 2000) — the first real run may need a smaller `--limit` and a
  retry if it hits that limit; no retry logic was added for it (kept
  simple, surfaces as a plain error to the human running it).
