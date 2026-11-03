# Datastores: where the board is kept

Status: PRD 2026-11-03, issue p2: an architecture decision first, then
a first bridge.

## Why

git-board keeps the board as commits under `refs/board/*`, with a local
sqlite index (`core/index`). Some users won't have that: people who never
clone, a hosted git-board, a team that wants issues in a private PostgreSQL
(local or not), a forge as the only store. One already asked to "export all
issues and comments as a folder of Markdown files". The user should be able
to **choose the datastore**: git, PostgreSQL, Markdown files, another forge
(user).

## Decisions

| Question | Decision |
|---|---|
| First deliverable | an **ADR** (`docs/adr/0002-datastores.md`): the store interface the core writes through, what each datastore must give, and what it may give up |
| First bridge | **Markdown export/import**: a folder, one `NNN-slug.md` per issue (front matter: fields, labels, people, dates; then the description; then the comments in order). Export first; import once the ADR settles identity and history |
| What every datastore must keep | the issue's history as operations (not only its current state), identities, the sync bookkeeping (last seen, echoes), so a datastore can sync with a forge or another datastore |
| What a datastore may give up | offline work, `git log` as history, signed check notes and proof refs: each listed in the ADR, and the features that need them say so when absent |
| A forge as the only store | its sync bookkeeping needs a home; the ADR chooses among a local file, the datastore beside it, or markers on the forge |

## Acceptance

1. The ADR is accepted, naming the interface and each datastore's gaps.
2. `git board export --md <dir>` writes every issue and comment; a second
   run with no change writes nothing.
3. Reading the folder back (a test) gives the same issues, comments and
   fields.

## Out of scope

PostgreSQL itself (after the ADR); a hosted git-board.
