# A long sync you can follow, and cut: plan

**Spec:** docs/prd/sync-progress.md. One branch, `feat/sync-progress`, a
commit or more per task, `make ci` once at the end. Each task's tests are
written first and seen failing.

## Global constraints

- A new test calls `t.Parallel()` first, unless it sets an environment
  variable or swaps a package variable.
- `core/` imports nothing from `internal/`.
- Writes to a forge are never retried.
- No live call at an EPFL repository. The end-to-end check runs against
  a local fake GitHub: a throwaway binary built with
  `-ldflags -X github.com/guilbep/git-board/internal/store.GitHubAPI=http://127.0.0.1:PORT`.

## Review focus

1. A comment posted twice because the push planned against a partial
   issue (task 2).
2. An issue skipped on going on that changed and is never read again,
   once a complete run records `lastRun` (task 4).
3. Two runs at once, through the lock (task 1).
4. A read retried after the run's context ended (task 5).
5. The journal of a dry run or of a one-issue run touching the real one
   (task 4).

### Task 1: the OS lock

- `core/syncer/lock.go`: `lock(s)` opens `board/sync.lock`
  (`O_RDWR|O_CREATE`), takes the OS lock without waiting
  (`lock_unix.go`: `syscall.Flock(LOCK_EX|LOCK_NB)`; `lock_windows.go`:
  `windows.LockFileEx` with `LOCKFILE_EXCLUSIVE_LOCK|LOCKFILE_FAIL_IMMEDIATELY`).
  It returns the open file, and an unlock that closes it. The file is
  never removed. Held: `ErrLocked`, "another sync runs (process N)", N
  read from the file's first line.
- Tests (`core/syncer/lock_test.go`):
  - `TestALeftLockBlocksNothing`: a `sync.lock` holding `"12345\n"`
    and held by no one; a Sync runs.
  - `TestAHeldLockRefuses`: the test locks the file; a Sync gets
    `ErrLocked`, naming the process.
- Check: `GOOS=windows go vet ./core/syncer/`, and
  `GOOS=windows GOARCH=amd64 go build ./cmd/git-board`.
- Spec: docs/spec/sync.md, "A run", step 1.

### Task 2: the partial listing

- `core/forge/forge.go`:
  - `Issue.Partial bool`: listed without its comments (and, on GitLab,
    its change times).
  - `Forge.Complete(ctx, Issue) (Issue, error)`: reads what is missing,
    and gives back an issue that isn't partial as it is.
- **GitHub:** `Issues` keeps the pages and the times query, and reads no
  comments; an issue with comments is `Partial`. `Complete` reads its
  comments.
- **GitLab:** `Issues` is `g.issue()`, `Partial`, and `Complete` is
  `full`.
- **fake:** `Issues` leaves the comments out of an issue that has some,
  which is then `Partial`, so every syncer test runs on partial listings.
  `Complete` gives the stored issue.
- **`internal/store`'s `counted`:** passes `Complete` through. A `dry-`
  id comes back as it is.
- **The syncer:** `pullFrom` completes each issue before it pulls it,
  the one place.
- Tests:
  - the conformance suite gains "a listed issue, completed, is the issue
    read alone", run by both adapters;
  - `TestAPartialListingPostsNothingTwice` (syncer): a linked issue
    with forge comments; two syncs post nothing and pull each comment
    once;
  - the existing syncer tests, on the fake's partial listing.

### Task 3: progress

- `syncer.Options.Progress func(key, way string, done, total int)`:
  - the pull calls it per listed issue reached, `way` `"pull"`;
  - the push calls it per issue walked, `way` `"push"`.
- `store.SyncEvent` gains `Done`, `Total`. What `"progress"`:
  - the server's stepper sets `syncStep.Total`;
  - the CLI prints a line every 100 issues.
- `web/src/ways.ts`:
  - `timeLeft(samples, total)`: `""` before 20 done;
  - `longRun(step, left)`: true at 200 left or more, or over a minute.
  - Tests in `ways.test.ts`.
- `SyncProgress.vue` shows the time left and the warning.

### Task 4: going on

- `core/syncer/journal.go`:
  - `readJournal(f) map[[2]string]int64`, (forge key, forge id) to
    start;
  - the run's header line;
  - `carry` writes back what is kept;
  - `reached(key, id)` appends a line.
- In `pullFrom`: skip when journaled, linked and `Updated < start-300`.
  Count it done, and keep it out of `p.listed`.
- After `runOp` is written: truncate the file.
- No journal for `opt.Issue`.
- Tests:
  - `TestACutPullGoesOn`: a fake forge with 10 issues; a Pull whose
    context ends at issue 6; the next Sync calls `Complete` only for
    issues 7–10, and ends with all 10;
  - `TestGoingOnReadsWhatChanged`: an issue reached, then updated on the
    forge, is pulled again;
  - `TestGoingOnReadsWhatItDidntReach`: an issue linked before the cut
    run, changed after the last complete run, and not reached, is pulled;
  - `TestAWaitingMirrorIsntReached`;
  - `TestTwoCutsInARow`;
  - `TestOneIssueLeavesTheJournal`;
  - `TestACompleteRunEmptiesTheJournal`.

### Task 5: reads that wait

- `core/forge`:
  - `WithWaits(ctx, func(Wait)) context.Context` and `Waiting(ctx, Wait)`;
  - `Wait{Why string; For time.Duration}`.
- **GitHub's `call`:** a read retries a connection error or a 5xx with
  waits of 1, 2, 4… up to 30 s, until 5 minutes have passed (package
  variables, for tests). Before each wait it calls `Waiting`, and so does
  a rate limit's wait. It stops at once when the context ends.
- **GitLab's request:** the same.
- **store:** puts the hook in the run's context and turns each wait into
  a `"waiting"` SyncEvent, which the stepper and the CLI show.
- Tests:
  - `TestAReadWaitsOutAnOutage` (github): the server fails 5 times,
    then answers; the waits are heard;
  - `TestAReadGivesUp`;
  - `TestAWriteIsntRetried`;
  - the same three for GitLab.

### Task 6: after a restart

- `syncer.Cut(s) (*CutRun, error)`: the journal, when no one holds the
  lock: its start, and per forge how many it reached.
- **`GET /api/forges`:** gains `cut` when no job runs.
- **Settings › Sync:** says it; `web/src/ways.ts` `cutWords` gets a test.
- **The CLI:** prints it before a forge pull or sync.
- Tests: `TestCutSaysWhereARunStopped` (syncer), and a server test.

### Task 7: docs

- docs/spec/sync.md: "A run" (the lock, the journal, going on), and the
  forge interface's `Complete`.
- docs/guide/forges.md: a long first sync.
- README, if it names the lock.

### Final

- Review the whole branch, in a fresh context.
- Run the end-to-end check against the fake GitHub:
  - 1,000 issues, each comments request delayed;
  - a bar with an end, and the warning;
  - `kill -9` mid-pull, then a restart: no lock error, the cut notice,
    and the next sync skips what was pulled;
  - a refused connection for 20 s mid-run, then it goes on.
- Run `make ci`, then open the PR.
