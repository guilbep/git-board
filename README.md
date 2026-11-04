# git-board

Issues, fields, sprints and views kept in git, next to your code — a
lightweight, config-as-code replacement for GitHub Projects, synced with
GitHub and GitLab. Your issues are yours: independent of any forge, every
change kept.

**Documentation: <https://guilbep.github.io/git-board/>** — the
[user manual](https://guilbep.github.io/git-board/manual/) (install, first
steps, everyday work, teams, owning your issues), the
[CLI reference](https://guilbep.github.io/git-board/guide/cli/), the
configuration guide, the core's spec and the architecture decisions. The
same pages are in [docs/](docs/index.md).

Each issue is a log of operations under `refs/board/` in the repository:
it travels with `git push` and `git pull`, works offline, and needs no
database. Field values are namespaced labels (`status:ready`, `prio:p1`,
`sprint:delivery`, ...), so they merge like any other operation. Sprints
and saved views are a `.git-board.toml` file, reviewed and diffed like the
rest of your repo. The forges are mirrors for the people who work there:
`git board sync` pulls from them and sends to them, found from `origin`
with no configuration (docs/guide/forges.md). The core library, its
storage and sync, is specified in docs/spec/.

## Install

git-board embeds its web app, which `make web-build` builds, so install
from a clone:

```bash
git clone git@github.com:guilbep/git-board.git
cd git-board
git checkout v0.5.0                 # or stay on main
make install                        # the web app built, then git-board with it inside
git-board --help
```

`make install` builds the web app first: its pages are embedded at build
time from `web/dist`. `make` alone lists every target.

From a clone, `make install-latest` installs the latest release's binary
instead, built with the web app. It downloads the archive for this machine
with `gh` and checks it against `checksums.txt`.

Requires Go 1.26+ and Node 22.12+ for the web app; `go install` fetches
the matching Go toolchain automatically if needed. `git` must be on the
path: git-board reads and writes the repository through it.

### What else to install

git-board runs on `git` alone. The rest is per feature,
and only for that feature:

| For | Install (macOS) |
|---|---|
| `project` (GitHub Projects), and `lint`'s issue-template check | `brew install gh`, then `gh auth login` (`lint --create-labels` itself goes through the forge token: `GITHUB_TOKEN`, else git's credential helper) |
| Start an agent (Cycle, Work; experimental) | [wtx](https://github.com/EPFL-ENAC/wtx) |
| Run step 4's tests and checks, `verify` | `ssh-keygen` (macOS has it), `git config user.signingkey` naming your SSH key |
| `release` and `deploy`, with `[release] images` | Docker Desktop (buildx), `brew install crane trivy` |
| `release` and `deploy`, with `[release] chart` | `brew install helm` (3.8+) |
| `release`, with `[release] binaries` (publishes the GitHub release) | `brew install gh`, then `gh auth login` |

A release command missing one names it and says how to get it.

**Docker** builds with an SBOM and provenance, which Docker Desktop's
default builder keeps only with the containerd image store (Settings ›
General › "Use containerd for pulling and storing images"). Otherwise,
make a builder that does, once: `docker buildx create --name git-board
--driver docker-container --use`.

**Pushing to ghcr.io** needs a token with `write:packages`. `gh` has one
once asked:

```bash
gh auth refresh -h github.com -s write:packages
gh auth token | docker login ghcr.io -u <github-login> --password-stdin
gh auth token | helm registry login ghcr.io -u <github-login> --password-stdin
```

crane uses docker's login. Another registry: its own `docker login` and
`helm registry login`.

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
the root of this repository, and its issues are git-board's, under `refs/board`
(`git board sync --git` fetches them, then run `git board`).

Optional: without it, every `ns:value` label is a field and `git board`
works on any repository. `git board init` writes the default one
(Settings › Project file offers the same): statuses, priorities, kinds,
estimates, branch names and four views, the conventions for labels and
titles in comments. The file adds what labels can't say: the order of a
field's values (`options`: status follows the workflow when sorted and on
a board), single or multiple values, sprint dates (`@current`), saved
views, and the GitHub Project to sync with.

An issue opens on the classic page: the description, the comments and
changes in order, History, the fields.

**Experimental features.** The development cycle, the coding agents and
the worktrees are still being built, and off unless the team turns them
on: `[experimental] features = ["cycle", "agents", "worktrees"]` in
`.git-board.toml` (Settings › Experimental), or, for one person trying
them in their clone, `git config board.experimental cycle,agents` (a
comma list that replaces the file's there). Off, their commands are
hidden and refuse naming the switch, their API routes answer 404, the web
UI hides their pages, tabs and controls, and a `[cycle]` table is not
checked (a broken one is a warning, not an error). See
[Experimental features](docs/guide/configuration.md#experimental-features).

With the cycle on, an issue opens on the cycle's page when the board's
`.git-board.toml` has a `[cycle]` table, or you pick it in Settings › You
(`git config board.view cycle`): its page is its step in the development cycle, computed from its
status and the evidence (criteria, risk, plan file, tests in the branch,
PR, sign-offs). A bar of the seven steps runs under the title: a done step
reopens, a later one is locked and says why. The step shown is the main
screen (the description at Intent, the plan to approve, the diff at
Review, the run and the sign-offs at Verify), with its button that moves
the status, and its own discussion beside it. With the cycle, the Work page is what needs
a person across every open issue: Needs you, Agents, Waiting on others, To
send, Since the last retro. With the agents on, from both, Start an agent runs
[wtx](https://github.com/EPFL-ENAC/wtx) on the issue: a worktree, a tmux
session and a prompt with its step and gate; Settings › Worktrees writes
`wtx.toml`. `[cycle]` in [docs/guide/configuration.md](docs/guide/configuration.md#cycle),
agents in [Worktrees](docs/guide/configuration.md#worktrees).

People show their picture where there were initials: their github.com
account's by default, also Gravatar's (by a hash of the email) if you
choose it, or none, in Settings › You › Pictures (`git config
board.avatars forge|gravatar|none`, `--global` for every repository). The
server makes the picture's address, the page never gets an email, and a
picture that can't load shows the initials
([docs/prd/avatars.md](docs/prd/avatars.md)).

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
filters, sprints, the GitHub Project, attachments: **[docs/guide/configuration.md](docs/guide/configuration.md)**.
The settings page shows the same reference next to its editor, at the
section of the table under the cursor.

## Usage

Run from inside a git repository (or a subdirectory of one), next to a
`.git-board.toml`.

```bash
# this repository's web UI, in the browser (see "Web UI" below)
git board

# who you write as: once per clone, before the first write
git-board me --new          # from git user.name and user.email
git-board me --ls           # or write as someone already on the board:
git-board me <person-id>

# open issues
git-board ls

# ad-hoc filter, grouped and sorted; the ten last changed
git-board ls "is:open -title:*spec*" --group status --sort "prio asc"
git-board ls --sort "updated desc" --limit 10

# a named view from .git-board.toml, rendered as markdown or JSON
git-board view current-sprint --md
git-board view backlog --json

# create an issue, with its description and fields (-F file, -e editor)
git-board new "Login fails on Safari" prio=p1 assignee=+@me -b "Steps: ..."

# an issue with its fields, description and comments (--md, --json)
git-board show <issue-id>

# comment (no flag: the editor), edit your last comment
git-board comment <issue-id> -b "Fixed in 4e5f6a7"
git-board comment <issue-id> --edit-last

# edit the title or the description
git-board edit <issue-id> --title "Login fails on Safari 17"

# set fields (single-cardinality fields swap; assignee is multi); held to
# the config's options unless --force; field= clears; label=+x a plain label
git-board set <issue-id> status=ready prio=p1 sprint=@next assignee=+@me
git-board set <issue-id> prio= label=+regression
git-board set --filter "status:ready no:assignee" assignee=+@me --apply

# an issue's fields (labels), one per line, for scripts
git-board get <issue-id>

# step status through the configured options (backlog -> ready -> ...)
git-board move <issue-id> --next
git-board move <issue-id> --prev

# set status to the last configured option (the terminal state, whatever
# it's called in your config) and close the issue; reopen undoes it
git-board close <issue-id> -c "Fixed in 4e5f6a7"
git-board reopen <issue-id>

# every change made to an issue: who, when, what (--field status, --json)
git-board log <issue-id>

# the branch to create for an issue, and the command creating it
# ([branches] in .git-board.toml); it creates nothing
git-board branch <issue-id>

# report label/config drift, read-only (see "Linting" below)
git-board lint

# whether a commit's checks are proven: the signed results in
# refs/notes/ci/*, against the base branch's [cycle.checks] (exit 1 if not;
# docs/guide/configuration.md, Tests and checks); the cycle's, experimental:
# hidden, and refused until [experimental] features turns "cycle" on
git-board verify
git-board verify fix/2518-retry --base origin/main

# the whole sync: git pull, the forges, the GitHub Project, git push
# (defaults to cwd); --check is its dry run, --git keeps to git
git-board sync --check
git-board sync
git-board sync --git ../co2-calculator ../git-time

# the project's own numbers, CO2-3075 (docs/guide/configuration.md, Numbers)
git-board numbers init --prefix CO2 --start 3052
git-board numbers

# sprints: list/resolve/create/report (see "Sprints" below)
git-board sprint ls
git-board sprint current
git-board sprint new q1-hardening --title "Q1 Hardening" --start 2026-10-05
git-board sprint report delivery

# what runs in each environment, and cut/promote/roll back/adopt/yank a
# release ([release] in .git-board.toml)
git-board release
git-board release cut
git-board release cut --final --version 1.4.20
git-board release cut --offline --no-deploy  # tags locally; the next online command pushes it
git-board release promote v1.4.20-rc.2
git-board release rollback prod v1.4.19 --reason "bad chart"
git-board release adopt --yes
git-board release notes --all
git-board release yank v1.4.19 --reason "corrupts uploads"

# a Go program with no deploy targets: Enable binary release on the Releases
# page writes [release] binaries and build for you; a cut then builds the
# archives in a clean checkout of the tag and publishes them on GitHub

# deploy the trunk's head to the environment that follows it; --watch keeps
# it there, fetching every minute and deploying when behind, until interrupted
git-board deploy dev
git-board deploy dev --watch
```

`@me` (in filters, e.g. the `mine` view, or as a field value) resolves from
`--me` on `ls`/`view`/`set`/`new`, falling back to the `GIT_BOARD_ME` env
var, then to the forge login of the person you write as. It errors if
none is set.

Every issue command, its flags, its JSON output and its exit codes, with
a table for those coming from `gh` and `glab`, are in
[docs/guide/cli.md](docs/guide/cli.md).

## Syncing (`git-board sync`)

`git-board sync [path...] [--remote origin]` runs the whole sync on each
path, as Settings › Sync does: a git pull from the remote, the forge sync
of every forge whose sync is on, the GitHub Project's fields with
`[github-project]`, then a git push, which numbers the issues origin has.
A failed pull stops the steps after it; a failed forge or Project step is
told, and the push still runs; a forge that can't be read is left out,
told, and the others go on. `--check` runs the same steps writing nothing;
`--git` syncs with git alone, a pull then a push. Defaults to the current
directory if no paths are given; keeps going past a path that fails so one
broken repo doesn't block the rest, then reports every failure together at
the end. [docs/guide/forges.md](docs/guide/forges.md) says how the forges sync.

Takes explicit local paths, **not** `.git-board.toml`'s `[repos]` list —
that table holds `"owner/repo"` display names, not filesystem locations,
and there's no established convention yet for mapping one to the other.
Config-driven multi-repo sync is tracked as a follow-up in
`docs/implementation-plan.md`, not solved here.

## The GitHub Project (`git board project`)

`git board project` keeps a GitHub Project (v2)'s custom field values and
the `ns:value` labels above in step, on git-board issues a sync already
linked to their GitHub issue (matched by the GitHub issue's URL, not its
title). It was `git-board bridge`, which still works, deprecated, for one
more release.

It has git's verbs, as the git remote and the forges have theirs:

- `pull` takes the Project's changes into the labels, and writes nothing
  to the Project;
- `push` writes git-board's changes to the Project, and changes no label;
- `sync` does both: the third step of `git board sync`.

Each acts at once; `--check` says what it would change and writes nothing.
They read the Project with `gh project item-list` (needs `gh`
authenticated with at least `read:project` scope); the labels change
through the same label change `git-board set` makes.

This is meant to be run **repeatedly**, not once — a GitHub Project's field
values (status, priority, sprint, assignees, ...) keep changing as work
progresses. Every run compares again, so running it twice back-to-back is
a no-op the second time.

```bash
# one-time: remember which GitHub Project to read (or, in the web UI,
# Settings › Sync, the Project's line: its number, or its address)
git board project configure --owner EPFL-ENAC --project 40

# what the Project would change on the labels, and a
# matched/unmatched/warnings summary
git board project pull --check

# write the labels
git board project pull
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
| Assignees | `assignee:<user>` | multi — one label per assignee, merged person by person (below) |
| MoSCoW | `moscow:<value>` | single |
| Milestone | `milestone:<slug>` | single, **read only**: the issue's GitHub milestone, which a Project edit can't set; the Project's value always wins |

`Size` is **not** imported — the PRD calls it a dead/unused field.

Category says the same thing as the kind labels (`bug`, `feat`, `perf`…),
and the labels win: Category only fills a kind the issue doesn't have,
written as `[branches] kinds` spells it, so `FEAT` lands as `feat`. A
Category that is none of the kinds (`FIX` when the kind is `bug`) warns
and writes nothing. A kind label is never removed: it's the team's to
change. What goes is the bare label an earlier bridge wrote, Category as
is (`FEAT`, `SPECS`): a sync would send it to GitHub, which applies `feat`
(beside a `bug`, if there is one) or creates `SPECS`.

Every verb reads values the same way: a
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
no sync has brought into git-board yet.

### Both ways

Status, prio, mod, est and sprint are kept in step the git way: each field
is compared with the value both sides last agreed on, which git-board
records on the issue (in its history, so it travels with the issue).
`pull` takes the first case below, `push` the second, `sync` both; a value
a verb leaves for the other side isn't recorded as agreed, so the other
verb still sends it.

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
git board project sync --check                 # what would change, on each side
git board project sync
git board project resolve 2605 status --theirs # or --ours
git board project resolve EPFL-ENAC/co2-calculator#2605 status --ours
```

A number two repositories of the Project both have in conflict is refused:
name the repository, `owner/repo#number` (or the issue's URL).

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
qualifier: [Filters in docs/guide/configuration.md](docs/guide/configuration.md#filters).

## Linting (`git-board lint`)

`git-board lint` is **read-only** — it reports drift, writes nothing, unlike
everything else in this CLI. It checks every `is:open` issue in the current
repository against `.git-board.toml`:

- **unknown keys** — a key in `.git-board.toml` git-board doesn't read, a
  misspelled one above all (`[github-project] wirte = true`, `[view.x]` for
  `[views.x]`): it would be its zero value without a word. Every command also
  warns of it on stderr, once, and runs on.
- **double singles** — more than one `ns:*` label for a `[fields.<ns>]` entry
  with `single = true` (e.g. two `status:*` labels). Structurally rare going
  forward (`fields.Apply`'s swap logic prevents it), but concurrent
  operations from two people could still produce it.
- **unknown option values** — a `field:value` label where `value` isn't in
  that field's configured `options` (fields with no configured `options` are
  free-form and skipped). Same check `git board project pull` already warns on.
  A value that matches an option but for case or punctuation (`P-2`) says
  which one was meant (`did you mean prio:p2?`).
- **near values** — on a field with no `options`, two spellings of one value
  (`risk:-normal` and `risk:normal`): each issue carrying the less used one
  is told which most issues use.
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
…) live inside git-board's issues, not GitHub's — so they don't
exist as real, colorable GitHub labels until someone creates them.
`--create-labels` closes that gap: it diffs every `ns:value` label
`.git-board.toml`'s `[fields.*]` options and `[[sprints.list]]` entries
imply against each configured `[repos]` entry's real GitHub labels (read
through GitHub's API with the forge sync's token), and reports what's
missing. Preview only by default, same convention as the old `bridge pull`;
`--apply` actually creates each missing one.

```bash
git-board lint --create-labels           # preview what's missing, writes nothing
git-board lint --create-labels --apply   # create the missing labels on GitHub
```

Free-form fields (no `options` configured — `assignee`, `est`) have no
fixed value set, so nothing to create labels for; Category isn't a
configured field at all anymore (see the Project's field mapping above),
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
  sprint's start, or `start + [sprints].length_days` for the last one
  (without `length_days`, as long as the sprint before it, else 14 days).
  `current`/`next`/`previous` print the same computed range for the sprint
  they resolve to. Once the last sprint has ended there is no current one
  (`@current` matches nothing), and `previous` is the one that ended.
- **No sprint resolves** (before the first configured sprint has started,
  after the last one has ended, or nothing configured at all) is a normal outcome, not an error: `current`
  prints `no sprint currently active` and exits 0, same idea for
  `next`/`previous`. Matches `ResolveSprint`'s own `(nil, nil)` return for
  "nothing resolves" — it's not a `%!s(<nil>)` crash waiting to happen.
- **`new`** appends `[[sprints.list]]` via the same targeted text-surgery
  `git board project configure` uses to write `[github-project]` — find the
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
| `i` | list | preview `git board project pull` (a dry run): the Project's changes into the labels. Requires `git board project configure` to have been run first. Runs in the background; the TUI stays responsive while it fetches. |
| `o` | list | apply the pull `i` just previewed, decided again from the issues as they are now: an edit made in between stays. Refuses to run before an `i` (no blind apply); the preview is cleared after applying so `o` can't silently re-apply stale data. |
| `e` | detail | edit a field: pick the field, then a value from its configured options, or type free text if it has none (e.g. `assignee`, using the same `+value`/`-value` syntax as `git-board set`) |
| `esc`/`q` | detail, pickers | back |
| `q`/`ctrl+c` | list | quit |

`f` and `/` are two different tools: `/` is a quick fuzzy text match, useful
for "I remember a word in the title." `f` is precise and composable —
`assignee:guilbep -status:validated` narrows exactly like it would on the
CLI, because it's the exact same `core/filter` grammar and code path.

`i`/`o` show a one-line summary (matched/changed/unmatched/conflict/warning
counts), not the full per-issue diff — that's what `git board project pull --check`
on the CLI is still for. They are `git board project pull`, the sync's
git-board side: only what changed on the Project since both sides last agreed
is taken, a change made in git-board and not yet pushed stays, and someone
assigned in git-board stays assigned.

The TUI shows an issue's fields (labels), not its body and comments: the
web UI (`git board`) shows those.

The detail view renders each label under its configured field name (e.g.
`Status: ready`, `Assignee: alice, bob`); anything not matching a configured
field namespace is listed under "Other labels" rather than dropped. Field
edits write through the same `fields.Apply` primitive as `git-board set` and
the web UI's `PATCH`, so the write is real and immediate — a failure (e.g. a
repo/network issue) is shown inline and the TUI stays open.

## Web UI

A one-page board, backed by a Go HTTP API in the same module — no Python,
no separate service. `internal/server` reuses `internal/config`,
`internal/store`, `core/fields` and `internal/board` directly, so
`ls`/`view`/`set` and the API apply the exact same filter/group and
field-write logic.

**Opening it.** `git board`, with no command, opens this repository's
board in the browser. When its server already runs it opens that one;
otherwise it starts it in the foreground on `localhost` (both loopback
addresses), port 8080 or the next one free on both, prints the address,
and Ctrl-C stops it
(`--no-open`: no browser). Each repository gets its own server and its
own port: run `git board` in two repositories, get two boards. A running
server says where it is in `.git/git-board/server.json` (address and
pid), written when it starts and removed when it stops; `git board`
trusts it only when that process is alive and its server answers that it
serves this repository (`GET /api/repository`), so a stale file or a
port now serving another repository never opens the wrong board.
`git board serve` runs the server alone, without a browser, on `--addr`
(`localhost:8080` by default): what a hosted deployment runs.

While it runs, other `git board` commands work on the repository beside
it: what they change reaches the pages within seconds.

The page has one Filter box, the view's filter until changed; the server
matches it and counts the sidebar's checkbox filters, one per field (each
value counted against the rest of the filter), and a tick writes into the
box (`status:ready,qa`). ⌘K takes the same filter, on every issue. The page
only sorts the answer: table headers that sort on click (option order,
numbers as numbers, missing values last). The filter and sort live in the
URL (`#/?view=backlog&filter=is:open+status:ready&sort=-est`), so a link
keeps them.

**Issue management.** git-board is the one interface to the issues; The panel (and the full page,
`#/issues/<id or number>`, a link to share; `?issue=<id>` on the board)
shows an issue's whole history: the description and the
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
branch** gives the commands to copy (`[branches]`, docs/guide/configuration.md).
When origin is on GitHub, a branch there shows its pull request, its checks
(required first) and approvals, and **Open PR** or **Merge into dev**, done on
GitHub with your own token.
The full issue page's **Changes** tab shows the branch's diff, file by file,
with line comments, replies, resolve, and Comment / Approve / Request
changes; the sync mirrors them to the open pull request and brings its
comments back (docs/guide/configuration.md, Branches).
The panel opens at the side, resized by dragging its left edge
(double-click: the default width), or wide over the blurred board with
2.5rem around it: the expand button or `f` switches; this browser
remembers both.
Every write is signed by who you are in git-board (`board.me` in the
repository's git config): the page asks when none is set. Issue text is
rendered on the server (GitHub-flavored Markdown) and cleaned to what
GitHub allows; forge images go through `/api/media`, which fetches only
forge image addresses, with your token for private repositories, and
keeps them in `.git/board/git-board-media`.

**Attachments.** Files pasted, dropped or picked in a comment go to an
S3 bucket a CDN serves — EPFL ENAC's `enacit4r-cdn.epfl.ch` (bucket on
`s3.epfl.ch`) by default — and the comment gets their Markdown (an image
shows, other files link), so they show in git-board, on GitHub and on
GitLab alike. Set where the git-board server runs:
`ACCESS_KEY_ID`, `ACCESS_KEY_SECRET`, `BUCKET_NAME`, and `UPLOADS_ENDPOINT`
for an S3 service other than `https://s3.epfl.ch`: the keys go only where
the server's environment says, so an `[uploads] endpoint` in the committed
file that differs turns attaching off, as does a `prefix` with a `..`.
Optionally in `.git-board.toml`:

```toml
[uploads]
endpoint = "https://s3.epfl.ch"              # S3: must be UPLOADS_ENDPOINT's
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
its address (`#/settings/sync`…): first the **Overview**, what git-board
is and a card per part saying where it stands; then where your issues go,
the **Sync** (below); then this board: **Project file**, `.git-board.toml` itself: colored, checked
as you type (the line where it breaks, keys git-board doesn't know),
"+ View", "+ Sprint" (follows the last one) and "+ Field" to start from,
a diff to review, and saved without a restart, beside its reference (a
save is refused when the file changed on disk since the page read it);
**Numbers**, the project's own (`CO2-3075`), started and run from there
beside their guidelines (docs/guide/configuration.md, Numbers);
**Attachments**, and **You**, who you write as and the appearance.
**Sync** (`#/settings/sync`, the header's Sync button, which counts what
waits to go out) is where your issues go, one table with a row per place:
the git remote, `origin`, with what a push would send and a pull receive
as of the last fetch; each forge, its kind and project, found in the
remotes or in `.git-board.toml`, with its sync on or off (a table's can be
switched and removed); and the GitHub Project, a line under its GitHub
row, linking the Conflicts page. Each row says its last check, and the
remotes' URLs that name no forge are listed with why. **Check** runs the
whole sync writing nothing: a fetch and compare, the forges' dry run, the
Project's check. **Sync**, after a clean check of the last 15 minutes and
confirmed with its counts (an explicit "I understand" when it creates 50
issues or more on a forge), runs it: a git pull, the forges, the
Project's fields, a git push. A failed pull stops the steps after it,
any other failure is told and the push still runs, and the report says which, then per forge what came in and went out.
While it runs, edits wait and are saved after it. **+ Add a forge**, for
one no remote names, goes in four steps: the forge, the project, a stored
or pasted token, and the check the forge makes before anything is kept
(`#/settings/sync?add=1` opens it). "How forges work" shows
[docs/guide/forges.md](docs/guide/forges.md).

One issue syncs on its own from its panel ("On the forges"): **Send now**
when it has changes waiting, **Refresh from forges** otherwise, a few
calls per forge. It dry-runs
first, goes ahead when nothing would change on a forge, else says what it
would send and asks.

With a `[github-project]`, a **Conflicts** page (`#/conflicts`) is the web
side of `git board project sync`/`resolve`: "Check the Project" lists the
fields both sides changed since they last agreed, each with git-board's
value, the Project's and the last agreed one, and "Keep git-board's" /
"Keep the Project's" resolves it; "Sync now" is `git board project sync`. The
Project is read only when you press one of those (it costs GitHub API
points and takes seconds on a big Project); the header's count is the last
check's. Keeping git-board's value needs `write = true`, like the CLI.

**No auth in v1.** The server listens on this machine only
(`127.0.0.1:8080`) by default, and refuses writes from other sites (a
browser's `Origin`, or a body that isn't JSON). Anyone who can reach the
port can read and write every issue, and through the forge sync post to
GitHub and GitLab with your tokens: only pass an `--addr` other machines
can reach on a network you trust. On every interface (`--addr
0.0.0.0:8080`), it answers a request for an IP address or this machine's
own name, never for another name: a website pointing its own domain at
your machine (DNS rebinding) gets a 403. Running a branch's tests,
making a branch and starting an agent, which run your repository's code
or sign with your key, need a server on localhost. Upgrade path when that stops being fine: a reverse proxy doing auth
in front of it (simplest), or a token check in `withCORS`/a new middleware
in `internal/server` (more work, still small).

```bash
# run from inside the repository, next to .git-board.toml
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
`index.html` linking to all of them (a view's name is its page's file name:
one with a path in it, or named `index` or `releases`, is refused before
anything is written) — for PM/non-dev users who don't want a
CLI and shouldn't need the (also-unauthenticated) live web UI above. Reuses
`internal/board`'s `LoadMatching`/`SortByField`/`BucketBy` — the exact same
filter/sort/group semantics as `ls`/`view`/the web API — and the TUI's
`tui.ParseLabels`/`tui.DisplayFieldName` to render known fields under their
display name (`Status: ready`) instead of raw `status:ready` label text.
It writes no issue. On a fresh clone it first pulls origin's board, as
`ls`, `view` and `tui` do. A view with no `filter` lists every issue,
closed ones included, here and in `git-board view`; the web UI shows such
a view's open issues (`is:open`).

When the project has a `[release]` table, the export also writes
`releases.html`: the environment strip, the releases table and each
release's notes inline, read offline (the runner doesn't fetch) with the
export's own stylesheet. Without `[release]`, no such page.

`layout = "table"` vs `"board"` in a view's config picks the page shape: a
`"board"` view renders one column per bucket (kanban-style, cards); anything
else renders a flat table per bucket. This is a new distinction — `layout`
was parsed by `internal/config` and shipped to the web UI's API from the
start; `git-board html` was the first thing to branch on it, and the web
UI now does too (board columns or grouped tables). The CLI's own
`render.go` still prints tables only.

```bash
# run from inside the repository, next to .git-board.toml
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
core/model/            the read model: Issue, Thread, Event, Person — as every surface reads them.
core/fields/           the field schema (types, sprints, roles) and field=value assignments as labels.
core/filter/           grammar parser, Matches(issue, conditions, schema, me).
core/search/           free text's tokens and Match, as the index searches: the filter's, without it.
core/inbox/            what a person follows and has seen, and what's new for them.
core/release/          release records, versions and tags, the [release] table, GitOps overlays.
core/proof/            check results as signed git notes, keyed by content, and the gate (ADR 0001).
core/cycle/            an issue's step in the development cycle, the [cycle] table; work/: the Work page's rows.
internal/config/       TOML parsing and checks; embeds core/fields' Schema, holds core/release's and core/cycle's Config.
internal/evidence/     reads the cycle's evidence: branches, test files, plan files, the PR cache.
internal/ship/         the release runner: cut, promote, deploy, rollback, adopt — drives git, registries, gh.
internal/store/        git-board's store over the core (core/): issues, people, releases, forges.
internal/board/        load/sort/group issues — shared by the CLI, the TUI and the web API.
internal/ghbridge/     `gh project item-list` parsing + field-mapping/diff logic for `git board project`.
internal/lint/          read-only drift checks (labels vs. .git-board.toml) — internal/lint.Run.
internal/ghlabels/      GitHub-side labels, through the GitHub forge adapter — for `lint --create-labels` and Settings › Labels.
internal/server/        the web UI's HTTP API (net/http, no framework) + the embedded web/dist; `git-board serve`.
cmd/git-board/          main.go + cobra commands, thin rendering (incl. html.go, the static-site generator).
web/                    Quasar/Vue frontend — one board page, api/board.ts is the only HTTP client.
```

Split this way so the TUI and the web frontend both reuse `core/*` and
`internal/*` without touching the CLI; `core/` never imports `internal/`
(ADR 0005), so another tool can read issues, filter them, set fields,
read what shipped and compute an issue's step without git-board's
application.

## Development

See [`CONTRIBUTING.md`](CONTRIBUTING.md) for setup (including the
`GOPRIVATE` step above) and conventions.

```bash
git clone git@github.com:guilbep/git-board.git
cd git-board
make ci    # what CI runs: web checks and build, then go build + vet + test + gofmt
```

Run one package or one test by name:

```bash
go test ./core/filter/...
go test ./core/fields/... -run TestApplySingleCardinalitySwapsLabel -v
```

Tests run against real git repositories in `t.TempDir()` rather than a
hand-rolled fake, and the forges against fakes (`core/forge/fake`, httptest
servers). Every test binary is sealed off the network and your tokens
(`internal/hermetic`): only the live tests, with `BOARD_LIVE=1`, reach a
forge.

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

Shipped: config loading, the core library (issues in git, sync with GitHub and GitLab), the filter
grammar (with numeric-aware `--sort`), `set`/`get`/`ls`/`view`, an
interactive TUI (`git-board tui`), the web UI above, `git board
project` (GitHub Project ↔ labels, plus `i`/`o`
from inside the TUI), `git-board lint` (read-only drift report),
`git-board move`/`close`, `git-board sync` (explicit local paths,
not `[repos].list`), `git-board sprint` (`ls`/`current`/`next`/
`previous`/`new`/`report`), and `git-board html` (cron-published static
site) — the last planned M2 item, so M2 is now fully shipped. See
`docs/implementation-plan.md` for what's tracked next.

## License

git-board is under the MIT License, the text in [LICENSE](LICENSE).
