# git-board

git-board is a board for a repository's issues, as a GitHub Project is:
views, a kanban, sprints, insights, releases. The issues themselves live in
the repository, under `refs/board/`: they travel with git like the code,
work offline, and need no database. GitHub, GitLab and the GitHub Project
are mirrors, for the people who work there, and one sync moves the issues
everywhere.

## How to read this site

- **The guide** is for who runs a board.
    - [Configuration](guide/configuration.md): `.git-board.toml`, its
      fields, views, sprints, numbers, labels, releases and the rest.
    - [Forges](guide/forges.md): where your issues go, the sync with GitHub,
      GitLab and the GitHub Project, the tokens.
- **The spec** states the core's format: git-board's data model and its
  sync with forges, as a Go library, `github.com/guilbep/git-board/core/...`.
  It keeps issues, people and their links to forges in a git repository.
  Each one is a log of operations, and the logs of every clone merge into
  the same state. The pages state the format; the code implements them.
    - [Data model](spec/data-model.md): entities, operations and their ids,
      and the operations of each kind.
    - [Storage](spec/storage.md): how an entity lives in git. Refs,
      commits, packs, the clock, the order, merging, updating a ref, fetch
      and push.
    - [Queries and search](spec/queries.md): the local index that lists and
      search read, and the change events that tell a program what moved.
    - [Sync](spec/sync.md) and [Forges](spec/forges.md): how the core syncs
      with GitHub and GitLab, and what each forge needs.
- **The decisions**: [the spec's](spec/decisions.md), the choices it
  starts from, each linked to the PRD; and the
  [ADRs](adr/0001-check-results-are-signed-git-notes.md).

## The core

The core serves **git-board**, which runs on it: its server, its web app,
its TUI and its command line; and **any Go program** that wants the same
data. It never imports git-board's `internal/`, nor git-bug.

It isn't compatible with git-bug, on purpose. Its data lives under
`refs/board/`, which git-bug never reads, so git-bug can't see it nor send
it to a forge a second time. Nothing converts git-bug's data: a repository
moves to the core by pulling its issues from its forges again
(`git board switch`).

Each spec page states what M1 and M2 build. What comes later is marked with
its milestone, as planned, and isn't defined yet. In the examples, pack
hashes and the ids made from them are made up. Times are Unix seconds.
