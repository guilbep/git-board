# ADR 0022: one truth, many mirrors

- Status: accepted (2026-11-07, Pierre); the spec is written
  ([Where a value lives](../spec/places.md)); the explicit states it
  commits to are listed below with whether they are built.
- Date: 2026-11-07
- Builds on [ADR 0007](0007-every-field-is-a-label.md) (fields as labels),
  [ADR 0008](0008-sync-state-in-the-data-model.md) (sync state in the
  data model),
  [ADR 0010](0010-tokens-from-env-or-git-credential-helper.md) (tokens
  from the environment or git's credential helper) and
  [ADR 0016](0016-project-numbers-are-a-counter-on-origin.md)
  (numbers are a counter on origin).

## Context

Configuring the mirrors — the forges and the GitHub Project — leaves
states the config never states: a forge exists because `origin` names
it; a field lives in the Project on GitHub because `[github-project]`
is in the file, not because a mapping says so; a run's direction is
only how it was last started. Asked how the pieces relate, the answer
was spread over the sync spec, the field-mapping PRD and the code, and
the reversibility of switching a mirror on, off, or round — pull, push,
back — was nowhere stated as a whole.

## Decision

**The board is the only source of truth. Every forge and the GitHub
Project are mirrors of it, each write a mirror accepts ends in a
recorded agreement, and no mirror's behaviour may depend on a state the
config does not state.**

Three decisions place every value, each with a home in the config:

1. **where the board is kept** — `board.datastore` (git, PostgreSQL,
   Markdown); a clone has exactly one, and clones converge through git,
   not through a forge;
2. **which forges, and which way** — `[forges.<name>]` tables; with no
   table, the projects `origin` names, and a table turns that finding
   off;
3. **where each field lives on a forge** — `[forges.<name>.fields]`
   mappings, else the type's default on that forge's kind.

Everything else is derived from the agreements: per field, the board's
value, the mirror's and the value both last agreed on (a `seen` on a
forge, the issue's `git-board:<owner>/<number>:<field>` metadata under
the Project); only the side that changed since moves; both changed
differently, the field is held on both sides until a person resolves
it. Four reversibility guarantees follow and are stated in the spec
([the reversibility guarantees](../spec/places.md#the-reversibility-guarantees)):
an agreement survives its mirror; a held label comes back; a home's
move is explicit and previewed; a conflict is never auto-won.

## The explicit states this commits to

| State | Committed shape | State |
|---|---|---|
| A field's home on GitHub follows from `[github-project]`'s presence | configuring the Project writes the mappings it implies into `[forges.<name>.fields]`; no table's presence decides another table's behaviour | the rule is stated and the resolved homes are shown (Settings › Fields, a forge's row in Sync); writing the mappings is **not built** |
| A run's direction is how it was started | `[forges.<name>] way = "push"` (a mirror that only receives — a backup — or only sends) | **built**, 2026-11-08: `way` is the key, `both` the default; pull, push and sync are still chosen per run |
| An S3 datastore | a store implementing `Datastore` (heads, load, add, versions) | **not built**; the interface is the commitment |

## Consequences

- The spec gains a page, [Where a value lives](../spec/places.md),
  that names the invariant, the three decisions, the agreement, the
  guarantees and the use cases; the other spec pages keep the rules
  and refer to it for the model.
- The Sync page's lead says the model in one sentence; a forge's row
  already shows where its fields live.
- The Project bridge's token fallback (the posting login, else yours on
  the host, else any account) is stated in the code and the guide, not
  left as behaviour.
- Future features measure against the invariant: a feature that makes a
  mirror behave differently without a written decision is a bug of the
  model, not a feature.
- The questions the invariant leaves open — what "the only source of
  truth" says exactly, the three conflict laws, `way`, the Project's
  written placements, the shared mirrors — are framed, with options
  and recommendations, in
  [the open questions](../design/one-truth/open-questions.md).
