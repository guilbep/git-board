# M5: what git-board keeps of gitbugio

2026-10-24, before M5's first phase. `internal/gitbugio` wraps git-bug for
the rest of git-board. Each group of its exports is kept over the core,
dropped, or stays git-board's, judged by what git-board's commands and
tests use, as [core.md](core.md)'s M5 row says. Forge sync itself (bridges,
credentials, `SyncForges`, `GetForges`, `SendForges`, `SyncIssue`) is M5's
second phase: in the first, it answers "not wired yet".

| Group | Decision | Why, and where it goes |
|---|---|---|
| Comment metadata | **keep**, over the core | the cycle's sign-offs, approvals and runs (`cycle:*`, `ask`, `correction`) and review comments are comments with metadata. The core's `comment` gets a `meta` object, and one operation, `set-meta`, changes a text's metadata later |
| Review comments | **keep**, in git-board only | line comments, replies and resolving them stay (`CommentWith`, `CommentReview`, `SetCommentMetadata`). Mirroring them to GitHub pull requests is out of version 1: `StampPR`, `review:pr`, the pull requests' comments pulled in, and the sync's skip of review comments go |
| GitHub Project bases | **keep**, over `set-meta` | `git-board bridge sync` (GitHub Projects, where the PMs' fields live) compares with what both sides last agreed on. The bases are the create operation's metadata, under the Project's prefix |
| Releases | **keep**: a `release` kind in the core, its operations git-board's | `internal/release` and the Releases page live on the records. The core stores, fetches, pushes, merges and watches `release` entities; git-board folds their four operations, as it gives labels their meaning |
| Numbers | **stays git-board's** | `refs/board/seq`, read and written with `gitstore` and the git binary. The remote's copy goes to `refs/board-remotes/<remote>/seq`, out of `refs/remotes/`. Numbers are keyed by the core's issue ids (M6 keys them again) |
| Threads, events, history | **keep**, built by git-board from the issue's log | `Thread`, `Event`, `Version` and `History` (Insights) read `oplog.Load`. Where an issue is on forges comes from the sync entities' links, which the core now lists (`syncer.Links`). `Pending`, `EditPending` and `EditWas` need the sync's records: empty until phase 2. `Forge.Gone` and `Forge.Bridge` go with unlink and the bridges |
| Labels in use, mentions | **keep** | every label in use is a new index query, `Labels`. `Mentioning` narrows with the index's text search and confirms with `Mentions`, which stays git-board's |
| Git history helpers | **stay git-board's**, off git-bug | `Branches`, `Log`, `Diff*`, `Worktrees`, `Remotes` and the rest read the code's own history with go-git or the git binary, never git-bug's repository |
| `ErrChangedMeanwhile` | **stays git-board's** | `EditComment` compares the text it started from with the loaded state before it writes. The core has no base check |
| Id prefixes | **keep**: the index answers them | `index.Resolve` turns a prefix of an issue's or a person's id into the id, or says it's ambiguous |

Dropped with their commands and tests, as "Out, until asked":

- `LinkIssue`, `UnlinkIssue`, `TakeIssue`, and the server's link, unlink and
  take endpoints;
- `SetAssignees` and the assignees mode switch;
- Gitea: its bridge name, its forge name and its logins;
- `StampPR` and the pull requests' numbers handed to the sync (`pullsOf`).

Gone with git-bug: `NewForRepo`, `TestCache` (tests build on the core
through `gitbugiotest`), `Storage` as git-bug's folder (git-board's caches
move to `<common dir>/board/git-board/`).

Carried to phase 2: the push must leave out comments with `review:*`
metadata, so that line comments never reach a forge issue.

## Phase 1, done

`internal/gitbugio` keeps its exported API over the core, and its callers
barely changed. What phase 1 left:

- **Forge sync answers `ErrNotWired`:** `SyncForges`, `GetForges`,
  `SendForges`, `SyncIssue`, the bridges and the credentials
  (`gitbugio/forges.go`). Their tests skip with "M5 phase 2". These stubs,
  and the server's forge sync (`server.go`, `forges.go`) with its tests,
  are the only code still importing git-bug, for its sync result types.
- **The numbers page and pull requests** find the GitHub repository and
  the token through the bridges, so they wait for phase 2 too.
- **Existing repositories show no issue** until M6 pulls them from their
  forges: git-board reads `refs/board/`, and the fork's data stays in
  `refs/bugs/`.
- **A cold listing loads every issue** once per process; the server and
  the TUI keep them, by head, in memory.
- **The web app's link, unlink and take buttons** never show, as no forge
  names a bridge any more; their code waits for the end of the project.

## Phase 2, done

- **Forge sync runs on the core:** `SyncForges`, `GetForges`, `SendForges`
  and `SyncIssue` run `syncer.Sync`, `Pull`, `Push` and `Options.Issue` on
  the forges of `[forges.<name>]`, opened for each run. The check is the
  same run on a mirror of the repository, its forge writes counted and
  never made.
- **Bridges are forges of `.git-board.toml`:** Settings lists, adds,
  switches and removes them; a pasted token goes to `git credential
  approve`. Credentials list the forges whose posting account has a token,
  never the token.
- **Tokens, the numbers page and pull requests** take the adapters' token
  rule (`github.Token`, `gitlab.Token`), with the GitHub forge's
  repository.
- **Review comments never reach a forge** (`Options.Local`), and never
  show as waiting for one.
- **Gone with git-bug:** the check's "conflicts" guard on Send (a push
  alone writes a field only where git-board's change is the later), its
  "stays local" count (the posting account posts for everyone), and
  `TestForgeSendRefusesIssuesChangedBothWays` with them.
