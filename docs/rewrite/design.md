# git-board v2: the design of a rewrite

- Status: accepted ([ADR 0021](../adr/0021-git-board-v2-same-data-new-code.md),
  2026-11-05), and built: milestones M0 to M7 (section 5). v2 is
  git-board since the switch, v0.12.0.
- Asked: "start on a complete rewrite from scratch knowing what you
  learned this night, with elegance/modularity and speed in mind".
- Builds on every ADR, most of all [ADR 0002](../adr/0002-datastores.md)
  (datastores), [ADR 0004](../adr/0004-entities-are-operation-logs.md)
  (operation logs), [ADR 0009](../adr/0009-queries-read-a-disposable-index.md)
  (the index), [ADR 0018](../adr/0018-own-your-issues-work-in-progress-is-opt-in.md)
  (what the core is for) and [ADR 0020](../adr/0020-the-core-in-layers.md)
  (the layers), and on [Performance](../guide/performance.md).

The vision does not change: you own your issues. They live in your
repository, or a database or folder you choose, and no forge is needed to
read or write them. The rewrite is about the code: it rewrites the code
and keeps the data. A board written by v1 opens in v2, and the other way
round, for as long as both exist.

Reading time: about 20 minutes.

## 1. What stays

### The data: one format, every datastore

v2 reads and writes the boards v1 wrote, byte for byte. A user never
migrates anything. These documents are the contract; where the code and
the document disagree, the document wins, and the code that wrote the
data decides what the document should have said.

| Contract | Defined in | Holds |
|---|---|---|
| Entities, operations, ids, the four kinds' operations and states | [Data model](../spec/data-model.md) | `issue`, `person`, `sync`, `release`; every `kind` and `data` field; ids; how a state folds |
| The git format | [Storage](../spec/storage.md) | `refs/board/<kind>/<id>`, commits, packs, the clock, the order (clock, then pack hash), merges, the compare-and-swap, fetch and push refspecs, `refs/board-remotes/` |
| Query semantics | [Queries and search](../spec/queries.md) | what a filter, free text, a prefix, `Labels` and a change event answer. Not the index's tables: they are disposable |
| Sync | [Sync](../spec/sync.md), [Forges](../spec/forges.md) | links, last seen, posted, refused, runs; the markers in forge texts; the journal and `sync.lock` |
| Project numbers | [ADR 0016](../adr/0016-project-numbers-are-a-counter-on-origin.md) | `refs/board/seq`, its `numbers` file (`internal/store/numbers.go`, `ParseCounter`) |
| Check results | [ADR 0001](../adr/0001-check-results-are-signed-git-notes.md) | `refs/notes/ci/*`, signed lines |
| Tombstones | [ADR 0019](../adr/0019-deleting-is-a-tombstone.md) | `delete` hides, history keeps |
| PostgreSQL | `core/pgstore/pgstore.go`, the `schema` constant | `git_board.meta` (version 1), `entities`, `packs`, `counter` |
| Markdown folder | [Where the board is kept](../guide/datastores.md), `core/mdstore`, `internal/store/markdown.go` and `mdimport.go` | `<dir>/.board/<kind>/<id>.jsonl`, one pack a line; `.gitattributes` (union, `numbers` binary); `board.lock`; the rendered `NNN-slug.md` |
| Settings | [Configuration](../guide/configuration.md); `board.me`, `board.datastore`, `GIT_BOARD_DATASTORE` | `.git-board.toml`, each table; the clone's git config |

Three of these were only code: the PostgreSQL tables, the Markdown
folder and the counter's file. Milestone M0 wrote them into [Other
datastores and the counter](../spec/datastores.md) before any v2 code
reads them, so the contract is a document and not a reading of v1. The
golden boards and v1's outputs on them are in [v2: the golden boards and
make compat](compat.md).

Two local files would fight if v1 and v2 shared them, so v2 does not:

- **The index.** v1 deletes `.git/board/index.db` when its version isn't
  its own. v2 keeps its own, `.git/board/v2/view.db`, and on PostgreSQL
  its own schema, `git_board_view_v1`. Both are disposable.
- **The sync lock.** v2 takes the same `.git/board/sync.lock` and reads
  the same journal, so v1 and v2 never sync one board at once.

### The surfaces: what users and the web app call

| Contract | Defined in |
|---|---|
| Commands, flags, arguments | [Issues from the command line](../guide/cli.md), and `cmd/git-board` for what the guide leaves out |
| JSON field names | [JSON output](../guide/cli.md#json-output); the tags in `core/model/*.go`, `core/release`, `core/cycle` |
| Exit codes | [Exit codes](../guide/cli.md#exit-codes): 0 to 5 |
| The HTTP API | the routes in `internal/server/server.go` (about 120), and their types in `web/src/api/board.ts` (1,741 lines), the only typed record of them |
| The API's error codes | `writeCoded` in `internal/server`: `{"code": "changed"}`, `"syncing"`, `"missing"`, `"experimental"` and 21 more, which `web/src/apierror.ts` reads |
| The change events | `GET /api/events` (`internal/server/events.go`) |

The HTTP API has no document. M0 records it: a golden transcript per
route, made by v1 on a fixed board. v2 must answer each one the same.
The web app (`web/`) is not rewritten: it is the HTTP API's first
client, and its e2e tests run against v2 unchanged.

What is not a contract: `--help` texts, log lines, the TUI's keys, the
index's tables, the Go API of `core/` (no one outside imports it yet).

## 2. What we learned

Each point is a mistake in the code's shape, not in the data. Each has
its evidence.

### 2.1 Fold on demand, then caches, then caches of caches

v1 folds an entity from its packs when it is asked for, and every speed
fix since added a cache beside it. `internal/store.Backend` holds four:
issues and persons folded per head (`mu`), the last listing and its heads
(`listMu`, 92abf37), the forges' links per sync head (`recMu`), and the
index's states, which are a fourth copy of each issue (1708f4e). Each is
correct, and keyed by heads (performance.md: "none trusts a time"); none
knows the others. An issue has four shapes: `issue.Issue` (the fold),
`index.State` (its JSON), `index.Summary` (the index's row) and
`model.Issue` (what surfaces show, folded again in `internal/store`).

**v2:** one read model, the view, kept up to date incrementally. It is
the only place a state is kept. A kind has two shapes, its state and its
row; the surfaces' JSON is made from them in one place.

### 2.2 A facade that grew into the application

`internal/store` is 7,914 lines and 5,776 of tests. `Backend` has 125
exported methods; 113 take no `context.Context`. Its fields include
function hooks (`OnWrite`, `Tokens`, `Approve`) and two atomic pointers
the core needs from the config (`SetFlatKinds`, `SetHeld`). It was
`gitbugio`, the git-bug wrapper (its package comment says so), and kept
growing after git-bug left (ADR 0003). The core doubled its API to add
contexts late: `issue.Load` and `LoadContext`, `List` and `ListContext`.

**v2:** no facade. Domains are services with small APIs, every call
takes a context, and hooks are observers registered on the board.

### 2.3 The server is a second implementation

`internal/server` is 9,275 lines and 9,216 of tests, about 120 routes.
Domain rules live in its handlers. `handleComment`
(`internal/server/issues.go`) checks a review thread's anchor, refuses a
reply to anything but a thread's first comment, and marks a
```` ```suggestion ```` fence, all in the HTTP layer: the CLI has none of
it. Errors map twice, by hand: `writeIssueError` sends 404 for not found
and 400 for everything else, an internal error included; the CLI's
`exitCode` (`cmd/git-board/main.go`) has its own switch, down to a string
prefix, `"unknown command"`. One `writeMu` serializes every write, and a
forge sync holds it for minutes, so the page answers 409 `syncing` to
every edit meanwhile (`beginWrite`).

**v2:** one application API. The CLI, the TUI and HTTP are adapters that
decode a request, call it, and encode the answer. One error type carries
the exit code and the HTTP status. Writes don't wait for a sync: the
compare-and-swap already keeps them safe.

### 2.4 Kinds as special cases

The kinds are a global list (`datastore.Kinds`). The index imports
`core/issue` and indexes issues and persons only (ADR 0009). Until
tonight a release's fold lived in `internal/store/releases.go`, next to
the web server (b5a488d). The syncer syncs issues only, and releases
reach GitHub through `gh` in `internal/ship/binaries.go` (ADR 0020, *Left
open*). People and the forges' links are folded again by every command
(performance.md, *What's left*).

**v2:** a kind is a value registered on the kernel. The view, the sync
engine, fetch, push and change events work on any registered kind.

### 2.5 One config struct that knows every domain

`internal/config` is 3,347 lines and 2,984 of tests. Its `raw` struct
decodes every table, so it imports `core/cycle`, `core/fields`,
`core/proof` and `core/release`. Writing a table back is a hand-written
text editor per table: `setsection.go`, 751 lines (`SetFieldsText`,
`SetCycleText`, `SetReleaseText`, …). A broken `[cycle]` broke every
command, `ls` included, until ADR 0018 special-cased experimental tables.
ADR 0020 left the `[release]` and `[cycle]` checks in `internal/config`.

**v2:** each domain declares its table: its type, its defaults, its
check. The file is read once; a broken table is its own domain's
problem. One generic writer replaces a table's text and leaves the rest
of the file, comments included, as it was.

### 2.6 Concurrency found by luck

`TestGetIssueCacheConcurrent` failed 10 times in 400 for months
(fbda0b3, "the same before tonight's perf work"). go-billy's `ReadDir`
lstat'ed each ref while `git update-ref` renamed `<ref>.lock` over it;
the walk took the error for an empty directory, `Heads` listed no issue,
and the index dropped them all. The same commit found the loose refs
must be read before `packed-refs`, as git does. The Markdown store could
read a pack half written (12cfe5d, once in 3,000 appends). The datastore
suite tested "read while written" only after (36b431e).

**v2:** the datastore contract says what a listing guarantees under
concurrent writers, and the conformance suite proves it on every
datastore, with real `git update-ref` and `git pack-refs` racing the
reader. And the view never drops an entity because a listing missed it:
it asks for that entity before dropping it.

### 2.7 Speed was never budgeted

Tonight's profile (performance.md) found, on 20,000 issues, `ls` at 77 s:

- `packed-refs` read line by line for each ref looked up: n² (1136ab6);
- the pack file opened 75,000 times for 5,000 issues (85dcb43);
- every `ls` read and folded every issue (1708f4e: 3.2 s to 0.4 s on
  5,000);
- the labels table scanned once per issue on a first build (4dba672);
- a regexp per id, the free text cut again for each word (07b79d0,
  829c8d5);
- one `git update-ref` process per ref moved, so a fetch that merges
  1,000 issues starts 1,000 processes (`core/gitstore/store.go:305`).

After the fixes, `ls` is 1.5 s and a first `ls` 16 s. On a board of
2,000 issues (`cmd/boardgen`), v1's first `ls` takes 1.26 s where `git
cat-file` reads every object of the board in 0.17 s. The benchmarks and
the synthetic board came last (813006c, tonight).

**v2:** budgets in section 4, benchmarks from the first milestone, and a
CI job that fails when a budget is broken.

### 2.8 Layers drawn after the fact

ADR 0020's four steps moved about ten packages, in 26 commits
of one evening. It names the cost:
`core/model` instead of `core/board` because "a package board importing
a package board needs an alias"; `internal/branches` and
`internal/markdown` stay outside because moving them would drag a table
along. The guard (`core/layers_test.go`) came with step 1. The layers are
right; drawing them on 55,000 lines is what is slow.

**v2:** the layers and the import guard exist before the first line of
domain code.

### What was right, and stays

The format (operation logs, packs hashed as git blobs, the clock order);
cutting the board at the pack (ADR 0002), so git, PostgreSQL, a folder
and the forges alone are one interface; fields as labels (ADR 0007); a
disposable index (ADR 0009); hermetic tests on real repositories
(ADR 0014); one static binary (ADR 0012); the forge adapters and their
conformance suite (`core/forge/conformance`).

## 3. The architecture

### Packages

v2 grew in `next/` (section 5), and is the module's root since the switch
(M7): the paths below are the root's. Each layer imports only the layers
above it in this table; a test enforces it from day one (`layers_test.go`).

| Package | Purpose |
|---|---|
| `kernel` | entities, operations, ids, the order, kinds, the board: open, read, write, watch |
| `kernel/pack` | the format's bytes: encode a pack, hash it as a git blob, sort by (clock, hash) |
| `store` | the `Datastore` interface, its capabilities and errors |
| `store/git`, `pg`, `md`, `forgecopy`, `mem` | the datastores; `mem` for tests |
| `store/storetest` | the conformance suite every datastore runs |
| `view` | the materialized state: rows, terms, text, heads; sqlite and PostgreSQL |
| `conf` | `.git-board.toml`: tables registered by domains, read once, written table by table |
| `forge`, `forge/github`, `forge/gitlab`, `forge/fake`, `forge/forgetest` | the forges' adapters and their conformance suite |
| `sync` | the sync engine, generic over kinds: plan, pull, push, records, journal, lock |
| `domain/people` | the `person` kind; who owns an account; me |
| `domain/issues` | the `issue` kind; fields (ADR 0007), filters, search, the inbox, numbers |
| `domain/releases` | the `release` kind; versions, overlays, proof (ADR 0001) |
| `domain/cycle` | an issue's steps; experimental (ADR 0018) |
| `app` | opens a board, wires the domains, the one API every surface calls, errors |
| `surface/cli`, `http`, `tui` | adapters over `app`; `http` serves the web app |
| `tools/…` | the GitHub Project bridge, the release runner: apps over `app` (agents left: section 5) |
| `cmd/git-board` | `main`: the binary |

`domain/*` never imports another domain upward: releases read issues,
the cycle reads both, issues read people. A domain imports `kernel`,
`conf`, `sync` and `forge`; never `view` or `store` directly.

### The kernel's API

```go
package kernel

// ID names an entity: 40 lowercase hex digits.
type ID string

// OpID names an operation: "<pack hash>:<index>".
type OpID string

// Version is a datastore's opaque version of an entity: a commit in git.
type Version string

// Op is one operation, as docs/spec/data-model.md writes it.
type Op struct {
	Kind   string          `json:"kind"`
	Author ID              `json:"author,omitempty"`
	Time   int64           `json:"time"`
	Origin *Origin         `json:"origin,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

// Entry is an operation in its entity's order.
type Entry struct {
	ID    OpID
	Clock uint64
	Op
}

// Spec defines a kind. S is its state, R its row: what lists show
// and filter on.
type Spec[S, R any] struct {
	Name  string // the ref segment: refs/board/<Name>/<id>
	Given bool   // ids are given (sync: SHA-1 of the forge key), not the first pack's hash
	Rev   int    // bumped when Apply or Index change: the view rebuilds this kind
	New   func(id ID) S
	// Apply applies one operation. An unknown kind is an error: the
	// kernel never drops an operation (data-model.md, Operations).
	Apply func(s *S, e Entry) error
	// Index is what the view keeps of a state.
	Index func(s *S) Doc[R]
	// Row encodes and decodes R; nil is JSON. Issues bring a fast one.
	Row Codec[R]
}

// Doc is a state as the view keeps it.
type Doc[R any] struct {
	Row     R
	Terms   []string // exact terms, ns:value (ADR 0007): status:open, bug, author:<id>, account:github.com/42
	Texts   []Text   // free text by field: title, body, comments
	Created int64
	Updated int64
	Hidden  bool // a tombstone (ADR 0019): kept, never listed
}

type Text struct{ Field, Body string }

// K is a registered kind, typed.
type K[S, R any] struct{ /* … */ }

// Define makes a kind. A domain calls it once, in a package variable,
// and hands it to Open.
func Define[S, R any](s Spec[S, R]) *K[S, R]

// Entity is one entity's state at a version.
type Entity[S any] struct {
	ID      ID
	Version Version
	Clock   uint64 // its head's clock: the next pack's is Clock+1
	State   S
}

func (k *K[S, R]) Get(ctx context.Context, b *Board, id ID) (Entity[S], error)
// At is the entity as it was at a time: ls --at, show --at.
func (k *K[S, R]) At(ctx context.Context, b *Board, id ID, at int64) (Entity[S], error)
// Log is every operation in order: log, undo, diff.
func (k *K[S, R]) Log(ctx context.Context, b *Board, id ID) ([]Entry, Version, error)
// Create writes a new entity's first pack. id is "" unless the kind is Given.
func (k *K[S, R]) Create(ctx context.Context, b *Board, id ID, ops ...Op) (Entity[S], error)
// Do reads the entity, asks decide for the operations, and writes them.
// When the entity moved meanwhile it reads again and asks again (5
// tries): a decision always sees the head it is written on.
func (k *K[S, R]) Do(ctx context.Context, b *Board, id ID,
	decide func(cur Entity[S]) ([]Op, error)) (Entity[S], error)
func (k *K[S, R]) List(ctx context.Context, b *Board, q Query) ([]Listed[R], error)
func (k *K[S, R]) Resolve(ctx context.Context, b *Board, prefix string) (ID, error)

type Listed[R any] struct {
	ID      ID
	Version Version
	Row     R
}

// Query is what the view answers; queries.md gives the semantics.
type Query struct {
	All   []string   // every term
	Any   [][]string // one term of each group
	Not   []string
	Text  string
	In    []string // title, body, comments
	Sort  string   // "created", "updated"
	Desc  bool
	Limit int
}

// Kind is a K without its types, for Open.
type Kind interface{ Name() string }

// Board is an open board: a datastore and its view.
type Board struct{ /* … */ }

func Open(ctx context.Context, ds store.Datastore, v view.View, kinds ...Kind) (*Board, error)

// Fresh brings the view to the datastore's heads. Every Get and List
// calls it; it costs one listing of the heads when nothing moved.
func (b *Board) Fresh(ctx context.Context) error

// Watch tells what changed, whoever changed it (queries.md, Change events).
func (b *Board) Watch(ctx context.Context, every time.Duration) <-chan []Change

// Observe registers f, told of each change this process wrote, once it
// landed: the Markdown render, the forge-only sender.
func (b *Board) Observe(f func(ctx context.Context, c []Change))

func (b *Board) Broken(ctx context.Context) ([]Broken, error)
func (b *Board) Store() store.Datastore
func (b *Board) Close() error

type Change struct {
	Kind    string
	ID      ID
	Version Version // "" when gone
}

type Broken struct {
	Kind string
	ID   ID
	Err  error
}

// Errors.
var (
	ErrNotFound  = errors.New("not found")
	ErrAmbiguous = errors.New("ambiguous id prefix")
	ErrMoved     = store.ErrMoved // after 5 tries
)
```

A domain is then a few lines of registration and its rules:

```go
package issues

var Kind = kernel.Define(kernel.Spec[Issue, Row]{
	Name: "issue", Rev: 1,
	New:   func(id kernel.ID) Issue { return Issue{ID: id} },
	Apply: apply, // the eight operations of data-model.md › Issues
	Index: index, // labels as terms, title/body/comments as texts
	Row:   rowCodec{},
})
```

`person`, `sync` and `release` are defined the same way, in their
domains. A fifth kind is a new `Define`, and nothing else in the kernel,
the view or fetch and push changes. The refspecs come from the
registered kinds.

### One Datastore, with capabilities

```go
package store

type Datastore interface {
	// Heads lists every entity of the kinds, each with its version. It
	// is snapshot-safe: an entity that exists from the call's start to
	// its end is listed, whatever writers do meanwhile. A ref whose name
	// isn't an id is skipped (storage.md, Refs).
	Heads(ctx context.Context, kinds ...string) ([]Head, error)
	// Load streams the packs of each ref, at its version ("" for the
	// current one), in any order, as each is read. Load of one ref is a
	// point read; of 20,000, a first build.
	Load(ctx context.Context, refs []Ref, each func(Ref, [][]byte) error) error
	// Add stores one pack if the entity is at prev ("" for a new one),
	// and gives the new version; ErrMoved otherwise.
	Add(ctx context.Context, kind, id string, pack []byte, prev string) (string, error)
	Caps() Caps
	// Folder is this machine's own folder for the board: the view, the
	// sync lock and its journal. Never shared, never the board.
	Folder() string
	Close() error
}

type Head struct{ Kind, ID, Version string }
type Ref struct{ Kind, ID, Version string }

// Caps says what a datastore can do beyond the interface. A nil field
// is a capability it lacks; a feature that needs it fails with
// ErrMissing and a sentence the user reads (ADR 0002 §3).
type Caps struct {
	Pinned  bool    // a version names its packs for good (git): no recheck
	Shared  bool    // every machine reads the same store (PostgreSQL)
	Counter Counter // project numbers (ADR 0016)
	Locker  Locker  // a lock every machine shares (PostgreSQL)
	Remote  Remote  // fetch, push, start over (git)
	SQL     SQL     // the view may live here (PostgreSQL)
}

type Remote interface {
	// Fetch fetches the kinds' refs and merges them (storage.md, Merging
	// two heads), all ref moves in one git update-ref --stdin.
	Fetch(ctx context.Context, remote string, kinds ...string) ([]Head, error)
	Push(ctx context.Context, remote string, kinds ...string) error
	Drop(ctx context.Context, kinds ...string) error
}

var (
	ErrNotFound = errors.New("store: no such entity")
	ErrMoved    = errors.New("store: entity moved since it was read")
	ErrMissing  = errors.New("store: capability missing")
	ErrHeld     = errors.New("store: lock held")
)
```

What changes from v1's `core/datastore`: `Heads` takes every kind in one
call (ADR 0002, *Left open*); `Load` reads many entities at once, at
their versions, so a first build streams (`PacksAt` was a single read);
capabilities are a struct, not type assertions, so a wrapper (a counting
store, a test fake) forwards them by copying a value.

**The git datastore** reads refs itself: the loose refs with `os.ReadDir`
first, skipping an entry gone meanwhile, then `packed-refs`, parsed once
per call. It reads one entity's objects in process (go-git's storage,
packs kept open). A bulk `Load` runs one `git cat-file --batch --buffer`
per CPU, each walking its share of the entities level by level (heads,
then their trees and parents, then the packs). Writes: objects in
process, then `git update-ref` (ADR 0006); a fetch's merges go in one
`git update-ref --stdin` transaction.

### The read path: the view

The view is the read model: every list, filter, search, lookup and
`show` reads it. It is disposable (ADR 0009) and incremental: it changes
by the entities that moved, never by a rebuild, except when a kind's
`Rev` or the view's own version changes.

Its tables, in sqlite (`.git/board/v2/view.db`, WAL) or in PostgreSQL
(`git_board_view_v1`, when the datastore is `Shared` and has `SQL`):

| Table | Holds |
|---|---|
| `heads` | one row per kind: every (id, version, clock) of the kind, as one sorted blob |
| `rows` | (kind, id) → version, created, updated, hidden, the row (the kind's codec) |
| `terms` | (kind, term, id): the terms, indexed by term |
| `texts` | FTS5 (sqlite) or a `tsvector` (PostgreSQL), per (kind, id, field) |
| `states` | (kind, id) → version, the state as JSON; filled by a write or a `show`, not by a build |
| `broken` | (kind, id) → version, error |

**Freshness, every query:**

1. `Heads` lists the kinds the query needs. Git: about 5 ms on 20,000
   refs.
2. The listing, sorted, is compared with the `heads` blob, byte for
   byte. Equal: the view is fresh, and the query runs. This is the
   common case.
3. Else the difference is computed: entities new or moved, and entities
   missing from the listing.
4. The moved ones are loaded at the version listed (`Load`), and folded:
   incrementally when the new packs all sort after the stored head (an
   append, which is what a write makes), from the start otherwise (a
   merge).
5. A missing one is dropped only after a point read answers
   `ErrNotFound`. A listing that missed an entity (2.6) costs a read, never
   a lost issue.
6. One transaction writes them, row by row, only where the stored
   version is still the one step 2 read. A row another process wrote
   meanwhile is left to it; a version left behind is picked up by the
   next query.

**In one process,** a write updates the view itself (write-through,
below), so it reads its own writes without a listing. **Across
processes** (the server and a command, two worktrees), step 1 sees what
the others wrote. **After a fetch,** the merged refs are seen the same
way. No answer relies on a time: every row is keyed by the version it
was folded at.

**The server** keeps the rows of every kind in memory, keyed by version,
and refreshes them by the same steps from one poller (one second), which
also feeds the change events. A request still runs step 1 first, so a
command's write is never missed between two polls.

**`show`** reads the row and the state at the current version; with no
state stored, it loads the entity's packs (one entity: about 30 objects)
and stores its state.

### The write path

1. The surface calls `app`, which calls a domain.
2. The domain validates the request against its config and its rules
   (fields, transitions, thread anchors, "changed meanwhile"), inside
   `Do`'s `decide`, so the decision sees the head it writes on.
3. The kernel encodes one pack: `{"clock": head clock + 1, "ops": …}`,
   compact, keys in the spec's order. Its id is its git blob hash.
4. `Add(kind, id, pack, prev = version read)`. On `ErrMoved`: read again,
   decide again, up to 5 tries.
5. Write-through: the new pack sorts last (its clock is the highest), so
   the new state is the old state with the new operations applied. The
   view's row is replaced where its version is still `prev`.
6. Observers run: the Markdown files render, the forge-only sender
   queues the issue, the server pushes an event.

No process lock guards writes. Two writers are safe by the
compare-and-swap; a sync is a writer like the others.

### Sync: one engine, kinds and forges as plug-ins

`sync` runs the [Sync](../spec/sync.md) algorithm once, for any kind
that has a mapper and any forge that carries that kind. The records stay
operations in `sync` entities, the journal and the lock stay as the spec
says.

```go
package sync

// Mapper says how one kind meets a forge. Values are compared field by
// field against the last seen value: sync.md › The four cases.
type Mapper interface {
	Kind() string
	Fields() []string                        // title, body, status, labels, assignees…
	Local(state any) map[string]Value        // ours
	Remote(it forge.Item) map[string]Value   // theirs
	Pull(field string, v Value) []kernel.Op  // theirs, as our operations
	Push(field string, v Value) forge.Change // ours, as a forge write
}

func Run(ctx context.Context, b *kernel.Board, f forge.Forge, ms []Mapper, o Options) (Report, error)
```

```go
package forge

type Forge interface {
	Key() string    // github.com/guilbep/git-board
	Server() string // github.com
	Me(ctx context.Context) (Account, error)
	As(ctx context.Context, a Account) (Forge, error)
	// Items is the forge's side of a kind: issues, releases. nil: this
	// forge doesn't carry it.
	Items(kind string) Items
	// Pulls are its pull or merge requests, read by the cycle.
	Pulls() Pulls
}

type Items interface {
	List(ctx context.Context, since int64) ([]Item, error)
	Get(ctx context.Context, ref Ref) (Item, error)
	Create(ctx context.Context, it Item) (Item, error)
	Apply(ctx context.Context, ref Ref, c Change) error
}
```

Issues are the first mapper, people's accounts come with them, releases
are the second (publish only: ours to the forge). GitHub and GitLab both
carry both, so a release reaches GitLab too (ADR 0020, *Left open*).
v1's adapters (`core/forge/github`, `gitlab`, 3,355 lines) and their
conformance suite are ported, not rewritten: they were right.

### Config: each domain owns its table

```go
package conf

// Table is one table of .git-board.toml, declared by the domain that
// reads it.
type Table[T any] struct {
	Name    string           // "cycle": [cycle]
	Default func() T
	Check   func(T) []Problem // the domain's own rules
	Switch  string            // "" or its [experimental] key (ADR 0018)
}

// Declare registers a table on the file. Two domains can't declare one
// name.
func Declare[T any](f *File, t Table[T]) *Handle[T]

// Get is the table, checked. A broken table breaks its domain alone:
// the others answer, and app reports this one's problems.
func (h *Handle[T]) Get() (T, []Problem)

// Set rewrites this table's text in the file, and nothing else:
// comments, order and the other tables stay byte for byte.
func (h *Handle[T]) Set(ctx context.Context, v T) error

// Load reads the file once. A key no table declares is a problem of the
// file, reported by `config` and by Settings, and fails no command.
func Load(path string) (*File, error)
```

`[fields]`, `[labels]`, `[sprints]`, `[views]`, `[roles]` are the issue
domain's; `[release]` the release domain's; `[cycle]` the cycle's;
`[branches]`, `[uploads]`, `[github-project]` the tools'. Defaults live
with their table. One `Set` replaces the 751 lines of `setsection.go`.

### The application: one API

`app` opens a board and exposes each domain as a service. Every
surface calls it; none holds a rule.

```go
package app

type Board struct {
	Issues   *issues.Service
	People   *people.Service
	Releases *releases.Service
	Cycle    *cycle.Service // nil while the cycle is off (ADR 0018)
	Sync     *sync.Service
	Config   *conf.File
}

// Open finds the repository at dir, its datastore (board.datastore),
// its view, its config and who writes (board.me).
func Open(ctx context.Context, dir string, o Options) (*Board, error)
```

A service method takes a context and one request struct, and returns one
result or an `*app.Error`:

```go
func (s *issues.Service) List(ctx context.Context, r issues.ListReq) ([]issues.Issue, error)
func (s *issues.Service) Show(ctx context.Context, r issues.ShowReq) (issues.Thread, error)
func (s *issues.Service) Create(ctx context.Context, r issues.CreateReq) (issues.Issue, error)
func (s *issues.Service) Comment(ctx context.Context, r issues.CommentReq) (issues.Thread, error)
func (s *issues.Service) Set(ctx context.Context, r issues.SetReq) (issues.Issue, error)
```

`issues.Issue` and `issues.Thread` carry the JSON tags of today's
`core/model`: the CLI's `--json` and the API print the same structs.

The adapters are generic:

```go
// http: a route is a method; path values and the query fill the request
// by its tags (`path:"id"`, `query:"filter"`), the body by JSON.
func Route[Req, Res any](mux *http.ServeMux, pattern string,
	f func(context.Context, Req) (Res, error))

Route(mux, "POST /api/issues/{id}/comments", b.Issues.Comment)
```

The CLI's commands build the same request from flags, and print the
result through one presenter: `--json [fields]`, `--jq`, `--template`,
`--md`, tables. A route or a command that needs more than a call and an
encoding is a sign the rule is in the wrong place.

### Errors and exit codes

```go
package app

type Class int

const (
	Internal Class = iota
	Usage          // a wrong request: a flag, a value, a config key
	NotFound       // no such entity, or an ambiguous prefix
	NoMe           // no one to write as
	Changed        // the text changed meanwhile
	Busy           // a sync or a release runs; try again
	Missing        // the datastore can't (ADR 0002 §3)
	Forbidden      // the server's host and origin checks (ADR 0013)
	Off            // an experimental feature is off
)

type Error struct {
	Class Class
	Code  string // the API's stable code: "changed", "syncing", "missing"…
	Err   error  // what the user reads
}
```

| Class | Exit code | HTTP |
|---|---|---|
| `Internal` | 1 | 500 (v1: 400) |
| `Usage` | 2 | 400 |
| `NotFound` | 3 | 404 |
| `NoMe` | 4 | 409 `no-identity` (v1: so the inbox; its writes, 400) |
| `Changed` | 5 | 409 `changed` |
| `Busy`, `Missing` | 1 | 409, with the code |
| `Forbidden` | 1 | 403 |
| `Off` | 1 | 404 `experimental` |

One table, in `app`, read by both adapters. Errors go to stderr prefixed
`error:`, as today.

### Testing

- **`storetest`**, one suite every datastore runs (git, PostgreSQL,
  Markdown, forge copy, mem): add and read, one writer wins, the counter,
  and concurrency: `Heads` while `git update-ref` and `git pack-refs`
  run, `Load` while packs are appended, two processes writing one entity.
  v1's `dstest` is its seed.
- **`forgetest`**, one suite every forge adapter runs against its
  recorded fake (ADR 0014: never the network). v1's
  `core/forge/conformance` is ported.
- **`viewtest`**: sqlite and PostgreSQL answer every query alike; a
  refresh racing a write and another refresh never loses or regresses a
  row; a missed ref never drops an entity.
- **The spec's examples are fixtures.** Every JSON block of
  `docs/spec/*.md` is parsed by a test and folded; a spec edit that
  breaks the code fails the build, and the other way round.
- **Golden boards.** `testdata/golden/boards/` holds git bundles: v1's
  `boardgen` boards, and hand-made edge cases (a merge, two first commits
  of one sync entity, a tombstone, a broken entity, an unknown field, a
  `refs/board/issue/notes` ref). For each, the output v1 gives for `ls
  --json`, `show --json`, `log --json`, `status --json` and the API's
  routes is recorded. v2 must give the same, field by field.
- **Cross-writes.** `make compat` builds both binaries and runs scripts
  that alternate them on one board: v2 writes, v1 reads, and the other
  way round, on git, PostgreSQL and Markdown. On Markdown the rendered
  files too (`golden/mdfiles_test.go`): after one version's writes
  the folder is what the other renders from no file; the same hand edits
  are read in the same (operations, conflicts, files); a `datastore
  copy` into a folder renders the same files.
- **Budgets.** The benchmarks of section 4 run on a 20,000-issue board in
  CI (nightly), and fail above budget.
- **The web app's e2e tests** (`web/e2e`) run against `git-board-next
  serve` (`git board serve` since M7) from M5 on.

## 4. Speed budget

### What was measured

On the reference machine (2 CPUs, Xeon 2.1 GHz, Linux), a board made by
`go run ./cmd/boardgen -n 2000`:

| | 2,000 issues | ×10: 20,000 |
|---|---|---|
| packs (commits) | 13,266 | ≈ 133,000 |
| git objects (commit, tree, blob each) | 39,798 | ≈ 400,000 |
| pack JSON | 10 MB | ≈ 100 MB |
| `packed-refs` | 200 KB | ≈ 2 MB |
| `git cat-file --batch`, every object, by id, random order | 0.20 s: 5 µs an object | ≈ 2.0 CPU-s |
| v1, first `ls` (no index) | 1.26 s | 16 s (measured) |
| v1, `ls` | 0.16 s | 1.5 s (measured) |
| v1, `show` | 45 ms | 207 ms (measured) |

### Targets

On 20,000 issues, a git board, the reference machine (a laptop of 8 cores
does better on the first build only).

| Operation | v1 | v2 target |
|---|---|---|
| `ls`, view fresh | 1.5 s | **< 100 ms** |
| `ls` after a pull that moved 500 issues | | **< 300 ms** |
| `ls`, first on a clone (no view) | 16 s | **< 3 s** |
| `ls "free text"`, first on a clone | | < 5 s |
| `show <id>` | 207 ms | **< 30 ms** |
| `new`, `set`, `comment` | 221–283 ms | **< 50 ms** |
| server: `GET /api/issues`, all 20,000 | ≈ 170 ms | **< 60 ms** |
| server: `GET /api/issues/{id}/thread` | | < 10 ms |
| fetch that merges 1,000 issues | 1,000 processes | one `update-ref --stdin` |

### Why they hold

**`ls`, view fresh: about 85 ms.**

| Step | Cost |
|---|---|
| start the process, read `.git-board.toml` | 10 ms (`--version` today: 12 ms) |
| `Heads`: read 2 MB of `packed-refs`, a few loose refs | 5 ms |
| compare with the `heads` blob (20,000 × 40 bytes) | 2 ms |
| read the rows: 20,000 × 300 bytes from sqlite | 40 ms |
| decode them (a hand-written codec, 1 µs a row, 2 CPUs) | 10 ms |
| filter, sort | 8 ms |
| print | 10 ms |

A filter with terms (`status:ready assignee:dev3`) reads fewer rows:
the terms table narrows first. What v1 spends here is decoding 20,000
whole states as JSON (performance.md, *What's left*); v2's rows hold
what a list shows, and no text.

**After a pull that moved 500 issues: about 200 ms.** The 85 ms above,
plus 500 entities loaded: 500 × 30 objects × 5 µs = 75 ms, folded
incrementally where they were appends, and written in one transaction
(20 ms).

**First `ls` on a clone: about 2 s.**

| Step | CPU | Wall, 2 CPUs |
|---|---|---|
| read 400,000 objects, two `cat-file` processes | 2.0 s | 1.0 s |
| parse commits and trees, decode 100 MB of packs, fold | 1.2 s | 0.6 s |
| write 20,000 rows and 100,000 terms, one transaction | 0.3 s | 0.3 s |
| total | | ≈ 1.9 s |

The free-text index is not in it: its 60 MB or so of text are indexed
after the rows, in the server's background, or by the first query that
asks for text (the 5 s line). States are not stored by the build either:
`show` stores each one the first time. Both choices keep the 100 MB of
pack JSON from being written a second time into the view.

**`show`: about 15 ms.** Freshness 6 ms; the row and the state from the
view, or one entity's 30 objects in process (under 1 ms) and a fold;
the people from the view; the HTML of its texts.

**A write: about 30 ms.** Freshness 6 ms; three objects written in
process; `git update-ref`, 3 to 5 ms; the view's row replaced, one small
transaction; no fold of the whole entity (step 5 of the write path).

**The server's list: about 50 ms.** The rows are in memory; freshness 6
ms; the filter 6 ms; encoding 20,000 issues as JSON (8 MB) 40 ms. A paged
API would cut the last line; it changes the contract, so it is a
question (section 6).

### Limits, said now

- **Markdown boards hash every file on each `Heads`** (ADR 0002, *Left
  open*: 52 ms for 1,000 files). At 20,000 files that is a second a
  query: a Markdown board is for a few thousand issues.
- **PostgreSQL's `Heads`** is one query of 20,000 rows, 15 to 30 ms over a
  network. The budgets above add it.
- **The first build's 5 µs an object** is git's C, measured on 2,000
  issues. M1 measures it at 20,000, and the first build's design is
  changed or the target moved before anything else is built on it.

## 5. Migration plan

### v2 grows in `next/`, in the same module

As planned, and as built until M7: the switch moved `next/` to the root
(below).

- `next/` is a tree of this repository and of its Go module,
  `github.com/guilbep/git-board`. One `go.mod`, one `make ci`, one set of
  dependencies (ADR 0017).
- Its binary is `git-board-next`, so `git board-next ls` works next to
  `git board ls`. Releases go on shipping v1 until the switch.
- The guard, written first: nothing in `next/` imports `core/`,
  `internal/` or `cmd/`, and nothing outside imports `next/`. Code from
  v1 that was right (the forge adapters, the search tokenizer, the
  filter grammar, the cycle's computation, the release versions) is
  copied in and owned, not imported, so deleting v1 is one `git rm`.
- v1 takes fixes only. A new feature waits for the milestone that brings
  its domain to v2, and lands there (a question for Pierre: section 6).

Why not a module `v2/`: Go reads a `/v2` module path as major version 2,
and the tags would have to be `v2.x.y`, while git-board is at `v0.11`.
Why not a branch: compatibility must be proven on every commit of v1
too, so both live in one tree.

### Compatibility, proven continuously

- `make compat` (in `make ci` from M1): both binaries, the golden boards
  of section 3, every recorded output compared. Allowed differences are
  listed in one file, `testdata/compat/allowed.txt`, each with its
  reason (an internal error that was 400 and is 500).
- Nightly, the same on the 20,000-issue board, with the budgets.
- From M2, dogfooding: Pierre's boards (git-board's own, co2-calculator)
  are read by both binaries daily, by a scheduled job that diffs `ls
  --json` and `show --json` of every issue, and reports any difference.

**Dogfooding, as built (2026-11-05).** `make dogfood DIR=<repository>`
runs `golden`'s `TestDogfood`: each binary reads its own copy of the
board (a clone of the repository's commit, its working tree's
`.git-board.toml`, its `refs/board/*`, no remote; the repository is never
written, nothing is fetched or pushed), and `ls --json`, `ls is:closed
--json` and `show <id> --json` of every issue, deleted ones too, are
compared through make compat's `Run`, `Env` and `Normalise`
([compat.md](compat.md#dogfooding)): exit code and stdout. `OLD` is v1's
binary, else v1 built from `OLD_REF` (`git archive`, no worktree; the
newest `v0.11.*` tag by default); `NEW` v2's, else built now. A difference
fails it, and the differing outputs and their diff go to `DOGFOOD_OUT`
(`dogfood-out/dogfood.diff`). It reads boards kept in git: a clone whose
`board.datastore` names PostgreSQL or a folder is refused.
`.github/workflows/dogfood.yml` runs it daily (and by hand, with an
`old_ref`) on this repository's board, fetched from origin, and uploads
the diff when it fails; Pierre's other boards (co2-calculator's) are run
by hand, `make dogfood DIR=<its checkout>`, once their issues are in
`refs/board/`. Pierre asked to run it after v0.12 rather
than hold the release on it.

Its baseline is a release, and **v0.11.6 can't be one**: it has no `show`
command, and its `ls --json` has neither `closed_at` nor `comments`
(added on main since). Against it every case differs (git-board's own
board, 2026-11-05: 42 of 42). v1 built from this tree and v2 agree on that
board, every case. So the job needed a last v1 release cut from main
before the switch (v0.11.7). Pierre decided otherwise (M7, below): no
v0.11.7 is cut, and the baseline is v1 built from its last commit,
`7a546cc` (the merge base of the switch with main), named in the
Makefile's `V1_REF` and the job's.

### Milestones

Sizes are lines of Go, without tests; tests are about as many again.

| | What | Done when | Size |
|---|---|---|---|
| **M0** | Contracts frozen: the PostgreSQL, Markdown and counter formats written into the spec; the golden boards and v1's recorded outputs; the API transcripts; `make compat`; the import guard | v1 passes its own goldens | 1,500 (tools, tests) |
| **M1** | `kernel`, `kernel/pack`, `store/git`, `store/mem`, `storetest`, `view` (sqlite), the `issue` and `person` kinds; `ls`, `show`, `log` read-only | the goldens' `ls`, `show`, `log` match; the budgets of section 4 hold for reads | 5,000 |
| **M2** | Writes; `conf`; `domain/issues` (fields, filters, inbox, numbers); `app`; the CLI's issue commands (`new`, `comment`, `edit`, `set`, `move`, `close`, `delete`, `undo`, `status`, `follow`, `labels`, `config`, `me`) | cross-writes pass; write budgets hold; dogfooding starts | 7,000 |
| **M3** | `store/pg` (with the view in PostgreSQL), `store/md`, `store/forgecopy`; `datastore copy` | `storetest` and `viewtest` on all; goldens on each | 3,000 |
| **M4** | `forge` (ported), `sync` engine, issue and release mappers; `fetch`, `pull`, `push`, `sync` | `forgetest`; sync's tests from v1 ported and green; v1 and v2 sync one board alternately | 5,000 |
| **M5** | `surface/http` on `app`, every route; change events | every transcript matches; `web/e2e` green on v2 | 4,000 |
| **M6** | `domain/releases`, proof, `domain/cycle`, the TUI, the tools (the release runner, the GitHub Project bridge, insights, lint, export and import; agents left, [below](#agents-and-wtx-their-own-binary)) | every command of v1 exists in v2, or is dropped by a decision | 6,000 |
| **M7** | The switch: `next/` becomes the root, `cmd/git-board` is v2, v1 is deleted | done, 2026-11-05 (below) | 0 |

**M4, as built (2026-11-05).** `ls` and `show` read the sync records
(`sync`): `pending.txt` is empty. The adapters, the conformance
suite (`forge/forgetest`, green over the fake, GitHub and GitLab)
and v1's syncer are ported: the kind-free parts (records, texts' forms,
lock, journal) in `sync`, the issue mapper and the run in
`domain/issues/issuesync`, v1's 85 sync tests green there. v1 and
v2 sync one board alternately against one fake forge, and pull, merge
and push one board between two clones (`golden/v1gen`, deleted at M7). `forge
pull|push|sync`, `fetch`, `pull`, `push`, `sync` and numbering through
origin's counter run. Left: the `Mapper` interface (with releases, the
second kind; the run is the issue mapper today), the release mapper, the
whole sync's dry run (`sync --check`; `forge … --check` runs), the
switch marks on pull and push (v0.11's, ADR 0021), the quote over a thread
comment and the note after a published body (v1's `Quote` and
`Published`, which read the body's sections), and the GitHub Project
step (built in M6, below).

**M6, as built (2026-11-05, partial).** GitLab replies are posted as
replies (ADR 0021): the wrapper that hears a run's writes forwards
`forge.Replier`. `domain/releases` is the release kind, its records
read whole from the datastore, v1's versions, tags and overlays, and
`proof` (ADR 0001). The sync engine has its `Mapper` interface
(`sync`), as a run-level plug-in rather than the field-level shape
above: the issues' run is one mapper (`issuesync.Mapper`), the releases'
the second (`releasesync`, publish only, to GitHub and GitLab through
`forge.Releaser`, run by a forge push on a board that publishes
binaries). The release runner is `tools/ship` over `app.Book`; the
forge's token stays in `app`, which publishes (`ship.Publisher`). The
cycle is `domain/cycle` (+ `work`), its evidence read in `app`;
`tools/code` reads git, `tools/branches` names branches. The
commands `release`, `deploy`, `verify`, `branch`, `sprint` and `tui` are
v1's; a golden board, `releases`, holds the release reads, and basic the
sprint, branch and verify cases. Left: agents, insights, and
the plan's HTML in the cycle's evidence (the caller renders).

**v1's CLI, closed for v0.12 (2026-11-05).** `init` (its templates;
not yet the start from origin's GitHub labels and Project), `lint`
(its cycle `read:` lines too; not yet `--create-labels`), `export --md` and `import --md` (the
Markdown datastore's files, `mdfiles`), `reset --hard`, `push --prune`
and the reset guards on pull and push, and `datastore drop` are v1's,
each held to v1 by a cross test in `golden` (`TestCrossInit`,
`TestCrossLint`, `TestCrossExport`, `TestCrossReset`, `TestCrossDrop`).
git-bug's `switch` and `prune` are not ported: v0.11 migrates (ADR
0021).

**The GitHub Project bridge, as built (2026-11-05).** `tools/ghproject`
is v1's `internal/ghbridge`, copied and owned, the sync engine's third
mapper (`ghproject.Mapper`): a Project's fields (single select,
iteration, number, date, text) and the labels, decided field by field
against the base both last agreed on, kept in the issue's metadata
(`git-board:<owner>/<n>:<field>`, as v1), with v1's conflict rules and
the assignees merged person by person. A date and a text column are
v2's, by the column's type. v1 read the Project with the `gh` CLI; v2
reads and writes it through GitHub's GraphQL API
(`forge/github.ProjectV2`), with the GitHub forge's token (ADR 0010), and
the assignees through REST. `project pull|push|sync` (`--check`,
`--limit`), `resolve` and `configure` are v1's; `sync` runs it between
the forges and the push when `[github-project]` names a Project (`--git`
skips it); the TUI's i/o keys pull it. The API's routes 52 and 53 answer
its last run and where each field lives (`places`); the web app has a
Settings tab for it, its conflicts with their two sides, and Fields'
places mark a column the Project hasn't. v1's ghbridge tests are
ported; `projectfake` is a Project served by httptest; and
`golden/v1gen`'s `TestProjectCross` (deleted at M7) had v1's bridge and v2's take
turns on one board and one Project: they planned the same at every turn,
and neither had anything left after the other ran.

About 31,000 lines of Go where v1 has 55,000. One engineer with agents:
M0 a week; M1 and M2 three weeks; M3 to M5 four; M6 three; with
dogfooding, two to three months.

**v1 is deleted** when, all at once: every milestone is done; `make
compat` has been green for two weeks of v1 fixes; the dogfooding job has
reported no difference for two weeks on real boards; the web e2e tests
pass on v2; and a release of v2 as `git-board` (still `v0.x`) has run a
week on Pierre's machines. Then one pull request moves `next/` to the
root and deletes `core/`, `internal/` and the old `cmd/`.

**M7, as built (2026-11-05).** Pierre waived the waiting criteria above
(ADR 0021): v1 is dropped entirely, with no v0.11 support, and v2 ships
as v0.12 now; the dogfooding job runs after. One branch,
`rewrite/switch`, did the switch:

- **v1 deleted**: `core/`, `internal/`, v1's `cmd/git-board`,
  `cmd/boardgen` and `cmd/smoketui`, `web/` (v1's web app),
  `golden/v1gen`, v1's API transcripts, the guard between `next/` and
  v1, and `docs/embed.go`.
- **`next/` moved to the root**, each package keeping its path under it
  (`next/kernel` is `kernel`, `next/web` is `web`), and
  `next/cmd/git-board-next` is `cmd/git-board`: the binary is
  `git-board`, `--version` says `git-board version <v>`.
  `cmd/boardgen` is `tools/boardgen`'s, for `make perf`.
- **The layers**: `layers_test.go` at the root, the table at the new
  paths; it also checks the table has no cycle and that nothing imports
  `contrib/`.
- **Compatibility without v1**: the golden boards and v1's outputs stay,
  and `make compat` runs git-board against them. The cross tests' v1 side
  was recorded before the deletion (`testdata/golden/cross/`), and
  `make golden` now records from git-board, a change of the contract
  reviewed as one ([compat.md](compat.md#running-it)). `pending.txt`
  went: it was empty.
- **Dogfooding** compares git-board with v1 built from `V1_REF`
  (`7a546cc`), not with a v0.11 tag.
- `contrib/board-agents` builds `./cmd/git-board` for its test against
  the server (`GIT_BOARD_BIN` names another).
- **The docs** describe v0.12 alone: the README (what git-board is, its
  install, a quick start, related work), CONTRIBUTING (the layout, no v1
  freeze), the guide and the manual (no v1-only behaviour; git-bug's
  `switch` and `prune` are v0.11's, to migrate with first), the spec's
  paths, ADR 0020 (its layout superseded) and ADR 0021 (done).

### Agents and wtx: their own binary

v2 has no agents ([ADR 0021](../adr/0021-git-board-v2-same-data-new-code.md)).
v1's (`internal/wtx`, `internal/ai`, the agent and worktree routes) are
ported to `git-board-agents`, a Go module of its own in
`contrib/board-agents/` (module `github.com/guilbep/board-agents`). It
imports nothing of git-board's: it reads and writes the board through the
`git-board` CLI's `--json` outputs and, for what only the server computes
(the cycle, the Work page's rows), through v2's HTTP API, all of it in
one file (`internal/board/board.go`). Its routes are v2's since
2026-11-05 (`/api/board`, `/api/issues/{id}`, `/api/issues/{id}/cycle`,
`/api/cycle?who=team`, comments with `If-Match`); a test runs it against
`git-board serve`. v2 lacks three things it used from v1, and it
says so where they're missing: a comment's `cycle:step` (comment `meta`,
M6), line comments located on the branch's head, and worktrees' paths on
the Work page (it matches rows by branch). Its README says how it leaves this
repository with `git subtree split`. `make agents` builds and tests it;
`make ci` doesn't: it is no longer part of git-board. It stays when v1 is
deleted, until it moves out.

## 6. Risks and open questions

**Answered on 2026-11-05: see [ADR 0021](../adr/0021-git-board-v2-same-data-new-code.md).**

**For Pierre to decide:**

1. **Freeze v1?** Fixes only, from the start of M1, or until M2? Each
   feature added to v1 meanwhile is built twice.
2. **`next/` in the same module** (proposed), or a separate module or
   repository?
3. **Reads through `git cat-file`.** The first build streams objects
   from `git cat-file` processes, where ADR 0006 says go-git reads
   objects. Amend ADR 0006 for bulk reads, or keep go-git and take a
   slower first build (v1's, about 5 times slower per object)?
4. **The HTTP API: frozen or reshaped?** Freezing it lets the web app
   move unchanged and proves parity. Reshaping it (paged lists, one
   error shape, fewer routes) is cheaper during the rewrite than after.
   Proposed: frozen until M5, reshaped after the switch, with the web app.
5. **What v2 drops.** Candidates no core needs and few people use: the
   GitHub Project bridge (`internal/ghbridge`, 1,241 lines), `html`,
   `ghlabels`, `migrate`, agents (`internal/ai`, `wtx`). Each dropped is
   a milestone shorter.
6. **The version number.** v2 ships as `v0.12` (proposed: the data is
   the same), or as `v1.0` to mark the rewrite?
7. **Writes during a sync.** v2 allows them (the compare-and-swap keeps
   them safe), where v1 answers 409 `syncing`. The web app then never
   sees that code; is that the behaviour wanted?
8. **One ADR.** This design becomes ADR 0021 once accepted, and the
   decisions it changes (ADR 0006's reads, ADR 0009's tables) are
   amended there.

**Risks:**

- **Hidden contracts.** v1 behaviour that no document states and some
  board relies on: the Go JSON decoder takes a key in any case and keeps
  the last of two equal keys (data-model.md marks it a known ceiling). v2
  keeps that leniency; the goldens include such packs.
- **sqlite in pure Go.** `modernc.org/sqlite` is slower than C. The 40
  ms for 20,000 rows is an estimate; M1 measures it. If it fails, the
  rows move to one flat file per kind next to `view.db`, read whole, and
  sqlite keeps terms and text.
- **The first build at 20,000.** Measured at 2,000 and multiplied by 10.
  M1 measures it on a real 20,000 board before anything else.
- **Two binaries on one board** during months: two views (fine, both
  disposable), one sync lock (shared on purpose), and the counter, which
  both must hand out by the same rule. The cross-write tests cover
  numbering.
- **Diverging merges** (storage.md, *Known point*): two clones that
  merge each other at once keep writing merge commits. Not v2's to fix,
  but v2's batched fetch must not make it worse; a golden board holds
  such a pair.
- **The rewrite stalls half done**, and two code bases live on. The
  milestones are ordered so that each is usable alone (M1 is a faster
  read-only `ls`), and v1 is deleted on criteria, not on a date.
