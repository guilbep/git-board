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
known point, not settled yet: see [Merging two
heads](storage.md#merging-two-heads)). The spec got wrong: the refspecs (`refs/board/*`), what
a reader rejects, the sync state's edge cases (first link wins), and the
merge-commit divergence. All were fixed in the spec as the tasks found them.
The final review found more, fixed the same way: go-git ignored a linked
worktree's common directory, so a worktree saw no issue; fetched heads sat
in `refs/remotes/`, where branch lists and `git fetch --prune` reach them;
a forge comment pulled on two clones became two comments; and labels had no
check.

## M2 results

Measured on 2026-10-24, at the end of M2, lines with `wc -l`, tests apart
from code. M1's are the trial's, above.

| Package | Code | Tests | M1 code | M1 tests |
|---|---|---|---|---|
| `gitstore` | 413 | 512 | 301 | 305 |
| `oplog` | 454 | 770 | 354 | 499 |
| `issue` | 246 | 444 | 240 | 370 |
| `people` | 248 | 267 | | |
| `index` | 468 | 399 | | |
| `watch` | 108 | 164 | | |
| `forge`, `forge/fake` | 105 | 59 | 92 | |
| `syncer` (`pull.go` 272, `state.go` 148) | 420 | 599 | 346 | 377 |
| `core` (doc, import guard) | 4 | 59 | 4 | 59 |
| **Total** | **2,466** | **3,273** | **1,337** | **1,610** |

`gitstore`, `oplog` and `issue` together are 1,113 lines of code, still
under M1's target of 1,500. People, the index and change events add 824.

Concepts M2 added:

- the **person**, who authors every operation, with its **accounts**: a
  user on a forge server, named by (server, id), whose login can change;
- the **first pack**, the pack whose hash is a person's id: only its
  operations may have an empty author, since none can name the person;
- the **account owner**: of two persons holding one account, the one whose
  id sorts first;
- **me**, the local `board.me`, never shared;
- the **index**, a sqlite file every worktree shares, never the source of
  truth;
- the **refresh**, which brings the index up to the refs before every
  query, and the **broken entity** it keeps out and lists;
- the **change event**: a poll's batch of the entities that appeared,
  moved or went, after a baseline that reports nothing.

What was harder than planned: again git and its neighbours, not the model.
go-git lists a `<ref>.lock` as a ref, so an empty one from another process's
`git update-ref` failed every listing, and so every query; gitstore now
reads loose and packed refs itself. A cancelled git was killed and left its
lock behind, so it now gets SIGINT first. Two processes on one sqlite file
needed WAL, a busy timeout and transactions that take the write lock at
once, and a path with `#` or `?` opened another file until it was escaped.
A person's id comes from its first pack, so nothing in that pack can name
it: the author rule had to allow empty authors there, and a pulled person
became one write instead of two. One broken entity failed every query until
the index kept it out and listed it. A person that won't load first got
skipped by the pull, which made duplicates, so the pull now stops on it.
The old id pattern let in names git refuses, like `a..b` and `x.lock`, so
ids are now 40 lowercase hex digits.

What the core still lacks for M5, against git-board's `internal/gitbugio`.
Its issue methods take an id prefix; the core takes whole ids, so M5 needs
a prefix lookup, which the index can answer.

| `gitbugio` | The core |
|---|---|
| `Open`, `OpenCwd`, `NewForRepo`, `Close`, `ErrNoRepo` | `gitstore.Open`, `index.Open`, `Close`; `NewForRepo` goes with git-bug |
| `NewIssue`, `GetIssue`, `ListIssues`, `SetTitle`, `SetStatus`, `CloseBug`, `ApplyLabelChange`, `GetLabels`, `AddComment`, `Issue` | `issue.Create`, `Load`, `Do` and its operations; `index.Issues` |
| `EditComment`, `ErrNoSuchComment` | `issue.EditText`. Not its `base` check, `ErrChangedMeanwhile` |
| `SearchText` | `index.Issues` with a text |
| `Identities`, `NewIdentity`, `Me`, `Adopt`, `Person` | `people.List`, `Load`, `Create`, `Me`, `SetMe` |
| `Logins`, `MyLogin` | partly: a person's accounts. Which login a person is assigned as waits for M3's assignees |
| `RefsFingerprint` | `watch.Watch`, for issues and persons. Not releases |
| `Fetch`, `Push`, `Pull` | `gitstore.Fetch`, `Push`, `oplog.MergeFetched` |
| `Thread`, `Threads`, `Event`, `Version`, `History` | partly: the issue's state, and its log through `oplog.Load`. No events, edit history or participants as such |
| `Labels`, `Mentioning`, `Mentions` | none: every label in use, and issues mentioning a login |
| `AddCommentMeta`, `CommentWith`, `SetCommentMetadata` | none: operations hold an origin, no other metadata |
| `CommentReview`, `Review`, `StampPR` | none: review comments |
| `ProjectBases`, `SetProjectBases` | none: a GitHub Project's last agreed values |
| `Counter`, `ParseCounter`, `CheckNumbers`, `InitNumbers`, `NumberIssues`, `Numbering`, `FetchNumbers`, `NumbersRemote`, `ErrNumbered`, `ErrNumbersMoved` | none: issue numbers, `refs/board/seq`, stay git-board's |
| `CreateRelease`, `Releases`, `ReleaseFor`, `CommentRelease`, `SetReleaseNotes`, `YankRelease`, `PullReleases`, `PushReleases`, `ReleasesFingerprint`, `ReleasesNamespace`, `ReleaseRecord`, `ReleaseComment`, `Artifact`, `Asset`, `ChartArtifact` | none: release records |
| `AddBridge`, `Bridges`, `RemoveBridge`, `SetBridgeSync`, `SyncBridges`, `Bridge`, `SyncBridge`, `ErrNoBridge`, `Credentials`, `Credential`, `ForgeHosts`, `ForgeToken`, `GitHubToken`, `NoCredentials`, `Forge`, `SetAssignees` | M3 and M4: forge settings and tokens |
| `GetForges`, `SendForges`, `SyncForges`, `SyncIssue` | M3: only the trial's pull, `syncer.Pull`, exists |
| `LinkIssue`, `UnlinkIssue`, `TakeIssue` | none: out of version 1 |
| `Branches`, `Branch`, `Ancestors`, `ChangedFiles`, `ChangesIn`, `DiffFile`, `DiffStat`, `FileStat`, `FileLines`, `FirstCommitAdding`, `GrepFiles`, `HasCommit`, `LineAt`, `Log`, `Commit`, `MergeBase`, `ReadCommit`, `ReadFile`, `RevParse`, `RemoteHead`, `Remotes`, `Remote`, `Worktrees`, `Worktree`, `Ahead`, `GitHubRepo`, `GitHubRepoOf` | none: reading the code's own git history isn't the core's job, and stays in git-board |
| `SetFlatKinds`, `Storage`, `TestCache` | none: git-bug's cache and storage go with it |

[prd]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md
[decision]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md#the-decision
[rules]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md#rules
[model]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md#the-data-model-in-brief
[v1]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md#version-1
[answered]: https://github.com/guilbep/git-board/blob/main/docs/prd/core.md#answered-by-guilbep-2026-10-24
