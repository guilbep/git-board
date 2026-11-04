# ADR 0008: Sync with forges is part of the data model

- Status: accepted (recorded 2026-11-04, decided earlier)
- Date: 2026-10-24 (`docs/prd/core.md` › The decision: "Sync first";
  M3 built the engine the same day, M4 the GitHub and GitLab adapters).
- Spec: `docs/spec/sync.md` (all of it), `docs/spec/data-model.md` › Sync,
  `docs/spec/forges.md`.
- Builds on ADR 0004 (operations, origins, ids). ADR 0002 § 4 relies on it
  for a forge-only board.

## Context

git-board's issues are mirrored, both ways, on GitHub and GitLab, where
people outside the team read and write them. In the git-bug fork, links and
"the value both sides last agreed on" were `sync:<remote>:*` metadata
strings that git-bug's own bridges misread, duplicating issues
(ADR 0003). A sync must survive a crash at any point, never post a thing
twice, never echo back what it pulled, and run from any clone.

## Decision

### 1. The bookkeeping is operations in a `sync` entity per forge project

- One entity per project, its id the SHA-1 of the forge key
  (`core/syncer/state.go:20`). Operations: `link`, `seen`, `posted`,
  `refused`, `run` (`docs/spec/data-model.md` › Sync).
- It merges between clones like any entity, so any clone can sync and the
  next one knows what was done.
- A `run` is recorded only when the run wrote something, so a quiet
  project's entity doesn't grow.

### 2. A three-way compare against `seen`, field by field

- For each field: git-board's value (G), the forge's (F), last seen (S).
  Only G changed: push. Only F changed: pull. Both: the later change by
  time wins; at the same second the forge's (`docs/spec/sync.md` › The
  four cases).
- Between clones, the order of ADR 0004 decides; between git-board and a
  forge, time does (`docs/spec/sync.md` › Conflicts).
- Labels and assignees are compared one by one and never conflict. A label
  push is a diff from `seen`, never the whole set.
- **What git-board took from a forge counts as seen there**: when the last
  operation on a field has that forge as `origin`, sync compares with it
  and never sends it back. This is the echo rule, and it settles two clones
  pulling one comment.

### 3. Markers on forge text name git-board ids

- Each issue and comment sync creates carries an HTML comment on its last
  line: `<!-- git-board issue <id> -->`, `<!-- git-board comment
  <pack>:<n> [quoted] [k=v…] -->` (`core/syncer/text.go:20`, `:24`).
- A marker counts only in text posted by sync's own accounts, so no
  stranger can tie a forge issue to a git-board one.
- Markers are how crash recovery works: a push that stopped after the
  forge write and before the `link` or `posted` leaves a marker, and the
  next run links it (`docs/spec/sync.md` › Crash recovery). A write is
  never retried blindly.

### 4. Other fixed rules

- Deletions are never propagated, either way. git-board has no delete
  operation (`docs/spec/sync.md` › Deletions).
- Nothing private is published: confidential issues are skipped, and
  nothing from a non-public project goes to a public one unless
  `publish-private` (`docs/spec/sync.md` › Nothing private published).
- People without a token are posted for by a posting account, under a
  header naming them; a token owner acts as themselves.
- One run per machine at a time: an OS lock (`flock`, `LockFileEx`) on
  `board/sync.lock`, which also journals what a cut run reached
  (`core/syncer/lock.go:22`, `lock_unix.go:12`). No other write takes a
  lock (ADR 0004).
- The engine talks to forges through one interface,
  `core/forge/forge.go:85` (`Forge`, 15 methods, plus `Replier`). Every
  adapter, the fake included, runs the same conformance suite
  (`core/forge/conformance`, `Run` at `:37`; called from
  `core/forge/github/github_test.go:32`, `gitlab/gitlab_test.go:28`,
  `fake/conformance_test.go:14`).

## Alternatives considered

Reconstructed, except the first row, which is what ADR 0003 left. Scored
on: no duplicates and no lost edit after a crash (35), any clone can sync
(25), no echo loops (20), cost (20).

| Option | Score | Why not |
| --- | --- | --- |
| **Sync state as operations, three-way compare, markers (this decision)** | 85 | |
| Metadata strings on issues (the fork's way) | 35 | Misread by other tools; duplicated issues on GitHub |
| Sync state in a local file or database per machine | 45 | Only one machine could sync; a lost machine re-pulls everything, and markers could no longer be trusted without the records |
| Last-writer-wins with no `seen` (compare G and F only) | 30 | Can't tell who changed what; every difference becomes a conflict, and echoes loop |
| Forge webhooks into a server | 30 | Needs a public server; against offline and forge independence |

## Consequences

Good:

- Any clone, or CI, can run a sync and pick up where another stopped.
- A run after a run writes nothing (tested by the conformance suite's
  "mirror then stable").
- A board can be rebuilt from its forges (`git board reset --hard`, the
  switch of ADR 0003, a forge-only board of ADR 0002).

Bad, and accepted:

- The sync entity grows forever; no snapshot. 3,000 issues make 21,000
  operations, read in 0.25 s (`docs/spec/decisions.md` › M6 results).
- Conflicts between git-board and a forge use wall-clock time, so a wrong
  clock can pick the wrong winner. The losing value stays in history.
- Markers are visible in a forge's raw text and editable by anyone with
  write access; a deleted marker makes the comment plain text.
- Two sync runs on two machines at once are not excluded: the lock is per
  machine. The CAS on the sync entity keeps its records whole.

## Left open

- Forge change times: pulled operations carry the pull's time, not the
  forge's change time, so "later" between two forges is approximate
  (`docs/prd/core.md` › Carried to later milestones, M4).
- Rate-limit pacing is the adapters', not the engine's.
- `link`, `unlink` and `take` are out of version 1.
- A command-line forge sync exists now (`git board forge sync`,
  `cmd/git-board/forge.go:28`), though `docs/prd/core.md` › Carried still
  lists "git-board has no command-line forge sync" as open (M6): stale.
