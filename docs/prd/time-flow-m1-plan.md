# Time flow M1: the column, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The issue page (and the board's side panel) is one column as
wide as GitHub's, with the sidebar beside it: the tabs Conversation and
History *n*; under Conversation, the current step's workspace from step 2
on, the description folded under it, then the flow cut into one segment
per step, with its filters, order, grouping, "new since your last visit",
and a composer that carries the issue's step. The 340 px aside goes.
Screens 6c and 6g.

**Architecture:** One pure module, `web/src/flow.ts`, holds the rules:
which segment an entry is in, the filters, the folding of review threads,
the divider's words, the first new entry. It is tested in
`flow.test.ts`. One new component, `IssueFlow.vue`, draws the controls
and the segments from it. `IssueStep.vue` stays the frame (the rail and
the workspace) and trades its two columns (`gb-cols`: the shown step, the
`gb-talk` aside) for a wrapping row: the column, then the sidebar.
`IssuePanel.vue` fills the slots. Nothing changes on the server: every
key M1 reads or writes exists today.

**Tech Stack:** Vue 3, `node:test`. No Go change.

**Spec:** `docs/prd/time-flow.md`, "Decisions", "Calls taken" and "M1:
the column"; `docs/design/time-flow/README.md`, 6c and 6g, "States and
interactions", "Layout", "Edge cases", "Accessibility";
`screens/F3-flow.html` and `screens/F7-step3.html` for the markup and the
copy.

**Base:** main at `31e37b5`, branch `feat/time-flow`. The PRD, this plan
and the design folder go first, in their own PR (approved 2026-10-27);
the code follows on the same branch, its commits pushed once that PR has
landed.

## Global Constraints

- No new entity, operation kind or metadata key. M1 reads `cycle:step`,
  `ai:model` and `review:*`, and writes `cycle:step` only, with today's
  values "1" to "7". "07" is display.
- The workspaces (`Cycle*.vue`) and `HistoryView.vue` are mounted,
  unchanged inside. History's scrubber, lanes and snapshot behave as
  today.
- The copy is the screens' (F3, F7), with Person A, B, C in fixtures and
  tests; logins alice, bob, carol.
- EPFL tokens (`var(--…)` from `web/epfl-design`), not hex. Teal for step
  numbers and dividers, red for the "new since" line, the current
  divider in black. The column is `flex: 999 1 640px; min-width: 0`,
  padding 24px 40px 40px, content `max-width: 1000px`, a gap of 18px;
  the sidebar `flex: 1 1 260px`; a flow entry a grid `44px minmax(0,
  1fr)` with a 12px gap.
- Accessibility: `<button aria-pressed>` for every toggle, a
  `role="tablist"` with `role="tab"` and `aria-selected` for the tabs,
  `<a href>` for the rail, `role="separator"` with an `aria-label` for the
  "new since" line. Focus order: title, rail, tabs, workspace or
  description, flow controls, entries, composer, sidebar. Only a thread's
  unfold may move (120 ms ease-out).
- While developing: `cd web && npx tsx --test src/flow.test.ts`, then
  `cd web && npm run typecheck && npm test` before each commit (lefthook
  runs the typecheck). `make ci` once, at the end (Task 9), through
  `rtk proxy make ci` into a log.
- Commit on `feat/time-flow`, never push `main`, never tag. No
  attribution trailer.
- No new dependency.

## Review Focus

1. A comment written today under *This step* while a done step was
   shown carries that done step: it sits in that step's segment, not in
   the one current when it was written. (Task 1: `a comment keeps the
   step it carries`.)
2. An issue at step 5 by evidence alone (a test committed, no status
   change since in-progress): entries after the last status move are in
   segment 5, not 4. (Task 1: `after the last status move, the issue's
   step`.)
3. *This step* with Mine on, and a review reply whose root is filtered
   out: the reply shows as an entry of its own, not lost. (Task 1:
   `a reply without its root stays`.)
4. The rail's link opened in a new tab (`#/issues/<id>?step=3`) lands on
   the page scrolled to segment 03, and a click on the page scrolls
   without a navigation. (Task 4's browser check in Task 9.)
5. The side panel at its default width: the sidebar stacks under the
   column, j/k and Esc still work. (Task 9.)
6. A new comment posted with *This step* off and the page scrolled to
   segment 02: it carries the issue's step. (Task 7, checked in Task 9.)
7. A doc issue past in-progress (step 4 skipped): its in-progress
   entries are in 05, there is no 04 divider, and the rail's 04 is not a
   link. (Task 1: `a doc issue skips 4`; Task 4.)

---

### Task 1: the flow's rules

**Files:**
- Create: `web/src/flow.ts`
- Test: `web/src/flow.test.ts` (and add it to `web/package.json`'s
  `test` list)

**Interfaces:**
- Consumes: `IssueEvent`, `CycleStages` (`api/board.ts`), `threadsOf`
  (`review.ts`), `STEPS` (`cycle.ts`), `on` (`when.ts`).
- Produces:

```ts
export interface Segment { n: number; entries: IssueEvent[] }
// every step from 1 to max(step, the highest cycle:step carried), empty
// ones included, minus `skipped`; step order, time order inside; the
// description excluded. No cycle (step 0): one segment, n 0, every entry
export function segments(events: IssueEvent[], stages: CycleStages | undefined, step: number, skipped: number[] = []): Segment[]

// a person's reading of the issue page (call 9, open point 6); Task 8
// keeps it in the browser
export interface Reading { tab: 'conversation' | 'history'; show: 'step' | 'all' | 'open'; mine: boolean; agent: boolean; milestones: boolean; newest: boolean; threads: boolean; description: boolean }
export const READING: Reading // All, Threads, Oldest first, the description folded
// the filters: one of step / all / open; Mine, Agent and Milestones
// narrow it to their union; newest reverses segments and entries
export function pick(segs: Segment[], events: IssueEvent[], r: Reading, meId: string, step: number): Segment[]

export interface Item { event: IssueEvent; replies: IssueEvent[]; resolved?: boolean }
// Threads grouping: a review reply whose root is among the entries folds
// under it; everything else is an item with no replies
export function fold(entries: IssueEvent[], events: IssueEvent[]): Item[]

// "01 Intent · Oct 21 – 22 · 12 entries", "07 Verify · Oct 25 · current"
export function divider(s: Segment, current: boolean): string

// the first entry after `since`, not mine; undefined on a first visit
export function firstNew(segs: Segment[], since: number | null, meId: string): string | undefined
```

The segment rule is the PRD's call 3. Walk the events in time order (as
`replay.ts` sorts them), with a running step that starts at 1:

| Event | Running step | Its segment |
| --- | --- | --- |
| `labels` adding `status:<s>` | `before` or unknown 1; `ready` 2, or 3 when a `risk:` label is on; `in_progress` 4; `in_review` 6; `validation`, `closing` 7 | the new running step |
| `labels` adding `risk:` at `ready` | 3 | 3 |
| `status` closed | 7 | 7 |
| a comment with `cycle:step` | raised to it, until the next status move | its `cycle:step` |
| a review comment without it | unchanged | 6 |
| anything else | unchanged | the running step |

A running step that lands on a skipped step moves up to the next one not
skipped. After the last status move, the running step is `step` (the
issue's, from the server), which knows the evidence the timeline
doesn't. A
`ponytail:` comment on `segments` names the ceiling (a step opened by
evidence alone shows only through the comments that carry it) and the
upgrade (the server replaying `internal/cycle` over the operations).

- [ ] **Step 1:** Write the failing tests, on one fixture timeline (an
  `ev(id, at, extra)` helper, the stages
  `{ before: ['backlog'], ready: 'ready', in_progress: ['in-progress'], in_review: ['in-review'], validation: [], closing: 'done', … }`):

```ts
test('segments, one per step opened', () => {
  // created; c1 (no key); +status:ready; c2 step 2; +risk:structural;
  // c3 step 3; +status:in-progress -status:ready; +status:in-review;
  // r1 a review line comment; c6 (no key). The issue is at 6.
  assert.deepEqual(segments(events, stages, 6).map((s) => [s.n, s.entries.map((e) => e.id)]), [
    [1, ['c1']], [2, ['l1', 'c2']], [3, ['l2', 'c3']], [4, ['l3']], [5, []], [6, ['l4', 'r1', 'c6']],
  ]);
});
test('a comment keeps the step it carries', …)          // c7 step 3, written at in-review: in 3, after c3
test('after the last status move, the issue\'s step', …) // in-progress, then c8 (no key); the issue at 5: c8 in 5
test('the current segment shows empty', …)              // only the created event, step 1: [[1, []]]
test('a doc issue skips 4', …)                          // skipped [4], in-progress then c9 (no key), the issue at 5: no 4, l3 and c9 in 5
test('sent back from Verify', …)                        // c10 step 7, then +status:in-progress, the issue at 4: segments 1 to 7, current 4
test('no cycle', …)                                     // stages undefined, step 0: [[0, every entry but the created]]
test('closing the issue is step 7', …)
test('the filters', …)        // step: only the current segment; open: the open review threads' roots and replies;
                              // mine; agent (ai:model); milestones (no comment); mine + agent: their union;
                              // newest: segments and entries reversed
test('a reply without its root stays', …)               // fold with the root filtered out: the reply is an item
test('threads fold their replies', …)                   // r1 with two replies: one item, two replies, resolved false
test('the divider', …)        // a span of two days, one day, current, an empty current segment ('· current' only)
test('new since the last visit', …)                     // null → undefined; mine skipped; nothing after → undefined
```

- [ ] **Step 2:** Run: `cd web && npx tsx --test src/flow.test.ts`.
  Expected: FAIL, `flow.ts` doesn't exist.
- [ ] **Step 3:** Implement `flow.ts`. `forStep` in `cycle.ts` stays
  until Task 2 removes its last caller.
- [ ] **Step 4:** Run: `cd web && npm run typecheck && npm test`.
  Expected: PASS.
- [ ] **Step 5:** Commit `feat(flow): the segments, the filters and the
  folds of the issue's flow`.

### Task 2: one column and the sidebar

**Files:**
- Modify: `web/src/components/IssueStep.vue` (the frame, the slots)
- Modify: `web/src/components/IssuePanel.vue` (the tabs, the slots)
- Modify: `web/src/cycle.ts`, `web/src/cycle.test.ts` (`forStep` goes)
- Delete: `web/src/components/ReviewAside.vue`

**Interfaces:**
- `IssueStep` gains a prop `head: boolean` (the Conversation tab is
  shown) and these slots, replacing `talk`:
  - `tabs`: above the head, inside the column;
  - `intent` as today, with `{ folded: boolean; cycle: Cycle | null }`
    (folded from step 2 on; the criteria are `cycle.evidence.criteria`);
  - `review` as today;
  - `flow` with `{ cycle: Cycle | null; send?: { name: string; brief: (body: string) => void } }`:
    the flow reads `cycle.step`, `cycle.stages` and `cycle.skipped` from
    it (step 0 and no stages while it loads or when it fails); `send` is
    set when the agent has a worktree (today's `button === 'brief'`),
    `name` its tool (`agent.session.agent`, "the agent" when unknown);
  - `side`: the sidebar, after the column.
- The template: the rail, then `div.gb-row` holding
  `section.gb-column` (`tabs`; when `head`, the eyebrow, the workspace or
  the intent, the agent box and the status bar, as today; then `flow`)
  and the `side` slot. `section.gb-shown`, `aside.gb-talk` and
  `gb-cols` go.

- [ ] **Step 1:** In `IssueStep.vue`, the row and the column as above.
  From step 2 on, the head is the shown step's workspace and the
  `intent` slot follows it with `folded`; at step 1 the `intent` slot
  comes first, unfolded, then `CycleIntent`, as today.
- [ ] **Step 2:** In `IssuePanel.vue`, `talk` becomes
  `reading = reactive({ ...READING })`, whose `tab` is the tab. The `tabs`
  slot is a `div role="tablist" aria-label="Issue"` with two
  `button role="tab"`: Conversation, History *n* (`changeCount`, as
  today). The `flow` slot shows `HistoryView` under History; under
  Conversation, today's list of every comment and event in time order
  (`rest`) and the composer, so the app keeps working until Task 3.
  `IssueSide` moves to the `side` slot. `ReviewAside` and `said()` go,
  and with `said()` the last caller of `forStep`: it goes from `cycle.ts`
  with its test.
- [ ] **Step 3:** Styles: `.gb-row { display: flex; flex-wrap: wrap }`,
  the column and the sidebar as the Global Constraints say; the sidebar's
  left hairline, none when it wraps under the column. The existing media
  queries for the panel stay.
- [ ] **Step 4:** Run: `cd web && npm run typecheck && npm test`.
  Expected: PASS.
- [ ] **Step 5:** Commit `feat(issue): one column and the sidebar, the
  aside goes`.

### Task 3: the flow

**Files:**
- Create: `web/src/components/IssueFlow.vue`
- Modify: `web/src/components/IssuePanel.vue` (the `flow` slot)

**Interfaces:**
- `IssueFlow` props: `{ events: IssueEvent[]; cycle: Cycle | null; meId: string; since: number | null; forges?: Forge[]; fields: Fields; save: … }`
  (`save` as `CommentCard` takes it), and `v-model:reading` (the panel's
  `reading`). Emits `line(path)`, `ref(n, ev)`, `workspace(n)`. Exposes
  `jump(n)` and `unfold()`.
- `IssueSide`, `CommentCard` and `TimelineEvent` are reused as they are.

- [ ] **Step 1:** The controls, as F3 draws them: the label *Flow*; the
  chips *This step n*, *All n*, *Open threads n*, *Mine n*, *Agent n*,
  *Milestones n* (open point 6; not drawn, after *Agent*), each a
  `button aria-pressed`, counts from `pick` per chip; the segs
  *Oldest first / Newest first* and *Flat / Threads*; the helper line
  "n segments, one per step: a comment carries the step it was written
  in. Threads fold to their first line; the composer carries the current
  step." Every control reads and writes `reading`. The controls wrap
  under 1280 px. *On v*N and *Threads: under their section / in the
  flow* are not drawn (M3, M2).
- [ ] **Step 2:** The segments: an `ol` per segment under a divider
  (`h3`, `id="step-<n>"`, `tabindex="-1"`, the `divider()` words; the
  current one black, the others teal with a hairline to the right). An
  entry is `li.gb-fe`, the grid of the constraints: the gutter holds the
  segment's number ("03") on its first entry; a comment is a
  `CommentCard`, an event a `TimelineEvent`. With no cycle (one segment,
  n 0) there is no divider, no gutter number and no *This step* chip.
  Under *Threads*, a folded
  thread is one line, "<b>Person B</b> on `path:line` · "first words"",
  with a `button aria-expanded` "n comments · open ▸" (or "resolved")
  that unfolds the root and its replies in place; `path:line` emits
  `line`.
- [ ] **Step 3:** *This step*: above the current segment, the pointer
  "Segments 01 to 0k are folded · n entries · show all" (`show all`
  picks *All*); nothing when the current segment is the first.
- [ ] **Step 4:** The long flow: over 100 entries under *All*, the
  segments before the current one render their divider only, with their
  count, and one *show all* unfolds them. `unfold()` does the same, and
  `jump(n)` picks *All* when segment n is hidden, then scrolls to
  `#step-<n>` and focuses it.
- [ ] **Step 5:** "New since your last visit": a `li role="separator"
  aria-label="New since your last visit, <on(since, true)>"` before
  `firstNew()`'s entry, the red line of F3. `IssuePanel` computes `since`
  once per issue: `visit(loadVisit('issue:' + id), Date.now())`, saves
  it, passes `prev`.
- [ ] **Step 6:** In `IssuePanel`, the `flow` slot mounts `IssueFlow`
  with the composer under it; `reveal(id)` calls `flow.unfold()` before
  it scrolls (the inbox's `at`, the sidebar's Inbox).
- [ ] **Step 7:** Run: `cd web && npm run typecheck && npm test`.
  Expected: PASS.
- [ ] **Step 8:** Commit `feat(issue): the flow, cut into segments, with
  its filters and folded threads`.

### Task 4: the rail jumps to a segment

**Files:**
- Modify: `web/src/components/IssueStep.vue` (the rail)
- Modify: `web/src/components/IssuePanel.vue` (a `step` prop)
- Modify: `web/src/pages/IssuePage.vue` (`?step=`)

- [ ] **Step 1:** In the rail (`aria-label="The cycle, click a step to
  jump to its segment"`), a done or current step is an
  `<a href="#/issues/<id>?step=<n>">` with `aria-current="step"` on the
  current one. A plain click emits `jump(n)` and prevents the
  navigation; a click with `metaKey`, `ctrlKey` or `shiftKey`, or not
  the main button, is left to the browser. A locked or skipped step (the
  row states of `steps()` in `cycle.ts`) is a `span aria-disabled="true"`
  with its lock or its "skipped, …" line, as today.
- [ ] **Step 2:** `IssuePanel` handles `jump(n)`: the Conversation tab,
  then `flow.jump(n)`. `IssuePage` passes `route.query.step` as the
  panel's `step` prop, jumped to once the issue has loaded, as `at` is
  revealed today.
- [ ] **Step 3:** Run: `cd web && npm run typecheck && npm test`.
  Expected: PASS.
- [ ] **Step 4:** Commit `feat(issue): the step rail links to the
  segments`.

### Task 5: the head of the column

**Files:**
- Modify: `web/src/components/IssueStep.vue`
- Modify: `web/src/components/IssuePanel.vue` (the `intent` slot)
- Modify: `web/src/components/IssueFlow.vue` (the divider's button)

- [ ] **Step 1:** The folded description, in the `intent` slot when
  `folded`: one bordered line, "<b>Person A</b> · the description · 4
  criteria · edited Oct 22" (the criteria from `cycle.evidence.criteria`,
  passed in the slot's props; "edited" only when the created event has
  `history`), and a `button aria-expanded` *Read the description* that
  shows the `CommentCard` in place (`reading.description`). M1 leaves out
  F7's versions and threads (M2, M3).
- [ ] **Step 2:** A done segment's divider carries *Show the workspace*
  (call 5): it emits `workspace(n)`; `IssuePanel` calls
  `step.show(n)` and scrolls to the head, where today's "Back to step N"
  returns.
- [ ] **Step 3:** At step 7 the head is `CycleVerify` (call 6).
  `showLine` still shows step 6's workspace on a `path:line` click.
- [ ] **Step 4:** Run: `cd web && npm run typecheck && npm test`.
  Expected: PASS.
- [ ] **Step 5:** Commit `feat(issue): the step's workspace heads the
  column, the description folded under it`.

### Task 6: the comment's header

**Files:**
- Modify: `web/src/components/CommentCard.vue`

- [ ] **Step 1:** The header gains, after the time, "step 03" when the
  comment carries `cycle:step`, and at its right the sync state per
  forge: "on GitHub #37 ✓" (a link, as today's footer), "not on GitHub
  yet, next sync" (and the edit's pending state, as today), or "stays
  local" for a review comment. The footer goes.
- [ ] **Step 2:** Run: `cd web && npm run typecheck && npm test`.
  Expected: PASS.
- [ ] **Step 3:** Commit `feat(issue): a comment's step and sync state in
  its header`.

### Task 7: the composer carries the step

**Files:**
- Modify: `web/src/components/IssueComposer.vue`
- Modify: `web/src/components/IssuePanel.vue`
- Modify: `web/src/flow.ts`, `web/src/flow.test.ts` (`carries`)

**Interfaces:**
- `IssueComposer` gains `carries?: string` ("07 Verify") and
  `send?: string` (the agent's name); `comment(body, status?, send?:
  boolean)`.
- `flow.ts`: `export function carries(step: number): string` ("" at 0).

- [ ] **Step 1:** A test: `carries(7)` is "07 Verify", `carries(0)` is
  "".
- [ ] **Step 2:** The footer: "Saved in git-board now, on GitHub at the
  next sync. Carries [07 Verify]", the chip in teal. With `send`, a
  checkbox "Send to Claude Code" before *Comment*.
- [ ] **Step 3:** `IssuePanel` posts with the slot's `cycle.step` (call 2),
  never 0 once the cycle has loaded, whatever the tab or the filter; then,
  when the box is ticked, `send.brief(body)`. Revise's `comment(body, 3)`
  stays.
- [ ] **Step 4:** Run: `cd web && npm run typecheck && npm test`.
  Expected: PASS.
- [ ] **Step 5:** Commit `feat(issue): the composer carries the issue's
  step, and can send to the agent`.

### Task 8: the reading remembered

**Files:**
- Modify: `web/src/flow.ts`, `web/src/flow.test.ts`
- Modify: `web/src/components/IssuePanel.vue`

**Interfaces:**
- `flow.ts`: `export function readingOf(raw: string | null): Reading`
  (`READING` under what was kept, a key of the wrong type dropped,
  `READING` when `raw` is null or broken), `loadReading(): Reading`,
  `saveReading(r: Reading): void`, under the key `gb-reading`, in
  `try`/`catch` as `lastvisit.ts` does.

- [ ] **Step 1:** A test: `readingOf(null)` and `readingOf('{')` are
  `READING`; `readingOf('{"show":"open","newest":true,"tab":3}')` is
  `READING` with *Open threads* and *Newest first*, the tab Conversation.
- [ ] **Step 2:** Implement. `IssuePanel` starts from `loadReading()`
  and saves on every change (`watch(reading, saveReading, { deep: true })`).
  One reading for every issue, and the board's side panel shares it.
- [ ] **Step 3:** Run: `cd web && npm run typecheck && npm test`.
  Expected: PASS.
- [ ] **Step 4:** Commit `feat(issue): the page remembers your reading`.

### Task 9: verification and hand-off

- [ ] **Step 1:** `cd web && npm run typecheck && npm test` green, then
  `GOCACHE=$TMPDIR/go-build rtk proxy make ci > $TMPDIR/ci.log 2>&1`
  green (`TestCutAndPromoteSameDigests` is known to flake under the full
  run: rerun it alone before blaming the branch).
- [ ] **Step 2:** The browser check on a throwaway repo under `$TMPDIR`
  (unsigned commits: go-git can't read SSH-signed ones): `cd web && npm
  run build`, `go build -o $TMPDIR/git-board ./cmd/git-board`; a
  `.git-board.toml` with `[cycle]`; one issue walked to step 7 with
  comments on each step, a review thread, an `ai:model` comment, a
  keyless comment, 100+ entries on a second issue. One server per
  repository: `$TMPDIR/git-board serve --addr localhost:8099 --me alice`.
  Playwright at 1440 px, 1100 px and 800 px, the page and the side
  panel: tick 6c's five criteria and 6g's first two; History's scrubber,
  the arrows and *Now*; the Review Focus.
- [ ] **Step 3:** Fix what the check shows, commit. `git log --oneline
  main..` lists the commits; no push. Say it is ready for `wtx land
  feat/time-flow`.

## Self-review notes

- Spec coverage: the aside goes ✓ (2); the description first, the flow
  after ✓ (2, 3); segments ✓ (1, 3); Flat / Threads ✓ (1, 3); order ✓
  (1, 3); This step, All, Open threads, Mine, Agent, Milestones ✓ (1,
  3); the reading remembered ✓ (8); the
  folded description from step 2 with the workspace heading ✓ (5); the
  composer's step chip ✓ (7); "new since your last visit" ✓ (3); the
  rail as links ✓ (4); the sync state per entry ✓ (6, `TimelineEvent`
  already says it); the long flow ✓ (3); no agent, no *Send to* ✓ (7);
  loading and error states unchanged ✓ (2 keeps them).
- Not in M1, each with its milestone: *On v*N and the composer's version
  chip (M3); *Threads: under their section / in the flow*, *Unfold
  threads*, the role chip, body threads (M2); *Changes n* (M4); plan
  anchors (open point 5); the rules under 720 px (open point 4).
- The README lists `FlowControls` as a new component; M1 keeps the
  controls inside `IssueFlow.vue`, its only user.

wtx-effort: high
