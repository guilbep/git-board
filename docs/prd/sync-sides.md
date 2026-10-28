# One verb per side

PRD, 2026-10-28, from the discussion of the same day. Decisions marked
**(user)** were taken by the user; those marked **(call)** are this
document's, open to change.

## Why

A sync moves the issues to three places, and they aren't alike:

- **The git remote** (origin) holds the board itself, in its own format.
  A pull merges every clone's logs without loss; a push is a git push.
- **The forges** (GitHub, GitLab issues) hold a translation of it, made
  both ways. A write there emails people, and a creation waits for consent.
- **The GitHub Project** holds field values, matched to labels, with
  conflicts of its own.

`git board sync` runs all three, and the command line can't run one side
alone, except git with `--git`. Two things follow:

- **There is no starting over.** `reset --hard` only makes sense against
  git, where the remote holds the same data: a forge holds a translation,
  not a copy to reset to.
- **The log stays cluttered.** `git log --all` shows git-bug's commits and
  git-board's (115 in this repository, signed `git-board`). New ones carry
  no identity since #65, and `git board prune` (built, 3c1649a) removes
  git-bug's data. git-board's old commits go only with the board that
  holds them.

**One noun per side, git's verbs on each. `sync` stays the run of all
three.**

## Decisions

| Topic | Taken |
| --- | --- |
| The sides | Three: the git remote, the forges, the GitHub Project. **(user)** |
| Verbs | git's: `fetch`, `pull`, `push`, and `sync` for both ways. Subcommands with spaces, not `sync:pull`. **(user)** |
| The git remote | At the top: `git board fetch`, `pull`, `push`, each taking a remote, `origin` by default. **(user)** |
| The forges | `git board forge pull`, `push`, `sync`. **(user)** |
| The Project | `git board project pull`, `push`, `sync`, `resolve`, `configure`: `bridge` renamed. **(user)** `bridge` stays one release as a deprecated alias, hidden from the help, with its old flags. **(call)** |
| `sync` | Unchanged: git pull, the forges, the Project, git push. `--git` stays, as `pull` then `push`. **(call)** |
| Dry runs | Every new verb acts by default and takes `--check`, as `sync` does. `project` follows that rule, so it doesn't take `bridge`'s `--apply`. **(call)** |
| `reset --hard` | The git side only. `git board reset --hard` empties this clone's board; `git board reset --hard <remote>` makes it the remote's. **(user)** It writes a backup bundle first, and only `--hard` exists, with no soft or mixed form. **(call)** |
| `push --prune` | Deletes the remote's board refs that this clone doesn't have. **(user)** It fetches the remote's board and bundles it first, then pushes forced (M2). **(call)** |
| A clone left behind | Guarded. **(user)** Each reset writes a mark, and a pull or a push between two boards with different marks refuses, naming the command that settles it. A clone that missed a start-over would otherwise merge the old board into the refilled one: every issue twice, both copies linked to the same forge issue, pushed to origin for everyone. |
| What a reset keeps | The switch's mark, `refs/board/switched`: without it, a repository that still holds git-bug's data refuses every pull, forge sync and `prune`. The numbers' counter, `refs/board/seq`: the refilled issues are numbered after the old highest, so no number is handed out twice, and the prefix stays. `push --prune` keeps origin's copy of both, since this clone has them. **(call)** |
| `prune` | Removes git-bug's data after the switch. Built (3c1649a). **(user)** |
| What a reset loses | Whatever only the board held: the project numbers, the release records GitHub has no release for (7 of this repository's 17), `board.me` and the people. Accepted. **(user)** |
| The web page | Settings › Sync gets each side's actions, and a Start over section with the three resets, each explained, its command shown. **(user)** |
| One branch | One branch for all of it, with `make ci` once at the end, as the user prefers. **(user)** |

## The command line

### The git remote

- **`git board fetch [<remote>]`** brings the remote's board into
  `refs/board-remotes/<remote>/` and changes nothing else: the Inbox's
  fetch, `store.Backend.Fetch`. It prints what waits to come in and to go
  out, as the header counts it (`store.Remote`'s `issues` and `people`).
- **`git board pull [<remote>]`** fetches, then merges: `Backend.Pull`,
  then `FetchNumbers` for origin, as the whole sync's pull step
  (`pullStep`).
- **`git board push [<remote>]`** is the whole sync's push step
  (`pushStep`): the issues, the people and the switch's mark, then the
  numbering of the new issues. It isn't forced.
- **`git board push --prune [<remote>]`** fetches the remote's board,
  writes it whole to a bundle in the git directory and names it, then
  pushes and deletes on the remote every ref under
  `refs/board/` that this clone doesn't have, with git's own `--prune`
  (`git push --prune --force <remote> 'refs/board/*:refs/board/*'`).
  Checked on 2026-10-28: with no local ref left, it deletes them all. The
  push is forced, after the bundle of the remote's whole board (M2's
  ruling): a clone that started over can neither pull, which the reset's
  mark refuses, nor push over a ref the remote moved meanwhile. After the
  review: it refuses (`ErrNoStartOver`) unless this clone's reset's mark is
  one the remote hasn't, so that it never rewinds teammates' work outside a
  start-over, and it takes a numbers' counter the remote moved on rather
  than rewinding it, so that no number is handed out twice.

### Starting over

- **`git board reset --hard`** empties this clone's board:
  - it deletes `refs/board/*` and `refs/board-remotes/*`, but keeps the
    switch's mark and the numbers' counter (Decisions), and writes a new
    reset's mark (below);
  - it unsets `board.me`, whose person is gone with the refs.

  First it writes every ref it deletes to a bundle in the git directory,
  and names it: `git fetch <bundle> 'refs/*:refs/*'` brings them back.
  git-bug's data stays (that's `prune`'s), and so do `refs/notes/ci` and
  the code. The index, `.git/board/index.db`, drops the entities whose
  ref is gone at its next read.
- **`git board reset --hard <remote>`** makes this clone's board the
  remote's: it fetches, deletes the local refs the remote doesn't have,
  and sets each of the others to the remote's head, forced, the marks
  below included. It writes the same bundle first. Every other clone
  runs this after one clone ran `push --prune`.
- **The reset's mark** is `refs/board/reset`, a commit of nothing at the
  reset's time, as the switch's mark is. **(call)** `reset --hard` writes
  a new one, `push --prune` sends it, and `reset --hard <remote>` takes
  the remote's.
  - It travels as the switch's mark does: fetched to
    `refs/board-remotes/<remote>/reset` beside the entities.
  - **A pull and a push compare it** with the remote's. That covers
    `pull`, `push`, `fetch`'s counts, and the whole sync's git steps. When
    the two differ, a missing mark counting as one, they refuse and
    name the way out:
    - this clone missed a start-over: "origin's board was started over
      on <date>: `git board reset --hard origin` takes it";
    - origin wasn't pruned after this clone's reset: "this clone's board
      was started over on <date>, origin's wasn't: `git board push
      --prune` makes origin match, `git board reset --hard origin` goes
      back to it".
  - **Two exceptions.** `push --prune` and `reset --hard <remote>` are
    the ways to settle a difference, so they don't refuse. A clone with
    an empty board and no mark takes the remote's, as a new clone does.
  - **The guard's limit.** Versions of git-board from before the mark
    don't check it. The guide and the Start over cards say that every
    clone needs this version or a later one before anyone starts over.
- **Starting over everywhere** takes these steps:
  1. `reset --hard` here;
  2. `push --prune`;
  3. `reset --hard origin` on every other clone, with automated syncs
     stopped first;
  4. `git board forge pull`, which refills the board from the forges.
- **Refused while a server runs.** On the command line, both resets
  refuse while `git board serve` runs on the repository: a
  `git-board/server.json` that names a live pid, in the git directory of
  any of its worktrees, since they share `refs/board`. The web page's
  resets run inside the server instead.
- **What a forge pull brings back after a reset.** It brings every issue
  and comment that came from a forge. Not yet what git-board itself
  posted to a forge: that text carries git-board's marker, which names an
  id the empty board no longer has, and the pull skips it
  (`core/syncer/sync.go`, `byMarker`; `plan.go`, comments). In a
  repository where git-board never posted, nothing is missing; this
  repository is one (none of its 38 GitHub issues or 32 comments carries
  the marker). Adopting those ids is a follow-up, below. The help and
  the guide say so.

### The forges

**`git board forge pull`, `push` and `sync`** are the forge step alone,
`Backend.SyncForges`, one way or both: `syncer.Pull`, `syncer.Push`,
`syncer.Sync`. There's no git step. They take `--check` and
`--allow-create` as `sync` does, and print each forge's lines as `sync`
prints them. `push` is the web page's Send.

### The Project

**`git board project pull`, `push` and `sync`** cover the GitHub Project:
- `pull` sets the labels from the Project's fields: `bridge sync`'s plan
  restricted to its label changes, the mirror of `push`, so a field
  changed on both sides is held as a conflict, and the bases are
  recorded. **(call)** Today's `bridge pull` overwrites the labels and
  records no base; it stays only behind the `bridge` alias;
- `push` writes the labels to the Project, `bridge sync`'s plan restricted
  to its Project edits, and needs `write = true`;
- `sync` goes both ways, as `bridge sync` does.

`resolve` and `configure` move under `project` as they are.

## The web page

Settings › Sync keeps its table, a row per place, and its Check and Sync
for the whole. **(call)**

- **Each side's actions.** The origin row gets Fetch, Pull and Push.
  - **The forges** get Pull, Push and Sync, run as the forge sync job runs
    today (`/api/forges/sync`, its modes). A real run still needs a check
    of the last 15 minutes, and creations still wait for consent.
  - **The Project's line** gets Pull, Push and Sync.
  - **Every button says in its tooltip** which command does the same.
- **Start over**, below the table, is collapsed by default. It holds one
  card per action:
  1. **Empty this clone's board**, `reset --hard`.
  2. **Make origin match this clone**, `push --prune`.
  3. **Take origin's board**, `reset --hard origin`.
  4. **Remove git-bug's data**, `prune`, shown only while there is some.

  Each card says:
  - what it does, and what it loses;
  - when to use it, and what to run on the other clones;
  - the command in a terminal.

  Each asks for confirmation by typing the repository's name, as GitHub
  does for a deletion, and then says where the backup bundle is. Above
  the cards, the four steps for starting over everywhere, with the forge
  pull's limit in one line.
- **The server** gets an endpoint for each new action. Their shapes are
  left to the builder: they follow `/api/remotes/{name}/{action}` and
  `/api/forges/sync`, and a reset is refused while a sync job runs.

## Milestones

One branch, `make ci` once at the end.

- **M1, one verb per side.** The command line: `fetch`, `pull`, `push`,
  `forge …`, `project …`, the `bridge` alias, the help and the guide.
- **M2, starting over.** `reset --hard`, `reset --hard <remote>`,
  `push --prune`, the reset's mark and its guard, with `prune` already
  built, the help and the guide.
- **M3, the web page.** The server's endpoints and Settings › Sync.

## Not in this PRD

- **A forge pull that adopts git-board's markers**, so that a reset gives
  back what git-board posted, and a second forge's copies join again. It
  changes the core of sync and gets its own design.
- **Release records from GitHub's releases**, for the records a reset
  loses (10 of this repository's 17 have a GitHub release).
- **Rewriting git-board's old commits in place**, for a repository that
  keeps its board: dropped. Starting over replaces them.
- **Choosing forges on the command line** (`--forge`): the web page
  chooses them already.

## Left to the builder

- The endpoints' shapes, and how the components split.
- The wording of each explanation, in the page's voice.
- How `project push` restricts `bridge sync`'s plan.
