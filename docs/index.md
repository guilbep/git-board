# git-board

git-board is a board for a repository's issues, as a GitHub Project is:
views, a kanban, sprints, insights, releases. The issues themselves live in
the repository, under `refs/board/`: they travel with git like the code,
work offline, and need no database. GitHub, GitLab and the GitHub Project
are mirrors, for the people who work there, and one sync moves the issues
everywhere. The issues are yours: independent of any forge, every change
kept, in the store you choose (git, PostgreSQL or Markdown files).

## How to read this site

- **The [user manual](manual/index.md)** is for everyone who uses a board:
  [install](manual/install.md), [first steps](manual/first-steps.md),
  [everyday work](manual/everyday.md), [working as a team](manual/together.md),
  [owning your issues](manual/ownership.md) and [questions](manual/faq.md).
- **The guide** is for who runs a board.
    - [Configuration](guide/configuration.md): `.git-board.toml`, its
      fields, views, sprints, numbers, labels, releases and the rest.
    - [Forges](guide/forges.md): where your issues go, the sync with GitHub,
      GitLab and the GitHub Project, the tokens.
    - [Issues from the command line](guide/cli.md): every issue command,
      its flags, JSON output and exit codes, and coming from `gh` or `glab`.
    - [Where the board is kept](guide/datastores.md): git, PostgreSQL,
      Markdown files or the forges alone.
- **The spec** states the core's format: git-board's data model and its
  sync with forges, as Go packages of `github.com/guilbep/git-board`
  (`kernel`, `store/…`, `domain/…`, `sync`, `forge/…`).
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
    - [Where a value lives](spec/places.md): the model the sync serves —
      one truth, many mirrors; the three decisions that place a value,
      the agreement that makes a mirror reversible, and the rule for
      each behaviour the config leaves unwritten.
- **The decisions**: [the spec's](spec/decisions.md), the choices it
  starts from, each linked to the PRD; and the architecture decision
  records, from [ADR 0001](adr/0001-check-results-are-signed-git-notes.md)
  to [ADR 0022](adr/0022-one-truth-many-mirrors.md):
  why git-board has its own core ([0003](adr/0003-own-core-instead-of-git-bug.md)),
  keeps issues as operation logs ([0004](adr/0004-entities-are-operation-logs.md)),
  syncs forges as mirrors ([0008](adr/0008-sync-state-in-the-data-model.md)),
  lets you choose where the board is kept ([0002](adr/0002-datastores.md)),
  what it is for ([0018](adr/0018-own-your-issues-work-in-progress-is-opt-in.md)),
  how the core is layered ([0020](adr/0020-the-core-in-layers.md)), why
  v0.12 is new code on the same data ([0021](adr/0021-git-board-v2-same-data-new-code.md)),
  and the one-truth-many-mirrors invariant with the explicit states it
  commits to ([0022](adr/0022-one-truth-many-mirrors.md)).

## The core

The core serves **git-board**, which runs on it: its server, its web app,
its TUI and its command line; and **any Go program** that wants the same
data, through `app`, which opens a board. Nothing in it imports git-bug.

It isn't compatible with git-bug, on purpose. Its data lives under
`refs/board/`, which git-bug never reads, so git-bug can't see it nor send
it to a forge a second time. git-board v0.12 doesn't convert git-bug's
data: a repository that still holds it is refused, and migrates with
git-board v0.11 first (`git board switch`; [Forges › Leaving
git-bug](guide/forges.md#leaving-git-bug)).

Each spec page states what M1 and M2 build. What comes later is marked with
its milestone, as planned, and isn't defined yet. In the examples, pack
hashes and the ids made from them are made up. Times are Unix seconds.
