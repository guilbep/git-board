# One verb per side M3: the web page, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Settings › Sync gets each side's actions and a Start over
section:
- the origin row gets Fetch, Pull and Push;
- each forge row gets Pull, Push and Sync, after a check;
- the Project's line gets Pull, Push and Sync;
- Start over holds four cards: empty this clone's board, make origin
  match, take origin's board, and remove git-bug's data. Each card
  explains itself, shows its command, asks for the repository's name, and
  then names its backup.

**Architecture:**
- **The server** grows by small steps on what it has:
  - `push` joins `fetch` and `pull` on `/api/remotes/{name}/{action}`;
  - `get` joins the forge job's modes (`GetForges`);
  - `/api/sync/apply` takes an optional side (`LabelsOnly`,
    `ProjectOnly`);
  - `/api/start-over` is new: GET says what Start over would find, POST
    runs one action under the write lock, and refuses while a sync job
    runs.
- **The page:** a new module, `web/src/startover.ts`, holds the cards'
  words, commands and confirmation rule, tested with `node:test`. A new
  component, `StartOver.vue`, renders it under the run buttons of
  `SyncSettings.vue`, which gets each row's actions.

**Tech Stack:** Go (`net/http`), Vue 3 with `<script setup>`, Quasar's
`q-dialog`, `node:test`, and `vue-tsc`.

**Spec:** `docs/prd/sync-sides.md`: "The web page", "Starting over", and
the M3 row.

## Global Constraints

- **Every button's `title`** names its command:
  - origin: `git board fetch`, `git board pull`, `git board push`;
  - the forges: `git board forge pull|push|sync`;
  - the Project: `git board project pull|push|sync`.
- **A forge's real run** still needs a clean check of the last 15
  minutes, as today. Its buttons are disabled until there is one, and
  their title says so.
- **A Start over action asks for the repository's name**: `repo` from
  `web/src/remotes.ts`, typed exactly, with spaces at the ends trimmed.
  The server refuses while a sync job runs, with 409 and code `syncing`.
- **The page's voice:** short sentences, no jargon. Each card says what
  it does, what it loses, when to use it, and the command, in that order.
- **Commits** are `feat(server): …`, `feat(web): …` or `docs: …`, with
  no `Co-Authored-By` trailer.
- **New Go tests** call `t.Parallel()` unless they swap a package
  variable.

## Review Focus

1. **A Start over action while a forge job runs** answers 409 and changes
   nothing. Test: Task 1, `TestStartOverWaitsForTheSync`.
2. **A typed name that differs only in case**, or a partial name, doesn't
   enable the button. Test: Task 2, the `canConfirm` cases.
3. **The git-bug card on a repository that was never switched** (git-bug's
   data, no switch's mark) isn't offered as a prune. It says to switch
   first. Test: Task 1 (`switched: false` in GET), and Task 2
   (`cardsFor`).
4. **After a reset, the page shows the empty board** without a reload: the
   page reloads its status and the remotes after any Start over action.
   Checked by hand in Task 4.
5. **An unknown action** answers 400. Test: Task 1,
   `TestStartOverActions`.

---

### Task 1: The server: push, get, the Project's side, start-over

**Files:**
- Modify: `internal/server/remotes.go`. `push` joins the action map,
  calling `s.b.PushStep`.
- Modify: `internal/server/forges.go`:
  - `forgeModes` is `{"sync", "send", "get"}`;
  - `forgeRun` calls `GetForges` for `get`;
  - the 400 message says `want sync, send or get`.
- Modify: `internal/server/sync.go`. `handleSyncApply` decodes an
  optional body, `{"side": "labels" | "project"}`, where empty means
  both, and `apply(side string)` restricts the plan.
- Create: `internal/server/startover.go`, with `handleStartOverStatus`
  (GET) and `handleStartOver` (POST).
- Modify: `internal/server/server.go`. Two routes:
  `GET /api/start-over` and `POST /api/start-over`.
- Test: `internal/server/startover_test.go`. Update
  `remotes_test.go` (push now works) and `issues_test.go`'s
  `TestForgeSend` (`get` is a mode now).

**Interfaces:**
- **GET `/api/start-over`** answers
  `{"switched": bool, "git_bug": {"refs": int, "cache": bool, "keys": int}, "remote": bool}`:
  - `git_bug` comes from `s.b.Prune("", true)`;
  - `switched` is false when that is `ErrNotSwitched`, and `git_bug`
    then counts `refs/bugs/*` alone;
  - `remote` says origin exists.
- **POST `/api/start-over`** takes `{"action": "reset" | "take" | "push-prune" | "prune", "on_remote": bool}`:
  - `reset` is `ResetHard("")`, `take` is `ResetHard("origin")`, and
    `push-prune` is `PushPrune("origin")`;
  - `prune` is `Prune(on_remote ? "origin" : "", false)`.

  It answers `{"deleted": int, "backup": string, "me": bool}`. For
  `prune`, `deleted` is the sum of its refs. It runs under `beginWrite`,
  and with `s.forgeMu` held, `s.syncing` true answers 409 `syncing`.
  After a reset it clears `s.job`, so the page drops the old check.
- `apply(side string) error`; `handleSyncResolve` and the whole sync keep
  calling it with `""`.

- [ ] **Step 1: Write the failing tests.**
  - `TestStartOverActions`, with `t.Parallel()`:
    1. `storetest.NewPair` and `serverOf(mine)`. Mine creates an issue
       and pushes.
    2. POST `reset`: 200 with `backup` set, and the issue list is empty.
    3. POST `push-prune`: 200, and origin's `refs/board/issue` is empty.
    4. POST `take` on `serverOf(theirs)` after a theirs issue: 200, and
       theirs' list is empty.
    5. POST `{"action": "nope"}`: 400.
  - `TestStartOverWaitsForTheSync`, with `t.Parallel()`: set
    `s.syncing.Store(true)` on an `apiServer`. POST `reset` answers 409,
    and the issue is still there.
  - `TestStartOverStatus`, with `t.Parallel()`: on a repository with a
    `refs/bugs/<id>` ref and a switch's mark, GET says
    `switched: true, git_bug.refs: 1`. Without the mark, it says
    `switched: false`.
  - In `TestRemotesPushAndPull`, the push 404 assertion becomes: POST
    `/api/remotes/origin/push` answers 200, and leaves
    `Issues == store.Ahead{}`.
  - In `TestForgeSend`, the loop over `{"push", "get"}` keeps `"push"`
    alone. After the clean dry run, `start({"mode": "get"})` runs as
    `get`.
  - `TestSyncApplyOneSide`: if the server's Project tests have a fake
    (`s.project`), apply with `{"side": "labels"}`, and the fake writer
    records no edit. Otherwise skip it, with a ruling.
- [ ] **Step 2: Run them, and see them fail.**
  `go test ./internal/server/ -run 'TestStartOver|TestRemotesPushAndPull|TestForgeSend|TestSyncApplyOneSide'`.
- [ ] **Step 3: Implement** the four server changes.
- [ ] **Step 4: Run Step 2's command, then `go test ./internal/server/`.**
  Expected: all pass.
- [ ] **Step 5: Commit**: `feat(server): push, a forge pull, a Project side, and start over`.

### Task 2: The cards' words, as a tested module

**Files:**
- Create: `web/src/startover.ts` and `web/src/startover.test.ts`.
- Modify: `web/package.json`. Add `src/startover.test.ts` to the `test`
  script's list.
- Modify: `web/src/api/board.ts`:
  - `remoteAction`'s action takes `'push'` too;
  - `startForgeSync`'s mode takes `'get'`;
  - `applySync(side?: 'labels' | 'project')`;
  - `getStartOver()` and `startOver(action, onRemote)`, with the types
    `StartOverStatus` and `StartOverResult`.

**Interfaces:**
- `export type CardId = 'reset' | 'push-prune' | 'take' | 'prune'`
- `export interface Card { id: CardId; title: string; does: string; loses: string; when: string; command: string; button: string }`
- **`export function cardsFor(st: StartOverStatus): Card[]`** returns the
  three resets in the order `reset`, `push-prune`, `take`:
  - `push-prune` and `take` only when `st.remote`;
  - then `prune`, when `st.git_bug.refs + st.git_bug.keys > 0` or
    `st.git_bug.cache`;
  - unswitched, `prune`'s `does` says
    `git board switch first: git-bug's issues are this repository's only copy until then`,
    and its `button` is `''`, which renders no button.
- **The commands:** `git board reset --hard`, `git board push --prune`,
  `git board reset --hard origin` and `git board prune`.
- **The buttons:**
  - `Empty this clone's board`;
  - `Make origin's board this clone's`;
  - `Take origin's board`;
  - `Remove git-bug's data`.
- **`export function canConfirm(typed: string, repo: string): boolean`**
  is true when `typed.trim() === repo && repo !== ''`.
- **`export const steps: string[]`** holds the four steps of starting
  over everywhere, and the forge pull's limit. Their words follow
  `docs/guide/forges.md`'s "Starting over".
- **`export function resultLine(r: StartOverResult): string`**:
  `"<n> refs deleted, backed up in <backup>"`, or `"Nothing to delete"`
  when `deleted` is 0, plus `"; board.me is unset: its person is gone"`
  when `me`.

- [ ] **Step 1: Write `startover.test.ts`** with `node:test` and
  `assert/strict`, as `gate.test.ts` does. It covers:
  - `cardsFor` with no remote and no git-bug data: one card, `reset`;
  - with a remote: three cards, in order;
  - with git-bug refs and switched: four, the last `prune`, with a
    button;
  - unswitched: `prune`'s button is `''`;
  - `canConfirm`: `('git-board', 'git-board')` true,
    `(' git-board ', 'git-board')` true, `('Git-Board', 'git-board')`
    false, `('git', 'git-board')` false, `('', '')` false;
  - `resultLine`: the three forms.
- [ ] **Step 2: Run it, and see it fail.**
  `cd web && node --test src/startover.test.ts`.
- [ ] **Step 3: Implement** `startover.ts` and the API functions.
- [ ] **Step 4: Run Step 2's command, then `cd web && npm test && npm run typecheck`.**
  Expected: all pass.
- [ ] **Step 5: Commit**: `feat(web): the start over cards' words, and the API's new calls`.

### Task 3: The page: each row's actions, and Start over

**Files:**
- Create: `web/src/components/StartOver.vue`. It sits in a `<details>`
  titled "Start over", collapsed, holding:
  - the steps;
  - one card per `cardsFor(status)` entry, with its title, does, loses,
    when, `<code>{command}</code>`, a text input labelled
    `Type {repo} to confirm`, and its button, enabled by `canConfirm`;
  - for `prune` with a remote, a checkbox `Also on origin (once no clone
    or automation uses git-bug)`;
  - after a run, `resultLine`, and a call to `emit('done')`.

  Errors show in an `epfl-alert-danger` paragraph.
- Modify: `web/src/components/SyncSettings.vue`:
  - **the table** gets an "Actions" column;
  - **the origin row:** Fetch, Pull and Push, each `remoteAction` then
    `reload()` and `loadRemotes()`;
  - **each forge row:** Pull, Push and Sync, with `startForgeSync(false, mode, [name])`
    for the modes `get`, `send` and `sync`. They're enabled when the job
    is a clean dry run within the page's knowledge (`job.dry_run`,
    state `done`, no errors), and otherwise their title says
    `Run Check first`;
  - **the Project's line:** Pull, Push and Sync, with
    `applySync('labels' | 'project' | undefined)`, then `reload()`;
  - **the lead paragraph** says each row has its own actions, and the
    run buttons do all of them;
  - `<StartOver @done="afterStartOver" />` goes under the run buttons.
    `afterStartOver` reloads the status, the remotes and the forges.
  - **`reportTitle`'s map** gains `get: 'The forges’ pull'`.
- Modify: `web/src/ways.ts`, if its mode or label maps list the modes,
  to add `get`.

- [ ] **Step 1: Make the edits.**
- [ ] **Step 2: Run `cd web && npm run typecheck && npm test`.**
  Expected: both pass.
- [ ] **Step 3: Commit**: `feat(web): Settings › Sync gets each side's actions, and Start over`.

### Task 4: Seeing it work

- [ ] **Step 1: Build the binary.** `make web-build && go build -o <scratchpad>/git-board ./cmd/git-board`.
- [ ] **Step 2: Set up a scratch clone with a bare origin** in the
  scratchpad: two issues, pushed.
- [ ] **Step 3: Serve it,** `git-board serve` on a free port. Open
  Settings › Sync with the Playwright browser tools and screenshot it.
  - Open Start over, type the repository's name, and run "Empty this
    clone's board". The board is empty, and the backup line shows.
  - Run "Make origin's board this clone's". Origin's `refs/board/issue`
    is empty.
- [ ] **Step 4: Ledger what was seen.** Fix what's broken, each fix with a
  test where one fits, and commit as `fix(web): …`.
