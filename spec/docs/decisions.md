# Decisions

The choices this spec starts from, one per line. Each links to the place in
the [PRD][prd] where it was decided.

## The decision

- The core is a Go library inside git-board: git-board's code moves onto
  it, and git-bug goes. ([PRD][decision])
- Go, for its build times, one static binary per platform, and git-board's
  code and tests, which stay. ([PRD][decision])
- Sync first: links to forges, and what each forge last showed, are part of
  the data model. ([PRD][decision])
- Not compatible with git-bug: the data lives under refs of its own, which
  git-bug never sees. ([PRD][decision])
- One binary, `git board`, for people and for CI. ([PRD][decision])
- MIT, once nothing in git-board links GPL code (M6). ([PRD][decision])
- A documented schema: this site, which the code implements.
  ([PRD][decision])

## Rules

- Clean room: the spec is in our own words. Implementers read it, the PRD
  and the milestone plans, never git-bug's code or its spec. ([PRD][rules])
- `core/` never imports `github.com/git-bug/git-bug`, and a test fails if
  it does. ([PRD][rules])
- The library is a library: `github.com/guilbep/git-board/core/...` is
  importable, and never imports git-board's `internal/`. ([PRD][rules])
- Version 1 is what the problem forces, for GitHub and GitLab. The rest
  waits until someone asks. ([PRD][rules])
- The fork stays frozen at v0.11.1-sync.10 until the core passes the
  conformance suite and a live run. ([PRD][rules])

## The data model

- One kind of record, the entity: a log of operations in a chain of commits
  under `refs/board/<kind>/<id>`, ordered by (clock, pack hash).
  ([PRD][model])
- Three kinds: `issue`, `person` and `sync`. ([PRD][model])
- An operation pulled from a forge says where it came from, as a field.
  ([PRD][model])
- Every field is a label, `ns:value`, assignees included. The forges get
  labels, never their own fields. ([PRD][model])
- One exception: `assignee:` labels become the forge's own assignees,
  always. ([PRD][model])
- Acceptance criteria stay text, a task list in the description. The core
  doesn't model them. ([PRD][model])
- Settings shared by the project live in `.git-board.toml`. Who "I" am is
  local. ([PRD][model])
- Tokens stay on each machine: an environment variable, else git's
  credential helper. ([PRD][model])
- Writes check the ref they move. No lock files: only a sync run takes a
  lock. ([PRD][model])
- Queries and search read a local sqlite index, rebuilt from the refs. It
  is never the source of truth. ([PRD][model])
- Fetch and push go through the `git` binary. ([PRD][model])

## Version 1

- In: issues and people, sync with GitHub and GitLab both ways, and
  git-board on the core. ([PRD][v1])
- Out until asked: Gitea, review comments mirrored to pull requests, the
  forges' own fields besides assignees, the `link`, `unlink` and `take`
  commands, Jira, Launchpad, and any API besides git-board's own.
  ([PRD][v1])
- No converter: a repository moves to the core by pulling from its forges
  (M6). ([PRD][answered])
- One sync panel in git-board, no separate git-bug sync. ([PRD][answered])

## Trial results

[prd]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md
[decision]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md#the-decision
[rules]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md#rules
[model]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md#the-data-model-in-brief
[v1]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md#version-1
[answered]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md#answered-by-guilbep-2026-10-24
