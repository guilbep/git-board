
## Worktrees

Every branch has its own git worktree, tmux session and agent. The session is
`git-board/<branch>` and its panes are `agent`, `shell`.

- There are no dev server panes. The board (`refs/board/*`, and its index and
  sync lock in `.git/board/`) is shared by every checkout of this repository:
  a `git board serve` started from a worktree serves the same board as the main
  checkout's. Tests are unaffected, they use throwaway repos.
- Work on this branch only. Never push `main`. When the work is ready, say
  so and a human runs `wtx land <branch>` from the main checkout.
- A pre-push hook enforces this. If it refuses a push, that is the design, not a
  bug to work around.

### Directories outside this repo

- `co2-calculator` at `../co2-calculator`, access `read`.
- `co2-calculator-back-office-doc` at `../co2-calculator-back-office-doc`, access `read`.
- `epfl-design-skill` at `../epfl-design-skill`, access `read`.
- `git-bug` at `../git-bug`, access `read`.
- `org` at `../org`, access `read`.
- `wtx` at `../wtx`, access `pair`.

A `read` directory is yours to read as much as you like, with no prompt, and you
may never write in it. To change one, ask for a paired worktree instead: a human
runs `wtx go <branch> --with <name>=<branch>`, and the change lands through that
repo's own pull request.
