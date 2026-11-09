# Other datastores and the counter

[Storage](storage.md) is the git datastore's format. A board may be kept
elsewhere: in PostgreSQL, or in a folder of Markdown files
([Where the board is kept](../guide/datastores.md)). This page is their
formats, and the format of the project numbers' counter, which every
datastore that counts keeps as the same bytes. It is precise enough to
reimplement them from: the code that wrote them (v0.11's `core/pgstore`,
`core/mdstore`, `internal/store/numbers.go` and `internal/store/markdown.go`,
whose boards are the golden boards git-board is held to, [compat](../rewrite/compat.md))
decides what this page should have said, and this page decides for any
code written after, as v0.12's `store/pg`, `store/md`, `app/origin.go`
and `domain/issues/mdfiles` were.

## What every datastore keeps

A datastore keeps, for each entity, its kind, its id and its packs, the
bytes [Packs](storage.md#packs) gives, and a version. Every datastore
reads and writes the same pack bytes, so a pack's hash, the ids of its
operations and the [order](storage.md#order) are the same everywhere:

- **A pack's hash** is the git object id of its bytes,
  `sha1("blob " + <length in decimal> + "\0" + <bytes>)`, in lowercase hex,
  whether or not git stores it.
- **The order** is the packs sorted by (clock, hash), each pack's
  operations in their stored order. A datastore hands its packs back in
  any order; the reader sorts them.
- **Two packs of the same bytes** are one pack.
- **A version** is a string the datastore gives with the packs it read,
  opaque to the reader. A write names the version it read, and lands only
  if the entity is still at it (the compare-and-swap of
  [Updating a ref](storage.md#updating-a-ref)); else it fails as moved,
  and the writer reads again. `""` names an entity that doesn't exist yet.

Outside git there are no commits, so there are no merge packs: a write
appends its pack to the entity, with a clock of one more than the highest
clock the writer read. Two writers that read the same version can't both
land.

A datastore's ids, kinds and the rules of [Entities](data-model.md#entities)
are git's: a kind is `issue`, `person`, `sync` or `release`; an id is 40
lowercase hex digits.

## Capabilities

What a datastore gives beyond the packs is a field of `store.Caps`
(`store/store.go`). A datastore without a capability leaves its field
nil, and a feature that needs it fails with `store.ErrMissing` and a
sentence ([ADR 0002](../adr/0002-datastores.md), section 3).

| Capability | What needs it | git | PostgreSQL | Markdown folder | Forges alone |
|---|---|---|---|---|---|
| `Pinned`: a version names its packs for good | loads with no recheck | yes | no | no | yes |
| `Shared`: every machine reads the same store | the view and the lock on the server | no | yes | no | no |
| `Counter`: a counter with compare-and-swap | project numbers | `refs/board/seq` | a row | `.board/numbers`, on the default branch | no |
| `Locker`: a lock that every machine shares | one forge sync at a time per board | no | an advisory lock | no | no |
| `Remote`: fetch, push, start over | git remotes, Sync's git part, Start over | yes | no | no | yes, its copy's |
| `SQL`: a database for the view | the view beside the board | no | yes | no | no |
| `Put`: takes another datastore's packs as they are | `git board datastore copy` | yes | yes | yes | yes |

A datastore without `Locker` takes the machine's lock, `<folder>/sync.lock`.
The journal of a sync run stays the machine's on every datastore.

The code repository gives the rest, whatever keeps the board: branches,
diffs, tags, and the proof notes. A git-board with no clone goes without
them, and so without the cycle's gates.

## PostgreSQL

A board kept in PostgreSQL is one database (`board.datastore` holds its
`postgres://` URL or libpq's `key=value` pairs). One database is one
board.

### The tables

They are in the schema `git_board`, made on first use:

```sql
CREATE SCHEMA IF NOT EXISTS git_board;
CREATE TABLE IF NOT EXISTS git_board.meta (key text PRIMARY KEY, value text NOT NULL);
CREATE TABLE IF NOT EXISTS git_board.entities (
	kind text NOT NULL, id text NOT NULL, version bigint NOT NULL,
	PRIMARY KEY (kind, id));
CREATE TABLE IF NOT EXISTS git_board.packs (
	kind text NOT NULL, id text NOT NULL, hash text NOT NULL, data bytea NOT NULL,
	PRIMARY KEY (kind, id, hash),
	FOREIGN KEY (kind, id) REFERENCES git_board.entities);
CREATE TABLE IF NOT EXISTS git_board.counter (
	one boolean PRIMARY KEY DEFAULT true CHECK (one), value bytea NOT NULL, version bigint NOT NULL);
```

- **`meta`** holds one row, `('version', '1')`: the tables' version. A
  reader that finds another refuses the database ("the tables are version
  N, this git-board reads 1"). Making the tables and the row runs in one
  transaction that first takes `pg_advisory_xact_lock(7340176)`, so two
  first uses don't make them at once.
- **`entities`** has a row per entity: its kind, its id, and its version,
  the number of writes it took, from 1. Its version as a string is that
  number in decimal: `"1"`, `"2"`…
- **`packs`** has a row per pack: its entity, its hash (40 lowercase hex,
  as above) and its bytes, as written. A pack written twice to one entity
  is one row.
- **`counter`** has at most one row: the [counter](#the-counter)'s bytes
  and its version, the number of writes it took, from 1. No row: the
  project doesn't number its issues.

The index's tables (lists and search) are in a schema of their own,
`git_board_index_v<n>`, and v2's view in `git_board_view_v1`. They are
disposable, rebuilt from the tables above, and no contract.

### Reading

- **The heads** of a kind are `SELECT id, version FROM git_board.entities
  WHERE kind = $1`.
- **An entity's packs** are read with its version in one statement, one
  snapshot: `SELECT e.version, p.data FROM git_board.entities e JOIN
  git_board.packs p USING (kind, id) WHERE e.kind = $1 AND e.id = $2`. No
  row: the entity doesn't exist.

### Writing

A write is one transaction:

1. **A new entity** (the version read is `""`): `INSERT INTO
   git_board.entities (kind, id, version) VALUES ($1, $2, 1) ON CONFLICT DO
   NOTHING RETURNING version`. **An existing one:** `UPDATE
   git_board.entities SET version = version + 1 WHERE kind = $1 AND id = $2
   AND version = $3 RETURNING version`, `$3` the version read.
2. No row returned, or a serialization failure (SQLSTATE `40001` or
   `40P01`), even at the commit: the entity moved.
3. `INSERT INTO git_board.packs (kind, id, hash, data) VALUES (...) ON
   CONFLICT DO NOTHING`.

The new version is the one the statement returned. The counter is written
the same way, on its one row: `INSERT ... VALUES ($1, 1) ON CONFLICT DO
NOTHING` when none was read, else `UPDATE ... SET value = $1, version =
version + 1 WHERE version = $2`.

### The lock

A datastore every machine shares has one lock every machine sees: a
forge sync takes it, so two machines never sync one board at once
([A run](sync.md#a-run)). In PostgreSQL a lock named `name` is a session
advisory lock, `pg_try_advisory_lock(key)`, where `key` is the 64-bit
FNV-1a hash of `git-board/<name>` read as a signed integer. The sync's
name is `sync`. It is held on its own connection until the run ends; the
server lets it go when the session ends, so a killed run keeps no one out.
A lock already held is refused at once, not waited for.

### Copying a board in

A copy from another datastore writes, entity by entity, each in one
transaction, the packs this database lacks, and moves the version by one
for each entity it wrote to. The counter is copied when this database has
none. Ids stay: they are the packs' hashes, the same bytes.

## Markdown folder

A board kept in a folder of the code checkout: `board.datastore` is
`markdown:<dir>`, `<dir>` relative to the checkout's top (`markdown:issues`).
The folder is committed with the code, so a board goes with its branch.
It holds two things:

- `<dir>/.board/`, the board: the packs, the counter, the locks. This is
  the datastore.
- `<dir>/NNN-slug.md`, one file per issue, rendered from the board after
  each write, and read back in before the next one (hand edits). They are
  a view, never the record.

```text
issues/
  .board/
    .gitattributes          *.jsonl merge=union, /numbers merge=binary
    .gitignore              *.lock, *.takeover
    board.lock              the folder's lock (not committed)
    numbers                 the counter
    issue/<id>.jsonl        an entity's packs, one a line
    person/<id>.jsonl
    sync/<id>.jsonl
    release/<id>.jsonl
  1-kept-in-a-folder.md     an issue, rendered
  2-another-one.md
```

### Pack files

`.board/<kind>/<id>.jsonl` is an entity: its packs, one per line, each the
pack's bytes followed by a line feed (`\n`). A file whose name isn't
`<40 hex>.jsonl`, in a kind's folder, isn't an entity and is ignored.

- **A pack is one line.** Its bytes hold no line feed nor carriage return:
  compact JSON escapes them in strings. A write of a pack that holds one is
  refused.
- **Reading** a file: its bytes, without a last line being appended (see
  below), split on `\n`; each line with its surrounding white space
  (spaces, tabs, `\r`) taken off is a pack; an empty line is skipped. The
  pack's hash is that of the line's bytes so trimmed.
- **A line twice** (a merge can leave one) is one pack, as two packs of the
  same bytes always are.
- **No packs** (no file, or one of white space alone): the entity doesn't
  exist.
- **A last line being appended.** A reader may see an append in parts. So
  when the bytes after the last `\n` aren't a valid JSON value, they are
  left out, as not written yet. A whole pack whose `\n` a hand edit took is
  valid JSON, and is read.
- **The version** is the first 16 bytes of the SHA-256 of the file's bytes
  (without a last line being appended), in lowercase hex: 32 digits. `""`
  for no packs. It changes whenever the file does, a branch switched to
  included, even when two branches' files hold as many packs.

**Merging.** `.board/.gitattributes` holds `*.jsonl merge=union`. Two
branches that both appended to an entity merge with both packs kept, ours
then theirs; the reader orders them by (clock, hash), as it orders the
packs of two clones. Their clocks may be equal: neither saw the other.

**Writing** a pack to `<id>.jsonl`, at the version read:

1. Take the entity's lock: create `<id>.jsonl.lock` exclusively
   (`O_CREAT|O_EXCL`). While another holds it, wait, trying every 5 ms.
   A lock file older than 30 seconds is a dead writer's, taken over: under
   an OS lock on `<id>.jsonl.lock.takeover`, its age is checked again and
   the file removed. After 30 seconds of waiting, the write fails as moved.
2. Read the file. Its version (as read above) must still be the one the
   writer read; else the write fails as moved.
3. When the bytes after the file's last `\n` aren't a valid JSON value (a
   last line being appended, as reading leaves it out), cut the file to
   its last `\n`, or to nothing when it holds none: under the lock no
   append is half done, so they are a dead writer's. Appended after them,
   the pack would join their line.
4. Append the pack and a `\n`, with one `\n` before them when the file
   doesn't end with one (a hand edit took it).
5. Remove the lock file. The new version is the file's, appended.

**The folder's files.** `.board/.gitattributes` and `.board/.gitignore`
are written when the folder is opened, if absent:

```text
*.jsonl merge=union
/numbers merge=binary
```

```text
*.lock
*.takeover
```

The `/numbers` line is added the first time the counter is written, to a
folder made before it.

**The folder's lock**, `.board/board.lock`: an OS lock (`flock` on Unix,
`LockFileEx` on Windows) on that file, which stays. A write holds it from
reading the hand edits in to the last file rendered, so a server and a
command never read an edit in twice nor render over each other. It waits
up to a minute. The OS lets it go when its holder dies.

### The heads, and the stat cache

A listing of the heads needs each file's version, and a version is a hash
of the whole file. At 5,000 issues, that is 34 MB to read on each
listing. So `store/md/heads.go` keeps each version in this machine's
folder, `<folder>/v2/md-<16 hex>.versions`, with the file's stat: its
size, mtime, ctime and inode. A file whose stat is the one kept is not
read again.

A version is kept only under two conditions:

- The file last changed (its ctime, or its mtime when later) a second or
  more before the read.
- Its stat is the same before and after the read.

A version must change whenever the file does. A coarse clock can give two
writes in one tick the same mtime, and `git checkout` writes files at the
time of the checkout. A file written again within that second has the
stat of the first write, so a version read then is not kept. A missed
change is an old page and an old view with nothing to tell, so the cache
trusts no recent file.

### The counter in a folder

`.board/numbers` holds the [counter](#the-counter)'s bytes. Its version is
a pack file's: the truncated SHA-256 of its bytes, `""` when absent or
white space alone. It is written whole: under `.board/numbers.lock` (taken
as a pack file's lock), the version checked, then the bytes written to
`numbers.new` and renamed over `numbers`.

It is written on the default branch alone: origin's `HEAD`
(`refs/remotes/origin/HEAD`), else `main`, else `master`; in a new
repository with none of these, on `main` or `master`. On any other branch,
or on no branch, numbering waits, and says why. A folder outside any git
checkout has no branch, and writes. `merge=binary` keeps a merge from
mixing two counters: a branch never writes it, so a merge takes the
default branch's; two writes on the default branch at once conflict.

### The rendered files

Each issue not deleted is a file `<dir>/<name>.md`; a deleted issue's file
is removed. They are written again after each write, where their bytes
changed.

- **The name** is `NNN-slug.md`. `NNN` is the issue's bare number. When
  the project numbers its issues, it is its project number without the
  prefix (`CO2-3075` is `3075`), and an issue without one yet has none.
  When the project doesn't, it is the number on its first forge that has
  one. An issue with no bare number takes the first 7 digits of its id. The slug is the title in lower
  case, each run of characters outside `a-z0-9` a `-`, `-` taken off both
  ends, cut at 50 bytes and `-` taken off its end again. No slug: `NNN.md`.
  Two issues of one name: the one whose id sorts later takes
  `-<first 7 digits of its id>` before `.md`.
- **A file of an issue under another name** (its title changed) is
  removed. A file that isn't one of these, or whose front matter has no id,
  is left alone.

A file is YAML front matter, then the description and the comments:

```text
---
id: 3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5
number: MD-1
title: Kept in a folder
state: open
fields:
  prio:
    - p1
  status:
    - ready
labels:
  - plain
people:
  author: Ada King
  assignees:
    - ada
created: 2025-03-03T09:00:00Z
updated: 2025-03-03T14:01:00Z
---

The description.

<!-- git-board comments -->

### Bob · 2025-03-03T10:00:00Z

A comment.

<!-- git-board comment 8c04e5b1d97f2a63c18e0b4d5f6a7928e3b1c0d4:0 -->
```

- **The front matter** is YAML, indented by two spaces, its keys in this
  order: `id`; `number`, the project number (`MD-1`), else the bare
  number above, left out when there is none; `title`; `state`, `open` or `closed`; `fields`, each label
  `ns:value` but `assignee:` under its namespace, and a flat kind (a
  `[branches] kinds` label, `bug`) under `kind`, left out when empty;
  `labels`, the plain labels, left out when none; `people`, its `author`
  (the person's name, else their login, else their id) and `assignees`
  (left out when none); `created` and `updated`, RFC 3339 in UTC, to the
  second. Lists keep the labels' order.
- **The body**: after `---`, the description trimmed of surrounding white
  space, between empty lines, when not empty; then an empty line and
  `<!-- git-board comments -->`, which ends the description, so a `### `
  line in it stays text.
- **Each comment**, in order: an empty line, `### <author> · <time>` (the
  name as above, its time in RFC 3339 UTC), an empty line, its text
  trimmed and an empty line when not empty, then
  `<!-- git-board comment <comment id> -->`. Deleted comments aren't
  written.

A write first reads the files in (as `git board import --md` does,
[Importing Markdown](../guide/datastores.md#importing-markdown)): what a
hand changed becomes the writer's operations, a file with a title and no
id a new issue. Then it writes, then renders.

## The counter

A project that numbers its issues ([ADR
0016](../adr/0016-project-numbers-are-a-counter-on-origin.md)) keeps one
counter: a text file, `numbers`, the same bytes in every datastore.

```text
prefix GB
start 100
100 26a4ba85e9ef4a8ff1b61f228dd598c8b3667905
101 398523d66eaebdebff64257a35e73c84d5c31f47
```

**Writing it**: `prefix <prefix>\n`, `start <start>\n`, then a line
`<n> <issue id>\n` per numbered issue, by number ascending, `n` in
decimal.

**Reading it**, line by line (split on `\n`), each line trimmed of
surrounding white space and cut at its first space into a key and a value:

- an empty line is skipped;
- `prefix`: the prefix, a letter then letters or digits
  (`^[A-Za-z][A-Za-z0-9]*$`);
- `start`: the first number handed out, a decimal integer, 1 at least;
- `github`: the project's github.com repository, `owner/repo`, whose
  numbers below `start` are its issues' own (v2 writes it when it starts
  the counter on a board with one github.com forge; v1 skips it); a value
  that isn't `owner/repo` is skipped;
- a key that is a decimal integer (Go's `strconv.Atoi`: an optional sign,
  then digits): that number is the value's issue. A number with no value
  is an error; a number given twice is an error;
- any other key is skipped: a later version may write it.

A counter without a valid prefix and start is an error: the issues still
list, without numbers, and numbering refuses. An issue id on two lines
takes the later line's number.

- **The next number** is the highest of `start` and every number + 1.
- **An issue's number** is its line's, `<prefix>-<n>`; else, below
  `start`, its number on the project's GitHub repository (a link of a
  sync entity whose URL is `https://github.com/<owner>/<repo>/…`):
  `#2518` is `CO2-2518`, the number it had before the project numbered
  its own; else none yet. The repository is the `github` line's; else the
  one `[forges.*]` table of kind `github` on `github.com`; else the one
  github.com repository the sync records link to. Two, and none named:
  no GitHub number is an issue's own, since two repositories each have
  a `#5`, and an issue linked on both would change number.
- **Numbering** gives the next numbers to the issues without one that
  every clone has, oldest first (by creation time, then id), in one write
  of the counter: on git the issues origin has, on a shared datastore all.

### In git

`refs/board/seq` is a chain of commits, each with one tree entry,
`numbers` (a blob, mode `100644`), holding the counter's bytes; its first
commit has no parent, each later one the commit it was written on. The
commits are written as an entity's (no name, no email, no message,
unsigned). It isn't an entity: fetch and push of the board never name it.

- **Only origin hands numbers out.** A mirror never does.
- **Fetching it**: `git fetch --no-write-fetch-head origin
  +refs/board/seq:refs/board-remotes/origin/seq`. A remote without it is no
  error: the project doesn't number its issues. This clone's counter takes
  origin's when it is origin's or one of its ancestors; ahead or apart, it
  is refused (its numbers aren't on origin yet).
- **Numbering** fetches it, writes the next commit on it, moves
  `refs/board/seq` (`git update-ref`, compare-and-swap), and pushes it
  unforced: `git push origin refs/board/seq:refs/board/seq`. A refused
  push means someone numbered first: fetch, and number again, five tries.
  When a push fails, origin's counter is fetched and says what happened:
  ours (it landed), another (refused), or the one written on (a failure;
  the local ref goes back to it).
- **Starting the numbers** is the counter's first commit, with no issue
  line, pushed: pushing it is the switch for everyone.

In PostgreSQL it is the `counter` row; in a Markdown folder,
`.board/numbers` ([above](#the-counter-in-a-folder)).
