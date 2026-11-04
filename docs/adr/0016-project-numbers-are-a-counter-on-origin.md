# ADR 0016: Project numbers come from one counter, `refs/board/seq` on `origin`

- Status: accepted (recorded 2026-11-04, decided earlier)
- Date: before 2026-10-20 (no doc dates it; the core's M2 inventory of
  2026-10-24 lists `refs/board/seq` as git-board's own and "staying"). The
  re-keying at the switch is M6, 2026-10-25. A datastore's counter is ADR
  0002 (2026-11-04).
- Sources: `internal/store/numbers.go:20-36`; `docs/guide/configuration.md`
  › Numbers; `docs/spec/decisions.md` › M6 results; `docs/spec/storage.md`
  › Refs.
- Alternatives below are reconstructed.

## Context

An issue's id is a 40-digit hash (ADR 0004): nobody says it aloud or puts
it in a branch name. Forge numbers (`#3075`) belong to one forge, differ
between GitHub and GitLab, and don't exist for an issue never pushed to a
forge. A board independent of any forge needs numbers of its own that every
clone agrees on, without a server.

## Decision

**A project numbers its issues itself, `<PREFIX>-<n>` (`CO2-3075`), from a
counter kept in git under `refs/board/seq`, handed out by `origin` alone.
Pushing the counter is the lock.**

- The counter is a chain of commits whose tree holds one file, `numbers`:
  the prefix, the start, and one line per numbered issue, `<n> <issue id>`
  (`internal/store/numbers.go:20-27`, `:58` `ParseCounter`).
- It isn't an entity: the core leaves it alone, and fetch and push name the
  four entity kinds so `refs/board/*` doesn't sweep it in
  (`docs/spec/storage.md` › Refs; `docs/spec/decisions.md` › Trial
  results).
- Only `origin` hands numbers out: "a mirror never does: two counters would
  give a number twice" (`internal/store/numbers.go:33-36`
  `NumbersRemote`).
- Numbering fetches the counter, writes the next commit, and pushes it
  unforced. A refused push means someone else took the number: fetch and
  try again, five tries (`:358` `NumberIssues`, `:383` `numberOnce`,
  `:487` `pushCounter`).
- Only issues `origin` already has are numbered, so no clone sees a number
  for an issue it can't fetch. Creating an issue or a forge sync never
  pushes the counter; `git board sync`, `git board numbers` or Settings ›
  Numbers do.
- Below `start`, an issue keeps its GitHub number: `#2518` is `CO2-2518`
  (`:112` `NumberOf`).
- On another datastore the counter is a capability, `datastore.Counter`
  (`core/datastore/datastore.go:49-50`): a row in PostgreSQL, absent for
  Markdown and forge-only boards, which then say "project numbers need a
  datastore that counts" (`internal/store/numbers.go:550-556`).

## Alternatives considered

Reconstructed. Scored on: one number per issue on every clone (35), no
server (25), short and stable (20), forge-independent (20).

| Option | Score | Why not |
| --- | --- | --- |
| **One counter in git, on `origin`, push as lock (this decision)** | 80 | |
| Forge numbers only | 45 | Differ per forge; none for an issue on no forge; lost on moving forge |
| Short id prefixes (the first 7 hex digits) | 50 | No coordination needed, but not sayable, not ordered, can collide as boards grow |
| A number in the issue's own operations (`set-number`) | 40 | Two clones can give the same number offline; the merge would have to renumber |
| Per-clone ranges (each clone a block of numbers) | 45 | Numbers stop following creation order; ranges to manage |

## Consequences

Good:

- Numbers are short, ordered, forge-independent, and go in branches and
  commit messages (`fix/co2-3075-…`).
- No server: git's refusal of a non-fast-forward push serializes numbering.

Bad, and accepted:

- An issue has no number until it is pushed to `origin` and numbered;
  until then it shows its short id.
- Numbering needs `origin` reachable: offline, nothing is numbered.
- A clone that took back an older counter would renumber everything. So a
  fetch refuses a counter behind this clone's, and the switch re-keys on
  top of origin's and must be pushed before anyone syncs
  (`docs/spec/decisions.md` › M6 results).
- `#N` is always a forge's number, `PREFIX-N` the project's: two notations
  to learn.

## Left open

- `NumbersRemote` is hard-coded to `origin`: "a setting when a project's
  isn't" (`internal/store/numbers.go:33-35`, a `ponytail:` note).
- Five tries, "more people numbering in the same second is beyond a team
  of ten" (`:365-366`).
- `docs/guide/configuration.md` › Numbers says an un-numbered issue "shows
  its short git-bug id": it is the core's id now (ADR 0003).
