# ADR 0003: git-board owns its data model; git-bug goes

- Status: accepted (recorded 2026-11-04, decided earlier)
- Date: 2026-10-24 (`docs/prd/core.md`, "Decided with guilbep"); M5 took
  git-bug out of go.mod on 2026-10-25, M6 wrote the switch the same day.
- Sources: `docs/prd/core.md` › The problem, The decision, Rules;
  `docs/spec/decisions.md` › The decision, Rules, M6 results.
- Reconstructed from those pages. The scored table of languages lives in an
  artifact outside the repository; the table below is rebuilt from the PRD.

## Context

Until October 2026 git-board stood on a fork of git-bug
(`bridge-sync`, frozen at v0.11.1-sync.10). The PRD lists what that cost:

- git-board used about 30,000 of git-bug's lines (data model ~17,000,
  bridges ~13,000) and carried its CLI, GraphQL API, TUI, web UI, Jira and
  Launchpad, none of them used.
- The value git-board and a forge last agreed on lived in
  `sync:<remote>:*` metadata strings. An original git-bug running
  `bridge pull` or `bridge push` on a synced repository duplicated issues.
- Upstream announced breaking changes (git-bug#1628, 2026-10-22). Each
  rebase cost a session.
- git-bug is GPL-3.0, so git-board could not be MIT while linking it.

## Decision

**Rewrite the data model and the sync as a Go library inside git-board
(`core/`). Move git-board's code onto it. Drop git-bug.**

1. **Not compatible with git-bug, on purpose.** The board lives under
   `refs/board/<kind>/<id>`, which git-bug never reads, so git-bug can't
   duplicate anything (`docs/spec/storage.md` › Refs). git-bug's
   `refs/bugs/*` stay until `git board prune` removes them.
2. **Clean room.** The spec (`docs/spec/`) is written in our own words.
   Implementers read the spec, the PRD and the plans, never git-bug's code
   or its spec. The one exception was the fork's `sync-decisions.md`, which
   guilbep wrote (`docs/prd/core-m1-plan.md:33-38`).
3. **Enforced by a test.** `core/importguard_test.go:44`
   `TestCoreImportsNoGitBugNorInternal` and `:52`
   `TestModuleImportsNoGitBug` run `go list -deps -test` and fail on any
   `github.com/git-bug/` package. go.mod has no git-bug and no `replace`.
4. **No converter.** A repository moves to the core by pulling from its
   forges (`git-board switch`, `cmd/git-board/switch.go`,
   `internal/store/switch.go`). The switch joins the fork's mirrors by its
   marker and header, re-keys project numbers, carries release records and
   `git-bug.identity` into `board.me`, and writes `refs/board/switched`
   last.
5. **A repository still on git-bug is refused.** `refs/bugs` with no
   `refs/board/switched` makes a forge sync and `git-board sync` stop with
   `ErrNotSwitched` (`internal/store/forges.go:270`).
6. **Version 1 is what the problem forces**, for GitHub and GitLab. Gitea,
   Jira, Launchpad, review comments mirrored to pull requests, `link`,
   `unlink`, `take` wait until someone asks.

## Alternatives considered

Reconstructed from the PRD's problem list. Scored on: no duplicates on
forges (30), cost to keep up (25), licence freedom (20), cost to build (25).

| Option | Score | Why not |
| --- | --- | --- |
| **Own core, git-bug dropped (this decision)** | 80 | |
| Keep the fork, rebase on upstream | 45 | A session per rebase, upstream breaking changes announced, GPL stays, `sync:*` metadata stays misreadable by git-bug's own bridges |
| Upstream the sync changes into git-bug | 40 | Depends on another project's roadmap and review; GPL stays |
| Own core, but compatible with git-bug's format | 50 | Inherits the format bent to sync and the duplicate risk; the clean room would be hard to hold |

The language was scored separately in the PRD's artifact: Go first (2.7 s
rebuild, one static binary for five platforms, git-board's 27,000 lines
and 22,000 lines of tests kept), Rust second, C and C++ far behind.
`docs/prd/core-other-languages.md` keeps plans for the others.

## Consequences

Good:

- The data model is ~6,000 lines of code (M5 result), against ~30,000 used
  from git-bug.
- Sync's bookkeeping is part of the model (ADR 0008), not metadata strings.
- The licence could move to MIT (ADR 0017).
- No upstream to follow.

Bad, and accepted:

- What lived only in git-bug was lost at the switch: issues never sent to a
  forge, local labels never pushed, comments the fork couldn't post,
  git-board's caches in `.git/git-bug/` (`docs/prd/core.md` › Risks).
- Comment metadata isn't carried by the switch (`docs/spec/decisions.md`
  › M6 results).
- Two systems ran side by side until each repository switched. A clone
  still on the fork's git-board would number every issue again.
- git-board now owns every forge quirk the fork used to absorb
  (`docs/spec/forges.md`).

Unchanged: forges stay the place people outside the team read and write
issues.

## Left open

- **Docs still describe git-bug as current.** `CONTRIBUTING.md:7-9` says
  git-bug comes from the fork through a `replace` in go.mod, and `:88-93`
  and `:103` describe git-bug identities and "a real on-disk git-bug repo";
  `CLAUDE.md` says git-bug locks its cache in `.git/git-bug/lock`.
  `cmd/git-board/main.go:1-3` says "fields, sprints and views over git-bug
  issues". `docs/guide/configuration.md` names git-bug repositories and git-bug
  ids (9 mentions), `internal/fields/fields.go:1` "namespaced git-bug labels".
  The code has none of this: go.mod has no git-bug, and the import guard
  passes. These texts need a pass.
- `git board bridge` (`cmd/git-board/bridge.go`) keeps git-bug's word for
  the GitHub Project import; whether it keeps that name is open.
- When `refs/bugs` can be pruned on every remote is guilbep's call, once
  every clone has switched.
