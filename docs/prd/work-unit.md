# The issue is the unit of work

A discussion to turn into a PRD. Start a session here with:

```sh
cd ~/works/git/github/git-board-work-unit && claude
# then: "Read docs/prd/work-unit.md, and let's discuss it before writing the PRD."
```

## The idea

Pull requests and merge requests should never have existed as a second
object. A PR was first an email (`git request-pull`: "please pull branch X
from URL Y"); forges made it a thing of its own, with its own number,
thread, labels, assignees and state. One piece of work is now issue #12 and
PR #13 "Fixes #12": two threads, two label sets, two statuses, two cards on
the GitHub Project.

The proposal: **one issue is the unit of work, and the code hangs off it.**

- the branch is linked by its name (`fix/2518-…` ↔ #2518), not stored;
- "merged" is computed by git (is the branch's head in `dev`?), not stored;
- review is the issue's comments, with comments anchored to a file, a line
  and a commit as the one real gap;
- what forges add (merge button, required checks, protected branches, code
  owners) are gates on pushing to the base branch, which a hosted git-board
  could enforce with server hooks.

## What's already decided or built

- git remotes carry everything (code, branches, issues, identities):
  `git bug push/pull <remote>` works with any remote, and concurrent edits
  merge without conflicts. Forge bridges are optional mirrors.
- **First small step, being built in the main session:** a "Create branch"
  button on an issue (the name from the repository's convention, a copy
  button for `git switch -c <name> origin/dev`), and a read-only
  "Development" section listing the branches that name the issue, ahead and
  behind, merged or not. Config: `[branches] base, name, kinds`.

## The user's ideas to discuss (in their words where possible)

- **Issue numbering.** "How do we define a specific id/number for our local
  git-bug to override the forge id?" The PM refers to issues by number;
  without the forge number git-bug was unusable for the team. Does a
  project get its own sequence (`CO2-123`), independent of every forge?
  Who hands out numbers when two clones create issues offline?
- **Interface first.** Design the screens before the model?
- **Merge, rebase, and the way of merging/rebasing**: squash, rebase,
  merge commits, who decides, per project or per issue.
- **Who owns the code**: code owners, and how they relate to the issue's
  people.
- **Roles as fields on the issue**: `assignee:pm`, `reviewer:…`,
  `lead-dev:…`, following the team's roles (wiki, "Projects roles &
  responsabilities at ENAC-IT4R"):
  - **Lead developer**: responsible for the implementation and the
    deliverables on time, regular code reviews, technology choices;
    accountable for code quality and reproducibility.
  - **Code reviewer** = co-maintainer: consulted, able to take over if the
    lead developer is unavailable.
  - **Project manager**: realistic deliverables, follow-up, keeping the
    board up to date, retrospectives; accountable for agreements, keeping
    collaborators informed, billing.
  - **Contributing developer**: implements specific parts.
  - The wiki's rule "Always use GitHub for discussions": what replaces it.
- **Old-school patches by email** were considered and set aside for the
  team (Outlook mangles patches, no review page); email notifications are
  their own session (`git-board-notify`).

## Prior art worth reading

- Radicle: issues and patches as collaborative objects stored in git, peer
  to peer. The closest to git-bug.
- Gerrit: a change is `refs/changes/…`, one change per commit, reviewed.
- git-appraise: reviews in git notes.
- Fossil: tickets, wiki and code in one repository.
- sourcehut, and the Linux kernel with `b4`: patches by email today.

## Questions to answer in the PRD

1. What is a "change" here: an issue with a branch, or a new kind of
   git-bug entity?
2. How are numbers handed out, and shown, across forges and offline clones?
3. How do line comments live in git, and travel to forges' PR reviews?
4. What happens to a forge PR on import: an issue with a branch?
5. Which gates (checks, approvals, code owners) does a hosted git-board
   enforce, and how?
6. How do the roles map to fields, filters and boards (the role boards
   already exist: `lanes`, `limits`)?

## Constraints

- One branch and one PR for the git-bug fork; the upstream PR is not
  opened, and git-bug#1476 is not commented on.
- co2-calculator stays local (no sync); GitHub Project 40 is read-only.
- Live tests only on the scratch forges: guilbep/git-bug-sync-test,
  gitlab.epfl.ch project 42400, guilbep/git-board-test, Project #1.
