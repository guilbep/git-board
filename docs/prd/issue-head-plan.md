# The issue head: plan

**Spec:** `docs/design/issue-head/README.md` (decisions, screens,
acceptance criteria), with its screens in `docs/design/issue-head/screens/`.
Not one branch: each milestone below is small (a day or less), ships on its
own branch and worktree (`wtx go`), and leaves the app working. Most can run
at the same time: the table says what each needs first. A milestone whose
*Decision* column names an open point waits until guilbep has settled it
and the README's *Decisions* table has the row.

## Global constraints

- The EPFL design system as `web/README.md` says it: tokens only (no hex, no
  px; rem or `var(--…)`), Suisse Int'l through `--font-sans`, Feather icons,
  borders not shadows (a shadow on popovers only), 2px radii, pills on counts
  only, hover changes color or border, a 2px focus outline 2px off, disabled
  at opacity 1, sentence case, no emoji. Run the adherence linter from
  `web/README.md` on every file a milestone touches: no new problem, and a
  file a milestone rewrites ends with none.
- A pure function goes in `web/src/<name>.ts` with `web/src/<name>.test.ts`,
  run by `node --test`, and joins the `test` script of `web/package.json`,
  as `inboxcheck.ts` does.
- A new Go test calls `t.Parallel()` first, unless it chdirs, sets an
  environment variable or swaps a package variable. Real fixtures over mocks
  (`CONTRIBUTING.md`).
- No real person's name in mocks, fixtures or tests.
- git-board runs only commands written in its code or the toml, as an argv;
  nothing from the page reaches a command line.
- Until W3-1, the new head shows behind a reading switch, `head: 'v2'`, kept
  per browser in `localStorage`, as the panel's width and mode are
  (`panelsize.ts`). The side
  column stays as it is until then, so the team's daily board never loses a
  control halfway.
- The package's own tests while working; `make ci` once, at the end of each
  milestone.

## Review focus

1. The arrow moving the status twice on a double click, or moving it while
   a sync holds the issue (`untilSyncEnds`, `forges.ts`).
2. A forge's count disagreeing with the waiting changes listed for it: both
   must come from the events' `pending` and `edit_pending`, as
   `IssueSide.vue`'s `waitingFor` does.
3. A branch chip's word not matching the first of its gate's waits
   (`internal/ghpr/ghpr.go`, the waits in `Gate`).
4. The inbox popover's words against `internal/inbox/rows.go`: Not following
   still lets a mention, an add and a remove through.
5. Popovers: Esc closes and gives the focus back to their control; a click
   outside closes; nothing traps the focus.

## Calls taken

Mine, of 2026-10-30, for guilbep's eye.

1. **The top fields.** Until a key says which fields the line shows first,
   it shows the status, then `assignee`, `prio`, `sprint` and `kind` when the
   project has them (the default file's), else the first four configured
   fields; the others set are "+N more", the unset top ones "Set …". Row 8a's
   `[issue] top` key is not built here.
2. **The switch.** `head: 'v2'` lets the new head land milestone by
   milestone on the real board without a half-built drawer. Since decision
   1 (2026-10-30) v2 is the default: `localStorage` `gb-head = v1` keeps the
   head before (`web/src/headswitch.ts`); More › *The previous head* sets
   it, and the old head's meta line offers *The new head* back.
3. **No new icon set.** Feather's shapes go into `GbIcon.vue` as path
   strings, beside today's 30; no package.

## Progress

**2026-10-30, the first pass, web only.** In the working tree, not committed
(on `main`: branch first). `npm run typecheck` clean; `npm test` 298 of 298
(26 new); the adherence linter clean on every file touched.

- Done: W0-1 (the Feather shapes the head uses, upload-cloud as upload;
  download-cloud waits for W3-6; arrow-right, rotate-ccw, alert-triangle,
  alert-circle, refresh-cw and edit-3 were already there as forward, undo,
  warn, error, sync and edit), W0-3, W0-4 (with
  `folderName`, the create popover's interim name), W0-5 (pure; out of
  reach and both sides wired in W2-2), W0-6, W0-7, W1-1 to W1-9 (the fields
  line lives in `IssueHead.vue`; `headfields.ts` picks the top fields),
  W2-3, W2-4 (`BranchDetail.vue`), W2-5 (Copy only, see W3-3), W2-6.
- In part: W1-2 (Copy link and Open as a page stay in the bar under
  30rem), W2-1 (the popover hosts today's `IssueSync`; not extracted), W2-7
  (the held flag; no Resolve in its popover), W3-1 (the side column is gone
  behind the switch; Spec and Linked under the branch line), W3-2 (the
  create popover proposes `{kind}/{number}/{n}-{slug}` from the server's
  suggestion and the branches it lists; `Suggest` itself unchanged).
- Not started: W0-2, W0-8 (the side column's ✓ ✗ ●; the new popovers use
  icons), W0-9, W0-10, E-*, W2-2, L-1 to L-3, W3-3 (*Create* needs the
  endpoint: no Go toolchain in this session to build and test it), W3-4
  (the words are in the new head; the cycle's own texts unchanged), W3-5
  (decision 6 to confirm), W3-6, D-1.

**2026-10-30, the second pass, after a first look.** Built and tested
(Go: `go test ./...` passes with git 2.43, but for `TestInferGitBoard`,
which needs the real clone's remote; web: typecheck, 298 tests, the linter
clean on every file touched):

- W3-2: `{n}` in `[branches] name`, counted by `branches.Next` from the
  branches and the base's last 500 merge commits; the default name and
  `default.git-board.toml` put each branch in the issue's folder;
  `branches.Names` reads no number in an id's folder.
- W3-3: `POST /api/issues/{id}/branches` (`handleCreateBranch`): `git
  branch --no-track`, then `git push -u origin`, the checkout untouched;
  loopback and same-origin only; the name must be valid, name the issue,
  be new. The create popover offers *Create* and *Copy the commands*.
- W3-5: `cycle.Lane` and `PickLane`: each branch a lane; the evidence is
  the least advanced open one; a note says which branch holds the issue
  back; the step bar shows the lanes (2 or more).
- After the first look: the bar's *Fit* and *Fill*; task lists in
  descriptions flow as text again (`gitboard.css`: a hanging indent, no
  flex row); a dense editor per field (`FieldEditor.vue`, `FieldPanel.vue`)
  instead of the side column's form in a popover.
- L-1 to L-3, the labels: `[labels]` (`list`, `colors`, `descriptions`;
  a colon or a kind refused), in `ghlabels.Desired` for Settings › Labels
  and its Send to GitHub (Adopt writes `[labels.colors]`); `GET
  /api/config/labels`; `LabelPicker.vue`, checkboxes over the list, the
  others warned, no free text; lint's `label-outside`; the guide's
  *Labels*.
- After the second look: a person field as faces only, the name on hover
  and in its editor, an unset one as an empty face in its place; *Set…*
  gone, *+N more* or *More fields* the one way to the other fields
  (`IssueHead.vue`, `headfields.ts`'s `order`).
- After a critique of the density: the bar's *Comfortable* and *Compact*
  (`headswitch.ts`, `gb-head-density`; More's *Compact spacing* in a
  narrow drawer), Compact's rules in `head.css`; in Fill the prose keeps
  Fit's width (`--gb-prose`); the bar's crumb and a paragraph lose the
  browser's margins; a loose checklist's boxes hang.

**2026-10-31, the second pass.** Finished in `docs/prd/issue-head.md` and
its plan, `docs/prd/issue-head-m2-plan.md`: W0-2, W0-9, W0-10, E-* for the
head's files, W2-2, W3-1, W3-4 and W3-6 landed there as H-7, H-3, H-9, H-8,
H-4, H-11, H-6 and H-5; D-1 there too. Left from this plan: W0-8 (✓ ✗ ● in
`WorktreesSettings.vue`, the side column's are gone with it), W2-7's
*Resolve*, and E-* for the other files `make web-lint` names.

## The milestones

| ID | Milestone | Wave | Needs | Decision | Size |
| --- | --- | --- | --- | --- | --- |
| W0-1 | Feather icons in `GbIcon` | 0 | — | — | S |
| W0-2 | Value colors into a token file | 0 | — | — | S |
| W0-3 | `nextstep.ts`: the arrow's target | 0 | — | — | S |
| W0-4 | `branchstate.ts`: a chip's tone, icon, word, face | 0 | — | — | M |
| W0-5 | `forgestate.ts`: a forge's badge and words | 0 | — | — | S |
| W0-6 | `inboxcheck.ts`: the bell's state | 0 | — | — | S |
| W0-7 | `people.ts`: the conversation's people, in order | 0 | — | — | S |
| W0-8 | ✓ ✗ ● × become icons | 0 | — | — | S |
| W0-9 | `make web-lint` | 0 | — | — | S |
| W0-10 | Branch names: tests of the folder form | 0 | — | — | S |
| E-* | One file's EPFL fixes (one milestone per file) | 0 | — | — | S |
| W1-1 | The bar: the number, the Closed badge | 1 | — | — | S |
| W1-2 | More in the bar | 1 | W0-1 | — | S |
| W1-3 | The forge icons in the bar | 1 | W0-1, W0-5 | — | S |
| W1-4 | The bell in the bar | 1 | W0-1, W0-6 | — | S |
| W1-5 | The title alone | 1 | W1-1, W1-2 | — | S |
| W1-6 | The status stepper | 1 | W0-1, W0-3 | — | M |
| W1-7 | The fields line | 1 | W0-1 | — | M |
| W1-8 | The branch line | 1 | W0-1, W0-4 | — | M |
| W1-9 | The people in the tab row | 1 | W0-7 | — | S |
| W2-1 | The forge popover | 2 | W1-3 | — | M |
| W2-2 | The forge states | 2 | W1-3 | — | S |
| W2-3 | The inbox popover | 2 | W1-4 | — | S |
| W2-4 | A branch's popover | 2 | W1-8 | — | M |
| W2-5 | The create-a-branch popover | 2 | W1-8 | — | S |
| W2-6 | The people popover | 2 | W1-9 | — | S |
| W2-7 | Held: the flag's popover | 2 | W1-7 | — | S |
| L-1 | `[labels] list` in the project file | 2 | — | 7 | M |
| L-2 | The label picker | 2 | L-1, W1-7 | 7 | S |
| L-3 | Lint: labels outside the list | 2 | L-1 | 7 | S |
| W3-1 | The side column retired; the switch removed | 3 | W1-*, W2-1 to W2-5, L-2 | 1, 2 | M |
| W3-2 | Branch names in the issue's folder | 3 | W0-10 | 3 | M |
| W3-3 | git-board makes the branch | 3 | W2-5 | 4 | M |
| W3-4 | The words: branch, review, merge; the gate | 3 | W2-4, W3-2 | 5 | S |
| W3-5 | The cycle with several branches | 3 | W3-2 | 6 | M |
| W3-6 | "New on the forge" after a check | 3 | W2-1 | 8 | S |
| D-1 | The guide: `[branches]` and `[labels]` | 3 | W3-2, L-1 | 3, 7 | S |

Wave 0 is eleven milestones and the E-* files, all at once. Wave 1 needs
only wave 0; wave 2 only its own control. The decisions are the
README's *Open points*, by number.

## Wave 0

### W0-1 · Feather icons in `GbIcon`

- [ ] `web/src/components/GbIcon.vue`: add github, gitlab, bell, bell-off,
  git-branch, git-pull-request, git-merge, upload-cloud, download-cloud,
  refresh-cw, cloud-off, alert-triangle, alert-circle, eye, edit-3, slash,
  arrow-up, arrow-down, arrow-right, rotate-ccw, more-horizontal, copy,
  from feather-icons 4.29.0 (the EPFL skill's version), each `<circle>`,
  `<line>`, `<polyline>` and `<rect>` written as path data.
- [ ] Extend `IconName`; keep the existing 30 names and their paths.

Test: `npm run typecheck`. Done when each new icon matches Feather's at 24
and at the small size, side by side in a scratch page (not committed).

### W0-2 · Value colors into a token file

- [ ] Move the `:root` block of `--gb-<color>-bg/-fg/-dot` (and `--gb-done`)
  from `web/src/css/gitboard.css` to `web/src/css/tokens/values.css`.
- [ ] `web/src/css/app.css` imports it after `epfl.css`, before
  `gitboard.css`.

Test: the adherence linter's count for `gitboard.css` drops by those lines;
the board and a value chip look the same before and after.

### W0-3 · `nextstep.ts`: the arrow's target

- [ ] `nextStep(statuses: string[], current: string | null, closed: boolean)`
  returns `{ icon: 'arrow-right' | 'check' | 'rotate-ccw', label, target }`
  or `null`: no status gives the first, "Move to <first>"; the one before
  the last gives "Close as <last>" (the last closes the issue,
  `default.git-board.toml`); a closed issue gives "Reopen"; no workflow
  gives `null`.
- [ ] `web/src/nextstep.test.ts`, joined to the `test` script.

Test: a table of the five cases with co2-calculator's nine statuses and the
default six.

### W0-4 · `branchstate.ts`: a chip's tone, icon, word, face

- [ ] `branchState(branch, gate, base)` returns `{ tone, icon, word, who,
  sentence }`, tone one of `author`, `review`, `checks`, `ready`, `draft`,
  `merged`, `closed`, `branch`, by the README's table: a failing required
  check, changes requested, a conflict or behind the base give `author`;
  checks running or GitHub checking give `checks`; review required gives
  `review` with the requested reviewers as `who`; `canMerge` gives `ready`.
  `sentence` lists every wait in `internal/ghpr`'s words.
- [ ] `branchLine(items)`: red, yellow, blue, green, drafts, branches; four
  chips, the rest as `more`; merged and closed counted apart.
- [ ] `web/src/branchstate.test.ts`.

Test: one case per tone; #3201's three waits in order; ten branches give
four chips, two more, three merged, one closed.

### W0-5 · `forgestate.ts`: a forge's badge and words

- [ ] `forgeState(forge, waiting, job, last)` returns `{ badge: 'count' |
  'syncing' | 'unreachable' | 'conflict' | 'new' | null, n, label }`:
  `unreachable` when the last job left the forge out (`BridgeTally.left`),
  `conflict` when the last dry run found it changed on both sides, `syncing`
  while a job runs, `count` with the waiting changes, `new` with a forge not
  on the issue yet; the label in words ("GitLab #1: 6 changes wait to be
  sent").
- [ ] `web/src/forgestate.test.ts`.

Test: one case per badge, and the order when two apply (out of reach wins
over a count).

### W0-6 · `inboxcheck.ts`: the bell's state

- [ ] `bellState(f: FollowState)` returns `{ shape: 'bell' | 'bell-off',
  tone: 'on' | 'off' | 'part', label }` from `followLine`: following gives
  bell, on; a project role gives bell, part; not following gives bell-off,
  off. `label` is `followLine`'s state and reason.
- [ ] Cases in `web/src/inboxcheck.test.ts`.

Test: the four `followLine` cases already tested, plus their bells.

### W0-7 · `people.ts`: the conversation's people, in order

- [ ] `peopleOf(thread)`: the author, then each commenter by their latest
  comment, newest first; the viewer marked.
- [ ] `peopleLabel(n)`: "1 in this conversation", "7 in this conversation".
- [ ] `web/src/people.test.ts`.

Test: a thread where the latest commenter commented first too comes second.

### W0-8 · ✓ ✗ ● × become icons

- [ ] `IssueBranches.vue`'s `marks` (✓ ✗ ●), `IssueSide.vue`'s "✓ Saved" and
  the chip's ×, and any other found by `grep -rn "[✓✗●×]" web/src`: a
  `GbIcon` (check, close, clock) with the word kept for screen readers.

Test: typecheck; the Development section and a saved field read the same.

### W0-9 · `make web-lint`

- [ ] A Makefile target running `web/README.md`'s `npx stylelint` command on
  `web/src/**/*.{css,vue}` against the skill's
  `adherence/.stylelintrc.json`, reporting only; not part of `make ci` until
  the count is 0.
- [ ] `CONTRIBUTING.md`'s *Workflow* lists it.

Done when `make web-lint` prints today's count (517 problems on
2026-10-30, over `css/`, `components/`, `pages/` and `layouts/`).

### W0-10 · Branch names: tests of the folder form

- [ ] `internal/branches/branches_test.go`: `Names` links
  `test/2771/1-implement-tests` and `2811/1-pdf-footer` to their issues; it
  doesn't link `test/2771b-x` (a letter glued to the number: not a number
  word); a project number links `feat/co2-421/1-x`.

No code change: the tests write down what the folder form relies on.

### E-* · One file's EPFL fixes

One milestone per file, any order, all at once: px to rem or a token, hex
to a token, a font to `var(--font-sans)`. By count on 2026-10-30:
`gitboard.css` 145 (after W0-2, less), `dark.css` 61 (its values to a
`tokens/dark.css`, as W0-2 does for light), `IssueFlow.vue` 37,
`IssueChanges.vue` 34, `ConfigEditor.vue` 27, `HistoryView.vue` 20,
`IssueTimeline.vue` 20, `IssueSections.vue` 17, `ThreadCard.vue` 14,
`InsightsPage.vue` 11, then the 46 other files `make web-lint` names.

Done when the file has no problem and looks the same at 1440px and 416px.

## Wave 1

### W1-1 · The bar: the number, the Closed badge

- [ ] `IssuePanel.vue` `.gb-ptop-crumb`: after the view and its position,
  "›" and the number (`thread.number`, mono), then the Closed badge
  (`epfl-badge epfl-badge-tertiary`) when closed. The view's name shortens
  first.
- [ ] The visible kbd hints (k, j, Esc) go; the buttons' names already carry
  them ("Previous issue (k)"); add them to the tooltips.

Done when 19a's and 19c's bars match on the left half.

### W1-2 · More in the bar

- [ ] A More button (more-horizontal) after the issue's icons: Edit the
  title, Copy the id (today's `id 4347840` line), Close or Reopen the issue.
- [ ] Under 30rem, Copy link and Open as a page join it.

Done when every action of today's title row and meta line is reachable.

### W1-3 · The forge icons in the bar

- [ ] `ForgeIcons.vue`: one `gb-iconbtn` per forge of the issue, github or
  gitlab, with W0-5's badge and label; red on hover.
- [ ] Its waiting counts come from the events as `waitingFor` does.

Done when 16e's states render from fixtures and #2771's bar shows "↑6" on
GitLab.

### W1-4 · The bell in the bar

- [ ] `InboxBell.vue`: W0-6's shape and tone, its label as name and tooltip,
  from today's `getFollow`.

Done when 16g's three states render.

### W1-5 · The title alone

- [ ] `.gb-ihead`: the number leaves the `h2`; the edit button and *Close
  issue* leave the row (they are in More); the meta line goes behind the
  switch.

Done when 19a's title row matches: #2771 in two lines at 1024.

### W1-6 · The status stepper

- [ ] `StatusStepper.vue`: the status box (value mark, words, hold flag,
  chevron) as a listbox button, joined to W0-3's arrow button.
- [ ] The arrow calls today's status move; disabled while saving; the words
  fade (`--t-fast`) out before and in after; an error restores them with
  today's "Not saved" line.

Test: typecheck; 19d's eight specimens by hand.

### W1-7 · The fields line

- [ ] `FieldLine.vue`: the stepper, "by #N" for a blocked-by value, the
  assignees (four faces, two names, +N), the values with their marks, the
  held flag, "+N more", "Set …" (Calls taken, 1).
- [ ] Each value opens today's field editor for it.

Done when 16a's lines match for #2810 and #2771.

### W1-8 · The branch line

- [ ] `BranchLine.vue` and `BranchChip.vue` from W0-4, over today's
  `getBranches` and `getPulls`; the plus at the end (W2-5 opens it).
- [ ] The chip's sentence as its name and tooltip.

Done when 16c's lines render: #2771, #2810, ten branches, none.

### W1-9 · The people in the tab row

- [ ] `PeopleButton.vue` at the end of the tabs: W0-7's order, four faces
  and the label; three faces and +N under 30rem.

Done when 16f's tab row matches.

## Wave 2

### W2-1 · The forge popover

- [ ] Extract `IssueSync.vue`'s run into `web/src/useissuesync.ts`.
- [ ] The popover: each forge with its link and its state in words; *Send
  now* (or *Put it on the forges*) and *Refresh*, both the run; "See the N
  changes" opens `WaitingChanges`.

Done when the popover runs a dry run and asks before writing, as today.

### W2-2 · The forge states

- [ ] Syncing from `forgeStatus.job`; out of reach from the last job's
  `bridges[name].left`; changed on both sides from the last dry run's
  `conflicts`; not on it yet from a configured forge the issue lacks.

Done when each badge shows on a scratch clone in its situation.

### W2-3 · The inbox popover

- [ ] Three radios: Automatic ("Now following: …" or "Now not following:
  …"), Following, Not following; a change calls today's `setFollow` with
  `null`, `true`, `false`.

Done when each choice round-trips and the bell follows.

### W2-4 · A branch's popover

- [ ] The gate's checks (required flagged), approvals, decision and waits,
  *Merge into dev* (disabled until `can_merge`), *Open PR into dev* for a
  branch with no gate, the push command for a local one: today's
  `IssueBranches.vue` gate, moved.

Done when every action of today's Development section is reachable from a
chip.

### W2-5 · The create-a-branch popover

- [ ] The plus: `Suggest`'s name in an input, "From <start>", the two
  commands, Copy: today's *Create branch* details, moved.

Done when 16d's popover matches.

### W2-6 · The people popover

- [ ] The list: face, name, "opened it" or "commented", "you".

### W2-7 · Held: the flag's popover

- [ ] The held flag's tooltip names the Project's value; the prio's popover
  offers Resolve, as the conflicts page does today.

### L-1 · `[labels] list` in the project file

Decision 7.

- [ ] `internal/config/config.go`: a `labels` table with `list []string`;
  an unknown key under it warns, as `check.go` does for any; no table, no
  labels.
- [ ] Settings › Labels lists them with the fields' options and the kinds.
- [ ] `config_test.go`: parsed, empty, absent.

### L-2 · The label picker

Decision 7.

- [ ] The fields line shows one tag and "+N" when the list exists;
  `LabelPicker.vue`: checkboxes over the list, no text box; a label outside
  the list with a warning.
- [ ] Today's *Add label* goes.

### L-3 · Lint: labels outside the list

Decision 7.

- [ ] `git board lint` and Settings › Labels › Lint report the issues'
  plain labels outside `[labels] list`.

## Wave 3

### W3-1 · The side column retired; the switch removed

Decisions 1 and 2.

- [ ] `IssueSide.vue` loses Fields, Labels, Development, On the forges,
  Participants and Inbox; Spec and Linked move under the branch line while
  they have content.
- [ ] `.gb-cols` gives the column the panel's width; the switch goes and the
  new head is the head.

Done when 19a to 19c match at 1024, 704 and 416, and every acceptance
criterion of the README is ticked.

### W3-2 · Branch names in the issue's folder

Decision 3.

- [ ] `internal/branches/branches.go`: `{n}` in the name template; `Suggest`
  counts it: one more than the highest found among the issue's branches
  (`Linked`) and the first-parent merge commits on the base that name one.
- [ ] `default.git-board.toml`: `name = "{kind}/{number}/{n}-{slug}"`.
- [ ] Tests: a first branch is `/1-`, a second `/2-`, a merged and deleted
  first still makes the next `/2-`; with no kind, `2811/1-…`.

### W3-3 · git-board makes the branch

Decision 4.

- [ ] `POST /api/issues/{id}/branches`: `git branch --no-track <name>
  <start>`, then `git push -u origin <name>`; never `git switch`; refused,
  with a code, when the server listens beyond localhost or the request is
  not the board's own page (as `wtxAllowed` refuses).
- [ ] The popover's *Create*; on a refusal it keeps *Copy the commands*.
- [ ] Tests on a throwaway repo with a bare origin.

### W3-4 · The words: branch, review, merge; the gate

Decision 5.

- [ ] Chips name the branch (`2771/2`); the popover says "The gate: GitHub
  #3155"; *Open PR into dev* becomes *Ask for review*.

### W3-5 · The cycle with several branches

Decision 6.

- [ ] `internal/cycle/gather.go` follows the branch the decision names, and
  Verify opens as it says; tests with two open branches.

### W3-6 · "New on the forge" after a check

Decision 8.

- [ ] After *Refresh*'s dry run, each forge with something to bring in shows
  a blue "↓N" until the next sync.

### D-1 · The guide: `[branches]` and `[labels]`

- [ ] `docs/guide/configuration.md`: `{n}` and the folder form under
  Branches; `[labels]` with its default.
- [ ] `docs/implementation-plan.md`: a dated status update per landed
  milestone, as `CONTRIBUTING.md` asks.
