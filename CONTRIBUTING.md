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
make ci      # everything above, the full pre-commit check
make install # go install the git-board/git-board-server binaries
```

Run `make ci` before every commit — it's the same sequence used to vet
every change in this repo's history.

The `web/` frontend (Quasar/Vue) has its own toolchain: `make web-dev` /
`make web-build`, or `cd web && npm run dev`/`build`/`typecheck` directly.

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
