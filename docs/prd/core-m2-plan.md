# Core M2: the full core, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The core becomes complete enough for git-board to stand on.
- People, as entities with an account per forge server.
- Who "I" am.
- A local sqlite index for lists and search.
- Change events for the server and the TUI.
- The trial's pull, authoring operations as people.

It also takes the hardening M1 carried over.

**Architecture:** Two new packages over the M1 core:
- `core/people`: the `person` entity, its state, and who "I" am;
- `core/index`: a rebuildable sqlite file, `<git common dir>/board/index.db`.

`core/watch` polls the refs and reports what changed. `gitstore` and `oplog` gain ctx-aware writes, validated names, and appends that don't reload. `syncer` authors pulled operations as people. The spec comes first (Task 1); each later task implements its pages.

**Tech Stack:** Go 1.26.6, go-git v5, the `git` binary, `modernc.org/sqlite` v1.60.1 (pure Go, BSD-3, already in the module graph; FTS5 checked), Zensical 0.0.72.

**Spec:** [core.md](core.md) (the PRD, including "Carried to later milestones") and `spec/docs/`. Task 1 writes M2's part of the spec; later tasks implement it.

## Global Constraints

- **Clean room:** read only `docs/prd/`, this plan, and `spec/`. Never git-bug's source, the fork's code (`../git-bug`), or github.com/git-bug/spec.
- **Imports:** `core/...` imports neither `github.com/git-bug/git-bug/...` nor `github.com/guilbep/git-board/internal/...`; the import guard enforces it, test files included.
- **One new dependency, allowed:** `modernc.org/sqlite` v1.60.1, only in `core/index`. Nothing else new, and nothing GPL, ever. `CGO_ENABLED=0 go build ./...` keeps working.
- **Kinds:** entities are `issue`, `person` and `sync`, listed once, as `gitstore.Kinds`. Names: an entity id matches `^[0-9A-Za-z._-]+$`.
- **The index** lives at `<git common dir>/board/index.db` (`git rev-parse --git-common-dir`), shared by every worktree. It is never the source of truth: deleting it loses nothing.
- **"Me"** is the local git config key `board.me`, holding a person id.
- **Writes take a context:**
  - `gitstore.UpdateRef`, `oplog.Create`/`Append`/`Merge`/`MergeFetched`, `issue.Create`/`Do`, `people.Create`/`Do` take `ctx` first;
  - reads (`Load`, `List`, `Ref`, `Refs`) don't.
- **Tests:**
  - each test calls `t.Parallel()` first, unless it chdirs, sets an environment variable or swaps a package variable (CLAUDE.md);
  - package tests while working, then `go test -race -count=1 ./core/...` and `make ci` once at the end;
  - gofmt with Go 1.26 (`$(GOTOOLCHAIN=go1.26.0 go env GOROOT)/bin/gofmt -l .`) prints nothing.
- **Commits:** conventional (`feat(core): …`, `docs(spec): …`), SSH-signed, no attribution lines. Branch `prd/core`; nothing is pushed.
- **Voice:** git-board's: short, plain sentences saying what a thing is; `ponytail:` for deliberate ceilings.

## Review Focus

1. **Two processes use the index at once** (a server and a command): neither gets "database is locked", and both see the other's writes after a refresh. *Test:* Task 6, `TestIndexTwoProcesses`.
2. **Two clones each create a person for the same forge account:** every clone resolves the account to the same person, the one whose id sorts first. *Test:* Task 4, `TestAccountOwnerIsFirstID`, and Task 6, `TestIndexAccountOwner`.
3. **The index file is missing, corrupt, or from another schema version:** it's rebuilt, and queries never answer from stale data. *Test:* Task 6, `TestIndexRebuilds`.
4. **A forge login is renamed:** the account keeps its person, whose login changes. *Test:* Task 8, `TestPullLoginRename`.
5. **Search text with FTS5's own syntax** (quotes, `*`, `-`, `:`, `AND`): no error; each word matches as a word prefix. *Test:* Task 6, `TestSearchTextIsPlainWords`.

---

### Task 1: M2's part of the spec

**Files:**
- Modify: `spec/docs/data-model.md`, `spec/docs/storage.md`, `spec/docs/sync.md`, `spec/zensical.toml`, `spec/docs/index.md`, `Makefile`
- Create: `spec/docs/queries.md` ("Queries and search")

**Interfaces:**
- Produces: the rules Tasks 2–8 implement.

- [ ] **Step 1: `data-model.md`, the person.** Define it fully, replacing the "planned for M2" note.
  - **Operations:** `create {name, email}`, `set-name {name}`, `set-email {email}`, `add-account {server, id, login}`, `remove-account {server, id}`.
  - **State:**
    - name, email;
    - accounts, sorted by (server, id). `server` is a forge host, like `github.com` or `gitlab.epfl.ch`; `id` is the forge's numeric or opaque user id.
  - **Rules:**
    - the first operation must be `create`, and a second is an error;
    - `add-account` for an account the person already has replaces its login, which is how a renamed login is followed;
    - `remove-account` of an absent account is ignored;
    - name, server, id and login are non-empty, else it's an error.
  - **Ownership:** when two persons hold the same (server, id), the account belongs to the person whose id sorts first in byte order. Lookups answer that one.
  - **Me:** who "I" am is the local git config key `board.me`, holding a person id, never shared.
  - **Authors:** every operation's `author` is a person id from M2 on. The trial's `forge:<login>` authors are gone.
  - **Each JSON example,** as in M1.
- [ ] **Step 2: `data-model.md`, the polish M1 carried.**
  - The issue state's JSON uses `created` and `updated` for the issue, and `time` and `edited` for a comment, and shows `origin`.
  - An operation without `kind`, `author` or `time` makes the reader reject its commit.
  - Ids match `^[0-9A-Za-z._-]+$`.
  - JSON keys are case-sensitive and unique (say this implementation doesn't check case: a `ponytail:` ceiling in oplog).
- [ ] **Step 3: `storage.md`, the polish.**
  - Retries are "at most 5 attempts in total".
  - A nonce is at least 16 random bytes.
  - Only SHA-1 repositories (go-git v5).
  - Fetch never propagates deletions.
  - The kinds are listed once.
  - An append doesn't reread the entity: its pack has the highest clock, so it sorts last.
- [ ] **Step 4: `sync.md`.**
  - Link to data-model.md's state rules instead of restating them.
  - Say why `link` and `seen` take the issue's author: only the pull's time matters, and the author is the pulled issue's person.
  - Say that pulled operations are authored by the person holding the forge account: found, or created with the login as name and the account added.
- [ ] **Step 5: `queries.md`, "Queries and search".**
  - **Where:** the index is a local sqlite file at `<git common dir>/board/index.db`, shared by worktrees, never the source of truth.
  - **Freshness:**
    - it stores each entity's head;
    - a refresh lists `refs/board/<kind>/*`, rereads the entities whose head moved, and drops the removed ones;
    - every query refreshes first.
  - **Rebuild:** a schema version (`PRAGMA user_version`) other than the code's, or a file that doesn't open, means drop it and rebuild from the refs.
  - **What it holds:** for issues, id, title, status, author, created, updated, labels and comment count; for persons, id, name and accounts.
  - **Query fields:**
    - `status` (any, open, closed);
    - `labels` (all of them);
    - `author` (a person id);
    - `text`;
    - sort by `created` or `updated` (default `updated`), descending or not;
    - `limit`.
  - **Text:**
    - full-text over title, body and comments;
    - each whitespace-separated word matches as a word prefix, and all words must match;
    - FTS5's own syntax is never interpreted: the words are quoted.
  - **Accounts:** a lookup by (server, id) answers the owner, by the rule in data-model.md.
  - **Change events:**
    - `Watch` polls `refs/board/<kind>/*` every given interval and reports each entity whose head changed or that was removed, whoever wrote it;
    - the first poll sets the baseline and reports nothing.
- [ ] **Step 6: nav, overview, Makefile.**
  - Add Queries and search to `zensical.toml`'s nav and link it from `index.md`.
  - `make clean` also removes `spec/site/`.
  - Run `make spec`. Expected: exit 0, no warnings.
- [ ] **Step 7: Commit** `docs(spec): people, queries, change events, and M1's polish`.

### Task 2: `gitstore` hardening

**Files:**
- Modify: `core/gitstore/store.go`, `core/gitstore/remote.go`, and every caller of `UpdateRef` (`core/oplog/*.go`)
- Test: `core/gitstore/store_test.go`, `core/gitstore/remote_test.go`

**Interfaces:**
- Produces:
  - `var Kinds = []string{"issue", "person", "sync"}`, used by `remote.go`'s refspecs;
  - `func (s *Store) UpdateRef(ctx context.Context, name string, next, prev Hash) error`;
  - `func (s *Store) CommonDir() string`: the absolute git common dir, found once at `Open` with `git rev-parse --git-common-dir`;
  - `func (s *Store) Config(ctx context.Context, key string) (string, error)`: `""` when unset, through `git config --local --get`;
  - `func (s *Store) SetConfig(ctx context.Context, key, value string) error`;
  - an unexported `command(ctx, args...) *exec.Cmd` that `git` runs through, so a test can read its `Env`.

- [ ] **Step 1: Write the failing tests:**
  - `TestUpdateRefLockFileIsMoved`: an existing `<ref>.lock` file gives `ErrRefMoved`.
  - `TestUpdateRefDeletedRefIsMoved`: a non-zero `prev` on a ref that no longer exists gives `ErrRefMoved`.
  - `TestUpdateRefOtherFileExistsIsNot`: a "File exists" failure that isn't a `.lock` (a directory where the ref file should go) is not `ErrRefMoved`.
  - `TestGitRunsInCLocale`: `command`'s `Env` holds `LC_ALL=C`.
  - `TestOpenNestedBare`: a bare repository inside another repository's worktree opens as itself, not as the enclosing one.
  - `TestCommonDirInWorktree`: from a linked worktree, `CommonDir` is the main repository's git dir.
  - `TestConfig`: set, get, and unset returning `""`.
- [ ] **Step 2: Run them.** Expected: FAIL.
- [ ] **Step 3: Implement.**
  - Match `.lock': File exists` only.
  - A non-zero `prev` refused because the ref doesn't resolve is `ErrRefMoved`.
  - `Open` tries the path as a repository root first, with `EnableDotGitCommonDir`, and detects upward only when that fails.
  - Update the callers in `oplog` to pass their ctx (Task 3 gives oplog's own functions a ctx; until then pass `context.Background()` there, and Task 3 replaces it).
- [ ] **Step 4: Run** `go test -race -count=1 ./core/...`. Expected: PASS.
- [ ] **Step 5: Commit** `fix(core): gitstore's refusals, nested bare repos, common dir, config`.

### Task 3: `oplog`: names, contexts, appends that don't reload

**Files:**
- Modify: `core/oplog/oplog.go`, `core/oplog/merge.go`, and the callers in `core/issue`, `core/syncer`
- Test: `core/oplog/oplog_test.go`, `core/oplog/merge_test.go`

**Interfaces:**
- Consumes: `gitstore.Kinds`, `UpdateRef(ctx, …)`.
- Produces:
  - `var ErrBadName = errors.New("oplog: bad kind or id")`
  - `func Create(ctx context.Context, s *gitstore.Store, kind, id string, ops []Op) (*Log, error)`
  - `func (l *Log) Append(ctx context.Context, s *gitstore.Store, ops []Op) error`
  - `func Merge(ctx context.Context, s *gitstore.Store, kind, id string, other gitstore.Hash) (*Log, error)`
  - `func MergeFetched(ctx context.Context, s *gitstore.Store, remote string) (int, error)`
  - `Load(s, kind, id)` and `List(s, kind)` reject a bad kind or id with `ErrBadName` too.
  - An unexported package variable `updateRef` (defaulting to the Store's method), which tests swap.

- [ ] **Step 1: Write the failing tests:**
  - `TestBadKindOrID`: kind `seq`, ids `""`, `a/b` and `a b` all give `ErrBadName`, for each function.
  - `TestAppendEntriesMatchLoad`: after `Append`, `l.Entries` and `l.Head` equal a fresh `Load`'s. A swapped `updateRef` that counts reads shows `Append` doesn't reload.
  - `TestAppendGivesUpAfterFive`: `updateRef` always returns `ErrRefMoved`. `Append` returns an error wrapping it after exactly 5 attempts.
  - `TestMergeRetries`: `updateRef` fails once with `ErrRefMoved`, then the merge lands.
  - `TestMergeFetchedGoesOnPastFailure`: one corrupt fetched entity and one good one give `n == 1` and a non-nil error, and the good one is merged.
  - `TestOpMissingFieldIsRejected`: packs with an op lacking `kind`, `author` or `time` make `Load` reject the commit.
- [ ] **Step 2: Run them.** Expected: FAIL.
- [ ] **Step 3: Implement.**
  - **`Append` builds its new entries locally:** ids `<packhash>:<i>`, clock head+1, all sorting last. `Create` does the same.
  - **`Merge`:** if the ref moved and the reload then fails, return the error, and let `MergeFetched` still count the move.
  - **The decode site** gets: `// ponytail: encoding/json matches keys in any case; the spec says case-sensitive. A strict decoder if another implementation ever disagrees.`
  - **Callers:** pass ctx through `issue.Create`/`Do` (now `Create(ctx, s, ops...)` and `Do(ctx, s, id, ops...)`) and `syncer`.
- [ ] **Step 4: Run** `go test -race -count=1 ./core/...`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(core): oplog checks names, takes contexts, and appends without rereading`.

### Task 4: `core/people`

**Files:**
- Create: `core/people/people.go` (types, `Apply`), `core/people/ops.go` (constructors, `Create`, `Load`, `List`, `Do`, `Me`, `SetMe`), `core/people/directory.go`
- Test: `core/people/people_test.go`, `core/people/directory_test.go`

**Interfaces:**
- Consumes: oplog (Task 3), `gitstore.Config`/`SetConfig` (Task 2).
- Produces:
  - `type Account struct{ Server, ID, Login string }`
  - `type Person struct{ ID, Name, Email string; Accounts []Account }`, with accounts sorted by (Server, ID)
  - `var ErrUnknownOp`, `ErrBadValue`
  - Constructors returning `oplog.Op` with `Time` set to now: `NewCreate(author, name, email string)`, `SetName(author, name)`, `SetEmail(author, email)`, `AddAccount(author string, a Account)`, `RemoveAccount(author, server, id string)`
  - `func Apply(l *oplog.Log) (*Person, error)`
  - `func Create(ctx context.Context, s *gitstore.Store, ops ...oplog.Op) (*Person, error)`, which validates before writing, like issues
  - `func Load(s *gitstore.Store, id string) (*Person, error)`, `func List(s *gitstore.Store) ([]string, error)`
  - `func Do(ctx context.Context, s *gitstore.Store, id string, ops ...oplog.Op) (*Person, error)`
  - `func Me(ctx context.Context, s *gitstore.Store) (string, error)`: `""` when unset. `func SetMe(ctx context.Context, s *gitstore.Store, id string) error`
  - `type Directory struct{…}`, `func LoadDirectory(s *gitstore.Store) (*Directory, error)`, `func (d *Directory) ForAccount(server, id string) (*Person, bool)`, applying the owner rule (the lowest id holding the account)

  A person's own `create` op has as author the person who made it, which may be itself: use the creating person's id, or `""` only for the very first person of a repository.

- [ ] **Step 1: Write the failing tests:**
  - `TestPersonEveryOp`: the state after each op, field by field.
  - `TestAddAccountRenamesLogin`: same (server, id), new login, one account.
  - `TestRemoveAbsentAccountIgnored`.
  - `TestPersonBadValues`: an empty name, server, id or login is `ErrBadValue`, and nothing is written.
  - `TestPersonUnknownOp`: `ErrUnknownOp`, naming the kind.
  - `TestMe`: unset gives `""`; set, then get.
  - `TestAccountOwnerIsFirstID`: two persons with the same account. `ForAccount` answers the one with the lower id, whichever was created first.
- [ ] **Step 2: Run them.** Expected: FAIL.
- [ ] **Step 3: Implement**, following `core/issue`'s shape.
- [ ] **Step 4: Run** `go test -race -count=1 ./core/...`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(core): people, their accounts, and who I am`.

### Task 5: Issues: updated times, and M1's test gaps

**Files:**
- Modify: `core/issue/issue.go`
- Test: `core/issue/issue_test.go`

**Interfaces:**
- Produces: `Issue.Updated int64` (the latest op time), `Comment.Edited int64` (the latest edit's time, 0 if never edited).

- [ ] **Step 1: Write the failing tests:**
  - `TestUpdatedAndEdited`: `Updated` follows the last op of any kind. `Edited` follows the comment's last edit and stays 0 for an unedited comment.
  - `TestAddRemoveSameLabelInOneOp`: add runs before remove, so the label is gone.
  - `TestBodyEditAfterCommentEdit`: both texts end as edited.
- [ ] **Step 2: Run them.** Expected: FAIL (the first two at least).
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `go test -race -count=1 ./core/issue/`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(core): issues know when they and their comments changed`.

### Task 6: `core/index`

**Files:**
- Create: `core/index/index.go` (Open, schema, Refresh), `core/index/query.go` (Issues, ForAccount, the text rule)
- Modify: `go.mod` (`modernc.org/sqlite` becomes a direct dependency), `go.sum`
- Test: `core/index/index_test.go`, `core/index/query_test.go`

**Interfaces:**
- Consumes: `issue.Load`/`List`, `people.Load`/`List` and the owner rule, `gitstore.CommonDir`, `Refs`.
- Produces:
  - `func Open(ctx context.Context, s *gitstore.Store) (*Index, error)`
  - `func (ix *Index) Close() error`
  - `func (ix *Index) Refresh(ctx context.Context) error`
  - `type Query struct{ Status issue.Status; Labels []string; Author, Text string; Sort string; Desc bool; Limit int }`. `Sort` is `"created"` or `"updated"`; empty means `"updated"`.
  - `type Summary struct{ ID, Title string; Status issue.Status; Labels []string; Author string; Created, Updated int64; Comments int }`
  - `func (ix *Index) Issues(ctx context.Context, q Query) ([]Summary, error)`, which refreshes first
  - `func (ix *Index) ForAccount(ctx context.Context, server, id string) (string, bool, error)`, the owner's person id

  The sqlite file is `<CommonDir>/board/index.db`, opened with `_pragma=busy_timeout(5000)` and `journal_mode(WAL)`. The schema version is `PRAGMA user_version = 1`.

- [ ] **Step 1: Write the failing tests:**
  - `TestIndexQueries`: four issues varying status, labels, author and text. Each `Query` field alone, then combined. Both sorts, both directions, and `Limit`.
  - `TestIndexSeesOtherWrites`: an issue written through another `Store` on the same repository appears at the next query; a changed one updates.
  - `TestIndexRebuilds`: three cases.
    1. Delete the file.
    2. Write garbage into it.
    3. Set `user_version` to 99.

    Each time the next `Open` and query answer correctly.
  - `TestIndexTwoProcesses`: two `Index` values on one file. Concurrent refreshes and queries in goroutines, with writes in between, give no error under `-race`.
  - `TestSearchTextIsPlainWords`: texts `crash`, `"crash`, `crash*`, `-crash`, `title:crash` and `crash AND` give no error. The first matches "Crashes on save"; words match as word prefixes.
  - `TestIndexAccountOwner`: two persons hold one account, and `ForAccount` answers the lower id.
- [ ] **Step 2: Run them.** Expected: FAIL.
- [ ] **Step 3: Implement.**
  - Tables for issues, labels, persons and accounts, plus an FTS5 table over title, body and comments. Each entity's head is stored.
  - Refresh compares the heads with `Refs`.
  - The text rule: split on whitespace, wrap each word in double quotes with inner quotes doubled, append `*`, and join with spaces (FTS5's implicit AND).
- [ ] **Step 4: Run** `go test -race -count=1 ./core/...`, and `CGO_ENABLED=0 go build ./...`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(core): a local index for lists and search`.

### Task 7: `core/watch`

**Files:**
- Create: `core/watch/watch.go`
- Test: `core/watch/watch_test.go`

**Interfaces:**
- Consumes: `gitstore.Kinds`, `Refs`.
- Produces:
  - `type Change struct{ Kind, ID string; Head gitstore.Hash }`. `Head` is `ZeroHash` when removed.
  - `func Watch(ctx context.Context, s *gitstore.Store, every time.Duration) <-chan []Change`. The channel closes when ctx ends; the first poll is the baseline.

- [ ] **Step 1: Write the failing tests:**
  - `TestWatchSeesOtherStore`: a write through another Store arrives within 5 intervals (10 ms).
  - `TestWatchQuiet`: no write, no event over 5 intervals.
  - `TestWatchRemoval`: a deleted ref is reported with `ZeroHash`.
  - `TestWatchStops`: cancelling ctx closes the channel.
- [ ] **Step 2: Run them.** Expected: FAIL.
- [ ] **Step 3: Implement.** One ticker, a map of heads, and a diff. `// ponytail: polls every interval; inotify-style notification if polling ever costs too much`.
- [ ] **Step 4: Run** `go test -race -count=1 ./core/watch/`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(core): change events by polling the refs`.

### Task 8: The pull authors as people

**Files:**
- Modify: `core/forge/forge.go`, `core/forge/fake/fake.go`, `core/syncer/pull.go`, `core/syncer/state.go`
- Test: `core/syncer/pull_test.go`, `core/forge/fake/fake_test.go`

**Interfaces:**
- Consumes: `people` (Task 4).
- Produces:
  - `Forge.Server() string`: the forge's host, like `github.com`. The fake's is `key` up to the first `/`.
  - `fake.(*Forge).Edit(number string, change func(*forge.Issue)) bool`: changes only the first issue with that number, and returns false when there's none.

- [ ] **Step 1: Write the failing tests:**
  - `TestPullCreatesPeople`: a pulled issue's ops are authored by a person id. That person has the account (server, id, login) and the login as name.
  - `TestPullReusesPerson`: two issues by one account give one person.
  - `TestPullLoginRename`: the account's login changes on the forge. Same person, new login, no new person.
  - Tighten the M1 tests, as the PRD's carried list says:
    - the link's number and URL;
    - a comment's time;
    - author, time and origin of set-status, change-labels, link and seen;
    - the issue's `Origin.Forge`;
    - the order of the seen ops;
    - a second pull in `TestPullForgeChange`;
    - the conflict pull's report in `TestPullKeepsLocalChange`.
  - `fake_test.go`:
    - `TestFakeReturnsCopies`: changing a returned issue's labels or comments doesn't change the fake;
    - `TestFakeEditFirstOnly`;
    - `TestFakeEditUnknown` returns false.
- [ ] **Step 2: Run them.** Expected: FAIL.
- [ ] **Step 3: Implement.**
  - Load the directory once per pull. For an account with no person, create the person, then use it. For a known account with a new login, write `add-account` on its person.
  - `// ponytail: the directory loads every person once per pull; the index's ForAccount when people get many`.
- [ ] **Step 4: Run** `go test -race -count=1 ./core/...`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(core): the pull authors operations as people`.

### Task 9: M2's gate

**Files:**
- Modify: `spec/docs/decisions.md` (an "M2 results" section, after "Trial results" and above the link definitions), `docs/prd/core.md` (an "M2 result" line in Milestones; remove the carried M2 items that are now done)

- [ ] **Step 1: Measure.** Lines per package, code and tests apart, against M1's table.
- [ ] **Step 2: Write "M2 results"** in four parts:
  - the table;
  - the concepts M2 added (person, account owner, me, index, refresh, change event);
  - one paragraph on what was harder than planned;
  - what the core still lacks for M5, compared with git-board's `internal/gitbugio`. List `gitbugio`'s exported names, and say which the core covers.
- [ ] **Step 3: Update the PRD.** The "M2 result" line, and strike the done M2 items from "Carried to later milestones".
- [ ] **Step 4: Run** `make spec`, `go test -race -count=1 ./core/...`, `make ci`, and gofmt 1.26. Expected: all pass.
- [ ] **Step 5: Commit** `docs(spec): M2's results`.
