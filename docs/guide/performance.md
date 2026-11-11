# Performance

How fast git-board answers on a board of thousands of issues, how to
measure it, and how it stays fast. The numbers below were taken on a
2-CPU Linux machine (Xeon, 2.1 GHz), on boards in git. The design's
targets are in [the design](../rewrite/design.md), section 4.

## A synthetic board

`cmd/boardgen` makes a board as git-board's commands write one
(`tools/boardgen`):

```sh
go run ./cmd/boardgen -n 5000 /tmp/board5k
go run ./cmd/boardgen -n 20000 -seed 7 /tmp/board20k
```

- A git repository at the path, with a `.git-board.toml` (the cycle on),
  `board.me`, and the board in `refs/board/`.
- 25 people (`-people`), each with a GitHub account; issues with fields,
  bodies, comments, edits and status moves, some closed. Each change is
  its own pack, as a command writes it. The same seed makes the same
  board.
- `-forge` gives it a forge, GitHub's `acme/board`, as a `[forges]`
  table: its token is looked for on each read of the board's state, as
  on a team's clone, and nothing is asked of GitHub until a sync (a forge
  found in the remotes would be resolved on GitHub, a call out). The
  timed boards of `make perf` have one.
- For a board in PostgreSQL, copy it there: set `board.datastore` and run
  `git board datastore copy` ([Where the board is kept](datastores.md)).

## Measuring

The Go benchmarks of `surface/cli` and `surface/http` run on
`$GIT_BOARD_BENCH_REPO`, a board `boardgen` made; they skip without one.
The writes add to it: run them on a copy.

```sh
export GIT_BOARD_BENCH_REPO=/tmp/board5k
go test ./surface/cli -run - -bench . -benchtime 5x
go test ./surface/http -run - -bench . -benchtime 20x
```

Add `-cpuprofile cpu.out -memprofile mem.out` and read them with `go tool
pprof -top -cum`. For the command line itself, build the binary outside
the checkout and time it in the board.

## Traces

git-board records what each request, command and sync does as
OpenTelemetry spans, and sends them to Grafana Tempo, when
`OTEL_EXPORTER_OTLP_ENDPOINT` says where (`OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`,
whole, wins; `OTEL_SERVICE_NAME` names the service, `git-board` by
default). Without it nothing is recorded. The spans are the OTLP/JSON
format, sent by the `obs` package with no SDK (ADR 0014).

```sh
make obs-up        # Tempo on :4318 and :3200, Grafana on http://localhost:3001
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 git board serve
```

| Span | What |
|---|---|
| `GET /api/issues/{id}`… | a request, by its route; its status, its size |
| `git <command>`, `git credential fill` | a git process: the store's, the app's, the code's |
| `load` | entities read through `git cat-file`, how many |
| `view rows`, `view heads`, `view write` | the view's queries, rows read |
| `GET <host>` | a forge's request: method, path, status; its waits (rate limit, pace) as events |
| `sync run`, `sync <stage>`, `sync issues` | a sync, in the trace of the request that started it |
| `interaction <name>` | a page's act that took over 200 ms (`web/src/interaction.ts`): its requests are its children |
| `git board <command>` | a command, its spans under it |

Every reply says what it cost in a `Server-Timing` header (`app;dur`,
the git processes, loads and view calls, the trace id), so a page's
devtools show it, and a slow request's log line has its counts and trace.

The questions to ask Tempo, from a terminal or an agent:

```sh
# every request or interaction over 200 ms, the last hour
curl -sG localhost:3200/api/search --data-urlencode 'q={resource.service.name="git-board" && duration>200ms}' --data-urlencode limit=50
# one trace, every span
curl -s localhost:3200/api/traces/<trace id>
# the git processes of slow requests
curl -sG localhost:3200/api/search --data-urlencode 'q={resource.service.name="git-board" && name=~"git .*" && duration>50ms}'
```

## Tried, and left

Measured with Lighthouse's mobile run on the 5,000-issue board, each
worse, and reverted; not to try again without a new reason:

- fetching `/api/config` beside the board page's code: largest paint
  2.79 s to 3.08 s (3 runs);
- preloading the opened page's chunks from `index.html` (a generated
  `route-preload.js`): the board's largest paint 2.47 s to 2.68 s, an
  issue's 2.76 s to 2.91 s (3 runs, 2026-11-11). On slow 4G they take
  the bandwidth the first paint needs.

## Guarded, then timed

git-board holds its speed two ways: guards that count work, in every
`go test`, and budgets that time it, nightly.

### What's guarded, in every `go test`

The complexity guards run on a board of 2,000 issues that
`tools/boardgen` makes in a second (the same for a seed: the packs
as commands write them, one `git fast-import`), once per package run,
copied for each test. Each read or write runs with a `store.Counts` in
its context; the git store, the kernel's view and the code's reads
(`code.Counting`) add to it, and nothing is counted without one. A guard
holds each count to a limit far below the board's size, so an N+1 or a
rebuild fails it by thousands, and a slow runner can't fail it at all.

| Guard (`app/complexity_test.go`) | Holds |
|---|---|
| first `ls` | every entity loaded once, in a few loads and processes |
| `ls`, a field filter, `is:closed`, `assignee:@me`, free text (its index built), facets | no entity, no git object, no process; a few listings and view calls |
| first free-text `ls` | the index built in a few loads |
| `show` | its issue alone: one load, its objects |
| `comment`, `set` | one pack added, one row put, one entity (two for `set`) |
| `ls` after a pull moved one issue | that one loaded and put |
| the Work page: first, again, after a comment | every open thread in one load and a few git processes; then nothing; then that thread |

`surface/http/complexity_test.go` holds the routes the same way:
`/api/issues` (a page, all, a column), `/api/labels`, `/api/fields`,
`/api/people`, `/api/board`, `/api/inbox` read no entity and make a
few view calls each; `/api/issues/{id}` loads one; `/api/cycle` reads
its threads in one load, then nothing.

A guard that fails names the counter, the count, its limit and every
counter of the call. Raise a limit only when the call does a constant
amount more (a new listing), never by the board's size.

### What's timed, `make perf`

`make perf` makes `cmd/boardgen`'s boards of 5,000 and 20,000 issues
once for a seed in `PERF_CACHE` (`~/.cache/git-board/perf`; 20,000 take
three minutes the first time), runs the CLI's and the server's benchmarks
on a copy of each with the cycle on (`PERF_COUNT` runs, 5), and
`tools/perfcheck` compares each median with
`testdata/perf/budgets.txt`: a name (`20k/cli/Ls`), a budget, and
why. It fails when a median is over its budget by more than
`PERF_TOLERANCE` (15%), or when a budgeted benchmark didn't run, and
writes `perf-results/report.md`. It takes about seven minutes.

| Benchmarks | |
|---|---|
| `surface/cli` | `Ls`, `LsFields`, `LsText`, `LsFirst` (no view), `LsTextFirst` (no index), `Show`, `Comment`, `Set`, `New` |
| `surface/http` | `ListOpen`, `ListColumn`, `ListAll`, `Facets`, `Thread`, `EventsFanOut` (a comment told to 50 pages), `WorkTeam`, `WorkTeamPage`, `WorkTeamFirst` |
| `web` (`make perf-web`) | `BoardLCP`, `BoardFCP`, `BoardTBT`, `IssueLCP`, `IssueFCP`, `IssueTBT`: Lighthouse's mobile run on the 5k board's page and an issue's, `PERF_WEB_RUNS` (3) each |

The `perf` workflow runs it nightly and on a pull request labelled
`perf`, and keeps `perf-results/` as an artifact; not on every push,
where a shared runner's noise would read as a regression. What a page
asks for, counted rather than timed, is held on every push by
`web/e2e/budget.spec.ts`: each page's API requests, none asked twice,
its files gzipped.

**A budget** moves down when the code gets faster. It moves up only with
the reason in its line (a feature that must cost more, a target found
wrong), in the commit that needs it, never to let a slower commit pass.
Where design.md's target isn't met yet (a first `ls`, a first free-text
`ls`, all 20,000 issues at once, a thread), the budget is what holds
now, and its line names the target.

### The Work page

`GET /api/cycle?who=team` took 71 s on the 20,000-issue board: each
open issue's `Show` (a listing, the counter, the sync records and the
people again for each), a `git grep` for its plan file at the base, and
a branch name the cycle never reads. Now the threads are read at once
(`issues.ThreadsOf`, one load), kept slimmed by version and by what
every thread shows (`issues.ThreadsMark`: the people, the sync records,
the counter), and each row by the viewer and the code's refs; the plan
files are one `git grep` a ref. 20,000 issues, 12,000 open:

| | Before | After |
|---|---|---|
| first request | 71 s | 2.7 s (the server reads it in the background after the index) |
| again, every row (7 MB) | 71 s | 230 ms |
| again, the first page (`limit=100`) | | 190 ms |
