# ADR 0009: Queries read a disposable index, never the source of truth

- Status: accepted (recorded 2026-11-04, decided earlier)
- Date: 2026-10-24 (`docs/prd/core.md` › The data model, in brief; built in
  M2, `docs/prd/core-m2-plan.md`). Amended 2026-11-04 by ADR 0002's As
  built: a PostgreSQL board keeps the index in PostgreSQL.
- Spec: `docs/spec/queries.md`.
- Alternatives below are reconstructed; the M2 plan records only the
  dependency choice.

## Context

The board is a set of operation logs (ADR 0004). Showing a list, a board
column or a search result means replaying every issue. The server, a
command and the TUI may run at once on the same clone, and several
worktrees share one repository. git-bug's own cache was one of the parts
the rewrite dropped.

## Decision

**Lists, filters and search read a local index built from the entities.
It holds nothing the entities don't, and can be deleted at any time.**

- **Where:** one sqlite file, `<git common dir>/board/index.db`, shared by
  every worktree (`core/index/index.go:72-95`; the folder is the
  datastore's `Folder()`, `core/gitstore/datastore.go:104`). Git never
  commits, fetches or pushes it.
- **Engine:** `modernc.org/sqlite`, pure Go, with FTS5, the one new
  dependency M2 allowed, so `CGO_ENABLED=0` builds keep working
  (`docs/prd/core-m2-plan.md:20`, `:28`).
- **Fresh on every query:** a refresh lists the heads, reloads entities
  whose head moved, drops those gone, in one transaction
  (`core/index/index.go:163` `Refresh`). Whoever moved a ref (this
  process, another, a fetch), the next query sees it.
- **Two processes:** WAL, `busy_timeout(5000)`, transactions that take the
  write lock at once (`core/index/index.go:104`;
  `core/index/index_test.go:175` `TestIndexTwoProcesses`).
- **Rebuilt, never migrated:** the schema version is in `PRAGMA
  user_version`; a missing, unreadable or older file is deleted and built
  again (`core/index/index.go:123-145`).
- **A broken entity never fails a query:** it is kept out and listed
  (`core/index/index.go:380` `Broken`).
- **Only issues and persons** are indexed; sync and release entities are
  read from their logs.
- **PostgreSQL boards** (ADR 0002): when the datastore implements
  `index.SQL`, the same tables live on the server, shared by every
  machine, in a schema of their version, `git_board_index_v<version>`
  (`index.PGSchema`; amended 2026-11-04, issue #92).

## Alternatives considered

Reconstructed. Scored on: correct after any ref move (30), fast lists and
search (25), no server, single static binary (25), simplicity (20).

| Option | Score | Why not |
| --- | --- | --- |
| **Disposable sqlite index, refreshed per query (this decision)** | 85 | |
| No index: replay every issue per request | 45 | Simple, but every list and search reads every log; slow on boards of thousands of issues |
| An in-memory cache per process | 55 | Rebuilt at every start of every command; nothing shared between the server and a CLI call |
| sqlite through cgo (`mattn/go-sqlite3`) | 50 | Breaks `CGO_ENABLED=0` and cross-builds for five platforms (ADR 0012) |
| A key-value store (bbolt, Badger) | 45 | No SQL and no full-text search; the queries would be written by hand |
| The index as source of truth, entities as export | 20 | Loses offline merge through git (ADR 0004) |

## Consequences

Good:

- Deleting `index.db` loses nothing.
- A schema change costs one rebuild, no migration code.
- Search, filters and sorting are SQL.

Bad, and accepted:

- Every query pays a refresh: a listing of heads, at least.
- A fresh clone pays a full build on its first query.
- Two SQL dialects to keep in step (sqlite, PostgreSQL); `make test-pg`
  runs the index tests on PostgreSQL, outside `make ci`.
- A process holding an old, unlinked index file keeps answering from it
  until it reopens (`core/index/index.go:84-85`, a `ponytail:` note).

## Amendment: one PostgreSQL schema per index version (2026-11-04)

The index on PostgreSQL first lived in one schema, `git_board_index`, and
a git-board of another index version dropped its tables and built them
again. Two git-board versions sharing a database then rebuilt the index
in turn at every open, and one could drop the tables under the other's
queries. Each version now has its schema, `git_board_index_v<version>`:
each keeps its own index up to the same datastore, which is all an index
is. An older version's schema is kept, not dropped: a git-board of that
version may still be reading it, and dropping it would fail its queries
until it opens again. It costs only space, and `DROP SCHEMA … CASCADE`
by hand once no git-board of that version is left. The schema
`git_board_index`, of builds before this, is kept for the same reason.
The sqlite file keeps one name: it is a machine's, and one machine
rarely runs two versions on one clone.

## Left open

- A hosted git-board with no local folder needs the PostgreSQL index or a
  folder path (ADR 0002 › Left open).
- `make test-pg` needs a PostgreSQL DSN, so CI doesn't run the PostgreSQL
  index tests (`Makefile`, `test-pg`).
