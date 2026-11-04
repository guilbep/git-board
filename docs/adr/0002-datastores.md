# ADR 0002: The board is kept in a datastore, cut at the pack

- Status: accepted (2026-11-03); steps 3 to 5 built (2026-11-04), see
  [As built](#as-built)
- Date: 2026-11-03
- Issue #76. The PRD is `docs/prd/datastores.md`.
- Builds on the core's spec: [Data model](../spec/data-model.md),
  [Storage](../spec/storage.md) and [Sync](../spec/sync.md). It changes
  neither the storage format nor the refs.

## Context

git-board keeps the board as entities, each a chain of commits under
`refs/board/<kind>/<id>`, with a local sqlite index (`core/index`).
`internal/store` is what the server, the TUI and the commands read and
write through. Some users won't have that:

- people who never clone;
- a hosted git-board;
- a team that wants its issues in a private PostgreSQL, local or not;
- a forge as the only store.

One user already asked to "export all issues and comments as a folder of
Markdown files". The user should be able to choose where the board is kept:
git, PostgreSQL, Markdown files, or a forge.

Reading the code (see [Evidence](#evidence), where git is assumed today)
shows that "git" means four different things here:

1. **The entity log**: packs in commits, refs, fetch, push, the merge after
   a fetch. Only this is the datastore.
2. **git-board's own refs, which aren't entities**: the counter of project
   numbers (`refs/board/seq`), the reset and switched marks, the copies under
   `refs/board-remotes/`.
3. **The code repository**: branches, diffs, worktrees, tags, and the proof
   notes of ADR 0001. git-board is a tool for projects kept in git. These
   stay git whatever holds the board.
4. **The machine's own state**: the index, the sync lock and its journal,
   the inbox and media caches, all under `<common dir>/board`, and
   `board.me` and `board.view` in git config.

Every id in the system is derived from git:

- an issue's or a person's id is the git blob hash of its first pack;
- an operation's id is `<pack hash>:<index>`;
- `edit-text` and `set-meta` name their target by that id, and so does a
  sync entity's `posted`;
- so do the markers sync leaves on forges (`<!-- git-board comment <id> -->`).

A datastore that hands out its own ids breaks every one of these.

## Decision

### 1. The datastore keeps packs, and only packs

The core writes through one interface. Its unit is the **pack**: the JSON
blob a commit holds today (`{"clock": N, "nonce": …, "ops": [...]}`), byte
for byte. An issue's history is its operations, and the packs are its
operations.

```go
// in core/oplog

// A Datastore keeps entities as packs. It never reads them: oplog encodes,
// hashes and orders them.
type Datastore interface {
	// Heads are a kind's entities, by id, each with its version: an
	// opaque string that changes whenever the entity does. List, the
	// index's refresh and the server's change events read it.
	Heads(ctx context.Context, kind string) (map[string]string, error)
	// Packs are every pack of an entity, in any order, and its version:
	// ErrNotFound when there is none.
	Packs(ctx context.Context, kind, id string) (packs [][]byte, version string, err error)
	// Add stores one pack on an entity if its version is still prev ("" for
	// an entity that must not exist yet), and gives the new version:
	// ErrMoved otherwise.
	Add(ctx context.Context, kind, id string, pack []byte, prev string) (version string, err error)
}
```

- **The pack's hash is computed by oplog**, not by the datastore: the git
  blob id of its bytes, `sha1("blob <len>\0" + bytes)`, which needs no git.
  So an entity's id, its operations' ids and every reference to them are
  the same in every datastore. A board moves out of git by copying its
  packs, and stays the same board. Into git, its packs must be chained into
  commits again (see [Left open](#left-open)).
- **The order stays oplog's**: every pack, sorted by (clock, hash), as
  [Order](../spec/storage.md#order) says. Two packs with the same bytes are
  one.
- **A write is a compare-and-swap** on the version, as `git update-ref
  <ref> <new> <old>` is today. On `ErrMoved`, oplog loads again and writes a
  new pack with the new highest clock + 1, at most 5 tries, as
  [Updating a ref](../spec/storage.md#updating-a-ref) says.
- **The DAG is git's.** In git, a version is a commit, a pack's parents
  are commits, and a merge commit carries an empty pack. A datastore with
  one writer at a time (PostgreSQL) has no merges: its clocks just count.
  Two of the reader's checks are the git datastore's, which alone has
  parents: the clock is 1 + the parents' highest, and a pack with no
  operation is a merge. Another datastore keeps git's merge packs as they
  came, and they add no operation.

Which package sits where:

| Side | Packages |
|---|---|
| Above the interface: reads operations, never storage | `core/issue`, `core/people` (their `Apply` and state rules), `core/syncer` (plan, state, pull, push), `core/forge` and its adapters, `core/index` (it reads `Heads` instead of refs), `internal/*` except the parts below |
| The interface | `core/oplog`: `Op`, `Entry`, `Log`, hashing, order, `Create`/`Load`/`Append`/`List`, which take a `Datastore` instead of a `*gitstore.Store` |
| The git datastore | `core/gitstore`, and oplog's git half (`writePack`, `readNode`, `walk`, `Merge`, `MergeFetched`) in one file, the only one in oplog that imports gitstore |
| Not a datastore at all | the code repository: `internal/store`'s `code.go` and `diff.go`, `internal/proof`, `internal/release`, `internal/cycle`, `internal/branches` |

`core` imports nothing of `internal` (`core/importguard_test.go`), so the
interface lives in core, and a PostgreSQL datastore would be a core
package too (`core/pgstore`).

### 2. What every datastore gives

| It must give | Why |
|---|---|
| **Packs, unchanged**, under the four kinds (`issue`, `person`, `sync`, `release`) | the history as operations; the ids and every reference to them |
| **Compare-and-swap** on an entity's version | two writers never lose a write |
| **Sync entities**, like any other kind | the sync bookkeeping is operations in them: links, last seen (`seen`), `posted`, `refused`, `run`. Echoes are settled by `seen`, by origins and by markers, which are all operations or forge text. A datastore that keeps packs keeps all of it, with no code of its own |
| **Persons**, like any other kind | identities: who wrote each operation, and their forge accounts |

Besides the datastore, git-board needs two things of the machine it runs on:

- **A local folder**, for the index, the sync lock and its journal, and the
  caches (inbox, assignable people, media). For a git board it stays
  `<common dir>/board`. For another datastore it is a folder its
  configuration names.
- **Who I am** (`board.me`) and the view (`board.view`). For a git board,
  git config as today. Elsewhere, a file in that local folder.

Neither is shared, and neither is the board.

### 3. What a datastore may give up, and how a feature says so

Each capability beyond section 2 is a Go interface that a datastore may
also implement. git-board checks for it with a type assertion, never by the
datastore's name.

| Capability | What needs it | git | PostgreSQL | Markdown folder | Forge only |
|---|---|---|---|---|---|
| **Replica**: fetch, push, merge with remotes; the reset and switched marks guard it | offline work with several writers; Sync's git part; Start over; `git board remote` | yes | no: one shared server. Offline only when the server is local | no | no |
| **Counter**: a shared counter with compare-and-swap | project numbers (`CO2-3075`) | `refs/board/seq` | a row | no | the forge's numbers only |
| **git log of the board** | debugging, nothing in the product | yes | no | no | no |
| **A code clone** (not the datastore's: the deployment's) | signed check notes and proof refs (ADR 0001), branches, diffs, the cycle, releases' tags, `verify` | with a clone | with a clone | with a clone | with a clone |

- **Proof is not the datastore's to give.** Check notes and proof refs
  live in the code repository. A PostgreSQL board in a clone keeps them. Only
  a git-board with no clone (a hosted one) goes without them, and so without
  the cycle's gates.
- **A missing capability is one error**, `store.ErrMissing`, wrapped with
  the sentence the user reads: "project numbers need a datastore that
  counts; this board is kept in PostgreSQL without one". The CLI prints it.
  The server answers it as it answers a refusal.
- **The UI doesn't offer what can't work.** The server's API lists the
  datastore's capabilities, beside what `GET /api/config` gives. A control that needs a missing one is shown
  disabled, with the same sentence as its title. It isn't hidden: the user
  learns why.

### 4. A forge as the only store: a disposable local copy

A forge doesn't keep git-board's packs. So it can't give section 2 by
itself: its history is its own timeline, not operations, and its ids are
its own. This is the tension with the PRD, and this decision settles it by
keeping a copy.

**A forge-only board is a disposable local copy: a git datastore in the
local folder, with no remote, that pull fills from the forge.** Its sync
bookkeeping is that copy's own sync entity. Each write goes to the forge at
once (the per-issue sync, `SyncIssue`), and the copy can be thrown away and
pulled again. That is what tells it from the datastore beside a forge:
throwing it away loses only the writes the forge hadn't got yet.

The three homes for its sync bookkeeping:

| Home | For | Against |
|---|---|---|
| **A local file**: the disposable copy's own sync entity (recommended) | nothing new: sync works as today. A lost copy costs one full listing, and no duplicate (below) | a write that the forge never got dies with the machine |
| The datastore beside it (git or PostgreSQL) | everything works as today | it isn't "forge only" any more: it is today's board with a forge sync |
| Markers on the forge | they travel with the forge | titles, statuses, labels and assignees carry no marker (sync.md, [The forge's form](../spec/sync.md#the-forges-form)), so `seen` has nowhere to go. Writing it in the text would edit every issue and comment, and notify everyone |

Why a copy that is lost costs nothing:

- **Everything in a pulled copy came from the forge.** Each field's last
  operation has the forge as its `origin`, so sync compares with that value
  and not with `seen` ([What git-board took from a forge](../spec/sync.md#what-git-board-took-from-a-forge)).
  Nothing goes back, and `seen` isn't needed.
- **Links come back from origins**: a forge issue unlinked, with an issue
  whose `create` has it as origin, is linked to it ([Crash recovery](../spec/sync.md#crash-recovery)).
- **The copy's own writes are pushed at once.** Until the push lands, the
  issue shows "not on the forge yet", as a write that waits.

What it gives up, besides section 3: its history is what its pulls saw,
not every change (the forge's timeline holds those, one link away); and it
needs the network for every write.

**One rule changes**, and only in this mode. Today a marker from sync's
accounts naming an id that isn't here is skipped until that id arrives
([Echoes](../spec/sync.md#echoes)). A forge-only copy has nowhere for it to
arrive from: a second machine, or a copy pulled again, would skip its own
comments forever. In this mode a marker naming an unknown id is pulled as
new, as anyone's text is.

A forge-only board that also syncs to a second forge has a store of its own
to keep: choose git or PostgreSQL then.

### 5. Markdown is a bridge first

A folder of Markdown holds a state, not operations, so it fails section 2
as a datastore. It is first a bridge: an export, then an import.

**Export** (`git board export --md <dir>`, this issue's milestone 2):

- One file per issue, `NNN-slug.md`:
  - `NNN` is the issue's project number when the project numbers its issues,
    else its first forge number, else its id's first 7 digits (`BareNumber`,
    then the id);
  - `slug` is the title in lower case, with every run of characters other
    than `a-z` and `0-9` turned into one `-`, at most 50 characters.
- The name is for people. **The id in the front matter is what names the
  issue.** When two issues give the same name, the one whose id sorts later
  takes `-<7 digits of its id>`. When a title changes, the file of that id
  under its old name is removed.
- The front matter is YAML (`go.yaml.in/yaml/v3`, already a dependency):
  - `id`, `number`, `title`, `state`;
  - `fields`: the issue's fields as `.git-board.toml` names them, value by
    field;
  - `labels`: the plain labels;
  - `people`: the author and the assignees;
  - `created`, `updated`, in RFC 3339, UTC.
- Then the description. Then each comment, in order:
  - a heading with its author and its date;
  - its text;
  - the marker sync already writes, `<!-- git-board comment <id> -->`, so an
    import finds which comment it is.
- **A second run with no change writes nothing**: a file is written only
  when its bytes differ. A test reads the folder back and compares the
  issues, the comments and the fields.

**Import** (next issue): the folder is a forge. A `core/forge` adapter over a
folder gives sync its issues and takes its writes. Import is then a pull,
and export with sync is a push:

- the folder's last seen values live in a sync entity keyed by the folder,
  as a forge project's do;
- an edit made by hand is a forge change, and conflicts follow
  [The four cases](../spec/sync.md#the-four-cases);
- the comment markers say which comment is which;
- a new file with no `id` is a new issue.

So import needs no rule of its own, and the "every issue and comment as a
folder of Markdown files" layout is a second shape of the same adapter.

**As a datastore** (not planned): a folder that keeps each entity's packs,
as one JSON line per pack, beside the Markdown it renders. It would give
section 2 and a `git diff`-able board without git-board's refs. It waits
for someone to ask.

### 6. The order

1. **Markdown export.** Milestone 2 of this issue. It reads through
   `internal/store` and needs none of section 1.
2. **Markdown import.** The folder as a forge (section 5). The next issue.
3. **The cut.** `oplog`, `issue`, `people`, `syncer` and `index` take a
   `Datastore`, and `internal/store` holds one. The git datastore behaves
   exactly as today: same refs, same commits, same packs, every test
   unchanged. It lands alone, before any second datastore.
4. **PostgreSQL**:
   - two tables, `packs (kind, id, hash, data)` and
     `entities (kind, id, version)`;
   - `Add` is one transaction that moves the version only from `prev`;
   - Counter is a row;
   - it needs `pgx`, a new dependency: the standard library has no
     PostgreSQL driver.
5. **Forge only**, after PostgreSQL, if someone asks.

## Alternatives considered

Scored on: keeps the ids and the history as operations (30), git unchanged
(25), cost to add a datastore (25), a small interface (20).

| Option | Score | Why not |
| --- | --- | --- |
| **Packs (this decision)** | 88 | |
| Operations: the datastore stores an entity's ops and hands out their ids | 55 | Every id, target, `posted` and marker names a pack hash. A datastore's own ids would make the same board different ids in two datastores, so nothing moves or syncs between them |
| State: the datastore stores each issue as it is now (CRUD) | 30 | Loses the history, the order every clone agrees on, `changes` (which sync's conflicts and "what git-board took from a forge" read), and the echoes |
| `internal/store`'s `Backend` as the interface | 40 | Over 150 methods. Each datastore would build numbering, sync, releases and resets again |
| Git only, with exports and imports as bridges | 60 | Cheapest, but a team that wants PostgreSQL gets a copy, not a store |

## Consequences

Good:

- A board keeps its ids in every datastore, so it leaves git by copying
  its packs, and markers already on forges stay true.
- Sync, its bookkeeping and its echoes work unchanged on any datastore that
  keeps packs: they are operations.
- The interface is three methods. The git datastore is today's code behind
  them.
- A feature that can't work says why, in the UI and in the CLI.

Bad, and accepted:

- A datastore stores bytes it can't query: PostgreSQL holds packs, not
  rows of issues. Queries keep going through the local sqlite index. A
  hosted git-board that can't keep a local folder would need the index in
  PostgreSQL: a later decision.
- The cut (step 3) touches every caller of `*gitstore.Store` in core and in
  `internal/store`, as listed under Evidence, for no visible change.
- A forge-only board loses what its machine hadn't pushed yet, and its
  history is what its pulls saw.
- PostgreSQL adds a dependency.

Unchanged:

- The storage format, the refs, and, for now, the spec's pages. When
  forge-only is built, [Echoes](../spec/sync.md#echoes) gains its one rule
  (section 4), for that mode only.
- `.git-board.toml` stays in the code checkout. Its defaults (#75) aren't
  touched here.

## As built

Steps 2 (Markdown import), 3 (the cut), 4 (PostgreSQL) and 5 (forge
only) landed on 2026-11-04, and the Markdown folder as a datastore with
them. Where the
build differs from the decision above, and why:

- **The interface is `core/datastore`, not `core/oplog`.** `*gitstore.Store`
  implements it itself, so every caller and test that held a git store
  keeps working; gitstore can't import oplog, which imports it for
  `Merge`. `oplog.Datastore` names the same interface.
- **The interface has a fourth method, `Folder`**: the machine's own
  folder (section 2), which the index, the sync lock and the caches need.
  For git it stays `<common dir>/board`.
- **The git datastore's checks live in `gitstore`** (`Walk`, `ReadNode`):
  a pack's clock and the merge rule read parents, which only git has.
  `Merge` and `MergeFetched` stay oplog's git half (`merge.go`).
- **A malformed operation is named by its pack's id**, not by its commit:
  the same message in every datastore.
- **`ErrRefMoved` is `datastore.ErrMoved`**, so a caller of either checks
  one error.

What this ADR left open is settled:

- **Where a datastore is configured:** `board.datastore` in the clone's
  git config (`git`, `forge`, or a `postgres://` URL), over which
  `GIT_BOARD_DATASTORE` wins. Never `.git-board.toml`: a URL may hold a
  password. Settings › Datastore edits it, tests a URL and copies a git
  board in; a change takes effect when the server starts again.
- **The PostgreSQL index replaces sqlite**: the same tables, in the schema
  `git_board_index`, shared by every machine; its search tokenizes as
  unicode61 does, and every index test runs on it (`make test-pg`).
- **Who I am** stays `board.me` in the clone's git config, whatever keeps
  the board: it is the clone's, not the board's. `<folder>/me` is only for
  a datastore with no clone.
- **`core/watch`** reads `Heads`; the server's change events compare the
  datastore's heads.
- **Copying a board out of git** is `git board datastore copy` (and the
  tab's button): packs as they are, the counter with them. Into git stays
  open.
- **Forge only** sends each write from `git board serve` at once; a
  write from the command line goes with the next sync. Its re-pulled texts
  keep the header sync posted them with, as anyone's text does.

Markdown, as built, differs from section 5:

- **Import is not a forge adapter.** `git board import --md` reads the
  files straight into operations: a file's `updated` against the issue's
  is the last seen (a field changed on both sides is a conflict, the
  board's kept), names find the people, and a new file's id is written
  back. A folder adapter for sync would have taken every author as sync's
  account and every pulled text with sync's header; this keeps them.
- **The Markdown folder is a datastore** (`core/mdstore`,
  `board.datastore markdown:<dir>`): packs as JSON lines in
  `<dir>/.board/<kind>/<id>.jsonl`, merged by git's union driver; a
  version is the count of lines, a write takes the file's lock. After
  each write, the backend reads the folder's hand edits in, then renders
  the files.

## Left open

- The local folder's default path for a board with no clone (a hosted
  git-board).
- Whether `Heads` should return the versions of every kind in one call: the
  index reads two kinds and the change events three.
- Copying a board into git: its packs must be chained into commits again,
  clock by clock, and packs of one clock need a merge each.
- The pages don't yet disable a control a datastore can't run (pull, push,
  start over): the server refuses it, 409 `missing`, with the sentence.

## Evidence

Where git is assumed today, read on 2026-11-03 at fd9df99. Tests are left
out.

### The entity log: behind the interface

- `core/gitstore/store.go`:
  - `:64` `Open`: go-git, plus the git binary;
  - `:99`–`:181` blobs, trees, commits;
  - `:204` `Ref`, `:220` `Refs`;
  - `:289` `UpdateRef`, the compare-and-swap through `git update-ref`.
- `core/gitstore/remote.go`: `:11` `Kinds`; `:25` `Fetch` and `:45` `Push`,
  the refspecs.
- `core/oplog/oplog.go`:
  - `:77` `RefName`, `refs/board/<kind>/<id>`;
  - `:80` `IsID`: 40 hex digits, SHA-1's;
  - `:116` `Create`, `:146` `Load`, `:169` `Append`, `:207` `List`;
  - `:229` `writePack`, `:251` `readNode`, `:293` `walk`: commits and
    parents;
  - `:322` `order`: (clock, blob hash).
- `core/oplog/merge.go`: `:18` `Merge`, merge commits; `:72` `MergeFetched`,
  `refs/board-remotes/`.
- Callers that take a `*gitstore.Store`:
  - `core/issue/ops.go`: `:60`, `:75`, `:84`, `:87`;
  - `core/people/ops.go`: `:40`, `:56`, `:65`, `:68`;
  - `core/people/directory.go:10`;
  - `core/syncer/sync.go`: `:97`, `:102`, `:108`, `:150`;
  - `core/syncer/state.go`: `:78` `loadState`, `:239` `writeOps`;
  - `core/syncer/links.go`: `:31`, `:48`;
  - `core/syncer/pull.go:28`;
  - `core/forge/conformance/conformance.go`: `:90`, `:148`.
- `core/index/index.go`:
  - `:33` `entities.head`, a commit hash;
  - `:150` `Refresh`: lists refs, and compares heads.
- `core/watch/watch.go:33`: polls the refs.
- `internal/store/store.go`:
  - `:144` the `Backend` holds a `*gitstore.Store`, and `:218` `Store()`
    hands it out;
  - `:289` `loadIssue` caches by head;
  - `:310` `all` lists refs;
  - `:648` `Pull`, `:663` `Push`, `:677` `Fetch`.
- `internal/store/history.go`:
  - `:72` `RefsFingerprint`;
  - `:80` `ReleasesFingerprint`, which also reads `refs/tags/`;
  - `:84` `fingerprint`, read by `internal/server/events.go:170` and `:196`.
- `internal/store/releases.go:249` `pushKinds`: a refspec per kind (`:263`).
- `internal/store/body.go:190` `meanings`: takes a `*gitstore.Store`.
- `internal/store/whole.go`: `:46` `SyncWhole`, `:111` `PullStep`, `:147`
  `PushStep`: the git part of Sync.

### git-board's own refs: capabilities

- **The counter**, `internal/store/numbers.go`:
  - `:39` `refs/board/seq`;
  - `:130` `readCounter`, `:454` `pushCounter`, `:477` `commitCounter`;
  - `internal/server/numbers.go:209`.
- **The reset mark**, `internal/store/reset.go`:
  - `:28` `refs/board/reset`;
  - `:157` `ResetHard`, with a git bundle as its backup (`:233`);
  - `:267` `PushPrune`, `refs/board-pruned/`;
  - `cmd/git-board/reset.go:87`.
- **The switched mark**, `internal/store/forges.go`: `:262`
  `refs/board/switched`, `:299` `fetchMark`, `:317` `pushMark`.
- **The dry runs' scratch copy**, `internal/store/forges.go:348`, `:355`: a
  throwaway git repository.
- **What a push or a pull would move**, `internal/store/remotes.go`: `:88`
  `ahead`, `:130` `compare` (`refs/board-remotes/`).
- **The switch from git-bug**:
  - `internal/store/switch.go`: `:160`, `:212` (`refs/bugs/`), `:358`,
    `:757` (`refs/releases/`);
  - `internal/store/prune.go`: `:20`, `:33`;
  - `internal/server/startover.go:43`.

### The code repository: not the datastore

- `internal/store/code.go`: branches (`:36`, `refs/heads/` and
  `refs/remotes/`), `:105` go-git on the common dir, worktrees (`:160`).
- `internal/store/diff.go:22`: `git` for rev-parse, merge-base, diff, log.
- `internal/proof/notes.go`: `:15` `refs/notes/ci/<writer>`, `:47` `Append`,
  `:94` `Lines` (ADR 0001).
- `internal/release/git.go`: `:53` tags, `:93` `PushTag`, `:147` the config
  repository.
- `internal/cycle/gather.go:188`, `internal/branches/branches.go:280`,
  `internal/server/work.go:137`: commits read.
- `cmd/git-board/verify.go:80`: `refs/remotes/origin/HEAD`.

### The machine's own state: a local folder

- `core/index/index.go:62`: `<common dir>/board/index.db`.
- `core/syncer/lock.go`: `:23` `<common dir>/board/sync.lock`, `:71`
  `Running`.
- `core/syncer/journal.go`: `:213` `Cut`, `:260` `Forget`.
- `internal/store/store.go:228`: `Storage()`, `<common dir>/board`, read
  by:
  - the inbox (`internal/server/inbox.go:68`);
  - the assignable people (`internal/server/assignable.go:91`);
  - the media (`internal/server/issues.go:100`).
- `core/people/ops.go`: `:92` `Me` and `:95` `SetMe`, in git config
  `board.me`.
- `internal/store/store.go:635` `SetView`, git config `board.view`.
- `internal/store/tokens.go`: tokens from git's credential helper. Not the
  board's, and kept as it is.

Already outside git: attachments go to S3 (`internal/uploads`).
