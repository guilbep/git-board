# git-board core

The core is git-board's data model and its sync with forges, as a Go
library. It keeps issues, people and their links to forges in a git
repository. Each one is a log of operations, and the logs of every clone
merge into the same state.

## For whom

- **git-board**, which runs on it: its server, its web app, its TUI and its
  command line.
- **Any Go program** that wants the same data. The library is
  `github.com/guilbep/git-board/core/...`. It never imports git-board's
  `internal/`, nor git-bug.

## What it isn't

The core isn't compatible with git-bug, on purpose. Its data lives under
`refs/board/`, which git-bug never reads, so git-bug can't see it nor send
it to a forge a second time. Nothing converts git-bug's data: a repository
moves to the core by pulling its issues from its forges again.

## How to read this site

The pages state the format. The code implements them.

- [Data model](data-model.md): entities, operations and their ids, and the
  operations of each kind.
- [Storage](storage.md): how an entity lives in git. Refs, commits, packs,
  the clock, the order, merging, updating a ref, fetch and push.
- [Sync](sync.md) and [Forges](forges.md): how the core syncs with GitHub
  and GitLab, and what each forge needs.
- [Decisions](decisions.md): the choices the spec starts from, each linked
  to the PRD.

Each page states what M1 builds. What comes later is marked with its
milestone, as planned, and isn't defined yet.

In the examples, pack hashes and the ids made from them are made up. Times
are Unix seconds.
