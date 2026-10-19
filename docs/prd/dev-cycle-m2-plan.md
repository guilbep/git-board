# Dev cycle M2: the Cycle view, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The issue page gets an Issue/Cycle switch; the Cycle view is the
spine of the seven steps with one line of evidence each, the workspace for
the current step (the Plan workspace at step 3, the Verify workspace at
step 7), and a status bar along the bottom, as the canvas artboards E1, E3
and E4 draw them.

**Architecture:** Everything the view shows is already in the step answer
(`GET /api/issues/{id}/cycle`, M1) or in the thread: M2 adds the words
(a pure `spine()` in `web/src/cycle.ts`) and the components
(`IssueCycleView.vue` with `CyclePlan.vue` and `CycleVerify.vue`). The
server grows three small things: the plan file rendered to HTML and who
drafted it (`cycle.Plan.HTML`, `cycle.Plan.By`), the merged head from the
PR (`ghpr.PR.MergeCommit`) so a sign-off's run check accepts a run at the
merge commit, and the guessed stages in `GET /api/settings`. No new
endpoint, no new dependency, no stored state: the view is a function of
the thread and the step.

**Tech Stack:** Go (`internal/cycle`, `internal/ghpr`, `internal/server`,
`internal/markdown`), Vue 3 + Quasar (`web/src`), `node:test` for the
pure TS, `vue-tsc` for the components.

**Spec:** [dev-cycle.md](dev-cycle.md), "M2, the Cycle view" and the
calls left to M2 in "Calls taken"; the canvas artboards E1 (Plan), E3
(Verify), E4 (Issue view).

**Base:** `main` at e6bc428 (M1 and the H1 header), on the branch
`prd/dev-cycle`.

## Global Constraints

- The Issue view stays today's page: the progress line gains E4's ticks,
  nothing else moves.
- No real person's name in fixtures: Person A/B/C in copy, logins
  alice/bob/carol/dan/erin, identity `dev` in server tests.
- Canvas tokens: canard `#007480` for done and the gate, `--red-dark`
  primary buttons (the `epfl-btn-primary` class), eyebrow 12px uppercase
  `.12em`, the spine grid `30px minmax(0, 1fr) auto`, `aria-current="step"`
  on the current spine item, `aria-pressed` on the Issue/Cycle segment.
- The spine folds to the progress line under 900px (`56.25rem`): one new
  media query, the two existing ones (`63.99rem`, `47.99rem`) stay.
- The Cycle view fetches nothing the Issue view doesn't: the step answer
  and the thread. The status bar reads `evidence` and the thread's
  pending events. No worktree, no agent: nothing on the server knows
  them (said in the PRD's calls taken).
- `GOCACHE=$TMPDIR/go-build` for Go tests in the sandbox; server tests
  bind `httptest` ports. `make ci` and `cd web && npm run typecheck &&
  npm test` before every commit. Commit on `prd/dev-cycle` (SSH signing),
  never push.
- No new dependency (CONTRIBUTING.md).

## Review Focus

1. A thread whose step answer has no evidence at all (a fresh issue in
   backlog): every spine row shows its gate text, the workspace is the
   gate and nothing crashes on `undefined`. (Task 1: `the spine with no
   evidence`.)
2. A sign-off's run posted at the merge commit after the PR merged, when
   the branch head differs from the merge commit: the sign-off is
   accepted without a reason. (Task 2: `TestCycleSignoffAtTheMergedHead`.)
3. A plan file with no front matter author on the base branch (delivered):
   `plan.by` is empty and the Plan workspace omits "Drafted by". (Task 3:
   `TestGatherFromRepo`'s delivered row; Task 5's template guard.)
4. Statuses with two validation stages and one signed off before the
   current one: tiles read done / Now / Next in the configured order, and
   "Sign off <status>" names the current status. (Task 1:
   `the sign-off tiles`.)
5. The Issue/Cycle segment on the dialog at 900px and the page at 1440px:
   the page's Cycle view takes the window's width, the dialog keeps
   `min(64rem, 100vw)`, and `Esc`/j/k keep working on the dialog. (Task
   4's browser check in Task 8.)

---

### Task 1: the words for the spine, the tiles and the status bar

**Files:**
- Modify: `web/src/api/board.ts` (`IssueEvent.meta`, `Cycle.evidence`)
- Modify: `web/src/cycle.ts`
- Test: `web/src/cycle.test.ts`

**Interfaces:**
- Consumes: `Cycle` from `board.ts` (M1), extended here with what Go
  already sends:

```ts
export interface IssueEvent { …; meta?: Record<string, string> }
export interface CyclePlan { path: string; ref: string; status?: string; html?: string; by?: string }
export interface CycleSignoff { status: string; by: string; commit?: string; why?: string; at: string }
export interface Cycle {
  …;
  evidence: {
    criteria?: string[]; risk?: string; author?: string; assignees?: string[];
    people?: Record<string, string>;
    branch?: string; head?: string; base?: string;
    plan_branch?: string; plan_head?: string; plan?: CyclePlan;
    plan_approval?: { by: string; commit?: string; at: string }; plan_pr?: GatePR;
    tests?: { path: string; commit?: string }[];
    pr?: GatePR; signoffs?: CycleSignoff[];
    runs?: { by: string; commit?: string; at: string }[];
    checks?: { index: number; by: string; commit?: string; at: string }[];
  };
}
```

- Produces, in `web/src/cycle.ts`:

```ts
export interface SpineRow { n: number; name: string; state: 'done' | 'current' | 'todo'; line: string }
export function spine(c: Cycle): SpineRow[]            // seven rows
export interface Tile { status: string; state: 'done' | 'current' | 'next'; by?: string; when: string }
export function tiles(c: Cycle, status: string): Tile[] // one per stages.validation
export function statusBar(c: Cycle, waiting: number): string[] // the bottom strip's segments
```

The seven names are the server's (Intent, Risk class, Plan, Tests first,
Code, Review, Verify). A row is `done` under `c.step`, `current` at it,
`todo` above it (every row is done when `c.done`). The line, E1 and E3
verbatim:

| n | done / current | todo |
|---|---|---|
| 1 | `${criteria.length} acceptance criteria` (`no acceptance criteria` when none) | gate |
| 2 | `${Risk}` + `, set by ${riskBy}` when known (the thread's last label change adding `risk:`, its author); `Skipped, low risk` when `skipped` has 2 | gate |
| 3 | `Approved by ${plan_approval.by}`; else `Plan PR #${plan_pr.number}, waits for approval`; else `${plan.path}` when there is a plan file; `Skipped, low risk` when skipped | gate |
| 4 | `${tests[0].path} at ${commit.slice(0,7)}`, `Failed on …` is not known: `Added at ${commit}` when the path is long is wrong, keep the path | gate |
| 5 | `Committed by ${pr.author}` when a PR; else `${branch}` at `${head.slice(0,7)}` | gate |
| 6 | `${approvers} approved` from `pr.reviews` with state APPROVED (two names joined with " and "); `Merged` when `pr.state === 'MERGED'` and no approver; `PR #${n} open` otherwise | gate |
| 7 | `Sign-off ${k} of ${n}` with k = `signoffs.length`, n = `stages.validation.length`; `Done` when `c.done` | gate |

When the evidence a row needs is missing on a done or current row, the
row shows the gate text. The row's state `current` also carries
`c.note` after the line (" · plan not approved") so the spine says what
the progress line says.

`tiles`: one per `stages.validation` in order; `done` when a sign-off
with that status exists (`by`, `when: on(at)` from `@/me`); `current`
when it is `status` (`when: 'Now'`); `next` otherwise (`when: 'Next'`).

`statusBar`: `[branch at head7, "PR #n · checks: k of m green" | "PR #n merged" | "no PR", "waits for N changes on the forges" | "in sync"]`, each segment omitted when unknown (no branch → "no branch yet").

- [ ] **Step 1:** Write the failing tests in `web/src/cycle.test.ts`,
  extending the `at()` helper's evidence:

```ts
test('the spine, one line of evidence per step', () => {
  const rows = spine(at({ step: 3, name: 'Plan', gate: 'Structural: plan PR approved first', note: 'plan not approved', evidence: {
    criteria: ['a', 'b', 'c', 'd'], risk: 'structural', people: { risk: 'erin' }, plan_pr: { number: 3089 } as GatePR } }));
  assert.deepEqual(rows.map((r) => [r.n, r.state, r.line]), [
    [1, 'done', '4 acceptance criteria'], [2, 'done', 'Structural, set by erin'],
    [3, 'current', 'Plan PR #3089, waits for approval · plan not approved'],
    [4, 'todo', 'Every bug fix ships its test'], [5, 'todo', 'A human commits and opens the PR'],
    [6, 'todo', 'Structural: a second human'], [7, 'todo', 'Green codes are not proof'],
  ]);
});
test('the spine with no evidence', …)  // step 1, evidence {}: seven rows, row 1 current 'no acceptance criteria', the rest the gates
test('the spine at verify', …)         // E3: 'Approved by erin', 'api/a_test.go at 2c8e4a1', 'Committed by dan', 'erin and alice approved', 'Sign-off 2 of 3'
test('the sign-off tiles', …)          // validation [ready-for-validation, qa, tests-metier], one sign-off → [done alice 'Signed off 18 Oct', current 'Now', next 'Next']
test('the status bar', …)              // branch+head, PR checks, waiting count; and 'no branch yet'
```

The todo rows' gate texts are the server's `Gate` strings per step: the
test states them as the spec does (E1); `spine` takes them from a
`GATES` constant in `cycle.ts` mirrored from `internal/cycle/cycle.go`
(the server only sends the current step's gate).

- [ ] **Step 2:** Run: `cd web && npx tsx --test src/cycle.test.ts`.
  Expected: FAIL, `spine` is not exported.
- [ ] **Step 3:** Implement the three functions and the types.
- [ ] **Step 4:** Run: `cd web && npm run typecheck && npm test`.
  Expected: PASS.
- [ ] **Step 5:** Commit `feat(cycle): the words for the spine, the
  sign-off tiles and the status bar`.

### Task 2: the merged head for the sign-off's run check

**Files:**
- Modify: `internal/ghpr/ghpr.go` (`PR.MergeCommit`, `findQuery`,
  `prNode.MergeCommit`, `pr()`)
- Modify: `internal/ghpr/testdata/pr.json` (`"mergeCommit": null` on the
  node, so the fixture matches the query)
- Modify: `internal/server/cycle.go` (`handleCycleAction`, signoff)
- Test: `internal/ghpr/ghpr_test.go`, `internal/server/cycle_test.go`

**Interfaces:**
- Produces: `ghpr.PR.MergeCommit string json:"merge_commit,omitempty"`,
  the merge commit's oid when the PR is merged. `GatePR.merge_commit?:
  string` in `board.ts`.

- [ ] **Step 1:** In `ghpr_test.go`, a test that decodes a node with
  `"mergeCommit": {"oid": "deadbeef…"}` and `state: MERGED` through
  `pr()` and finds `MergeCommit`. In `cycle_test.go`:

```go
func TestCycleSignoffAtTheMergedHead(t *testing.T) {
	ts, s, id := prRepo(t, "qa")   // branch head is the fixture's
	s.pulls.put("fix/2518-x", &ghpr.PR{Number: 1, State: "MERGED", MergeCommit: "deadbee0"}, nil)
	// a run at the merge commit, not at the branch head
	post(t, ts.URL+"/api/issues/"+id+"/cycle/verify", `{"output":"ok","commit":"deadbee0"}`) → 200
	post(… "/cycle/signoff", `{}`) → 200, the status moved
}
```

  and the existing "no run" test still refuses a run at another commit.
- [ ] **Step 2:** Run: `GOCACHE=$TMPDIR/go-build go test ./internal/ghpr
  ./internal/server -run 'Merged|Signoff'`. Expected: FAIL to compile
  (`MergeCommit`).
- [ ] **Step 3:** Add `mergeCommit { oid }` to `findQuery`, the field on
  `prNode` and `PR`, set it in `pr()`. In the signoff branch, `ran` is
  true when `run.Commit == ev.Head` or `ev.PR != nil && ev.PR.MergeCommit
  != "" && run.Commit == ev.PR.MergeCommit`; drop the M1 ponytail
  comment.
- [ ] **Step 4:** Run the two packages. Expected: PASS.
- [ ] **Step 5:** Commit `feat(cycle): a sign-off's run counts at the
  merged head too`.

### Task 3: the plan file rendered, and who drafted it

**Files:**
- Modify: `internal/cycle/cycle.go` (`Plan`)
- Modify: `internal/cycle/gather.go` (the plan file block)
- Test: `internal/cycle/gather_test.go`

**Interfaces:**
- Produces: `cycle.Plan{Path, Ref, Status, HTML string json:"html,omitempty", By string json:"by,omitempty"}`.
  `HTML` is `markdown.Render(body, identity)` of the file below its front
  matter; `By` is the author of the first commit adding the file on the
  plan branch (`b.FirstCommitAdding(in.Base, planRef, path)` then
  `b.ReadCommit`), empty when the file is only on the base.

- [ ] **Step 1:** Extend `TestGatherFromRepo`: `in.Plan.HTML` contains
  `<h1>Plan</h1>` and not `issue: 2518`; `in.Plan.By` is the repo's git
  identity name (`git(t, dir, "config", "user.name")`). Add a row where
  the plan is only on `dev` (`TestGatherNoBranch` or a new
  `TestGatherDeliveredPlan`): `By == ""`, `Status == "delivered"`.
- [ ] **Step 2:** Run: `GOCACHE=$TMPDIR/go-build go test ./internal/cycle
  -run Gather`. Expected: FAIL (`HTML` undefined).
- [ ] **Step 3:** Implement: a `frontMatterBody(doc) string` next to
  `frontMatter`, `markdown.Render` with `func(u string) string { return u }`
  (ponytail: no media URL in gather; plan files have no uploads).
- [ ] **Step 4:** Run the package. Expected: PASS.
- [ ] **Step 5:** Commit `feat(cycle): the plan file rendered, and who
  drafted it`.

### Task 4: the Issue/Cycle switch, the spine and the status bar

**Files:**
- Create: `web/src/components/IssueCycleView.vue`
- Create: `web/src/usecycle.ts` (the fetch/act/press/postRun logic lifted
  from `IssueCycle.vue`, used by both)
- Modify: `web/src/components/IssueCycle.vue` (use the composable; the
  `.prog` ticks before the progress line, E4)
- Modify: `web/src/components/IssuePanel.vue` (the segment, the view)
- Modify: `web/src/css/gitboard.css` (`.gb-seg`, `.gb-prog`, the spine,
  the 56.25rem fold, `.gb-panel-page.gb-panel-cycle`)

**Interfaces:**
- Consumes: Task 1's `spine`, `statusBar`; `cycleAction`, `getCycle`,
  `patchIssue` (M1).
- Produces:

```ts
// web/src/usecycle.ts
export function useCycle(props: { id: string; status: string; labels: string[] }, emit: (e: 'changed') => void) {
  return { cycle, error, busy, output, commit, why, needsWhy, has, checkedBy, act, press, postRun };
}
```

  `IssueCycleView.vue` props `{ id, status, labels, events: IssueEvent[], page?: boolean }`, emits
  `changed`, `comment: [body: string]`. The template: the `.gb-prog` line
  (hidden at ≥ 56.25rem), `<ol class="gb-spine">` with
  `<li :aria-current="row.state === 'current' ? 'step' : undefined" :class="'is-' + row.state">`
  (number, name, line), the workspace `<section class="gb-ws">` (Task 5
  and 6 fill it; here the eyebrow `Step N · Name · risk`, the gate, the
  note and the M1 card buttons via `cardFor`, so steps 2 and 6 keep
  their buttons), and `<footer class="gb-cycle-bar">` with
  `statusBar(cycle, waitingFor(events))`'s segments.
  `IssuePanel.vue`: `view = ref<'issue' | 'cycle'>('issue')`; the
  segment `<div class="gb-seg" role="group" aria-label="View">` with two
  buttons `:aria-pressed` next to Close issue; `view === 'cycle'` renders
  `<IssueCycleView … @comment="comment">` in place of `<IssueCycle>` and
  `.gb-cols`; the root gets `gb-panel-cycle` when `page && view ===
  'cycle'`, whose rule is `max-width: none`.

- [ ] **Step 1:** Write `usecycle.ts`, move `IssueCycle.vue`'s script
  onto it, add the ticks: `<a class="gb-prog" href="#" @click.prevent="emit('view')">` is
  not needed, the ticks are `<span v-for="i in cycle.of" :class="['gb-tick', i < step ? 'is-done' : i === step ? 'is-current' : '']">`
  before the line's text.
- [ ] **Step 2:** Write `IssueCycleView.vue` and the panel's switch.
- [ ] **Step 3:** CSS: `.gb-seg button[aria-pressed="true"]` filled black;
  `.gb-spine li { display: grid; grid-template-columns: 30px minmax(0, 1fr) auto }`;
  `.is-done .gb-spine-n { color: #007480 }`; `.gb-tick { width: 22px; height: 4px }`;
  `@media (max-width: 56.25rem) { .gb-spine { display: none } .gb-cycle-view .gb-prog { display: flex } }`.
- [ ] **Step 4:** Run: `cd web && npm run typecheck && npm test`.
  Expected: PASS.
- [ ] **Step 5:** Commit `feat(cycle): the Cycle view, its spine and its
  status bar`.

### Task 5: the Plan workspace

**Files:**
- Create: `web/src/components/CyclePlan.vue`
- Modify: `web/src/components/IssueCycleView.vue` (step 3 renders it)

**Interfaces:**
- Consumes: `cycle.evidence.plan{path, status, html, by}`, `plan_branch`,
  `plan_pr`, `useCycle().act('approve-plan')`.
- Produces: props `{ cycle: Cycle; busy: boolean; mine: boolean }`, emits
  `approve`, `revise: [body: string]`.

Template, E1 top to bottom: `<p class="gb-mono">{{ plan.path }}</p>` with
`status: {{ plan.status }}` and `plan PR #n` (a link to `plan_pr.url`)
when known; `<div class="gb-md gb-clamp" :class="{ 'is-open': open }"
v-html="plan.html">` with the button `Read the whole plan` / `Fold the
plan` toggling `open` (the summary paragraph, the options table and the
why paragraph come as the plan writes them: `ponytail: no parsing of the
plan's sections`); the gate block (`.gb-gate`, canard left border) with
`Drafted by {{ plan.by }} in {{ plan_branch }}` when `by` is known and
`, {{ plan_pr.reviews.length }} reviews so far` when a plan PR; the
buttons `Ask the agent to revise` (toggles a textarea + `Post` that
emits `revise`) and `Approve the plan` (primary, `v-if="mine"`); the
footnote `Once it's approved, step 4 starts: the agent writes the
failing test first.` No plan file → the gate and `No plan file yet: a
plan/<n>-… branch with a docs plan whose front matter says issue: <n>.`

- [ ] **Step 1:** Write the component and wire `IssueCycleView` (`revise`
  → emit `comment`).
- [ ] **Step 2:** Run: `cd web && npm run typecheck`. Expected: PASS.
- [ ] **Step 3:** Commit `feat(cycle): the Plan workspace`.

### Task 6: the Verify workspace

**Files:**
- Create: `web/src/components/CycleVerify.vue`
- Modify: `web/src/components/IssueCycleView.vue` (step 7 renders it)

**Interfaces:**
- Consumes: Task 1's `tiles`; `evidence.runs`, `checks`, `criteria`,
  `plan`, `signoffs`; the thread's `events` (the newest event with
  `meta['verify:run']` is the full run's comment: its `html`);
  `useCycle().act('check' | 'signoff' | 'verify')`, `patchIssue(id,
  {status: stages.in_progress[0]})`.
- Produces: props `{ cycle: Cycle; status: string; events: IssueEvent[]; busy: boolean; mine: boolean; needsWhy: boolean }`,
  emits `check: [index]`, `signoff: [why?: string]`, `run: [output, commit]`, `back`.

Template, E3 top to bottom: `<h3>Run it, read it, then sign it off</h3>`;
"The full run" with `{{ run.by }} at {{ run.commit.slice(0,7) }}`, the
run comment's `html` in a clamped `.gb-md` with `Read the whole output`;
when no run yet, M1's run box (textarea + commit + `Post the run`);
"Acceptance criteria, from the issue": the M1 checklist with `✓ by
{{ by }}`; `✓ Plan marked delivered by {{ plan.by }} · {{ plan.path }}`
when `plan.status === 'delivered'`, else `Plan not marked delivered`;
"Sign-offs": `<ul class="gb-so">` with one `<li :class="'is-' + t.state">`
per tile (`t.status`, `t.by`, `Signed off {{ t.when }}` / `Now` /
`Next`); the buttons `Sign off {{ status }}` (primary, `v-if="mine"`,
then the `needsWhy` form) and `Send back to in-progress`.

- [ ] **Step 1:** Write the component and wire `IssueCycleView` (`back`
  → `patchIssue`, then emit `changed`).
- [ ] **Step 2:** Run: `cd web && npm run typecheck && npm test`.
  Expected: PASS.
- [ ] **Step 3:** Commit `feat(cycle): the Verify workspace`.

### Task 7: the guessed stages in Settings, the docs and the PRD

**Files:**
- Modify: `internal/server/settings.go` (`settingsResponse.Cycle config.Stages json:"cycle"`)
- Modify: `web/src/api/board.ts` (`Settings.cycle`), `web/src/pages/SettingsPage.vue` (project-file tab: a `.gb-note` when `cycle.guessed`)
- Modify: `docs/configuration.md` (Cycle section: the Cycle view, the guessed note), `README.md` lines 80–84
- Modify: `docs/prd/dev-cycle.md` (M2 done, calls taken)
- Test: `internal/server/settings_test.go`

- [ ] **Step 1:** In `TestSettingsListAndToggleBridgeSync`, assert
  `got.Cycle.Guessed` and `got.Cycle.Closing` for the test config.
- [ ] **Step 2:** Run: `GOCACHE=$TMPDIR/go-build go test ./internal/server -run Settings`. Expected: FAIL.
- [ ] **Step 3:** Add the field (`s.config().Stages()`), the TS type, the
  note: `Cycle stages guessed from the status names: ready <ready>,
  in progress <…>, in review <…>, validation <…>, closing <…>. Set
  [cycle] to fix them.`
- [ ] **Step 4:** Docs: configuration.md's Cycle section gets the view
  (Issue/Cycle, the spine, the two workspaces, the sign-off's run at the
  merged head); README's paragraph mentions the Cycle view. PRD: the M2
  section's "Done when" ticked, "Calls taken" gets M2's: no worktree or
  agent in the status bar (nothing knows them, M3's agent sync will),
  the plan rendered whole rather than parsed into sections, "Ask the
  agent to revise" is a plain comment.
- [ ] **Step 5:** `make ci`, web typecheck + test. Commit
  `docs(cycle): the Cycle view, and the guessed stages in Settings`.

### Task 8: verification and hand-off

- [ ] **Step 1:** `GOCACHE=$TMPDIR/go-build make ci` and `cd web && npm
  run typecheck && npm test` green.
- [ ] **Step 2:** The browser check on a throwaway repo under `$TMPDIR`:
  `cd web && npm run build`, `go build -o $TMPDIR/git-board ./cmd/git-board`;
  `git init -b dev`, a git-bug identity, a `.git-board.toml` with the
  co2 statuses, `[roles]` and `[cycle]`, an issue with four acceptance
  criteria at `in-progress` with `risk:structural`, a `plan/<n>-…`
  branch with a plan file (front matter `issue:`, `status:`, an options
  table and a why paragraph), a `fix/<n>-…` branch adding a test; a
  second issue at `qa` with a run, two checks and one sign-off. One
  server per repository: `$TMPDIR/git-board serve --addr localhost:8099
  --me alice` from the throwaway repo. Playwright: `#/issues/<id>`,
  Cycle, screenshots at 1440px and 900px against E1 (the plan issue),
  E3 (the qa issue), E4 (the Issue view). Then press `Sign off qa` and
  check History shows the comment and the status move. No PR evidence
  without GitHub: note it.
- [ ] **Step 3:** Fix what the check shows, commit, `git log --oneline
  main..` lists the commits, no push. Say it is ready for `wtx land
  prd/dev-cycle`.

## Self-review notes

- Spec coverage: the switch ✓ (4), the spine with one line each and the
  fold ✓ (1, 4), Plan workspace ✓ (5), Verify workspace ✓ (6), status
  bar ✓ (1, 4), Open as page width ✓ (4), done-when: E1/E3/E4 in the
  browser ✓ (8), a sign-off moves the status and shows in History ✓ (M1
  code, checked in 8), the Issue view is today's page + ticks ✓ (4);
  M1's leftovers: merged head ✓ (2), guessed stages in Settings ✓ (7).
- Not in M2, said in Task 7: worktree and agent in the status bar; the
  plan parsed into front matter/options/why (rendered whole); a GitHub
  approval posted for structural plans; "two rounds of comments" counts
  the plan PR's reviews, not comment rounds.

wtx-effort: high
