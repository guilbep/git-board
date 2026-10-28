# Project cycle M1: the list, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A project declares its steps, `[cycle] steps = [...]`, and the
issue page follows: one rail cell and one flow segment per listed step,
numbered by the list. A file with no `steps` reads as today, through the
three reads: the default seven; `[cycle.signoffs]` entries as sign-off
cells after review, with `verify` dropped; a kind's rule as a string or as
`[cycle.kinds.<kind>] tests`. `git board lint` says what it read and
refuses a list out of order. `cycle:step` stores the step's id, and `"1"`
to `"7"` read as the default list's ids.

**Architecture:** The list lives in the config: `Stages().Steps`, built
from `[cycle] steps` or by the reads. `checkCycle` holds the order rule,
so every reader of the file refuses a bad list. `cycle.Compute` keeps its
kind index (`Step`, 1 to 7, and `Skipped`) for the gates and adds `At`,
the id of the list entry the issue has reached; an unlisted kind's gate
doesn't hold the issue. The page builds the rail and the segments from
`stages.steps` and `cycle.at`. Ids travel in `cycle:step`, in `?step=` and
from the composer; one function, `idOf` in `flow.ts`, reads `"1"` to
`"7"`. On the server, the card's actions write their step's id and a
comment's step is checked against the list. The sync marker carries
whatever `cycle:step` holds, so sync doesn't change.

**Tech Stack:** Go (BurntSushi toml's `Primitive` for the kinds), Vue 3,
`node:test`.

**Spec:** `docs/prd/project-cycle.md`, "Decisions", "The file", "Reading
today's files" and the M1 row; `docs/design/project-cycle/screens/F8-cycles.html`
for the rail and the segments of a short list (the PRD wins where they
differ).

**Base:** main at `c586370` (time-flow M1 to M5 landed), branch
`project-cycle`. Risk class: structural, since it changes what
`cycle:step` stores and how `[cycle]` is read. The PRD's decisions, the
design folder and this plan go first, in their own PR; the code follows
on the same branch, its commits pushed once that PR has landed.

## Global Constraints

- Every existing `.git-board.toml` reads unchanged: no `steps` key walks
  the default seven, `[cycle.signoffs]` and `[cycle.kinds] bug =
  "failing-test"` keep meaning what they mean. Nothing is rewritten on
  disk. co2-calculator's file (no `[cycle]`) shows the seven.
- Gates belong to kinds and never move with the list: a listed step's
  gate is today's. A kind the list leaves out holds nothing back: without
  `intent` no criteria are needed, without `risk` no class, without
  `plan` no approval, without `tests` no test (as the rule `none`).
  Review and sign-off steps left out change nothing in Compute; the
  stage simply has no cell.
- The ordinal is never stored: not in `cycle:step`, not in a URL. It is
  the entry's position in the served list, computed on the page and in
  the Work rows.
- The step is never typed: Compute stays its only source. The composer
  writes the issue's `at`, a card's action its own step's id.
- Nothing touches sync: `internal/store/body.go` is unchanged. Built-in
  ids and status names are carriable (`step=review`); a status with a
  space stays local, as any non-carriable value does.
- Compute's kind index stays where it decides something today:
  - `gate()` and the `Skipped` list;
  - `server/cycle.go`'s `cur.Step == 7` (at Verify), `== 1` (the
    verdict) and `!= 2` (the risk suggestion);
  - `worktrees.go`'s `cyc.Step == 3` and `> 3`;
  - `cycle.ts`'s `gate`, `evidence` and `LOCKS`.

  Only what shows or stores a step moves to ids and positions.
- No new metadata key, operation kind or entity; no new dependency.
- Tests: a new Go test calls `t.Parallel()` first, unless it chdirs (the
  lint tests do: they stay serial). Fixtures name Person A, B, C; logins
  alice, bob, carol. While developing, the package's own tests: `go test
  ./internal/config/ -run TestCycle`, `cd web && npx tsx --test
  src/flow.test.ts`; `cd web && npm run typecheck && npm test` before
  each web commit (lefthook runs the typecheck). `make ci` once, at the
  end (Task 10), through `rtk proxy make ci` into a log.
- Commit on `project-cycle`, never push `main`, never tag. No
  attribution trailer.

## Review Focus

1. A file with no `steps` and no `[cycle.signoffs]`: the list is the
   seven, `at` is today's step's id at every stage, the segments and the
   rail are today's with ids. (Task 1: `TestCycleSteps/no steps`; Task 3:
   `TestComputeAt/the default list`; Task 6: the existing flow tests,
   moved to ids, pass with the same entries in the same segments.)
2. An old `"6"` comment sits in the review segment; an old `"7"` on a
   project whose `[cycle.signoffs]` dropped `verify` sits in the segment
   running when it was written and shows `verify` as its chip. (Task 6:
   `"1" to "7" read as the default ids`, `an id the list lacks stays where
   it was written`.)
3. `[cycle.kinds.bug] steps = [...]` is a warning, not a refusal, and
   changes nothing yet; `tests` in the same table sets the rule. (Task 1:
   `TestCycleKindsTable`.)
4. `steps = ["review", "code"]` is refused when the file is read, with
   the reason, and `git board lint` prints it. (Task 1:
   `TestCycleStepsChecked`; Task 2: `TestLintRefusesStepsOutOfOrder`.)
5. A feat on `steps = ["code", "review"]`, at in-progress with no test
   file: `code` is current and Mark ready for review is offered; at ready,
   no cell is current. (Task 3: `TestUnlistedKindsHoldNothing`.)
6. `POST /comments` with a step the list lacks is a 400; a comment
   written in review carries `cycle:step = review`, and the card's
   actions write `plan`, `intent`, `risk` and the sign-off's id. (Task 4.)
7. co2's statuses with `[cycle.signoffs] qa = "pm"` and `tests-metier
   = "metier"`, no `steps`: the list ends `review, ready-for-validation,
   qa, tests-metier`, `verify` gone, `ready-for-validation` a cell with
   no role as today, and at each of them `at` is that status. With
   `steps` ending `review, qa, verify`: at `qa`, `qa`; at
   `tests-metier`, `verify`. (Task 1: `TestCycleSteps`; Task 3:
   `TestComputeAt/sign-offs`.)
8. `#/issues/<id>?step=6`, an old link, opens the review segment;
   `?step=review` does too. (Task 8, checked in Task 10.)

---

### Task 1: the list in the config

**Files:**
- Modify: `internal/config/cycle.go`, `internal/config/config.go`
- Test: `internal/config/cycle_test.go`

**Interfaces:**

```go
// Cycle gains
Steps []string `toml:"steps"`
// the kinds as the file writes them, string or table; parse fills Kinds
KindsFile map[string]toml.Primitive `toml:"kinds"`
Kinds     map[string]string         `toml:"-"`

// Step is one cell of the rail. Status is a sign-off's validation status;
// verify has none: it holds every validation status no listed sign-off
// has, all of them when it is alone, as today.
type Step struct {
	ID     string `json:"id"`
	Kind   string `json:"kind"` // intent, risk, plan, tests, code, review, signoff
	Name   string `json:"name"` // "Review"; a sign-off's is its id until M2
	Status string `json:"status,omitempty"`
}

// Stages gains, never null in JSON
Steps []Step `json:"steps"`

// DefaultSteps: intent, risk, plan, tests, code, review, verify
var DefaultSteps []Step

// Reads is what the file's old forms were read as, one line each, for
// lint: the default list (named), [cycle.signoffs] as cells, a kind's
// string as its tests.
func (c *Config) Reads() []string
```

- [ ] **Step 1:** Failing tests.
  - `TestCycleSteps`, a table:
    - no `steps`: the seven;
    - `[cycle.signoffs]` with `qa` and `tests-metier` over co2's status
      options: the six, then `ready-for-validation`, `qa`,
      `tests-metier`, in `validation`'s order, no `verify`;
    - a `[cycle.signoffs]` key no validation status names: last;
    - `steps = ["code", "review"]`: two;
    - `steps` naming a sign-off: its `Status` is its key;
    - `steps = [..., "review", "qa", "verify"]`: read, both kept.
  - `TestCycleStepsChecked`: each refused with its message:
    - `["review", "code"]`;
    - `["code", "qa"]` with no `[cycle.signoffs] qa`;
    - `["code", "code"]`;
    - `[]`.
  - `TestCycleKindsTable`:
    - `bug = "failing-test"` and `[cycle.kinds.doc] tests = "none"`
      both land in `Kinds`;
    - `[cycle.kinds.bug] steps = ["code"]` comes out of `Check` as the
      warning `cycle.kinds.bug.steps: git-board doesn't know this key`;
    - a table without `tests` keeps the default rule.
  - `TestCycleReads`: the lines for a file with no `steps`, with
    `[cycle.signoffs]`, with a string kind.
- [ ] **Step 2:** `go test ./internal/config/ -run 'TestCycle'`: fails.
- [ ] **Step 3:** Implement.
  - `parse` fills `Kinds` from `KindsFile`, one entry at a time:
    - `md.Type("cycle", "kinds", k)` is `"String"`: the string is the
      rule;
    - otherwise `md.PrimitiveDecode` into `struct{ Tests string }`.
      PrimitiveDecode marks only `tests` decoded, so `steps` falls to
      `Check`'s unknown-key warning with no new code;
    - a string kind's name is kept for `Reads`.
  - `checkCycle`, on `Steps` when set:
    - the known ids are the six built-ins, `verify`, the validation
      statuses and the `[cycle.signoffs]` keys (what read 2 reads, so a
      project can write it out);
    - the built-ins in the order `intent, risk, plan, tests, code,
      review`, then the sign-offs and `verify` in any order;
    - no id twice, at least one id.
  - The messages:
    - `[cycle] steps: "code" after "review": steps keep the cycle's order, intent, risk, plan, tests, code, review, then the sign-offs (a review needs the PR that code opens)`;
    - `[cycle] steps: "qa" is no step: intent, risk, plan, tests, code, review, verify, a validation status or a [cycle.signoffs] key`;
    - `[cycle] steps: "code" twice`.
  - `Stages()` builds `Steps`:
    - the file's list, each id to its `Step`;
    - else `DefaultSteps`; when `[cycle.signoffs]` has entries,
      `verify` is replaced by one sign-off per validation status, in
      `validation`'s order, then a key no status names, by name. A
      status the table doesn't key has no role, as today.
- [ ] **Step 4:** `go test ./internal/config/`: green, the existing
  tests unchanged.
- [ ] **Step 5:** Commit: `feat(config): [cycle] steps, read with today's files`.

### Task 2: lint says what it read

**Files:**
- Modify: `cmd/git-board/lint.go`
- Test: `cmd/git-board/lint_test.go`

**Interfaces:** `runLint` prints `cfg.Reads()` under a `read:` heading
before the findings. A refused list never reaches it: `loadConfig` fails
first with `checkCycle`'s message.

- [ ] **Step 1:** Failing tests, serial as the file's others (they chdir
  into a throwaway repo):
  - `TestLintSaysWhatItRead`: a `.git-board.toml` with
    `[cycle.signoffs] qa = "pm"` and `[cycle.kinds] bug = "failing-test"`;
    the output names both reads.
  - `TestLintRefusesStepsOutOfOrder`: `steps = ["review", "code"]` exits
    non-zero, and the error says `"code" after "review"`.
- [ ] **Step 2:** `go test ./cmd/git-board/ -run TestLint`: fails.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** `go test ./cmd/git-board/ -run TestLint`: green.
- [ ] **Step 5:** Commit: `feat(lint): say how the cycle was read`.

### Task 3: Compute walks the list

**Files:**
- Modify: `internal/cycle/cycle.go`
- Test: `internal/cycle/cycle_test.go`

**Interfaces:**

```go
// Cycle gains; Of becomes the list's length, Name the entry's name (the
// kind's when At is "")
At string `json:"at"` // the list entry reached, "" before the first one opens

// Position is At's place in the list, from 1; 0 when At is "". The
// ordinal, computed, never stored: the Work row and the agent's prompt.
func (c Cycle) Position() int
```

The entry reached, the first rule that gives one:
1. a closed issue: the last entry;
2. at a validation status: the sign-off listed for it, else `verify`
   when listed;
3. the last listed entry whose rank is at most the issue's.

Ranks, each a pair compared in order:
- a built-in entry: its kind index, then 0;
- `verify`: (7, 0);
- a sign-off: (7, 1 + its status's position in `validation`);
- the issue: Compute's `Step`, then 1 + its status's position in
  `validation` (0 when the status isn't a validation status).

Rule 3 is also where a status no listed step opens at falls: the last
one before it (M4's note will name it). An `Input` with no
`Stages.Steps`, as the tests build them, walks `DefaultSteps`.

An unlisted kind holds nothing back, inside Compute's switch:
- no `intent`: the criteria count as there, and no verdict is asked;
- no `risk`: no risk wait and no "risk not set" note;
- no `plan`: the plan counts as approved;
- no `tests`: step 4 passes, as for the rule `none`.

`Skipped` keeps 3 and 4 only when the list has the step.

- [ ] **Step 1:** Failing tests.
  - `TestComputeAt`, a table over co2's stages:
    - the default list: `at` at each stage is today's step's id;
    - sign-offs read from `[cycle.signoffs]`: at each validation
      status, that status;
    - `[..., "review", "qa", "verify"]`: at `qa`, `qa`; at
      `tests-metier`, `verify`;
    - `[..., "review", "qa"]`: at `tests-metier`, `qa`, the last before
      it;
    - closed: the last entry.
  - `TestUnlistedKindsHoldNothing`, with `["code", "review"]`:
    - a feat at ready with no criteria and no risk: `at` is `""` and the
      action is `start`;
    - at in-progress with no test: `at` is `code` and the action is
      `ready-for-review`;
    - `Skipped` is empty.
  - `TestComputeOfAndName`: `Of` is the list's length, `Name` the
    entry's, `Position()` its place (0 with `At` empty).
- [ ] **Step 2:** `go test ./internal/cycle/ -run 'TestCompute|TestUnlisted'`:
  fails.
- [ ] **Step 3:** Implement: a `listed(kind)` read of the list at the
  top of Compute, the four conditions, then `At`, `Of` and `Name` where
  `c.Name, c.Gate` are set today.
- [ ] **Step 4:** `go test ./internal/cycle/`: green, `TestWalk` and the
  others unchanged.
- [ ] **Step 5:** Commit: `feat(cycle): the step the list has reached`.

### Task 4: the server writes ids

**Files:**
- Modify: `internal/server/cycle.go`, `internal/server/issues.go`
- Test: `internal/server/cycle_test.go`, `internal/server/issues_test.go`

**Interfaces:** `handleComment`'s `step` is an id of
`s.cfg.Stages().Steps`, or none; anything else is `400 step: one of
<ids>, not "<step>"`. A tab opened before the upgrade still sends a
number (a hash route doesn't reload the bundle). The number 1 to 7, as
JSON, is read through the default ids, like a stored `"6"`, so the
comment isn't refused.

The card's actions write:

| Action | `cycle:step` |
| --- | --- |
| verify, check, sign-off | the issue's `At` (`verify`, or the sign-off's id) |
| approve-plan | `plan` |
| spec-verdict, no-criteria | `intent` |
| the AI's risk suggestion | `risk` |

- [ ] **Step 1:** Change the expectations, then add a test.
  - `cycle_test.go`: lines 234, 306, 556 and 621 expect `plan`,
    `verify`, `intent`, `intent`; the suggestion's test expects `risk`.
  - `TestCommentOnAStep` sends `"plan"` and expects it.
  - `TestCommentStepIsTheLists`:
    - on a project with `steps = ["code", "review"]`, `"review"` is
      kept;
    - `6` (a number, an old tab) is kept as `review`;
    - `"plan"` and `"qa"` are 400s.
- [ ] **Step 2:** `go test ./internal/server/ -run 'TestComment|TestCycle|TestSuggest|TestSpec|TestNoCriteria'`:
  fails.
- [ ] **Step 3:** Implement: the five writers. The request's `step`
  decodes as `json.RawMessage`: a JSON number 1 to 7 is the default id,
  a string is taken as it is, and either is checked against the list.
- [ ] **Step 4:** Same command: green. `go test ./internal/store/`:
  green, untouched (`step=review` rides the marker as `step=6` did).
- [ ] **Step 5:** Commit: `feat(cycle): cycle:step holds the step's id`.

### Task 5: the Work rows and the agent's prompt count in the list

**Files:**
- Modify: `internal/work/work.go`, `internal/server/worktrees.go`,
  `web/src/work.ts`
- Test: `internal/work/work_test.go`, `internal/server/worktrees_test.go`,
  `web/src/work.test.ts`

**Interfaces:**
- `Row.Step` is `c.Position()`; `Of` and `Name` are Compute's.
- The agents' row (`server/work.go:149`) copies the issue's row:
  unchanged, it follows.
- The agent's prompt (`worktrees.go:350`, "step 4 of 7: Tests") takes
  `cyc.Position()`; without it, a two-step list would read "step 5 of
  2: Code".
- `stepLabel(0, name)` is the name alone.

- [ ] **Step 1:** Failing tests.
  - `TestRowStepIsTheListsOrdinal`: with `["code", "review"]` at
    in-review, the row is `Step 2, Of 2, Name "Review"`.
  - `TestAgentGoCountsInTheList`: with `["code", "review"]` at
    in-progress, the prompt says `step 1 of 2: Code`. The existing
    `step 4 of 7: Tests` expectations stand.
  - `work.test.ts`, `the small words`: `stepLabel(0, 'Code')` is
    `Code`.
- [ ] **Step 2:** `go test ./internal/work/`, `go test ./internal/server/
  -run 'TestAgent'` and `cd web && npx tsx --test
  src/work.test.ts`: fail.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** Both green.
- [ ] **Step 5:** Commit: `feat(work): a row's step is the list's`.

### Task 6: the flow reads ids

**Files:**
- Modify: `web/src/flow.ts`, `web/src/api/board.ts`
- Test: `web/src/flow.test.ts`

**Interfaces:**

```ts
// api/board.ts
export interface StageStep { id: string; kind: string; name: string; status?: string }
// CycleStages gains steps: StageStep[]; Cycle gains at: string

// the default list's ids by number: "6" is review
export const DEFAULT_IDS: readonly string[] // ['', 'intent', … 'verify']
// a cycle:step or ?step= value as an id: "1" to "7" through DEFAULT_IDS
export const idOf = (v: string): string
// the id a comment carries, '' for none
export const stepOf = (e: IssueEvent): string

// n: the entry's position in the list, from 1; at: when it opened
export interface Segment { id: string; n: number; name: string; entries: IssueEvent[]; at?: string }
// one segment per list entry, from the first to the highest reached,
// minus the skipped kinds (never the current one)
export function segments(events: IssueEvent[], stages: CycleStages | undefined, current: string, skip: number[] = []): Segment[]
export function pick(segs: Segment[], events: IssueEvent[], r: Reading, meId: string, current: string, on?: On): Segment[]
// "02 Review"
export const carries = (s: { n: number; name: string } | undefined): string
```

The walk is today's, with list positions in place of step numbers:

| Event | Its segment |
| --- | --- |
| a status move | the entry reached at that status (the rank rule of Task 3; ready with a risk set ranks as plan) |
| the risk set at ready | the entry reached at plan's rank |
| the close | the last entry |
| a comment carrying a listed id | that entry, which the running position rises to |
| a review comment with no step | `review`'s entry when listed, else the running one |
| anything else (no step, an id the list lacks) | the running one |

When no entry is reached, the first segment takes the entries: they sit
under the description. After the last move, the running position is
`current`'s.

- [ ] **Step 1:** Move the existing tests to ids (`step` → `current`,
  `n` → `id` where they name a step), then add failing ones:
  - `"1" to "7" read as the default ids`: a `"6"` comment sits in
    `review`;
  - `two steps, two segments`: `["code", "review"]`;
  - `an id the list lacks stays where it was written`: a `"7"` on a
    sign-off list;
  - `before the first step, the first segment`;
  - `a move to qa opens qa's segment`;
  - `idOf`.
- [ ] **Step 2:** `cd web && npx tsx --test src/flow.test.ts`: fails.
- [ ] **Step 3:** Implement. `cycle.test.ts` and `replay.test.ts` keep
  their `'cycle:step': '1'` fixtures: they read through `idOf`.
- [ ] **Step 4:** `cd web && npx tsx --test src/flow.test.ts`: green.
- [ ] **Step 5:** No commit yet. Tasks 6, 7 and 8 are one commit, made
  at the end of Task 8. Changing `Segment` and `stepOf` breaks their
  callers until Task 8, and lefthook typechecks every commit.

### Task 7: the rail from the list

**Files:**
- Modify: `web/src/cycle.ts`, `web/src/components/IssueStep.vue`
- Test: `web/src/cycle.test.ts`

**Interfaces:** `steps(c, risk, events)` returns one row per list entry:
`{ id, i, n, name, state, line, gate }`.
- `i` is the entry's position, from 1.
- `n` is its kind's index, for `gate`, `evidence`, `LOCKS` and the
  workspace.
- `state` comes from positions against `c.at`: before is `done`, at is
  `current`, after is `locked`.
- A skipped kind is `skipped`, unless it is the current entry.
- With `at` empty, every row is `locked`.

`IssueStep.vue`:
- the rail keys and links by id: `?step=${r.id}`, `jump($event, r.id)`;
- `show(id: string)`;
- `shown` holds an id, and the workspace is picked by the shown entry's
  kind (`signoff` is `CycleVerify`, unchanged);
- with no entry shown and `at` empty, no workspace heads the column;
- emits `comment: [body, step: string]` and `open: [id: string]`;
  line 260's `3` is `'plan'`;
- "Back to step …" names the current entry's position and name.

- [ ] **Step 1:** Failing tests in `cycle.test.ts`:
  - `the rail is the list's`: `["code", "review"]` gives two rows,
    `i` 1 and 2;
  - `a struck plan stays a cell`;
  - `nothing current before the first step opens`;
  - `the sign-off cells`.

  Update `the step bar…` tests to the rows' new fields.
- [ ] **Step 2:** `cd web && npx tsx --test src/cycle.test.ts`: fails.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** `cd web && npx tsx --test src/cycle.test.ts`: green.
- [ ] **Step 5:** No commit yet (see Task 6).

### Task 8: the page carries ids

**Files:**
- Modify:
  - `web/src/components/IssuePanel.vue`
  - `web/src/pages/IssuePage.vue`
  - `web/src/components/IssueFlow.vue`
  - `web/src/components/CommentCard.vue`
  - `web/src/components/HistoryView.vue`
  - `web/src/timeline.ts`
  - `web/src/components/IssueTimeline.vue`
- Test: `web/src/timeline.test.ts`

**Interfaces:**
- `IssuePage` passes `step` as `idOf(String(route.query.step ?? ''))`
  or `undefined`. `IssuePanel`'s `step` prop and `openStep` take an id.
  `showLine` shows `'review'`.
- The composer, the anchored comment and the replies carry `cycle?.at
  ?? ''`, and `carries` is given the current entry.
- `cycle.step === 1` (the spec bar and Move to ready) becomes
  `cycle.at === 'intent'`: with no Intent cell, no verdict (F8's doc).
- `IssueFlow` passes `cycle.at` to `segments` and `pick`, ids to the
  dividers' anchors (`step-${s.id}`), and the list to `CommentCard`.
- `CommentCard`'s chip reads the list: `step 02` for a listed id, the id
  itself for one the list lacks.
- `HistoryView` passes `cycle.at`. In `timeline.ts`, a `Bar` gains
  `name`, and the first bar is the first segment's, whatever its `n`.
  `IssueTimeline`'s `stepName` reads the bar's name, not `STEPS`.

- [ ] **Step 1:** Failing test: `timeline.test.ts`, `the step bars of a
  two-step list`: two bars named Code and Review, the first from the
  start.
- [ ] **Step 2:** `cd web && npx tsx --test src/timeline.test.ts`:
  fails.
- [ ] **Step 3:** Implement.
- [ ] **Step 4:** `cd web && npm run typecheck && npm test`: green, with
  Tasks 6 and 7.
- [ ] **Step 5:** Commit Tasks 6, 7 and 8 together: `feat(cycle): the
  rail, the flow and the page follow the list`.

### Task 9: the guide

**Files:**
- Modify: `docs/guide/configuration.md` (Cycle). `docs/spec/data-model.md`
  names the key, not its values: unchanged.
- Test: none (docs); Task 10's `make ci` runs the docs checks there are.

- [ ] **Step 1:** Write the changes.
  - Cycle's opening paragraph: the steps are the project's list, the
    seven by default.
  - A `steps` row in the keys table, with the order rule and its
    refusal.
  - `kinds`: the table form beside the string, and `steps` under a kind
    is M3's (a warning until then).
  - `signoffs`: with entries, every validation status is a cell after
    Review, its role from `signoffs` when keyed, and `verify` goes.
    Listed with sign-offs, `verify` holds the statuses they don't, so
    it goes where those statuses fall.
  - "The comments with metadata": `cycle:step` holds the step's id, and
    `1` to `7` read as the default ids.
  - Lint's paragraph says it prints what it read.
- [ ] **Step 2:** Commit: `docs(guide): the project's cycle`.

### Task 10: verification and hand-off

- [ ] **Step 1:** `cd web && npm run typecheck && npm test` green, then
  `GOCACHE=$TMPDIR/go-build rtk proxy make ci > $TMPDIR/ci.log 2>&1`
  green (`TestCutAndPromoteSameDigests` is known to flake under the full
  run: rerun it alone before blaming the branch).
- [ ] **Step 2:** Check in a browser, on throwaway repos under
  `$TMPDIR`, never on co2-calculator itself (no call at it, not even a
  dry run). Use unsigned commits: go-git can't read SSH-signed ones.
  - Build: `cd web && npm run build`, then `go build -o
    $TMPDIR/git-board ./cmd/git-board`.
  - Repo A, `steps = ["code", "review"]`, an issue walked to in-review
    with a comment at each status:
    - the rail shows two cells and the flow two segments;
    - a comment written in review carries `cycle:step = review` (`git
      bug show`);
    - an older comment given `"6"` in a fixture sits in the review
      segment.
  - Repo B, a copy of co2-calculator's `.git-board.toml`, untouched:
    the seven, as on main.
  - `git board lint` in repo A with the list turned to `["review",
    "code"]`: refused, with the reason.
  - One server per repository: `$TMPDIR/git-board serve --addr
    localhost:8099 --me alice`. Use Playwright at 1440 px and 800 px;
    check the Review Focus, `?step=6` and `?step=review`.
- [ ] **Step 3:** Fix what the check shows, commit. `git log --oneline
  main..` lists the commits; no push. Say it is ready for `wtx land
  project-cycle`.

## Self-review notes

- Spec coverage:
  - the list and its default ✓ (1);
  - read 1 ✓ (1);
  - read 2, a cell per validation status, each holding today's
    Verify ✓ (1, 3, 7);
  - read 3, both forms ✓ (1);
  - lint says what it read ✓ (2);
  - the order rule, unknown ids and duplicates in `checkCycle` ✓ (1, 2);
  - opens at the last reached, none current before ✓ (3, 6, 7);
  - risk and the `none` rule skip only listed steps, struck ✓ (3, 7);
  - `cycle:step` ids, every writer ✓ (4) and reader ✓ (5, 6, 7, 8);
  - `"1"` to `"7"` read ✓ (6, 8);
  - in flight: unknown ids and the first segment ✓ (6);
  - the rail follows on save ✓ (the list is in `Stages()`, which
    `setConfig` already replaces);
  - the guide ✓ (9).
- Three calls the PRD didn't state, answered by the user on 2026-10-27
  and written into the PRD's decisions:
  1. A kind the list leaves out holds nothing back (Global Constraints,
     Task 3).
  2. `verify` may be listed with sign-offs: it holds the validation
     statuses none of them has (Task 3's rule 2).
  3. Read 2 gives every validation status a cell, not one per
     `[cycle.signoffs]` entry, so a file keying only some statuses keeps
     each of them signed off where today's Verify did (Task 1).
- ponytail: the rail's done and locked follow positions. A list that
  puts `verify` before a sign-off whose status comes earlier shows
  `verify` done while it hasn't opened; the guide says to list `verify`
  where its statuses fall.
- Not in M1, each with its milestone:
  - the `[cycle.signoffs.<id>]` tables, their `name`, the sign-off
    workspace per status, `[cycle.names]` (M2);
  - a kind's `steps` (M3);
  - Settings › Cycle and the "not in the cycle" note (M4);
  - `cycle:waived` (M5);
  - the merge as code's gate when review isn't listed (F8's small bug):
    review left out changes nothing in Compute, the in-review stage just
    has no cell.
- `CycleVerify.vue` is unchanged: a sign-off cell holds it as Verify
  does today, and its Sign off acts on the issue's status.

wtx-effort: high
