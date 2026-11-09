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
  starts from, each linked to the PRD, and the nine architecture
  decision records, read in order on [The decisions](adr/index.md):
  the board is the team's ([0003](adr/0003-own-core-instead-of-git-bug.md)),
  where it is kept ([0002](adr/0002-datastores.md)),
  issues as operation logs and fields as labels ([0004](adr/0004-entities-are-operation-logs.md)),
  the shared and the personal configuration ([0011](adr/0011-shared-settings-in-git-board-toml.md)),
  one truth and many mirrors ([0008](adr/0008-sync-state-in-the-data-model.md)),
  no stored secret and a local server ([0010](adr/0010-tokens-from-env-or-git-credential-helper.md)),
  one binary in layers ([0006](adr/0006-go-git-reads-git-binary-moves-refs.md)),
  the tests and the licences ([0014](adr/0014-hermetic-tests-on-real-repositories.md)),
  and proof in git notes with the opt-in cycle ([0001](adr/0001-check-results-are-signed-git-notes.md)).

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
