# ADR 0003: The board is the team's

- Status: accepted; merged 2026-11-09 from ADR 0003 (decided 2026-10-24)
  and the ownership part of ADR 0018 (decided 2026-11-04)
- Date: 2026-10-24, 2026-11-04
- Absorbs [ADR 0018](0018-own-your-issues-work-in-progress-is-opt-in.md),
  section 1 (the core is ownership). Its cycle part is in
  [ADR 0001](0001-check-results-are-signed-git-notes.md).

## Context

git-board started on a fork of git-bug. That fork carried about 30,000
lines that git-board used and many more that it did not use. git-bug's
own bridges misread the fork's sync metadata and made duplicate issues
on GitHub. The fork was GPL-3.0, and each upstream change cost a rebase.

A team also needs to own its issues. A forge can go away, change its
rules or charge for a feature. The team then loses the issues, their
history, or both.

## Decision

The board is the team's. git-board owns its data model and its sync,
and depends on no forge and on no other issue tracker.

### 1. The core is ownership

A feature belongs to the core when it serves one of these goals:

- The issues, their history and the people who wrote them are the
  team's, in a datastore the team chooses
  ([ADR 0002](0002-datastores.md)), with the same ids everywhere.
- A forge is a mirror that the sync keeps in step. It is never the
  source of truth ([ADR 0008](0008-sync-state-in-the-data-model.md)).
- git-board does for issues all that a forge's CLI or UI does. The CLI
  aims past `gh issue` and `glab issue` ([CLI reference](../guide/cli.md)).
- Leaving a forge, or git-board itself, loses nothing. A board exports
  to Markdown, copies between datastores, and its operations are
  readable without git-board.

Releases and deploys are in the core. Which issue went out in which
version, and where that version runs, is part of owning the issues.

The development cycle, the agents and the worktrees are not the core.
They are opt-in ([ADR 0001](0001-check-results-are-signed-git-notes.md),
section 10).

### 2. Its own data model, not compatible with git-bug

- The board lives under `refs/board/<kind>/<id>`. git-bug never reads
  these refs, so git-bug cannot send an issue to a forge a second time.
- The spec (`docs/spec/`) is in git-board's own words. Implementers read
  the spec and the plans, never git-bug's code.
- `go.mod` names no git-bug module.

### 3. No converter

git-board v0.12 does not convert git-bug data. A repository that still
holds `refs/bugs` is refused, and the message names git-board v0.11's
`git board switch` (`app/forges.go`, `ErrNotSwitched`). v0.11 moves a
repository to this data model by a pull from its forges.

### 4. Version 1 is what the problem forces

Version 1 syncs with GitHub and GitLab. Gitea, Jira, Launchpad and
review comments mirrored to pull requests wait until someone asks.

## Consequences

Good:

- The data model is small enough to hold in one's head, and the spec
  says all of it.
- The sync bookkeeping is part of the model
  ([ADR 0008](0008-sync-state-in-the-data-model.md)), not metadata
  strings.
- git-board is MIT ([ADR 0014](0014-hermetic-tests-on-real-repositories.md)).
- A new user meets issues, views, sync and the CLI first: what git-board
  is for.

Bad, and accepted:

- git-board owns every forge quirk that git-bug used to absorb
  ([Forges](../spec/forges.md)).
- What lived only in git-bug was lost at the switch: issues never sent to
  a forge, local labels never pushed, comment metadata.
- The GitHub Project bridge is a GitHub-only feature in the core. It
  stays, because many teams come from GitHub Projects.
