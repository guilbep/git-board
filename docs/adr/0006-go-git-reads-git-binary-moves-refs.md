# ADR 0006: One binary, built in layers

- Status: accepted; merged 2026-11-09 from ADR 0006 (decided 2026-10-24),
  ADR 0005 (2026-10-24), ADR 0012 (2026-09-02), ADR 0020 (2026-11-04) and
  ADR 0021 (2026-11-05)
- Date: 2026-09-02, 2026-10-24, 2026-11-04, 2026-11-05
- Absorbs [ADR 0005](0005-core-is-a-library.md) (the core is a library),
  [ADR 0012](0012-one-go-binary-web-app-embedded.md) (one static binary),
  [ADR 0020](0020-the-core-in-layers.md) (the layers) and
  [ADR 0021](0021-git-board-v2-same-data-new-code.md) (v2: new code, the
  same data).
- Design: [docs/rewrite/design.md](../rewrite/design.md). History:
  [How v2 came to be](../rewrite/history.md).

## Context

Developers use git-board in a terminal, project managers and QA in a
browser, and CI from a script. The project avoids a server to host. The
board is a set of domains on one base: issues, releases and the
development cycle. A domain rule that lives next to the web server is a
second implementation of it. v0.11 grew that way, and v0.12 was written
again from a design.

## Decision

### 1. One static binary

git-board ships as one binary, `git-board`, so `git board` works as a git
subcommand. It is the CLI, the terminal UI, the web server and the web
app.

- `make dist` builds it with `CGO_ENABLED=0 -trimpath` for linux and
  darwin (amd64, arm64) and windows/amd64, with `checksums.txt`.
- `git board` with no command opens this repository's web app, and starts
  a server on localhost when none runs. `git board serve` runs the server
  alone, and `git board tui` the terminal UI.
- The web app is Vue 3 and Quasar, built into `web/dist` and embedded
  with `//go:embed all:dist` (`web/embed.go`). The starter
  `.git-board.toml` files are embedded too (`app/init.go`).
- The HTTP server is the standard library's `net/http`, with no router
  framework.
- `make ci`, `make install` and `make dist` build the web app first,
  because the binary embeds whatever `web/dist` holds.
- The server writes error, warning and info lines, at a level the user
  chooses (`--log-level`).

### 2. The module is built in layers

Each package of the module has a layer, and imports only the layers
below it. `layers_test.go` at the module's root holds the table of what
each package can import. `TestLayers` fails on an import outside the
table, and on a package missing from it. `TestLayersHaveNoCycle` keeps
the table free of cycles. Tests can import a short list more: the
datastores, the view, the fakes and the conformance suite.

| Layer | Packages | What it owns |
| --- | --- | --- |
| Datastores | `store`, `store/git`, `store/pg`, `store/md`, `store/mem`, `store/forgecopy` | where packs are kept ([ADR 0002](0002-datastores.md)) |
| Kernel | `kernel`, `kernel/pack`, `kernel/search` | operations, their order and writes, for any kind |
| View | `view` | the disposable read model |
| Configuration | `conf` | the file, its edits and its schema version |
| Forges and sync | `forge` and its adapters, `sync` | the forge interface, the sync engine and its records |
| Domains | `domain/people`, `domain/issues`, `domain/releases`, `domain/releases/proof`, `domain/cycle` | each kind's rules and its own table of the file |
| Tools | `tools/...` | what drives the outside world: the release runner, branches, the Project bridge, uploads, insights |
| Application | `app` | opens a board and gives every surface one API |
| Surfaces | `surface/cli`, `surface/http`, `surface/tui`, `web` | what people and scripts call |

- The kernel knows no domain. Issues know nothing of releases or of the
  cycle. The cycle reads issues and the proof.
- The release runner (`tools/ship`) stays outside the domains. It keeps
  no data and drives tools: git remotes, registries, the forge. The
  records it writes are `domain/releases`.
- A deploy is a commit to an app's configuration repository. That
  repository is the deploy log, and the board stores no deploy.
- Every surface calls `app`. No surface holds a domain rule of its own.

### 3. Reads go through `git cat-file`, writes through `git update-ref`

- The git datastore reads objects through `git cat-file --batch-command`,
  one process per CPU for a bulk load (`store/git/git.go`). It reads
  loose and packed refs itself.
- It writes objects in process, as loose objects.
- It moves refs with `git update-ref <ref> <new> <old>`: the
  compare-and-swap of [ADR 0004](0004-entities-are-operation-logs.md).
- `git fetch` and `git push` move the board, so every remote, URL and
  credential helper that git knows works.
- Every git runs with `LC_ALL=C`, because git-board reads git's refusals
  by their English text.
- No package of the binary uses go-git.

### 4. v0.12 is new code on the same data

v0.12 replaced v0.11. It keeps the data byte for byte, and changes the
rest where that makes it better.

- A board kept by v0.11 opens in v0.12 unchanged. `make compat` holds
  v0.12 to v0.11's recorded outputs on the golden boards
  ([compat](../rewrite/compat.md)).
- The CLI's commands, flags, `--json` fields and exit codes stay. The
  HTTP API is new, with the web app.
- Writes are allowed during a sync. The compare-and-swap keeps them safe.
- `html` is gone. The agents and `wtx` are their own binary,
  `git-board-agents`, in `contrib/board-agents`, a module of its own. It
  talks to a board through git-board's CLI and HTTP API alone.
- The GitHub Project bridge stays, as a sync target beside the forges.
- The releases go to the forges through the sync engine
  (`domain/releases/releasesync`), ours to the forge, never back.

## Consequences

Good:

- Install is one file, which `make install-latest` compares with `checksums.txt`.
- The CLI, the server and the TUI share `app` and its rules.
- A reviewer knows where a rule belongs: its layer says so, and the test
  holds it there.
- A bulk load is about five times faster than with go-git.

Bad, and accepted:

- A build needs Node and npm as well as Go. A plain `go build` embeds
  whatever `web/dist` holds, so a stale build ships a stale UI.
- `git` must be on `PATH`.
- Error handling depends on git's English messages.
- A new package takes its row in the table first.
