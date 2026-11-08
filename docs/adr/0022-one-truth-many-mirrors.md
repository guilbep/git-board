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

1. **where the board is kept** — `board.datastore`, in the clone's git
   config, not in the file (git, PostgreSQL, Markdown, or the forges
   alone, ADR 0002 section 4); a clone has exactly one, and clones
   converge through git, not through a forge;
2. **which forges, and which way** — `[forges.<name>]` tables; with no
   table, the projects `origin` names, and a table turns that finding
   off;
3. **where each field lives on a forge** — `[forges.<name>.fields]`
   mappings, and `[github-project] fields` for the Project's columns,
   else the type's default on that forge's kind.

Everything else is derived from the agreements: per field, the board's
value, the mirror's and the value both last agreed on (a `seen` on a
forge, the issue's `git-board:<owner>/<number>:<field>` metadata under
the Project); only the side that changed since moves. Both changed
differently: on a forge, the later change wins, because a forge's
change is already public; on the Project, the field is held on both
sides until a person resolves it. Four reversibility guarantees follow and are stated in the spec
([the reversibility guarantees](../spec/places.md#the-reversibility-guarantees)):
an agreement survives its mirror; a held label comes back; a home's
move is explicit and previewed; a Project conflict is never auto-won.
The spec's table of cases names the test that proves each switch, and
`make proved` fails when one of those tests is gone.

## The explicit states this commits to

| State | Committed shape | State |
|---|---|---|
| A field's home on GitHub follows from `[github-project]`'s presence | `[github-project] fields = [...]`, the fields the Project holds, written by `project configure` and the Project page; a table without the key holds the default seven, and the page says so (decided 2026-11-08: one key, not mappings copied into each forge table) | **built**, 2026-11-08 |
| A run's direction is how it was started | `[forges.<name>] way = "push"` (a backup: it only receives, and an edit made there is put back) or `"pull"` (read-only) | **built**, 2026-11-08: `way` is the key, `both` the default; pull, push and sync are still chosen per run |
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
- `way` and the Project's fields were decided and built on 2026-11-08.
  What stays open — the invariant's exact words, and mirrors shared by
  several repositories — is in
  [the open questions](../design/one-truth/open-questions.md).
