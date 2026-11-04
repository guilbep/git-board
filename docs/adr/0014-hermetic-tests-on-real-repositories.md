# ADR 0014: Tests are hermetic, on real repositories and fakes, never on the network

- Status: accepted (recorded 2026-11-04, decided earlier)
- Date: 2026-10-26 for the seal (commit "test: every test binary is sealed
  off the network and the user's tokens"); "real fixtures over mocks" is
  older (`CONTRIBUTING.md` › Conventions; `docs/implementation-plan.md`,
  2026-09-02: "real-repo fixtures, no mocks").
- Sources: `internal/hermetic`, `core/internal/hermetic`,
  `core/forge/conformance`, `CONTRIBUTING.md` › Conventions, Live tests;
  `CLAUDE.md` › Tests.
- Alternatives below are reconstructed.

## Context

git-board talks to git, to GitHub and GitLab, and to a developer's
credential helper. A test that reaches any of them can post to a real
forge with the developer's token, read their global git config, depend on
the network, or pass only on one machine. Several agents run tests in
parallel worktrees on one machine (ADR 0001's context).

## Decision

### 1. Every test binary is sealed

Each test package's `TestMain` calls `hermetic.Run`
(`internal/hermetic`; its twin `core/internal/hermetic/hermetic.go:23`,
since `core/` can't import `internal/`, ADR 0005). Before any test runs:

- `http.DefaultTransport` dials loopback hosts only, with no proxy
  (`core/internal/hermetic/hermetic.go:61-73`);
- forge tokens and hosts, proxies and `GIT_BOARD_DATASTORE` are unset
  (`:51-54`);
- git reads no system config and a throwaway global one that only names a
  committer, so no credential helper answers; `GIT_ALLOW_PROTOCOL=file`
  and `GIT_SSH_COMMAND=false` keep git on local repositories
  (`:41-50`, `:55-60`);
- `BOARD_LIVE` skips the seal for the live tests, which choose their own
  scratch projects (`:24-26`).

`internal/hermetic/hermetic_test.go` `TestSealed` proves it: a loopback
server answers, `api.github.com` and `git ls-remote https://…` are
refused, no token and no credential helper are left.

### 2. Real repositories and fakes, not mocks

- Tests create throwaway git repositories on disk and run the real
  `gitstore`, `oplog` and `internal/store` on them (`storetest`).
- Forges are fakes that keep state (`core/forge/fake`), served over
  loopback HTTP to the real adapters. One conformance suite runs the same
  scenarios on the fake and on the GitHub and GitLab adapters
  (`core/forge/conformance/conformance.go:37` `Run`; ADR 0008).
- External CLIs are replaced by a script on `PATH` or a package variable
  seam (`ghbridge.Gh`, `internal/ghbridge/ghbridge.go`), not by an
  interface mock (`CONTRIBUTING.md:103-106`).
- The browser tests serve a throwaway repository with a fake GitHub on
  loopback: "nothing leaves the machine" (`CONTRIBUTING.md:46-52`).
- Every bug fix ships with a test that fails without it.

### 3. Live runs are separate and named

Live tests run with `BOARD_LIVE` on scratch forges only
(guilbep/git-board-test, guilbep/git-bug-sync-test, gitlab.epfl.ch 42400,
GitHub Project #1), never on a real board (`CONTRIBUTING.md` › Live tests).

## Alternatives considered

Reconstructed. Scored on: a test can't touch a real forge or token (35),
tests catch real git and forge behaviour (30), speed and parallel runs
(20), cost (15).

| Option | Score | Why not |
| --- | --- | --- |
| **Sealed binaries, real repos, stateful fakes, one conformance suite (this decision)** | 85 | |
| Interface mocks per call | 45 | Fast, but miss git's real behaviour, which caused most of the core's surprises (`docs/spec/decisions.md` › "What was harder than planned") |
| Recorded HTTP fixtures (VCR) | 50 | Recordings go stale and need a real token to refresh |
| Rely on discipline: no seal, tests "shouldn't" call out | 30 | One forgotten test posts with a developer's token |
| Run tests in a container without network | 55 | Seals the network but not the developer's environment in local runs; heavier |

## Consequences

Good:

- `go test ./...` is safe on any developer machine and in parallel
  worktrees.
- Adapters are held to the same scenarios as the fake.
- Git quirks show in tests (the go-git and lock-file problems of ADR 0006
  were found this way).

Bad, and accepted:

- Tests that spawn git are slower than mocks; `make ci` takes minutes.
- The seal covers `http.DefaultTransport` only: a client built with its own
  transport would bypass it.
- Tests that set environment variables or chdir can't be parallel
  (`CLAUDE.md` › Tests).

## Left open

- ~~Not every test package was sealed~~: settled 2026-11-04. The four
  that weren't (`core/datastore/memstore`, `internal/avatar`,
  `internal/fromforge`, `internal/migrate`) are, and
  `internal/hermetic`'s `TestEveryPackageIsSealed` fails for any package
  under `cmd/`, `internal/` or `core/` whose tests don't call
  `hermetic.Run`.
- `CONTRIBUTING.md:103` still calls the fixture "a real on-disk git-bug
  repo" (ADR 0003).
- `make web-e2e` is not in `make ci`.
