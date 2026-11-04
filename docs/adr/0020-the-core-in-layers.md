# ADR 0020: The core in layers: a kernel, then issues, releases and the cycle

- Status: accepted (2026-11-04, Pierre: "we should have a kernel"). Step
  1, the guard, and step 2, the issue domain's satellites, are done
  (2026-11-04; see Step 2 below).
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
| **Releases** | `core/release` (from `internal/store/releases.go`, `internal/release`), `core/proof` (from `internal/proof`) | versions, what each shipped (which issues), deploys, signed check results (ADR 0001) |
| **Cycle** | `core/cycle` (from `internal/cycle`, `internal/work`) | an issue's steps from intent to verified; experimental (ADR 0018) |

Issues know nothing of releases; releases read issues (what shipped); the
cycle reads both. Proof sits with releases because a deploy gate and the
cycle's signoff both read it, and releases are below the cycle.

**What's outside the core**, as apps over it: the CLI, the TUI, the web
server and the web app; insights (a report over issues); agents and
`wtx` (they drive tools, they keep no data); the GitHub Project bridge
(`internal/ghbridge`, GitHub-only, ADR 0018); attachments (an S3 adapter,
ADR 0015).

### What the three-way split misses

1. **The kernel.** Issues, releases and the cycle all need the log, the
   datastores, people and sync. Without a named kernel, each domain grows
   its own copy, or the issue domain becomes the kernel by accident.
2. **People.** Who you are and who wrote what is shared by all three.
3. **Sync with forges.** Issues go to forges through `core/syncer`;
   releases are published to GitHub by `internal/release/binaries.go`, on
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

## Left open

- Whether deploys are a release's operations or their own kind.
- The order of the steps left: the issues' satellites went second; the
  proposal for the rest is releases, then the cycle.
