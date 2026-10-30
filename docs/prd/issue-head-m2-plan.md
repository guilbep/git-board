# The issue head, finished: plan

**Spec:** `docs/prd/issue-head.md` (its *Decisions*), on
`docs/design/issue-head/README.md` and the screens of rows 20 and 21
(`docs/design/issue-head/screens/V*.html`, `W*.html`). The first pass is
`docs/prd/issue-head-plan.md`; its work sits uncommitted on `main` and goes
on a branch first (`docs/prd/issue-head-handover.md`, *First, before any
commit*): every milestone here starts from that branch once it is landed,
or from its worktree before. Not one branch: each milestone is a day or
less, ships on its own branch and worktree (`wtx go`), and leaves the app
working. The table says what each needs first; the retirement of v1 comes
last but for the docs.

## Global constraints

The first plan's hold, in short: the EPFL design system as `web/README.md`
says it, the adherence linter on every file touched (no new problem; a file
a milestone rewrites ends with none); a pure function in `web/src/<name>.ts`
with its `node --test` file in `web/package.json`'s `test` script; a new Go
test calls `t.Parallel()` first unless it chdirs, sets an environment
variable or swaps a package variable; real fixtures over mocks; no real
person's name in mocks, fixtures or tests; git-board runs only commands
written in its code or the toml, nothing from the page; the package's own
tests while working, `make ci` once at the end of each milestone, `make
docs` when a milestone touches `docs/`. And:

- A state the bar shows twice (Fit or Fill, Comfortable or Compact: a
  segmented control above 56rem, a More item below) reads one `ref`
  (`headswitch.ts`'s `headWidth`, `headDensity`); no second store.
- The web tests have no DOM: a CSS change is checked by eye against the
  screens named in its milestone, at 1440px and the widths it names, and
  the milestone says so in its *Done when*.
- A removed component is grepped for first (`grep -rn "<Name" web/src`);
  a removed CSS rule too (`grep -rn "gb-<class>" web/src`), so no other
  page loses a style.
- Nothing lands on `main`, nothing is tagged (`CLAUDE.md`); the user runs
  `wtx land <branch>`.

## Review focus

1. The two controls for one state: the bar's *Fill* and More's *Fill the
   panel* disagreeing after a click on either (H-1).
2. A "↓N" that outlives what it says: it must clear on any run that touches
   the issue, a full sync from Settings included, and never show from a
   dry run of another issue (H-5).
3. *Out of reach* and *changed on both sides* pinned to the wrong forge or
   issue: the bridge's name against `forge.name`, the check kept per issue
   and cleared on its next run (H-4).
4. The old head's removal taking a control that has no home in the new
   one: H-11's table of homes, checked against `IssueSide.vue` before the
   delete.
5. A color that changes with the move to `tokens/values.css` and
   `tokens/dark.css`: the same custom properties, the same values, loaded
   before `gitboard.css` (H-7).

## The milestones

| ID | Milestone | Wave | Needs | Decision | Size |
| --- | --- | --- | --- | --- | --- |
| H-1 | The bar whole at every width | A | — | the bar between 30 and 56rem | S |
| H-2 | The red dot under the active tab | A | — | the red dot | S |
| H-3 | `make web-lint` | A | — | the linter | S |
| H-4 | The forge badges wired | A | — | forge badges | M |
| H-5 | New on the forge, after a check | A | H-4 | new on the forge | M |
| H-6 | The cycle's words: branch, review, merge | A | — | the cycle's words | S |
| H-7 | Value and dark colors into token files | A | H-3 | the linter | S |
| H-8 | The head's files to zero problems | A | H-7 | the linter; dead rules | M |
| H-9 | Branch names: the folder form's tests | A | — | — | S |
| H-10 | The PM's check of the arrow | A | — | the arrow without words | S |
| H-11 | The old head retired | B | H-1, H-2, H-4, H-5, H-10 | the old head goes | M |
| D-1 | The docs: README, screens, status update | C | H-11 | the faces on a phone; the crumb | S |

Wave A is ten milestones at once (H-5 after H-4, H-8 after H-7). Wave B needs the new
head complete and the check done. Wave C closes the README.

## Wave A

### H-1 · The bar whole at every width

Decision: *the bar between 30 and 56rem*. Screens: 20c `V3-widths.html`,
20e `V5-bar-moves.html`.

- [ ] `web/src/css/head.css`: a second container step, `@container
  (max-width: 56rem)` on `.is-head-v2.gb-panel`, hides `.gb-ptop-measure`
  (the two segmented controls); the 30rem step keeps what it hides today.
  `.gb-ptop-crumb` gets `min-width: 10rem` at every width, `.gb-ptop-view`
  its ellipsis as today.
- [ ] `web/src/components/IssuePanel.vue`: More gains two
  `menuitemcheckbox` items, `gb-mid-only`, shown by the 56rem step as
  `gb-narrow-only` is by the 30rem one: *Fill the panel* (checked when
  `headWidth === 'fill'`, toggles it) and *Compact spacing* (today's item,
  now from 56rem). Under 30rem the three narrow items follow, as today.
- [ ] `web/src/headswitch.ts`: unchanged; both controls read and set the
  same refs.

Tests: none in Node (CSS and a template). Done when, in the app with the
side panel dragged to 1024, 800, 704 and 416px, the crumb reads whole and
no icon overlaps it, the switches show above 56rem and sit in More below,
and a click in More flips the bar's button and the layout at once; the
adherence linter clean on `head.css`. The screens `V3` and `V5` are
regenerated by D-1.

### H-2 · The red dot under the active tab

Decision: *the red dot*. Screen: 20a `V1-bar-2771.html` (its open box),
row 15b `O2-epfl.html`.

- [ ] `web/src/css/head.css`: `.is-head-v2 .gb-issue-tabs
  .gb-tab[aria-selected="true"]::after` draws a `0.4375rem` by `0.375rem`
  block, `--primary`, `--radius-pill`, centred under the tab
  (`left: 50%; translate: -50% 0; bottom: -0.1875rem`), in place of the
  2px bar; the Compact variant keeps it.

Tests: none in Node. Done when the Conversation and History tabs show the
dot at 1440 and 416px, Comfortable and Compact, and the board's other tabs
(the views' header, Settings) keep their underline; the linter clean.

### H-3 · `make web-lint`

Decision: *the linter and the old problems*. The first plan's W0-9.

- [ ] `Makefile`: `web-lint`, running `web/README.md`'s `npx stylelint`
  command on `web/src/**/*.{css,vue}` against
  `../epfl-design-skill/adherence/.stylelintrc.json` (the path a variable,
  `EPFL_SKILL ?= ../epfl-design-skill`), reporting only, `|| true`; not in
  `ci`.
- [ ] `CONTRIBUTING.md`'s *Workflow* lists it, with the count of the day.

Done when `make web-lint` prints the count (517 on 2026-10-30, less after
the first pass's files) and `make ci` is unchanged.

### H-4 · The forge badges wired

Decision: *forge badges*. The first plan's W2-2. Screen: 20m
`W5-forges.html`, the two "not wired" boxes.

- [ ] First, a check, written down in the milestone's commit: where the
  last run's `left` per forge is read from. `/api/forges` gives
  `ForgeStatus.job` (the run now) and `cut`; if no field holds the last
  run's tallies, `internal/server/forges.go` gains `last: { at, bridges:
  { <name>: { left?: string } } }` from the last completed job, with a
  test in `internal/server/forges_test.go`.
- [ ] `web/src/forges.ts`: `lastCheck`, a `ref<Record<issueId,
  IssueSyncResult>>`: `noteCheck(id, result)` after an `IssueSync` dry
  run on the issue, `clearCheck(id)` after any run that touches it (its
  own sync; a full sync's end, `forgeStatus.job` going from running to
  done). H-5 reads the same store.
- [ ] `web/src/forgestate.ts`: `forgeStates(forges, waitingFor, status,
  last, check)` gives one `ForgeState` per forge from `forgeState`, with:
  `running` when `status.job` runs and is not a dry run; `left` from
  `last.bridges[forge.name].left`; `conflict` when the issue's `check`
  has `conflicts > 0` in that forge's tally. Pure.
- [ ] `web/src/forgestate.test.ts`: out of reach wins over a count and a
  run; a check of another issue shows nothing here; a dry run is not
  syncing; a forge the issue lacks while a bridge exists is *not on it
  yet*.
- [ ] `web/src/components/ForgeIcons.vue` calls it; the popover's state
  words come from the same labels.

Done when each badge shows on a scratch clone in its situation: the
fake forge of `CONTRIBUTING.md`'s *Live tests* stopped (out of reach), a
title changed on both sides then a check (both sides), a run started
(syncing).

### H-5 · New on the forge, after a check

Decision: *new on the forge*. The first plan's W3-6, open point 8.

- [ ] `web/src/forges.ts`: H-4's `lastCheck` is the store; nothing new.
  (Built before H-4: the store is made here and H-4 reuses it.)
- [ ] `web/src/forgestate.ts`: `incomingOf(result: IssueSyncResult):
  Record<string, number>`, the pull side of each forge's tally (`from`,
  its count, as `tallyWords` reads it in `IssueSync.vue`); pure.
  `ForgeState` gains `incoming: number`; the label gains ", N new there
  since the last sync"; the badge is the count pill, "↑6", "↓3" or "↑6 ↓3".
- [ ] `web/src/forgestate.test.ts`: a tally with nothing to pull gives 0;
  both directions give one pill with both; a count alone keeps today's.
- [ ] `web/src/css/head.css`: `.gb-fcount .gb-fcount-in` in `--info-fg`
  with `--fg-inverse` text, beside the yellow part; one pill.
- [ ] `web/src/components/ForgeIcons.vue`: after *Refresh from forges*
  (`IssueSync`'s `synced` with a dry run result), `noteCheck`; the badge
  from `incomingOf(lastCheck[id])`.

Done when, on a scratch clone, a comment posted on the fake forge then
*Refresh* shows "↓1" on that forge until *Send now* or a Settings › Sync
runs, and a check on another issue shows nothing here.

### H-6 · The cycle's words: branch, review, merge

Decision: *the cycle's words*. The first plan's W3-4, finished.

- [ ] `web/src/components/CycleCode.vue`, `CyclePlan.vue`,
  `CycleReview.vue`, `CycleRisk.vue`: "PR #N" and "Draft PR #N" become
  "GitHub #N" and "GitHub #N, a draft"; "plan PR #N" becomes "the plan's
  gate, GitHub #N"; a sentence that explains says the gate, in review,
  merged; `grep -rn "\bPR\b" web/src/components/Cycle*.vue` names what is
  left, each kept on purpose or changed. *Mark ready for review* and
  *Start verification* keep their names.
- [ ] `docs/prd/cycle-protocol.md`: unchanged in its names; a line under
  its words says the pages say "GitHub #N" for the gate.

Tests: `npm run typecheck`; the existing tests. Done when the grep names
nothing but comments and the protocol's own names, and `make docs` is
green.

### H-7 · Value and dark colors into token files

Decision: *the linter and the old problems*. The first plan's W0-2, and
`dark.css`'s half of E-*.

- [ ] A check first: `adherence/.stylelintrc.json`'s `ignoreFiles` or
  overrides for `tokens/`; if the skill's config does not spare a
  `tokens/` folder outside its own, `web/.stylelintrc.json` extends it
  and ignores `src/css/tokens/**`, and `make web-lint` uses that file.
- [ ] `web/src/css/tokens/values.css`: the `:root` block of
  `--gb-<color>-bg/-fg/-dot` and `--gb-done`, moved from `gitboard.css`
  unchanged.
- [ ] `web/src/css/tokens/dark.css`: `dark.css`'s `:root[data-theme="dark"]`
  values, moved; `dark.css` keeps only rules that reference tokens, or
  goes if none is left.
- [ ] `web/src/css/app.css` imports `tokens/values.css` and
  `tokens/dark.css` after `epfl.css`, before `gitboard.css`; the primary
  button's `#871717` fallback goes: `--primary-active` is a token.

Done when `make web-lint`'s count drops by the moved lines, and the board,
a value chip, the head's marks and the dark theme look the same before and
after at 1440px (by eye, the screens `V1` and `W1` beside the app).

### H-8 · The head's files to zero problems

Decision: *the linter and the old problems*; *dead rules*. E-* for the
files the head touched.

- [ ] `web/src/css/gitboard.css`: the remaining problems (px to rem or a
  token, hex to a token, `1px` borders to `var(--border-w)`), file-wide;
  the `.gb-md ul.gb-tasks` rules (four lines) removed.
- [ ] `web/src/components/LabelsSettings.vue`: its `1px`.
- [ ] `web/src/css/app.css`: anything H-7 left.

Done when `make web-lint` names none of these files, and the board, the
drawer and the settings pages look the same at 1440px and 416px (by eye,
the screens `V1`, `V3`, `V7`, `V8` beside the app).

### H-9 · Branch names: the folder form's tests

The first plan's W0-10. No code change: the tests write down what the
folder form relies on.

- [ ] `internal/branches/branches_test.go`: `Names` links
  `test/2771/1-implement-tests` and `2811/1-pdf-footer` to their issues;
  it does not link `test/2771b-x` (a letter glued to the number is not a
  number word); a project number links `feat/co2-421/1-x`. Beside
  `TestNamesInFolders`, which covers the folder form.

Done when `go test ./internal/branches/` passes with the three new cases,
each seen failing first against a `Names` made to link everything.

### H-10 · The PM's check of the arrow

Decision: *the arrow without words*. Open point 9.

- [ ] The user shows the PM 20d (`V4-stepper.html`, the first row) cold,
  and asks what the arrow does. Five minutes; the answer written in the
  README's *Decisions*, dated.
- [ ] If misread: `web/src/components/StatusStepper.vue` gives the arrow
  its target's words at 64rem and wider (`gb-wide-only`, the 64rem step),
  the icon alone below; `head.css` for the wider arrow.

Done when the README's row says which, and H-11 may land.

## Wave B

### H-11 · The old head retired

Decision: *the old head goes*. The first plan's W3-1. Needs H-1, H-2, H-4,
H-5 and H-10.

Before the delete, the table of homes, checked against `IssueSide.vue` as
it is that day:

| In v1 | In v2 |
| --- | --- |
| Fields, with Saved | the fields line; each value's editor; `+N more` |
| Labels | the label line and picker |
| Development (`IssueBranches.vue`) | the branch line, a chip's popover, the create popover |
| On the forges (`IssueSync.vue`, `WaitingChanges.vue`) | the forge icons' popover, hosting both |
| Participants | the tab row's faces |
| Inbox | the bell |
| Spec, Linked issues | `IssueSide.vue`, `only`, under the branch line |
| the title row's Edit, Close issue; the meta line's author, count, forge links, id, page switch | More; the first post's header; the tab's count; the bar's icons; More |

- [ ] `web/src/headswitch.ts`: `head` and its `localStorage` key go;
  `headWidth` and `headDensity` stay.
- [ ] `web/src/components/IssuePanel.vue`: `v2` is gone as a condition
  (the class `is-head-v2` set always), the v1 template branches go (the
  title row, the meta line, `.gb-keys`, the `#side` slot), More loses *The
  previous head*.
- [ ] `web/src/components/IssueSide.vue`: keeps Spec and Linked issues
  only; `only` goes; its scoped styles with it.
- [ ] `web/src/components/IssueBranches.vue` deleted; `IssueStep.vue`
  loses the `side` slot and `.gb-sidebar` if nothing else uses them
  (grep first).
- [ ] `web/src/css/gitboard.css`: `.gb-ititle`, `.gb-imeta`,
  `.gb-forgelinks`, `.gb-pageswitch`, `.gb-dev*`, `.gb-keys`, `.gb-saved`
  removed when the grep finds no producer.
- [ ] `web/src/css/head.css`: the comment at its top says the head, not v2;
  the names stay.

Tests: `npm run typecheck`, `npm test`; `web/src/headswitch.test.ts` if one
exists loses the `head` cases. Done when every acceptance criterion of the
README's *The screens* is ticked or struck by a decision on a scratch clone
at 1024, 704 and 416px, `make ci` is green, and the drawer has one head.

## Wave C

### D-1 · The docs: README, screens, status update

Decisions: *the faces on a phone*, *the crumb's three parts*, and the
closing of the open points. Needs H-11.

- [ ] `docs/design/issue-head/README.md`: *Decisions* gains dated rows for
  the bar's 56rem step, the red dot built, four faces at every width, the
  crumb's three parts, new on the forge, the arrow's check and its answer,
  dark mode deferred; *Participants* says four; *Layout*'s column row says
  Fit 52rem and Fill; *Open points* 2, 8, 9, 10, 11 marked decided; *The
  screens*' open boxes ticked, or struck with their decision.
- [ ] `docs/design/issue-head/screens/`: `V1` (the dot), `V3` and `V5`
  (the bar at 704, the 56rem step), `W5` (the badges wired, "↓N")
  redrawn to match; `gitboard.css` and `head.css` copied again, their
  date in the README; the superseded `P1` to `P7` and `U1` to `U6`
  deleted, `drawer.css` kept for `O2` and `R1`.
- [ ] `docs/prd/issue-head-plan.md`: *Progress* gains a line pointing
  here.
- [ ] `docs/implementation-plan.md`: a dated *Status update* for the first
  pass and this one, as `CONTRIBUTING.md` asks (the first plan's D-1,
  second box).
- [ ] `docs/guide/configuration.md`: unchanged unless H-5 or H-6 added a
  word a user meets.

Done when `make docs` is green and the README has no open box without a
decision beside it.
