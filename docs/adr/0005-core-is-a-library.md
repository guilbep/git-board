# ADR 0005: `core/` is a library and never imports `internal/`

- Status: accepted (recorded 2026-11-04, decided earlier)
- Date: 2026-10-24 (`docs/prd/core.md` › Rules: "The library is a
  library"). The guard test exists since M1.
- Sources: `docs/spec/decisions.md` › Rules; `core/doc.go`;
  `core/importguard_test.go`.
- Alternatives below are reconstructed.

## Context

git-board is two things in one module: a data model with its sync (the
"core"), and an application around it (server, web app, TUI, CLI, the
cycle, releases, the GitHub Project bridge). The core replaced git-bug
(ADR 0003), whose application and library had grown into each other.
The PRD wanted the core usable on its own and small enough to hold in one's
head, with a spec it implements.

## Decision

**`github.com/guilbep/git-board/core/...` is importable by any Go program,
and imports nothing of `github.com/guilbep/git-board/internal/...`.**

- Enforced by `core/importguard_test.go:44`
  `TestCoreImportsNoGitBugNorInternal`: `go list -deps -test ./...` from
  `core/`, failing on any `.../internal` package, tests included.
  `:59` `TestGuardSeesForbiddenImports` checks the guard itself.
- The core reads no application file. It doesn't read `.git-board.toml`:
  git-board passes the forge projects, the posting account and
  `publish-private` as `syncer.Options` (`docs/spec/sync.md`, intro). No
  file under `core/` imports a TOML parser.
- Application policy reaches the core as callbacks in `syncer.Options`:
  `Held`, `Local`, `Quote`, `Carry`, `Restore`, `Published`
  (`core/syncer/sync.go:54-57` for `Held`, `:58` for `Assignees`; `docs/spec/sync.md` › Markers).
- Where the core needs a helper the application also has, it keeps its own
  copy: `core/internal/hermetic` is the twin of `internal/hermetic`
  (`core/internal/hermetic/hermetic.go:1-4`).
- New datastores are core packages for the same reason
  (`core/pgstore`, `core/mdstore`; ADR 0002 › Decision 1).

What sits where:

| `core/` | `internal/` |
| --- | --- |
| `datastore`, `gitstore`, `pgstore`, `mdstore`, `oplog`, `issue`, `people`, `index`, `watch`, `syncer`, `forge` and its adapters, `forge/conformance`; since ADR 0020's step 2, `model`, `fields`, `filter`, `inbox` | `store` (the `Backend` the server, TUI and CLI use), `config`, `server`, `tui`, `board`, `cycle`, `proof`, `release`, `ghbridge`, `uploads`, … |

## Alternatives considered

Reconstructed. Scored on: the core stays small and spec-shaped (35), a
second program can use it (25), cost of the boundary (20), refactor
freedom (20).

| Option | Score | Why not |
| --- | --- | --- |
| **`core/` public, guarded against `internal/` (this decision)** | 85 | |
| One tree, no boundary | 50 | Cheapest, but the core would drift back into the application, as git-bug's did |
| The core in its own module or repository | 60 | Two versions to release and pin for every change; the PRD wanted it "inside git-board" |
| `core/` under `internal/` | 45 | No other program could import it; the PRD lists other languages and tools as later users (`docs/prd/core-other-languages.md`) |

## Consequences

Good:

- The core's size is measurable and was measured at each milestone
  (`docs/spec/decisions.md` › Trial, M2, M3 results).
- The forge adapters, the conformance suite and the datastores are
  reusable by anyone.
- A reviewer knows where a rule belongs: the spec's rules in `core/`,
  product choices in `internal/`.

Bad, and accepted:

- Some code exists twice (`hermetic`).
- `core/` is a public API in practice: a change to an exported name may
  break an importer outside the repository. No compatibility promise is
  written yet.
- Policy has to be threaded through `syncer.Options`, which keeps growing.

Unchanged: the CLI, the server and the TUI keep going through
`internal/store`.

## Left open

- Whether `core/` promises any API stability, and how it would be
  versioned (it shares the module's tags).
- The guard forbids `internal/` and git-bug only. Nothing stops `core/`
  from importing a heavy third-party package; that rests on review and on
  ADR 0017.
