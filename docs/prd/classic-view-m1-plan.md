# The classic issue page by default, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The issue page is the plain page of 17 October unless the board
has a `[cycle]` table or the person set `git config board.view cycle`. The
header's Work, and Settings' hint about the guessed stages, follow the
same view.

**Architecture:**
- **The config** records whether the file has `[cycle]` (`Cycle.On`).
- **The store** reads `board.view` at every level git reads.
- **The server's `/api/me`** says the view and where it came from.
- **In the web:**
  - `view.ts` holds the words, tested;
  - `IssuePanel.vue` draws today's `IssueStep` layout only for the cycle
    view, and otherwise the classic one, built from today's components
    (`CommentCard`, `TimelineEvent`, `IssueComposer`, `HistoryView`,
    `IssueSide`);
  - `AppHeader.vue` and `SettingsPage.vue` follow the view.

**Tech Stack:** Go (BurntSushi `toml.MetaData.IsDefined`), Vue 3, `node:test`.

**Spec:** `docs/prd/classic-view.md`.

## Global Constraints

- **`view`** is exactly `"classic"` or `"cycle"`.
- **`view_from`** is exactly `"board.view"`, `".git-board.toml"` or
  `"default"`.
- **`board.view`** is compared after trimming spaces and lowering case,
  and any other value counts as unset.
- **Until `/api/me` has answered, or failed,** the issue panel shows its
  loading skeleton. On a failure, the view is classic.
- **No cycle code is removed.** The classic layout is a second branch of
  the panel's template, not a copy of the old file.
- **New Go tests** call `t.Parallel()` unless they set an environment
  variable.
- **Commits** are `feat(…)`, `fix(…)` or `docs: …`, with no
  `Co-Authored-By` trailer.

## Review Focus

1. **A `[cycle.signoffs]` subtable alone,** with no bare `[cycle]`, still
   counts as having `[cycle]`. Test: Task 1, `TestCycleTableTurnsTheCycleOn`.
2. **`board.view` set in the global config only** is read. Test: Task 1,
   `TestViewReadsTheGlobalConfig`.
3. **`board.view = "Cycle "`,** with a capital and a space, is `cycle`;
   `board.view = "kanban"` is ignored, and the page says so. Test: Task 1,
   `TestResolveView`; Task 2, `viewWords`.
4. **An `/api/me` that fails** leaves the page classic, not stuck on the
   skeleton. Task 3, seen by hand.
5. **The page opened at `?step=…`** with the classic view shows the
   classic page and ignores the step. Task 3, seen by hand.

---

### Task 1: The view, on the server

**Files:**
- Modify: `internal/config/cycle.go`. `Cycle` gains
  `On bool \`toml:"-" json:"on"\``, which says the file has a `[cycle]`
  table.
- Modify: `internal/config/config.go`. In `parse`, set
  `r.Cycle.On = md.IsDefined("cycle")` before `r.Cycle` is copied.
- Modify: `internal/store/store.go`, to add `ViewSetting`.
- Modify: `internal/server/issues.go`: `meResponse` gains `View`,
  `ViewFrom` and `ViewIgnored`; `writeMe` fills them; add `resolveView`.
- Test: `internal/config/cycle_test.go`, `internal/store/store_test.go`
  and `internal/server/issues_test.go`.

**Interfaces:**
- Produces:
  - `func (b *Backend) ViewSetting() (string, error)`: runs
    `git config --get board.view` without `--local`. Exit code 1 means
    `""`, nil.
  - `func resolveView(setting string, cycleOn bool) (view, from, ignored string)`:
    - **`setting` is `classic` or `cycle`** (trimmed, lower case): that
      value, from `"board.view"`;
    - **otherwise:** `"cycle"` from `".git-board.toml"` when `cycleOn`,
      else `"classic"` from `"default"`;
    - **`ignored`** is the original setting when it was non-empty and not
      one of the two.
  - `meResponse` gains `View string \`json:"view"\``,
    `ViewFrom string \`json:"view_from"\`` and
    `ViewIgnored string \`json:"view_ignored,omitempty"\``.

- [ ] **Step 1: Write the failing tests.**
  - `TestCycleTableTurnsTheCycleOn`, with `t.Parallel()`, using `parse`:
    `""` gives `On == false`; `"[cycle]\n"`,
    `"[cycle]\nready = \"ready\"\n"` and `"[cycle.signoffs]\nqa = \"pm\"\n"`
    give `On == true`; `"[fields.status]\noptions = [\"a\"]\n"` gives
    `false`.
  - `TestResolveView`, table-driven, with `t.Parallel()`:
    - `("", false)` gives `classic`, `default`, `""`;
    - `("", true)` gives `cycle`, `.git-board.toml`, `""`;
    - `("classic", true)` gives `classic`, `board.view`, `""`;
    - `("Cycle ", false)` gives `cycle`, `board.view`, `""`;
    - `("kanban", true)` gives `cycle`, `.git-board.toml`, `kanban`.
  - `TestMeSaysTheView`, with `t.Parallel()` and `newIssueServer`:
    - `GET /api/me` says `view` is `classic` and `view_from` is
      `default`;
    - `storetest.Git(t, b.GitDir(), "config", "board.view", "cycle")`,
      then GET: `cycle`, from `board.view`;
    - `s.setConfig` with `Cycle.On` true and `board.view` unset again
      (`config --unset`): `cycle`, from `.git-board.toml`.
  - `TestViewReadsTheGlobalConfig`, serial: `t.Setenv("GIT_CONFIG_GLOBAL", <temp file>)`,
    `git config --file <it> board.view cycle`, then
    `storetest.New(t).ViewSetting()` is `"cycle"`.
- [ ] **Step 2: Run them, and see them fail.**
  `go test ./internal/config/ -run TestCycleTableTurnsTheCycleOn; go test ./internal/server/ -run 'TestResolveView|TestMeSaysTheView'; go test ./internal/store/ -run TestViewReadsTheGlobalConfig`.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run Step 2's commands, then `go test ./internal/config/ ./internal/server/ -run 'Cycle|Me|View|Settings'`.**
  Expected: all pass.
- [ ] **Step 5: Commit**: `feat(server): /api/me says which issue page: classic, or the cycle's`.

### Task 2: The view's words, in the web

**Files:**
- Modify: `web/src/api/board.ts`. `MeResponse` gains
  `view?: 'classic' | 'cycle'`,
  `view_from?: 'board.view' | '.git-board.toml' | 'default'` and
  `view_ignored?: string`.
- Modify: `web/src/me.ts`, to add `meLoaded` and `cycleView`.
- Create: `web/src/view.ts` and `web/src/view.test.ts`, and add the test
  file to `package.json`'s `test` script.

**Interfaces:**
- Produces:
  - `export function viewWords(m: Pick<MeResponse, 'view' | 'view_from' | 'view_ignored'> | null): string`:
    - `classic`, `default`: `Your issues open on the classic page: the board's .git-board.toml has no [cycle] table.`
    - `cycle`, `.git-board.toml`: `Your issues open on the cycle's page: the board's .git-board.toml has a [cycle] table.`
    - `classic` or `cycle` from `board.view`: `Your issues open on the {classic|cycle’s} page: your git config sets board.view.` That's `the classic page` or `the cycle’s page`.
    - **any of these, with `view_ignored`:** append
      `` board.view = “{value}” is neither classic nor cycle, so it’s ignored. ``
    - **`null`:** `''`.
  - `me.ts` exports `meLoaded = ref(false)`, set in `loadMe`'s
    `finally`, and `cycleView = computed(() => me.value?.view === 'cycle')`.

- [ ] **Step 1: Write `view.test.ts`**, covering the four forms and
  `null`, with exact strings.
- [ ] **Step 2: Run `node --test src/view.test.ts`, and see it fail.**
- [ ] **Step 3: Implement** `view.ts`, the type and the `me.ts` exports.
- [ ] **Step 4: Run `cd web && npm test && npm run typecheck`.**
  Expected: both pass.
- [ ] **Step 5: Commit**: `feat(web): the view's words, and cycleView`.

### Task 3: The classic page, the header and Settings

**Files:**
- Modify: `web/src/components/IssuePanel.vue`:
  - **Script:**
    - `const viewKnown = computed(() => meLoaded.value)`;
    - `onMounted`: `if (!meLoaded.value) void loadMe()`;
    - `const rest = computed(() => thread.value?.events.filter((e) => e.kind !== 'created') ?? [])`.
  - **Template:** the skeleton branch shows while `!thread || !viewKnown`.
    `<IssueStep v-if="cycleView" …>` keeps today's layout. `v-else` is
    the classic one:

    ```
    <div class="gb-cols">
      <div class="gb-maincol">
        tabs Conversation / History (reading.tab: 'conversation' | 'history')
        Conversation:
          CommentCard of `created` (no slot: MarkdownBody shows the body)
          <ol class="gb-tl"> per `rest`:
            CommentCard for comment, else TimelineEvent
          IssueComposer, with :comment="(b, st) => post(b, st, '')", no carries, no send
        History: HistoryView :thread :fields (no cycle)
      </div>
      IssueSide with today's props, without spec and reviewer
    </div>
    ```

    `reading.tab === 'changes'` falls back to Conversation in classic.
- Modify: `web/src/components/AppHeader.vue`: Work only with `cycleView`.
- Modify: `web/src/pages/SettingsPage.vue`:
  - **You tab:** a paragraph with `viewWords(me)`, then
    `` To change it for you: <code>git config board.view classic</code> or <code>cycle</code> (<code>--global</code> for every repository). For the whole team: a <code>[cycle]</code> table in <code>.git-board.toml</code>. ``
  - **The Project file hint** about the guessed stages gets `v-if="cycleView"`.

- [ ] **Step 1: Make the edits.**
- [ ] **Step 2: Run `cd web && npm run typecheck && npm test`.**
  Expected: both pass.
- [ ] **Step 3: Commit**: `feat(web): the classic issue page, unless the view is the cycle's`.

### Task 4: The guide, and seeing it

**Files:**
- Modify: `docs/guide/configuration.md`. The `[cycle]` section opens with
  three things: a board with this table walks the cycle; one without gets
  the classic page; and `board.view`, with an example.
- Test: by hand, on a scratch repository, with `git-board serve`:
  - **Classic by default:** the page shows Conversation, History and the
    side column, with no rail. The header has no Work.
  - **`git config board.view cycle`:** reload, and the cycle's page is
    back. The header has Work.
  - **`board.view kanban`:** Settings › You says it's ignored.
  - **`?step=plan`:** the classic page.

- [ ] **Step 1: Write the guide's lines.**
- [ ] **Step 2: Build** (`make web-build`, `go build`), serve a scratch
  repo, and check the four cases with Playwright. Fix what's wrong, each
  fix with a test where one fits.
- [ ] **Step 3: Commit**: `docs: the classic page, [cycle] and board.view`.
