# Decisions

The choices this spec starts from, one per line. Each links to the place in
the [PRD][prd] where it was decided.

## The decision

- The core is a Go library inside git-board: git-board's code moves onto
  it, and git-bug goes. ([PRD][decision])
- Go, for its build times, one static binary per platform, and git-board's
  code and tests, which stay. ([PRD][decision])
- Sync first: links to forges, and the value git-board and the forge last
  agreed on, are part of the data model. ([PRD][decision])
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

Measured on 2026-10-24, after the final review's fixes, lines with
`wc -l`, tests apart from code.

| Package | Code | Tests |
|---|---|---|
| `gitstore` | 301 | 305 |
| `oplog` | 354 | 499 |
| `issue` | 240 | 370 |
| `forge`, `forge/fake` | 92 | exercised through `syncer` |
| `syncer` (`pull.go` 208, `state.go` 138) | 346 | 377 |
| `core` (doc, import guard) | 4 | 59 |
| **Total** | **1,337** | **1,610** |

Against the targets:

- `gitstore`, `oplog` and `issue` together: 895 lines of code, target under
  1,500. git-bug's operation logs, issues and cache are about 9,200 lines
  and its git layer about 7,100, so 16,300 against 895. The 895 lines are
  the trial's part only: issues, their order, merging, and what a pull of
  current state writes. git-bug's are its whole layers, a cache included.
  The trial has no people, index, search, change events or push yet, and
  reads no forge events.
- The pull: `pull.go` is 208 lines, target under 300; with the sync state
  it keeps (`state.go`) it is 346. git-bug's fork has 910 in `syncpull.go`,
  which also reads events, compares conflict times and finds markers: the
  208 leave those out.

Concepts a reader must hold: entity, operation, pack, clock, order, merge,
ref update, link and seen. Six crept in:

- the **nonce**, in an entity's first pack, so that two entities created
  with the same content have different ids;
- the **forge**, a 35-line package that the pull reads through, so that
  the fake and the real adapters look the same; the interface has two
  methods;
- the **effective link**: of two links for one issue, or for one forge
  issue, the first in the order wins and the other is ignored;
- **identical packs**: two commits whose packs are the same blob count as
  one pack;
- **assignee labels**, `assignee:` in any case, which are labels in
  git-board but a field of their own to sync;
- **duplicate-origin comments**: a comment with an earlier comment's origin
  is that comment, as when two clones pull one forge comment.

`seen` also changed meaning: it is now the **base**, the value both sides
last agreed on. It is still one concept, but a reader must hold the new
meaning.

What was harder than planned: git, not the model. Fetch and push had to
name the three entity kinds, because `refs/board/*` also matched
git-board's own `refs/board/seq`. go-git caches its list of pack files, so
packs written by `git fetch` (over 100 objects) or `git gc` were invisible;
gitstore now re-reads the index after a fetch and once on a missing object.
Its index isn't safe across goroutines either, so one lock serializes
access. git's refusals are matched by their English text, so gitstore runs
git with `LC_ALL=C`. An issue must validate before it writes: a rejected
operation once stayed in the log and made the issue unloadable on every
clone. The base kept a conflict visible, where `seen` hid it, and labels
merge one by one so they can't conflict. Two clones that exchange directly
and merge at the same time each write a merge commit, so their refs keep
diverging while their states agree; through one origin they converge (a
known point for M2). The spec got wrong: the refspecs (`refs/board/*`), what
a reader rejects, the sync state's edge cases (first link wins), and the
merge-commit divergence. All were fixed in the spec as the tasks found them.
The final review found more, fixed the same way: go-git ignored a linked
worktree's common directory, so a worktree saw no issue; fetched heads sat
in `refs/remotes/`, where branch lists and `git fetch --prune` reach them;
a forge comment pulled on two clones became two comments; and labels had no
check.

[prd]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md
[decision]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md#the-decision
[rules]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md#rules
[model]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md#the-data-model-in-brief
[v1]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md#version-1
[answered]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md#answered-by-guilbep-2026-10-24
