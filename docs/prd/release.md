# Releases: the pre-release and release pipeline

A discussion to turn into a PRD. Start a session here with:

```sh
cd ~/works/git/github/git-board-release && claude
# then: "Read docs/prd/release.md, and let's discuss it before writing the PRD."
```

## Where this comes from

Split off the work-unit discussion (`docs/prd/work-unit.md`, branch
`prd/work-unit`). The work unit is an issue and its own branches; a release
merges long-lived branches (`dev` → `stage` → `main`), 100k+ lines that
bundle work already reviewed, and was set out of scope there: "Release PRs
stay on GitHub exactly as today. Revisit if releases ever need an issue of
their own."

## What co2-calculator does today

- **Pre-release**, `dev` → `stage`: a PR like `chore: pre-release 1.4.19
  (#2982)`, with a commit bumping `CHANGELOG.md`.
- **Release**, `stage` → `main`: a PR like `chore: release 1.4.19 (#2984)`.
  Merging it runs `release-please.yml` ("Tag and Release"): the version from
  `package.json`, a tag, the changelog entry, a GitHub Release.
- `changelog.yml` runs on pull requests.
- These are co2's largest merges: v1.4.3 is 163k lines over 1137 files,
  mostly other branches' work plus lockfiles and OpenAPI snapshots.

## Decided in work-unit, which this builds on

- Issues are numbered `CO2-N` (a project sequence; GitHub numbers kept up to
  the switch, `#N` always means GitHub's).
- Work lands with merge commits (`--no-ff`); history is read with
  `--first-parent`, so "merged" is exact by ancestry.
- Branch names: `fix/3075-title`.
- Roles: `[roles]` in `.git-board.toml` (lead dev, PM, code reviewer),
  overridable per issue.
- Review happens in git-board; the GitHub PR is the gate (checks,
  code-owner approval, merge).

## Ideas to discuss

- **A release as an issue**: head `dev`, base `stage` (then `stage` →
  `main`). Its Changes tab lists the issues it carries, found from the
  merges between the two branches and their branch names, instead of the
  163k-line diff.
- **Where an issue has got to**: in `dev`, in `stage`, in `main`, computed
  by git the way "merged" is. A field, a board, or a line in Development.
- **The changelog from issues** rather than from commits.
- **release-please**: keep it, feed it, or replace it.

## Questions to answer in the PRD

1. Is a release an issue, a config entry like a sprint, or only tags?
2. How does an issue show which environment it has reached?
3. Where does the changelog come from, and who edits it?
4. Who opens the pre-release and release PRs: git-board, release-please, a
   person?
5. What stays GitHub's (tags, Releases, Actions)?

## Constraints

- co2-calculator stays local (no sync); GitHub Project 40 is read-only.
- Live tests only on the scratch forges: guilbep/git-bug-sync-test,
  gitlab.epfl.ch project 42400, guilbep/git-board-test, Project #1.
