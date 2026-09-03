# git-board

Fields, sprints and views over [git-bug](https://github.com/git-bug/git-bug)
issues — a lightweight, config-as-code replacement for GitHub Projects.

Field values live as namespaced git-bug labels (`status:ready`, `prio:p1`,
`sprint:delivery`, ...) instead of a separate Projects database, so they merge
like any other git-bug operation. Sprints and saved views are just a
`.git-board.toml` file, reviewed and diffed like the rest of your repo.

Field writes go through git-bug's `cache.RepoCache` API **in-process**
(`github.com/git-bug/git-bug` imported as a Go library), not the `git-bug`
CLI binary — so `git-board` needs the target repo to be a normal
git-bug-initialized repo, but does **not** need `git-bug` installed on PATH.

This is the **M1** milestone (fields, `set` / `get` / `ls` / `view`, the
filter grammar, and config loading) plus `lint`, `move`, `close`, `sync`,
`sprint`, numeric-aware `--sort`, and `html` (cron-published static site)
from **M2** — the last M2 item, so M2 is now fully shipped.

## Install

This repo is **private**, so Go's default module proxy/checksum database
can't fetch it — tell Go to go straight to GitHub instead. That alone isn't
enough: `go install` fetches `github.com/...` paths over **HTTPS** by
default, which fails non-interactively even if you have an SSH key on your
GitHub account, unless `git` is told to use it for that host:

```bash
git config --global url."git@github.com:".insteadOf "https://github.com/"
export GOPRIVATE=github.com/guilbep/*   # add to your shell profile to persist
go install github.com/guilbep/git-board/cmd/git-board@latest
```

(No SSH key set up? Use an HTTPS credential helper with a token that has
`repo` scope instead, and skip the `git config` line above.)

If you see `reading https://sum.golang.org/...: 404` — `GOPRIVATE` isn't
actually set in this shell (it doesn't persist across shells unless it's in
your profile). If you see `could not read Username ... terminal prompts
disabled` — the `git config` line above is missing or your SSH key isn't
registered with GitHub.

Requires Go 1.24+ (git-bug's own module requirement); `go install` fetches
the matching toolchain automatically if needed. Verify it worked:

```bash
git-board --help
```

### From a local clone (no GOPRIVATE needed)

```bash
git clone git@github.com:guilbep/git-board.git
cd git-board
go install ./cmd/git-board
```

## Display convention: known fields get their own column/line

Every surface that shows an issue's fields — `ls`/`view`'s table (and
`--md`), the TUI's detail view, and the `html` static site — follows the
same rule: a label matching a field configured in `.git-board.toml`'s
`[fields.*]` renders under that field's own column or line, with a
capitalized display name (`status:ready` → a `Status` column showing
`ready`), not as a raw `field:value` string. This is driven **dynamically**
by whatever a project's own config actually defines — not a hardcoded
field list — so it stays correct as fields are added, renamed, or
removed, and it's the same classification logic
(`internal/tui.ParseLabels`/`DisplayFieldName`) everywhere, not
reimplemented per surface.

A label that doesn't match any configured field still shows — under
"other" in tables, "Other labels" in the TUI/`html` — never silently
dropped. The one field-specific special case: a numeric field literally
named `est` gets an `h` (hours) suffix per value; this isn't a general
"numeric implies hours" rule, `est` is the only field this project has
ever needed it for.

## Config: `.git-board.toml`

```toml
[repos]
list = ["EPFL-ENAC/co2-calculator", "EPFL-ENAC/co2-calculator-back-office-doc"]

[fields.status]
single = true
options = ["backlog", "comite", "ready", "in-progress", "in-review", "ready-for-validation", "qa", "tests-metier", "validated"]

[fields.prio]
single = true
options = ["p0", "p1", "p2", "p3"]

[fields.est]
single = true
options = ["0.5", "1", "2", "3", "4", "6", "8", "16"]
numeric = true

[fields.assignee]
single = false

[sprints]
length_days = 14
start_weekday = "monday"

[[sprints.list]]
slug = "delivery"
title = "Delivery"
start = 2026-08-24

[[sprints.list]]
slug = "post-delivery"
title = "Post-delivery"
start = 2026-09-07

[views.current-sprint]
layout = "board"
group = "status"
filter = "sprint:@current is:open -title:*spec*"

[views.backlog]
layout = "table"
filter = "is:open"
group = "sprint"
sort = "prio asc"

[views.mine]
layout = "table"
filter = "assignee:@me is:open"
```

## Usage

Run from inside a git-bug-initialized repo (or a subdirectory of one), next
to a `.git-board.toml`.

```bash
# default view: open issues
git-board

# ad-hoc filter, grouped and sorted
git-board ls "is:open -title:*spec*" --group status --sort "prio asc"

# a named view from .git-board.toml, rendered as markdown or JSON
git-board view current-sprint --md
git-board view backlog --json

# set fields on an issue (single-cardinality fields swap; assignee is multi)
git-board set <issue-id> status=ready prio=p1 sprint=@next assignee=+guilbep

# show an issue's fields (labels)
git-board get <issue-id>

# advance status to the next configured option (backlog -> ready -> ...)
git-board move <issue-id> --next

# set status to the last configured option (the terminal state, whatever
# it's called in your config) and close the issue
git-board close <issue-id>

# report label/config drift, read-only (see "Linting" below)
git-board lint

# git bug pull/push on one or more git-bug repos (defaults to cwd)
git-board sync
git-board sync ../co2-calculator ../git-time

# sprints: list/resolve/create/report (see "Sprints" below)
git-board sprint ls
git-board sprint current
git-board sprint new q1-hardening --title "Q1 Hardening" --start 2026-10-05
git-board sprint report delivery
```

`@me` (in filters, e.g. the `mine` view, or as a field value) resolves from
`--me` on `ls`/`view`/`set`, falling back to the `GIT_BOARD_ME` env var. It
errors if neither is set.

## Syncing (`git-board sync`)

`git-board sync [path...] [--remote origin]` runs `git bug pull` then
`git bug push` on each path — via the library (`cache.RepoCache.Pull`/
`Push`), not a `git bug` subprocess, same as everywhere else in git-board.
Defaults to the current directory if no paths are given; keeps going past
a path that fails so one broken repo doesn't block the rest, then reports
every failure together at the end.

Takes explicit local paths, **not** `.git-board.toml`'s `[repos]` list —
that table holds `"owner/repo"` display names, not filesystem locations,
and there's no established convention yet for mapping one to the other.
Config-driven multi-repo sync is tracked as a follow-up in
`docs/implementation-plan.md`, not solved here.

## Bridging GitHub Projects into labels (`git-board bridge`)

`git-board bridge` maps a GitHub Project (v2)'s custom field values onto the
`ns:value` labels above, on git-bug issues already bridged in locally via
`git-bug bridge pull` (matched by GitHub issue URL, not title — a git-bug
bug carries the original issue URL in its Create-operation metadata when it
came from `git-bug`'s own GitHub bridge).

It is **read-only against GitHub**: it only runs `gh project item-list`
(needs `gh` authenticated with at least `read:project` scope) and never
writes back to the Project or to any GitHub issue. All writes land on the
local git-bug repo, via the same `ApplyLabelChange`/`ChangeLabels` call
`git-board set` uses.

This is meant to be run **repeatedly**, not once — a GitHub Project's field
values (status, priority, sprint, assignees, ...) keep changing as work
progresses, so re-running `bridge pull` is how the label model stays in
sync. Every run recomputes the diff against the issue's current labels, so
running it twice back-to-back is a no-op the second time.

```bash
# one-time: remember which GitHub Project to pull from
git-board bridge configure --owner EPFL-ENAC --project 40

# preview only — prints the labels that would be added/removed, and a
# matched/unmatched/warnings summary. This is the default; no flags needed.
git-board bridge pull

# actually write the labels
git-board bridge pull --apply

# override the configured project for one run
git-board bridge pull --owner EPFL-ENAC --project 40 --dry-run
```

Field mapping:

| GitHub Project field | Label | Cardinality |
|---|---|---|
| Status | `status:<value>` | single |
| Priority | `prio:<value>` | single |
| Category | `<value>` (bare label, verbatim — not slugified) | none — plain label, not a field:value pair |
| Module | `mod:<value>` | single — the two duplicate "Generic" options are merged into one `mod:generic` |
| Est. hours | `est:<value>` | single |
| Sprint | `sprint:<slug>` | single, matched against `[[sprints.list]]` titles |
| Assignees | `assignee:<user>` | multi — one label per assignee, added *and* removed to mirror GitHub exactly |

`Milestone` and `Size` are **not** imported — the PRD calls them dead/unused
fields.

Category isn't a structured field like Status/Priority — it's added as a
plain, unnamespaced label instead. Category and GitHub's own native issue
labels do the same job; a `cat:<value>` field would just be a second,
duplicate way to say the same thing. Being a plain label, it's additive
only (no configured options, no removal on re-run) — same tradeoff every
other plain label already has.

A field value that doesn't slugify to one of a configured `[fields.<name>]`
`options` list is **skipped with a warning**, not silently dropped or
guessed at — real Project data has drift (the PRD's own numbers: dozens of
open items with no status or priority set at all). An empty GitHub value
just leaves that label untouched, no warning; a non-empty value with no
matching option prints a warning line and leaves the label untouched too.
A project item that isn't a GitHub Issue (a PR added straight to the
Project) is excluded outright and reported as unmatched, same as an Issue
that hasn't been bridged into git-bug yet.

### Filter grammar

| Syntax | Meaning |
|---|---|
| `field:value` | issue has label `field:value` |
| `-field:value` | issue does **not** have that label |
| `field:@current` / `@next` / `@previous` | resolves against `[sprints]` |
| `field:@me` | resolves to the current user |
| `is:open` / `is:closed` | issue status |
| `title:*glob*` | glob match on title |
| free text | substring match on title |

Conditions are AND'd together.

## Linting (`git-board lint`)

`git-board lint` is **read-only** — it reports drift, writes nothing, unlike
everything else in this CLI. It checks every `is:open` issue in the current
git-bug repo against `.git-board.toml`:

- **double singles** — more than one `ns:*` label for a `[fields.<ns>]` entry
  with `single = true` (e.g. two `status:*` labels). Structurally rare going
  forward (`fields.Apply`'s swap logic prevents it), but concurrent git-bug
  operations from two people could still produce it.
- **unknown option values** — a `field:value` label where `value` isn't in
  that field's configured `options` (fields with no configured `options` are
  free-form and skipped). Same check `bridge pull` already warns on.
- **missing values** — an open issue with no value at all for a configured
  single-cardinality field (the PRD calls out `prio` specifically; every
  configured single field is checked, since real drift shows up on `status`
  too, not just priority).
- **orphan sprint labels** — a `sprint:<slug>` label with no matching
  `[[sprints.list]]` entry.
- **dead options** — an option value listed in `[fields.<name>].options`
  that no issue in the repo (open or closed) actually carries. This is the
  one check that isn't `is:open`-scoped: it's a config-inventory question,
  and an option only used on a closed issue (e.g. a terminal `validated`
  status) is still in use.

Not checked: "unknown assignees" — the PRD lists it, but there's no
configured roster of valid assignees in `.git-board.toml` to validate labels
against, unlike every other check above.

```bash
git-board lint             # exit 0 even with findings — a report, not a build gate
git-board lint --strict    # exit 1 if anything is found, e.g. in CI
```

## Sprints (`git-board sprint`)

Thin presentation over the date-window math already in `internal/config`
(`ResolveSprint`, used by `@current`/`@next`/`@previous` in the filter
grammar) — this command doesn't reimplement resolution, it exposes it.

```bash
git-board sprint ls                    # every configured sprint, with computed date ranges
git-board sprint current               # the sprint @current resolves to
git-board sprint next                  # same for @next
git-board sprint previous              # same for @previous
git-board sprint new <slug> --title <t> --start <date>   # appends a [[sprints.list]] entry
git-board sprint report <slug>         # issues in that sprint, aggregated by status
```

- **`ls`**'s end date is computed, not stored: a sprint's end is the next
  sprint's start, or `start + [sprints].length_days` for the last one.
  `current`/`next`/`previous` print the same computed range for the sprint
  they resolve to.
- **No sprint resolves** (before the first configured sprint has started, or
  nothing configured at all) is a normal outcome, not an error: `current`
  prints `no sprint currently active` and exits 0, same idea for
  `next`/`previous`. Matches `ResolveSprint`'s own `(nil, nil)` return for
  "nothing resolves" — it's not a `%!s(<nil>)` crash waiting to happen.
- **`new`** appends `[[sprints.list]]` via the same targeted text-surgery
  `git-board bridge configure` uses to write `[github-project]` — find the
  right spot in the raw file and splice text in, rather than decode the
  whole TOML document and re-encode it (which would silently drop comments
  and reorder tables). Refuses a slug that's already used, or a `--start`
  that doesn't parse as `YYYY-MM-DD`, without touching the file.
- **`report <slug>`** loads `sprint:<slug>` (open and closed issues both —
  a sprint retro wants to see what shipped, not just what's still open),
  groups by `status`, and prints a count per bucket plus a total. Same
  `internal/board.BucketBy` aggregation the `--group` flag on `ls`/`view`
  already uses. Errors on an unconfigured slug rather than silently
  printing `total: 0` for a typo.

### TUI

```bash
git-board tui                  # default filter (is:open)
git-board tui "is:open prio:p1"
```

| Key | Where | Does |
|---|---|---|
| `↑`/`↓`, `/` | list | native `bubbles/list` scroll and fuzzy text search (title + labels) |
| `enter` | list | open the detail view for the selected issue |
| `c` | list | quick-filter by the configured `cat` field's options (`(all)` clears it); coexists with `/` search. The shipped example config no longer defines `[fields.cat]` (category is a plain label now — see the bridge field mapping below), so `c` shows "no cat field configured" unless a project opts back into one. |
| `f` | list | structured filter — the same `field:value`/`-field:value`/`is:open`/`title:*glob*` grammar as `ls`/`view`/the launch argument, not the fuzzy `/` search. Empty input + enter clears it. |
| `s` | list | sort by field, e.g. `prio` (ascending) or `prio desc` — same syntax as `ls --sort`. Persists across a later `f`/`c` filter change. Empty input + enter clears it back to natural order. |
| `i` | list | preview the GitHub Project bridge (`git-board bridge pull`, dry-run) — matches git-bug's own termui, where `i` is "Pull". Requires `git-board bridge configure` to have been run first. Runs in the background; the TUI stays responsive while it fetches. |
| `o` | list | apply the plan `i` just previewed (`bridge pull --apply`) — matches git-bug's termui `o`, "Push". Refuses to run before an `i` (no blind apply); the plan is cleared after applying so `o` can't silently re-apply stale data. |
| `e` | detail | edit a field: pick the field, then a value from its configured options, or type free text if it has none (e.g. `assignee`, using the same `+value`/`-value` syntax as `git-board set`) |
| `g` | detail | view the issue's body and comments — shells out to `git bug bug show <id>` (requires `git-bug` installed as a git subcommand; git-board itself never needs it), suspending the TUI and resuming when it exits |
| `esc`/`q` | detail, pickers | back |
| `q`/`ctrl+c` | list | quit |

`f` and `/` are two different tools: `/` is a quick fuzzy text match, useful
for "I remember a word in the title." `f` is precise and composable —
`assignee:guilbep -status:validated` narrows exactly like it would on the
CLI, because it's the exact same `internal/filter` grammar and code path.

`i`/`o` show a one-line summary (matched/changed/unmatched/field/warning
counts), not the full per-issue diff — that's what `git-board bridge pull`
on the CLI is still for. Note these are **not** git-bug's own `i`/`o`
(which sync git-bug's issue refs with a remote) — same keys, same
pull/push mnemonic, but bound to git-board's GitHub Project bridge
instead, since that's the sync a git-board session actually needs.

git-board's own data model is fields (labels) only — it doesn't store or
render issue bodies/comments, and doesn't need `git-bug` on PATH for
anything else (writes go through the library, not the CLI). `g` is the one
place that changes: it's a deliberate, narrow exception to reuse git-bug's
own mature viewer instead of building a second one.

The detail view renders each label under its configured field name (e.g.
`Status: ready`, `Assignee: alice, bob`); anything not matching a configured
field namespace is listed under "Other labels" rather than dropped. Field
edits write through the same `fields.Apply` primitive as `git-board set` and
the web UI's `PATCH`, so the write is real and immediate — a failure (e.g. a
repo/network issue) is shown inline and the TUI stays open.

## Web UI

A one-page board, backed by a Go HTTP API in the same module — no Python,
no separate service. `cmd/git-board-server` reuses `internal/config`,
`internal/gitbugio`, `internal/fields` and `internal/board` directly, so
`ls`/`view`/`set` and the API apply the exact same filter/sort/group and
field-write logic.

**No auth in v1** — same trusted-internal-network assumption as git-bug's
own `git bug webui`. Anyone who can reach the port can read and write every
issue. Upgrade path when that stops being fine: a reverse proxy doing auth
in front of it (simplest), or a token check in `withCORS`/a new middleware
in `cmd/git-board-server` (more work, still small).

```bash
# backend — run from inside the git-bug repo, next to .git-board.toml
go run ./cmd/git-board-server              # listens on :8080
go run ./cmd/git-board-server --addr :9090 --me guilbep

# frontend, dev mode (separate terminal) — talks to the backend on :8080
# via CORS (see cmd/git-board-server/cors.go); Quasar's dev server is :9000
cd web && npm install && npm run dev

# production: build the frontend into web/dist, then build one binary that
# embeds it (web/embed.go) — no separate frontend deploy/CDN needed
cd web && npm install && npm run build
cd .. && go build -o git-board-server ./cmd/git-board-server
./git-board-server   # serves the API and the built UI on the same origin
```

## Static site (`git-board html`)

Generates a static HTML site, one page per configured `[views.*]`, plus an
`index.html` linking to all of them — for PM/non-dev users who don't want a
CLI and shouldn't need the (also-unauthenticated) live web UI above. Reuses
`internal/board`'s `LoadMatching`/`SortByField`/`BucketBy` — the exact same
filter/sort/group semantics as `ls`/`view`/the web API — and the TUI's
`tui.ParseLabels`/`tui.DisplayFieldName` to render known fields under their
display name (`Status: ready`) instead of raw `status:ready` label text.
**Read-only** — writes nothing to git-bug, unlike everything else in this
CLI except `ls`/`view`/`get`/`lint`.

`layout = "table"` vs `"board"` in a view's config picks the page shape: a
`"board"` view renders one column per bucket (kanban-style, cards); anything
else renders a flat table per bucket. This is a new distinction — `layout`
was parsed by `internal/config` and shipped to the web UI's API from the
start, but nothing (not the CLI's own `render.go`, not the web frontend)
actually branched on it before this command; the Vue board always rendered
a stacked table regardless of the configured value. `git-board html` is the
first thing to read it.

```bash
# run from inside the git-bug repo, next to .git-board.toml
git-board html                          # writes site/*.html
git-board html -o /var/www/board        # custom output dir

# intended usage: cron-publish on a timer, e.g. a crontab entry running
git-board html -o /var/www/board && rsync -a --delete /var/www/board/ host:/srv/board/
```

`-o`'s directory is regenerated in full on every run but never emptied first
— an old page whose `[views.*]` entry gets renamed or removed stays on disk,
just no longer linked from `index.html`. `rsync --delete` (above) is enough
to keep the *published* copy in sync; if `-o`'s own local directory needs to
mirror the config exactly between runs, `rm -rf` it first.

**No auth, no JS required to view** — plain server-rendered HTML, same
trusted-network stance as `git-board-server` above (see its "No auth in v1"
note): anyone who can reach the published files can read every issue in
every configured view. A view whose filter uses `@me` resolves once at
generation time against `--me`/`GIT_BOARD_ME`, the same single shared
"current user" the web API uses — not per-viewer, there's no login here.

## Layout

```
internal/config/       TOML parsing, sprint resolution — pure, no I/O beyond reading the file.
internal/gitbugio/     thin wrapper around git-bug's cache.RepoCache API.
internal/fields/       ApplyField single/multi-cardinality logic and alias resolution.
internal/filter/       grammar parser, Matches(issue, conditions, config, me).
internal/board/        load/sort/group issues — shared by the CLI, the TUI and the web API.
internal/ghbridge/     `gh project item-list` parsing + field-mapping/diff logic for `bridge pull`.
internal/lint/          read-only drift checks (labels vs. .git-board.toml) — internal/lint.Run.
cmd/git-board/          main.go + cobra commands, thin rendering (incl. html.go, the static-site generator).
cmd/git-board-server/   HTTP API (net/http, no framework) + embeds web/dist for prod.
web/                    Quasar/Vue frontend — one board page, api/board.ts is the only HTTP client.
```

Split this way so the TUI and the web frontend both reuse `internal/*`
without touching the CLI.

## Development

```bash
git clone git@github.com:guilbep/git-board.git
cd git-board
go build ./...      # compile everything
go vet ./...         # static checks
go test ./...        # full suite
gofmt -l .            # should print nothing; if it does, gofmt -w .
```

Run one package or one test by name:

```bash
go test ./internal/filter/...
go test ./internal/fields/... -run TestApplySingleCardinalitySwapsLabel -v
```

Tests exercise real git-bug repo caches in `t.TempDir()` (via git-bug's own
`repository.CreateGoGitTestRepo` test helper) rather than a hand-rolled fake
— it's fast (single-digit milliseconds per test) and the most honest test of
the real integration. **No `git-bug` binary or network access is needed to
test** — everything runs against on-disk repo fixtures.

## Lefthook automation examples

Two [Lefthook](https://github.com/evilmartians/lefthook) hooks a project
adopting git-board can add to its own `lefthook.yml` — these are examples
to copy in, not something `git-board` installs itself. Both verified
against a real repo before documenting, not just written from the PRD
sketch. Need `jq` on PATH.

```yaml
# Advance status to ready-for-validation when a commit whose subject
# references an issue lands (e.g. a squash-merged PR). GitHub's own
# squash-merge appends the *PR* number as a second "(#NNNN)" — take the
# *first* match, that's the referenced issue, not the merging PR.
post-merge:
  commands:
    board-status-on-merge:
      run: |
        NUM=$(git log -1 --format=%s | grep -oE '\(#[0-9]+\)' | head -1 | tr -d '()#')
        [ -z "$NUM" ] && exit 0
        ID=$(git-board ls "is:open" --json | jq -r --arg n "$NUM" '.[] | select(.github_url | endswith("/" + $n)) | .id')
        [ -z "$ID" ] && exit 0
        git-board set "$ID" status=ready-for-validation

# Non-blocking warning (always exits 0) if the current branch's issue
# (from a name like issue/123-slug or fix/123-slug) isn't in the
# current sprint.
commit-msg:
  commands:
    board-sprint-check:
      run: |
        NUM=$(git branch --show-current | sed -n 's|.*/\([0-9][0-9]*\).*|\1|p')
        [ -z "$NUM" ] && exit 0
        CURRENT=$(git-board sprint current 2>/dev/null | cut -d: -f1)
        [ -z "$CURRENT" ] && exit 0
        ID=$(git-board ls "is:open" --json | jq -r --arg n "$NUM" '.[] | select(.github_url | endswith("/" + $n)) | .id')
        [ -z "$ID" ] && exit 0
        git-board get "$ID" | grep -q "sprint:$CURRENT" || echo "⚠️  issue #$NUM is not in the current sprint ($CURRENT)"
        exit 0
```

## Scope

Shipped: config loading, the in-process git-bug cache adapter, the filter
grammar (with numeric-aware `--sort`), `set`/`get`/`ls`/`view`, an
interactive TUI (`git-board tui`), the web UI above, `git-board bridge
configure`/`bridge pull` (GitHub Project → label bridge, plus `i`/`o`
from inside the TUI), `git-board lint` (read-only drift report),
`git-board move`/`close`, `git-board sync` (explicit local paths,
not `[repos].list`), `git-board sprint` (`ls`/`current`/`next`/
`previous`/`new`/`report`), and `git-board html` (cron-published static
site) — the last planned M2 item, so M2 is now fully shipped. See
`docs/implementation-plan.md` for what's tracked next.
