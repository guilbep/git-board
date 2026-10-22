# The current step is the issue's main screen

PRD, 2026-10-22. Builds on `dev-cycle.md`: the step still comes from the
status and the evidence, and is never stored. The decisions below were
taken with the user on 2026-10-21, on the design canvas "one issue, one
plane?" (rows 4a and 4b, and the artboards C1 to C7, one per step). Issue
7fd2891 on the board.

## The problem

The issue page has an Issue / Cycle switch, so one issue lives on two
pages: the description and the conversation on one, the spine and the
step's workspace on the other. Nobody knows which one to open.

The cycle is also hard-coded. It reads acceptance criteria only under a
heading named exactly "Acceptance criteria". GB-31's four criteria, under
"Expected", were not seen, and the Cycle view's "Save the criteria" wrote a
second list below them. Every project words its templates its own way, and
every project has its own rule for tests.

**One page per issue, and the current step is its main screen. The rules
of the cycle are declared in `[cycle]`, with a default for everything.**

## Decisions

Taken with the user on 2026-10-21. Not reopened here.

| Topic | Decision |
| --- | --- |
| One page | The current step is the main screen, with a horizontal step bar under the title. A done step reopens when clicked. A later step is locked and says why ("opens when the PR is ready for review"). Step 1, Intent, is the issue's description and its discussion. The Issue / Cycle switch goes. |
| The protocol | `[cycle]` grows the cycle's rules, each with a default. Criteria headings are a list: Acceptance criteria, Done when, Expected. Rules by kind and risk: a bug ships a test that fails before the fix; a feature has the suites the diff touches green and each criterion names its test (a criterion with no test is a warning, not a block); doc and chore skip the tests; low risk skips the plan. |
| Statuses are the stages | The status is the floor, as in `dev-cycle.md`. Each step's button moves the status: Move to ready gives `ready`; Approve the plan, or Start work at low risk, gives `in-progress`; Mark ready for review gives `in-review`; the merge gives the first validation status; each sign-off gives the next validation status, then the closing status. With no validation status (git-board's own board), Verify happens in `in-review` once the PR is merged, and its sign-off closes the issue. A card dragged on the board still works: the step follows the status, with a note when the evidence disagrees. |
| Step 3 starts the branch | `wtx go` starts the agent in plan mode, and a draft PR holds the plan; a per-project switch keeps plans out of PRs. Revise goes through `wtx brief`. Approve takes the agent out of plan mode. Review opens when the draft PR is marked ready. At low risk the branch starts at step 4. |
| Step 4 runs tests | `[cycle.tests]` declares suites, each with the files it covers; the suites the diff touches come pre-ticked. git-board runs the fast suites in the worktree: only commands from the toml, as an argv, never input from the page. Slow suites (e2e, Lighthouse, performance) are CI's. `[cycle.checks]` (lint, format, type-check) always run locally and gate "Mark ready for review". git-board finds the checks in lefthook's pre-commit, else the Makefile or `package.json`, and proposes them for a person to confirm, as Enable binary release infers `make dist`. |
| Each step has its discussion | A comment carries its step as git-bug metadata. The full conversation keeps every comment in order, as GitHub shows it. The agent takes part: "Send to Claude Code" (or OpenCode) briefs it. The agent never decides pass or fail. |

## The steps

The lock reasons are C1's words.

| # | Step | Opens | Its button | Moves the status to |
| --- | --- | --- | --- | --- |
| 1 | Intent | always | Move to ready, once the description lists a criterion | `ready` |
| 2 | Risk | at ready | Low, Normal, Structural | (the `risk` field) |
| 3 | Plan | once the risk is set; skipped at low risk | Approve the plan; Ask the agent to revise. Start work at low risk | first `in-progress` |
| 4 | Tests | when work starts; skipped for a kind whose rule is `none` | Run the selected (M4) | |
| 5 | Code | with the test, or at once when step 4 is skipped | Mark ready for review | first `in-review` |
| 6 | Review | when the PR is ready for review | Approve, Request changes (M5); Start verification once merged | first validation status |
| 7 | Verify | when the PR is merged | Sign off, Send back to in-progress | next validation status, then the closing status |

## The protocol in `.git-board.toml`

Every key below shows its default, and the milestone that reads it. A key a
milestone hasn't built yet is a warning in Settings, as any unknown key is
today.

```toml
[cycle]
ready = "ready"                  # the stages: guessed from the status names when left out
in-progress = ["in-progress"]
in-review = ["in-review"]
validation = []                  # none: Verify happens in in-review once merged
criteria = ["Acceptance criteria", "Done when", "Expected"]  # M1: headings, any level or bold, any case
plans = "docs/src/implementation-plans"
test-files = ["*_test.go", "*.test.ts"]  # M1's test-file patterns, named `tests` until M4
signers = ".allowed_signers"     # M4: the SSH keys whose results count, read from the base branch
plans-in-pr = true               # M3: the draft PR holds the plan
risk = "lead-dev"                # the role holding each gate; left out: anyone
plan = "lead-dev"
review = "lead-dev"

[cycle.kinds]                    # M1: step 4's rule by kind
bug = "failing-test"             # a test that fails before the fix
feat = "per-criterion"           # the touched suites green, each criterion names its test
doc = "none"                     # step 4 skipped
chore = "none"

[cycle.signoffs]                 # a validation status = the role that signs it off
qa = "qa"

[cycle.tests.go]                 # M4: a suite and the files it covers
run = ["go", "test", "./..."]    # an argv, from the base branch's file only, never from the page
files = ["**/*.go", "go.mod", "go.sum"]  # everything it reads: the result's content key
where = "worktree"               # or "ci": e2e, Lighthouse, performance

[cycle.checks.lint]              # M4: always local, gates Mark ready for review
run = ["make", "lint"]           # proposed from lefthook's pre-commit, else the Makefile or package.json
```

The kind is the one `[branches]` already reads: the `kind:` field, else a
flat label listed in `[branches] kinds`. An issue with no kind, or a kind
the table doesn't list, follows `per-criterion`, which is today's step 4.
Risk keeps its fixed rule, low skips the plan: no key until a project needs
another.

## Calls taken

The decisions leave these to the builder. They are mine, of 2026-10-22,
and two of them need the user's eye.

1. **The defaults apply without a `[cycle]` table.** Decision 2 asks both
   for "no `[cycle]` table means today's behaviour" and for these
   defaults. The two disagree twice: an unconfigured board now also finds
   criteria under "Done when" and "Expected", and its doc and chore issues
   skip step 4. I take the defaults. The first fixes GB-31's bug on every
   board, and the second is the rule decided for doc and chore. Everything
   else an unconfigured board does is unchanged.
2. **The merge is a button, not a write on read.** Decision 3 says the
   merge moves the status to the first validation status. git-board learns
   of a merge when it reads the PR from GitHub, during a GET. Writing a
   status there would make reading an issue change it, and two servers
   reading at once would both write. So step 6 shows "Start verification"
   once the PR is merged, and the press moves the status. With no
   validation status there is nothing to move: step 7 opens in `in-review`
   as soon as the PR reads merged.
3. **The step buttons PATCH the status from the page**, as setting the risk
   does today. The server names the button as one of the step's actions;
   the page sends the status the stages name. Approve the plan is the one
   move made by the server, after its comment. M4 moves Mark ready for
   review to the server, when the checks gate it.
4. **A comment's step is the metadata `cycle:step`**, "1" to "7". The gate
   comments (plan approval, run, criterion checked, sign-off) carry it too,
   so each step's discussion holds its own events. A comment without it is
   step 1's: every comment written before this change. The fork's bridge
   skips only `review:*` keys and PR comments, so a step's comment still
   exports to the forge as a plain comment.
5. **The step names follow the canvas**: Intent, Risk, Plan, Tests, Code,
   Review, Verify. "Risk class" and "Tests first" go.
6. **The page's right column is the discussion**: "This step", "All" (the
   full conversation with its timeline events, as the Conversation tab is
   today) and "History". The fields and labels stack under it. On the page,
   step 6 holds the Changes view, which M5 reworks.
7. **`?view=cycle` goes.** The Work page links to the issue's page. No
   `?step=` link until someone needs one.

## Milestones

Each ships on its own branch, in this order. This branch builds M1 and M2.

### M1: the protocol

`[cycle] criteria` and `[cycle.kinds]` with their defaults; step 4 skipped
by kind; the step buttons that move the status; `cycle:step` on comments;
Verify in `in-review` when there is no validation status.

Done when:
- GB-31's four criteria under "Expected" count, and a project's own
  heading in `criteria` is read;
- a doc issue at `in-progress` is at step 5, with step 4 skipped;
- Move to ready, Approve the plan, Start work, Mark ready for review and
  Start verification are the step's actions for the right person, and each
  moves the status;
- with no validation status, a merged PR in `in-review` is at step 7, and
  its sign-off closes the issue;
- a comment posted with a step reads back with its `cycle:step`;
- `go test ./internal/...` passes.

### M2: one page

The step bar, its locks and their reasons; the current step as the main
section; each step's discussion; step 1 as the description. The Issue /
Cycle switch, `IssueCycleView` and `IssueCycle` go.

Done when:
- the switch and `?view=cycle` are gone, and the dialog and the page show
  the same frame;
- a locked step says why, and a done step reopens;
- "Save the criteria" writes under the description's own heading, never a
  second list;
- a comment posted on step 3 shows under "This step" there, and under
  "All" in order;
- the page at 1440px and 900px matches C1 to C3;
- `npm run typecheck && npm test` pass.

### M3: step 3 with wtx

Start opens the branch with `wtx go` in plan mode and a draft PR holding
the plan; Revise through `wtx brief`; Approve takes the agent out of plan
mode; `plans-in-pr`. Done when a normal-risk issue's Start opens a worktree
and a draft PR with the plan, and Review unlocks when that PR is marked
ready.

### M4: step 4, the tests and the checks

`[cycle.tests.*]` and `[cycle.checks.*]`; the checks proposed from
lefthook, the Makefile or `package.json`; Mark ready for review gated on
the server. Done when the touched suites come pre-ticked, a fast suite runs
in the worktree from the toml's argv, a red check blocks Mark ready for
review, and the bug and feature rules are told apart.

### M5: step 6, review

The Changes view as C6's review screen: threads sent to the agent, Approve
and Request changes. Done when the page matches C6.

### M6: steps 2, 5 and 7

C2's three options with the AI's suggestion; C5's commits and the PR's
title and description; C7's checklist with who and when, and the run at the
merged head. Done when the pages match C2, C5 and C7.

## Out of scope

- Environments for e2e suites.
- Reading CI results back, and running CI locally.
- GB-21.

## Constraints

- git-board runs only commands declared in the toml, as an argv. Nothing
  from the page reaches a command line.
- The agent drafts, fixes and answers. It never decides pass or fail.
- Live checks go to scratch repositories only: guilbep/git-board-test,
  guilbep-sandbox. Never co2-calculator.
- No real person's name in mocks, fixtures or this file.
