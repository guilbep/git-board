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

The `perf` workflow runs it nightly and on a pull request labelled
`perf`, and keeps `perf-results/` as an artifact; not on every push,
where a shared runner's noise would read as a regression. The web app's
first paint is measured by hand (web.md), not here yet.

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
