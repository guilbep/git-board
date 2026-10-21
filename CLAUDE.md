# git-board

Setup, workflow and conventions are in CONTRIBUTING.md. Run `make ci` and, in
`web/`, `npm run typecheck && npm test` (what CI runs) once, when the work is
done and verified, before saying it's ready: not before every commit.

## Tests

- While developing, and for each commit on the way, run the package's own
  tests: `go test ./internal/server/ -run TestName`. The full `make ci` takes
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
- There are no dev server panes. git-bug locks its cache in the main checkout's
  `.git/git-bug/lock`, even from a worktree, so only one process across all
  checkouts can hold this repo's board: a `git board` (or `git board serve`)
  run from a worktree fails with "already locked by the process pid N" while
  another checkout's server or TUI is running, and names that board's URL. Tests are unaffected, they use
  throwaway repos.
- `refs/bugs` (this repo's issues) is shared by every checkout. `git bug push`
  and `git bug bridge` are denied in a worktree.

### Directories outside this repo

- `git-bug` at `../git-bug`, access `read`: the fork go.mod's replace pins.
  Fork changes go in a worktree of the fork off `bridge-sync`, not through
  `--with`: wtx pairs from `origin`, and there `origin` is upstream.
- `co2-calculator` at `../co2-calculator`, access `read`: a real board, with
  its own `.git-board.toml`.
- `co2-calculator-back-office-doc` at `../co2-calculator-back-office-doc`,
  access `read`.

A `read` directory is yours to read as much as you like, with no prompt, and you
may never write in it. To change one, ask for a paired worktree instead: a human
runs `wtx go <branch> --with <name>=<branch>`, and the change lands through that
repo's own pull request.
