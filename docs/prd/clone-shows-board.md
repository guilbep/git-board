# A clone shows its board

Status: PRD 2026-11-03, issue p0; built the same day: acceptance 1 to 4 (2: a page
opening fetches origin once in five minutes, Sync's badge says "N to get"); 5
waits for visibility-matrix's test repository. First of four (then
board-template, visibility-matrix, datastores).

## Why

The board lives in git, under `refs/board/*`. Pushing works; but a teammate
who clones the repository opens git-board on an **empty board**, with no word
that the remote holds issues. They conclude nothing was pushed.

Reproduced 2026-11-03 (a throwaway repository, `/tmp/git-board` built from
main):

```text
repo a:  create an issue · git board push   → "pushed to origin"
origin:  refs/board/issue/f6e7f870…, refs/board/person/31816c25…, refs/heads/main
git clone origin b
repo b:  refs: refs/remotes/origin/main only
         git board ls   → (nothing)
         git board pull → "pulled from origin" · git board ls → f6e7f870 seen after a clone?
```

`git clone` fetches branches and tags, never other refs, so the board stays
behind until a `git board pull` nobody told the user to run. The same holds
for every later teammate, and for the "no forge at all, only a git remote"
setup this project promises.

## Decisions

| Question | Decision |
|---|---|
| A clone whose board is empty, origin's isn't | git-board **pulls it on first open**, web, TUI and CLI alike, and says so ("Pulled 42 issues from origin"). Nothing local can be lost: there is nothing local |
| A board with local work, behind origin | never a silent pull: Sync's badge shows what a fetch found ("N to get"), Pull is a click. A fetch on open (once per 5 minutes, as the inbox does) is the next step, not in the first cut |
| `git clone` itself | can't be changed, and needs no fetch rule: git-board fetches the board itself, into `refs/board-remotes/<remote>/`, and merges it on pull (`store.Fetch`, `Pull`) |
| No forge configured, only origin | the supported minimum: every acceptance test below runs with no forge |
| The empty board's page | when origin can't be read, a notice under the header says why ("origin can't be read, your issues may be there"), not just "No issues" |
| The first look at origin | `git ls-remote origin refs/board/issue/*`, no prompt (BatchMode, no askpass), 20 s at most: an SSH host to confirm or a dead remote is said, never waited on with the server's writes held |

## Acceptance

1. Clone a repository whose origin holds a board, open git-board (web): the
   issues show, and a notice says they came from origin. Same for
   `git board ls` and `git board tui`.
2. A clone with one local unpushed issue, origin ahead by two: opening shows
   "2 to receive", and nothing changes until Pull; after Pull, three issues.
3. Origin unreachable on first open: the page says so; nothing is created.
4. No `[forges]` and no forge remote: 1 to 3 hold.
5. A test repository (the visibility-matrix issue's) replays 1 and 2 against
   GitHub and GitLab remotes.

## Out of scope

Choosing another datastore (`datastores.md`); the forges' own sync.
