# One verb per side M2: starting over, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A board can start over:
- `git board reset --hard` empties this clone's board;
- `git board reset --hard <remote>` makes it the remote's;
- `git board push --prune` makes the remote's board this clone's.

A mark, `refs/board/reset`, keeps a clone left behind from merging the old
board into the new one.

**Architecture:** Three store methods, each named by its command:
- `Backend.ResetHard(remote)`;
- `Backend.PushPrune(remote)`;
- the guard, `Backend.ResetGuard(remote)`, which `Pull`, `Push` and
  `pushKinds` call, so that every path that merges or sends the board is
  guarded.

The reset's mark follows the switch's mark: a commit of nothing, fetched
to `refs/board-remotes/<remote>/reset`. Every deletion writes a bundle in
the git directory first. The commands stay thin; the command-line resets
refuse while a server runs (`server.Running`) on any worktree's git
directory.

**Tech Stack:** Go, cobra, git (`fetch --prune`, `push --prune --force`,
`bundle`).

**Spec:** `docs/prd/sync-sides.md`: "Decisions" (`reset --hard`,
`push --prune`, A clone left behind, What a reset keeps), "Starting
over", and the M2 row.

## Global Constraints

- **A reset keeps** `refs/board/switched` and `refs/board/seq`. It
  deletes every other ref under `refs/board/`, and all of
  `refs/board-remotes/`.
- **Every command that deletes refs** writes a bundle in the git common
  directory first, `board-backup-<YYYYMMDD-HHMMSS>.bundle`, and prints its
  absolute path with the `git fetch` that restores it.
- **The mark** is `refs/board/reset`; its fetched copy is
  `refs/board-remotes/<remote>/reset`.
- **The guard's two messages**, verbatim, with `<date>` as `2006-01-02 15:04`:
  - `<remote>'s board was started over on <date>: git board reset --hard <remote> takes it`
  - `this clone's board was started over on <date>, <remote>'s wasn't: git board push --prune makes <remote> match, git board reset --hard <remote> goes back to it`
- New tests follow M1's rule: `t.Parallel()` unless they chdir or swap a
  package variable.
- Commits are `feat(…)` or `docs: …`, with no `Co-Authored-By` trailer.

## Review Focus

1. **A `reset --hard origin` from a remote with no board at all** (pruned
   empty) leaves this clone's board empty, and doesn't fail. Test: Task 2,
   `TestResetHardTakesAnEmptyRemote`.
2. **A `push --prune` whose remote moved since this clone's reset** (a
   teammate numbered an issue: origin's `seq` is ahead) still makes
   origin match. That's the forced push, after the bundle. Test: Task 3,
   `TestPushPruneOverADivergedRef`.
3. **A brand-new clone** (no board, no mark) pulling a started-over
   origin takes it without a refusal. Test: Task 1,
   `TestResetGuardLetsANewCloneIn`.
4. **`board.me` after `reset --hard origin`** stays when origin still has
   that person, and goes when it doesn't. Test: Task 2,
   `TestResetHardKeepsMeWhenTheRemoteHasIt`.
5. **A command-line reset while `git board serve` runs** on any worktree
   refuses, naming the server's URL. Test: Task 2,
   `TestResetRefusesWhileServed`.

## Rulings this plan makes on the spec

- **`push --prune` is forced.** The spec says its push "isn't forced".
  But a reset clone whose origin moved meanwhile (a teammate numbered an
  issue, so origin's `seq` is ahead) could then neither pull, which the
  guard refuses, nor push. `push --prune` means "make origin match this
  clone", so it backs up origin's whole board to a bundle, then pushes
  `+refs/board/*:refs/board/*` with `--prune`. Task 4 corrects the PRD's
  text.
- **The bundle of `push --prune`** holds origin's board under
  `refs/board-pruned/*`, the names it was fetched to. Its restore command
  is `git fetch <bundle> 'refs/board-pruned/*:refs/board/*'`, then
  `git board push --prune`.

---

### Task 1: The reset's mark and the guard

**Files:**
- Create: `internal/store/reset.go`, holding `resetRef`, `resetCopy`,
  `ResetGuard` and `fetchResetMark`.
- Modify: `internal/store/store.go`:
  - `Fetch` also fetches the remote's mark to its copy;
  - `Pull` calls `ResetGuard` after the fetch and before `MergeFetched`;
  - `Push` calls `ResetGuard` first, after `fetchResetMark`.
- Modify: `internal/store/releases.go`: `pushKinds` calls
  `fetchResetMark`, then `ResetGuard`, before pushing.
- Modify: `cmd/git-board/remote.go`: `fetch` returns `ResetGuard`'s
  error after its fetch, before printing the counts.
- Test: `internal/store/reset_test.go`.

**Interfaces:**
- Produces:
  - `const resetRef = "refs/board/reset"`
  - `func resetCopy(remote string) string`, which is
    `"refs/board-remotes/" + remote + "/reset"`.
  - `func (b *Backend) fetchResetMark(remote string) error`: fetches
    `+refs/board/reset:<resetCopy>`. A remote without the ref deletes the
    copy, and isn't an error ("couldn't find remote ref", as `fetchMark`
    handles it).
  - `func (b *Backend) ResetGuard(remote string) error`. With `ours` the
    local mark and `theirs` the copy, both possibly zero:
    - equal: nil;
    - `ours` zero, and this clone holds no ref under `refs/board/issue/`
      or `refs/board/person/`: take theirs (`UpdateRef(resetRef, theirs, zero)`),
      nil;
    - `theirs` non-zero, and `ours` zero or older (by commit time):
      `ErrStartedOver` wrapped with the first message;
    - otherwise: `ErrStartedOver` wrapped with the second message, the
      date being `ours`'.
  - `var ErrStartedOver = errors.New("a board started over")`, matched by
    `type startedOver struct{ msg string }`, with
    `Error() string { return s.msg }` and
    `Is(target error) bool { return target == ErrStartedOver }`. The
    text then reads exactly as the Global Constraints say.
  - `func writeResetMark(b *Backend) (gitstore.Hash, error)`: a commit of
    nothing, as `markSwitched` writes, set at `resetRef` whatever it held.
    Task 2 uses it.

- [ ] **Step 1: Write the failing tests** in `reset_test.go`. Each starts
  with `t.Parallel()` and uses `storetest.NewPair` (mine, theirs, origin
  shared).
  - `TestResetGuardRefusesACloneLeftBehind`:
    1. Both create an issue; both `PushStep("origin")` and `Pull`.
    2. Mine writes a mark (`store.WriteResetMark`, exported for the tests
       in `export_test.go`), then pushes the mark alone
       (`git push origin refs/board/reset`).
    3. Theirs: `Pull("origin")` errors with `errors.Is(err, store.ErrStartedOver)`,
       and its text starts with `origin's board was started over on`.
       `Push("origin")` errors the same way.
    4. Theirs' issue list is unchanged by the refused pull.
  - `TestResetGuardTellsTheOneThatStartedOver`: mine writes a mark and
    doesn't push it. `mine.Pull("origin")` errors with text starting
    `this clone's board was started over on`, and containing
    `git board push --prune makes origin match`.
  - `TestResetGuardLetsANewCloneIn`:
    1. Mine writes a mark and pushes it with its issue.
    2. A third clone of origin, empty (`storetest.Repo` plus
       `remote add` plus `storetest.Open`, with no `Init`, so no person),
       pulls: no error. Its `refs/board/reset` equals mine's, and it
       lists mine's issue.
  - `TestResetGuardIsQuietWithoutMarks`: no mark anywhere. Pull and push
    work, as they did before M2.
- [ ] **Step 2: Run them, and see them fail.**
  `go test ./internal/store/ -run TestResetGuard`. Expected: a build
  failure, since `WriteResetMark` and `ErrStartedOver` are undefined.
- [ ] **Step 3: Implement** `reset.go`, the three calls in `store.go` and
  `releases.go`, the `fetch` command's guard, and `WriteResetMark` in
  `export_test.go`.
- [ ] **Step 4: Run Step 2's command, then
  `go test ./internal/store/ ./cmd/git-board/ -run 'Pull|Push|Sync|Fetch|Release'`.**
  Expected: all pass.
- [ ] **Step 5: Commit**: `feat(store): the reset's mark, and a pull or push between started-over boards refuses`.

### Task 2: `git board reset --hard [<remote>]`

**Files:**
- Modify: `internal/store/reset.go`, to add `ResetHard`, its report and
  the bundle helper.
- Create: `cmd/git-board/reset.go`, holding `newResetCmd` and
  `refuseWhileServed`.
- Modify: `cmd/git-board/cmd.go`, to register it.
- Test: `internal/store/reset_test.go` and `cmd/git-board/reset_test.go`.

**Interfaces:**
- Produces:
  - `type ResetReport struct { Deleted int; Backup string; Me bool }`.
    `Deleted` counts the refs deleted or moved; `Backup` is the bundle's
    absolute path, "" when there was nothing; `Me` says `board.me` was
    unset.
  - `func (b *Backend) ResetHard(remote string) (ResetReport, error)`:
    - **`remote == ""`:** bundle and delete every ref under `refs/board/`
      except `seq` and `switched`, and every ref under
      `refs/board-remotes/`. Then write a new mark (`writeResetMark`).
    - **`remote != ""`:** `checkRemote`, bundle every ref under
      `refs/board/` and `refs/board-remotes/`, then
      `git fetch --prune --no-write-fetch-head <remote> '+refs/board/*:refs/board/*'`,
      then `b.Fetch(remote)` to refresh the copies.
    - **Then, both:** unset `board.me` when the person it names has no
      ref left; `b.s.Reindex` is not needed (the index reads the heads).
  - `func (b *Backend) backup(refs []string) (string, error)`: writes the
    bundle `board-backup-<YYYYMMDD-HHMMSS>.bundle` in `b.s.CommonDir()`
    from `refs` given on stdin, as `Prune` writes its own. It returns ""
    for no ref. `Prune` keeps its own file name, `git-bug-backup-…`.
- **The command `git board reset --hard [<remote>]`:**
  - `--hard` is required: without it, the error is
    `only reset --hard exists: it empties this clone's board, or with a remote makes it the remote's`;
  - `refuseWhileServed` runs first;
  - it prints `reset: <n> refs deleted, backed up in <path>` and the line
    `  git fetch <path> 'refs/*:refs/*' brings them back`;
  - without a remote it adds:
    `this clone's board is empty. git board push --prune empties origin's; every other clone then runs git board reset --hard origin`;
  - when `Me`: `board.me is unset: its person is gone`.
- `func refuseWhileServed(b *store.Backend) error` looks at
  `server.Running` on `b.Store().CommonDir()` and on each
  `<common>/worktrees/*`. For a live one, it returns
  `git board serve runs on this repository (<url>): stop it first, or use Settings › Sync, Start over`.

- [ ] **Step 1: Write the failing store tests**, each with `t.Parallel()`.
  - `TestResetHardEmptiesTheBoard`:
    - **Setup:** `storetest.New`, two issues, the switch's mark (an
      `UpdateRef` of `refs/board/switched`), a counter (`WriteCounter`),
      and a fetched copy, `refs/board-remotes/origin/issue/<id>`.
    - `ResetHard("")`: `ListIssues` is empty;
      `refs/board/switched` and `refs/board/seq` are unchanged;
      `refs/board/reset` is set; nothing is left under
      `refs/board-remotes/`; `rep.Me` is true, and `people.Me` is "".
    - `git bundle list-heads rep.Backup` lists the two issues' refs.
  - `TestResetHardTakesTheRemote`:
    1. `NewPair`. Mine creates "kept" and pushes. Theirs pulls, then
       creates "local only", never pushed.
    2. `theirs.ResetHard("origin")`: theirs lists only "kept".
  - `TestResetHardTakesAnEmptyRemote`: `NewPair`. Mine never pushes;
    theirs creates an issue, then `ResetHard("origin")`. No error, and
    theirs lists nothing.
  - `TestResetHardKeepsMeWhenTheRemoteHasIt`:
    1. `NewPair`. Theirs pushes its person (any issue push carries it).
    2. `theirs.ResetHard("origin")`: `rep.Me` is false, and `board.me`
       is unchanged.
- [ ] **Step 2: Write the failing command test** `TestResetRefusesWhileServed`.
  It is serial: it chdirs.
  - **Setup:** write `<gitdir>/git-board/server.json` naming this test
    process's pid and an `httptest.Server` that answers
    `/api/repository` with `{"gitDir": <gitdir>}`. That's what
    `server.Running` checks.
  - `reset --hard` errors with `git board serve runs on this repository`.
  - **In the same file,** `TestResetNeedsHard`: `reset` errors with
    `only reset --hard exists`.
- [ ] **Step 3: Run them, and see them fail.**
  `go test ./internal/store/ -run TestResetHard && go test ./cmd/git-board/ -run 'TestReset'`.
  Expected: they fail.
- [ ] **Step 4: Implement** `ResetHard`, `backup`, `newResetCmd` and
  `refuseWhileServed`.
- [ ] **Step 5: Run Step 3's commands.** Expected: they pass.
- [ ] **Step 6: Commit**: `feat(cli): git board reset --hard, empty or the remote's, after a backup bundle`.

### Task 3: `git board push --prune`

**Files:**
- Modify: `internal/store/reset.go`, to add `PushPrune`.
- Modify: `cmd/git-board/remote.go`, to give `push` a `--prune` flag.
- Test: `internal/store/reset_test.go` and `cmd/git-board/remote_test.go`.

**Interfaces:**
- Produces: `func (b *Backend) PushPrune(remote string) (ResetReport, error)`.
  1. `checkRemote`.
  2. `git fetch --no-write-fetch-head <remote> '+refs/board/*:refs/board-pruned/*'`.
  3. Bundle every `refs/board-pruned/*` ref with `backup`, then delete
     them.
  4. `git push --prune --force <remote> 'refs/board/*:refs/board/*'`.
  5. `b.Fetch(remote)` to refresh the copies.

  `Deleted` counts the remote's refs that this clone doesn't have. There
  is no guard, since this is a way out of one. A remote with no board
  bundles nothing (`Backup` "").
- **The command:** `push --prune` calls it, then prints
  `pushed to <remote>, and deleted there <n> refs this clone doesn't have`,
  and with a bundle
  `  origin's board as it was: <path>; git fetch <path> 'refs/board-pruned/*:refs/board/*' brings it here`.
  `push` without `--prune` is unchanged.

- [ ] **Step 1: Write the failing tests.**
  - `TestPushPruneMakesTheRemoteMatch`, with `t.Parallel()`:
    1. `NewPair`. Mine and theirs each create and push an issue.
    2. `mine.ResetHard("")`, then `mine.PushPrune("origin")`: origin's
       `refs/board/issue/` is empty, and `refs/board/reset` equals
       mine's.
    3. `theirs.Pull("origin")` is `ErrStartedOver`.
    4. `theirs.ResetHard("origin")`, then `theirs.Pull("origin")`: no
       error, and theirs lists nothing.
    5. The bundle's heads number the issue refs origin had.
  - `TestPushPruneOverADivergedRef`, with `t.Parallel()`:
    1. `NewPair`, and mine writes a counter and pushes.
    2. Theirs pulls and moves origin's counter one commit on
       (`WriteCounter`, then `git push origin refs/board/seq`).
    3. `mine.ResetHard("")`, then `PushPrune("origin")`: no error, and
       origin's `refs/board/seq` equals mine's.
  - `TestPushPruneCommand`, serial: in a clone, after `reset --hard`,
    `push --prune` prints `deleted there`.
- [ ] **Step 2: Run them, and see them fail.**
  `go test ./internal/store/ -run TestPushPrune && go test ./cmd/git-board/ -run TestPushPruneCommand`.
  Expected: they fail.
- [ ] **Step 3: Implement** `PushPrune` and the flag.
- [ ] **Step 4: Run Step 2's commands, and `go test ./internal/store/ ./cmd/git-board/`.**
  Expected: all pass.
- [ ] **Step 5: Commit**: `feat(cli): git board push --prune makes the remote's board this clone's`.

### Task 4: The words

**Files:**
- Modify: `docs/guide/forges.md`. After "Leaving git-bug", add a section
  "Starting over" with:
  - the four steps of the spec's "Starting over everywhere";
  - the guard's two refusals;
  - what a reset keeps and loses;
  - the forge pull's limit, in one sentence: a forge pull doesn't yet
    bring back what git-board itself posted to a forge, and a repository
    where git-board never posted misses nothing;
  - that every clone needs this version or later first.
- Modify: `internal/server/configfile_test.go`. The forge docs' sections
  end `…,on,leaving,starting`.
- Modify: `docs/prd/sync-sides.md`. In "The git remote", `push --prune`
  is forced, after the bundle of origin's whole board, with the reason
  from this plan's rulings.
- Modify: `cmd/git-board/remote.go`, `push`'s `Long`: `--prune` in one
  paragraph.

- [ ] **Step 1: Make the edits.**
- [ ] **Step 2: Run `go test ./internal/server/ -run TestForgeDocsComeInSections`.**
  Expected: it passes.
- [ ] **Step 3: Commit**: `docs: starting over, in the guide and the PRD`.
