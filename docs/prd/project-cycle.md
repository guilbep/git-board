# The cycle is the project's own

PRD, 2026-10-27. Builds on `cycle-protocol.md` (the current step is the
main screen; the step comes from the status and the evidence; the rules
of the cycle live in `[cycle]`) and on the time-flow handoff
(`docs/design/time-flow/`: the rail, the flow's segments, `cycle:step` on
a comment). Drawn on the design canvas "one issue, one plane?", row 7
(artboards 7a and 7b, copied to `docs/design/project-cycle/screens/`).
The decisions below were proposed on the canvas on 2026-10-27 and taken
with the user the same day, one line at a time.

## The problem

Every issue walks the same seven steps. Two can be skipped, Plan at low
risk and Tests when the kind's rule is `none`, but the list is the same
for a doc and for a structural feature, and the same for every project.
A project whose practice is code, then review, shows five empty or skipped
cells, and the people who need two steps learn to ignore the rail. A
project that wants intent, risk, code and verify, with no plan and no
test-first step, can't say so. And co2-calculator's three validation
statuses, `qa`, `tests-metier` and `validated`, each a different person's
check, are today three sign-offs inside one Verify step, where the rail
shows one cell for three gates.

**The project declares its steps, and the page follows: one cell of the
rail per step, one segment of the flow per step, the numbers are the
list's. Gates belong to the kind of step, so declaring the list changes
nothing about what a step requires.**

## Decisions

Taken on 2026-10-27, each with the user.

| Topic | Taken |
| --- | --- |
| The list | `[cycle] steps = [...]`, in order. The default is the seven of `dev-cycle.md`: `intent, risk, plan, tests, code, review, verify`. A project with no `steps` walks the default, so nothing changes until a project writes the key. |
| Kinds of step | Seven: `intent`, `risk`, `plan`, `tests`, `code`, `review`, `signoff`. Each kind brings its workspace (today's `CycleIntent` … `CycleVerify`), its evidence and its gate, unchanged. A kind the list doesn't have holds nothing back: without `intent` no criteria, without `risk` no class, without `plan` no approval, without `tests` no test (as the rule `none`). `verify` is the default list's sign-off step and opens where today's Verify opens: the first validation status, or the merge when there is none. Until M2 it is today's Verify, every `[cycle.signoffs]` sign-off inside it. Listed with sign-offs, it holds the validation statuses none of them has. |
| Sign-off steps | `[cycle.signoffs.<id>]` with `name`, `status` (default: the id) and `role`; today's `qa = "pm"` is its short form, a sign-off `qa` held by the role `pm`. No `kind` field: whatever sits under `[cycle.signoffs]` is a sign-off. A sign-off is the only kind a list can hold several times. The criteria of Intent are ticked across the sign-off steps; the last sign-off closes the issue, and the forge's with it. M1 reads today's entries as steps, one cell per validation status after `review`, each holding today's Verify workspace for its status; the table form, the generalized workspace and the closing are M2's. |
| Per kind of issue | `[cycle.kinds.<kind>]` gains `steps`, a sublist of the project's list in the same order; a kind without it walks the whole list. Today's `bug = "failing-test"` is the short form of `[cycle.kinds.bug] tests = "failing-test"`. M1 reads `tests` in both forms; until M3, a kind's `steps` gets the warning any key git-board doesn't know gets. |
| Risk still skips inside the list | Low risk skips `plan`, and a kind whose rule is `none` skips `tests`, as today, each only when the list has the step. The skipped cell stays on the rail, struck. |
| The order is a selection, not a free order | The list must be a subsequence of `intent, risk, plan, tests, code, review`, followed by sign-offs and `verify` in any order. `checkCycle` refuses another order when the file is read, with the reason, so `git board lint`, `serve`, the TUI and Settings' save all refuse it; it refuses an unknown id and a step listed twice the same way. This keeps each gate meaningful: a review needs a PR, a test-first step needs a branch, a sign-off needs a merge. The user's three examples (code · review; code · review · verify; intent · risk · code · verify) all pass. |
| Opens at | Each step opens at a status, as today: `ready` for risk and plan, `in-progress` for tests and code, `in-review` for review, a sign-off's own `status`. Intent is the floor. The current cell is the last listed step the issue has reached. With none reached (a list that starts at `code`, an issue still at `ready`), there is no current cell and the rail shows every cell to come. The "not in the cycle" note under a card, for a status no step opens at, is M4's. |
| The step's id, not its number | `cycle:step` on a comment stores the step's id (`intent`, `review`, `qa`), never the ordinal. The ordinal is the rail's and changes with the list; the id doesn't. Existing values `"1"` to `"7"` are read as the default list's ids (`1` intent … `6` review, `7` verify), and the server writes ids from the first release that reads them. The writers are `internal/server/cycle.go` (the card's actions) and `internal/server/issues.go` (a comment, its step checked against the list); the readers are `web/src/flow.ts` (`stepOf`, `segments`), the comment's chip, History's step lane, the rail and the Work page. This replaces time-flow's "`cycle:step` keeps today's values". The sync marker carries the id as it carried the number. |
| Settings › Cycle | A page lists the steps (kind, name, opens at, role, gate), lets a person reorder, add a sign-off or remove a step, and shows which steps each kind of issue walks. It writes the tables to the working tree's `.git-board.toml`, as Enable checks does. The list is read like the rest of `[cycle]`, from the file the server loaded, so the rail follows on save; only `[cycle.checks]` are read from the base. |
| Issues in flight | The step still comes from the status and the evidence, read against the list the server serves. A comment whose `cycle:step` names a step the list doesn't have, or that has none, sits in the segment running when it was written; an unknown id shows as a chip. That covers an old `"7"` once `[cycle.signoffs]` drops `verify`. Entries from before the first listed step opens go to the first segment, under the description. |

## What it looks like

- `screens/F8-cycles.html` (7a): the same issue page under three
  declarations. co2-calculator, a structural feature, nine steps, the
  three validation statuses as sign-off steps with their roles. git-board,
  a doc, two steps, the description folded and without verdict since
  Intent is not a step. A small project's bug, four steps, no plan, no
  test-first step, the merge as code's gate.
- `screens/F9-settings-cycle.html` (7b): Settings › Cycle, the list, the
  per-kind table, the toml the page writes, the rails that result.
- The rail and the flow are the time flow's (`docs/design/time-flow/`,
  screens 6c and 6g): one cell per step, one segment per step.

Three things stay fixed, stated on 7a: the description is always the
first entry of the column, whether or not Intent is a step; a comment
carries its step's id; gates belong to kinds.

## The file

```toml
[cycle]
steps = ["intent", "risk", "plan", "tests", "code", "review",
         "qa", "tests-metier", "validated"]
ready = "ready"
in-progress = ["in-progress"]
in-review = ["in-review"]

[cycle.signoffs.qa]
name = "QA on staging"
role = "pm"

[cycle.signoffs.tests-metier]
name = "Tests métier"
role = "metier"

[cycle.signoffs.validated]
name = "Validated"
role = "lead-dev"

[cycle.kinds.feat]
tests = "per-criterion"

[cycle.kinds.bug]
tests = "failing-test"
steps = ["intent", "risk", "tests", "code", "review", "validated"]

[cycle.kinds.doc]
tests = "none"
steps = ["code", "review"]
```

A step of a built-in kind needs no table: its id is its kind. A
sign-off's id is its table's name; `role` is required, `status` and
`name` default to the id. Everything else in `[cycle]` (`signers`,
`[cycle.tests.*]`, `[cycle.checks.*]`) is untouched.

### Reading today's files

Three reads keep every existing `.git-board.toml` working, all three in
M1:

1. no `steps`: the default seven;
2. `[cycle.signoffs] qa = "pm"`: a sign-off step `qa` with `role = "pm"`
   and `status = "qa"`. When `[cycle.signoffs]` has entries, `verify` is
   dropped from the default list and every validation status becomes a
   sign-off step after `review`, in the project's order, its role from
   `[cycle.signoffs]` when keyed and none otherwise, as today; a key no
   status names comes last. A file keying some statuses keeps each one
   signed off where Verify signed it off;
3. `[cycle.kinds] bug = "failing-test"`: `[cycle.kinds.bug] tests =
   "failing-test"`.

`git board lint` says what it read, so a project can write the new form
when it wants to.

## Milestones

| Milestone | Ships | Done when |
| --- | --- | --- |
| M1 · the list | `[cycle] steps`, the three reads and lint saying what it read, the order rule, the rail and the flow's segments built from the list, `cycle:step` ids with the read of `"1"` to `"7"`. | A project with `steps = ["code", "review"]` shows a two-cell rail and two segments; co2-calculator's file, untouched, shows the seven, as today; `git board lint` refuses `["review", "code"]` and says why; a comment written in review carries `cycle:step = review`; an old comment with `"6"` is in the review segment. |
| M2 · sign-off steps | `[cycle.signoffs.<id>]` tables with `name`, `status` and `role`; the sign-off workspace (today's Verify generalized: the criteria ticked, the role's person signs off, the status moves, the last one closes); `[cycle.names] <id> = "..."` renaming a built-in step, display only. | A copy of co2-calculator's file with its three validation statuses as sign-offs shows qa, tests-metier and validated as three cells with their roles; signing off qa opens tests-metier; the last sign-off closes the issue and GitHub's; `[cycle.names] tests = "Red first"` renames the cell and its segment, not the id. |
| M3 · per kind of issue | `[cycle.kinds.<kind>] steps`. | A doc shows code · review; a bug shows its six; an unlisted kind walks the whole list. |
| M4 · Settings › Cycle | The page of 7b; Write to .git-board.toml; the "not in the cycle" note under a card. | Adding a sign-off on the page writes the table; the rail follows on save. |
| M5 · waiving a step | `cycle:waived = <id>` on a comment, with the reason, by the step's role, on one issue. | A reviewer waives review on one issue with a reason: its cell shows waived with the reason and the next step opens; another issue of the same kind still walks review. |

M1 is the one that touches the data (`cycle:step`), so it is the one to
plan first and to class structural: `project-cycle-m1-plan.md`.

## Open points

Settled on 2026-10-27 with the user.

1. **The order rule.** A subsequence of the canonical order, sign-offs
   last, was proposed. Does any project need review before tests, or a
   sign-off before code (a design review)? Settled: the subsequence,
   sign-offs last. A sign-off before code is a later PRD, if a project
   asks for one.
2. **Waiving a step on one issue.** A role's person could skip a step
   with a reason, recorded as a comment with `cycle:waived = <id>`.
   Settled: in, as M5.
3. **Renaming built-in steps.** `name` on a built-in kind (Tests shown as
   "Red first"). Settled: in, with M2, as `[cycle.names] <id> = "..."`;
   display only, the id stays.
4. **The default list for a project with validation statuses.** Read 2
   above drops `verify` when `[cycle.signoffs]` has entries. Settled:
   dropped. co2-calculator's file has no `[cycle]` today, so it keeps
   the seven, its validation statuses signed off inside Verify.
5. **Who edits the cycle.** Settings › Cycle writes the working tree's
   file; anyone with the checkout can. Settled: no role, as on every
   Settings tab today; the commit's review is the guard.
