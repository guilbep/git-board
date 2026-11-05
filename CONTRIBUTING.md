# Contributing

## v1 is frozen

git-board v2 is being built in `next/` ([ADR
0021](docs/adr/0021-git-board-v2-same-data-new-code.md)). v1 (`core/`,
`internal/`, `cmd/git-board`) takes fixes only. A fix to v1 is also made
in `next/`, unless `make compat` shows v2 doesn't need it. New features go
to `next/`.

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
- `gh` CLI, authenticated, for `git board project` (`read:project` scope at
  minimum). The labels (`lint --create-labels`, Settings › Labels) go
  through GitHub's API with the forge sync's token instead.

## Workflow

```bash
make         # make help: every target, in a line each
make build   # go build ./...
make test    # go test ./...
make vet     # go vet ./...
make fmt     # gofmt -l . (lists files needing formatting; run gofmt -w to fix)
make web-check # the web app's typecheck and tests
make web-e2e # the browser tests (web/e2e, Playwright) on this checkout's binary and a throwaway repo; first run: cd web && npx playwright install chromium
make web-lint  # the EPFL adherence linter on web/src, reporting only (EPFL_SKILL=../epfl-design-skill); its count is in docs/prd/issue-head-m2-plan.md
make next-web-check # v2's web app (next/web): typecheck and tests
make next-web-e2e # v2's browser tests (next/web/e2e) on git-board-next serve and throwaway boards
make ci      # the web checks and builds (web, next/web), then everything above: what CI runs
make install # the web app built, then go install the git-board binary
make install-latest # the latest release's binary instead, web app inside
make docs    # the docs site (guide, spec, decisions), in site/: a broken link fails it
make docs-serve # the docs site on localhost, rebuilt on save
```

Run `make ci` once a change is done, before it's pushed or landed: CI runs
`make ci`, and `make web-e2e` in a job beside it. It installs the web app's dependencies
(`npm ci`) only when `web/package-lock.json` is newer than `web/node_modules`.
While working, the package's own tests are enough
(`go test ./internal/server/ -run TestName`).

A page's regression gets an e2e test in `web/e2e/` (we favor them over unit
tests for what the browser shows). `web/e2e/serve.mjs` builds the web app and
the binary, makes a repository with a `.git-board.toml` and a GitHub origin, and
serves it with a fake GitHub on the port after `E2E_PORT` (18321): nothing
leaves the machine; `E2E_TOML=<path>` serves another board's file instead, for a
one-off smoke run of `e2e/tabs.spec.ts`. `make web-e2e` is not part of `make ci`, which stays fast:
CI runs it in its own job (`e2e` in `.github/workflows/ci.yml`), Playwright's
Chromium in a cache.

The `web/` frontend (Quasar/Vue) has its own toolchain: `make web-dev` /
`make web-build`, or `cd web && npm run dev`/`build`/`typecheck` directly.

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

- Real fixtures over mocks: tests spin up a real on-disk git repository
  (`newTestRepo`/`storetest`) or a fake CLI script on `PATH` (see
  `bridge.go`'s `fetchGithubItems` seam, `lint.go`'s
  `existingGithubLabels`/`createGithubLabel` seams), not hand-rolled mocks.
- Every bug fix ships with a regression test that fails without the fix.
- Read `docs/implementation-plan.md` before starting — it's the living
  status log for what's shipped, what's deliberately deferred, and why.
  Add a dated "Status update" section for anything non-trivial you land.
- Tag a release (`git tag vX.Y.Z && git push origin vX.Y.Z`) after every
  meaningful batch of work lands on `main` — `go install ...@latest`
  resolves to the newest semver *tag*, not the newest commit, so an
  untagged `main` silently ships nothing new to installs.
- No new dependency for what an already-installed one, the stdlib, or a
  few lines already covers — see the "why" notes throughout
  `docs/implementation-plan.md` for examples (stdlib `html/template` over
  a template engine, `bubbles/list` over a third-party dropdown, shelling
  to `gh`/`git-board` instead of importing their whole dependency graph).
