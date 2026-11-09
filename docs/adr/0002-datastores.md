# ADR 0002: Where the board is kept

- Status: accepted; merged 2026-11-09 from ADR 0002 (decided 2026-11-03),
  ADR 0009 (2026-10-24), ADR 0015 (before 2026-10-20) and ADR 0016
  (before 2026-10-20)
- Date: 2026-10-24, 2026-11-03
- Absorbs [ADR 0009](0009-queries-read-a-disposable-index.md) (a
  disposable index), [ADR 0015](0015-attachments-on-s3-not-in-git.md)
  (attachments on S3) and
  [ADR 0016](0016-project-numbers-are-a-counter-on-origin.md) (numbers
  from a counter on origin).
- Formats: [Storage](../spec/storage.md) and
  [Other datastores and the counter](../spec/datastores.md).

## Context

A team chooses where its board is kept: in the git repository, in a
PostgreSQL database, in a folder of Markdown files, or on its forges
alone. Every id in the board comes from the bytes of its operations
([ADR 0004](0004-entities-are-operation-logs.md)). A datastore that hands
out its own ids breaks every link, every sync record and every marker on
a forge. Lists and searches need to be fast on thousands of issues.
Comments carry files. People say an issue's number aloud.

## Decision

### 1. A datastore keeps packs, and only packs

A pack is the JSON blob of one or more operations, byte for byte. Every
datastore implements one interface, `store.Datastore` (`store/store.go`):
`Heads`, `Load`, `Add`, `Caps`, `Folder` and `Close`.

- The kernel computes a pack's hash, the git blob id of its bytes, with
  no git. So a board keeps its ids in every datastore, and moves between
  datastores by a copy of its packs.
- The kernel orders the packs by (clock, hash). A datastore hands them
  back in any order.
- A write is a compare-and-swap on the entity's version. On
  `store.ErrMoved`, the writer reads again.

### 2. What every datastore gives

- the packs, unchanged, of the four kinds: `issue`, `person`, `sync` and
  `release`
- a compare-and-swap on each entity's version
- a local folder (`Folder`), this machine's own, for the view, the sync
  lock, its journal and the caches. For a git board, it is
  `<common dir>/board`.

The sync bookkeeping and the people are entities like the issues, so a
datastore that keeps packs keeps all of them with no code of its own.

### 3. What a datastore can give up, and how a feature says so

Each capability beyond section 2 is a field of `store.Caps`: a
datastore without it leaves the field nil. A feature reads the field,
never the datastore's name. The table of capabilities is in
[Other datastores](../spec/datastores.md#capabilities).

A missing capability is one error, `store.ErrMissing`, wrapped in the
sentence the user reads. The CLI prints it, and the server answers it as
a refusal.

The code repository is not the datastore. Branches, diffs, tags and the
proof notes stay in git, whatever keeps the board.

### 4. The forges alone keep no board of their own

`board.datastore forge` keeps the board on the forges. The board is the
only truth ([ADR 0008](0008-sync-state-in-the-data-model.md)), so this
datastore keeps no board of its own. It keeps a disposable copy of what
this machine read of the forges, in `<folder>/forge-copy.git`
(`store/forgecopy`). The sync fills it, and a write goes to the forge at
once.

- Everything in the copy came from a forge, so a copy that is lost costs
  one full pull, and makes no duplicate.
- A marker from sync's accounts that names an unknown id is pulled as
  new. That id has nowhere to arrive from.
- It has no counter: the forges number the issues.
- A board that also syncs to a second forge needs a store of its own:
  git or PostgreSQL.

### 5. Markdown is a datastore, and the files are a view

`board.datastore markdown:<dir>` keeps the packs as JSON lines in
`<dir>/.board/`, committed with the code and merged by git's union
driver. After each write, git-board renders one Markdown file per
issue, and reads hand edits back in before the next write. The files are
a view, never the record. `git board export --md` writes the same files
from any datastore, and `git board import --md` reads them back.

### 6. Queries read a disposable view

Lists, filters and search read a local view built from the entities
(`view/`). It holds nothing that the entities do not hold, and can be
deleted at any time.

- It is a sqlite file, `<folder>/v2/view.db`, with `modernc.org/sqlite`
  in pure Go, so `CGO_ENABLED=0` builds keep working.
- A board in PostgreSQL keeps the view in its own database, in a schema
  per view version (`view.PGSchema`).
- A file of an older version is deleted and built again, never migrated.
- Each query first catches up with the heads that moved.

### 7. Attachments go to S3, not into the board

A file in a comment goes to an S3 bucket that a CDN serves. The comment
gets a plain Markdown link (`tools/uploads`).

- The keys and the S3 service are the server's environment:
  `ACCESS_KEY_ID`, `ACCESS_KEY_SECRET`, `BUCKET_NAME` and
  `UPLOADS_ENDPOINT`. Without the keys, attaching is off.
- A request signs with AWS Signature V4 in the standard library, with no
  SDK.
- The bucket is public but not browsable. A random part in each file's
  key is the only protection.

A file stays out of the datastore. A large file in the board goes into
every clone, every backup or the working tree, and no forge shows it.
The team owns its files through the S3 service it chooses.

### 8. Project numbers come from one counter on origin

A project numbers its issues itself, `<PREFIX>-<n>` (`CO2-3075`).

- On git, the counter is `refs/board/seq`, a chain of commits that is
  not an entity. Only `origin` hands numbers out (`app/numbers.go`).
- A number is taken by a fetch, a new commit and an unforced push. A
  refused push means that someone else took a number: fetch and try
  again.
- Only issues that `origin` already has get a number. `git board sync`,
  `git board numbers` and Settings › Numbers number them.
- Below `start`, an issue keeps its GitHub number: `#2518` is
  `CO2-2518`.
- PostgreSQL keeps the counter in a row. A Markdown folder keeps it in
  `.board/numbers`, and writes it on the default branch only.

## Consequences

Good:

- A board keeps its ids in every datastore, and the markers on forges
  stay true after a copy.
- Sync works unchanged on every datastore, because its records are
  operations.
- Deleting the view loses nothing. A schema change costs one rebuild.
- Numbers are short, ordered, independent of the forges, and need no
  server.

Bad, and accepted:

- A datastore stores bytes that it cannot query. Queries go through the
  view.
- Two SQL dialects for the view: `make test-pg` runs its tests on
  PostgreSQL, outside `make ci`.
- Files are public to anyone with the link, even for a private
  repository, and stay on the CDN for up to a year after a delete.
- An issue has no number until `origin` has it. Offline, nothing gets a
  number.
- `#N` is a forge's number and `PREFIX-N` the project's: two notations to
  learn.
