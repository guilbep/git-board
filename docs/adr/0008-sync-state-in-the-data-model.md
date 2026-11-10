# ADR 0008: One truth, many mirrors, and how they sync

- Status: accepted; merged 2026-11-09 from ADR 0008 (decided 2026-10-24),
  ADR 0022 (decided 2026-11-07, built 2026-11-08) and the forge mapping
  of ADR 0007 (built 2026-11-01)
- Date: 2026-10-24, 2026-11-07
- Absorbs [ADR 0022](0022-one-truth-many-mirrors.md) (one truth, many
  mirrors) and the forge half of
  [ADR 0007](0007-every-field-is-a-label.md) (where a field goes on a
  forge).
- Spec: [Where a value lives](../spec/places.md), [Sync](../spec/sync.md)
  and [Forges](../spec/forges.md).

## Context

People outside the team read and write the issues on GitHub and GitLab,
and some project managers keep their priorities in a GitHub Project. A
sync survives a crash at any point, never posts a thing twice, never
echoes back what it pulled, and runs from any clone. Each mirror holds a
field in its own way, and a team switches a mirror on, off or round. The
result of each switch must follow from the configuration alone.

## Decision

### 1. The board is the only truth

The board is the only source of truth. Every forge and the GitHub Project
are mirrors of it. Each write that a mirror accepts ends in a recorded
agreement. No mirror behaves by a state that the configuration does not
state. Three decisions place every value:

1. where the board is kept: `board.datastore` in the clone's git config
   ([ADR 0002](0002-datastores.md))
2. which forges, and which way: the `[forges.<name>]` tables, else the
   projects that `origin` names
3. where each field lives on a mirror: section 5

The [reversibility guarantees](../spec/places.md#the-reversibility-guarantees)
follow: an agreement survives its mirror, a held label comes back, a
home's move is explicit and previewed, and a Project conflict is never
won automatically. `make proved` fails when a test that the spec's
table of cases names is gone.

### 2. The bookkeeping is operations in a sync entity

- Each forge project has one `sync` entity. Its id is the SHA-1 of the
  forge key. Its operations are `link`, `seen`, `posted`, `refused` and
  `run` (`sync/state.go`).
- It merges between clones like any entity, so any clone can sync, and
  the next one knows what the last one did.
- A `run` is recorded only when the run wrote something.

### 3. A three-way compare, field by field

- For each field, the sync compares the board's value, the mirror's value
  and the agreement: `seen` on a forge, the issue's
  `git-board:<owner>/<number>:<field>` metadata under the Project.
- Only the side that changed since the agreement moves.
- Both sides changed on a forge: the later change wins, the forge's at
  the same second. Both changed on the Project: the field is held on both
  sides until a person resolves it.
- Labels and assignees are compared one by one, and never conflict.
- What git-board took from a forge counts as seen there. When the last
  operation on a field has that forge as its `origin`, the sync never
  sends it back. This is the echo rule.

### 4. Markers on forge text name git-board ids

- Each issue and comment that the sync creates ends with a marker:
  `<!-- git-board issue <id> -->` or `<!-- git-board comment <pack>:<n> -->`
  (`sync/text.go`).
- A marker counts only in text that sync's own accounts posted.
- A push that stopped after the forge write leaves a marker, and the next
  run links it. A write is never retried blindly.

### 5. Where a field lives on a mirror

In the board, a field is a label
([ADR 0004](0004-entities-are-operation-logs.md), section 5). On a forge,
it lives where `[forges.<name>.fields.<field>] to` says, else at its
type's default on that forge's kind (`forge/held.go`, `DefaultTarget`).
The table of defaults is in
[Configuration › Forges](../guide/configuration.md#where-a-field-goes-on-a-forge).
On GitHub with a Project, the fields that `[github-project] fields` names
live in the Project's columns.

The forge sync writes labels and native assignees only. A label is held
on a forge unless its field goes to a label or a scoped label there. The
sync never sends, pulls or removes a held label. `milestone:` is always
held. The assignees go to the forge's own assignees, and the Project's
fields go to its columns through the bridge. Any other home leaves the
field in git-board alone on that forge.

### 6. Each forge table states its way

`[forges.<name>] way` is `both` (the default), `push` or `pull`. With
`push`, the forge is a backup: the sync writes to it, pulls nothing, and
puts back an edit made there at the next run. With `pull`, the forge is
a read-only mirror: the sync reads it and writes nothing there.

### 7. Other fixed rules

- A delete is never sent to a forge, and a delete on a forge is never
  pulled. A `delete` in git-board hides the issue or the comment, and the
  sync skips it from then on
  ([ADR 0004](0004-entities-are-operation-logs.md), section 6).
- Nothing private is published. Confidential issues are skipped. Nothing
  from a project that is not public goes to a public one, unless
  `publish-private` says so.
- A posting account posts for the people without a token, under a header
  that names them. A token owner acts as themselves.
- One run per machine at a time: an OS lock on `<folder>/sync.lock`,
  which also journals what a cut run reached (`sync/lock.go`). A
  PostgreSQL board takes a lock that every machine shares.
- The engine talks to forges through one interface (`forge/forge.go`,
  `Forge`). The fake and every adapter run the same conformance suite
  (`forge/forgetest`).

## Consequences

Good:

- Any clone, or CI, can run a sync, and pick up where another stopped.
- A run after a run writes nothing.
- A board can be rebuilt from its forges (`git board reset --hard`, the
  forges-only datastore).
- A mirror switched off and on again moves only what changed meanwhile.
- A feature that makes a mirror behave by an unwritten state is a bug of
  the model.

Bad, and accepted:

- The sync entity grows forever. 3,000 issues make about 21,000
  operations, read in a quarter of a second.
- A conflict between git-board and a forge uses wall-clock time, so a
  wrong clock can pick the wrong winner. The losing value stays in the
  history.
- Anyone with write access on the forge can edit a marker. A deleted
  marker makes the comment plain text.
- Pulled operations carry the pull's time, not the forge's change time.
- Forge users see `status:ready` as a label, unless a mapping moves it.
