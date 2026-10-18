# The step comes from the status and the evidence

PRD, 2026-10-18. Designed from the team's eight-step loop ("How we build
with agents: 8 steps", the proposal deck), the design canvas (artboards E,
F, G, H) and `work-unit.md`, which this builds on: the issue is the unit of
work, its branches are linked by name, the PR is the gate. The layout and
the pages were chosen on the canvas before this file; the open points of
the brief are settled here and marked as such.

## The problem

An issue has a status and, since M3, a PR gate. Nothing says which step of
the loop it is in, whose turn it is, or what its gate waits for. The
developer knows, the PM asks, the validators find out when someone moves
the status. Agents wait for an answer in a terminal nobody is looking at.
Review, the one step that costs human time, gets a diff with no plan beside
it and no record of what the bots already found. And what a reviewer
corrects an agent on is gone once the thread scrolls away: the same
correction is made three times, on three issues, by hand.

**git-board shows each issue's step and what its gate waits for. Humans
decide and can explain the result; agents draft, type and check.** The step
is never typed in: it comes from the status and the evidence (a risk, a
plan file, a test in the diff, a PR, a sign-off). Statuses stay the
project's own.

## What the deck and co2 showed

The loop, from the deck (draft v0.1, to validate together):

| Step | What happens | Gate, done when | An issue is here when |
| --- | --- | --- | --- |
| 1 Intent | Issue with acceptance criteria, from the template; the agent may draft it | No issue, no agent | Before the first stage status, or at `ready` with no criteria |
| 2 Risk class | Low, normal or structural: data model, migrations, auth, pipeline, emission totals, cross-module | Class written on the issue | `ready`, no risk set |
| 3 Plan | The agent drafts options and a plan; we argue, cut it back, write down why | Structural: plan PR approved first | `ready`, risk set, plan not approved |
| 4 Tests first | A behaviour or regression test that fails before the fix | Every bug fix ships its test | `in-progress`, no test in the diff |
| 5 Code | One worktree per agent; local database only; no secrets on disk | A human commits and opens the PR | `in-progress`, a test in the diff |
| 6 Review | Bots first (CI, secret scan, AI review), then a human who can explain the diff | Structural: a second human | `in-review` |
| 7 Verify | Run the whole app, read the full output; mark the plan delivered | Green codes are not proof | A validation status |
| 8 Learn | At each retro, move agent corrections into shared rules, then into hooks and CI | One rule down the stack per retro | The sprint, not the issue |

"Review is the bottleneck: human time. Steps 1 to 3 exist so that step 6
stays short." The deck leaves five points to the team: may an agent commit
or push by itself; sandbox by default or only for risky tasks; which code
and data may go to external models; who owns the shared rules; which KPIs
(rework, change-failure rate, plan-first rate, cost). Its kit,
it4r-agent-kit, already shares wtx (one worktree, session and agent per
branch), the plan-conventions skill (options, rejected alternatives, plan
PR first for structural work) and EPFL-hosted models; risk classes and
"agent memory to shared rules" are marked "to build".

co2-calculator, read in the local clone on 2026-10-18:

- Nine statuses: `backlog, comite, ready, in-progress, in-review,
  ready-for-validation, qa, tests-metier, validated`. The last three before
  `validated` are each a different person's check, done today by moving the
  status by hand. Nothing in the repository moves a status automatically.
- No `[roles]`; fields `prio`, `est`, `assignee`; `[branches] base = "dev"`,
  `name = "{kind}/{number}-{slug}"`; a `dev` board with lanes from `ready`
  to `ready-for-validation`.
- `lefthook.yml` runs the secret scan first at pre-commit ("the repo is
  public, so a committed secret is a published one"), then format, lint
  and type-check; it appends ` (#NNNN)` to the commit subject from the
  branch name; pre-push runs `make ci` on `dev`, `stage` and `main` only.
  The workflows `security.yml`, `quality-check.yml`, `test.yml` and
  `integration-tests.yml` are step 6's bots today.
- Plans live in `docs/src/implementation-plans/` with the kit's front
  matter (`status`, `issue`, `last_updated`, `summary`), and some already
  carry an "Acceptance criteria" heading.

The plan-conventions skill fixes the plan's lifecycle: `draft`, `accepted`,
`in-progress`, `delivered`, `deferred`, then `rejected`, `withdrawn`,
`superseded` in `archive/`. "If the PR diverges from its plan, update the
plan in the same PR." This repository's own `wtx.toml` shows the
orchestration a project already runs: plan on one model, build on another,
`[permissions] deny` for what the agent may never run.

## Decisions

| Topic | Decision |
| --- | --- |
| The step | Computed by the server from the status and the evidence, never stored. Seven steps per issue; Learn belongs to the sprint. `GET /api/issues/{id}/cycle` returns the step, the risk, what it waits for (a role or a person, and what) and the evidence it used. |
| Stages | `[cycle]` in `.git-board.toml` maps the project's statuses to four stages: `ready`, `in-progress`, `in-review` and an ordered `validation` list. Defaults are guessed from the status names, as `colors.go` guesses colors; Settings › Project file shows what was guessed. Statuses before `ready` are before the first stage. |
| Step rules | Intent: before `ready`, or `ready` with no acceptance criteria. Risk class: `ready`, no risk. Plan: `ready`, risk set, plan not approved. Tests first: `in-progress`, no test file in the diff. Code: `in-progress`, a test in the diff. Review: `in-review`. Verify: a validation status. Done: the closing status. **The status is the floor**: evidence moves the step within a stage (2 from 3, 4 from 5), never below it, so co2's existing issues without criteria don't fall back to step 1. Evidence that disagrees with the status (a PR merged while `in-review`, a PR open while `ready`) is a note on the step, "merged, waits for the status", never another step. |
| Which branch | An issue's branches are linked by name (work-unit). The evidence branch is the one matching `[branches] name` for the issue's number, the newest head when there are several. `plan/{number}-…` branches carry the plan only and are never the evidence branch. |
| Per risk class | Low: step 3 is skipped (grey on the spine, "skipped, low risk"); the plan is backfilled as the kit asks, and its absence is a warning at Verify, not a gate. Normal: the plan is approved from git-board by the plan role's person. Structural: the plan PR is approved on GitHub before code, and review needs a second human approval. |
| Acceptance criteria | A convention: an "Acceptance criteria" heading in the description, followed by a list. Step 1's evidence, step 7's checklist. The AI may draft them (G2); a person edits and keeps them. |
| Risk | A `select` field `risk`, options `low`, `normal`, `structural`, single; `[cycle] risk` names it if the project calls it otherwise. Shown with the title, as the canvas does ("Risk: structural"). |
| The plan file | Found by convention: a file under `[cycle] plans` (default `docs/src/implementation-plans/`) whose front matter `issue:` names the issue (`CO2-N`, `#N` or the git-bug id), read from the evidence branch, else the plan branch, else the base. `status: accepted` or later means approved; `delivered` at the merged head is step 7's "plan marked delivered". git-board reads plans and never writes them: the plan is code, and a person commits code. |
| Approve the plan | Normal: the button posts a comment with `plan:approve` and `plan:commit` (the plan file's blob at that moment) by the plan role's person. Structural: the button posts a GitHub approval on the plan PR under the viewer's token, as M4's approvals do; approved means the plan PR's review decision is APPROVED or the PR is merged. |
| Plan steps to files | Convention first: the plan's task headings and their `**Files:**` lists (the shape the kit's plans already have). A changed file no task names is "Not in the plan". A plan without file lists gets an AI guess when AI is on, labelled as a guess. |
| Tests first | A file matching `[cycle] tests` (defaults `*_test.go`, `*.test.ts`, `*.spec.ts`, `test_*.py`, `tests/**`) in `git diff base...branch --name-only`. The commit that first added it is shown. |
| Whose turn | The viewer is the server's identity (`--me`, `GIT_BOARD_ME`, the `@me` of filters). `[cycle]` names a role per gate, roles from `[roles]` (M2): `risk = "pm"`, `plan = "lead-dev"`, `review = "reviewer"`, and `[cycle.signoffs] <status> = <role>`; the assignee owns Tests first and Code. A gate whose role is unset, or names nobody (co2 has no `[roles]`), is anyone's: its card shows to everyone and the Work page lists it under Team, not Mine. |
| Local first | The step is computed from local git and git-bug data. PR state (`internal/ghpr`) comes from a per-branch cache the server fills when an issue is opened or the Work page asks, at most every few minutes and only for issues at `in-review` or later with a pushed branch. The Work page never calls GitHub once per issue. Offline, the step still computes and the cache shows its age. |
| Code and who commits | A G1 setting with two values, written as `wtx.toml` deny rules: **a person commits and opens the PR** (default, the deck's gate: the agent may not run `git commit`, `git push`, `gh pr create`), or **the agent may commit, a person pushes** (push denied only). The deck leaves this to the team; the setting makes the choice explicit per repository, and the Cycle view reads step 5's gate from it. |
| Review | Builds on work-unit M4's Changes tab. Bots first (CI checks, the secret scan check, AI review comments), then the diff beside its plan. Approving asks for one line explaining the change; the line is the review's body ("don't merge what you can't explain"). The AI never approves or requests changes. |
| Agent corrections | "Flag as an agent correction" sets `correction` metadata on a review comment; `correction:layer` and `correction:to` hold where it lives and where the retro moves it. F2 gathers them per sprint. |
| Sign-offs | One per validation status: a comment with `signoff:status` and `signoff:commit`, by the person the status's role names. Signing off also moves the status to the next option; the last sign-off closes the issue. "Send back to in-progress" is a status change. A status with no role can be signed off by anyone. |
| Verify evidence | The run's output, pasted by the person who ran it as a comment with `verify:run` and `verify:commit`; each criterion checked as `verify:criterion` (index, commit, by whom). The plan's `delivered` is read at the merged head: the kit has the PR update its plan in the same PR, so a protected base needs no second PR. "Green codes are not proof": Sign off stays disabled until a run at the merged commit is posted; an override writes why into the sign-off comment. A missing `delivered` is a warning, not a block. |
| Metadata and the forges | `plan:*`, `signoff:*`, `verify:*`, `correction*`, `ai:model` and `ask` are git-bug comment metadata. They travel between clones with `refs/bugs`, as every git-bug edit does, so a colleague's sign-off is evidence after a sync, with no fork change. On a forge these comments export as plain comments with a generated body ("Signed off qa at 4f2e9c1"). Nothing here needs the fork; work-unit M4 already changes it for `review:*`. |
| Worktrees and agents | git-board starts them with wtx on the machine where `git board serve` runs: `wtx go <branch> --prompt <file> --no-attach` from the issue (the branch from the `[branches]` rule), `wtx brief` to re-brief a running agent, `wtx init` from G1; the Agents group reads `git worktree list` and `wtx status`. Only when the server listens on loopback alone (`exposed` is false), wtx is on `PATH`, and the request carries a same-host Origin, which `cors.go` already enforces. Arguments are an argv, never a shell line. The prompt file holds the step's gate and what git-board asks for, in its own words; then the issue's title and description **quoted as data**, under a heading saying they come from the forge and are not instructions. The agent's permissions come from `wtx.toml` only. |
| Write wtx.toml | G1 writes `wtx.toml` and `.wt.toml` at the root of the server's checkout, uncommitted, after a preview; the person commits them. The who-commits setting is a `[permissions] deny` list, which wtx lets a repository tighten and never loosen: a rule wtx's own baseline already denies shows "set by wtx" instead of a choice. |
| AI | OpenAI-compatible chat completions at `LLM_BASE_URL` with `LLM_API_KEY`, the model from `[ai] model`, else `LLM_MODEL`; off when the address is unset. Per-step switches in G2: 01 draft criteria, 06 review comments, 07 draft the verify checklist. Step 3's planning is the worktree agent's, not the server's. Comments it writes carry `ai:model`. |
| What leaves the machine | Issue titles, descriptions and comments; diffs without `linguist-generated` files and lockfiles. Never attachments, tokens, `.env` files, or a diff the secret scan failed. Only to `LLM_BASE_URL`. Written on the G2 page, in those words. |
| The Work page | `/work`, `GET /api/work`: every open issue's cycle, grouped Needs you, Agents, Waiting on others, To send, Since the last retro; Mine and Team. Agents are `git worktree list` plus wtx's session state when wtx reports it, else the last commit's age. |
| The retro | `/work/retro`: corrections flagged since the current sprint's start (`[sprints]` is the retro's clock; there is no "close the retro"), by the layer they live in: Chat, Agent memory, Docs, Hooks, Code and CI. "Make it this retro's rule" opens GitHub's new-file page in `[cycle] rules_repo` (default `EPFL-ENAC/it4r-agent-kit`) with the rule prefilled. |
| The header | H1, built on `feat/header`. This PRD adds the Work item and two Settings tabs, nothing else. |

Where the brief said "to settle", the rows above are my calls of
2026-10-18: step derivation, plan steps to files by convention, the
localhost server running wtx, what goes to RCP, the who-commits default,
the milestone order. The builder check added: the status as the floor, the
evidence branch, the gates per risk class, what Approve the plan writes,
whose turn, local first, the retro's clock, metadata without fork work,
how `wtx.toml` is written, the prompt quoting the issue as data.

Scored alternatives, for the record: a stored `step` field (2/10: lies as
soon as the evidence moves), statuses renamed to steps (3: the statuses
are the PM's), AI-only mapping of plan steps to files (4: a guess where a
convention is free), a signed token for the wtx endpoints (5: loopback and
the Origin check already decide who is local; the upgrade if the server is
ever exposed), git-board writing the plan's `status:` (3: the plan is
code), a Learn step on the issue (2: a correction is the sprint's).

## Milestones

Each ships on its own, in this order: the step is the base; the Work page
needs only it; the review workspace waits for work-unit M4; worktrees, AI
and the retro come last because each can be off.

### M1: the step

Done: plan in `dev-cycle-m1-plan.md`.

```toml
[fields]
risk = { options = ["low", "normal", "structural"], single = true }

[cycle]
ready = "ready"
in-progress = ["in-progress"]
in-review = ["in-review"]
validation = ["ready-for-validation", "qa", "tests-metier"]
plans = "docs/src/implementation-plans"
risk = "pm"
plan = "lead-dev"
review = "reviewer"

[cycle.signoffs]
ready-for-validation = "lead-dev"
qa = "pm"
tests-metier = "metier"
```

Every key has a default: the stages are guessed from the status names, the
plans directory is the kit's, the roles are unset (anyone's turn). A
project writes only what the guess got wrong.

- **The computation** (`internal/cycle`): status to stage, then the
  evidence for that stage: the risk field; the plan file's front matter
  from the evidence branch, the plan branch or the base; `git diff
  base...branch --name-only` against the test patterns; the cached PR;
  the `plan:*`, `signoff:*` and `verify:*` comments. The result names the
  step, the gate's text from the loop ("Structural: plan PR approved
  first"), who it waits for and the evidence it used, so the UI never
  re-derives anything.
- **Shown** in the Issue view: a progress line under the title, "Step 7 of
  7 · Verify, waits for Person B to sign off qa", and the next-step card
  when it is the viewer's turn: Set the risk (three buttons), Approve the
  plan, Review, Sign off. Nothing when it is not.
- **Written**: the risk is a field edit; a sign-off, a plan approval and a
  verify check are comments with metadata, posted by `POST
  /api/issues/{id}/cycle/{action}`, which checks the viewer against the
  role and refuses with "waits for the pm" otherwise.

Done when: with co2's statuses, a fixture issue walks all seven steps in a
Go table test, each from status plus evidence, once per risk class (low
skips step 3); a status alone never tells step 2 from step 3; an issue with
no criteria at `in-progress` stays at step 4 or 5; the card shows only to
the role's person, and to everyone when the role has none.

Calls taken building it:
- **The risk field is always `risk`.** No `[cycle]` key renames it (see
  Open, not blocking).
- **The plan approved, or low risk:** step 3 waits for the assignee to move
  the status to in-progress.
- **Criteria written before `ready`:** step 1 waits for the author to move
  the status to ready.
- **A structural plan is approved on its PR.** git-board reads the PR's
  approval from GitHub and posts none: the card links to the PR until
  work-unit M4's approvals exist.
- **`verify` and `check` are M1 actions.** A run's output and each
  criterion are comments, so step 7 can be reached without M2's workspace.
- **The sign-off's run check uses the branch head.** M2 checks the merged
  head instead.
- **What git-board's identity wrote is the viewer's.** The server writes as
  that identity, and `--me` names the viewer, so the identity's comments
  count as the viewer's. Anyone else's count under their forge login.
- **Settings doesn't show the guessed stages yet.** That is left to M2. The
  API already answers `stages.guessed`.

### M2: the Cycle view

The issue page gets an **Issue / Cycle** switch (E1, E3, E4 on the canvas).

- **The spine**: the seven steps down the left, each with its one-line
  evidence ("Plan PR #3089, waits for approval", "Failed on 2c8e4a1,
  before the fix", "Sign-off 2 of 3"); the current step open. Under
  900px, and in the Issue view, the spine folds to the stepper line.
- **Plan workspace**: the plan's front matter, its options table as the
  plan writes it, the "why" paragraph, Read the whole plan; the gate; who
  drafted it and where ("Drafted by the agent in `plan/421-…`, two rounds
  of comments"); **Approve the plan** and **Ask the agent to revise**. The
  latter posts a comment on the issue; M5 also sends it to the agent.
- **Verify workspace**: the full run (who, where, at which commit, the
  output), the acceptance criteria as a checklist with who checked each,
  the plan marked delivered, the sign-offs one per validation status with
  their person and date; **Sign off <status>**, **Send back to
  in-progress**.
- **The status bar** along the bottom: branch, worktree, agent, checks,
  what waits for the forges. Each reads what the side already knows
  (Development, On the forges); it adds nothing new to fetch.
- **Open as page** gives the Cycle view the window's width; the Issue view
  keeps today's dialog.

Done when: E1, E3 and E4 are reproduced against a fixture board in the
browser at 1440px and 900px; a sign-off moves the status and shows in
History; the Issue view is today's page plus the progress line and the
card.

### M3: the Work page

`/work` (F1), the header's Work item: "Human time is the bottleneck. Agents
wait here, not in a terminal." Mine and Team.

- **Needs you**: issues whose gate waits for the viewer, oldest wait first:
  Issue, Step, Waits for, Since, and the action (Review, Answer, Read the
  plan, the three risk buttons). An **Answer** row is an agent's question:
  a comment with `ask` metadata, which the brief tells the agent to set
  through the CLI; until agents do, the newest comment by the agent that
  ends in a question mark.
- **Agents**: one row per worktree, with its branch, issue, step and state.
- **Waiting on others**: the viewer's issues whose gate waits for someone
  else, with who and since when.
- **To send**: what waits for the forges, Send all, Check first (the Sync
  page's data).
- **Since the last retro**: the count of corrections flagged this sprint,
  Open the retro.

`GET /api/work` returns the groups computed server-side from every open
issue's cycle; it is the Inbox's shape (`internal/inbox`) with the cycle as
its source.

Done when: the fixture board shows F1's rows in F1's order; an issue
appears in exactly one of Needs you and Waiting on others; the page is
empty and says so when nothing waits.

### M4: the Review workspace

After work-unit M4 lands (the Changes tab, line comments, approvals). E2 on
the canvas: "Read #3095 against its plan".

- **Bots first**: the checks (CI, the secret scan) and the AI review's
  findings with how many are resolved, before any code.
- **The plan, step by step**: each task of the plan with the files it
  named, ticked when the diff touches them; **Not in the plan** for the
  changed files no task names.
- **The diff** is M4's, with its comments; an AI comment carries **Flag
  as an agent correction** beside Reply and Resolve.
- **People**: who approved at which commit; the structural gate ("a second
  human, Merge into dev waits for it").
- **Approve** asks for one line, "Explain the change in one line, to
  approve it", and refuses an empty one; Request changes and Comment as
  M4.

Done when: E2 is reproduced; a changed file no task names is flagged; an
approval without a line is refused; a flagged comment shows in F1's count.

### M5: Worktrees

Settings › Worktrees (G1), and the agent actions.

- **The page**: whether wtx, wt and tmux are on this machine; what wtx read
  in the repository (base branch from origin's HEAD, protected branches,
  ports, panes, the agent's plan and build models); **Who commits**, the
  two values; **Never allowed**, rules added to wtx's own; **Write
  wtx.toml**, with **Preview the files** first: "Writes wtx.toml and
  .wt.toml. You commit them."
- **Start an agent** on the Cycle view's status bar and the Work page,
  when the issue has no worktree: writes the prompt file, runs `wtx go`,
  shows the session's name. **Brief the agent** when it has one: a text
  box, `wtx brief`. Ask the agent to revise (M2) becomes a brief.
- **Refused** with the reason when the server is exposed beyond loopback,
  wtx is missing, or there is no `wtx.toml` yet.
- **Agents** on the Work page read the worktrees and `wtx status`.

Done when: from a fixture issue on this machine, Start an agent creates
the worktree and the tmux session, and the prompt file holds the gate and
the quoted issue; the same request against `--addr 0.0.0.0:8080` is
refused; Write wtx.toml produces the preview byte for byte and leaves the
tree uncommitted.

### M6: AI

Settings › AI (G2): "Drafts and checks, never decisions."

- **The page**: on or off, the model and the address, Test the
  connection; where each setting is set and its current value (the key:
  "Set, never shown"); the per-step switches with what each does and who
  keeps the result; **What leaves this machine**.
- **01 Draft the criteria**: from a thin description, into the editor, not
  the issue; the person keeps them.
- **06 Review**: when the Cycle view or the Work page loads and the
  branch's head moved since the last `ai:model` comment, the server sends
  the diff (filtered as the page says) and the plan, and posts the
  findings as line comments. Never an approval or a request for changes.
- **07 Draft the verify checklist**: from the criteria and the diff, into
  the Verify workspace as unchecked items; the people signing off keep,
  edit or drop each.

Done when: with `LLM_BASE_URL` unset every switch is off and no request
leaves; a recorded fixture reply becomes comments with `ai:model`; the
review switch never produces a review state; a `linguist-generated` file
and a lockfile are absent from what is sent.

### M7: the retro

`/work/retro` (F2): "One rule down the stack."

- **The ladder**: Chat, Agent memory, Docs, Hooks, Code and CI, each with
  how long it lasts and how many of this sprint's corrections live there.
- **The table**: Correction, Flagged on (issue, reviewer, how many times),
  Lives in, Move it to.
- **Make it this retro's rule**: opens GitHub's new-file page in the rules
  repository with the rule prefilled, "so every IT4R project gets it".

Done when: three flagged comments from the fixture sprint appear under
their layer; moving one writes `correction:to`; the rule opens prefilled on
GitHub.

## The questions the brief left open

1. *How is the step computed from status plus evidence?* Status is the
   floor, evidence moves within the stage (Step rules). Seven rules, one
   table test per risk class.
2. *Plan steps to files, a convention or AI?* The plan's `**Files:**`
   lists; AI only as a labelled guess when a plan has none.
3. *The unauthenticated localhost server running wtx?* Loopback-only,
   same-host Origin, argv not shell, the issue quoted as data; the agent's
   permissions come from `wtx.toml`. A signed token is the upgrade if the
   server is ever exposed.
4. *Which code and data go to RCP?* Titles, descriptions, comments, diffs
   without generated files and lockfiles; never attachments, tokens or a
   diff the secret scan failed. Only to `LLM_BASE_URL`.
5. *Who commits?* A repository setting; the default is the deck's gate. The
   team's agreement decides the default for every project; git-board shows
   what the repository chose.
6. *The milestone order?* Step, Cycle view, Work page, Review, Worktrees,
   AI, Retro: value first, dependencies honoured, the switchable parts
   last.

## Out of scope

- Releases (`docs/prd/release.md` on `prd/release`).
- Gates enforced by a hosted git-board; GitLab merge requests as gates.
- The header itself: `feat/header`, artboard H1.
- The agent's own tools: wtx, the kit's skills, the models. git-board
  calls them and shows their output; it replaces none of them.
- The deck's KPIs beyond what "Since the last retro" counts, and the
  sandbox default: the team's agreement, not this PRD.

## Open, not blocking

- A `[cycle]` key naming the risk field, for a project whose field isn't
  `risk`.
- The CI run where the test failed before the fix: per-commit check runs
  from GitHub, once the test's first commit is known.
- Telling an agent's commits from a person's, for step 5's gate.
- A real PR to the rules repository through the API, in place of GitHub's
  new-file page.
- Block the sign-off when a required check is red on the merged commit.
- A Work page across several repositories.
- `[cycle]` for a status order other than the options' order.
- An agent posting its own run (`verify:run`) and questions (`ask`) through
  the CLI: the convention is named here, the CLI command is not designed.
- Showing the plan's options table when a plan writes it in another shape
  than the kit's.

## Constraints

- One server per repository at a time: git-bug locks its cache in the main
  checkout's `.git/git-bug/lock`, worktrees included.
- `../co2-calculator` is read-only and never pushed to; its statuses and
  plans are the fixture, not a test bed.
- Live tests only on the scratch forges: guilbep/git-board-test,
  guilbep/git-bug-sync-test, gitlab.epfl.ch project 42400.
- `LLM_API_KEY` is set in the environment where the server runs, never
  shown and never written to the repository, like the attachments' S3 keys.
- No real person's name in mocks, fixtures or this file: Person A, B, C.
- Any comment metadata the forge bridge must carry goes through the fork's
  `bridge-sync`, as work-unit's M4 does; this PRD adds none that must.
