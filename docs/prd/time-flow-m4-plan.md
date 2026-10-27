# Time flow M4: History and Changes, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** History gains two lanes (Versions and ticks, Steps) and a red
cursor; its snapshot shows the body's version and the threads' states as
of the cursor; its log is one row per core operation, `set-meta`
included, with the op id and the forge state, cut at the cursor. A third
tab, *Changes n*, lists the body's versions and ticks, diffs two versions
section by section with the threads written on them, then shows the
code: branch, commits, PR, files, plan. Screens 6d and 6e.

**Architecture:** The store sends each text's `set-meta` operations
beside its edits (`meta_history`), so the client sees every operation
with its id and time; no event kind is added. `replay.ts` learns the
core's operation names, the `set-meta` steps, each description step's
version, and `asOf`: the events as they were after a step, which M2 and
M3's pure rules (`versions`, `bodyThreads`, `stateOf`, `resolvedBy`)
read unchanged. `timeline.ts` gains the Versions lane and the step bars,
from `flow.ts`'s segments, which now carry the time each one opened.
`HistoryView`, `IssueTimeline` change; one new component,
`IssueChanges.vue`. The core is untouched.

**Tech Stack:** Vue 3, `node:test`; Go (the store).

**Spec:** `docs/prd/time-flow.md`, "Three readings", ruling 7 of M1,
"M4: History and Changes"; `docs/design/time-flow/README.md`, 6d's and
6e's criteria, "Components" (`HistoryView.vue`, `lanes.ts`, `history.ts`),
the History lanes row, "States and interactions" (the scrubber),
"Responsive behaviour"; `screens/F4-history.html` and
`screens/F5-changes.html` for the markup and the copy.

**Base:** `feat/time-flow-m2` at `6d7999c` (M3 done). M4 and M5 stay on
this branch, one `make ci` after the last (the user, 2026-10-27); no
approval stop between plan and code.

## What exists today, found in the code

**Operations the client sees.** `thread()` (`internal/store/thread.go:118`)
replays the log: `create`, `comment`, `set-title`, `set-status`,
`change-labels` become events whose `id` is the operation's id;
`edit-text` becomes a `history` entry of its text with its id (M2);
`set-meta` is merged into its target's `meta` and dropped: no id, no
time, no author reach the client. So a log row per `set-meta` and a
thread's resolution *as of then* both need the store to send them.
Log order is the oplog's (clock, then pack); the client has only times,
to the second.

**History today.** `HistoryView.vue` flattens the events with
`steps()` (`replay.ts`: kinds created, comment, edit, labels, status,
title; edits carry `before`/`after`), keeps `pos` (the latest change on
open and on growth), and shows: `IssueTimeline` (lanes Texts and
Changes, the status band with the closed strip, a date axis, label
column 4.5rem), the scrubber (prev, range, next, "As of … · change n of
N", *Latest*), the snapshot (`stateAfter`: status, title, labels with the
added ones outlined, the touched text, "All n texts as of then"), and the
log `hv-steps`: one row per step, later rows dimmed, each with its diff
(edits), word diff (renames) or field change (labels).

**Segments.** `segments()` (`flow.ts:31`) knows which operation opened
each step (`moves`, and a carried `cycle:step` raising `running`) but
returns only the entries; a comment carrying step 1 written at step 4
sits in segment 1, so a segment's entry times overlap the next one's.

**The code data.** `getChanges(id, {branch})` → `Changes {branch,
base_name, commits[{sha, subject, author, date}], files[{path, added,
deleted}]}`, as `CycleCode` uses it. `cycle.evidence` has `branch`,
`pr {number, url, state, draft…}`, `plan {path, status, by}`,
`plan_approval {by, at}`. Per-commit CI is not in `Changes`.

## Global Constraints

- No new entity, operation kind or event kind; the core gives no key a
  meaning. `meta_history` is a read of operations the log has.
- History's scrubber keeps today's behaviour: prev / next, the range
  (its arrow keys), the latest change on open and when the history
  grows; *Latest* is renamed *Now* (6d), same action.
- Thread states as of then reuse `stateOf` against the versions as of
  then; versions and states stay computed, never stored.
- Lanes: label column 110 px, 80 px under 720 px; dashed baseline;
  markers as % of the span; the cursor a 2 px line in `--gb-red-dot`
  across the lanes. The log's cursor row on `--gb-teal-bg` with an inset
  3 px `--gb-red-dot` bar; the snapshot's changed field outlined in
  `--gb-done`. EPFL tokens, not hex.
- The copy is F4's and F5's with Person A, B, C; logins alice, bob,
  carol.
- No Vue component tests: each component task is checked in a browser
  on `$TMPDIR/tf2` (rebuild the bundle, then the binary, restart
  `serve`, navigate with a fresh `?r=`).
- While developing: `cd web && node --experimental-strip-types --test
  src/replay.test.ts src/timeline.test.ts src/flow.test.ts
  src/sections.test.ts`; `go test ./internal/store/ -run …`;
  `npm run typecheck && npm test` before each commit. `make ci` once,
  after M5. A new Go test calls `t.Parallel()` first.
- Commit on `feat/time-flow-m2`; no push, no tag, no attribution
  trailer, no new dependency. Shortcuts carry a `ponytail:` comment.

## Review Focus

1. A thread resolved, then reopened (`thread:resolved` `""`): with the
   cursor between the two the snapshot reads it resolved, after the
   reopen open. (Task 2: `asOf follows a reopen`.)
2. The cursor before v2: the snapshot shows v1, and a thread written on
   v1 reads open (current), not carried or outdated. (Task 2: `asOf
   before v2 is v1`.)
3. Apply's `edit-text` and `set-meta` in the same second: the edit's
   step comes first, so the cursor on the edit never shows "resolved by
   v2" before v2. (Task 2: `an apply's edit sorts before its meta`.)
4. A comment carrying step 1 written at step 4 does not stretch step 1's
   bar over step 4's. (Task 3: `a late carried comment keeps the bars`.)
5. `tab: 'changes'` survives a reload, and a description never edited
   gives one diamond, one version card and no compare seg. (Task 3:
   `readingOf takes changes`, `one version is one diamond`; Task 7's
   browser step.)

---

### Task 0: the plan in the repository

- [ ] **Step 1:** This file, `docs/prd/time-flow-m4-plan.md`.
- [ ] **Step 2:** Commit `docs(prd): the time flow's M4 plan`.

### Task 1: the store sends each text's set-meta operations

**Files:**
- Modify: `internal/store/thread.go` (`Event.MetaHistory`, the
  `set-meta` case), `web/src/api/board.ts` (`IssueEvent.meta_history`)
- Test: `internal/store/thread_test.go`

**Interfaces:**

```go
// MetaSet is one set-meta operation on a text, oldest first.
type MetaSet struct {
	ID     string            `json:"id"`
	Author Person            `json:"author"`
	At     time.Time         `json:"at"`
	Meta   map[string]string `json:"meta"`
}
// Event gains: MetaHistory []MetaSet `json:"meta_history,omitempty"`
```

```ts
// IssueEvent gains
meta_history?: { id: string; author: Person; at: string; meta: Record<string, string> }[];
```

- [ ] **Step 1:** `TestThreadMetaHistory` (parallel): a comment, then
  `SetCommentMetadata` with `thread:resolved` `alice`'s id, then with
  `""` → the comment's `MetaHistory` has two entries in that order,
  distinct `<40hex>:<n>` ids, the second's meta `{"thread:resolved": ""}`;
  `Meta` is today's merge; a never-set comment has none.
- [ ] **Step 2:** `go test ./internal/store/ -run TestThreadMetaHistory`:
  FAIL. Implement (append in the `set-meta` case). PASS; `go test
  ./internal/store/ ./internal/server/` green.
- [ ] **Step 3:** Commit `feat(store): a text's set-meta operations, with
  their ids and times`.

### Task 2: replay knows the core's operations and the issue as of a step

**Files:**
- Modify: `web/src/replay.ts`
- Test: `web/src/replay.test.ts`

**Interfaces:**

```ts
export type Op = 'create' | 'comment' | 'edit-text' | 'change-labels' | 'set-status' | 'set-title' | 'set-meta';
// Step: kind gains 'meta'; new fields
op: Op;
id: string;               // the operation's id: the event's, history[i].id, meta_history[i].id ('' when the server sent none)
meta?: Record<string, string>; // meta: the keys it set
version?: number;         // the description's created and edits: the version it published, 0 for a tick
// the events as they were once steps[0..i] happened: the events created
// by then; each text's history and body cut to its edits by then (html
// from the last kept history entry); meta rebuilt from the keys no
// set-meta touched plus the meta_history entries by then. Other events pass
// as they are. ponytail: a key both written by the comment and set later
// reads unset before its first set-meta; the comment's own meta from the
// server if that bites
export function asOf(all: Step[], i: number): IssueEvent[]
```

`steps()` pushes a text's `meta_history` entries after its edits, as
steps of their own (kind `meta`, op `set-meta`, `pending` empty); the
existing stable sort keeps a text's entries in log order. `version`
reuses `isTick` (`sections.ts`): an edit whose text differs and is not a
tick is the next version. `stateAfter` is unchanged.

- [ ] **Step 1:** Add to `replay.test.ts`: `steps name the core's
  operations` (create, comment, edit-text, change-labels, set-status,
  set-title, set-meta from one thread, each with its id); `a tick
  publishes no version` (created v1, an edit v2, a tick 0); `an apply's
  edit sorts before its meta` (the body's edit and the root's set-meta
  at the same second: edit first); `asOf before v2 is v1` (cut before
  the edit: `versions(created)` has one, `stateOf` of a v1 thread is
  current); `asOf follows a reopen` (resolve then `""`: resolved between,
  open after, through `bodyThreads`); `asOf drops later comments`.
- [ ] **Step 2:** Run: FAIL. Implement. Run: PASS.
- [ ] **Step 3:** Commit `feat(issue): History's steps are the core's
  operations, and the issue as of any of them`.

### Task 3: the lanes' data and the third tab

**Files:**
- Modify: `web/src/flow.ts` (`Segment.at`, `Reading.tab`, `ONE_OF`),
  `web/src/timeline.ts`
- Test: `web/src/flow.test.ts`, `web/src/timeline.test.ts`

**Interfaces:**

```ts
// flow.ts
export interface Segment { n: number; entries: IssueEvent[]; at?: string } // when the operation that opened it ran; step 1: undefined (the opening)
Reading.tab: 'conversation' | 'history' | 'changes'
// timeline.ts
Marker.lane: 'texts' | 'versions' | 'changes'   // created and the description's edits go to versions
Marker.tick?: boolean                           // a description edit that published no version: hollow
export interface Bar { n: number; left: number; width: number } // a step, from its opening to the next one's, the last to now
// timeline(all, now, field, segs?) gains bars: Bar[] (empty without segs or with step 0)
```

`segments()` records `at` where `running` rises (a move, or a carried
`cycle:step` above it); a segment never opened has no `at` and no bar.

- [ ] **Step 1:** Tests: `segments carry when each step opened`; `a late
  carried comment keeps the bars` (a `cycle:step` 1 comment after the
  move to in-progress: bar 1 ends where bar 4 starts); `readingOf takes
  changes`; `the description goes to the versions lane` (created and its
  edits `versions`, a comment's edit `texts`, a tick `tick: true`); `one
  version is one diamond`; `no cycle, no bars`.
- [ ] **Step 2:** Run: FAIL. Implement. Run: PASS; `npm run typecheck`
  (the tab's union reaches `IssuePanel`).
- [ ] **Step 3:** Commit `feat(issue): the Versions lane, the step bars,
  and a Changes tab in the reading`.

### Task 4: the lanes and the cursor

**Files:**
- Modify: `web/src/components/IssueTimeline.vue`,
  `web/src/components/HistoryView.vue`, `web/src/components/IssuePanel.vue`

**Interfaces:**
- `IssueTimeline` props gain `segments?: Segment[]` (flow.ts). Rows, in
  order: Texts (squares), Versions and ticks (a `◆` diamond per version,
  hollow `◇` per tick, labelled "v2 · Person A · …"), Changes (today's),
  the status band (today's), Steps (a bar per `Bar`, "01 Intent" in mono
  `--gb-done` with a 2 px left border, hidden when there are none). A
  legend line under the axis: "◆ a version · ◇ a tick · ■ a text". The
  cursor: an absolutely placed 2 px `--gb-red-dot` line at the current
  step's `left`, across the lanes' tracks (`aria-hidden`).
- `HistoryView` props gain `cycle?: Cycle`; it passes `segments(events,
  cycle.stages, cycle.step, cycle.skipped)` when there is a cycle.
  *Latest* reads *Now*. The intro line above the lanes: "The same flow,
  with a cursor: pick a moment, read the issue as it was then."
- `IssuePanel` passes `:cycle="cycle"` (the `#flow` slot has it).

- [ ] **Step 1:** Implement. Label column `110px`, `80px` under 720 px.
- [ ] **Step 2:** `npm run typecheck && npm test`: PASS.
- [ ] **Step 3:** Browser, issue A at 1440 and 800 px: five lanes, three
  diamonds for v1–v3 (and a hollow one for a tick, seeded), step bars
  01 and 02 (D), the cursor following prev / next / the range's arrows /
  *Now*; a marker click still selects its change.
- [ ] **Step 4:** Commit `feat(issue): History's Versions and Steps lanes,
  and its cursor`.

### Task 5: the snapshot as of the cursor

**Files:**
- Modify: `web/src/components/HistoryView.vue`, `web/src/sections.ts`
  (`criteriaOf`)
- Test: `web/src/sections.test.ts`

**Interfaces:**

```ts
// the checkbox lines of a text, ticked or not: { n: 4, ticked: 1 }
export function criteriaOf(body: string): { n: number; ticked: number }
```

- The snapshot (`hv-asof`, dashed, "As of … · what the change touched is
  outlined") keeps the status, the title and the fields, the changed
  ones outlined in `--gb-done`, then three boxes from `asOf(all, pos)`:
  - *The description, as of then*: "v2, published 09:30 · 4 criteria,
    none ticked" (`criteriaOf`; "n ticked"), then "threads: §2 open · §3
    resolved by v2 · §4 open": each body thread as of then, its
    `anchorLabel`, then `resolvedBy` when resolved, else "open", "open,
    carried from v1" or "open, written on v1" from `stateOf`.
  - *Texts as of then*: "12 · 10 comments and 2 versions" and "the log
    below is cut at this change".
  - *This change*: who and `whatOf`, and for a field change "status:
    backlog → ready".
  - Today's touched text and "All n texts as of then" stay below.

- [ ] **Step 1:** `criteriaOf` test: `criteria are the checkbox lines`
  (6a's body: 4, 0; one ticked: 4, 1; a fenced `- [ ]` not counted).
  FAIL, implement, PASS.
- [ ] **Step 2:** Implement the boxes. `npm run typecheck && npm test`.
- [ ] **Step 3:** Browser, issue A: the cursor on the created step reads
  "v1", every thread absent; on the comment after v2, "v2" and §3's
  thread "resolved by v2" once Apply's meta step is passed, open before;
  a resolve then reopen (seeded) reads resolved between, open after.
- [ ] **Step 4:** Commit `feat(issue): History's snapshot has the
  description's version and its threads as of then`.

### Task 6: the log, one row per operation, cut at the cursor

**Files:**
- Modify: `web/src/components/HistoryView.vue`

**Interfaces:**
- Above the log: chips *All n · Texts n · Versions and ticks n · Fields n
  · Not on GitHub yet n* (the last only with a forge; `set-meta` counts
  in All only) and the seg *Oldest first | Newest first*: `aria-pressed`
  buttons in component state, not remembered (ruling 4).
- A `<table>` with columns #, Time, Who, Operation (the `op`, mono),
  What (`whatOf`, a `set-meta` row naming its keys: "resolved the thread
  on §3", "set thread:applied"), Op id (`id.slice(0, 7)` + its `:n`,
  the full id as `title`), and, with a forge, "GitHub #37": "not yet"
  while `pending` names it, "local" for a `review:*` set-meta, "—" for
  any other `set-meta` (M5 syncs them), else "on GitHub". Day header
  rows ("Oct 22"). The row's # button moves the cursor.
- The cursor row is `.at` (`--gb-teal-bg`, the red inset bar) and shows
  today's detail under it: the edit's folded diff, the rename's word
  diff, the field changes, the text's excerpt. Rows after the cursor
  fold to one row "n later changes, after the cursor · show", dimmed
  once shown; with *Newest first* the fold is on top.
- A marker click and the range still scroll the cursor row into view.

- [ ] **Step 1:** Implement.
- [ ] **Step 2:** `npm run typecheck && npm test`: PASS.
- [ ] **Step 3:** Browser: issue A lists `create`, `comment`,
  `edit-text`, `set-meta` and `change-labels` rows with op ids; Apply's
  `edit-text` then `set-meta`; the chips' counts add up; *Newest first*
  reverses; the later rows fold and show dimmed; the cursor row has the
  diff; 800 px scrolls the table inside its box, not the page.
- [ ] **Step 4:** Commit `feat(issue): History's log is the core's
  operations, cut at the cursor`.

### Task 7: the Changes tab, the description's versions

**Files:**
- Create: `web/src/components/IssueChanges.vue`
- Modify: `web/src/components/IssuePanel.vue` (the tab, *Changes n*),
  `web/src/sections.ts` (`ticksOf`)
- Test: `web/src/sections.test.ts`

**Interfaces:**

```ts
// the description's ticks: the history edits that only flip checkboxes,
// with the criteria they ticked and unticked (1-based)
export function ticksOf(created: IssueEvent): { at: string; author: Person; ticked: number[]; unticked: number[] }[]
```

- The tab *Changes n*, n = versions + ticks (ruling 1), after History;
  `IssuePanel` renders `IssueChanges` for `reading.tab === 'changes'`
  with the thread, `cycle`, and the props `IssueSections` gets for its
  thread cards.
- Header "The description · 2 versions, 3 ticks" and F5's muted rule
  line. A strip of version cards (`.ver`, → between): "v1 · Oct 21 ·
  Person A", "the original · 4 criteria" for v1, else the sections
  changed against the one before (`changeOf`: "§1 §3 › 3 §4 changed"),
  "n suggested edits applied" (roots whose `thread:applied` is set and
  `thread:resolved` is `v`N), "resolves n threads" (`thread:resolved`
  `v`N); then its verdict (`verdicts`: the last on vN, "approved by
  Person B, 10:40" or "changes requested by Person B, 14:10", else "no
  verdict"); the last card adds "the current text". Then one dashed card
  for the ticks, when any: "ticks · Oct 25 · Person A", "criteria 1, 2
  and 3 ticked", "3 edits in History · not a version".

- [ ] **Step 1:** `ticksOf` test: `ticks list what they flip` (two ticks
  then an untick: `[1]`, `[2]`, then unticked `[1]`). FAIL, implement,
  PASS.
- [ ] **Step 2:** Implement the tab and the cards.
- [ ] **Step 3:** Browser: issue A's cards v1, v2, v3 with their
  sections, applied edits and verdicts; issue B (never edited) one card,
  no compare seg; the tab survives a reload.
- [ ] **Step 4:** Commit `feat(issue): the Changes tab, the description's
  versions and ticks`.

### Task 8: the diff, section by section, with its threads

**Files:**
- Modify: `web/src/components/IssueChanges.vue`

**Interfaces:**
- With two versions or more: the seg *Compare: v1 → v2 | v2 → v3*
  (consecutive pairs, the last pair by default), the seg *Layout: Inline
  | Side by side*, and the checkbox *Threads written on these lines*
  (on). The diff is `renderMarkdown(diffMarkdown(from.body, to.body))`
  cut with `sectionsOf`, behind the latest-request guard `IssueSections`
  uses. Each section: its content, a gutter with "§n Heading" and
  "unchanged" / "changed" / "› 3 changed" (`changeOf`). *Side by side*:
  two columns of the same diff, the left hiding `ins`, the right `del`
  (ruling 3). Under 720 px one column.
- Under a changed section, its threads written on the from-version
  (ruling 2), folded to one line each: "Person B on §2 · "first words…"
  · 2 comments · resolved by v2 [Show]"; *Show* unfolds a `ThreadCard`
  (M2's: a suggestion applied reads "applied in vN").

- [ ] **Step 1:** Implement.
- [ ] **Step 2:** `npm run typecheck && npm test`: PASS.
- [ ] **Step 3:** Browser: issue A v1 → v2 marks §3's changed item, its
  thread folded "resolved by v2", unfolded the suggestion "applied in
  v2"; v2 → v3 shows only that pair's change; *Side by side* shows the
  before and the after; switching pairs fast paints the last clicked.
- [ ] **Step 4:** Commit `feat(issue): Changes diffs two versions, with the
  threads written on them`.

### Task 9: the code part

**Files:**
- Modify: `web/src/components/IssueChanges.vue`

**Interfaces:**
- Under the description: "The code · `branch` · PR #41, merged" (the PR
  a link, its state lower case), "3 commits · 5 files · +135 −14"; a
  commits table (Commit, Message, Who, When) from `getChanges(id,
  {branch})`; "Files" with +/− per file; "The plan": its path, its
  status, "by Person A", "approved by Person B, Oct 22" from
  `plan_approval`. No branch: "No branch yet." No plan: nothing. Read
  only: no `CycleCode` actions. A failed fetch shows its message.

- [ ] **Step 1:** Implement.
- [ ] **Step 2:** `npm run typecheck && npm test`: PASS.
- [ ] **Step 3:** Browser: an issue with a branch on the fixture repo
  lists its commits and files; issue A says "No branch yet."
- [ ] **Step 4:** Commit `feat(issue): Changes ends with the code: branch,
  commits, PR, files, plan`.

### Task 10: verification

- [ ] **Step 1:** `npm run typecheck && npm test`, `go test ./internal/...`
  for the touched packages.
- [ ] **Step 2:** Playwright at 1440, 1100 and 800 px on History and
  Changes: 6d's four criteria and 6e's three; the scrubber, the arrows
  and *Now* as before; M2's and M3's checks still hold on Conversation.
- [ ] **Step 3:** Fix, commit. `make ci` waits for M5.

## Done when

The PRD's M4 list:
- 6d's and 6e's criteria hold;
- the scrubber, the arrows and *Now* behave as today.

## Rulings

This plan's calls (the user asked for continuity: no approval stop).

1. **Changes n counts the versions and the ticks.** The README leaves it
   open; F4's "Changes 5" is F5's "2 versions, 3 ticks".
2. **"Threads anchored on changed lines" are those written on the
   compare's from-version, in the sections it changed.** M2's anchors are
   sections (its ruling 4), so a section stands for its lines.
3. **Side by side is built**, as one diff in two columns with CSS hiding
   `ins` on the left and `del` on the right: no second render. *Compare
   plan v1…v2* and the per-commit CI pill are not: plans have no
   versions yet (open point 5) and `Changes` has no per-commit checks.
4. **The log's chips and order are the component's**, not the
   remembered reading: they belong to a visit of History.
5. **The lanes are today's plus two**: Texts, Versions and ticks,
   Changes, the status band, Steps. The README's criterion wins over its
   "four lanes" prose and F4's sample.
6. **Today's diffs stay in History**, under the cursor row: a comment's
   edits have no other home.
7. **A `set-meta` row's forge state is "local" for `review:*`, "—"
   otherwise** until M5 syncs the keys.

## Self-review notes

- Spec coverage: lanes ✓ (3, 4); the cursor and *Now* ✓ (4); the
  snapshot's version, threads, fields ✓ (2, 5); the log's operations
  with forge state ✓ (1, 2, 6); the versions with who, when, sections,
  verdict, ticks apart ✓ (7); the diff's threads folded, *applied in vN*
  ✓ (8); the code part ✓ (9); the tab ✓ (3, 7).
- `lanes.ts` and `history.ts` in the README's components row are
  History's lanes and replay: here `timeline.ts` and `replay.ts`, which
  already hold them.

wtx-effort: high
