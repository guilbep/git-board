# ADR 0019: Deleting is a tombstone: hidden everywhere, kept in history

- Status: accepted (2026-11-04)
- Date: 2026-11-04
- Builds on [ADR 0004](0004-entities-are-operation-logs.md) (an entity is
  a log of operations) and [ADR 0008](0008-sync-state-in-the-data-model.md)
  (sync is part of the data model).
- Spec: `docs/spec/data-model.md` › `delete`; guide `docs/guide/cli.md` ›
  Deleting.

## Context

`gh issue delete` and a forge's "delete comment" are expected of an issue
tracker. People delete for two reasons: noise (a duplicate, a test issue,
a comment on the wrong issue), and a secret pasted by mistake.

An issue is a log of operations, and nothing in it is ever removed
(ADR 0004). An operation's id is its pack's hash and its index; a pack's
commit names its parents. Removing the text of one operation changes its
pack's hash, so every later commit, every id that names it (`edit-text`,
`set-meta`, the sync entity's `posted`), and every clone's copy. A clone
that still has the old commits merges them back on its next fetch.

## Decision

### 1. `delete` is an operation, and its target is hidden

One new issue operation, `delete`, with a `target`: the `create`
operation's id hides the issue, a comment's id hides that comment. It is
appended like any other, so it merges, syncs between clones and orders as
they do.

- A deleted comment leaves the issue's comments. Its text and metadata
  are not in the state. An `edit-text` or `set-meta` of it, from a clone
  that hadn't the delete yet, changes nothing.
- A deleted issue is still read, marked deleted, so that sync and the
  index know it. No list, query, search, page, export or lookup shows it:
  `git board show <id>` answers not found (exit 3).
- Deleting twice changes nothing. A delete of anything else is an error,
  checked before anything is written.

### 2. The text stays in the history

A delete hides; it doesn't erase. The deleted text is still in the
issue's packs, in every clone that fetched them, and readable with `git
cat-file`. git-board says so where you delete: `git board delete` and
`comment --delete` print it.

**A secret pasted into an issue is leaked.** Revoke it. Deleting the
comment keeps it from being shown, searched, exported or synced again; it
doesn't take it back from the clones and forges that already have it.

Rewriting the log was rejected: it changes ids every clone and every
sync record holds, and the next fetch from a clone that still has the old
commits brings the text back.

### 3. Sync never brings it back, and doesn't delete on the forge

- A deleted issue stays linked to its forge issues. A pull finds the
  link and skips the issue: nothing is pulled into it, and no new issue is
  made from the forge's. Nothing is pushed from it. An issue deleted
  before it was ever pushed is never created on a forge.
- A deleted comment that came from a forge is known by its origin: the
  next pull skips that forge comment, even edited there. One sync posted
  is known by its record. Neither is edited on the forge.
- The forge keeps its copy. Deleting there is the forge's own command:
  git-board doesn't send deletes, since a mirror (ADR 0008) shouldn't lose
  what the forge's own users wrote because one clone hid it.

### 4. Who may delete

Anyone who writes to the board, as for every other operation. The
operation names who deleted, and when.

## Alternatives considered

Scored on: honest about what is gone (30), every clone converges (30),
ids and sync records stay valid (25), cost (15).

| Option | Score | Why not |
| --- | --- | --- |
| **A `delete` tombstone, the text kept in history (this decision)** | 85 | |
| Rewrite the issue's log without the text | 30 | New ids for every later operation; the sync records and every clone's copy name the old ones; the next fetch from an old clone merges the text back |
| A tombstone that also blanks the text in the pack store (git replace, a filtered pack) | 40 | Each datastore needs its own purge; clones and forges keep their copies; looks like an erase and isn't one |
| `edit-text` to an empty text | 50 | The comment stays, empty; the old text stays in the history anyway; nothing tells sync not to pull the forge's copy back |
| No delete, close instead | 45 | A secret or a test issue stays in every list, search and export |

## Consequences

Good:

- Deleting works offline, merges and converges like any operation.
- No id changes: the sync records, the links and the anchors stay valid.
- A deleted issue or comment never comes back from a forge.

Bad, and accepted:

- A deleted text can still be read from the history by anyone with a
  clone. The CLI says so; the guide says to revoke a leaked secret.
- The forge keeps the issue and the comment.
- A deleted issue keeps its links: its forge issue isn't mirrored to any
  other project either.

## Left open

- **Undelete.** The text is there, so a `restore` operation would be
  cheap. Not added: `undo` refuses to undo a delete, and nothing asked for
  it yet. That includes the delete `undo` writes for a new comment
  (2026-11-05): undoing twice redoes everything else, not a comment; a
  second undo refuses and says to post the text again. A copy posted by
  undo would be a new comment, which a sync posts to the forge beside
  the first.
- **Purging.** A real erase of one issue (a new log under a new id, the
  old refs deleted on every clone and remote, the sync records moved) is
  `git board reset --hard` for one issue. Not planned.
- **Deleting on the forge.** `DELETE` of a GitHub or GitLab comment is
  one call per adapter; it would need its own rule for comments the forge's
  users wrote.
- **The web UI.** Deleting is in the CLI only, for now.
