# ADR 0020: The core in layers: a kernel, then issues, releases and the cycle

- Status: accepted (2026-11-04, Pierre: "we should have a kernel"). Step
  1, the guard, step 2, the issue domain's satellites, and step 3, the
  release domain, are done (2026-11-04; see Step 2 and Step 3 below).
- Date: 2026-11-04
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
| **Issues** | `core/issue`, `core/index`, `core/syncer`, `core/model` (the read model, from `internal/store`), `core/fields` (the schema, from `internal/config`, and the assignments, from `internal/fields`), `core/filter`, `core/inbox` (from `internal/filter`, `internal/inbox`) | issues, comments, fields as labels (ADR 0007), queries, the inbox, their sync with forges |
| **Releases** | `core/release` (the record, from `internal/store/releases.go`; versions and the overlay, from `internal/release`; the `[release]` table, from `internal/config`), `core/proof` (from `internal/proof`) | versions, what each shipped (which issues), deploys, signed check results (ADR 0001) |
| **Cycle** | `core/cycle` (from `internal/cycle`, `internal/work`) | an issue's steps from intent to verified; experimental (ADR 0018) |

Issues know nothing of releases; releases read issues (what shipped); the
cycle reads both. Proof sits with releases because a deploy gate and the
cycle's signoff both read it, and releases are below the cycle.

**What's outside the core**, as apps over it: the CLI, the TUI, the web
server and the web app; insights (a report over issues); agents and
`wtx` (they drive tools, they keep no data); the GitHub Project bridge
(`internal/ghbridge`, GitHub-only, ADR 0018); attachments (an S3 adapter,
ADR 0015); and, since step 3, the release runner (`internal/ship`: it
cuts and deploys, driving git remotes, registries and the forge).

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

## Left open

- The cycle, the step left: `internal/cycle` and `internal/work` to
  `core/cycle`.
- Releases reach GitHub alone (`internal/ship/binaries.go`); through the
  kernel's forge layer they would reach GitLab too.
