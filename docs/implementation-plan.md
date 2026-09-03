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
  fixture rows. **Unverified**: `gh project field-list 40 --owner
  EPFL-ENAC --format json` — the one call that would show the two
  duplicate "Generic" options' actual display names and settle whether
  they really are a casing/whitespace variant (handled) versus something
  like `"Generic"` vs `"Generic (misc)"` (not handled by this regex) — hit
  the GitHub Projects secondary rate limit on every attempt during this
  work (shared across all agents/tools, not just this one) and was never
  retrieved. Re-run `field-list` before relying on this normalization for
  the real `--apply`.
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

## Status update (2026-09-02): first real `bridge pull --apply`, a TUI crash fix, structured filter in the TUI, `--help` examples

- **Ran for real against EPFL-ENAC/co2-calculator project 40** (`--apply`,
  not the earlier dry-run-only state): 291 matched (284 changed), 563
  unmatched (16 not-an-issue, 547 not yet bridged into git-bug), 840
  fields set, 261 value warnings — all as predicted by the dry-run
  numbers above. Confirmed real field data (`status`, `prio`, `est`,
  `assignee`, `mod`) now renders correctly via `git-board ls`/`get`. Local
  only — not pushed to `origin` (that's a deliberate, separate decision
  for whoever owns the target repo).
- **Fixed a real crash**: `git-board tui` panicked on every launch
  (nil-pointer in `bubbles/list`'s `updatePagination`). Root cause:
  `valuePicker`/`catPicker` were left as the zero-value `list.Model`
  until the user opened them, but `Update`'s `WindowSizeMsg` handler
  calls `SetSize` on all four pickers unconditionally — and bubbletea
  sends that message immediately on startup. Fix: construct them as real
  (empty) `list.Model`s in `New`, same as `list`/`fieldPicker` already
  were. Regression test reproduces the exact panic against the
  pre-fix code (verified by reverting the fix and re-running) and passes
  with it.
- **`f`: structured filter inside the TUI**, distinct from `bubbles/list`'s
  native `/` fuzzy search. Reuses `internal/filter.Parse`/`Matches`
  directly — the exact same grammar and code path `ls`/`view`/the launch
  argument already use, not a reimplementation. Opens empty (not
  pre-filled with the active filter — an earlier draft did that, but then
  "press f, enter" without editing just silently re-applied the same
  filter instead of the expected "empty submit = clear," so it was
  simplified); the currently-active filter is shown in the input view and
  the list title instead. `@me` with no `--me`/`GIT_BOARD_ME` configured
  surfaces as a status line (mirrors `runLs`'s CLI-side check), doesn't
  crash. Tests: narrows correctly, empty clears back to the full set and
  original title, `esc` cancels without applying, the `@me`-without-`--me`
  path. `f` and `c` (category quick-filter) both always re-derive from
  `allIssues` — last one applied wins, they don't compose with each
  other, only with `/`.
- **`--help` overhaul**: every command had only a one-line `Short`
  description before this — no `Long`, no `Example`. Root `--help` now
  documents the filter grammar inline; every subcommand (`ls`, `view`,
  `set`, `tui`, `get`, `bridge configure`/`pull`) has realistic examples,
  including the compound-filter pattern
  (`assignee:guilbep -status:validated`) that prompted this — it already
  worked, it just wasn't discoverable without reading the README.
- **Not done here**: switching to git-bug's own `termui`/`bug show` (or
  an inline body/comment preview) from the TUI detail view; sorting in
  the TUI. Both real asks, not built yet.

## Status update (2026-09-02): `s` sort in the TUI

- **`s`: sort by field**, same `"field"`/`"field desc"` spec syntax
  `internal/board.SortByField` (and the CLI's `--sort`) already use — no
  new sort implementation, just wiring. Opens empty, same convention as
  `f`; the active spec shows in the input view.
- **Persists across a later `f`/`c` filter**: added `Model.baseIssues`
  (the currently-narrowed set *before* sorting, distinct from
  `m.allIssues` the unfiltered set and from whatever's currently in
  `m.list`), which `applyCatFilter`/`applyStructuredFilter` update and
  `applySort` reads. Everywhere the visible list gets rebuilt routes
  through a `sortedItems` helper that applies `m.sortSpec` (a no-op on
  `""`) to a copy of the base set.
- **Caught before shipping**: a first draft had `applySort("")` (the
  "clear" path) re-sort whatever was *currently visible* with a no-op
  spec — which doesn't restore natural order at all, it just leaves the
  already-sorted order in place (a no-op over already-sorted data is
  still that sorted data). Same "clear that doesn't actually clear" bug
  class as `applyStructuredFilter`'s design went through earlier. Fixed
  by having `applySort` always read from `baseIssues`, not the visible
  list. A test asserts the true natural order (captured via a fresh
  `ListIssues()` before any sort) comes back on clear, not just "some
  order."

## Status update (2026-09-02): web UI smoke-tested against real data, `g` for body/comments via git-bug

- **First real smoke test of the web UI** (previously only ever tested
  against synthetic fixtures/`httptest`). Against the actual bridged
  co2-calculator data: `GET /api/issues` returns correctly bucketed real
  field data (status/prio/assignee/est), `GET /` serves the embedded UI
  and its referenced JS/CSS assets resolve, a real `PATCH` (setting
  `prio:p3` on a live issue) round-tripped correctly — verified via the
  PATCH response and independently via `git-board get` after stopping
  the server. No bugs found this time.
- **Real operational constraint, not a bug**: `git-board-server` and any
  `git-board` CLI/TUI invocation can't run concurrently against the same
  repo — git-bug's own cache holds a repo-level lock. Confirmed directly
  (`git-board get` failed with "already locked by process pid ..." while
  the server was still up, succeeded once it exited). Not a git-board
  defect to fix — it's git-bug's own concurrency-safety mechanism
  working as intended — but worth knowing before running the server
  long-lived on a box where you also want to use the CLI/TUI.
- **`g`: view an issue's body/comments via git-bug**, from the TUI's
  detail view. Shells out to `git bug bug show <id>` via
  `tea.ExecProcess` (suspends git-board's TUI, hands the terminal to
  git-bug, resumes on exit) rather than reimplementing a comment/body
  viewer — git-board's own data model is fields/labels only, it never
  stores or reads issue content. This is the one place git-board expects
  `git-bug` installed as a git subcommand; everything else (including
  `bridge`) only needs the Go library. A failed exec (e.g. `git-bug` not
  installed as a subcommand) surfaces as a status line, doesn't crash.
  Verified the underlying command directly against real data (`git bug
  bug show cb7ca37e` — real title/body/comments/actors came back)
  since `tea.ExecProcess` itself isn't practically unit-testable
  (needs a live terminal).

## Status update (2026-09-03): `lint` (first M2 item), on `feat/lint`

- **`internal/lint`** — pure package, `lint.Run(issues []gitbugio.Issue, cfg
  *config.Config) []Finding`, plus a thin `cmd/git-board/lint.go` cobra
  wrapper, matching how `internal/board`/`internal/fields`/`internal/filter`
  are each shaped. Read-only end to end: no new write paths anywhere.
- **All 5 in-scope checks shipped as scoped**, generalizing check 3 as the
  plan flagged as worth considering: instead of hardcoding just `prio`, it
  walks every configured `single = true` field and reports any with zero
  matching labels on an open issue. Same loop already needed to count
  labels for the double-singles check, so it was less code to do both at
  once, not more — and it matches the PRD's own drift numbers, which show
  empty `status` alongside empty `prio`, not just one field.
- **Unknown-assignees skipped, as the issue said to**: no configured roster
  of valid assignees exists in `.git-board.toml` to validate an
  `assignee:*` label against (every other check validates against config
  that exists), so there's nothing concrete to check. Noted in the README
  rather than guessed at.
- **Reused, not duplicated, the unknown-value check**: pulled
  `internal/ghbridge`'s existing `value ∈ options` logic out into
  `config.Field.KnownValue`, used by both `ghbridge.planSingleValue` (a
  `bridge pull` warning) and `lint.checkUnknownValues` — one predicate,
  two call sites, `ghbridge`'s existing tests confirm the extraction didn't
  change its behavior.
- **Open/closed scope needed a second look mid-build**: the task's intro
  frames every check as `is:open`-scoped, but check 5 (dead options) reads
  "no issue in the *repo*" — literally, including closed ones. Scoping dead
  options to `is:open` only would make any terminal-state option (e.g.
  `status:validated`, which by definition only ever lands on closed issues)
  report as dead on every real repo — a confidently wrong finding from a
  tool whose entire job is not being confidently wrong. Resolved it as: `Run`
  takes the full issue list, splits to open-only for checks 1-4 (double
  singles, unknown values, missing values, orphan sprints — all genuinely
  per-issue, actionable questions a closed issue doesn't need), and check 5
  runs over the whole list, since it's a config-inventory question ("is this
  option used anywhere"), not a per-issue one. Regression-tested directly: a
  closed issue carrying `status:validated` must not report `status:validated`
  as dead.
- **`--strict` flag, opt-in**: default exit code is 0 even with findings —
  matches the task's "report, not a build gate" framing. `--strict` exits 1
  if any finding is reported, for a CI use case, without changing the
  default.
- **CLI output**: findings grouped by check (double singles first, dead
  options last — rarer/higher-signal checks aren't buried under a
  potentially large missing-value count), each line `<short-id> <title>:
  <message>` (dead options have no issue, just the message), then a summary
  line with a per-check breakdown, e.g. `summary: 4 finding(s) (double-single:
  1, missing-value: 3)`.
- **Tests**: `internal/lint/lint_test.go` — real git-bug repo fixtures via
  `gitbugiotest`, one test per check with a tripping + a clean fixture
  issue, the open/closed dead-option regression case above, plus a
  clean-repo-reports-nothing case. That last one needed its own minimal
  one-option-per-field config rather than the shared `config/testdata`
  fixture — with `config/testdata`'s much larger option lists, no single
  fixture issue can exercise every option, so dead-option findings are
  unavoidable noise against it; asserting zero findings needed a config
  where "every option is used" is actually achievable. `cmd/git-board/lint_test.go`
  adds a thin CLI-wiring smoke test plus a `--strict` exit-code test.
- **Verification**: `go build ./...`, `go vet ./...`, `gofmt -l .` clean,
  `go test ./...` green (all packages, `internal/lint` and
  `cmd/git-board` included).

## M2 plan (2026-09-03)

Ordered by value/dependency, not strictly PRD section order. Each item
gets its own commit(s); check off here as they land, don't let this go
stale (grep `docs/implementation-plans` convention from co2-calculator
doesn't apply here, but "the plan is the source of truth for what's
next" does).

- [x] **`lint`** — double singles (two `status:*` on one issue), unknown
  option values, open issues with no `prio`, orphan sprint labels (no
  matching `[[sprints.list]]`), unknown assignees, dead options in
  config nothing uses. Read-only, no writes. Highest priority: directly
  serves the real drift already found bridging co2-calculator (6×
  unmatched status, 47× unmatched sprint, 315×/41× empty priority/status
  per the PRD's own numbers) — right now there's no tool that surfaces
  that except manually reading `bridge pull`'s warning output.
- [ ] **`move`/`close`** — `move <id> --next` advances `status` to the
  next option in `[fields.status].options`' configured order; `close`
  sets `status:validated` (or whatever the config calls the terminal
  state — don't hardcode "validated") then closes the git-bug bug via
  `internal/gitbugio`. Small, builds directly on `fields.Apply`.
- [ ] **`sprint ls|current|new|report`** — `ls` lists configured
  sprints with computed date ranges; `current`/`next`/`previous` show
  the resolved one; `new <slug> --start <date>` appends a
  `[[sprints.list]]` entry (mirror `config.SetGithubProject`'s
  targeted-text-surgery approach — don't decode+re-encode the whole
  TOML); `report <slug>` aggregates issues in that sprint by
  status/assignee (this is also most of what git-time's `est` will
  need later — shared sprint-math surface, don't duplicate it there).
- [ ] **`html`** — static site, one page per configured view, from
  `html/template` (stdlib — this doesn't need `jinja2`-equivalent
  templating power for a handful of table/board layouts). Meant to be
  cron-published for PM/non-dev read access. No auth, matches the web
  UI's own v1 stance.
- [ ] **`sync`** — `git bug pull`/`push` across every repo in
  `[repos].list`, wrapping `internal/gitbugio`'s existing per-repo
  backend. Multi-repo is the main new thing here — everything else
  already assumes one repo.
- [ ] **Numeric-aware `--sort`** — `internal/board.SortByField` sorts
  lexically (marked `ponytail:` — "16" < "2"). Needs a numeric-field
  lookup from `[fields.<name>].numeric` in config. Small, low priority
  since only `est` is numeric and nothing ships a shipped view sorting
  by it yet.

Not in this pass, decide later: `parent:`/sub-issues (PRD leaves this
an open M2 decision, not a commitment), the git-bug version-pin
recheck (`v0.10.1`, still fine as of this writing).
