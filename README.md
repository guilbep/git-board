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

This is the **M1** milestone: fields, `set` / `get` / `ls` / `view`, the
filter grammar, and config loading. `lint`, `move`, `close`, `sprint`, the
static HTML site, and `sync` are later milestones and not implemented here.

## Install

This repo is **private**, so Go's default module proxy/checksum database
can't fetch it — tell Go to go straight to GitHub instead, and make sure
`git` can authenticate to it (SSH key on your GitHub account, or an HTTPS
credential helper with a token that has `repo` scope):

```bash
export GOPRIVATE=github.com/guilbep/*   # add to your shell profile to persist
go install github.com/guilbep/git-board/cmd/git-board@latest
```

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
```

`@me` (in filters, e.g. the `mine` view, or as a field value) resolves from
`--me` on `ls`/`view`/`set`, falling back to the `GIT_BOARD_ME` env var. It
errors if neither is set.

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

## Layout

```
internal/config/       TOML parsing, sprint resolution — pure, no I/O beyond reading the file.
internal/gitbugio/     thin wrapper around git-bug's cache.RepoCache API.
internal/fields/       ApplyField single/multi-cardinality logic and alias resolution.
internal/filter/       grammar parser, Matches(issue, conditions, config, me).
internal/board/        load/sort/group issues — shared by the CLI, the TUI and the web API.
cmd/git-board/          main.go + cobra commands, thin rendering.
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

## Scope

M1 only: config loading, the in-process git-bug cache adapter, the filter
grammar, and `set`/`get`/`ls`/`view`. `lint`, `move`, `close`, `sprint`,
`html`, and `sync` are M2+ and not implemented here.
