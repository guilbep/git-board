# The visibility matrix: real test repositories

Status: PRD 2026-11-03, issue p1.

## Why

git-board's privacy rules (`docs/spec/sync.md`, "Nothing private
published") are strict and subtle: nothing from a private project reaches a
public one unless `publish-private`; an internal project (GitLab) is public
as a destination and private as a source; a project whose visibility can't
be read is left out. They are tested against a fake forge
(`core/forge/fake`), never against GitHub and GitLab themselves, and the UX
(what the page says, what it refuses) was never walked through on each case.

## Decisions

| Question | Decision |
|---|---|
| What | a **one-shot run, replayable** after a regression (user): a script that creates the test repositories, runs every scenario, records what happened, and deletes them |
| The cells | GitHub private · public; GitLab (gitlab.epfl.ch) private · internal · public. Pairs: each source → each destination, plus each one alone (git remote + one forge) |
| Each scenario | a fresh clone (the clone-shows-board issue), create, edit, comment, close; sync; then check the other side and the page's words (refused, skipped, "publish-private" offered) |
| How | `scripts/visibility-matrix` (bash or Go, `-run` per cell), tokens from the environment, never from the repository; output a Markdown report, one row per cell |
| UX | each cell also has a written checklist the script prints (what to look at in the page), for a human pass on the first run |
| Accounts | a dedicated bot account on each forge, its own repositories prefixed `gb-matrix-`; never a personal or EPFL-ENAC repository |

## Acceptance

1. One command runs every cell and writes the report; one cell runs alone.
2. The report shows, per pair, what crossed and what didn't, against the
   spec's rule for it; any difference is a failure.
3. Re-running after a teardown gives the same report.
4. No token is ever written to a repository, a log or the report.

## Open

The bot accounts and their tokens (who creates them, where they're kept).
