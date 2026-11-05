# The golden boards and `make compat`

The design's milestone M0 ([v2: the design of a rewrite](design.md),
section 5): the contracts git-board is held to, as files. v1 made them,
and was deleted at the switch (v0.12, M7, [ADR
0021](../adr/0021-git-board-v2-same-data-new-code.md)); git-board must
read the same boards and print the same outputs, byte for byte, but for
the differences [allowed](#allowed-differences).

| What | Where | Made by |
|---|---|---|
| The golden boards | `testdata/golden/boards/<board>/` | v1's code (`golden/v1gen`, deleted at M7): files since |
| v1's outputs on them | `testdata/golden/cli/<board>/` | v1's binary, before M7 |
| The cross tests' transcripts | `testdata/golden/cross/<test>/` | v1's binary, before M7 ([below](#the-cross-tests)) |
| The differences allowed | `testdata/compat/allowed.txt` | by hand, each with its reason |

The formats the boards are written in are the spec's: [Data
model](../spec/data-model.md), [Storage](../spec/storage.md), and [Other
datastores and the counter](../spec/datastores.md).

## Running it

```sh
make compat                                   # git-board, built now, against v1's outputs
make compat BIN=$(go env GOPATH)/bin/git-board
make golden                                   # record the outputs again, from git-board built now
```

`make compat` is part of `make ci`: about 3 seconds, the binary's build
aside. It runs `golden`'s `TestCompat` with `GIT_BOARD_COMPAT_BIN`
set to the binary, and `TestGoldensComplete`, which fails when a board's
cases and the recorded ones differ. `go test ./...` runs
`TestGoldensComplete` and the cross tests on every run; `TestCompat`
skips without a binary.

**Recording again.** Until the switch, `make golden` recorded v1's
outputs, and v2 was held to them. v1 is gone, so the recordings are now
the contract itself: `make golden` records them again from git-board
built now (`TestCompat` and the cross tests, with
`GIT_BOARD_GOLDEN_UPDATE=1`), and a recording that changes is a change of
the contract. Run it only for a difference meant, review the diff (`git
diff testdata/golden`), and say why in the commit; a difference that is
only allowed, not meant, goes to `allowed.txt` instead. The boards are
not made again: they are files v1 wrote, and a board written by
git-board would prove nothing about v1's.

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
| `releases` | `basic`, with a `[release]` table that publishes binaries, `[branches] base = "main"`, and an `origin` file: its checkout is its own origin, fetched, so the release commands read its two releases (notes, a comment, a yank) as they do on a clone. Its commands are the release commands' reads and their refusals |
| `postgres` | `basic`, copied into PostgreSQL by v1's `datastore copy` with its counter, kept as its tables' rows (`tables.txt`, bytes in hex). Each checkout loads them into a database of its own on the server `GIT_BOARD_TEST_POSTGRES` names, and drops it after; without one, the board is skipped. v1 printed on it what it printed on `basic`, file for file |

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

`testdata/golden/cli/<board>/index.txt` lists them: a name, the exit
code, the command line. `<name>.out` is its stdout, `<name>.err` its
stderr, each when not empty. `make compat` runs the command lines of the
index and compares the three.

Each command runs with: `HOME` a folder of its own, no system git
config, `TZ=UTC`, `LC_ALL=C`, `TERM=dumb`, `NO_COLOR=1`,
`GIT_BOARD_NO_PROMPT=1`, no stdin, no `GIT_*`, `XDG_*` or forge token
from the caller's environment; the checkout has no remote, but on a board whose folder holds `origin`.

## Normalisations

What differs from run to run is taken out, and nothing else
(`golden/golden.go`, `Normalise`):

1. **Paths.** The checkout's path is `$BOARD`, the home's `$HOME`, each
   as given and with its links resolved.
2. **Log stamps.** A stderr line's `2006/01/02 15:04:05 ` prefix (Go's
   `log` package) is taken off.

No more were needed: v1 prints no "time ago" in JSON, and the times it
prints are the operations', in UTC. Today's date reaches no recorded
output: `@today` and `@current` are only recorded under `--at`, and no
case prints the inbox, `status`, insights or sprints, which read the
clock.

## Allowed differences

`testdata/compat/allowed.txt`, one a line:

```text
<board> <case> <what> <reason>
```

`board` is a board's name or `*`; `case` a case's name from its
`index.txt`, a prefix ending in `*`, or `*`; `what` is `exit`, `stdout`,
`stderr` or `*`. The reason is required. Today the file allows nothing:
git-board prints what v1 printed, byte for byte.

## The cross tests

Until the switch the cross tests (`golden`'s `TestCross*`,
`TestMarkdownHandEdits`, `TestMarkdownCopy`) ran v1 and v2 side by side:
the writes on each datastore, `init`, `lint`, the release record's
writes, `export` and `import`, `reset --hard`, `push --prune` and
`datastore drop`, a fresh clone's first pull, a Markdown folder's hand
edits and its copy from git. Before v1 was deleted its side was recorded,
from v1 built at its last commit, in `testdata/golden/cross/<test>/`, in
the form of `cli/<board>/`: each step a case, a file left behind a case
whose stdout it is. Each test now runs git-board alone, normalises as it
did (new ids, times of now, a database's URL, the line v2 says when it
upgrades `.git-board.toml`'s schema) and is held to the recording. What
could only compare two binaries on one board went with v1: v1 reading
v2's writes, and the reverse. Two scenarios hold git-board to itself
instead: a fresh clone's first `ls` is origin's, and a Markdown folder
rendered one write at a time is the folder rendered from none.

## Dogfooding

`make dogfood DIR=<repository>` (design.md, section 5) holds two binaries
to each other on a real board rather than to recorded outputs: v1
(`OLD`, or built from `OLD_REF`, by default `V1_REF`: v1's last commit,
`7a546cc`, the merge base of the switch with main) and git-board (`NEW`,
else built now), `golden`'s `TestDogfood`. Each reads its own copy of the
board (a clone with its `refs/board/*` and no remote), through this
page's `Run`, `Env` and `Normalise`; the cases are `ls --json`, `ls
is:closed --json` and `show <id> --json` of every issue in
`refs/board/issue/`. Exit codes and stdout are compared; stderr is kept
beside, not compared (git-board words its errors anew). There is no
allowed list: a difference fails, and its outputs go to `DOGFOOD_OUT`.
`.github/workflows/dogfood.yml` runs it daily on this repository's
board. No v0.11 release is its baseline: none has `show`, nor `ls
--json`'s `closed_at` and `comments`.

## The layers

`layers_test.go`, at the module's root: each package imports only the
packages its row of the table names, the table has no cycle, and nothing
imports `contrib/` (modules of their own, which speak to git-board
through its CLI and HTTP API). Until the switch a guard kept v2 (in
`next/`) and v1 apart, so deleting v1 was one `git rm`; it went with v1.
