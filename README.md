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

git-board builds against the `bridge-sync` fork of git-bug
([guilbep/git-bug](https://github.com/guilbep/git-bug/tree/bridge-sync)),
through a `replace` in `go.mod`: the fork adds two-way sync with GitHub,
GitLab and Gitea (`git bug bridge sync`). Go refuses `go install ...@latest`
for a module with a `replace`, so install from a clone:

```bash
git clone git@github.com:guilbep/git-board.git
cd git-board
git checkout v0.5.0                 # or stay on main
(cd web && npm ci)                  # the web app's dependencies, once
make web-build install              # git-board, the web app inside
git-board --help
```

`make install` alone builds `git-board` without the web app (its pages
are embedded at build time from `web/dist`): its web UI is then an empty
page.

Requires Go 1.26+ (git-bug's module requirement) and Node 22.12+ for the web
app; `go install` fetches the matching Go toolchain automatically if
needed. `git` must be on the path (branches' "in dev, squashed" asks
`git merge-tree`). Install the fork's `git-bug`
too (see its `INSTALLATION.md`, "The bridge-sync fork"), so that both read
and write the repository the same way.

## Display convention: every `ns:value` label is a field

Every surface that shows an issue's fields — `ls`/`view`'s table (and
`--md`), the TUI, the `html` static site and the web UI — follows the same
rule: a `ns:value` label renders under its own column or line, with a
capitalized display name (`status:ready` → a `Status` column showing
`ready`), not as a raw `field:value` string. The fields are the configured
ones **plus every namespace found on the issues**: a new `sprint:` or
`mod:` label gets its column, its filter and its editable field without
touching `.git-board.toml`. The same classification
(`internal/tui.ParseLabels`/`FieldNames`, `web/src/facets.ts`) is used
everywhere, not reimplemented per surface.

A label without a namespace still shows — under "other" in tables, "Other
labels" elsewhere — never silently dropped. The one field-specific special
case: a numeric field literally named `est` gets an `h` (hours) suffix per
value; this isn't a general "numeric implies hours" rule, `est` is the
only field this project has ever needed it for.

## Config: `.git-board.toml`

git-board tracks its own work with git-board: its `.git-board.toml` is at
the root of this repository, and its issues are git-bug data here
(`git bug pull origin` fetches them, then run `git board`).

Optional: without it, every `ns:value` label is a field and `git board`
works on any git-bug repository. It adds what labels can't say: the order
of a field's values (`options`: status follows the workflow when sorted
and on a board), single or multiple values, sprint dates (`@current`),
saved views, and the GitHub Project to sync with.

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

Every key, with what it does and its default — fields, colors, views,
filters, sprints, the GitHub Project, attachments: **[docs/configuration.md](docs/configuration.md)**.
The settings page shows the same reference next to its editor, at the
section of the table under the cursor.

## Usage

Run from inside a git-bug-initialized repo (or a subdirectory of one), next
to a `.git-board.toml`.

```bash
# this repository's web UI, in the browser (see "Web UI" below)
git board

# open issues
git-board ls

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

# the branch to create for an issue, and the command creating it
# ([branches] in .git-board.toml); it creates nothing
git-board branch <issue-id>

# report label/config drift, read-only (see "Linting" below)
git-board lint

# git bug pull/push on one or more git-bug repos (defaults to cwd)
git-board sync
git-board sync ../co2-calculator ../git-time

# the project's own numbers, CO2-3075 (docs/configuration.md, Numbers)
git-board numbers init --prefix CO2 --start 3052
git-board numbers

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
`git bug bridge pull` or `git bug bridge sync` (matched by GitHub issue
URL, not title — a git-bug bug carries the original issue URL in its
Create-operation metadata when it came from `git-bug`'s own GitHub bridge).

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
| Category | the issue's kind: the `[branches] kinds` key it names, without case (`FEAT` → `feat`) | single — only when the issue has no kind yet |
| Module | `mod:<value>` | single — the two duplicate "Generic" options are merged into one `mod:generic` |
| Est. hours | `est:<value>` | single |
| Sprint | `sprint:<slug>` | single — the `[[sprints.list]]` slug of the iteration's title, else the title as a slug (`Hypercare` → `sprint:hypercare`); list the iterations for `@current` to find them |
| Assignees | `assignee:<user>` | multi — one label per assignee, added *and* removed to mirror GitHub exactly (`bridge sync` merges both ways instead) |

`Milestone` and `Size` are **not** imported — the PRD calls them dead/unused
fields.

Category says the same thing as the kind labels (`bug`, `feat`, `perf`…),
and the labels win: Category only fills a kind the issue doesn't have,
written as `[branches] kinds` spells it, so `FEAT` lands as `feat`. A
Category that is none of the kinds (`FIX` when the kind is `bug`) warns
and writes nothing. A kind label is never removed: it's the team's to
change. What goes is the bare label an earlier bridge wrote, Category as
is (`FEAT`, `SPECS`): a sync would send it to GitHub, which applies `feat`
(beside a `bug`, if there is one) or creates `SPECS`.

`bridge pull` reads values exactly as `bridge sync` does: a
`[fields.<name>] project = { … }` alias first, then the slug of the
Project's name; a numeric field takes any number (its `options` are the
usual picks, not a limit). A value that doesn't slugify to one of a
configured `[fields.<name>]` `options` list is **skipped with a warning**, not silently dropped or
guessed at — real Project data has drift (the PRD's own numbers: dozens of
open items with no status or priority set at all). An empty GitHub value
just leaves that label untouched, no warning; a non-empty value with no
matching option prints a warning line and leaves the label untouched too.
A project item that isn't a GitHub Issue (a PR added straight to the
Project) is excluded outright and reported as unmatched, same as an Issue
that hasn't been bridged into git-bug yet.

### Syncing both ways (`git-board bridge sync`)

`bridge pull` only reads the Project. `bridge sync` keeps status, prio,
mod, est and sprint in step both ways, the git way: each field is compared
with the value both sides last agreed on, which git-board records on the
issue (in git-bug's history, so it travels with the issue).

- Only the Project changed: the label follows.
- Only git-board changed: the Project follows.
- Both changed differently: a **conflict**, left alone on both sides until
  you resolve it, `--ours` writing git-board's value to the Project,
  `--theirs` setting the Project's on the label.
- The first sync of an issue takes the Project's values. A field the
  Project has empty while git-board has a value is a conflict: a field
  never set can't be told from one a teammate cleared.
- A value the other side has no option for leaves the field alone, with a
  warning.
- Assignees merge person by person, never as a conflict: someone assigned
  or unassigned on GitHub since the last sync is on the labels, and with
  `write = true` someone added or removed in git-board is on the GitHub
  issue (`gh issue edit`). GitHub silently drops an assignee without access
  to the repository, so the sync first reads who can be assigned (one REST
  call per repository): anyone else stays in git-board only, with a
  warning. Without `write`, assignees only come from GitHub.

```bash
git-board bridge sync                         # what would change, on each side
git-board bridge sync --apply
git-board bridge resolve 2605 status --theirs # or --ours
```

Writing to the Project is off unless the config says so, and needs a `gh`
token with the `project` scope (`gh auth refresh -s project`):

```toml
[github-project]
owner = "guilbep"
number = 1
write = true

[fields.status]
single = true
options = ["backlog", "comite", "ready", "qa"]
# the Project's name for a value, where its slug isn't the value
project = { comite = "Comité Spécialisé", qa = "QA Validation" }
```

### Filter grammar

Views, the `ls`/`tui` filter argument, the web UI's Filter box and ⌘K
share one syntax:
`status:ready -prio:p3 assignee:@me is:open "car sharing"`. Every
qualifier: [Filters in docs/configuration.md](docs/configuration.md#filters).

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
- **kinds**, with `[branches] kinds` (its keys are the project's kinds, as
  plain labels): **two-kinds**, an issue with two (`bug` and `feat`);
  **kind-title**, a title whose `[BUG]` names another kind than its label
  (the label wins); **no-kind**, how many open issues have none, as one
  line, since a missing kind is usually an issue template's to fix (see
  `--create-labels` below).

Not checked: "unknown assignees" — the PRD lists it, but there's no
configured roster of valid assignees in `.git-board.toml` to validate labels
against, unlike every other check above.

```bash
git-board lint             # exit 0 even with findings — a report, not a build gate
git-board lint --strict    # exit 1 if anything is found, e.g. in CI
```

### Creating GitHub labels (`git-board lint --create-labels`)

The labels git-board writes (`status:ready`, `prio:p0`, `sprint:delivery`,
…) live inside git-bug's own label store, not GitHub's — so they don't
exist as real, colorable GitHub labels until someone creates them.
`--create-labels` closes that gap: it diffs every `ns:value` label
`.git-board.toml`'s `[fields.*]` options and `[[sprints.list]]` entries
imply against each configured `[repos]` entry's real GitHub labels (via
`gh label list`), and reports what's missing. Preview only by default,
same convention as `bridge pull`; `--apply` actually runs `gh label
create` for each missing one.

```bash
git-board lint --create-labels           # preview what's missing, writes nothing
git-board lint --create-labels --apply   # create the missing labels on GitHub
```

Free-form fields (no `options` configured — `assignee`, `est`) have no
fixed value set, so nothing to create labels for; Category isn't a
configured field at all anymore (see "Bridging GitHub Projects" above),
so it's not part of this check either.

It also reads the labels each issue template applies
(`.github/ISSUE_TEMPLATE`, an issue form's `labels` or a Markdown
template's front matter) and reports the ones this checkout's GitHub
repository (`gh repo view`) doesn't have. GitHub drops those without a
word, so the issue comes out without them:

```
issue templates: feature.yaml applies enhancement, triage, needs-triage, which EPFL-ENAC/co2-calculator doesn't have: GitHub drops them
```

`--apply` doesn't create them: the fix is a label the repository has, in
the template. This check only warns (`issue templates: not checked: …`)
when it can't run, `gh` not knowing the repository or a template that
doesn't parse: the exit status is the labels'.

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
| `c` | list | quick-filter by the configured `cat` field's options (`(all)` clears it); coexists with `/` search. The shipped example config doesn't define `[fields.cat]` (Category fills the kind label now — see the bridge field mapping below), so `c` shows "no cat field configured" unless a project opts into one. |
| `f` | list | structured filter — the same `field:value`/`-field:value`/`is:open`/`title:*glob*` grammar and matcher as `ls`/`view`/the launch argument and the web UI (words look in descriptions and comments too), not the fuzzy `/` search. Empty input + enter clears it. |
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
no separate service. `internal/server` reuses `internal/config`,
`internal/gitbugio`, `internal/fields` and `internal/board` directly, so
`ls`/`view`/`set` and the API apply the exact same filter/group and
field-write logic.

**Opening it.** `git board`, with no command, opens this repository's
board in the browser. When its server already runs it opens that one;
otherwise it starts it in the foreground on `127.0.0.1`, port 8080 or
the next free one, prints the address, and Ctrl-C stops it
(`--no-open`: no browser). Each repository gets its own server and its
own port: run `git board` in two repositories, get two boards. A running
server says where it is in `.git/git-board/server.json` (address and
pid), written when it starts and removed when it stops; `git board`
trusts it only when that process is alive and its server answers that it
serves this repository (`GET /api/repository`), so a stale file or a
port now serving another repository never opens the wrong board.
`git board serve` runs the server alone, without a browser, on `--addr`
(`localhost:8080` by default): what a hosted deployment runs.

While it runs, the server holds the repository (git-bug's lock): another
`git board` or `git bug` command on it fails with "locked by the process
pid …"; `git board` adds that this repository's server holds it, and its
address. Use the web UI there, or stop the server first.

The page has one Filter box, the view's filter until changed; the server
matches it and counts the sidebar's checkbox filters, one per field (each
value counted against the rest of the filter), and a tick writes into the
box (`status:ready,qa`). ⌘K takes the same filter, on every issue. The page
only sorts the answer: table headers that sort on click (option order,
numbers as numbers, missing values last). The filter and sort live in the
URL (`#/?view=backlog&filter=is:open+status:ready&sort=-est`), so a link
keeps them.

**Issue management.** git-board is the one interface to the issues; it
replaces git-bug's own `git bug webui`. The panel (and the full page,
`#/issues/<git-bug id>`, a link to share; `?issue=<id>` on the board)
shows an issue's whole history from git-bug: the description and the
comments with their edit history, field changes in words ("set Status to
qa"), closed/reopened, renames, and per forge what hasn't reached it yet.
From it: comment (Write/Preview, "Close with comment"), edit any comment
or the description (refused if someone changed it meanwhile), rename,
close/reopen, set fields and labels. **New issue** (or `c`) prefills the
fields from the view's filter. `j`/`k` move through the board's issues.
In a table, a field's cell is edited in place (click it, or Enter on
it), the **Columns** button shows, hides and orders the columns (Links
opens the issue on each forge), and the boxes pick rows for the bulk
bar: set a field, add an assignee or close them all.
The side column's **Development** lists the branches naming the issue
(`fix/2518-…`), ahead and behind the base, in it or not (merged, or squashed in), and **Create
branch** gives the commands to copy (`[branches]`, docs/configuration.md).
When origin is on GitHub, a branch there shows its pull request, its checks
(required first) and approvals, and **Open PR** or **Merge into dev**, done on
GitHub with your own token.
The panel opens at the side, resized by dragging its left edge
(double-click: the default width), or wide over the blurred board with
2.5rem around it: the expand button or `f` switches; this browser
remembers both.
Every write is signed by the repository's git-bug identity: the page asks
who you are when none is set (`git bug user adopt`). Issue text is
rendered on the server (GitHub-flavored Markdown) and cleaned to what
GitHub allows; forge images go through `/api/media`, which fetches only
forge image addresses, with your token for private repositories, and
keeps them in `.git/git-bug/git-board-media`.

**Attachments.** Files pasted, dropped or picked in a comment go to an
S3 bucket a CDN serves — EPFL ENAC's `enacit4r-cdn.epfl.ch` (bucket on
`s3.epfl.ch`) by default — and the comment gets their Markdown (an image
shows, other files link), so they show in git-board, on GitHub and on
GitLab alike. Set where the git-board server runs:
`ACCESS_KEY_ID`, `ACCESS_KEY_SECRET`, `BUCKET_NAME`; optionally in
`.git-board.toml`:

```toml
[uploads]
endpoint = "https://s3.epfl.ch"              # S3
public_url = "https://enacit4r-cdn.epfl.ch"  # where the bucket is served
prefix = "git-board/co2-calculator"          # default git-board/<repository>
```

Files are public-read, under a random path (the CDN isn't browsable):
anyone with the link can open them, even for a private repository
(GitHub's own attachments to a private repository need a login).
Deleting one from the bucket doesn't unpublish it: the CDN caches files
for up to a year.
Images (not SVG), PDF, video, text, CSV, zip and office files, 25 MB at
most.

**Dark mode**: Settings → You → Appearance, System (as the computer is set),
Light or Dark; `web/src/css/dark.css` re-values the design system's
decision tokens.

**Sprint report** (Insights, or a view filtering `sprint:@current`): the
sprint as Markdown to paste in Slack, an email or an issue — its dates,
how much is done and how long is left, the Insights findings, the issues
by where they stand (linked to their forge, with their assignees), and
what was added after the sprint started. Copy or download it
(`GET /api/report?sprint=@current&format=md`).

**Live updates.** Every open page follows the server's changes
(`GET /api/events`): a write from another page, the end of a sync, a
saved view or configuration, or the issues' refs changing outside the
API. The board reloads (keeping its scroll), an open issue too — unless
you are writing in it, then it offers the reload and keeps your text.

**Settings** (`#/settings`, the gear in the header), in tabs, each with
its address (`#/settings/forges`…): first the **Overview**, what git-board
is and a card per part saying where it stands; then where your issues go,
**Remotes**, **Forges** and the **GitHub Project** (below); then this
board: **Project file**, `.git-board.toml` itself: colored, checked
as you type (the line where it breaks, keys git-board doesn't know),
"+ View", "+ Sprint" (follows the last one) and "+ Field" to start from,
a diff to review, and saved without a restart, beside its reference (a
save is refused when the file changed on disk since the page read it);
**Numbers**, the project's own (`CO2-3075`), started and run from there
beside their guidelines (docs/configuration.md, Numbers);
**Attachments**, and **You**, who you write as and the appearance.
**Forges** lists the bridges: sync on or off, remove one (its issues
stay), and **+ Add a forge**, in four steps: the forge, the project, a
stored or pasted token, and the check the forge makes before anything is
kept (`#/settings/forges?add=1` opens it). "How forges work" shows
[docs/forges.md](docs/forges.md).

**Where your issues go** (`#/sync`, the header's Sync button) is three
ways, side by side, each with its own buttons:

1. **Remotes** (git): issues and people go with git, to any remote. Per
   remote, how many issues and people a push would send and a pull would
   receive, as of the last fetch; **Check** fetches and compares
   (nothing local changes), **Pull** and **Push** are `git bug pull` and
   `git bug push`. Code still goes with `git push`.
2. **Forges** (their API): git-bug's forge sync — the fork's `git bug
   bridge sync` — in the server, which holds the repository (so the
   command can't run beside it). **Check** is a dry run with its counts
   per forge both ways; then **Get** brings the forges' changes in only
   (`git bug bridge pull`), **Send** sends yours only (`git bug bridge
   push`), **Sync** does both. Each is confirmed with the check's counts
   (an explicit "I understand" when it creates 50 issues or more on a
   forge). Send waits when the check found issues on a forge by their
   git-bug marker: only a pull links them, a send alone would create them
   again. It waits too when the check found an issue changed both here
   and on that forge: a send replaces the forge's labels whole (one added
   there goes), its title, state and comment texts. Get or Sync first.
   While a run goes on, edits wait and are saved after it.
3. **GitHub Project** (fields): its write mode, the last check's
   conflicts, **Check**, and **Pull fields** (**Sync fields** with
   `write = true`), as on the Conflicts page below.

One issue syncs on its own from its panel ("On the forges"): **Send now**
when it has changes waiting, **Refresh from forges** otherwise — the
fork's `git bug bridge sync --issue`, a few calls per forge. It dry-runs
first, goes ahead when nothing would change on a forge, else says what it
would send and asks.

With a `[github-project]`, a **Conflicts** page (`#/conflicts`) is the web
side of `git-board bridge sync`/`resolve`: "Check the Project" lists the
fields both sides changed since they last agreed, each with git-board's
value, the Project's and the last agreed one, and "Keep git-board's" /
"Keep the Project's" resolves it; "Sync now" is `bridge sync --apply`. The
Project is read only when you press one of those (it costs GitHub API
points and takes seconds on a big Project); the header's count is the last
check's. Keeping git-board's value needs `write = true`, like the CLI.

**No auth in v1.** The server listens on this machine only
(`127.0.0.1:8080`) by default, and refuses writes from other sites (a
browser's `Origin`, or a body that isn't JSON). Anyone who can reach the
port can read and write every issue, and through the forge sync post to
GitHub and GitLab with your tokens: only pass an `--addr` other machines
can reach on a network you trust. Upgrade path when that stops being fine: a reverse proxy doing auth
in front of it (simplest), or a token check in `withCORS`/a new middleware
in `internal/server` (more work, still small).

```bash
# run from inside the git-bug repo, next to .git-board.toml
git board                                  # this repository's board, in the browser
git board serve                            # the server alone, on localhost:8080
git board serve --addr 127.0.0.1:9090 --me guilbep

# frontend, dev mode (separate terminal) — talks to the backend on :8080
# via CORS (see internal/server/cors.go); Quasar's dev server is :9000
cd web && npm install && npm run dev

# production: build the frontend into web/dist, then build one binary that
# embeds it (web/embed.go) — no separate frontend deploy/CDN needed
cd web && npm install && npm run build
cd .. && go build -o git-board ./cmd/git-board
./git-board serve   # serves the API and the built UI on the same origin
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
start; `git-board html` was the first thing to branch on it, and the web
UI now does too (board columns or grouped tables). The CLI's own
`render.go` still prints tables only.

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
trusted-network stance as the web UI above (see its "No auth in v1"
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
internal/ghlabels/      `gh label list`/`gh label create` — GitHub-side label creation for `lint --create-labels`.
internal/server/        the web UI's HTTP API (net/http, no framework) + the embedded web/dist; `git-board serve`.
cmd/git-board/          main.go + cobra commands, thin rendering (incl. html.go, the static-site generator).
web/                    Quasar/Vue frontend — one board page, api/board.ts is the only HTTP client.
```

Split this way so the TUI and the web frontend both reuse `internal/*`
without touching the CLI.

## Development

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for setup (including the
`GOPRIVATE` step above) and conventions.

```bash
git clone git@github.com:guilbep/git-board.git
cd git-board
make ci    # build + vet + test + gofmt check, in one shot
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

## License

git-board is free software, under the GNU General Public License,
version 3 or (at your option) any later version: GPL-3.0-or-later, the
text in [LICENSE](LICENSE). It builds against git-bug, which is GPL-3.0.
