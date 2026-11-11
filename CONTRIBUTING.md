# Contributing

## Setup

- Go 1.26.6+ (`GOTOOLCHAIN=auto` will fetch a newer one automatically if
  `go.mod` needs it).
- This is a private module: `go install`/`go get` need
  ```
  go env -w GOPRIVATE=github.com/guilbep/*
  git config --global url."git@github.com:".insteadOf "https://github.com/"
  ```
  Without `GOPRIVATE`, `go install` tries the public module proxy/checksum
  DB first, 404s (it's private), and fails — even with the SSH rewrite
  above already in place.
- Node 22.12+ for the web app (`web/`), which the binary embeds.
- PostgreSQL, optional: the PostgreSQL datastore's tests and its golden
  board run on the server `GIT_BOARD_TEST_POSTGRES` names (a DSN whose
  user may create databases), and are skipped without it.

## Layout

One Go module, `github.com/guilbep/git-board`, in layers ([the
design](docs/rewrite/design.md#packages), [ADR
0020](docs/adr/0020-the-core-in-layers.md), [ADR
0021](docs/adr/0021-git-board-v2-same-data-new-code.md)), from the bottom:

| Path | What |
|---|---|
| `store/` | the datastores: `git`, `pg`, `md`, `forgecopy`, `mem`, and `storetest`, the suite every one passes |
| `kernel/` | entities, operations and packs (`kernel/pack`), the search tokenizer (`kernel/search`) |
| `view/` | the index the lists read (SQLite, or the PostgreSQL datastore's tables) |
| `conf/` | `.git-board.toml`, table by table, and its schema versions |
| `domain/` | the kinds and their rules: `issues` (with `issuesync`, `mdfiles`), `people`, `releases` (with `proof`, `releasesync`), `cycle` |
| `forge/`, `sync/` | the GitHub and GitLab adapters, a fake and their conformance suite; the sync engine |
| `tools/` | the release runner (`ship`), the GitHub Project bridge (`ghproject`), `insights`, `uploads`, `boardgen`, `perfcheck` |
| `app/` | opens a board and wires the domains: the one API every surface calls |
| `surface/` | `cli`, `http` (the server and its API) and `tui`, on `app` |
| `cmd/git-board` | the binary; `cmd/boardgen` makes the boards `make perf` times |
| `web/` | the web app (Vue on Vite), embedded in the binary |
| `golden/`, `testdata/golden/` | the golden boards and the outputs git-board is held to (`make compat`, [compat.md](docs/rewrite/compat.md)) |
| `contrib/board-agents` | `git-board-agents`, a module of its own: agents and wtx, through git-board's CLI and HTTP API |

`layers_test.go`, at the root, holds the layers: each package imports only
what its row of the table names, a new package takes its row first, and
nothing imports `contrib/`.

## Workflow

```bash
make         # make help: every target, in a line each
make build   # go build ./...
make test    # go test ./...
make test-pg # the PostgreSQL datastore's tests, the view's and the golden board's on it (GIT_BOARD_TEST_POSTGRES)
make vet     # go vet ./...
make fmt     # gofmt -l . (lists files needing formatting; run gofmt -w to fix)
make web-check # the web app's typecheck and tests
make web-e2e # the browser tests (web/e2e, Playwright) on this checkout's binary and throwaway boards; first run: cd web && npx playwright install chromium
make web-lint  # the EPFL adherence linter on web/src, reporting only (EPFL_SKILL=../epfl-design-skill)
make compat  # git-board over the golden boards, against the recorded outputs (docs/rewrite/compat.md)
make golden  # record those outputs again from git-board: a change of the contract, its diff reviewed
make dogfood DIR=<repo> # a real board read by v1 (V1_REF) and by git-board, every issue compared
make perf    # the timed budgets on 5,000 and 20,000 issues (testdata/perf/budgets.txt)
make agents  # build and test contrib/board-agents, its own module
make ci      # the web app's checks and build, then build, vet, test, compat, gofmt: what CI runs
make install # the web app built, then go install the git-board binary
make install-latest # the latest release's binary instead, web app inside
make dist    # a release's archives, in dist/
make docs    # the docs site (guide, spec, decisions), in site/: a broken link fails it
make docs-serve # the docs site on localhost, rebuilt on save
```

Run `make ci` once a change is done, before it's pushed or landed: CI runs
`make ci`, and `make web-e2e` in a job beside it. It installs the web app's dependencies
(`npm ci`) only when `web/package-lock.json` is newer than `web/node_modules`.
While working, the package's own tests are enough
(`go test ./surface/http/ -run TestName`).

A page's regression gets an e2e test in `web/e2e/` (we favor them over unit
tests for what the browser shows). `web/e2e/serve.mjs` builds the web app and
the binary, and serves throwaway boards on `E2E_PORT` (18421) and the ports after it:
a git board with a GitHub origin and a fake GitHub, and a Markdown board
beside it. Nothing leaves the machine; `E2E_TOML=<path>` serves another
board's file instead, for a one-off smoke run of `e2e/tabs.spec.ts`. `make web-e2e` is not part of `make ci`, which stays fast:
CI runs it in its own job (`e2e` in `.github/workflows/ci.yml`), Playwright's
Chromium in a cache.

The `web/` frontend (Vue on Vite) has its own toolchain: `make web-dev` /
`make web-build`, or `cd web && npm run dev`/`build`/`typecheck` directly
([web/README.md](web/README.md)).

## Commit hooks

Install the hooks once per clone (worktrees share them):

```bash
go tool lefthook install
```

lefthook is pinned by the `tool` line in `go.mod`. Its pre-commit
(`lefthook.yml`) runs gofmt on the staged `.go` files, `go vet ./...`, and the
web typecheck when `.ts` or `.vue` files under `web/` are staged. There is no
pre-push hook: wtx's push guard is `.git/hooks/pre-push`.

## Live tests

Only on the scratch forges: guilbep/git-board-test, guilbep/git-bug-sync-test,
gitlab.epfl.ch project 42400, GitHub Project #1. Run the server on a scratch
clone (`git board serve --addr localhost:<port>`), never on a real board.

guilbep/git-board-test is set up for the PR gate and review (M3, M4):

- **Public**: rulesets need a public repository or a paid plan.
- **`.github/workflows/tests.yml`** on `main`, a job named `Tests`.
- **`.github/CODEOWNERS`**: `* @guilbep-gbtest`.
- **guilbep-gbtest**, a second account, collaborator with write access
  (GitHub counts a code owner only if they can write).
- **A ruleset on `main`**, enforcement *Active* (not Evaluate), target the
  default branch: require a pull request, with *Require review from Code
  Owners*; require status checks to pass, with `Tests`. Check it took with
  `curl https://api.github.com/repos/guilbep/git-board-test/rules/branches/main`:
  `[]` means nothing is enforced.
- **Each account's token in git's credential helper**, never in a file or
  on the command line: Settings › Sync › Add a forge stores it with
  `git credential approve`, or `GH_TOKEN`/`GITLAB_TOKEN` for their own host
  only (ADR 0010). git-board picks the token whose login is the account of
  the person you write as on that forge, so choosing another person
  (`git board me <id>`) switches accounts.
- Tokens are classic, `repo` scope only: a GraphQL field that needs
  `read:org` fails the whole query.

Write a full sync from a scratch clone with care: git-board-test holds
issues and labels from earlier tests, and a dry run shows what it would
send. The per-issue sync touches only that issue.

## Conventions

- Real fixtures over mocks: tests run on real on-disk git repositories
  (`hermetic`, `store/storetest`, `tools/boardgen`), real datastores, a forge
  kept in memory that passes the adapters' conformance suite
  (`forge/fake`, `forge/forgetest`), and fakes served by `httptest`
  (`tools/ghproject/projectfake`), not hand-rolled mocks ([ADR
  0014](docs/adr/0014-hermetic-tests-on-real-repositories.md)).
- Every bug fix ships with a regression test that fails without the fix.
  A crash fix lands on its own, the day it is found, not with the branch
  it was found on.
- A new act a person does on a page (open, save, move, sort) adds its
  entry to `web/e2e/interactions.ts`: its budget, whether it shows before
  the server answers, how many requests it may make ([ADR
  0023](docs/adr/0023-v3-local-first-traced.md)).
- A slow request is asked of its trace before it is read in the code:
  `make obs-up`, serve with `OTEL_EXPORTER_OTLP_ENDPOINT`, and query Tempo
  ([Traces](docs/guide/performance.md#traces)).
- Read [the design](docs/rewrite/design.md) before starting: its section 5
  is the status of each milestone, what was built and what was left.
  `docs/implementation-plan.md` is v1's log, kept as history.
- A change to what git-board prints on the golden boards is a change of
  the contract: `make golden`, its diff reviewed and explained in the
  commit ([compat.md](docs/rewrite/compat.md#running-it)).
- Tag a release (`git tag vX.Y.Z && git push origin vX.Y.Z`) after every
  meaningful batch of work lands on `main` — `go install ...@latest`
  resolves to the newest semver *tag*, not the newest commit, so an
  untagged `main` silently ships nothing new to installs.
- No new dependency for what an already-installed one, the stdlib, or a
  few lines already covers — see the "why" notes throughout
  `docs/implementation-plan.md` for examples (stdlib `html/template` over
  a template engine, `bubbles/list` over a third-party dropdown, shelling
  to `gh`/`git-board` instead of importing their whole dependency graph).
