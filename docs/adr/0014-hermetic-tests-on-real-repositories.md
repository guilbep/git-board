# ADR 0014: Tests on real repositories, few licences

- Status: accepted; merged 2026-11-09 from ADR 0014 (decided 2026-09-02,
  sealed 2026-10-26) and ADR 0017 (decided 2026-10-24, scanned from
  2026-10-25)
- Date: 2026-09-02, 2026-10-25, 2026-10-26
- Absorbs [ADR 0017](0017-permissive-licences-few-dependencies.md) (MIT,
  permissive dependencies only, and few of them).
- Contributing: `CONTRIBUTING.md`, Conventions and Live tests.

## Context

git-board talks to git, to GitHub and GitLab, and to a developer's
credential helper. A test that reaches one of them can post to a real
forge with the developer's token, read their global git config, depend on
the network, or pass on one machine alone. Several agents run tests in
parallel worktrees on one machine.

Every dependency is code to audit, to build with `CGO_ENABLED=0`, and to
ship inside one binary. A GPL dependency stops anyone from embedding
git-board in a tool that is not GPL.

## Decision

### 1. A test binary is sealed off the network and the user's tokens

A package's `TestMain` calls `hermetic.Run` (`hermetic/hermetic.go`).
Before any test runs:

- `http.DefaultTransport` dials loopback hosts alone, with no proxy.
- The forge tokens and hosts, the proxies and `GIT_BOARD_DATASTORE` are
  unset.
- git reads no system configuration, and a throwaway global one, so no
  credential helper answers.
- `GIT_ALLOW_PROTOCOL=file` and `GIT_SSH_COMMAND=false` keep git on local
  repositories.
- `BOARD_LIVE` skips the seal for the live tests, which choose their own
  scratch projects.

### 2. Real repositories and fakes, not mocks

- A test creates throwaway git repositories on disk, and runs the real
  datastores, kernel and application on them.
- A forge is a fake that keeps state (`forge/fake`), served over loopback
  HTTP to the real adapters. One conformance suite runs the same
  scenarios on the fake and on the GitHub and GitLab adapters
  (`forge/forgetest`).
- A script on `PATH` or a package variable replaces an external CLI. No
  interface mock replaces it.
- The browser tests serve a throwaway repository with a fake GitHub on
  loopback.
- `make compat` runs git-board over the golden boards, and compares its
  outputs with v0.11's.
- Every bug fix ships with a test that fails without it.

### 3. Live runs are separate and named

Live tests run with `BOARD_LIVE`, on scratch forges only, never on a real
board. `CONTRIBUTING.md` names the scratch forges.

### 4. MIT, and no GPL module

- git-board is MIT (`LICENSE`), and so is the web package.
- No GPL, LGPL or AGPL module is built. `TestLicences`
  (`cmd/git-board/licences_test.go`) lists every module that the binary
  and its tests build, reads its licence from the module cache, and
  fails on any of them. It runs in `make ci`. `make licences` prints the
  list.
- The web app's runtime packages are on an allow-list: MIT, Apache-2.0,
  BSD, MPL-2.0, ISC, Unlicense and 0BSD (`TestLicencesOfTheWebApp`).

### 5. Few dependencies

- No new dependency for what the standard library, a dependency in place
  or a few lines cover. S3 requests are signed by hand, the server is
  `net/http`, and git-board runs `git` rather than importing it.
- A plan names each new dependency, with its licence and the reason.
  `modernc.org/sqlite` is sqlite in pure Go. `pgx` is there because the
  standard library has no PostgreSQL driver. `gojq` gives `--jq` in pure
  Go.
- Development tools are pinned with `go.mod`'s `tool` line, not installed
  apart.

## Consequences

Good:

- A sealed package's tests are safe on any developer machine and in
  parallel worktrees.
- The adapters are held to the same scenarios as the fake.
- git's real behaviour shows in the tests.
- Anyone can embed or redistribute the binary.
- A GPL module anywhere in the build fails `go test`, before review.

Bad, and accepted:

- Tests that start git are slower than mocks. `make ci` takes minutes.
- The seal covers `http.DefaultTransport` only. A client with its own
  transport goes around it.
- In v0.12, 15 packages call `hermetic.Run`, and no test fails a package
  that does not. `app` and `surface/cli` are among those that do not.
- Tests that set environment variables or change directory run serially.
- The scan reads an SPDX `OR` as an `AND`, so "MIT OR GPL-3.0" fails.
- Some libraries are out of reach, such as a GPL keyring or diff library.
- `make web-e2e` is not in `make ci`, because it needs a browser.
