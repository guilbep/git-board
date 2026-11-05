# ADR 0017: MIT, permissive dependencies only, and few of them

- Status: accepted (recorded 2026-11-04, decided earlier)
- Date: 2026-10-24 for "MIT once nothing links GPL code" (`docs/prd/core.md`
  › The decision); 2026-10-25 for the scan and the move to MIT
  (`docs/spec/decisions.md` › M6 results). "No new dependency for what the
  stdlib or a few lines covers" is older (`CONTRIBUTING.md` › Conventions,
  `docs/implementation-plan.md`).
- Sources: `cmd/git-board/licences_test.go`, `LICENSE`, `Makefile`
  (`licences`), `docs/prd/core-m2-plan.md:28`.

## Context

git-board linked git-bug, which is GPL-3.0, so it couldn't be MIT
(ADR 0003). The code is guilbep's, and the vision is a tool any team can
adopt and embed (`core/` is a library, ADR 0005). Every dependency is also
code to audit, to keep building with `CGO_ENABLED=0` (ADR 0012), and to
ship inside one binary, the web app's npm packages included.

## Decision

### 1. Licences

- git-board is MIT (`LICENSE`, "Copyright (c) 2026 guilbep"), and so is
  the web package's `license` field.
- **No GPL, LGPL or AGPL module is built**, ever. `TestLicences`
  (`cmd/git-board/licences_test.go:51`) lists every module the binary and
  its tests build (`go list -deps -test ./...`), reads its licence from the
  module cache, and fails on any of them. It runs with `go test`, so in
  `make ci`; `make licences` prints the list.
- **The web app's runtime packages** (each non-`dev` entry of
  `web/package-lock.json`) must be on an allow-list: MIT, Apache-2.0, BSD-*,
  MPL-2.0, ISC, Unlicense, 0BSD (`:93` `permissive`, `:109`
  `TestLicencesOfTheWebApp`). No network needed.
- Modules only in the module graph and never built are not scanned.

### 2. Few dependencies

- No new dependency for what the standard library, an existing dependency
  or a few lines covers (`CONTRIBUTING.md:115-119`). Examples in the code:
  S3 signing by hand (ADR 0015), `net/http` with no router (ADR 0012),
  shelling out to `git` and `gh` rather than importing them.
- New dependencies are named in the plans, with their licence and why:
  `modernc.org/sqlite` "the one new dependency" of M2, pure Go
  (`docs/prd/core-m2-plan.md:28`); `pgx`, "the standard library has no
  PostgreSQL driver" (ADR 0002 § 6); `github.com/itchyny/gojq` (MIT, with
  `itchyny/timefmt-go`, MIT) for `--jq`, a jq in pure Go: the standard
  library has none, and `gh` uses it too (`docs/guide/cli.md` › JSON
  output).
- Development tools are pinned with go.mod's `tool` line (lefthook), not
  installed separately (`CONTRIBUTING.md:65`).

## Alternatives considered

Partly recorded (the PRD made MIT conditional on leaving GPL); the rest
reconstructed. Scored on: anyone may embed and ship it (35), no licence
surprise in a dependency (30), cost (20), choice of libraries (15).

| Option | Score | Why not |
| --- | --- | --- |
| **MIT, scanned in every test run (this decision)** | 85 | |
| Stay GPL-3.0 with git-bug | 40 | Rules out embedding the core in non-GPL tools; the fork was leaving anyway |
| Apache-2.0 | 75 | Equally permissive with a patent grant; nothing recorded on why MIT was preferred |
| MIT, checked by hand at review | 50 | A transitive GPL module would slip in unseen |
| An external licence scanner (go-licenses, FOSSA) | 60 | A tool or service to run; `go list` and the module cache suffice |

## Consequences

Good:

- The core and the binary can be embedded or redistributed by anyone.
- A GPL module anywhere in the build fails `go test`, before review.
- The binary stays small and pure Go.

Bad, and accepted:

- Some libraries are out of reach (libgit2's GPL-with-exception would need
  a ruling; a GPL keyring or diff library is out).
- The scan reads licences by SPDX id or by the title at the top of the
  file; an unusual licence file reads as "unknown".
- An SPDX `OR` is read as an `AND`, so "MIT OR GPL-3.0" fails
  (`licences_test.go:96-97`, a `ponytail:` note).

## Left open

- **Drift: the recorded count.** `docs/spec/decisions.md` › M6 results
  lists 57 modules on 2026-10-25. `pgx` (MIT) and its `puddle` have joined
  since (ADR 0002), so the table is stale; the test, not the table, is the
  truth.
- **Drift: public licence, private module.** `LICENSE` is MIT, while
  `CONTRIBUTING.md:10-17` says "This is a private module" and needs
  `GOPRIVATE`. Whether and when the repository is published is open.
- The `tool` line pulls lefthook's module graph (koanf, urfave/cli, …) into
  go.mod as indirect requirements. Those are never built into the binary,
  so the scan doesn't see them.
