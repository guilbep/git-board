# One verb per side M1: the command line, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Each side of a sync gets its own commands with git's verbs:
- the git remote: `git board fetch`, `pull` and `push`;
- the forges: `git board forge pull`, `push` and `sync`;
- the GitHub Project: `git board project pull`, `push`, `sync`, `resolve`
  and `configure`, with `bridge` kept as a deprecated alias.

`git board sync` doesn't change.

**Architecture:** The store already keeps the sides apart. Each new command
wires one side:
- **The git remote:** the whole sync's own steps, `pullStep` and
  `pushStep`, which this plan exports.
- **The forges:** `SyncForges`, `GetForges` and `SendForges`, through one
  helper, which `sync`'s forge step calls too.
- **The Project:** `bridge sync`'s plan, restricted to one side by two new
  `SyncPlan` methods, which drop the bases of what they leave out as
  `ApplySync` does for a skipped edit.

**Tech Stack:** Go, cobra. Tests use the package's fakes: `execute`,
`twoReposWithRemote`, `stubForges`, `fake.Forge`, `withFakeProject`,
`newBridgeTestRepo`.

**Spec:** `docs/prd/sync-sides.md`: "Decisions", "The command line" (The
git remote, The forges, The Project), and the M1 row.

## Global Constraints

- Every new verb acts by default and takes `--check`, which writes
  nothing, as `sync` does. `project` doesn't take `--apply`.
- A remote argument defaults to `origin`.
- `git board sync` and its flags are unchanged, `--git` included.
- `bridge` stays: hidden from the help, `Deprecated` set, the same
  subcommands and flags it has today.
- A new test calls `t.Parallel()` first, unless it chdirs, sets an
  environment variable or swaps a package-level variable (`openForges`,
  `projectWriter`, `fetchProjectMeta`…). Then it stays serial.
- Commits are `feat(cli): …` or `docs: …`, with no `Co-Authored-By`
  trailer.
- No live call reaches any EPFL repository; the tests use fakes only.

## Review Focus

1. **A remote the repository doesn't have**, for example
   `git board pull upstream`, says `no remote "upstream"` and writes
   nothing. Test: Task 1, `TestPullNamesAMissingRemote`.
2. **A push the remote refuses as non-fast-forward** says to pull first,
   not "sync again", since `push` is now a command of its own. Test:
   Task 1, `TestPushAfterOriginMoved`.
3. **`forge …` with no forge whose sync is on** fails with
   "no forge has its sync on". It never succeeds having done nothing.
   Test: Task 2, `TestForgeNeedsAForge`.
4. **`project …` without `[github-project]`** fails, naming the
   `[github-project]` table and `git board project configure`. Test:
   Task 3, `TestProjectNeedsAProject`.
5. **A `project pull` that leaves a local change unsent** doesn't record
   it as agreed. Otherwise the next `project push` would never send it.
   Test: Task 3, `TestPlanOneSide`, and the `project push` assertion in
   `TestProjectPullThenPush`.

---

### Task 1: The git remote: `fetch`, `pull`, `push`

**Files:**
- Modify: `internal/store/whole.go`:
  - rename `pullStep` to `PullStep` and `pushStep` to `PushStep`, and their
    two callers in `SyncWhole`;
  - `PushStep`'s non-fast-forward message.
- Create: `cmd/git-board/remote.go`, holding `newFetchCmd`, `newPullCmd`
  and `newPushCmd`.
- Modify: `cmd/git-board/cmd.go`, to register the three.
- Test: `cmd/git-board/remote_test.go`.

**Interfaces:**
- Produces:
  - `func (b *Backend) PullStep(remote string, dry bool) (string, error)`
  - `func (b *Backend) PushStep(remote string) (string, error)`

  Both are today's bodies; the string is a note, often empty.
- **The commands** each take `[<remote>]`, with `cobra.MaximumNArgs(1)`
  and `origin` by default, and work on the current directory
  (`store.OpenCwd`).
- **What they print:**
  - **fetch** prints
    `<remote>: issues <r> to receive, <s> to send; people <r> to receive, <s> to send`,
    read from the `store.Remote` named so in `b.Remotes()`, after
    `b.Fetch`.
  - **pull** prints `pulled from <remote>`, then the note on a line of
    its own when there is one.
  - **push** prints `pushed to <remote>`, then the note likewise.

- [ ] **Step 1: Write the failing tests** in `remote_test.go`. They run
  serially, because they chdir with `t.Chdir`.
  - `TestPushThenPull`:
    1. `twoReposWithRemote`, and `seedBug` A with `"from A"`.
    2. Chdir A, then `push`.
    3. Chdir B, then `fetch`. The output contains
       `origin: issues 1 to receive`, and `bugTitles(B)` is empty.
    4. `pull`: `bugTitles(B)` contains `"from A"`, and the output contains
       `pulled from origin`.
  - `TestPullNamesAMissingRemote`: in A, `pull upstream` errors with
    `no remote "upstream"`.
  - `TestPushAfterOriginMoved`:
    1. A creates an issue and pushes; B pulls.
    2. A comments on it (`storetest.CommentAs`) and pushes.
    3. B comments on it too, without pulling, and pushes.
    4. B's push errors with `origin has changes this repository doesn't have: git board pull first`.
  - `TestPullWaitsForTheSwitch`: set up as `TestSyncWaitsForTheSwitch`
    does. `pull` errors with `store.ErrNotSwitched`'s text.
- [ ] **Step 2: Run them, and see them fail.**
  `go test ./cmd/git-board/ -run 'TestPushThenPull|TestPullNamesAMissingRemote|TestPushAfterOriginMoved|TestPullWaitsForTheSwitch'`.
  Expected: they fail, since the commands don't exist.
- [ ] **Step 3: Export and implement.**
  - Export the two steps.
  - `PushStep`'s non-fast-forward error becomes
    `"%s has changes this repository doesn't have: git board pull first"`.
    It matches git's `fetch first` as well as `non-fast-forward`: when
    the remote holds commits this clone never fetched, git rejects the
    push with `(fetch first)`, and that is the case
    `TestPushAfterOriginMoved` builds.
  - Write the three commands, each with a `Long` that names its side and
    what it moves.
- [ ] **Step 4: Run Step 2's command, then `go test ./internal/store/ -run 'Whole|Sync'`.**
  Expected: all pass. If a sync test matched on "sync again", it now
  reads "git board pull first".
- [ ] **Step 5: Commit**: `feat(cli): git board fetch, pull and push, the git remote alone`.

### Task 2: The forges: `forge pull`, `push`, `sync`

**Files:**
- Modify: `cmd/git-board/sync.go`. Move the body of `wholeOne`'s
  `run.Forges` closure into `runForgeWay`, and make the closure call it.
- Create: `cmd/git-board/forge.go`, holding `newForgeCmd`.
- Modify: `cmd/git-board/cmd.go`, to register it.
- Test: `cmd/git-board/forge_test.go`.

**Interfaces:**
- Produces:
  - `type forgeWay func(*store.Backend, context.Context, store.ForgeRun) (syncer.Report, error)`,
    whose values are `(*store.Backend).SyncForges`, `GetForges` and
    `SendForges`.
  - `func runForgeWay(ctx context.Context, b *store.Backend, cfg *config.Config, on []config.Forge, way forgeWay, check, allowCreate bool, merge string, say func(format string, a ...any)) error`.
    It opens `on` with `openForges`, prints each event as `wholeOne` does
    today (`say("  %s: %s", …)`, with ` (--allow-create)` after a held
    line), and runs `way`.
- **`git board forge pull|push|sync`:**
  - flags `--check` and `--allow-create`, on the current directory;
  - the forges come from `runForges`, keeping those whose `Sync` is on;
  - with none, the error is `no forge has its sync on`;
  - `merge` is `""`;
  - the last line is `checked: nothing written` with `--check`, else
    `done`.

- [ ] **Step 1: Write the failing tests** in `forge_test.go`. They run
  serially, because they use `stubForges` and `t.Chdir`.
  - `TestForgePullTakesOnly`:
    - **Setup:** a repo with a bare origin (`storetest.Repo`, then
      `remote add`), `storetest.ForgeOrigin(t, dir, "https://github.com/o/r.git")`,
      and `storetest.GitHub(t, map[string]string{"o/r": "o/r"})`.
      `fake.New("github.com/o/r")` holds an issue "From GitHub", and the
      repo has a local issue "Local only".
    - `forge pull --check`: the output contains
      `github: would pull a new issue`. The titles are unchanged, and
      `f.Writes()` is empty.
    - `forge pull`: the titles contain "From GitHub" and `f.Writes()` is
      empty. `git -C origin for-each-ref refs/board` is empty, since no
      git step ran.
  - `TestForgePushSendsOnly`, with the same setup:
    - `forge push --allow-create`: `f.Writes()` holds the creation of
      "Local only", and the titles don't contain "From GitHub".
  - `TestForgeNeedsAForge`: a repo with no forge remote. `forge sync`
    errors with `no forge has its sync on`.
- [ ] **Step 2: Run them, and see them fail.**
  `go test ./cmd/git-board/ -run 'TestForge'`. Expected: they fail.
- [ ] **Step 3: Implement** `runForgeWay` and `newForgeCmd`. Build the
  three subcommands with one constructor, `newForgeWayCmd(use, short string, way forgeWay)`.
  Each `Long` says which way it goes, and that `forge push` is the web
  page's Send.
- [ ] **Step 4: Run Step 2's command, and `go test ./cmd/git-board/ -run 'TestSync'`.**
  Expected: all pass, so `sync`'s output is unchanged.
- [ ] **Step 5: Commit**: `feat(cli): git board forge pull, push and sync, the forges alone`.

### Task 3: The Project: `project pull|push|sync|resolve|configure`, and `bridge` deprecated

**Files:**
- Modify: `internal/ghbridge/sync.go`, to add the two `SyncPlan` methods.
- Test: `internal/ghbridge/sync_test.go`.
- Modify: `cmd/git-board/bridge_sync.go`. `bridgeSync` becomes
  `projectRun`, with `side`, and `sync.go`'s Project step calls it with
  `"both"`.
- Create: `cmd/git-board/project.go`, holding `newProjectCmd`.
- Modify: `cmd/git-board/bridge.go`:
  - set `Hidden: true` on `bridge`;
  - set `Deprecated: "use git board project: pull, push, sync, resolve, configure"`
    on `bridge` and on each of its subcommands, since cobra prints only
    the executed command's notice.
- Modify: `cmd/git-board/cmd.go`, to register `project`.
- Test: `cmd/git-board/project_test.go`.

**Interfaces:**
- Produces:
  - `func (p SyncPlan) LabelsOnly() SyncPlan`:
    - `Edits` is nil;
    - each dropped edit's `Field` is removed from `Bases[edit.BugID]`;
    - `Changes`, `Conflicts` and `Unmatched` stay as they are.
  - `func (p SyncPlan) ProjectOnly() SyncPlan`:
    - each change keeps `BugID`, `GithubURL`, `Number`, `Title` and
      `Warnings`, with `Fields`, `Added` and `Removed` nil;
    - each dropped `FieldResult.Field` is removed from `Bases[BugID]`;
    - `Edits`, `Conflicts` and `Unmatched` stay.

  Neither changes `p`: the bases maps are copied.
  - `func projectRun(out io.Writer, cfg *config.Config, b *store.Backend, limit int, side string, apply bool) error`,
    where `side` is `"both"`, `"labels"` or `"project"`. It is today's
    `bridgeSync` with the plan restricted before printing and applying.
- **`git board project pull`, `push` and `sync`** call `projectRun` with
  `"labels"`, `"project"` and `"both"`. They take `--check`, and `--limit`
  as `bridge sync` does, and apply unless `--check` is given.
- **`resolve` and `configure`** are built again under `project` from
  `newBridgeResolveCmd` and `newBridgeConfigureCmd`. Their `Example`
  strings say `git board project …`; the `bridge` copies keep theirs.
- **Without `[github-project]`**, the three error with
  `no [github-project] in .git-board.toml: git board project configure --owner <org> --project <n>`.

- [ ] **Step 1: Write the failing unit test** `TestPlanOneSide` in
  `internal/ghbridge/sync_test.go`. It starts with `t.Parallel()`.
  - **The plan:**
    - `Changes`: `[{BugID: "b", Fields: [{Field: "status", Added: ["status:qa"]}], Added: ["status:qa"]}]`;
    - `Edits`: `[{BugID: "b", Field: "prio", Value: "high"}]`;
    - `Bases`: `{"b": {"status": "qa", "prio": "high", "est": "3"}}`.
  - **`LabelsOnly()`:** `Edits` is empty, `Bases["b"]` is
    `{"status": "qa", "est": "3"}`, and `Changes[0].Added` is
    `["status:qa"]`.
  - **`ProjectOnly()`:** `Changes[0].Added` and `Changes[0].Fields` are
    empty, the one edit is there, and `Bases["b"]` is
    `{"prio": "high", "est": "3"}`.
  - **The original plan is unchanged:** its `Bases["b"]` still has three
    keys.
- [ ] **Step 2: Write the failing command tests** in `project_test.go`.
  They run serially, because they use `newBridgeTestRepo` and
  `withFakeProject`.
  - `TestProjectPullThenPush`, with `rec := withFakeProject(t, true)`:
    - `project pull --check`: no labels.
    - `project pull`: the labels contain `status:qa-validation`, and
      `rec.edits` is empty.
    - `set <id> status=in-review`, then `project pull`: the labels contain
      `status:in-review`, and `rec.edits` is empty.
    - `project push`: one edit, with `Arg == "o-review"`, and the labels
      still contain `status:in-review`.
  - `TestBridgeStillWorks`: `bridge sync --apply` succeeds, its output
    contains `deprecated`, and the labels contain `status:qa-validation`.
  - `TestProjectNeedsAProject`: `newBridgeTestRepo` without
    `withFakeProject`. `project pull` errors with `no [github-project]`.
- [ ] **Step 3: Run them, and see them fail.**
  `go test ./internal/ghbridge/ -run TestPlanOneSide && go test ./cmd/git-board/ -run 'TestProject|TestBridgeStillWorks'`.
  Expected: they fail.
- [ ] **Step 4: Implement** the two methods, `projectRun`,
  `newProjectCmd`, and the `bridge` deprecation.
- [ ] **Step 5: Run Step 3's command, and `go test ./cmd/git-board/ -run 'TestBridge|TestSync'`.**
  Expected: all pass, so the old `bridge` tests are unchanged.
- [ ] **Step 6: Commit**: `feat(cli): git board project pull, push and sync; bridge is project now`.

### Task 4: The words: help, guide, README, messages

**Files:**
- Modify: `cmd/git-board/sync.go`'s `Long`:
  - `--git` is "`git board pull`, then `git board push`";
  - one paragraph names each side's commands.
- Modify: `docs/guide/forges.md`'s "On the command line" section. Add a
  table, side by side: the git remote's `fetch`, `pull` and `push`; the
  forges' `forge pull|push|sync`; the Project's
  `project pull|push|sync|resolve|configure`; and `sync`, the run of all
  three.
- Modify:
  - `README.md`, `docs/guide/configuration.md` and `CONTRIBUTING.md`:
    every `bridge` command becomes the `project` one. `--apply` goes,
    since the new verbs act by default; `--check` replaces `--dry-run`.
  - The user-facing strings that name a `git-board bridge …` command in
    `internal/config/config.go`, `internal/ghbridge/session.go`,
    `internal/ghbridge/sync.go`, `internal/lint/lint.go`,
    `internal/tui/tui.go`, `internal/server/sync.go`,
    `web/src/components/SyncSettings.vue` and
    `web/src/pages/ConflictsPage.vue` name the `git board project …`
    command now.
  - Leave `git bug bridge pull` in `ghbridge.go` alone: that's git-bug's
    command, not ours.

- [ ] **Step 1: Make the edits.**
- [ ] **Step 2: Check that nothing else names the old commands.**
  Run:

  ```
  rtk proxy grep -rnE 'git[- ]board bridge' --include='*.go' --include='*.md' --include='*.vue' --include='*.ts' . | grep -v '_test.go\|docs/prd/\|node_modules\|cmd/git-board/bridge'
  ```

  Expected: no output.
- [ ] **Step 3: Run the tests the strings touch.**
  `go test ./cmd/git-board/ ./internal/ghbridge/ ./internal/lint/ ./internal/tui/ ./internal/server/ ./internal/config/`.
  Expected: they pass. A test that matched an old string is updated to
  the new one.
- [ ] **Step 4: Commit**: `docs: one verb per side in the guide, the README and the messages`.
  The pre-commit hook runs the web typecheck on the two `.vue` files.
