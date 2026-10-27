# Time flow M3: versions and the verdict, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The description has versions: a switcher (v1 | v2) with
*Changes since v*N-1, each gutter saying whether its section changed;
a thread on an older version reads carried, outdated or resolved; the
verdict bar takes one verdict per version, *Approve v*N moving the status
to ready; the flow's *On v*N filter and the "published vN" milestone; the
Spec block in the sidebar. Screen 6b, and 6a's verdict bar.

**Architecture:** The rules stay pure and client-side, beside M2's:
`sections.ts` gains the versions of the body, a section's change, a
thread's state and the Markdown diff; a new `spec.ts` reads the verdicts.
The server sends each version's HTML, learns a `[cycle] spec` gate role,
and gains one cycle action, `spec-verdict`, shaped as `approve-plan`: one
comment with `spec:verdict` and `spec:on`, then, on *Approve* of the
current version before ready, the status move through `fields.Apply`, the
path `PATCH /api/issues/{id}` (*Move to ready*) takes. New components:
`VerdictBar.vue`; the switcher lives in `IssueSections.vue`. The core is
untouched.

**Tech Stack:** Vue 3, `node:test`; Go (`net/http`, `internal/cycle`).

**Spec:** `docs/prd/time-flow.md`, "Decisions" (the verdict is on the
version), "The protocol" (`spec:verdict`, `spec:on`), open points 1 and 2,
"M3: versions and the verdict"; `docs/design/time-flow/README.md`, 6b's
five criteria, 6a's verdict bar criterion, "States and interactions";
`docs/design/time-flow/protocol.md`, "Versions and ticks", the thread
states; `screens/F2-spec-v2.html` for the markup and the copy.

**Base:** `feat/time-flow-m2` at `39c2a12` (M2 done). M3, M4 and M5 go on
this branch with no landing between them, and one `make ci` after the
last (the user, 2026-10-27).

## What exists today, found in the code

**Versions.** `version(created)` (`sections.ts:72`) is 1 + each history
edit that changes the text and is not a tick (`isTick`, the `- [ ]` ↔
`- [x]` flips); an edit leaving the text identical counts for nothing.
`history` is `[{id, author, at, body}]`, oldest first, the original
first, and is `null` when the text was never edited
(`internal/store/thread.go:158-199`). No version has its HTML: only each
event's current text is rendered (`internal/server/issues.go:62`).

**Move to ready.** Step 1 waits for the body's author: "add acceptance
criteria", then, with criteria and `Stages.Ready`, "move the status to
ready" with the act `move-ready` (`internal/cycle/cycle.go:195,256-260`).
The button is the client's `PATCH {status: stages.ready}` (`cycle.ts`
`moves`, `usecycle.ts` `patch`), which the server applies with
`fields.Apply` (`internal/server/server.go:462`). `approve-plan` is the
precedent for a gate that writes a comment then moves the status, in one
call, behind `waitsFor` (`internal/server/cycle.go:214-320`).

**Gate roles.** `[cycle] risk`, `plan`, `review` name roles of
`[roles]`; `config.Stages.Roles` maps them (`internal/config/cycle.go:201`),
`checkCycle` checks the names (`:246`), and `gather` resolves each to the
login holding it on the issue (`internal/cycle/gather.go:67`), sent as
`evidence.people`.

**The flow's filters.** `Reading.show` is `step | all | open`
(`flow.ts:76`, `ONE_OF`); `pick` keeps the entries; `IssueFlow`'s chips
count them and a pointer says what *This step* hides. A body edit is no
event of its own: the flow has no "published vN" line today.

**Markdown.** `POST /api/markdown` renders `{body}` to `{html}`
(`renderMarkdown` in `board.ts`); raw HTML is kept, then cleaned by
bluemonday's UGC policy, which keeps `<ins>` and `<del>`.

## Global Constraints

- No new entity or operation kind; the core gives no key a meaning. M3
  writes `spec:verdict` (`approve` | `request-changes`) and `spec:on`
  (`body@v`N) on one `comment`, with `cycle:step` `1`. Versions, a
  section's change and a thread's state are computed, never stored. The
  step is never typed.
- A thread's state is computed against the current version, never the
  version the switcher shows.
- A tick is not a version, and a tick never makes a section "changed",
  a thread outdated, or a line of the diff.
- With `[cycle] spec` unset, step 1 behaves as today: the author has
  *Move to ready*; the verdict bar offers *Approve* to anyone but the
  body's author.
- History's scrubber keeps today's behaviour; `review:*` stays local.
- The copy is F2's, with Person A, B, C in fixtures and tests; logins
  alice, bob, carol. EPFL tokens (`var(--…)`), not hex: the amber chip
  and notice on the warning tokens `epfl-alert-warning` uses.
- No Vue component tests exist: every component task is checked in a
  browser on the fixture board (`$TMPDIR/tf2`, M2's seed extended). The
  server does not reload the toml: restart `serve` after editing it.
- While developing: `cd web && npx tsx --test src/sections.test.ts
  src/spec.test.ts src/flow.test.ts`, `go test ./internal/server/
  ./internal/cycle/ ./internal/config/ -run …`; `cd web && npm run
  typecheck && npm test` before each commit. `make ci` once, after the
  last milestone. A new Go test calls `t.Parallel()` first.
- Commit on `feat/time-flow-m2`; no push, no tag, no attribution
  trailer, no new dependency. Shortcuts carry a `ponytail:` comment.

## Review Focus

1. A heading renamed in v2: v1's thread on `expected` names a slug v2
   lacks. It is outdated ("§ expected changed in v2"), never carried, and
   it still shows in the flow. (Task 1: `a gone slug is outdated`.)
2. A tick between v1 and v2 inside a section: the section is unchanged,
   its v1 thread carried, the diff has no line for it. (Task 1: `a tick
   is unchanged`, `a tick is no diff line`.)
3. Approve while a v3 was published meanwhile: refused with 409, nothing
   written, the status unchanged. (Task 3: `a stale verdict is
   refused`.)
4. With `[cycle] spec` unset, the body's author cannot approve their own
   text, and still has *Move to ready*. (Task 2: `step 1 without spec is
   today's`; Task 3: `the author cannot approve`.)
5. Switching v1 ↔ v2 fast with *Changes since* on: the diff of the
   version clicked last is the one painted. (Task 5: a latest-request
   guard, checked in the browser.)

---

### Task 0: the plan in the repository

- [ ] **Step 1:** This file, `docs/prd/time-flow-m3-plan.md`.
- [ ] **Step 2:** Commit `docs(prd): the time flow's M3 plan`.

### Task 1: versions, changes, thread states and the diff

**Files:**
- Modify: `web/src/sections.ts`, `web/src/api/board.ts` (`Version.html`)
- Test: `web/src/sections.test.ts`

**Interfaces:**

```ts
// board.ts: Version gains html?: string (Task 2 sends it for the description)
export interface BodyVersion { n: number; id: string; author: Person; at: string; body: string; html: string; ticks: number }
// the body's versions, v1 first: vK's text is its last entry before
// v(K+1), its ticks included; the last is the current text. No history:
// [v1] from the created event itself
export function versions(created: IssueEvent): BodyVersion[]
// a section of `after` against `before`, by slug, ticks not counted:
// 'unchanged', 'changed', or the item number when the two have the same
// items and exactly one differs (the gutter's "› 3 changed in v2"); a
// slug `before` lacks is 'changed'
export function changeOf(before: Section[], after: Section): 'unchanged' | 'changed' | number
// a body thread's state against the current version (protocol.md):
// resolved (thread:resolved set), current (written on it), carried (its
// section unchanged since), outdated (changed, or gone); `since` is the
// first version after the anchor's in which the section differs
export type ThreadState = { state: 'resolved' | 'current' | 'carried' } | { state: 'outdated'; since: number }
export function stateOf(root: IssueEvent, vs: BodyVersion[]): ThreadState
// the Markdown of `after` with the lines removed since `before` struck
// (<del>) and the added ones marked (<ins>), inside their quote, list
// and checkbox markers; ticks compared away; a heading changed shows as
// the new heading, a removed heading line as struck text
export function diffMarkdown(before: string, after: string): string
```

Section texts compare on the HTML with the checkboxes' `checked`
stripped. `diffMarkdown` is a line LCS over the tick-normalized lines,
emitting the new lines (ponytail: O(n·m) and line-level; a fence or a
table line is marked as plain text inside the block; word diffs if the
line ones read badly).

- [ ] **Step 1:** Tests: `versions counts no tick` (history original,
  tick, edit → v1 holds the ticked text, v2 the edit, `ticks` 1 on v1);
  `versions without history is v1`; `a tick is unchanged`; `one item
  changed names it` (§3's item 3 → `3`); `a gone slug is outdated`;
  `a resolved thread is resolved` (with `thread:resolved` `v2`);
  `written on the current version is current`; `carried from v1`
  (§2 unchanged in v2); `outdated since v2` (§4 changed); `diff marks
  added and removed lines` (`- <del>old</del>` / `- <ins>new</ins>`,
  a quote kept quoted); `a tick is no diff line`; `a changed heading is
  the new one, plain`.
- [ ] **Step 2:** Run them: FAIL. Implement. Run: PASS.
- [ ] **Step 3:** Commit `feat(issue): the body's versions, each
  section's change and a thread's state`.

### Task 2: the server: versions' HTML and the spec role

**Files:**
- Modify: `internal/server/issues.go` (`writeThread`),
  `internal/config/cycle.go` (`Cycle.Spec`, `Roles`, `checkCycle`),
  `internal/cycle/gather.go` (`People["spec"]`), `internal/cycle/cycle.go`
  (step 1), `docs/guide/configuration.md` (`[cycle] spec`)
- Test: `internal/server/issues_test.go`, `internal/config/cycle_test.go`,
  `internal/cycle/cycle_test.go`

**Interfaces:**
- `writeThread` renders each `History[i].HTML` of the created event
  (`Version` gains `HTML string json:"html,omitempty"`).
- `[cycle] spec = "pm"`: the role whose holder gives the verdict;
  `Stages.Roles["spec"]`; `People["spec"]` the login.
- Step 1 with criteria, `Stages.Ready` and `People["spec"]` set: `Waits{Role:
  "spec", Login: People["spec"], What: "give the verdict on the
  description"}`, act `spec-verdict`. Without `People["spec"]`: as today.

- [ ] **Step 1:** Tests: `TestThreadVersionsHaveHTML` (two edits → each
  history entry's `html` rendered); `TestCycleSpecRole` (config: the key
  read, a bad name refused, `Roles["spec"]`); `TestStepOneWaitsForSpec`
  and `TestStepOneWithoutSpecIsToday` (in `cycle_test.go`: the waits,
  the act, `Mine` for bob and not for alice; unset: the author,
  `move-ready`).
- [ ] **Step 2:** FAIL, implement, PASS.
- [ ] **Step 3:** Commit `feat(cycle): a spec role gives step 1's
  verdict, versions carry their HTML`.

### Task 3: the spec-verdict action

**Files:**
- Modify: `internal/server/cycle.go` (`handleCycleAction`)
- Test: `internal/server/cycle_test.go`

**Interfaces:**
- `POST /api/issues/{id}/cycle/spec-verdict` with `{verdict, on, body}`:
  `verdict` `approve` | `request-changes`; `on` `body@v`N, which must be
  the current version (the client's rule, `bodyVersion` in Go: 1 + the
  history edits that change the text past a tick), else 409 "v%d is
  published: give the verdict on it"; `body` required for
  `request-changes`, defaulting to "Approved vN of the description"
  for `approve`. The viewer: with `People["spec"]`, `waitsFor("spec",
  …)` (403); without, not the body's author (403 "the description's
  author does not approve it"). `approve` without criteria: 400 "add
  acceptance criteria first" (adding them is an edit, a new version
  to approve). Writes one comment, `spec:verdict`, `spec:on`,
  `cycle:step` `1`; then, on `approve` while the stage is before ready
  and `Stages.Ready` is set, `fields.Apply(status, Stages.Ready)`.
  Answers the cycle, as the other actions.

- [ ] **Step 1:** Tests (parallel), `TestSpecVerdict`: bob's approve
  of v1 → the comment's meta and the status `ready`; `request-changes`
  without body → 400; with → no move; `a stale verdict is refused`
  (on v1 after an edit → 409, nothing written); `the author cannot
  approve` (spec unset, alice → 403; carol → 200); `the spec holder
  only` (spec `pm` = bob, carol → 403); `without criteria` → 400;
  `approve after ready moves nothing` (status `in-progress` stays).
- [ ] **Step 2:** FAIL, implement, PASS; `go test ./internal/server/`
  green.
- [ ] **Step 3:** Commit `feat(server): the description's verdict, one
  comment, Approve moving the status to ready`.

### Task 4: the client's verdict rules and call

**Files:**
- Create: `web/src/spec.ts`, `web/src/spec.test.ts` (in `package.json`'s
  list)
- Modify: `web/src/api/board.ts` (`specVerdict`), `web/src/cycle.ts`
  (`cardFor` leaves `spec-verdict` to the bar)

**Interfaces:**

```ts
export interface Verdict { verdict: 'approve' | 'request-changes'; v: number; event: IssueEvent }
export const verdicts = (events: IssueEvent[]): Verdict[]          // spec:verdict comments, in time order
export interface SpecState {
  v: number;                    // the current version
  on?: Verdict;                 // the last verdict on it
  before?: Verdict;             // the last verdict on an earlier version
  approved?: Verdict;           // the last approval of an earlier version ("approved v2, v3 awaits")
  open: number;                 // body threads open
  resolvedBy: number;           // threads resolved by vN (thread:resolved "vN")
}
export function specOf(events: IssueEvent[], vs: BodyVersion[]): SpecState
// who may give it: the spec holder when the role is set, else anyone
// but the body's author
export function mayGive(people: Record<string, string> | undefined, author: string | undefined, me: string | undefined): boolean
// board.ts
export const specVerdict = (id: string, verdict: string, on: string, body: string) => cycleAction(id, 'spec-verdict', { verdict, on, body })
```

- [ ] **Step 1:** Tests: `the last verdict on vN`; `a verdict on v1 is
  before`; `approved v2, v3 awaits`; `mayGive` both branches; `cardFor`
  returns null on `spec-verdict` alone.
- [ ] **Step 2:** FAIL, implement, PASS.
- [ ] **Step 3:** Commit `feat(issue): the verdicts read, and posted`.

### Task 5: the version switcher and Changes since

**Files:**
- Modify: `web/src/components/IssueSections.vue`,
  `web/src/components/IssuePanel.vue`

**Interfaces:**
- `IssuePanel` holds `shownV` (the version shown; reset to the current
  one when the issue or its version changes; not remembered) and passes
  it to `IssueSections` and `IssueFlow` (Task 8).
- The description card's head, with two versions or more: the seg
  `role="group" aria-label="Version"` of `v1 … vN` buttons with
  `aria-pressed`, and the checkbox "Changes since v*N-1*" (hidden on v1).
  vK renders its `html`; with *Changes since*, `renderMarkdown(diffMarkdown(
  v(K-1).body, vK.body))`, the plain vK shown until it answers; a
  counter keeps the last request's answer only (Review Focus 5).
- Each gutter: "changed in vK" / "unchanged" / "› i changed in vK"
  (`changeOf` against v(K-1); nothing on v1). The anchor of a comment
  posted from a gutter names the version shown.
- The `.ins` / `.dels` look: `ins` on `--gb-green-bg/fg`, no underline;
  `del` on `--gb-red-bg/fg`, struck.

- [ ] **Step 1:** Implement. `npm run typecheck && npm test`.
- [ ] **Step 2:** Browser on A with v2: the seg, v1 renders v1, *Changes
  since v1* marks §1 §3 §4, §2 says unchanged; a tick on v2 is no version
  (the seg keeps two); fast clicks paint the last.
- [ ] **Step 3:** Commit `feat(issue): the description's versions, with
  Changes since`.

### Task 6: carried, outdated, resolved

**Files:**
- Modify: `web/src/components/IssueSections.vue` (`chip`, `threadsIn`),
  `web/src/components/ThreadCard.vue`, `web/src/components/CommentCard.vue`
  (the header's "· on §4 · v1")

**Interfaces:**
- `threadsIn(s)` stays by slug (any version) and drops M2's ponytail.
  `chip(s)`, folded: "1 comment · open · carried from v1 ▸", "1 comment ·
  open · written on v1 ▾" with `is-warn` (amber) when outdated, "3 comments
  · resolved by v2 ▸" (M2's).
- A section whose threads are all resolved shows, unfolded, the folded
  line "Thread on §3 › 3 · Person B, Person A · 3 comments · **resolved by
  v2**" (`: the suggested edit of Person B was applied` when
  `thread:applied`) with *Show*, which opens the cards.
- `ThreadCard` on an outdated thread: the notice row (warning tokens)
  "Written on v1. §4 changed in v2, so this thread may be answered by the
  text itself; it stays open until someone resolves it." The root's header
  reads "· on §4 · v1" when the anchor's version is not the current one.
- A gone slug's threads (Review Focus 1) show in the flow as M2's do.

- [ ] **Step 1:** Implement, typecheck and test.
- [ ] **Step 2:** Browser: §2 carried, §3 resolved line with *Show*, §4
  amber with the notice; Resolve on §4 greys it.
- [ ] **Step 3:** Commit `feat(issue): a thread on an older version reads
  carried, outdated or resolved`.

### Task 7: the verdict bar

**Files:**
- Create: `web/src/components/VerdictBar.vue`
- Modify: `web/src/components/IssuePanel.vue` (`#intent`, after the
  description's card), `web/src/cycle.ts` (`steps`: step 1's line)

**Interfaces:**
- Props: `state: SpecState`, `reviewer` ("Person B, PM", or "Anyone but
  Person A" with no spec role), `may: boolean`, `moves: boolean` (the
  status is before ready and criteria exist), `criteria: boolean`, `busy`,
  `give(verdict, body): Promise<boolean>`.
- The screen's bar: eyebrow "Spec · v2 awaits a verdict" (or "Spec ·
  changes requested on v2", "Spec · approved v2 by Person B, Oct 22
  10:00", "Spec · approved v2, v3 awaits"); "Person B, PM, reviews v2";
  the muted line: the verdict before ("v1: changes requested by Person
  B, Oct 21 14:10."), "v2 resolved 1 thread; 2 are open.", and
  "Approving moves the issue to ready and opens step 2." when `moves`;
  [Request changes] [Approve v2] while `may` and no approval on vN.
  Request changes opens a textarea (required, labelled) with Send. No
  criteria: Approve disabled, "Add acceptance criteria first." Not
  `may`: no buttons, the line says who reviews.
- Shown with the description unfolded and two conditions: the issue has
  a cycle, and the status is before ready or a verdict exists.
- The rail's step 1 line while current: "v2 · awaits the verdict".

- [ ] **Step 1:** Implement, typecheck and test.
- [ ] **Step 2:** Browser, fixture with `[cycle] spec = "pm"` (restart
  serve): as alice no buttons, as carol none, as bob Request changes
  then Approve v2; the status goes to ready, step 2 opens; an edit then
  reads "approved v2, v3 awaits" with the status unchanged. Spec unset:
  alice keeps Move to ready, carol approves.
- [ ] **Step 3:** Commit `feat(issue): the verdict bar, one verdict per
  version`.

### Task 8: On vN and the published milestone

**Files:**
- Modify: `web/src/flow.ts` (`Reading.show`, `ONE_OF`, `pick`),
  `web/src/components/IssueFlow.vue`, `web/src/api/board.ts`
  (`IssueEvent.kind` gains `'version'`)
- Test: `web/src/flow.test.ts`

**Interfaces:**
- `show: 'step' | 'all' | 'version' | 'open'`; `pick(…, on?: { v: number;
  from: string; to?: string })` keeps, under `'version'`, the entries on
  vN: an anchor or `spec:on` naming it, else written while it was current
  (`from` ≤ at < `to`), and its milestone. With one version the chip is
  hidden and `'version'` reads as `'all'`.
- `IssueFlow` adds the milestones to its events, one per version past
  v1: `{kind: 'version', id: v.id, at, author, …}`, through `segments`,
  so each sorts into its step; `TimelineEvent` is not given them, the
  flow draws "Person A published v2 of the description · Oct 22 09:30"
  and "§1 §3 › 3 §4 changed · applied 1 suggested edit of Person B ·
  resolved 1 thread · Compare v1…v2"; *Compare* sets `shownV` and
  *Changes since*, and scrolls to the description. *Milestones* keeps them.
- The chip "On v2 n" between All and Open threads; the pointer "7 entries
  on v1 are hidden · show all".

- [ ] **Step 1:** Tests: `On vN keeps its entries and milestone`,
  `one version reads as all`, `readingOf takes version`.
- [ ] **Step 2:** FAIL, implement, PASS; browser: the chip, the pointer,
  the milestone in step 1's segment, *Compare*.
- [ ] **Step 3:** Commit `feat(issue): the flow's On vN filter and each
  version's milestone`.

### Task 9: the Spec block

**Files:**
- Modify: `web/src/components/IssueSide.vue`,
  `web/src/components/IssuePanel.vue`

**Interfaces:**
- A section labelled "Spec" under the fields, with a cycle and a body:
  "v2 · awaits the verdict of **Person B** PM" (or "v2 · approved by
  Person B, Oct 22", "v2 · changes requested"); muted "v1: changes
  requested, Oct 21 · 2 threads open". Props: `spec?: SpecState`,
  `reviewer?: string`.

- [ ] **Step 1:** Implement; browser at 1440 and 800 px.
- [ ] **Step 2:** Commit `feat(issue): the sidebar's Spec block`.

### Task 10: verification

- [ ] **Step 1:** `npm run typecheck && npm test`, `go test ./internal/...`
  for the packages touched.
- [ ] **Step 2:** Playwright at 1440, 1100 and 800 px: 6b's five
  criteria, 6a's verdict bar, the Done-when list, the Review Focus;
  M2's checks still hold (anchors, Apply, the seg).
- [ ] **Step 3:** Fix, commit. `make ci` waits for the last milestone.

## Done when

The PRD's M3 list:
- 6b's five criteria hold, and 6a's verdict bar;
- a tick is not a version;
- Approve vN moves the status to ready by the path of Move to ready
  (`fields.Apply`), and a later version shows "approved vN, vN+1 awaits"
  (open point 2).

## Rulings

This plan's calls, standing unless review says otherwise (the user asked
for continuity: no approval stop).

1. **The verdict is a cycle action.** `spec-verdict` writes the comment
   and moves the status in one call, as `approve-plan` does, through
   `fields.Apply`, which is what `PATCH` (Move to ready) calls. Not two
   client calls: the gate is checked where the status moves.
2. **`[cycle] spec`, as open point 1 settled.** Set, step 1 waits for its
   holder ("give the verdict on the description", act `spec-verdict`) and
   Move to ready is the verdict's. Unset, step 1 is today's and anyone but
   the body's author may give the verdict.
3. **No Approve without criteria.** Adding the criteria is an edit, so a
   new version: an approval before them would never move the status.
   Request changes stays open.
4. **The server checks `on`.** It counts the version as the client does
   (a tick is none) and refuses a verdict on an older one.
5. **On vN follows the switcher**, whose state is the panel's and is not
   remembered; the filter is a fourth `show`.
6. **The milestone is the client's**, a synthesized `version` entry
   through `segments`; no server event.
7. **The thread's version shows on its root only**: a reply copies the
   root's anchor (M2), so its own version is not known.

## Self-review notes

- Spec coverage: versions, ticks ✓ (1, 2); the switcher, Changes since ✓
  (5); carried / outdated / resolved, the notice ✓ (6); the verdict bar,
  its states, the status move, "approved v2, v3 awaits" ✓ (3, 7); On vN,
  the milestone, the pointer ✓ (8); the Spec block ✓ (9); the rail's line
  ✓ (7).
- Not in M3: History's Versions lane and the snapshot (M4); the bold
  first lines on GitHub, "**Approved v2 of the description**" and
  "**Published v2**" (M5).

wtx-effort: high
