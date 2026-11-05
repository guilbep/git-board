# v2: the golden boards and `make compat`

The design's milestone M0 ([v2: the design of a rewrite](design.md),
section 5): the contracts v2 is held to, as files. v1 made them; v2 must
read the same boards and print the same outputs, byte for byte, but for
the differences [allowed](#allowed-differences).

| What | Where | Made by |
|---|---|---|
| The golden boards | `next/testdata/golden/boards/<board>/` | `make golden-boards`: `next/golden/v1gen`, with v1's code |
| v1's outputs on them | `next/testdata/golden/cli/<board>/` | `make golden`: v1's binary |
| The HTTP API's transcripts | `next/testdata/golden/api/` | `make golden`: `internal/server`'s `TestAPITranscripts` |
| The differences allowed | `next/testdata/compat/allowed.txt` | by hand, each with its reason |

The formats the boards are written in are the spec's: [Data
model](../spec/data-model.md), [Storage](../spec/storage.md), and [Other
datastores and the counter](../spec/datastores.md).

## Running it

```sh
make compat                                   # v1's git-board, built now, against its own outputs
make compat BIN=$(go env GOPATH)/bin/git-board-next
make golden                                   # record v1's outputs again, and the API's
make golden-boards                            # make the boards again, then make golden
```

`make compat` is part of `make ci`: about 3 seconds, the binary's build
aside. It runs `next/golden`'s `TestCompat` with `GIT_BOARD_COMPAT_BIN`
set to the binary, and `TestGoldensComplete`, which fails when a board's
cases and the recorded ones differ (a board changed without `make
golden`). `go test ./...` runs the API's transcripts against v1 on every
run, and `TestGoldensComplete`; `TestCompat` skips without a binary.

`make golden` rewrites the outputs: review the diff. `make
golden-boards` writes every board again; their ids stay, as their packs
are written with fixed nonces and times, except the `generated` board's,
whose nonces boardgen draws at random, and the `merge` board's merge
commits, which v1's merge stamps with the time (no output shows them).

## The boards

Each board is a folder: `board.bundle`, every ref of a git repository
(`git bundle create --branches --tags --glob=refs/board/*`), or `files/`,
the Markdown board's folder, or `tables.txt`, the PostgreSQL board's rows; `git-board.toml`, its `.git-board.toml`;
`me`, its `board.me`; `datastore`, its `board.datastore` when it isn't
git; `commands`, the cases recorded beyond the automatic ones, one command
line each.

| Board | What it holds |
|---|---|
| `basic` | Five persons: the first made with no author; a renamed one (`set-name`, a login renamed by `add-account`); one whose email was cleared and account removed (and a remove of one it never had); one in Unicode; one made by a pull; a second person holding the first's GitHub account. Nine issues: every issue operation on one (comments, edits of the body and of a comment, twice, a title, labels and fields, close then reopen, `set-meta` on a comment and on the issue); a closed one whose title ends with its own forge number; a deleted one (ADR 0019); a deleted comment edited and deleted again after; Unicode, combining marks, ZWJ, RTL, tabs, CRLF, HTML; one pulled from GitHub (origins), a forge comment pulled twice (one comment, two ids) and edited through its second id; lenient packs (pretty JSON, unknown fields at every level, keys in capitals, a key twice, `clock` last); one from GitLab with nothing but a title; label churn. Project numbers (`refs/board/seq`, a line a later version may write, a GitHub number below `start`). Two forges' sync entities: links, a link to a `javascript:` URL, a second link of one issue, a link to an issue not there, `seen` (one of a field that isn't one), `posted`, `refused`, `run`. Two releases on tagged commits of `main`, notes, a comment, a yank. `refs/board/issue/notes`, a ref that isn't an id. Flat kinds (`[branches] kinds`), a role every issue inherits, sprints, a view |
| `merge` | Two clones that wrote one issue offline (each its comment, body, title and labels), merged by v1's merge both at once, then one merging the other's merge (storage.md, *Known point*); a sync entity made on both clones, so two first commits; an issue appended on one clone alone (a fast-forward) and one created on it alone |
| `broken` | One good issue, and issues no reader folds: an edit of an operation that isn't there, an unknown operation, a pack that isn't JSON, a wrong clock, a log that doesn't start with `create`, an operation with no author, a status that isn't one |
| `generated` | `boardgen`'s board (15 issues, 5 persons, seed 7), as v1 writes a board of its own, and an issue with a long history: 160 packs of comments, edits, label changes, titles, closes and deletes |
| `markdown` | A Markdown folder board (`markdown:issues`), copied from git by v1's `datastore copy` with its counter, then one issue written on two branches and merged by git's union driver, then rendered by v1: a description holding a `### ` line, a deleted issue, Unicode |
| `postgres` | `basic`, copied into PostgreSQL by v1's `datastore copy` with its counter, kept as its tables' rows (`tables.txt`, bytes in hex). Each checkout loads them into a database of its own on the server `GIT_BOARD_TEST_POSTGRES` names, and drops it after; without one, the board is skipped (`make golden-boards` leaves it as it is). v1 prints on it what it prints on `basic`, file for file |

## The cases

On each board, in a checkout of its own: `ls --json`; `show <id> --json`
and `log <id> --json` for every issue, the deleted ones too; then the
board's `commands`. They cover filters (fields, negation, `is:`,
`assignee:@me`, `author:`, `involves:`, `label:`, `no:`, `title:`, free
text, a phrase, `in:`, `#n` and project numbers, `comments:`, a number
field, `created:`, `updated:`, `closed:` with dates, times and ranges),
`@today` and `@current` through `--at` (the board as it was then, which
fixes today), `--sort`, `--limit`, `--json` with fields, `--jq`, `view`,
`show --at`, `show --no-comments`, `show --md`, `log --field`, `log
--since`, `labels`, `me --ls`, and errors: a flag that looks like a
negation, a field nothing knows, a date that isn't one, an issue not
there or not created yet, each with its exit code and its message.

`next/testdata/golden/cli/<board>/index.txt` lists them: a name, the exit
code, the command line. `<name>.out` is its stdout, `<name>.err` its
stderr, each when not empty. `make compat` runs the command lines of the
index and compares the three.

Each command runs with: `HOME` a folder of its own, no system git
config, `TZ=UTC`, `LC_ALL=C`, `TERM=dumb`, `NO_COLOR=1`,
`GIT_BOARD_NO_PROMPT=1`, no stdin, no `GIT_*`, `XDG_*` or forge token
from the caller's environment; the checkout has no remote.

## Normalisations

What differs from run to run is taken out, and nothing else
(`next/golden/golden.go`, `Normalise`):

1. **Paths.** The checkout's path is `$BOARD`, the home's `$HOME`, each
   as given and with its links resolved.
2. **Log stamps.** A stderr line's `2006/01/02 15:04:05 ` prefix (Go's
   `log` package) is taken off.

No more were needed: v1 prints no "time ago" in JSON, and the times it
prints are the operations', in UTC. Today's date reaches no recorded
output: `@today` and `@current` are only recorded under `--at`, and no
case prints the inbox, `status`, insights or sprints, which read the
clock.

The API's transcripts, which write, normalise what a write makes now
(`internal/server/transcript_test.go`):

1. **New hashes.** A 40-digit hex hash that isn't an object nor a ref name
   of the board before the run is `<newN>`, numbered in the order the
   answers first name it; a request takes `<newN>` back as that hash.
2. **Now.** An RFC 3339 time from the run's start on is `<now>`, and Unix
   seconds from it on `<now-unix>`.
3. **The time zone** is UTC (`time.Local`), and the requests' host is
   `localhost:8080`, as the web app's page asks.

## The API's transcripts

`next/testdata/golden/api/requests.txt` lists the requests, in order, on
one checkout of `basic`: lists with filters and groups, facets, an issue,
threads (a deleted issue's, Unicode, a pulled one, lenient packs), the
config's fields and labels, views, colors, me, releases; then writes:
a comment, its edit, a stale edit (409 `changed`), labels, a value no
option allows, a `PATCH`, a title, a status and a wrong one, a new issue
and one without a title; then the thread and the list again, and errors
(no such issue, a body that isn't JSON). Each answer is
`NN-<method>-<path>.txt`: the request line, then the status, the
`Content-Type` and the body as sent.

## Allowed differences

`next/testdata/compat/allowed.txt`, one a line:

```text
<board> <case> <what> <reason>
```

`board` is a board's name or `*`; `case` a case's name from its
`index.txt`, a prefix ending in `*`, or `*`; `what` is `exit`, `stdout`,
`stderr` or `*`. The reason is required. Today the file allows nothing:
v1 against itself is the same, byte for byte.

## Pending differences

`next/testdata/compat/pending.txt`, one a line:

```text
<board> <case> <what> <milestone> <reason>
```

The differences `git-board-next` has for now, each closed by a later
milestone (design.md, section 5): forges from sync records (M4), project
numbers and the commands that write (M2), the Markdown datastore (M3,
closed).
`make compat BIN=<git-board-next>` holds the binary to them as well as
to `allowed.txt` (it sets `GIT_BOARD_COMPAT_PENDING=1`; v1, with no
`BIN`, has none). Unlike an allowed difference, a pending one is to go:
an entry no difference needs any more fails the run, "remove it from
pending", so the list only shrinks.

## The import guard

`next/guard_test.go`: nothing in `next/` imports `core/`, `internal/` or
`cmd/`, and nothing outside `next/` imports `next/`, so deleting v1 is
one `git rm`. One exception: `next/golden/v1gen` writes the boards with
v1's code, as boards written by anything else would prove nothing about
v1. It is a `main` package nothing imports, and it goes with v1 at the
switch; the boards it wrote stay, as files. `next/golden` itself knows
v1 only as a binary on its command line, and stays.
