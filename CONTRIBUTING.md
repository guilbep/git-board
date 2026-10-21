# Contributing

## Setup

- Go 1.26+ (`GOTOOLCHAIN=auto` will fetch a newer one automatically if
  `go.mod` needs it).
- git-bug comes from its `bridge-sync` fork, through the `replace` in
  `go.mod`: bump it with
  `go mod edit -replace=github.com/git-bug/git-bug=github.com/guilbep/git-bug@<commit> && go mod tidy`.
- This is a private module: `go install`/`go get` need
  ```
  go env -w GOPRIVATE=github.com/guilbep/*
  git config --global url."git@github.com:".insteadOf "https://github.com/"
  ```
  Without `GOPRIVATE`, `go install` tries the public module proxy/checksum
  DB first, 404s (it's private), and fails — even with the SSH rewrite
  above already in place.
- `gh` CLI, authenticated, for `bridge pull` and `lint --create-labels`
  (`read:project` scope at minimum; `repo` scope for `--create-labels
  --apply`).

## Workflow

```bash
make build   # go build ./...
make test    # go test ./...
make vet     # go vet ./...
make fmt     # gofmt -l . (lists files needing formatting; run gofmt -w to fix)
make ci      # everything above, the full check
make install # go install the git-board binary
```

Run `make ci` once a change is done, before it's pushed or landed: it's the
sequence CI runs. While working, the package's own tests are enough
(`go test ./internal/server/ -run TestName`).

The `web/` frontend (Quasar/Vue) has its own toolchain: `make web-dev` /
`make web-build`, or `cd web && npm run dev`/`build`/`typecheck` directly.

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
- **Each account's token on its own git-bug identity** in the scratch
  clone, typed at the prompt, never on the command line:
  `git bug bridge auth add-token -t github -l <login> -u <identity id>`, run
  while no server holds the repository (git-bug locks it). git-board
  picks the token whose login matches the current identity's
  `github-login`, so switching identity in git-board switches accounts.
- Tokens are classic, `repo` scope only: a GraphQL field that needs
  `read:org` fails the whole query.

Write a full sync from a scratch clone with care: git-board-test holds
issues and labels from earlier tests, and a dry run shows what it would
send. The per-issue sync touches only that issue.

## Conventions

- Real fixtures over mocks: tests spin up a real on-disk git-bug repo
  (`newTestRepo`/`gitbugiotest`) or a fake CLI script on `PATH` (see
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
