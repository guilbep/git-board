# Performance

How fast git-board answers on a board of thousands of issues, how to
measure it, and what made it fast. The numbers below were taken on a
2-CPU Linux machine (Xeon, 2.1 GHz), on boards in git.

## A synthetic board

`cmd/boardgen` makes a board through the core, as git-board writes one:

```sh
go run ./cmd/boardgen -n 5000 /tmp/board5k
go run ./cmd/boardgen -n 5000 -pg postgres://me@localhost/bench /tmp/pg5k
```

- 25 people, each with a GitHub account.
- Each issue has a status, a prio, a category, an assignee and an area,
  half an estimate, a third a sprint; a body of a few paragraphs; up to
  six comments; an edit, a status move, sometimes a new title; a third
  are closed. Each change is its own pack, as a command writes it.
- The repository is packed, refs and objects, as a clone is. 5,000
  issues take half a minute to make; 20,000, three minutes.
- With `-pg`, the board is kept in that PostgreSQL database, and the
  repository's `board.datastore` names it.

## Measuring

The Go benchmarks run on `$GIT_BOARD_BENCH_REPO`, or on a board of 1,000
issues they make once:

```sh
export GIT_BOARD_BENCH_REPO=/tmp/board5k
go test ./internal/board -run - -bench . -benchtime 3x
go test ./internal/server -run - -bench API -benchtime 10x
```

| Benchmark | What it does |
|---|---|
| `CLIColdLs` | a command's `ls` with no index: it builds one |
| `CLILs/open`, `/fields`, `/text` | a command's `ls`: open the board, list, filter, close |
| `WarmLs/…` | the same on a board open already, as the server lists |
| `CLIShow` | a command's `show`: the thread and the issue |
| `CLINewSet` | `new`, then `set`, each in its own command |
| `Open` | opening the board and bringing the index up to date |
| `API//api/issues`, `/api/facets` | the server's lists, on a board open already |

`CLINewSet` adds issues to the board it runs on: run it on a copy. Add
`-cpuprofile cpu.out -memprofile mem.out` and read them with
`go tool pprof -top -cum`.

For the command line itself, build the binary outside the checkout and
time it in the board: `ls` with `.git/board/index.db` deleted is the
first one on a clone.

## The numbers

A command, wall time, 5,000 issues:

| | Before | After |
|---|---|---|
| first `ls` (no index) | 8.3 s | 3.9 s |
| `ls` | 8.2 s | 0.47 s |
| `ls "status:ready assignee:dev3"` | 8.7 s | 0.60 s |
| `ls "free text"` | 8.9 s | 0.73 s |
| `show <id>` | 113 ms | 82 ms |
| `new` | 132 ms | 95 ms |
| `set` | 88 ms | 91 ms |

20,000 issues:

| | Before | After |
|---|---|---|
| first `ls` (no index) | 74 s | 16 s |
| `ls` | 77 s | 1.5 s |
| `ls "status:ready assignee:dev3"` | 77 s | 1.3 s |
| `ls "free text"` | 282 s (it built the index) | 2.0 s |
| `show <id>` | 403 ms | 207 ms |
| `new` | 405 ms | 221 ms |
| `set` | 326 ms | 283 ms |

The benchmarks, 5,000 issues, time and memory an operation:

| Benchmark | Before | After |
|---|---|---|
| `CLILs/open` | 8.5 s, 4.0 GB | 339 ms, 152 MB |
| `CLILs/text` | 13.4 s, 5.4 GB | 391 ms, 178 MB |
| `CLIColdLs` | 8.8 s, 4.0 GB | 4.0 s, 503 MB |
| `WarmLs/open` | 48 ms | 41 ms |
| `WarmLs/text` | 135 ms | 128 ms |
| `CLIShow` | 121 ms, 68 MB | 80 ms, 37 MB |
| `CLINewSet` | 266 ms, 118 MB | 160 ms, 58 MB |
| `/api/issues` | 65 ms | 57 ms |
| `/api/facets` | 69 ms | 58 ms |
| filter's `Matches/text` (no index) | 61 ms, 121 MB | 11 ms, 4 MB |

On a PostgreSQL board of 5,000 issues, a command's `ls` went from 1.5 s
to 0.4 s (`CLILs`, the index built).

## What each fix did

- **An entity is read at the head the listing gave.** A listing listed
  the heads, then read each issue, and the git datastore read its ref
  again: go-git reads `packed-refs` line by line, so n refs cost n²
  lines. A git version is a commit, which never changes: `PacksAt`
  reads the packs from it, its ref unread.
- **The packs stay open.** go-git opened the pack file for each object
  it read, 75,000 times for 5,000 issues. A store keeps up to 8 open, and
  `Close` lets them go. Not on Windows, where an open file can't be
  deleted and `git gc` would fail.
- **A listing reads the index.** Every command read and folded every
  issue's packs. The index keeps each issue's state, its fold at the
  head it indexed (index version 3), and a listing reads those, decoding
  them on every CPU. The server decodes only the issues it doesn't hold
  at their head already.
- **The first build isn't quadratic.** Writing an issue deleted its
  labels by issue, a scan of the labels table, before inserting, even for
  a new one. A new entity has nothing to delete, and labels have an index
  by issue. The build prepares its statements once, loads several
  entities at once, and hands what it folded to the listing.
- **Free text is tokenised once.** The search's rules moved to
  `core/search`, so the filter no longer pulls in the index (go-git,
  sqlite). Matching an issue's own texts cut the query's words and each
  text once, in place of joining and splitting them for each word.
- **No regexp where bytes do.** An id is 40 hex digits; a title that
  doesn't end in `)` has no `(#2518)` to drop.
- **The server's lists are answered again** while no issue's head moved:
  the last listing is kept with the exact heads it was read at.

Every cache here is keyed by what it was read at, a head or a set of
heads, and is read again when they move: none trusts a time.

## What's left

- **The first build** reads every issue's git objects through go-git,
  one at a time under the store's lock: most of its 4 s on 5,000 issues.
- **A command's `ls` decodes every state**: the JSON of 5,000 issues is
  about 0.2 s of CPU.
- **Each query refreshes the index**: a listing of the refs and of the
  stored heads, about 25 ms on 5,000 issues; a free-text `ls` does it
  twice.
- **Free text on common words** reads every hit's summary where the
  filter needs its id alone.
- **People and the forges' links** are folded from their packs by every
  command. The synthetic board has no sync records; a board synced for
  years has long ones.
