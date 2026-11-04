# Where the board is kept

Your issues, people, sync records and release records are the board. By
default git-board keeps them in git, beside your code, under
`refs/board/*`. A clone can keep them elsewhere instead: in a PostgreSQL
database, or on the forges alone. Your code stays in git whatever you
choose. [How the data flows](../spec/data-flow.md) shows the layers.

## Choosing

| | Git (default) | PostgreSQL | The forges alone |
|---|---|---|---|
| Who sees a write | a clone, until it pushes | everyone, at once | everyone, once it reaches the forge |
| Offline work | yes | only with a local server | no |
| `git board pull`, `push`, start over | yes | no: nothing to move | no |
| Project numbers (`CO2-3075`) | yes, `refs/board/seq` on origin | yes, a row in the database | the forges' numbers only |
| Forge sync | yes | yes, the same | yes: it is the store |
| Lists and search | a sqlite file per clone | one index on the server | a sqlite file per clone |
| History of an issue | every operation | every operation | what the pulls saw |

Pick **PostgreSQL** when a team wants one shared board with no pull or
push, or a board that people who never clone can reach. Pick **the forges
alone** when GitHub or GitLab is the record and git-board is a way to work
on it.

## Settings › Datastore

In the web UI, **Settings › Datastore** shows where the board is kept now
and what that datastore can do.

1. Choose **PostgreSQL** and paste a URL:
   `postgres://user:password@host:5432/database`.
2. **Test connection** connects and writes nothing. It says the server's
   version, and whether a board is there already.
3. **Copy the board from git** copies every issue, person and record this
   clone has in git into that database. Every id is kept, so the links and
   markers sync left on the forges stay true. A second copy writes only
   what is new; the git refs aren't touched.
4. **Save**, then restart `git board serve`. The tab says when a saved
   choice waits for a restart.

The URL is kept in this clone's git config (`board.datastore`), never in
`.git-board.toml`: it may hold a password. The page never shows the
password again.

## From a terminal

```sh
git config board.datastore postgres://me@db.example.org/board
git board datastore copy     # the board in git, into the database
git board datastore          # where the board is kept, and what it can do
```

`GIT_BOARD_DATASTORE` sets it too, over git config: for a server started
by a service manager, say. While it is set, the Settings tab can't change
the datastore.

Back to git: `git config --unset board.datastore`, or choose Git in the
tab. What was written in PostgreSQL meanwhile stays there: copy it back
by hand only if you need it (see *Left open* in ADR 0002).

## PostgreSQL

- **The tables** are made on first use, in two schemas:
  `git_board` (the board: `entities`, `packs`, `counter`) and
  `git_board_index` (lists and search). The user needs the right to create
  schemas in that database the first time, and to read and write them
  after.
- **One database is one board.** Give each project its own database.
- **Who you write as** is this machine's, in `.git/board/me`. Choose it
  in Settings › You, as for any board.
- **Numbers.** Settings › Numbers starts them as on a git board; the
  counter is a row in the database, so numbering needs no push, and a sync
  numbers what has no number yet.
- **Backups** are the database's: `pg_dump` the two schemas.
- **Tests.** `make test-pg` runs the PostgreSQL tests against
  `GIT_BOARD_TEST_POSTGRES`, a DSN whose user may create databases.

## The forges alone

```sh
git config board.datastore forge
```

The board lives on the forges with sync on (Settings › Sync). This clone
keeps a copy in `.git/board/forge-copy.git`, filled by each sync, and
nothing under `refs/board/*`.

- **Each write goes to the forges at once.** `git board serve` sends an
  issue's change as soon as you make it, and tries again every 30 seconds
  while a forge doesn't answer. Writes from the command line go with the
  next `git board sync`.
- **The copy is disposable.** `git board datastore drop` throws it away,
  and the next sync pulls the board back. It refuses while an issue isn't
  on a forge yet; `--force` throws those away too.
- **What comes back** is what the forges show: an issue or a comment
  git-board posted comes back with the line it was posted with
  (*From Pierre, 2026-11-04, in git-board.*), as anyone's text is.
- **No project numbers** of git-board's own: the issues go by the forges'
  numbers.
- A board that syncs with a second forge has a store of its own to keep:
  choose git or PostgreSQL then.
