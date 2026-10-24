# Core M1: the spec and the trial, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The core's spec, as a Zensical site in `spec/`, and a trial core
that proves it: issues kept as operation logs in git, merged across clones,
and pulled from a fake forge, with "last seen" deciding what changed.

**Architecture:** Four packages under `core/`, each depending only on the
ones before it:
- `gitstore` reads and writes git objects (go-git), moves refs only if they
  still point where they were read (`git update-ref`), and fetches and
  pushes (the `git` binary);
- `oplog` keeps an entity as a chain of commits under
  `refs/board/<kind>/<id>`, each holding one pack of operations and a clock,
  and orders all of them by (clock, pack hash);
- `issue` folds an issue's operations into its state;
- `syncer` pulls a forge's issues into the core (`forge` defines a forge,
  `forge/fake` holds one in memory), recording links and "last seen" in one
  `sync` entity per forge project.

The trial ends at a gate: a size and concepts report, then guilbep decides
whether to go on.

**Tech Stack:** Go 1.26.6 (go.mod), go-git v5 (already a dependency), the
`git` binary, Zensical 0.0.72 through `uvx`.

**Spec:** [core.md](core.md) (the PRD: decisions, scope, rules). This
milestone writes the spec itself, in `spec/`.

## Global Constraints

- **Clean room:** read only `docs/prd/core.md`, this plan and `spec/`.
  Never git-bug's source, the fork's code (`../git-bug`), or
  github.com/git-bug/spec. One exception, for Task 3 only: the fork's
  `doc/design/bridges/sync-decisions.md`, which guilbep wrote. Read it for
  the decisions, and write them in this model's terms.
- `core/...` never imports `github.com/git-bug/git-bug/...` nor
  `github.com/guilbep/git-board/internal/...`. Task 1's test enforces it.
- No new dependencies in M1. Nothing GPL, ever.
- `CGO_ENABLED=0 go build ./...` must keep working.
- **Refs:** entities live only under `refs/board/<kind>/<id>`, for kinds
  `issue`, `person` and `sync`. git-board's `refs/board/seq` stays as it is.
- **Format:** an entity commit's tree has one entry, `pack`: a JSON blob
  `{"clock": N, "nonce": "<hex>", "ops": [...]}`. `nonce` is present only
  in an entity's first pack.
- **Order:** an entity's operations are every reachable commit's pack,
  sorted by (clock ascending, pack blob hash ascending), with each pack's
  ops in their stored order. A commit's clock is 1 + the highest of its
  parents' (1 for the first).
- **Ids:**
  - an entity created without an id gets its first pack's blob hash, in hex;
  - an operation's id is `<pack blob hash hex>:<index in the pack>`.
- Entity commits: author and committer `git-board <git-board@localhost>`,
  message `git-board`, unsigned.
- Tests call `t.Parallel()` first, unless they chdir, set an environment
  variable or swap a package variable (CLAUDE.md). Package tests while
  working (`go test ./core/...`); `make ci` once at the end.
- gofmt with Go 1.26: `$(GOTOOLCHAIN=go1.26.0 go env GOROOT)/bin/gofmt -l .`
  prints nothing.
- Commits: conventional (`feat(core): …`, `docs(spec): …`), SSH-signed, no
  attribution lines. Branch `prd/core`; nothing is pushed.
- Comments and docs in git-board's voice: short, plain sentences saying what
  a thing is; `ponytail:` for deliberate ceilings.

## Review Focus

1. **Two processes append to the same issue at the same moment:** both
   operations land, one after a retry. *Test:* Task 5,
   `TestAppendRaceKeepsBoth`.
2. **Two clones edit the same issue offline, then fetch each other:** both
   end with the same operations in the same order, and the same state.
   *Test:* Task 6, `TestMergeConverges`.
3. **Wall clocks disagree between machines:** order comes from the clock,
   never from `time`. *Test:* Task 5, `TestOrderIgnoresWallTime`.
4. **A log written by a newer version holds an operation this one doesn't
   know:** loading it fails with an error naming the kind, rather than
   dropping it silently. *Test:* Task 6, `TestUnknownOpIsAnError`.
5. **A fresh repository** (no commit, no `refs/board`): opening it works,
   listing is empty, creating works. *Test:* Task 4, `TestOpenFreshRepo`.

---

### Task 1: The library's place, and its import guard

**Files:**
- Create: `core/doc.go`
- Test: `core/importguard_test.go`

**Interfaces:**
- Produces: package `core` (documentation only), and the guard every later
  task runs under.

- [ ] **Step 1: Write the failing test.** `TestCoreImportsNoGitBugNorInternal`
  runs `go list -deps ./...` from the `core` directory, through
  `exec.Command`. It fails listing every package whose path starts with
  `github.com/git-bug/git-bug` or `github.com/guilbep/git-board/internal`.
  A second case, `TestGuardSeesForbiddenImports`, feeds the checking
  function a list holding `github.com/git-bug/git-bug/cache` and asserts it's
  reported. Put the check in `forbidden(pkgs []string) []string`, in the
  test file.
- [ ] **Step 2: Run it** (`go test ./core/`). Expected: FAIL, as there is no
  Go file in `core` yet.
- [ ] **Step 3: Create `core/doc.go`:** `package core`, with a doc comment
  of three lines: what the core is (git-board's data model and sync, a
  library), where its spec is (`spec/`), and the two import rules.
- [ ] **Step 4: Run it.** Expected: PASS.
- [ ] **Step 5: Commit** `feat(core): the library's package and its import guard`.

### Task 2: The spec site, the data model and the storage

**Files:**
- Create: `spec/zensical.toml`, `spec/docs/index.md`,
  `spec/docs/data-model.md`, `spec/docs/storage.md`,
  `spec/docs/decisions.md`
- Modify: `Makefile` (a `spec` target), `.gitignore` (`spec/site/`)

**Interfaces:**
- Produces: the pages Tasks 4–7 implement, and `make spec`.

- [ ] **Step 1: `spec/zensical.toml`.** `[project]`, with
  `site_name = "git-board core"` and a `nav` list: Overview (`index.md`),
  Data model, Storage, Sync, Forges, Decisions. Sync and Forges come in
  Task 3. Theme: the template's defaults (`zensical new` shows them).
- [ ] **Step 2: `index.md`.** What the core is, for whom (git-board, and any
  Go program), what it isn't (compatible with git-bug), and how to read the
  site.
- [ ] **Step 3: `data-model.md`.** State as the spec's own rules, each with
  an example in JSON:
  - an entity, an operation (`kind`, `author`, `time`, `origin`, `data`),
    the ids;
  - the three kinds, `issue`, `person` and `sync`, with every operation of
    `issue` and its `data`:
    - `create {title, body}`, `comment {body}`, `edit-text {target, body}`
      (target: the create op's id for the body, or a comment's id);
    - `set-title {title}`, `set-status {status: "open"|"closed"}`;
    - `change-labels {add, remove}`, `change-assignees {add, remove}`;
  - how an issue's state follows from its operations: the first must be
    `create`; labels and assignees are sets, applied in order; an unknown
    kind is an error;
  - `person`: its operations are listed as planned for M2, not defined yet;
  - `sync`: `link {issue, forge_id, number, url}` and
    `seen {issue, field, value}`;
  - `origin {forge, event}` on operations pulled from a forge.
- [ ] **Step 4: `storage.md`.**
  - refs, the commit and tree layout, the pack blob, the clock, the order
    rule and why it's the same on every clone;
  - merging two heads (fast-forward when one contains the other, else a
    commit with both parents and an empty pack);
  - updating a ref only if unmoved, and retrying;
  - what fetch and push run.

  Copy the exact values from this plan's Global Constraints.
- [ ] **Step 5: `decisions.md`.** The PRD's decisions and rules, one line
  each with a link back to `docs/prd/core.md`, plus one empty section, "Trial
  results", which Task 8 fills.
- [ ] **Step 6: `make spec`.** It runs
  `uvx --from zensical==0.0.72 zensical build --strict -f spec/zensical.toml`.
  Ignore `spec/site/`. Run it. Expected: exit 0, no warnings.
- [ ] **Step 7: Commit** `docs(spec): the core's data model and storage`.

### Task 3: The spec of sync and of the forges

**Files:**
- Create: `spec/docs/sync.md`, `spec/docs/forges.md`

**Interfaces:**
- Consumes: Task 2's pages and nav.
- Produces: the rules M3 and M4 implement. The trial (Task 7) implements only
  "Pull, the trial's part".

- [ ] **Step 1: `sync.md`.** In this order:
  - **Links.** One per issue and forge project, in that project's `sync`
    entity.
  - **Last seen.** Per field: title, body, status, labels (sorted, one per
    line), each comment, assignees.
  - **The four cases.** Compare git-board and the forge with last seen:
    nothing to do; pull; push; both changed, and the later change wins
    while the other stays in the history.
  - **Echoes.** An event the forge reports for a change we sent is
    recognized by its origin, and not pulled again.
  - **Posting for people without a token,** under a header naming the
    author, the date and where it was written.
  - **Mentions and references.** Mentions are defused; `#n` becomes the
    mirrored issue's number.
  - **Refusals.** A value a forge refuses stays in git-board, with one
    warning.
  - **Deletions** are never propagated.
  - **Nothing private published.** Confidential issues are skipped. A
    public project gets nothing pulled from one that isn't, unless
    `publish-private` is set.
  - **Pull, the trial's part.** A section of its own: a forge issue not
    linked yet becomes an issue, its link and its last-seen values. For a
    linked one, a field whose forge value differs from last seen is pulled
    only if git-board's value still equals last seen. Last seen then takes
    the forge's value. Pulling twice in a row writes nothing.

  Write it from the decisions guilbep took for the fork (its
  `doc/design/bridges/sync-decisions.md`, which he wrote), restated in this
  model's terms, without copying code.
- [ ] **Step 2: `forges.md`.** What GitHub and GitLab need, as checked live
  for the fork:
  - **GitHub:**
    - status and labels come from the issue, not only from the timeline
      (git-bug#609, #1638);
    - 80 writes per minute and 500 per hour;
    - a login can be renamed, so people are matched by numeric id;
  - **GitLab:**
    - text edits leave no event;
    - `updated_at` moves late, so list from 2 minutes before the last sync;
    - the free edition has one assignee per issue;
    - base URLs compare without their trailing slash;
    - confidential issues are skipped.
- [ ] **Step 3: Add both to the nav and run `make spec`.** Expected: exit 0,
  no warnings.
- [ ] **Step 4: Commit** `docs(spec): sync and the forges`.

### Task 4: `core/gitstore`

**Files:**
- Create: `core/gitstore/store.go`, `core/gitstore/remote.go`
- Test: `core/gitstore/store_test.go`, `core/gitstore/remote_test.go`

**Interfaces:**
- Produces:
  - `type Hash = plumbing.Hash`, `var ZeroHash`
  - `var ErrRefMoved = errors.New("gitstore: ref moved since it was read")`
  - `func Open(path string) (*Store, error)`: a worktree (found upward from
    `path`) or a bare repository
  - `func (s *Store) WriteBlob(data []byte) (Hash, error)`,
    `ReadBlob(h Hash) ([]byte, error)`
  - `type TreeEntry struct{ Name string; Blob Hash }`,
    `WriteTree([]TreeEntry) (Hash, error)`,
    `ReadTree(Hash) ([]TreeEntry, error)`
  - `type Commit struct{ Tree Hash; Parents []Hash }`,
    `WriteCommit(Commit) (Hash, error)`, `ReadCommit(Hash) (Commit, error)`
  - `Ref(name string) (Hash, error)`, `ZeroHash` when absent;
    `Refs(prefix string) (map[string]Hash, error)`
  - `UpdateRef(name string, next, prev Hash) error`: through
    `git update-ref <name> <next> <prev>`, where a `prev` of `ZeroHash` means
    "must not exist"; `ErrRefMoved` when git refuses because the ref moved
  - `Fetch(ctx context.Context, remote string) error`:
    `git fetch <remote> +refs/board/*:refs/remotes/<remote>/board/*`
  - `Push(ctx context.Context, remote string) error`:
    `git push <remote> refs/board/*:refs/board/*`, not forced

- [ ] **Step 1: Write the failing tests** (a temporary repository from
  `git init`, in `t.TempDir()`):
  - `TestOpenFreshRepo`: `Open` works on a fresh `git init`;
    `Refs("refs/board/")` is empty; `Ref("refs/board/issue/x")` is
    `ZeroHash` with no error.
  - `TestBlobTreeCommitRoundTrip`: the written bytes, entries and parents
    read back equal.
  - `TestUpdateRefCreatesAndMoves`: create with `prev` `ZeroHash`, then move
    with the right `prev`.
  - `TestUpdateRefRefusesMoved`: a stale `prev` gives
    `errors.Is(err, ErrRefMoved)`, and the ref is unchanged.
  - `TestUpdateRefRefusesExisting`: `prev` `ZeroHash` on an existing ref
    gives `ErrRefMoved`.
  - `TestFetchPush` (remote.go): two clones of a bare origin. A ref pushed
    from one is, after `Fetch` in the other, at
    `refs/remotes/origin/board/issue/x`, with the same hash.
- [ ] **Step 2: Run them** (`go test ./core/gitstore/`). Expected: FAIL to
  compile.
- [ ] **Step 3: Implement** `store.go` with go-git
  (`git.PlainOpenWithOptions` with `DetectDotGit`; objects through
  `Storer.SetEncodedObject`), and `remote.go` with `exec.CommandContext`
  in the repository's directory. Map git's two refusals to `ErrRefMoved`:
  `cannot lock ref '…': is at <hash> but expected <hash>` and
  `cannot lock ref '…': reference already exists` (git 2.50, checked).
  Every other failure wraps git's stderr. A race where another process holds
  the ref's lock file (`Unable to create '….lock': File exists`) is also
  `ErrRefMoved`, so the caller retries.
- [ ] **Step 4: Run them.** Expected: PASS. Also run `go test ./core/`
  (the guard).
- [ ] **Step 5: Commit** `feat(core): gitstore, objects and refs in git`.

### Task 5: `core/oplog`

**Files:**
- Create: `core/oplog/oplog.go`, `core/oplog/merge.go`
- Test: `core/oplog/oplog_test.go`, `core/oplog/merge_test.go`

**Interfaces:**
- Consumes: Task 4's `Store`, `Hash`, `ErrRefMoved`.
- Produces:
  - `type Origin struct{ Forge string \`json:"forge"\`; Event string \`json:"event"\` }`
  - `type Op struct{ Kind string \`json:"kind"\`; Author string \`json:"author"\`; Time int64 \`json:"time"\`; Origin *Origin \`json:"origin,omitempty"\`; Data json.RawMessage \`json:"data,omitempty"\` }`
  - `type Entry struct{ Op; ID string; Clock uint64 }`
  - `type Log struct{ Kind, ID string; Head gitstore.Hash; Entries []Entry }`
  - `var ErrNotFound = errors.New("oplog: no such entity")`
  - `func RefName(kind, id string) string`
  - `func Create(s *gitstore.Store, kind, id string, ops []Op) (*Log, error)`:
    `id` empty means the first pack's blob hash
  - `func Load(s *gitstore.Store, kind, id string) (*Log, error)`
  - `func (l *Log) Append(s *gitstore.Store, ops []Op) error`: on
    `ErrRefMoved`, reload and append on the new head, at most 5 tries,
    leaving `l` reloaded
  - `func List(s *gitstore.Store, kind string) ([]string, error)`: sorted
  - `func Merge(s *gitstore.Store, kind, id string, other gitstore.Hash) (*Log, error)`
  - `func MergeFetched(s *gitstore.Store, remote string) (int, error)`:
    every `refs/remotes/<remote>/board/<kind>/<id>` merged into its local
    ref, which is created when absent; returns how many refs moved

- [ ] **Step 1: Write the failing tests:**
  - `TestCreateLoad`: the ops come back in order, the ids are
    `<packhash>:0`, `<packhash>:1`, and the clock is 1.
  - `TestCreateIDs`: two `Create` calls with the same ops get different ids
    (nonce); `Create` with an id given uses it.
  - `TestAppendOrder`: three appends give clocks 2, 3, 4, all ops in order.
  - `TestOrderIgnoresWallTime`: a later pack whose ops carry an earlier
    `Time` still comes after.
  - `TestAppendRaceKeepsBoth`: two `Store`s opened on one repository, two
    goroutines appending once each. All ops are present after `Load`, and
    no error.
  - `TestMergeFastForward`: `other` contains `Head`, so `Head` becomes
    `other` and no new commit is made.
  - `TestMergeDiverged`: a commit with both parents, an empty pack, and a
    clock of 1 + the highest. The ops of both sides are sorted by (clock,
    pack hash), checked by building the expected order by hand.
  - `TestLoadMissing`: `errors.Is(err, ErrNotFound)`.
- [ ] **Step 2: Run them** (`go test ./core/oplog/`). Expected: FAIL to
  compile.
- [ ] **Step 3: Implement.** `Load` walks every commit reachable from the
  ref, reads each `pack`, then sorts. Store packs as JSON with
  `encoding/json`, `ops` in order. `Append` writes one commit whose parent
  is `Head` and whose clock is the head commit's clock + 1.
- [ ] **Step 4: Run them**, with `-race`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(core): oplog, entities as operation logs in git`.

### Task 6: `core/issue`

**Files:**
- Create: `core/issue/issue.go` (types, `Apply`), `core/issue/ops.go`
  (constructors, `Create`, `Load`, `List`, `Do`)
- Test: `core/issue/issue_test.go`, `core/issue/merge_test.go`

**Interfaces:**
- Consumes: Task 5's `Op`, `Origin`, `Log`, `Create`, `Load`, `Append`,
  `MergeFetched`; Task 4's `Fetch`, `Push`.
- Produces:
  - `type Status string`; `const Open Status = "open"`,
    `Closed Status = "closed"`
  - `type Comment struct{ ID, Author, Body string; Time int64; Origin *oplog.Origin }`
  - `type Issue struct{ ID, Author, Title, Body, BodyOp string; Created int64; Status Status; Labels, Assignees []string; Comments []Comment; Origin *oplog.Origin }`.
    Labels and Assignees are sorted and unique; BodyOp is the create op's id.
  - `var ErrUnknownOp = errors.New("issue: unknown operation")`,
    `var ErrBadTarget = errors.New("issue: edit of an unknown text")`
  - `func Apply(l *oplog.Log) (*Issue, error)`
  - Constructors, each returning an `oplog.Op` with `Time` set to now:
    `NewCreate(author, title, body string)`, `NewComment(author, body)`,
    `EditText(author, target, body)`, `SetTitle(author, title)`,
    `SetStatus(author string, s Status)`,
    `ChangeLabels(author string, add, remove []string)`,
    `ChangeAssignees(author string, add, remove []string)`
  - `func Create(s *gitstore.Store, ops ...oplog.Op) (*Issue, error)`: the
    first op must be a create
  - `func Load(s *gitstore.Store, id string) (*Issue, error)`;
    `func List(s *gitstore.Store) ([]string, error)`
  - `func Do(s *gitstore.Store, id string, ops ...oplog.Op) (*Issue, error)`:
    appends, then returns the new state

- [ ] **Step 1: Write the failing tests:**
  - `TestCreateAndLoad`: title, body, open status, author, and `ID` equal to
    the log's id.
  - `TestEveryOp`: comment, edit the body, edit the comment, title, close,
    labels `+bug +ui` then `-ui`, assignees `+p1`. The state is checked field
    by field.
  - `TestFirstOpMustBeCreate`: `Create(s, NewComment(...))` errors.
  - `TestUnknownOpIsAnError`: a log holding `{"kind":"set-priority"}`.
    `Apply` gives `errors.Is(err, ErrUnknownOp)` and the message contains
    `set-priority`.
  - `TestEditUnknownTarget`: `ErrBadTarget`.
  - `TestMergeConverges` (merge_test.go):
    1. clone A creates an issue and pushes; B fetches and merges.
    2. Offline, A sets the title "from A" and adds label `a`; B comments and
       sets the title "from B".
    3. Each pushes, fetches the other, `MergeFetched`.
    4. A's and B's `Load` give identical `Issue` values. The title is the
       one whose pack sorts last.
- [ ] **Step 2: Run them** (`go test ./core/issue/`). Expected: FAIL to
  compile.
- [ ] **Step 3: Implement `Apply`** as one switch on `Kind`, unmarshalling
  `Data` per kind. Origins: the create op's origin goes on `Issue`, a
  comment op's on its `Comment`.
- [ ] **Step 4: Run them**, with `-race`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(core): issues, their operations and state`.

### Task 7: A forge, a fake one, and the trial's pull

**Files:**
- Create: `core/forge/forge.go`, `core/forge/fake/fake.go`,
  `core/syncer/state.go` (the `sync` entity), `core/syncer/pull.go`
- Test: `core/syncer/pull_test.go`

**Interfaces:**
- Consumes: Tasks 4–6.
- Produces:
  - `forge.Account{ID, Login string}`,
    `forge.Comment{ID, Body string; Author Account; Time int64}`
  - `forge.Issue{ID, Number, URL string; Author Account; Created int64; Title, Body, Status string; Labels []string; Comments []Comment}`
  - `type forge.Forge interface{ Key() string; Issues(ctx context.Context) ([]Issue, error) }`.
    This is the trial's subset; M3 widens it.
  - `fake.New(key string) *fake.Forge`, `(*Forge).Add(forge.Issue)`,
    `(*Forge).Edit(number string, change func(*forge.Issue))`, plus `Key`
    and `Issues`
  - `syncer.StateID(forgeKey string) string`: the hex sha1 of the key; the
    `sync` entity's id
  - `type syncer.Report struct{ Created, Updated, Unchanged int }`
  - `func syncer.Pull(ctx context.Context, s *gitstore.Store, f forge.Forge) (Report, error)`

  The authors of pulled ops are `forge:<login>` until people exist (M2). An
  op's origin is `{f.Key(), "issue:<id>"}` for the issue's own ops, and
  `{f.Key(), "comment:<id>"}` for a comment.

- [ ] **Step 1: Write the failing tests:**
  - `TestPullCreatesIssue`: the fake holds a closed issue #1 with labels
    `bug` and one comment. After a pull: one issue, its title, body, status,
    labels and comment; `Report{Created: 1}`; and the `sync` entity holds a
    link plus seen values for title, body, status, labels and the comment.
  - `TestPullTwiceWritesNothing`: a second pull leaves every
    `refs/board/` ref at the same hash. `Report{Unchanged: 1}`.
  - `TestPullForgeChange`: the forge's title changes. The issue's title
    follows, with an op whose origin is set, and seen follows too.
  - `TestPullKeepsLocalChange`: the title is changed in git-board, the forge
    is unchanged. After a pull the local title stays, and nothing is
    written.
  - `TestPullNewComment`: one comment is added on the forge. It is appended
    once, and a second pull adds nothing.
- [ ] **Step 2: Run them** (`go test ./core/syncer/`). Expected: FAIL to
  compile.
- [ ] **Step 3: Implement.**
  - `state.go`: load the `sync` entity (an empty state when `ErrNotFound`)
    into links by forge id, and seen by (issue, field). Append `link` and
    `seen` ops.
  - `pull.go`: the rule in `spec/docs/sync.md`, "Pull, the trial's part".
    Write the issue's ops before the sync entity's.

  ```go
  // ponytail: pulls current state only, without events: a field changed on
  // both sides keeps git-board's value. The later-change-wins rule needs the
  // forge's event times, M3.
  ```
- [ ] **Step 4: Run them**, with `-race`. Then `go test ./core/...`.
  Expected: PASS.
- [ ] **Step 5: Commit** `feat(core): a fake forge, and the trial's pull`.

### Task 8: The gate

**Files:**
- Modify: `spec/docs/decisions.md` (the "Trial results" section),
  `docs/prd/core.md` (a dated "M1 result" line in Milestones)

- [ ] **Step 1: Measure.** Count the lines in `core/` per package, code and
  tests apart (`wc -l`, `_test.go` aside). For comparison, the parts of
  git-bug these replace:
  - its operation logs, issues and cache, about 9,200 lines of code;
  - its git layer, about 7,100;
  - the fork's pull, `syncpull.go`, 910.

  Target: `gitstore`, `oplog` and `issue` under 1,500 lines of code
  together, and the trial's pull under 300.
- [ ] **Step 2: List the concepts** a reader must hold:
  - entity, operation, pack, clock, order;
  - merge, ref update, link, seen.

  Add any that crept in, with why.
- [ ] **Step 3: Write both into "Trial results",** with one paragraph: what
  was harder than planned, and what the spec got wrong (fix the spec in the
  same commit).
- [ ] **Step 4: Run `make spec`, `go test -race ./core/...` and `make ci`.**
  Expected: all pass.
- [ ] **Step 5: Commit** `docs(spec): the trial's results`. Then stop: guilbep
  reads the report and decides whether to go on to M2.
