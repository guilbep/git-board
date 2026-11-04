# Working as a team

## Sharing the board with git

The board is part of the repository, under `refs/board/`. It travels
through your git remote, `origin`, like branches do, but with its own
commands, so a `git push` of your code never pushes issues by surprise:

```sh
git board pull            # fetch origin's board and merge it into yours
git board push            # send yours
git board fetch           # just look: what origin has that you don't
git board sync            # all of it: pull, the forges, push
```

Two people who changed the same issue offline both keep their changes:
the operations merge, in the same order on every clone. Two edits of one
field end with the later one; nothing is ever silently lost, and `git
board log` shows both.

A fresh clone gets the board with the first `git board` or `git board
ls`, which pulls `origin`'s.

## Forges

A forge (GitHub, GitLab) is a mirror of the board for the people who work
there. `git board sync` brings their changes in and sends yours out:
issues, comments, titles, descriptions, states, labels and assignees.

- **Found from `origin`** with no configuration: a repository on GitHub
  syncs with that GitHub repository. Settings › Sync adds others, and turns
  each one's sync on or off.
- **A token per forge**, kept by git's credential helper (Settings › Sync ›
  Add a forge stores it there) or in `GITHUB_TOKEN` / `GITLAB_TOKEN` for
  their own host only. git-board never writes one to a file.
- **Check first.** `git board sync --check` (or Sync now's check in the web
  UI) says what a sync would write, and writes nothing. Creating many issues
  on a forge the board never synced with waits for your consent: `git board
  sync --allow-create` once the check looks right.
- **One issue at a time.** An issue's panel has its own Sync, for that
  issue alone.

The details, the first sync of a large project, and what each forge
supports: [Forges](../guide/forges.md).

## Several forges at once

A board can sync with GitHub and GitLab together: an issue created on one
reaches the other through the board. That is how a team moves from one
forge to another without a cut-over day.

## Project numbers

```sh
git board numbers               # number what origin has that has none yet
```

With numbers on (Settings › Numbers), each issue gets `PREFIX-n`
(`CO2-3075`), the same on every forge and in branches and commits.
[Configuration › Numbers](../guide/configuration.md) explains choosing the
prefix and the start. A board kept in Markdown files numbers on its
default branch alone: on another branch, `git board numbers` says why it
waits, and the branch's issues take their numbers once merged.

## Who's who

Each person on the board has a name and, once a forge sync met them, their
forge accounts. Comments and changes pulled from a forge are signed with
the forge account's person. `git board me --ls` lists everyone; `git
board me <id>` makes you write as one of them, for instance as your forge
account's person after the first sync.

## The GitHub Project

A team that keeps a GitHub Project (v2) can sync its fields with the
labels: `git board project configure`, then `git board project pull` or
`sync`. It needs `gh`. See the README's *The GitHub Project* section.
