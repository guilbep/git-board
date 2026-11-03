# A new board starts as its forge, else as a template: PRD

Nov 3, 2026 · issue #75 · Status: done, the Labels e2e to run (branch feat/75-board-template)

Written from the issue's brief; the brief's choices are marked (user) and
are final.

## Problem

`git board init` writes `internal/config/default.git-board.toml`. On a
repository that already has its labels and a GitHub Project (every ENAC
repository, made from the org's template), Settings › Labels then shows
"Color differs · Adopt" on every label: the file and the forge disagree on
the first day, before anyone changed anything.

## Goal

No conflict between the forge and git-board on the first init.

## Decisions

| Question | Decision |
|---|---|
| A branch's prefix | the kind's label, always (user): `[branches] kinds` can't map a kind to another word; `bug = "fix"` is refused, the message naming the line and the fix. Lint reports a label description that names another prefix ("should prefix branch by fix/" on `bug`). |
| A built-in template | `git board init --template enac` (user): `internal/config/templates/enac.git-board.toml`, embedded like the default. `--template <path|url>` too. |
| Values' spelling | as the forge spells them (user): P0, Must. |
| A default template | `~/.config/git-board/config.toml` (`XDG_CONFIG_HOME` respected), `template = "enac"`: what a plain init uses (user). It never changes a board that exists (user). |
| Init from the forge | when origin's repository has labels or a GitHub Project, `.git-board.toml` is written from them: names, colors, descriptions, the Project's fields and values in order, their descriptions; the template fills only what the forge doesn't have (user). |
| Drift | the file records which template it was copied from, and its version; lint lists what drifted from it (user). |
| Out of scope | renaming values on a board that exists; storage (#76); scripts/ (#77) (user). |

## The ENAC reference

Read from GitHub on 2026-11-03, never from a screenshot.

Labels, `EPFL-ENAC/test` (`gh label list --json name,color,description`):

| Label | Color | Description |
|---|---|---|
| bug | d73a4a | Something isn't working: should prefix branch by bug/ |
| chore | BFDADC | dependencies, CI, tooling, upgrades, housekeeping |
| design | F9D0C4 | design work (Figma), usually a sub-issue of the feat it designs |
| docs | 0075ca | Improvements or additions to documentation: should prefix branch by docs/ |
| feat | 3A2CCD | New feature or request: should prefix branch by feat/ |
| hold:blocked | B60205 | waiting for something outside the team, or for another issue |
| hold:paused | D4C5F9 | deprioritized, or its developer away for a while |
| perf | 120608 | A code change that improves perforamnce: should prefix branch by perf/ |
| polish | EBCF04 | Clean up of UI, UX or phrasing (small changes <30mn): should prefix branch by polish/ |
| refactor | 68E4C5 | Refactor code structure or implementation: should prefix branch by refactor/ |
| style | 5A0DAB | Changes that do not affect the meaning of the code (white-space, formatting, missing semi-colons etc |
| test | FEDE6E | Adding missing tests or correcting existing tests: should prefix branch by test/ |

The template copies them byte for byte, typos included ("perforamnce"):
a corrected description is a description that differs.

GitHub Project "ENAC-IT4R Template", `EPFL-ENAC` number 53
(`gh project field-list`, the options' colors and descriptions by GraphQL):

| Field | Type | Values (color: description) |
|---|---|---|
| Status | single select | Backlog (gray: Known, not yet specified) · Ready (blue: Specified and prioritized; can be started) · In progress (yellow: Branch created) · In review (orange: Pull request open) · Ready for validation (purple: Merged into dev, waiting for validation) · Validated (green: Signed off; released with the next tag) |
| Priority | single select | P0 (red: Now: production broken, data at risk, or the team blocked) · P1 (orange: This cycle: before the next staging review) · P2 (yellow: Soon: the default for accepted work) · P3 (gray: Someday: when convenient) |
| MoSCoW | single select | Must (purple: The milestone fails without it) · Should (blue: Important; the milestone survives without it) · Could (green: If time allows; the first to go) · Won't (gray: Agreed out of this milestone, on record) |
| Sprint | iteration | 14 days, startDay 2; no iteration yet |
| Est. hours | number | |
| Sub-issues progress | GitHub's own | |

## Acceptance

1. `git board init` in a clone of a repository with the ENAC labels and
   Project: Settings › Labels shows **no** "Color differs", no "Adopt"; the
   Project's fields match, values and order included.
2. A repository with no labels and no Project: `init --template enac` gives
   the same board as 1.
3. `~/.config/git-board/config.toml` with `template = "enac"`: a plain
   `git board init` uses it; an existing board is untouched by it.
4. `[branches] kinds = { bug = "fix" }` is refused with a message naming the
   line; the branch for a `bug` issue starts `bug/`.
5. A board whose labels changed after init: lint lists the drift.

## Milestones

One commit each, tests first.

1. Branch prefix = the kind's label.
2. `git board init --template enac|<path>|<url>`.
3. The global file's `template`.
4. Init from the forge; acceptance: on a repository with the ENAC labels and
   Project, Settings › Labels shows no "Color differs" and no "Adopt" (an e2e
   test, the fake GitHub answering labels and Project fields).
5. Lint: drift from the template the file was copied from.
6. Docs: docs/guide/configuration.md, this status line.

## Open points

- Sprint's `startDay: 2`: the Project has no iteration yet to tell which
  weekday GitHub counts it from; the template keeps `start_weekday` to be
  checked on the first sprint.
- Boards that stop loading with milestone 1: `../co2-calculator`
  (`bug = "fix"`, line 193). It's read-only here, so its own PR has to fix it.
