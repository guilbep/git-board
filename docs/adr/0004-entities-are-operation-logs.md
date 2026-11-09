# ADR 0004: Issues are operation logs, fields are labels

- Status: accepted; merged 2026-11-09 from ADR 0004 (decided 2026-10-24),
  ADR 0007 (before 2026-09-02, confirmed 2026-10-24) and ADR 0019
  (decided 2026-11-04)
- Date: 2026-10-24, 2026-11-04
- Absorbs [ADR 0007](0007-every-field-is-a-label.md) (every field is a
  label, without its forge mapping, which is in
  [ADR 0008](0008-sync-state-in-the-data-model.md)) and
  [ADR 0019](0019-deleting-is-a-tombstone.md) (deleting is a tombstone).
- Spec: [Data model](../spec/data-model.md) and
  [Storage](../spec/storage.md).

## Context

The board is written offline, on several clones and machines, and merged
with `git fetch`, with no server to say who wrote first. Clocks differ
between machines. Every clone gives the same state once the clones
exchange their data. Each project needs its own fields, and the fields
travel with the issues through git and through every forge. People also
expect to delete an issue or a comment.

## Decision

### 1. An entity is a log of operations

An entity's state is what its operations add up to, applied in one
order. Nothing else is stored: no snapshot and no cache.

- There are four kinds: `issue`, `person`, `sync` and `release`.
- An operation is `{kind, author, time, origin?, data}`. Its `time` is for
  people, and never orders.
- A reader ignores a field that it does not know. It refuses an operation
  kind that it does not know, and never drops it.

### 2. Operations are written in packs, and ids come from packs

- A pack is one JSON blob, `{"clock": N, "nonce": …, "ops": [...]}`. In
  git, one commit holds one pack.
- A pack's hash is the git blob id of its bytes, computed with no git
  (`kernel/pack`).
- An issue's or a person's id is the hash of its first pack. That pack
  carries a random nonce, so two equal creates differ.
- An operation's id is `<pack hash>:<index>`. `edit-text`, `set-meta`,
  `delete`, the sync records and the markers on forges name operations
  this way.
- An id is 40 lowercase hex digits.

### 3. One order on every clone

- A pack's clock is 1 + the highest clock of its parents: a Lamport
  clock.
- An entity's operations are its packs sorted by (clock, pack hash), each
  pack's operations in their stored order. Two identical packs count
  once.
- A merge joins two heads with an empty pack, and writes no operation.
- So two clones resolve conflicting edits by the same order, and the
  other edit stays in the history.

### 4. A write is a compare-and-swap, with no lock file

- A write moves the entity's version only if it is still the one read.
  In git, this is `git update-ref <ref> <new> <old>`.
- When the version moved, the writer reads again and writes a new pack,
  5 tries at most (`kernel/board.go`).
- The issue domain refuses a bad operation before it writes it, so a rejected
  operation never enters the log.
- Only a sync run takes a lock ([ADR 0008](0008-sync-state-in-the-data-model.md)).

### 5. A field value is a label

- Every field besides the title, the body, the open or closed status and
  the comments is a label `ns:value`: `status:ready`, `prio:p1`,
  `due:2026-10-30`, `parent:#12`. A label without a colon is a plain
  label.
- One operation, `change-labels`, writes all of them. Labels merge one
  by one, so they never conflict.
- `.git-board.toml` gives them meaning
  ([ADR 0011](0011-shared-settings-in-git-board-toml.md)): `[fields.<ns>]`
  adds options, a type, cardinality, colors and units. A namespace found
  on issues is already a field.
- A single-value field swaps: a new value removes the other labels of its
  namespace.
- The workflow status (`status:in-progress`) is a field like the others.
  `git board close` sets its last option and closes the issue.
- On a forge, a field is a label unless its mapping says otherwise
  ([ADR 0008](0008-sync-state-in-the-data-model.md), section 5).

### 6. Deleting is a tombstone

- A `delete` operation names a target: the `create` operation's id hides
  the issue, a comment's id hides the comment. It merges and syncs like
  any operation.
- A deleted comment leaves the issue's state, with its text. A later
  `edit-text` or `set-meta` of it changes nothing.
- No list, query, search, page or export shows a deleted issue.
  `git board show <id>` answers not found.
- A delete hides, and does not erase. The text stays in the packs of
  every clone. `git board delete` and `comment --delete` say so. A secret
  pasted into an issue is leaked: revoke it.
- The sync never brings a deleted issue or comment back, and never sends
  a delete to a forge.
- Anyone who writes to the board can delete, as for any other operation.

## Consequences

Good:

- Clones converge once they exchange heads, offline included.
- Ids are the same in every datastore
  ([ADR 0002](0002-datastores.md)).
- The whole history is kept. The sync reads it to find what came from a
  forge.
- A new field needs no code and no migration: write a label.
- A delete keeps every id, link and sync record valid.

Bad, and accepted:

- The winner of a conflict between clones is arbitrary (clock, then
  hash), not the latest edit.
- Every load replays every operation of the entity.
- A single-value field can hold two values after a merge between clones.
  `git board lint` reports it, and a value outside `options`.
- A deleted text stays readable in the history of every clone.
- Nothing leaves a log. `git board reset --hard` starts a board over
  instead.
