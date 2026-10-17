# The issue is the unit of work

PRD, 2026-10-17. Written from the discussion this file used to hold; the
decisions below were taken one at a time in that session.

## The problem

One piece of work is today issue #12 and PR #13 "Fixes #12": two threads,
two label sets, two statuses, two cards. And without the forge's number the
PM can't refer to an issue, which made git-bug unusable for the team.

**One issue is the unit of work, and the code hangs off it.** Its branches
are linked by name, "merged" is computed by git, review is comments on the
issue, and while `dev` lives on GitHub a PR exists only as the gate into it.
There is no new kind of git-bug entity: a change is an issue with branches.

## What co2-calculator showed

Measured in the local clone and read from GitHub, 2026-10-17:

- `dev`'s ruleset: a PR is required (no direct push), **merge is the only
  allowed method**, one approving review from a **code owner**, six required
  checks (Backend Lint/Type Check, Frontent Lint/Type Check, Backend Tests,
  Frontend Tests, Backend Migration Smoke, Docs Build).
- Older work was squashed: 77 branch names are "not merged" by ancestry, and
  33 of them have a merged GitHub PR. `git merge-tree` finds 10 of the 33, a
  whole-branch patch-id 1. After a squash, git can't reliably say a branch
  landed.
- `append-issue-number.yml` appends ` (#N)` to every new issue's title: the
  PM's habit, written into CI.
- The largest diffs are releases (v1.4.3: 163k lines, 1137 files) and
  generated or data files (`purchase_factors.ts` 89k lines, lockfiles,
  OpenAPI snapshots). co2 has no `.gitattributes`.

## Decisions

| Topic | Decision |
| --- | --- |
| A change | An issue with branches, linked by name. No new entity. |
| Numbers | A project sequence, shown `CO2-3075`, handed out through a counter ref; a rejected push is the lock. Issues existing at the switch keep their GitHub number. |
| `#N` | Always GitHub's number. `CO2-N` is the project's. Before the switch they name the same issue. |
| Branch names | `fix/3075-title`: after the switch the number is the project number. |
| Landing | Merge commits (`--no-ff`); history read with `--first-parent`. "Merged" is exact by ancestry. co2's ruleset already enforces it. |
| Roles | Project roles in `[roles]`, overridable per issue. |
| The GitHub PR | The gate only: checks, code-owner approval, merge. Opened, watched and merged from the issue. |
| Review | In git-board: a Changes tab on the issue page. |
| Line comments | In git-bug, as issue comments with an anchor; the bridge mirrors them to the PR and back. |
| Approvals | In git-bug too, posted to the PR as a GitHub review under the reviewer's own token. |
| Discussion | The wiki's "Always use GitHub for discussions" becomes "discuss on the issue". |

Scored alternatives, for the record: forge numbers as the only number (3/10:
co2 never syncs), short hashes (5: not sayable), per-person counters (5),
renumbering on conflict (3: numbers change after they are in branch names),
block leases (7: kept as the offline upgrade), a server handing numbers out
(5 today, the upgrade if a hosted git-board exists). Squash anywhere (3: 23
of 33 wrong), rebase and fast-forward (6), squash through git-board with a
trailer (7).

## Milestones

Each ships on its own, in this order.

### M1: numbers

The PM's blocker, and what branch names hang on.

The prefix is in the counter itself (`prefix CO2`), so every clone reads
the same one; `.git-board.toml` has no key for it.

- **The counter** is `refs/board/seq` on one remote, `origin` (never a
  mirror such as co2's GitLab copy). It holds one file, `numbers`:
  `prefix CO2`, `start 3052`, then one line `N <git-bug id>` per numbered
  issue. The
  next number is the highest of `start` and the last N + 1. To number an
  issue, git-board fetches the ref, appends a line in a new commit, pushes;
  a non-fast-forward rejection means someone else took the number,
  so it fetches and tries the next. `git bug push` doesn't carry `refs/board/*`:
  git-board fetches and pushes it itself.
- **An issue's project number** is, in this order: its line in `numbers`;
  else its GitHub number, when that is below `start` (`#2518` is `CO2-2518`,
  whenever the issue is imported: co2 has bridged only ~325 of its ~811
  issues so far); else none yet, and the counter hands one out.
- **When**: the counter numbers an issue with no project number (made in
  git-board, or imported with a GitHub number at or above `start`) during
  a push to origin the user asks for: Push on the Sync page's Remotes,
  `git board sync`, `git board numbers`, and only once origin has the
  issue (found in the live test: numbering an issue only one clone had
  left gaps in everyone else's numbers). Until then the issue shows its
  short git-bug id. *Decided 2026-10-17, after review:* not on creation
  or after a forge sync, as first written: numbering pushes the counter,
  and nothing goes out without the user asking (the Sync page's rule;
  co2-calculator must never be pushed to). It also keeps numbering under
  the write lock, so it never fetches into the repository while a pull
  does.
- **The switch**, `git board numbers init --prefix CO2 --start 3052`: one
  commit holding only the prefix and `start`, one more than GitHub's highest
  number, PRs included. Pushing it is the switch; until then nothing
  changes. For co2 that is its owner's call.
- **One lookup, which knows the counters** (`internal/gitbugio/gitbugio.go`):
  `HasNumber` and `BareNumber` for a bare number, `HasForgeNumber` for a
  forge's `#N`, in place of a flat `Numbers()` list. After the
  switch, a bare `3060` and `CO2-3060` mean the project number and `#3060`
  means GitHub's, so `fix/3060-…` links to one issue, not to both the
  issue that is CO2-3060 and the one that is GitHub #3060. Before the switch,
  bare numbers mean the forge's, as today. The filter, the branch-link rule
  and `{number}` in branch names already go through this lookup.
- **Shown** before the title, `CO2-3075`, on cards, rows and the issue page.
- **Forges**: `CO2-3075` in a body is plain text on every forge, so no
  translation is expected in the fork's `#n` translation (`refIndex`,
  `bridge/core/syncpush.go`), which only rewrites `#n`, a forge's own
  reference syntax (confirmed with the main session). At the switch, co2's owner should retire
  `append-issue-number.yml`, or new titles become `… (CO2-3075) (#3071)`.

Done when: two clones number issues at the same time against
guilbep/git-board-test and get distinct numbers; an issue made offline gets
its number on the next sync; a GitHub issue below `start` imported after
the switch shows its GitHub number as its CO2 number; a bare number that is
one issue's CO2 number and another's GitHub number finds only the first; a
test races two clones on a local bare remote.

### M2: roles

```toml
[roles]
lead-dev = "bob"
pm = "claire"
reviewer = "alice"
```

- A role is a `person` field; `[roles]` gives its project value. An issue
  overrides one with its own label (`lead-dev:dan`). The value of a role on an
  issue is the issue's, else the project's.
- Filters and boards see inherited values: `lead-dev:@me` matches the issues
  that inherit you. The side shows an inherited value greyed, "Bob · project".
- `assignee` is the contributing developer; `reviewer` defaults to the
  project's code reviewer.
- From the wiki: the lead developer is accountable for the implementation and
  code quality; the code reviewer is co-maintainer and can take over; the PM
  keeps the board up to date (the PM board), the agreements and the
  follow-up.
- Code owners stay GitHub's CODEOWNERS, which the ruleset enforces; git-board
  doesn't read it in this PRD.

Done when: filter tests on inherited and overridden roles pass, and a PM
board view uses `pm:@me`.

### M3: the PR gate

The Development section, per branch:

- **The PR** whose head is the branch, found on GitHub by head ref (not
  stored), or an **Open PR** button: title `CO2-3075 <title>`; body naming
  `CO2-3075` and, when the issue is mirrored to the same repository,
  `Closes #<its GitHub number>`, so merging closes it on both sides through
  the bridge.
- **Checks** for the head commit, the ruleset's required ones first.
- **Approvals**: who approved, at which commit, and whether GitHub counts
  the review rule as met (its review decision, which includes the
  code-owner rule).
- **Merge into dev**: calls GitHub's merge with `method=merge` when GitHub
  says the PR is mergeable; otherwise disabled, saying what it waits for
  ("Frontend Tests, Alice's approval").
- **Who opens it**: whoever clicks, with their own token. GitHub refuses an
  approval from the PR's author, so agents open PRs under their own account,
  never the reviewer's: on a two-person project whose only code owner opened
  the PR, nothing could merge.

Done when: on guilbep/git-board-test, with a ruleset like co2's, a PR is
opened, its checks and approval show, and it merges from the issue.

### M4: review in git-board

The issue page gets a **Changes** tab beside Conversation and History
(mocked and chosen: layout A).

- **The diff** is computed locally: merge base of `dev` and the branch, to
  the branch's head. A commit picker shows one commit, or what changed since
  your last review.
- **Large diffs**: the tab loads the file list first (`git diff --numstat`),
  then each file's diff when opened. Three groups:
  - *To review*: the rest; files under 1,000 changed lines open by themselves.
  - *Large*: over 3,000 changed lines. Folded; "Load the first 5,000", and
    the command to see it locally (`git diff dev...<branch> -- <path>`) with a
    Copy button, as the Create branch panel does.
  - *Generated*: `linguist-generated` in `.gitattributes` (`git check-attr`),
    the attribute GitHub also honors. Folded, and left out of the "viewed"
    count. A few lines of `.gitattributes` would fold co2's lockfiles and
    snapshots on GitHub today.
  - "Viewed" is per viewer, in the browser.
- **A line comment** is an issue comment whose metadata anchors it: path,
  line, side (old or new), commit. It shows beside the code in Changes, and in
  Conversation as the quoted lines, "outdated" once those lines changed at the
  head. A reply is a comment with the same anchor and the comment it answers;
  resolving sets metadata on the first one.
- **A review** (Comment, Approve, Request changes) is a comment whose metadata
  holds its state and commit, with the summary as body. An approval at an
  older commit shows "at d4e5f6a, 2 commits ago" (co2 doesn't dismiss stale
  approvals).
- **The bridge** (the fork's `bridge-sync`, see Constraints):
  - out: line comments become PR review comments; a comment GitHub won't
    take inline (outside the diff, or on a replaced commit) becomes a plain PR
    comment with the anchor written as text; reviews become GitHub reviews.
    Each goes out under its author's own token, as the fork already does
    ("no GitHub token for <login>" otherwise): it reaches GitHub when a
    machine holding that token syncs.
  - in: PR review comments and PR comments, bots' included (Copilot), become
    issue comments with their anchor and their origin (the PR comment's id),
    so they are never sent on to the GitHub issue.

Done when: on guilbep/git-board-test, a comment made in git-board shows on
the PR's line; a comment made on the PR shows on the issue's line; an
approval made in git-board satisfies the code-owner rule; the PR merges from
the issue.

## The six questions

1. *What is a change?* An issue with branches, linked by name.
2. *Numbers across forges and offline clones?* M1: a counter ref on one
   remote; offline issues wait for their number.
3. *Line comments in git, and to forge reviews?* M4: anchored issue comments,
   mirrored by the bridge.
4. *A forge PR on import?* Not an issue: the issue whose branch it carries
   shows it (M3), and its comments land on that issue (M4).
5. *Which gates?* GitHub's ruleset, while `dev` lives there. git-board shows
   and drives them; it doesn't enforce its own.
6. *Roles to fields, filters, boards?* M2.

## Out of scope

- Releases and pre-releases (`dev` → `stage` → `main`): their own discussion,
  `docs/prd/release.md` on `prd/release`. Release PRs stay GitHub's.
- Gates enforced by a hosted git-board (server hooks).
- GitLab merge requests as gates; GitHub first, co2 is on GitHub.
- Email notifications: `git-board-notify`.

## Open, not blocking

- Should the forges' titles carry the CO2 number, for whoever still reads
  GitHub?
- Block leases (10 numbers reserved at a time) if someone works offline for
  days.
- A virtualized diff viewer, if a file someone must read exceeds the
  5,000-line view.

Left by the M1 and M2 reviews (2026-10-17), each small:

- Numbers:
  - Two `numbers init` at the same moment: the loser gets the push error,
    not "already numbers its issues".
  - Free text `3060` still finds a title ending `(#3060)` while co2's
    `append-issue-number.yml` runs.
  - The ref watcher (`RefsFingerprint`) ignores `refs/board`: a CLI
    `git board numbers` doesn't refresh open pages.
  - The local counter moves before its push: a read meanwhile can show a
    number a refusal then takes back.
  - A numbering failure after a successful push reports the push as failed
    (web 502, CLI sync failed).
  - `numberOnce` reads the ref twice; harmless while git-bug's PID lock and
    the write lock hold.
  - `NumberOf` takes any forge named GitHub, not the project's repository:
    a project bridging two GitHub repositories would number twice below
    `start`.
  - Issue fields don't take `CO2-N` (`parent:CO2-3060`); `refs.go`
    resolves `#N` and ids only.
  - Sprint and insights reports, and the palette's "Opened lately", show
    `#N`, not `CO2-N`.
  - `git board sync` prints go-git's push progress (from before M1).
- Roles:
  - `[roles]` accepts a numeric field, a date or iteration name (`due`,
    `sprint`) and the filter's keywords (`is`, `title`, `author`,
    `label`, `no`) as roles.
  - A role declared `single = true`: the server still takes several
    values, and `lint` reports "no lead-dev value" on issues that inherit
    one.
  - The CLI (`ls`, `html`) and the TUI don't show inherited values; the
    TUI's field picker lists a role once an issue sets it; a TUI edit
    leaves `Inherited` stale until `f`.
  - The sprint report lists an issue's own role people (`pm:dave`) as
    assignees.
  - On a board grouped by a role, a drop on the project's lane writes the
    value as the issue's own.
  - Board cards don't show inherited roles (the table, side panel and
    filters do).
  - Several values for one role on one issue: allowed, the first wins in
    groups; the spec says nothing.
  - The field label reads "Pm".

## Constraints

- One branch and one PR for the git-bug fork: `bridge-sync`, which the main
  git-board session is pushing to. M4's bridge work goes through or with that
  session. The upstream PR is not opened; git-bug#1476 is not commented on.
- co2-calculator stays local (no sync); GitHub Project 40 is read-only.
  Switching co2's numbering, and retiring its `append-issue-number.yml`, are
  its owner's calls.
- Live tests only on the scratch forges: guilbep/git-bug-sync-test,
  gitlab.epfl.ch project 42400, guilbep/git-board-test, Project #1.
