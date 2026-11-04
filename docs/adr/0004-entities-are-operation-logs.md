# ADR 0004: An entity is a log of operations, ordered by (clock, pack hash)

- Status: accepted (recorded 2026-11-04, decided earlier)
- Date: 2026-10-24 (`docs/prd/core.md` › The data model, in brief; M1 and
  M2 built and measured it the same day).
- Spec: `docs/spec/data-model.md` › Entities, Operations, Ids;
  `docs/spec/storage.md` › Packs, Clock, Order, Merging two heads, Updating
  a ref. ADR 0002 keeps this model and moves only where packs are kept.
- Alternatives below are reconstructed: the PRD records the choice, not
  what it was weighed against.

## Context

The board must be written offline, on several clones and several
machines, and merged with `git fetch`, with no server to settle who wrote
first. Wall clocks differ between machines. The same board must give the
same state on every clone once they have exchanged their data.

## Decision

### 1. One kind of record, the entity

An entity is a log of operations. Its state is what its operations add up
to, applied in one order. Nothing else is stored: no snapshot, no cache
(`docs/spec/data-model.md` › Entities).

- Four kinds, one list: `issue`, `person`, `sync`, `release`
  (`core/datastore/datastore.go:18`).
- An operation is `{kind, author, time, origin?, data}`
  (`core/oplog/oplog.go:30-36`). `time` is for people; it never orders.
- A reader ignores fields it doesn't know, and refuses an operation kind
  it doesn't know rather than drop it (`docs/spec/data-model.md` ›
  Operations).

### 2. Operations are written in packs, and ids come from packs

- A pack is one JSON blob, `{"clock": N, "nonce": …, "ops": [...]}`
  (`core/oplog/oplog.go:69-73`). In git, one commit holds one pack.
- A pack's hash is the git blob id of its bytes, computed without git
  (`core/oplog/oplog.go:84` `Hash`).
- An issue's or a person's id is its first pack's hash; the first pack
  carries a random nonce so two equal creates differ
  (`core/oplog/oplog.go:145-147`). A sync entity's id is the SHA-1 of its
  forge key (`core/syncer/state.go:20`).
- An operation's id is `<pack hash>:<index>`. `edit-text`, `set-meta`,
  `posted` and the markers on forges name operations this way.
- Ids are 40 lowercase hex digits (`core/oplog/oplog.go:94-97`), so no id
  is a ref name git refuses.

### 3. The order every clone agrees on

- A commit's clock is 1 + the highest of its parents' clocks: a Lamport
  clock (`docs/spec/storage.md` › Clock).
- An entity's operations are every reachable pack, sorted by (clock, pack
  hash), each pack's ops in stored order. Two identical packs count once
  (`core/oplog/oplog.go:286` `order`).
- Two heads that don't contain each other are joined by a merge commit
  with an empty pack (`core/oplog/merge.go:47` `Merge`). A merge writes no
  operation.
- So conflicting edits between clones resolve by this order, the same
  everywhere, and the other operation stays in the history
  (`docs/spec/sync.md` › Conflicts).

### 4. A write is a compare-and-swap; no lock files

- A write moves the entity's version only if it is still the one read: in
  git, `git update-ref <ref> <new> <old>`
  (`core/gitstore/store.go:292` `UpdateRef`).
- When it moved, the writer loads again and writes a new pack at the new
  clock + 1, at most 5 tries (`core/oplog/oplog.go:66`, `:197` `Append`).
  A retried pack gets new operation ids.
- Two processes writing at once both land. Only a sync run takes a lock
  (ADR 0008).
- An issue validates before it writes: a rejected operation once stayed in
  the log and made the issue unloadable on every clone
  (`docs/spec/decisions.md` › Trial results).

## Alternatives considered

Reconstructed. Scored on: every clone converges with no server (35),
offline writes (25), history kept (20), simplicity (20).

| Option | Score | Why not |
| --- | --- | --- |
| **Operation log, Lamport clock, (clock, hash) order (this decision)** | 85 | |
| Order by wall-clock time | 50 | Machines' clocks disagree; two clones could apply edits in different orders and diverge |
| Store each issue's current state (a file or row per issue), merge with git's text merge | 35 | Merge conflicts in data files, no history as operations, no echo rule for sync (ADR 0008) |
| A state-based CRDT library (Automerge, Yjs) | 45 | A large dependency and its own binary format, unreadable with git alone; not Go-native at the time |
| A central server that serializes writes | 30 | Not offline; against the project's "store data git already knows how to sync" (`docs/implementation-plan.md` › Why this exists) |

## Consequences

Good:

- Clones converge once they have exchanged heads
  (`core/issue/merge_test.go:31` `TestMergeConverges`).
- Ids are the same in every datastore, so a board moves out of git by
  copying its packs (ADR 0002).
- The whole history is kept; sync's "what git-board took from a forge"
  reads it.

Bad, and accepted:

- The winner of a conflict between clones is arbitrary (clock, then hash),
  not "the latest".
- Every load replays every operation; there is no snapshot. Measured: a
  sync entity of 21,000 operations loads in 0.25 s
  (`docs/spec/decisions.md` › M6 results).
- Nothing is ever deleted from a log. `git board reset --hard` starts a
  board over instead.

## Left open

- **The merge storm.** Two clones that exchange directly and merge at the
  same moment can keep writing merge commits for each other; through one
  origin they converge (`docs/spec/storage.md` › Merging two heads; "not
  planned yet" in `docs/prd/core.md`).
- **JSON strictness.** The spec says keys are case-sensitive and unique;
  Go's decoder accepts any case and keeps the last duplicate
  (`core/oplog/oplog.go:272-274`, a `ponytail:` note).
- **Drift: entity commits' identity.** `docs/spec/storage.md` › Commits
  says author and committer are `git-board <git-board@localhost>` and the
  message is `git-board`. The code writes an empty identity and no message
  (`core/gitstore/store.go:171-175`, since 2026-10-28, "entity commits
  carry no identity and no message"). Readers ignore both, so nothing
  breaks, but the spec is wrong.
