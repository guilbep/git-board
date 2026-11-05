# git-board

Setup, workflow and conventions are in CONTRIBUTING.md. Run `make ci` (what CI
runs) once, when the work is done and verified, before saying it's ready: not
before every commit. Each commit runs lefthook's pre-commit (gofmt, go vet, the
web typecheck); a failing hook is a fix to make, not one to skip.

## Tests

- While developing, and for each commit on the way, run the package's own
  tests: `go test ./surface/http/ -run TestName`. The full `make ci` takes
  minutes: run it once, at the end.
- A new test calls `t.Parallel()` first, unless it chdirs, sets an
  environment variable, or swaps a package-level variable: then it stays
  serial.

## Worktrees

Every branch has its own git worktree, tmux session and agent. The session is
`git-board/<branch>` and its panes are `agent`, `shell`.

- Work on this branch only. Never push `main`, never tag: a semver tag is the
  release. When the work is ready, say so and a human runs `wtx land <branch>`
  from the main checkout.
- A pre-push hook enforces this. If it refuses a push, that is the design, not a
  bug to work around.
- There are no dev server panes. The board (`refs/board/*`, and its index and
  sync lock in `.git/board/`) is shared by every checkout of this repository:
  a `git board serve` started from a worktree serves the same board as the main
  checkout's. Tests are unaffected, they use throwaway repos.

### Directories outside this repo

- `co2-calculator` at `../co2-calculator`, access `read`: a real board, with
  its own `.git-board.toml`.
- `co2-calculator-back-office-doc` at `../co2-calculator-back-office-doc`,
  access `read`.

A `read` directory is yours to read as much as you like, with no prompt, and you
may never write in it. To change one, ask for a paired worktree instead: a human
runs `wtx go <branch> --with <name>=<branch>`, and the change lands through that
repo's own pull request.
