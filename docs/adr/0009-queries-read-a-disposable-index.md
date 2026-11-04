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
  `index.SQL`, the same tables live in the schema `git_board_index` on the
  server, shared by every machine (`core/index/index.go:58-64`, `:73-75`).

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

## Left open

- A hosted git-board with no local folder needs the PostgreSQL index or a
  folder path (ADR 0002 › Left open).
- `make test-pg` needs a PostgreSQL DSN, so CI doesn't run the PostgreSQL
  index tests (`Makefile`, `test-pg`).
