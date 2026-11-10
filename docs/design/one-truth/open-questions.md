# One truth, many mirrors: the open questions

[ADR 0008](../../adr/0008-sync-state-in-the-data-model.md), section 1, states the
invariant: the board is the only source of truth, and each forge and
the GitHub Project are mirrors. [Where a value lives](../../spec/places.md)
gives the model and the table of cases, each with its test. This page
keeps only what is still open, and what was decided, with its date.

## Decided

- 2026-11-07: the invariant (ADR 0022, now ADR 0008); the Project bridge's token
  order (the first GitHub table's posting login, else yours on the host,
  else any account there).
- 2026-11-08: `way = "push" | "pull" | "both"` on each forge table, both
  the default. A backup (`push`) takes nothing, and the next run puts
  the board's value back where it was edited there.
- 2026-11-08: `[github-project] fields = [...]` states the fields the
  Project holds. `project configure` and the Project page write it; a
  table without it holds the default seven. This replaces the earlier
  proposal to copy mappings into every GitHub forge table: one key, no
  migration.
- 2026-11-08: conflicts follow two rules, one per mirror's audience. On
  a forge, already public, the later change wins. On the Project, the
  field waits on both sides for a person. The Project gets no
  later-wins setting.
- 2026-11-08: logging in to a forge from the web app is an OAuth device
  flow (the user clicks, types a short code on the forge, and the token
  goes to git's credential helper). It needs an ADR that amends ADR 0010 (it absorbed ADR 0013),
  and an OAuth app on each host (one shipped for github.com; one an
  admin registers on gitlab.epfl.ch).
- 2026-11-08: issues shown outside the app: first a read-only site
  built from the Markdown datastore's files, with a filter so private
  comments stay out; then contributions by editing those files, once
  the read-back credits the author of the git commit.

## Q1. The invariant, word for word

People will quote the headline alone, so it must hold alone. The
proposal:

> A field value lives where the board last recorded an agreement about
> it. A write to a mirror translates a value the board holds; a pull
> makes a mirror's value one the board holds. A mirror can be where a
> change starts; it is never the truth. Before the pull, the board is
> behind, not wrong.

It commits to three statements, each to confirm:

- A board that does not pull holds a stale value, not a false one. The
  agreement makes the difference measurable, and nothing overwrites a
  newer value of a mirror.
- "The board" is your clone's board, not one board of the world: three
  repositories that share a forge have three boards, and the forge is
  where they meet.
- Authorship is not truth: a value pulled from the Project is first the
  Project's; after the pull the board holds it as its own.

Proposal: add the definition to ADR 0008, in a section named What the
invariant says.

## Q2. Mirrors shared by several repositories

Use cases 3 and 4 share one mirror across repositories. Three questions
are left:

- **A filter per forge.** A board may want only its own issues from a
  shared forge (`issues = "label:x"`). Not built, and ADR 0008 does not
  commit to it. Decide whether the want is real first; the alternative is
  one issue repository per code repository, shared only through the
  Project.
- **Two boards, one Project conflict.** Each board records its own
  agreement, holds its own conflict and resolves it; two boards can keep
  different sides, and the Project takes the last resolve. Confirm this
  is acceptable.
- **Deletions.** A delete hides the issue here and keeps it on the forge.
  On a shared forge, three teams see that. Confirm it is still wanted.
