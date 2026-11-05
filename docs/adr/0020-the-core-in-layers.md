# ADR 0020: The core in layers: a kernel, then issues, releases and the cycle

- Status: accepted (2026-11-04, Pierre: "we should have a kernel"). All
  four steps are done (2026-11-04): step 1, the guard; step 2, the issue
  domain's satellites; step 3, the release domain; step 4, the cycle (see
  Step 2, Step 3 and Step 4 below).
- Date: 2026-11-04
- Package layout superseded by [ADR
  0021](0021-git-board-v2-same-data-new-code.md) (2026-11-05): v2 keeps
  the layers, a kernel and the domains on it, but not these packages.
  Since the switch (v0.12) `core/` and `internal/` are gone; the layers
  are `kernel`, `store`, `view`, `conf`, `domain/…`, `forge`, `sync`,
  `tools`, `app` and `surface`, and the guard is `layers_test.go` at the
  module's root. The paths below are v0.11's.
- Builds on [ADR 0005](0005-core-is-a-library.md) (core/ is a library),
  [ADR 0018](0018-own-your-issues-work-in-progress-is-opt-in.md) (what the
  core is for) and [ADR 0002](0002-datastores.md) (datastores).

## Context

On 2026-11-04 Pierre decided that releases and deploys belong in the core,
and asked whether the core should be split: issues, releases, the cycle.

Today `core/` holds the operation log (`oplog`), the datastores
(`datastore`, `gitstore`, `pgstore`, `mdstore`), people, the issue kind,
the index, the forges and the sync (`syncer`). A release is already an
entity of the core's own kind, in `refs/board/release/<id>`, with its
operations in the spec; but its fold and its rules live in
`internal/store/releases.go` and `internal/release` (5,700 lines), next to
the web server. The cycle is in `internal/cycle`, `internal/work`,
`internal/proof`, `internal/wtx`. Fields, filters and the inbox, which
every surface needs, are in `internal/` too.

So three domains sit on one base, and only one of them, issues, is in
`core/`.

## Decision

**The core has a kernel and three domains on it. Each layer imports only
the layers below; the import guard test (`core/importguard_test.go`)
enforces it.**

| Layer | Packages (target) | What it owns |
|---|---|---|
| **Kernel** | `core/oplog`, `core/datastore` and the stores, `core/people`, `core/watch`, `core/forge` | operations, where they are kept, who wrote them, the forges' adapters |
| **Issues** | `core/issue`, `core/index`, `core/syncer`, `core/model` (the read model, from `internal/store`), `core/fields` (the schema, from `internal/config`, and the assignments, from `internal/fields`), `core/filter`, `core/inbox` (from `internal/filter`, `internal/inbox`), `core/search` (free text's tokens and match, which the index and the filter share) | issues, comments, fields as labels (ADR 0007), queries, the inbox, their sync with forges |
| **Releases** | `core/release` (the record, from `internal/store/releases.go`; versions and the overlay, from `internal/release`; the `[release]` table, from `internal/config`), `core/proof` (from `internal/proof`) | versions, what each shipped (which issues), deploys, signed check results (ADR 0001) |
| **Cycle** | `core/cycle` (the computation, from `internal/cycle`; the `[cycle]` table, from `internal/config`), `core/cycle/work` (from `internal/work`) | an issue's steps from intent to verified; experimental (ADR 0018) |

Issues know nothing of releases; releases read issues (what shipped); the
cycle reads both. Proof sits with releases because a deploy gate and the
cycle's signoff both read it, and releases are below the cycle.

**What's outside the core**, as apps over it: the CLI, the TUI, the web
server and the web app; insights (a report over issues); agents and
`wtx` (they drive tools, they keep no data); the GitHub Project bridge
(`internal/ghbridge`, GitHub-only, ADR 0018); attachments (an S3 adapter,
ADR 0015); since step 3, the release runner (`internal/ship`: it
cuts and deploys, driving git remotes, registries and the forge); and,
since step 4, what reads the cycle's evidence (`internal/evidence`: git's
branches and histories, GitHub's PRs).

### What the three-way split misses

1. **The kernel.** Issues, releases and the cycle all need the log, the
   datastores, people and sync. Without a named kernel, each domain grows
   its own copy, or the issue domain becomes the kernel by accident.
2. **People.** Who you are and who wrote what is shared by all three.
3. **Sync with forges.** Issues go to forges through `core/syncer`;
   releases are published to GitHub by `internal/ship/binaries.go`, on
   their own path and GitHub-only. Both should go through the kernel's
   forge layer, so a release can reach GitLab too.
4. **Proof.** Signed check results are used by deploys and by the cycle.
5. **Config.** `.git-board.toml` carries every domain's table. Each domain
   should own and check its own table, so a broken `[cycle]` can't break
   issues again (the bug ADR 0018 fixed by hand).
6. **The read model.** The index and the inbox are derived and
   disposable (ADR 0009); they belong with the domain they answer for.

### The guard

`core/layers_test.go` gives every core package a layer and fails when one
imports a layer above its own. A new core package without a layer fails
too. Tests may reach up (a release test builds issues to ship).

`"core"` names the root package alone: as a prefix it named every
package, so a package without a layer passed (fixed in step 3, with a
test).

`core/syncer` is in the issue layer, not the kernel: it syncs issues, and
nothing else. When releases go to forges through it, its generic half
(journal, lock, forge plans) moves down to the kernel.

## Consequences

- `internal/store` (13,000 lines) shrinks to wiring: it opens a board and
  hands out the domains.
- A third-party tool can import `core/issue` without the cycle, or
  `core/release` to read what shipped, as ADR 0005 wants.
- The move is mechanical but wide: every surface's imports change. It goes
  in steps, one domain per pull request, with the import guard first.

## Step 2: the issue domain's satellites

Done 2026-11-04. What moved, and what it settles:

- **`core/model`**: `Issue`, `Thread`, `Event`, `Person`, `Forge`,
  `Review`, `Version`, `MetaSet`, `Mentions`, from `internal/store`, names
  and JSON tags unchanged (`docs/guide/cli.md` › JSON output).
  `internal/store` folds the core's operations into them. Not
  `core/board`: `internal/board` and ten of its callers use them, and a
  package board importing a package board needs an alias.
- **`core/fields`**: the schema (`Field`, the types, `Sprints` with
  `Resolve` and `Windows`, `Labels`, the roles, in a `Schema`), from
  `internal/config`, and the assignments, from `internal/fields`.
  `internal/config` reads and checks the TOML and embeds `fields.Schema`
  in `Config`; `Config.IssueSchema` hands it to the core.
- **`core/filter`**, **`core/inbox`**: they read a `model.Issue` or
  `model.Thread` and a `*fields.Schema`; the inbox keeps its file in
  `inbox.Files`, the four calls it makes of a billy filesystem.

Fields and filters are the issue domain's, not the kernel's: a field is
a label (ADR 0007), labels are an issue's, and a release has none. Should
releases grow fields, the schema moves down then.

`[branches] kinds` stay in `internal/config`: they name branches too. The
core knows a flat kind by its field, `Field.Flat`, which config sets on
parse.

## Step 3: the release domain

Done 2026-11-04. What moved, and what it settles:

- **`core/release`**: the record and its rules, from
  `internal/store/releases.go`: `Record` (was `store.ReleaseRecord`),
  `Artifact`, `ChartArtifact`, `Asset`, `Comment`, the operations'
  payload `Data`; `Apply`, `Records` (one per commit, the smallest id
  counting), `For`, `Create`, `SetNotes`, `AddComment`, `Yank`, over any
  datastore. Versions and tags (`Version`, `ParseTag`, `Bump`, `NextRC`,
  `NextTag`, `DevVersion`, `Tag`, `TagsAt`) and the GitOps overlay
  (`Overlay`, `Deployed`, `Pin`), from `internal/release`. The `[release]`
  table, `Config` (was `config.Release`), with `Image`, `Chart`, `Target`
  and the environments' roles; `internal/config` reads and checks it. JSON
  tags unchanged.
- **`core/proof`**, from `internal/proof`, with `Suite` (was
  `config.Suite`). It parsed `.git-board.toml` at the base branch, which
  the core never reads (ADR 0005): `config.AtBase` reads it into a
  `proof.Base`, and `Base.Gate` judges a commit.
- **`internal/ship`** is what was left of `internal/release`: the
  `Runner`, which cuts, promotes, deploys, rolls back and adopts.
  Renamed, so that two packages `release` don't import one another.
- `internal/store` keeps the wiring: it writes records as you, names
  their authors, and pushes and pulls them with the issues.

**The Runner stays outside the core.** It keeps no data and drives
tools, like agents and `wtx`: it pushes tags to the app's remote, clones
and pushes the GitOps config repos, builds, scans and copies images,
pushes charts, builds binaries and publishes them with `gh`; each needs
the network or a token. `Tools` stays its seam for docker, crane, trivy,
helm and gh. Moving it into the core would put behind interfaces the
records, the issues, the remote's head and the config repos, an
interface of ten methods with one caller. What it keeps, the records, is
in the core, and so is every rule that needs nothing but data: which tag
a cut pushes, which environment it goes to, what an overlay says runs.
A tool reads what shipped from `core/release`: the records, the tags'
versions, the overlays.

**`internal/branches` stays.** The core doesn't need it: which issues a
release carries is the Runner's draft, which matches merged branches to
the store's issues by name (`branches.Names`) and reads `[branches]
kinds`. Should the draft move into the core, `Names` goes with the issue
domain: a branch names an issue.

**Deploys are neither a release's operations nor their own kind.** A
deploy is a commit to an app's config repo: its overlay's
`git-board/commit` and `git-board/version` annotations say what runs,
and its history says since when. The record keeps what was built
(artifacts, chart, assets), not where it went; a rollback only comments
on the records it rolled back from. The config repo is the deploy log,
as GitOps means it, and the board stores none.

## Step 4: the cycle

Done 2026-11-04. What moved, and what it settles:

- **`core/cycle`**: the computation, from `internal/cycle`: `Input`,
  `Compute`, `PickLane`, `ApprovedPlan`, the acceptance criteria
  (`Criteria`, `TickCriterion`), `PlanIssue`. The `[cycle]` table,
  `Config` (was `config.Cycle`), with `Stages`, `Step`, `DefaultSteps`
  and the kinds' rules, from `internal/config`: `Config.Stages(options)`
  fills the defaults and guesses the stages from the status field's
  options. `internal/config` reads and checks the table (`[cycle.kinds]`'s
  two forms, the steps' order, the suites) and keeps `Config.Stages()`
  for its callers. JSON unchanged.
- **`Gather` fetches nothing.** It computes the evidence from the thread
  and `Sources`: the stages, the issue's kind, the viewer (`inbox.Me`),
  the base and the issue's branches as plain `Branch` values, and three
  small seams: `Repo`, the five reads of git the evidence needs (the
  files a branch changed, the commit that added one, the files matching
  a pattern, a file, a commit's author); `Pulls`, a branch's PR, as
  before; `Render`, the plan's HTML.
- **`core/cycle/work`**, from `internal/work`: the Work page's rows and
  their groups, pure over the model, the inbox's `Me` and the cycle.
- **`core/forge.PR`**, with `Check` and `Review`, from `internal/ghpr`:
  the evidence carries the PR, and a PR is a forge's, not the cycle's.
  `internal/ghpr` keeps the GitHub client and `Gate`.
- **`internal/evidence`** is the app's side, Gather's caller: it reads
  the branches (`internal/branches`), hands the store in as `Repo` and the
  server's PR cache as `Pulls`, and renders the plan with
  `internal/markdown`.

**The switch stays the app's.** The core has no `[experimental]`:
`core/cycle` computes when it is asked, and the application decides
whether anyone asks: the server's routes, the CLI's commands, and
`config.Load`, which refuses a broken `[cycle]` only when the cycle is on
(ADR 0018). A board sees no difference.

**`internal/branches` stays.** Half of it names branches (`Name`,
`Folder`, `Next`, `Slug`, `Valid`, `Names`), pure over the `[branches]`
table; half reads git: `Suggest` picks the base from the remote's
branches and counts the folder's merge commits, `Linked` counts ahead
and behind through the histories. The cycle needs neither: it takes the
branches as values. The naming half's callers are all applications (the
CLI's `branch --create`, the server, the release draft), and moving it
would bring the `[branches]` table and `config.Slug` into the core for no
core caller. When a core package needs `Names` (the release draft, if it
moves), the naming half goes to the issue domain, with its table.

**`internal/markdown` stays.** The cycle needs nothing of it but the
plan's HTML; the criteria were already the cycle's own parser. Turning
untrusted text into HTML is where a page's safety is (goldmark,
bluemonday), the surfaces' concern: the app hands it in as
`Sources.Render`.

### What's left in internal/, and why

The four steps done, `internal/` holds the applications and what they
need of the outside world; no domain rule is left there.

| Package | Why it's the application's |
|---|---|
| `store` | the wiring: opens a board on its datastore, writes as you, names authors, pushes and pulls with git, folds the core's operations into `core/model` |
| `config` | reads `.git-board.toml`, a file the core never reads (ADR 0005), and checks each table into the core's types |
| `server`, `tui`, `board` (and `cmd/git-board`) | the surfaces, and their loading, sorting and grouping |
| `evidence`, `branches`, `ghpr` | the cycle's outside world: git's branches and histories, GitHub's pull requests |
| `ship` | the release runner (step 3) |
| `wtx`, `ai` | agents, their worktrees, a chat client: they drive tools and keep no data |
| `ghbridge`, `ghlabels`, `fromforge`, `migrate`, `lint` | tools over a board: the GitHub Project bridge, GitHub's labels, a board started from its forge, a field's values moved, drift reported |
| `uploads`, `avatar`, `markdown`, `insights` | attachments on S3 (ADR 0015), people's pictures, HTML, a report over issues |
| `hermetic` | the tests' seal |

## Left open

- Releases reach GitHub alone (`internal/ship/binaries.go`); through the
  kernel's forge layer they would reach GitLab too.
- Pull requests too: `forge.PR` speaks GitHub's words, and only
  `internal/ghpr` reads one. A GitLab merge request maps to it when the
  cycle runs on GitLab.
- Each domain checks its own table, the split's fifth point: the
  `[release]` and `[cycle]` checks are still in `internal/config`, which
  reads the TOML. They move to `core/release` and `core/cycle` when a
  third-party tool writes those tables.
- `internal/branches`' naming half goes to the issue domain when a core
  package needs `Names` (Step 4).
