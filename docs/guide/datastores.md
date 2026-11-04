# Where the board is kept

Your issues, people, sync records and release records are the board. By
default git-board keeps them in git, beside your code, under
`refs/board/*`. A clone can keep them elsewhere instead: in a PostgreSQL
database, in a folder of Markdown files, or on the forges alone. Your code stays in git whatever you
choose. [How the data flows](../spec/data-flow.md) shows the layers.

## Choosing

| | Git (default) | PostgreSQL | Markdown files | The forges alone |
|---|---|---|---|---|
| Who sees a write | a clone, until it pushes | everyone, at once | whoever has the commit | everyone, once it reaches the forge |
| Offline work | yes | only with a local server | yes | no |
| `git board pull`, `push`, start over | yes | no: nothing to move | no: commit and push the folder | no |
| Project numbers (`CO2-3075`) | yes, `refs/board/seq` on origin | yes, a row in the database | yes, `.board/numbers`, on the default branch | the forges' numbers only |
| Forge sync | yes | yes, the same | yes, the same | yes: it is the store |
| Lists and search | a sqlite file per clone | one index on the server | a sqlite file per clone | a sqlite file per clone |
| Read and edit by hand | no | no | yes, a file per issue | on the forge |
| History of an issue | every operation | every operation | every operation | what the pulls saw |

Pick **PostgreSQL** when a team wants one shared board with no pull or
push, or a board that people who never clone can reach. Pick **Markdown
files** when the board should go with the code: branches, pull requests
and reviews, readable without git-board. Pick **the forges
alone** when GitHub or GitLab is the record and git-board is a way to work
on it.

## Settings › Datastore

In the web UI, **Settings › Datastore** shows where the board is kept now
and what that datastore can do.

1. Choose **PostgreSQL** and paste a URL:
   `postgres://user:password@host:5432/database`; or **Markdown files**
   and a folder of the checkout, `issues`.
2. **Test connection** (**Look in the folder**) writes nothing. It says
   the server's version, and whether a board is there already.
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
  `git_board_index_v<version>` (lists and search, one schema per version
  of its tables, so two git-board versions can share the database; an
  older one's is left for you to drop once nothing reads it, as is
  `git_board_index`, older builds'). The user needs the right to create
  schemas in that database the first time, and to read and write them
  after.
- **One database is one board.** Give each project its own database.
- **Who you write as** is this clone's, `board.me` in its git config, as
  for any board. Choose it in Settings › You.
- **Numbers.** Settings › Numbers starts them as on a git board; the
  counter is a row in the database, so numbering needs no push, and a sync
  numbers what has no number yet.
- **One sync at a time.** A forge sync holds the board's lock in the
  database: a second one, from any machine, is refused while it runs
  ("another sync runs"), and a sync killed midway holds nothing.
- **Backups** are the database's: `pg_dump` the two schemas.
- **Tests.** `make test-pg` runs the PostgreSQL tests against
  `GIT_BOARD_TEST_POSTGRES`, a DSN whose user may create databases.

## Markdown files

```sh
git config board.datastore markdown:issues
git board datastore copy     # the board in git, into issues/
```

The folder, from the checkout's top, holds one file per issue,
`NNN-slug.md`, as `git board export --md` writes it, and the board itself
in `issues/.board/`: each issue's, person's and record's operations, one
line per write, in `<kind>/<id>.jsonl`. Commit both with the code. A
folder outside the checkout (an absolute path, or one up with `..`) is
refused.

- **The files follow the board.** After each write here, a forge sync's
  too, every file whose issue changed is written again.
- **Edit a file by hand**: the title, the description, `state`, the fields,
  labels and assignees, a comment's text; or add a comment under a heading
  `### Your name · 2026-11-04T10:00:00Z`. The next write reads your edits
  in, as you, and so does `git board import`. A new file with a title and
  no `id` is a new issue. An issue changed here after the file's `updated`
  keeps its own values: the edit is reported as a conflict, and the file
  is written again from the board.
- **A heading in your text is safe.** The line `<!-- git-board comments -->`
  ends the description, and each comment ends at its marker, so a
  `### …` line inside either stays text.
- **Branches merge their boards.** `.board/.gitattributes` merges the
  `.jsonl` files with git's union driver: two branches' writes to one
  issue both stay, in the order every clone agrees on. A conflict in a
  `.md` file needs no care: take either side, the next write renders it
  again.
- **A server and a command at once** take turns: each write reads the
  hand edits in, writes and renders under `.board/board.lock`, so no edit
  is read in twice. `.board/.gitignore` keeps the lock files out of git.
- **Numbers are handed out on the default branch.** Settings › Numbers
  (or `git board numbers init`) starts them as on a git board; the counter
  is `issues/.board/numbers`, the file `refs/board/seq` holds, committed
  with the rest. Numbering (Number them now, a sync, `git board numbers`)
  writes it only while the checkout is on the default branch: origin's
  `HEAD`, else `main`, else `master`. On another branch the counter is
  read, and numbering says why it waits; the branch's issues take their
  numbers once it is merged and someone numbers on the default branch. A
  file is named by its issue's number, else by the forges' number, else
  by the id's first 7 digits.
- **The counter never merges line by line.** `.board/.gitattributes` gives
  `/numbers` git's binary driver: only the default branch writes it, so a
  merge from a branch takes the default branch's. Two clones that numbered
  on the default branch at once conflict on it when one pulls: take
  theirs (`git checkout --theirs issues/.board/numbers`), commit, and
  number again; your issues take the next numbers. The union driver would
  keep one number twice, and a text merge would write conflict markers in
  the file.

## Importing Markdown

On any board, `git board import --md <dir>` reads a folder that
`git board export --md <dir>` wrote, or files written the same way. A
file whose `id` is an issue here gives it your edits; a file with no
`id`, or an id this board lacks, is a new issue with the author, comments
and times it names, and its new id is written back into it. People are
found by name or login, and made when there is none. Other Markdown files
in the folder are left alone.

```sh
git board export --md issues
$EDITOR issues/0042-export-csv.md
git board import --md issues
```

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
- **The copy is disposable.** `git board datastore drop` throws its
  issues away, and the next sync pulls them back; the people and the
  release records, which no forge keeps, stay. It refuses while an issue
  has a write made here since the last whole sync (`git board sync`);
  `--force` throws those away too.
- **What comes back** is what the forges show: an issue or a comment
  git-board posted comes back with the line it was posted with
  (*From Pierre, 2026-11-04, in git-board.*), as anyone's text is.
- **No project numbers** of git-board's own: the issues go by the forges'
  numbers.
- A board that syncs with a second forge has a store of its own to keep:
  choose git or PostgreSQL then.
