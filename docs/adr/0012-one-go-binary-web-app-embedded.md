# ADR 0012: One static Go binary, the web app and its reference docs inside

- Status: accepted (recorded 2026-11-04, decided earlier)
- Date: 2026-09-02 for the embedded web app (`docs/implementation-plan.md`
  › Status update (2026-09-02): web UI landed); 2026-10-24 for "Go" and
  "one binary, `git board`, for people and for CI" (`docs/prd/core.md` ›
  The decision).
- Sources: `web/embed.go`, `docs/embed.go`, `internal/server/serve.go`,
  `Makefile` (`dist`, `ci`, `install`).

## Context

git-board is used by developers in a terminal, by PMs and QA in a browser,
and by CI. The project avoids standing up a server or a SaaS
(`docs/implementation-plan.md` › Why this exists). The implementation plan
of 2026-09-02 weighed a TUI against a web app and recommended the TUI first;
the web app was built next "on explicit ask", with a Go backend, not the
PRD's earlier FastAPI sketch.

## Decision

**git-board ships as one binary, `git-board` (so `git board` as a git
subcommand), built with `CGO_ENABLED=0` for five platforms. It is the CLI,
the TUI, the web server and the web app.**

- `git-board` with no command opens this repository's web app, starting a
  server on localhost if none runs (`cmd/git-board/cmd.go:51-54`,
  `runBoard`); `git board serve` runs the server alone; `git board tui`
  the terminal UI (bubbletea).
- The web app is Vue 3 and Quasar (`web/package.json`), built into
  `web/dist` and embedded with `//go:embed all:dist` (`web/embed.go:10`).
  The server serves `dist/spa` (`internal/server/serve.go:53-59`). A
  tracked `web/dist/.gitkeep` keeps `go build ./...` working before any
  web build.
- The configuration reference and the forges guide are embedded too, for
  the settings page (`docs/embed.go:8`, `:11`), as are the default
  `.git-board.toml` and its templates (`internal/config/default.go:13`,
  `:16`) and the EPFL design tokens (`web/epfl.go`).
- The HTTP server is the standard library's `net/http` with method
  patterns: no router framework (`docs/implementation-plan.md`, same
  update).
- Releases: `make dist` builds linux, darwin (amd64, arm64) and
  windows/amd64 with `CGO_ENABLED=0 -trimpath` (`Makefile:83-88`), web app
  inside, with `checksums.txt`.
- Build order matters: `make ci` and `make install` run the web build
  first, because the binary embeds whatever `web/dist` holds.

## Alternatives considered

Partly recorded (TUI first, FastAPI sketch, Go over Rust); the rest
reconstructed. Scored on: nothing to deploy (30), PMs and QA can use it
(25), one artifact for every platform (25), build cost (20).

| Option | Score | Why not |
| --- | --- | --- |
| **One Go binary, web app embedded (this decision)** | 85 | |
| CLI and TUI only, a static HTML export for others | 55 | The plan's first recommendation; non-developer adoption was the named risk, and `git-board html` alone didn't cover editing |
| A separate web server (FastAPI) and SPA, deployed | 35 | A server to host, auth to add; against "no server" |
| Serve the web app from disk next to the binary | 55 | Two artifacts to keep in step; `go install` would ship no UI |
| Desktop app (Electron, Wails) | 40 | A second toolchain and installer per platform; CI still needs the CLI |
| Rust instead of Go | 65 | Second in the PRD's scoring; git-board's 27,000 lines and 22,000 lines of tests are Go |

## Consequences

Good:

- Install is one file (`make install-latest` checks it against
  `checksums.txt`).
- The server, the TUI and the CLI share `internal/store` and its rules.
- Each repository gets its own server on its own port.

Bad, and accepted:

- Building needs Node and npm as well as Go; a Go-only build gives an
  empty UI.
- `web/dist` is embedded untracked: a stale build ships a stale UI, and
  ADR 0001 has to list `web/dist` among the Go tests' inputs.
- The binary carries the whole web app even for CI use.
- `git` must still be on `PATH` (ADR 0006).

## Left open

- A plain `go build` or `go install` embeds whatever `web/dist` holds; only
  the `make` targets build it first. `CONTRIBUTING.md:10-17` still documents
  `go install` of a private module, which would ship an empty UI.
- `web-e2e` (Playwright) is not in `make ci` yet: CI would need a browser
  (`CONTRIBUTING.md:51-52`).
