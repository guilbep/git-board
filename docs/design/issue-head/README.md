# The issue head: the bar, one block of fields, the conversation

Design handoff, 2026-10-30. From the design canvas "git-board · one issue,
one plane?", rows 8 to 21 (rows 20 and 21 are the newest, the head as
built; row 7 is `docs/design/project-cycle/`), decided with guilbep on
2026-10-29 and 2026-10-30. Builds on `docs/design/time-flow/README.md` (one
column, the flow) and `docs/prd/work-unit.md` (the issue is the unit of
work; a PR is only the gate into `dev`). The screens are in `screens/`, as
plain HTML that opens in a browser, redrawn on 2026-10-30 from the build
(see *The screens*); the rows that led here are summed up in
`exploration/index.md` (their boards stay on the canvas: they show real
people's names). `docs/prd/issue-head-plan.md` is the implementation plan, in
small milestones that can run in parallel; `brief.md` says how to start.

## The problem

The issue panel spreads one question over five places. The title row holds
the number, the title and *Close issue*; a meta line holds the state badge,
the author, the comment count, the forge links and the id; the side column
(`IssueSide.vue`, 17.5rem, under the conversation below 64rem) holds Fields,
Labels, Development, On the forges, Participants and Inbox, all at one
weight. What waits, and on whom, is spread over three of them: a held field,
a failing PR, six changes not yet on GitLab. The side column takes a third
of a 1024px drawer from the text, and in row 17's head, the last before the
bar, #2771's title still wrapped to three lines beside its icons.

**The drawer's bar says which issue this is and holds what applies to it as
a whole. Under the title, one block: the status and its next step, the
values, the branches. Each wait sits on its source. The people of the
conversation sit beside it. Everything on the EPFL design system's tokens
and classes.**

## Decisions

Taken with guilbep on 2026-10-29 and 2026-10-30. Not reopened here; what is
still open is under *Open points*.

| Topic | Decision |
| --- | --- |
| One block over the column | J-line 2 (rows 11 to 14): the fields line and the branch line under the title, over one centred 40rem column that the conversation shares. It won rounds 2 to 4 and the re-test (2 to 1). The side column's sections move into the block, the bar and the tab row. |
| The bar | The drawer's top line holds the issue's identity as a breadcrumb, *view › #2771* (and the Closed badge when closed), then the forges, the bell and More, then the drawer's own buttons as today (previous, next, copy link, expand, open as a page, close). The kbd hints (k, j, Esc) go into the buttons' names and tooltips (row 19). |
| The title | Alone on its row, the whole column wide: no number, no icons beside it. #2771's title takes two lines, not three; a title under about 50 characters takes one. |
| The next step | An arrow joined to the status box: *Move to backlog* when there is no status, the next status by name otherwise, in its tooltip and accessible name. A check when the next status is the last ("Close as validated", the last one closes the issue); a rotate icon to reopen a closed issue. A move fades the status's words out and in (`--t-fast`): nothing slides (row 19d). |
| Waits on their source | The block of what waits (rows 9 to 14) goes. A held prio carries a *held* flag; a hold carries its flag in the status box, and "by #2802" after it; the changes waiting for a forge are that forge's count; a PR's state is its branch's chip. |
| Inbox | A bell in the bar. Bell, in the text color: the issue reaches your inbox. Bell-off, grey: it doesn't. Grey bell: a project role brings new and closed only. Hover turns it red, as EPFL links and tags do. The popover offers git-board's three choices, applied on click: Automatic, Following, Not following (row 16g). |
| Forges | An icon each in the bar, GitHub's and GitLab's as Feather draws them. Nothing waiting: the bare icon. Changes to send: a yellow count with an up arrow. A sync running, a forge out of reach, an issue changed on both sides, an issue not on the forge yet: a small badge each. The popover lists each forge and offers Send now and Refresh (row 16e, row 18). |
| Participants | At the end of the tab row: the author, then the latest to comment, four faces then +N at every width (2026-10-31: they fit beside the tabs at 416, so no second rule for a phone); "7 in this conversation" is the button's name and tooltip. Assignees stay in the fields: who does it is not who talks (rows 16f and 18). |
| Branches | One chip per branch, its color for whose move: red the author's, yellow a reviewer's, blue the machines', green anyone may merge, grey a draft, canard merged. A chip shows its state icon, its number, a word, and an arrow to the face whose move it is. Red first, then yellow, blue, green, drafts, branches without a gate; four chips, then "+N more"; merged and closed fold at the end (rows 16c and 18). |
| Create a branch | A popover from the plus at the end of the branch line: the name `[branches]` proposes, editable, from origin's default branch, and the commands to copy (row 16d). |
| Labels | Off by default: no chip, no *Add label*. A manager who wants some lists them in `.git-board.toml`; the picker offers that list as checkboxes, never free text; a label from a forge outside the list shows with a warning (row 16b). |
| No PR in the words | A branch is reviewed, then merged, from the issue. GitHub's PR and GitLab's MR are the gate, as `work-unit.md` already says: the forge's place to run its checks and rules. |
| Branches live in the issue's folder | `test/2771/…`, the first branch too; never a branch named `test/2771` itself. |
| The side column goes | Settled 2026-10-30 (open point 1). Fields, Labels, Development, Forges, Participants and Inbox leave it; Spec and Linked issues sit under the branch line while they have something to say. Supersedes `time-flow`'s "Fields keep the sidebar". |
| The folder's name | Settled 2026-10-30 (open point 3): `{kind}/{number}/{n}-{slug}`, so `test/2771/1-implement-tests-to-validate-all-equations`; `{n}` counted, never stored. |
| Create and copy, both | Settled 2026-10-30 (open point 4), "both options, greater latitude": the create popover offers *Create*, git-board running `git branch --no-track` and `git push -u` (localhost only), and *Copy the commands*, for whoever prefers their own terminal. |
| Branch, review, merge | Settled 2026-10-30 (open point 5): the chips name the branch (2771/2), the gate's number goes into its popover ("The gate: GitHub #3155"), *Ask for review* opens the gate. |
| `[labels] list` | Settled 2026-10-30 (open point 7), built: `[labels] list`, `colors` and `descriptions`; the head's picker offers the list as checkboxes, a label outside it with a warning; Settings › Labels sends them to GitHub; `git board lint` reports `label-outside`. No table, nothing to pick. |
| Several branches: branched out | Settled 2026-10-30 (open point 6). Intent, Risk and Plan are the issue's, once; from Tests on each branch of the issue's folder runs its own Tests, Code and Review, shown as a row per branch under the step bar; the lanes join at Verify once each is merged or closed. The issue's step is its least advanced open lane (`cycle.PickLane`). A branch only in this clone with nothing in it is no lane yet. |
| Fit and Fill | 2026-10-30, after the first look: the bar offers *Fit*, the default, the head and the conversation at 52rem, and *Fill*, the panel's whole width; kept per browser. |
| One value, one editor | 2026-10-30, after the first look: a value opens its own dense editor (a menu of options, checkboxes for people, one small input for a date, a number or an issue); *+N more* (or *More fields*) lists every field, a row each, opening its editor in place. No input takes the focus by itself: a browser's autofill jumped in. |
| One way to the other fields; people as faces | 2026-10-30, after the second look: *Set prio, sprint…* and *More fields* opened the same list, so *Set…* went: *+N more* (or *More fields*) is the one way to every field, an unset one reading *Not set* there. A person field shows faces only, its initials; the full name shows on hover and in its editor. Nobody yet is an empty face in the field's place, one click to assign. |
| Comfortable and Compact | 2026-10-30, after an outside critique of the density: the bar offers *Comfortable*, the default, the spacing as designed, and *Compact*, opted into and kept per browser (in a narrow drawer, More's *Compact spacing*). Compact: the title in EPFL's h5 (20px bold), 8px between the head's lines, linked issues on one line, the tabs under the head, a comment's header a line rather than a band with its time and forges at 12px, the text at line height 1.5 with its headings under the title's weight, a loose list's items as close as a tight one's. The text keeps its 15px. Measured on a GB-36-like issue at 1440×900: the description's first line at 303px, not 464; a one-line comment 73px tall, not 126; the description and two comments on screen, not the description alone. |
| Fill keeps the prose at Fit's width | 2026-10-30, the same critique: in Fill, paragraphs, lists and headings stop at the width they have at Fit (115 characters a line, not 180); code, tables, images, the head and the branch rows take the panel's. Two browser margins went, in both spacings: the bar's crumb paragraph (a 62px bar, now 51) and a paragraph's 1em under it (15px between paragraphs, not the 12 asked, and 15 more under every comment). A loose checklist's boxes hang as a tight one's do, without bullets. |
| EPFL to the letter | Tokens only (no hex, no px), Feather icons, borders not shadows (a shadow on popovers only), 2px radii, pills only on counts, hover changes color or border, a 2px focus outline 2px off, disabled at opacity 1, sentence case, no emoji, the red dot under the active tab (row 15b). The value marks (status, prio, kind) stay git-board's own: Feather has none. |
| The bar under 56rem | 2026-10-31 (`docs/prd/issue-head.md`, H-1), after the redraw found the bar broken at 704: under 56rem of drawer, *Fit · Fill* and *Comfortable · Compact* leave the bar for More, as *Fill the panel* and *Compact spacing*, checkbox items that set the same state; the crumb never goes under 7rem, the view's name shortening first. Measured from 64 down to 24rem: the bar never overflows and the number always shows whole. |
| The red dot, built | 2026-10-31 (H-2): the issue's tabs carry EPFL's dot, 0.4375 by 0.375rem, `--primary`; the board's other tabs keep their underline until a pass of their own. |
| New on the forge | 2026-10-31 (open point 8, H-5): after this issue's *Refresh*, a forge with something to bring in shows a blue "↓N", gone once its own sync, or a whole sync from Settings, is done after it (a whole sync that fails keeps it), by the server's clock; never polled. With changes to send as well, the blue count sits in the icon's bottom corner, so the forge's shape still shows. *Changed on both sides* reads the same check; *out of reach* the newer of this issue's own last run and the last run the server keeps (H-4). A forge link is its bridge by its host and project, as the server lists each bridge's. |
| The crumb's three parts | 2026-10-31 (open point 11): the view, the position and the number, as built: *current-sprint / 3 of 12 › #2771*. |
| The waits block stays gone | 2026-10-31 (open point 2): each wait on its source, guilbep's "things live where they matter, always" (2026-10-29). |
| Dark mode, later | 2026-10-31 (open point 10): the dark values moved to `web/src/css/tokens/dark.css` (H-7); the tints' contrast in dark is a pass of its own. |
| The cycle's words | 2026-10-31 (H-6): the cycle's pages and the server's notes say "GitHub #N", "GitHub #N, a draft" and "the plan's gate" where they said PR; a branch waits to be reviewed. *Mark ready for review* and *Start verification* keep their names. |
| The arrow, checked | 2026-10-31 (open point 9, H-10): shown 20d cold, the PM read the arrow as moving the status on; it stays an icon, its target in its tooltip and name. |
| The head before, retired | 2026-10-31 (H-11): the switch, the old title row and meta line, and the side column are gone; Spec and Linked issues stay under the branch line. An issue field's values open their issue from its editor, and a date's editor says how near or late it is, as the side column did. |

## The screens

Redrawn on 2026-10-30 from the build, rows 20 and 21 of the canvas: each
file in `screens/` is one board at 1440px, drawn with the app's own
stylesheets (`gitboard.css` and `head.css`, copied unchanged from
`web/src/css` that day, plus `parts.css` for the components' scoped styles)
and the components' markup, class for class, so a screen is what the app
renders with fixtures. The people are made up (Person A to F; Person B is
the viewer). Each board says what differs from the morning's drawing (rows
16 to 19) and what is not built. The acceptance criteria are what a reviewer
ticks; a box left open is not built, or found wrong. `O2-epfl.html` and
`R1-blind.html` are records of rows 15b and 18, kept as drawn on
`drawer.css`; the other screens of rows 16 and 19 were deleted on
2026-10-31, the canvas keeps them. Redrawn on 2026-10-31 after the second
pass (`docs/prd/issue-head.md`): `V1`, `V3`, `V5`, `V6`, `W1` and `W5`, with
`gitboard.css` and `head.css` copied again that day.

### 20a `V1-bar-2771.html` · #2771: the bar, the head, the conversation

- [x] The crumb reads *view / position › #number*; a closed issue adds its Closed badge.
- [x] The forges, the bell and More, then *Fit · Fill* and *Comfortable · Compact*, then previous, next, copy link, expand, open as a page, close; the kbd hints in the names.
- [x] The title alone, 24px normal weight, over a 52rem column at Fit; two lines for #2771.
- [x] The status box and its arrow are one control; the arrow's tooltip and name say "Move to backlog".
- [x] Assignees as faces only, the viewer in black; *More fields* and *Label…* muted and dotted; no *Set prio · sprint*.
- [x] The branch line shows *2771/2 review → PD* in yellow and "1 merged" as a plain chip.
- [x] The tab row ends with four faces then +N; "N in this conversation" is the button's name.
- [x] The active tab carries EPFL's red dot (H-2).

### 20b `V2-bar-2810.html` · #2810, the crowded one

- [x] The title takes one line; the fields take two.
- [x] The status box shows *in-review* and a red *blocked* flag; "by #2802" opens the blocking issue; the arrow moves to ready-for-validation.
- [x] Prio *p1* carries a yellow *held* flag; its editor names the Project's value and links to Conflicts.
- [ ] The held flag's tooltip names the Project's value, and its popover offers Resolve (W2-7, in part; not in the second pass).
- [x] GitLab's icon carries "↑2"; GitHub's is bare.
- [x] *2810/1 checks fail → PA* red, *2810/2 draft → PB* grey, *2810/3 checks run* blue; the hover sentence in `internal/ghpr`'s words.
- [x] Linked issues under the branch line while there are any.

### 20c `V3-widths.html` · At 704 and on a phone

- [x] At 704 the bar keeps the crumb whole: under 56rem the two switches move into More (H-1).
- [x] At 416 the two switches, copy link and open as a page move into More, with *Compact spacing*; the view's name shortens before the number; the title is 20px; buttons are 2rem.
- [x] Four faces then +N at every width (decision *Participants*, 2026-10-31).

### 20d `V4-stepper.html` · The next step: the arrow on the status box

- [x] No status: an arrow, "Move to backlog". In the middle: an arrow, the next status by name. Before the last: a check, "Close as validated". On the last, open: a check, "Close the issue". Closed: a rotate icon, "Reopen".
- [x] Hover: `--bg-subtle` fill, `--fg-muted` border, the arrow's icon red. Focus: a 2px `--fg-muted` outline, 2px off.
- [x] Pressed: the words fade out (`--t-fast`) and the new status fades in with its mark; nothing moves. A move during a sync says "Waits for the sync…"; a refusal brings the old words back with "Not saved".
- [x] The box opens a menu of the workflow's values with their marks, the current one checked; the focus lands on it.

### 20e `V5-bar-moves.html` · The bar at 1024, 704 and 416

- [x] The bar at 1024 and 416 drops elements in the order the board's table says.
- [x] The bar at 880 and 704: the switches in More (H-1).

### 20f `V6-lanes.html` · Branches in the issue's folder; the cycle, branched out

- [x] The names follow `branches.go`: `{kind}/{number}/{n}-{slug}`, `{n}` counted from the branches and the base's last 500 merge subjects; no kind, no folder prefix.
- [x] With two branches or more, a row per branch under the step bar: Tests, Code, Review; past cells ticked, the issue's lane in red, a merged lane in canard, a closed one struck; the row's end names the gate, "merged", "closed" or "no test yet".
- [x] The issue's step is its least advanced open lane.
- [x] The cycle's own texts say "GitHub #N" and "the plan's gate" (H-6).

### 20g `V7-fit-fill.html` · Fit and Fill

- [x] Fit: the head, the tab row and the conversation at 52rem, centred. Fill: the panel's width; paragraphs, lists and headings stop at Fit's prose width; code, tables, images and the head take the panel's.
- [x] Kept per browser (`gb-head-width`); the bar's buttons say which is on.

### 20h `V8-density.html` · Comfortable and Compact

- [x] Compact: the title 20px bold, 8px between the head's lines, linked issues on one line, the tabs under the head, a comment's header a line with its time and forges at 12px, the text at 1.5 with headings at the body size, a loose list's items as close as a tight one's; the text keeps its 15px.
- [x] Both: no browser margin under the bar's crumb (a 51px bar) nor under a paragraph; a loose checklist's boxes hang.
- [ ] Which one is the default, once both have been used (guilbep's call).

### 20i `W1-fields.html` · The fields line, and one editor per value

- [x] The status first, "by #N" for a blocked-by, then assignee, prio, sprint and kind when the project has them, else the first four; the other fields set as *+N more*, else *More fields*.
- [x] A person field: up to four faces then +N; nobody yet is a dashed empty face in its place. The viewer's face in black. The name on hover and in the editor.
- [x] A value opens its editor: a menu of options (one click sets, *Not set* clears), checkboxes for several values or people (a login for someone else), one small input for a date, a number or a text (Save, Clear), the issues it names and one to add for an issue field. No input takes the focus by itself.
- [x] A held field's editor says what the Project says and links to Conflicts.
- [x] *+N more* lists every field but the status, a row each with its mark and value, *Not set* muted; a row opens its editor in place, *All fields* goes back.

### 20j `W2-labels.html` · Labels: the project's list, picked

- [x] Without a list in the project file: no chip and nothing to add.
- [x] With a list: *Label…* when none is set; else a tag, the first label, then +N; a warning triangle when one is outside the list.
- [x] The picker lists the project's labels as checkboxes with their color dot and description, no text box; a label outside the list shows with a warning and a remove button; without a list, the note points to the project file.

### 20k `W3-branches.html` · Branches: one chip each, the color says whose move

- [x] Each tone renders with its tint, its icon and the build's word: red (checks fail, changes asked, conflict, behind dev), yellow (review, waiting), blue (checks run), green (ready), grey (draft), canard (merged, folded), struck grey (closed, folded), outline (not in review, local).
- [x] The face after the arrow is the author's on red and grey, the asked reviewer's on yellow, none on blue and green.
- [x] Ten branches show four chips and one plain chip, "+2 more · 3 merged · 1 closed", which opens them as folds.
- [x] Hover or focus gives the sentence: the branch, its word, whose move, the gate, every wait.
- [x] Clicking a chip opens where it stands, the gate, its checks (required ones flagged), approvals, the decision, the waits, *Merge into dev* disabled until GitHub says it can merge; *Ask for review* without a gate; the push command when local.

### 20l `W4-branch.html` · Create a branch: Create, or copy the commands

- [x] The plus opens a popover: the name `[branches] name` proposes, editable, the start, the two commands, *Create* and *Copy the commands*.
- [x] Created and pushed: the branch on origin and what to run in the clone; the push refused: the branch local to the server's clone and the push command; refused: nothing created, the reason.

### 20m `W5-forges.html` · The forges in the bar

- [x] Bare (in sync), a yellow count with an up arrow (to send), a blue sync badge (syncing), an upload icon (not on the forges yet).
- [x] A red cloud-off (out of reach, from the newer of this issue's last run and the board's) and a triangle (changed on both sides, from this issue's Refresh) (H-4).
- [x] The blue "↓3" after this issue's Refresh; beside a count to send, in the bottom corner (H-5).
- [x] Each icon's name and tooltip say the forge, the issue's number there and the state in words.
- [x] The popover lists each forge with its link and state, *See the N waiting changes*, then *Send now* (or *Put it on the forges*) and *Refresh from forges*: a dry run first, then it asks.

### 20n `W6-people.html` · The conversation's people

- [x] The author first, then each commenter by their latest comment; four faces then +N.
- [x] The popover lists everyone, who opened it, who commented, the viewer marked "you".

### 20o `W7-inbox.html` · The inbox bell

- [x] Following: bell in the text color. New and closed only (a project role): grey bell. Not following: grey bell-off. Hover: red.
- [x] The name and tooltip are `followLine`'s state and reason: "Inbox: following. Because: assignee · commented", "Inbox: not following. You're not in it.", "Inbox: new and closed only. As lead-dev of the project."
- [x] The popover's three radios set the follow on click (automatic, follow, unfollow); Automatic says what it follows now.

### 15b `O2-epfl.html` and 18 `R1-blind.html`

The EPFL rules as applied, with the adherence linter's result; the blind
check of the three choices the notes left open. Records, kept as drawn
(on `drawer.css`).

## Layout

| Region | Rule |
| --- | --- |
| Bar | Today's `.gb-ptop`, 3.5rem high, a hairline under it. Left: the view's name (ellipsis), "›" (EPFL's breadcrumb separator), the number in mono, the Closed badge. Right: the forges, the bell, More, a separator, previous, next, a separator, copy link, expand, open as a page, close. |
| Column | *Fit*: `--gb-fit`, 52rem, centred; *Fill*: the panel's width, the prose kept at Fit's (`--gb-prose`). The head, the tab row and the conversation share it. |
| Title | `--fs-h4` (1.5rem), normal weight, `--lh-normal`; `--fs-h5` under 30rem. |
| Fields line | A wrapping flex row, gap `var(--space-2) var(--space-3)`, `--fs-sm`; the status stepper first. |
| Branch line | A wrapping flex row, gap `var(--space-2)`, `margin-top: var(--space-3)`; a git-pull-request mark first, the plus last. |
| Tab row | Conversation and History with their counts (pills), the faces at the end; the active tab carries EPFL's red dot (0.4375 × 0.375rem) instead of an underline. |
| Popovers | 21rem wide (create a branch 30rem), `--bg`, 1px `--border`, `--radius`, `--shadow`, padding `var(--space-3)`; anchored under their control, inside the drawer. |
| Tooltips | `--bg-dark`, `--fg-inverse`, `--fs-xs`, max 22rem, wrapping. |

## Design tokens

The drawer uses `web/epfl-design`'s tokens, vendored from the skill and
identical to the files the boards were drawn with (same hashes): no value of
its own. git-board's value colors (the `:root` block of `gitboard.css`,
`--gb-<color>-bg/-fg/-dot`) are data, set in the project file: they move to a
token file, `web/src/css/tokens/values.css`, where the adherence linter
allows raw values.

| Element | Tokens |
| --- | --- |
| Red chip, blocked flag | `--danger-bg`, `--danger-border`, `--danger-fg` |
| Yellow chip, held flag | `--warning-bg`, `--warning-border`, `--warning-fg` |
| Blue chip | `--info-bg`, `--info-border`, `--info-fg` |
| Green chip | `--success-bg`, `--success-border`, `--success-fg` |
| Merged | `--tertiary` border, `--tertiary-hover` text |
| Draft, closed | `--bg-subtle`, `--border`, `--fg-muted` |
| Forge count | `--warning` with `--fg` text (8:1; white on it would be 2:1); `--danger-fg`, `--info-fg` with `--fg-inverse` for the red and blue badges |
| Icon buttons | `--fg`; hover `--primary`; focus `--primary-hover`; off `--fg-subtle` |
| Status box, arrow | `--border-strong`; hover `--epfl-gray-100` fill, `--epfl-gray-600` border |
| Faces | `--bg-muted`; the viewer `--bg-dark`, `--fg-inverse` |
| Type | `--font-sans`; `--font-mono` for numbers and branch names; `--fs-h4`, `--fs-body`, `--fs-sm`, `--fs-xs` |
| Motion | `--t-fast` on color, background, border and the status's words |

`screens/gitboard.css` and `screens/head.css` are the app's stylesheets,
copied on 2026-10-30: the components' styles are theirs. `screens/drawer.css`
is the drawer as drawn before the build, lint-clean, kept for `O2` and `R1`.

## Components

| Component | Today | What changes |
| --- | --- | --- |
| `IssuePanel.vue` `.gb-ptop` | crumb and position, previous, next, kbd hints, copy link, expand, open as a page, Esc, close | gains the number (and Closed), the forges, the bell, More; the kbd hints move into the names |
| `IssuePanel.vue` `.gb-ihead` | title row (number, title, edit, Close issue), meta line (badge, author, comments, forge links, id, page switch) | the title alone; the meta line goes: the author and date are the first post's, the count the tab's, the forges the bar's, the id and edit in More, Close issue by the composer and in More |
| `IssueSide.vue` | Fields, Spec, Linked, Labels, Development, On the forges, Participants, Inbox | Fields, Labels, Development, Forges, Participants and Inbox move out; Spec and Linked stay where `time-flow` puts them (see *Open points* 1) |
| `IssueBranches.vue` | the Development section: branches, the gate's checks and approvals, Open PR, Merge, Create branch commands | becomes the branch line, a chip's popover and the create popover; ✓ ✗ ● become icons |
| `IssueSync.vue` | Send now, Refresh from forges, the plan, the result | its run moves into a composable the forge popover calls |
| `WaitingChanges.vue` | the waiting changes list | opened from "See the N changes" in the forge popover |
| `GbIcon.vue` | 30 icons, `<path>` only | gains Feather's github, gitlab, bell, bell-off, git-branch, git-pull-request, git-merge, upload-cloud, download-cloud, refresh-cw, cloud-off, alert-triangle, alert-circle, eye, edit-3, slash, arrow-up, arrow-down, arrow-right, rotate-ccw, more-horizontal, copy (circles, lines and polylines written as paths) |
| `ValueMark.vue` | value marks | unchanged |
| new `IssueBar`, `StatusStepper`, `FieldLine`, `BranchLine`, `BranchChip`, `ForgeIcons`, `InboxBell`, `PeopleButton`, `LabelPicker` | — | as drawn |
| new `nextstep.ts`, `branchstate.ts`, `forgestate.ts`, `people.ts`; `inboxcheck.ts` | `followLine` | pure, tested with `node --test` like the other `web/src/*.ts`; `inboxcheck.ts` gains the bell's state |

## States and interactions

| Element | State | Behaviour |
| --- | --- | --- |
| Status box | idle, hover, focus, open | a listbox of the configured statuses; Esc closes |
| Arrow | next, last, closed | moves the status as today's step buttons do; the last closes the issue; closed reopens |
| Arrow | pressed | the words fade out, the PATCH goes, the new words fade in; on an error the old words come back with today's "Not saved" line |
| Value | hover, focus | red text, as a link; opens its field's popover (row 13b's dropdowns) |
| Held flag | hover | tooltip "The GitHub Project says p2"; the prio's popover has Resolve |
| Forge icon | idle, hover, focus, open | red on hover; framed while open; the count's tooltip in words |
| Forge popover | Send now, Refresh | one run of this issue's sync: a dry run first, then it asks before writing (today's `IssueSync`) |
| Bell | following, not, project role | the popover's radios call `PUT /api/inbox/follow/{id}` with true, false, or null (automatic) |
| Branch chip | hover, focus | the sentence in a tooltip and the accessible name |
| Branch chip | click | the popover: checks, review, update, Merge into dev |
| Plus | click | the create popover; Copy copies both commands |
| People | click | the list popover |
| Label tag, +N | click | the picker, checkboxes over the project's list |

## Responsive behaviour

| Width | Changes |
| --- | --- |
| ≥ 56rem | as drawn (20a, 20b) |
| 30 to 56rem | *Fit · Fill* and *Comfortable · Compact* go into More; the column fills; the block wraps (20c, 704) |
| < 30rem | the bar drops copy link and open as a page into More too, the view's name shortens; the title steps down to `--fs-h5`; four faces then +N; popovers keep inside the drawer (20c, 416) |

## Edge cases

- **No forge configured**: no forge icons; the bar keeps the bell and More.
- **A forge out of reach and changes waiting**: the red badge wins; the popover still says how many wait.
- **More than four branches in play**: "+N more" opens the list; merged and closed stay folded.
- **A branch with no gate on a GitHub origin**: outline chip; its popover offers today's *Open PR into dev* (see *Open points* 5 for its name).
- **Local only**: "local" on the chip; its popover gives today's push command.
- **No status workflow configured**: no arrow; the box lists the values found on the issues.
- **A closed issue**: Closed in the bar, the arrow reopens, the branches are folded if all merged.
- **The viewer not identified**: no bell (the inbox needs an identity, as today).
- **Labels from a forge with no list configured**: not shown; `git board lint` reports them, as Settings › Labels › Lint does today for labels outside the project's.

## Accessibility

- Every control is the real element: `<button>` with `aria-haspopup` and `aria-expanded` for the popovers, the status as a listbox button, radios in a group with a label, checkboxes with labels.
- Icon-only buttons carry the words: "Inbox: Following", "GitLab #1: 6 changes wait to be sent", "Move to ready-for-validation", "Previous issue (k)".
- Color never says a state alone: every chip has its icon and its word; every badge its arrow or icon; the bell its two shapes.
- Contrast as drawn: the tinted chips' texts 6:1 or more, the grey draft chip's 4.5:1; the forge count `--fg` on `--warning` 8:1; muted `--fg-muted` 4.7:1 on white; `--fg-subtle` (bell-off) 3.3:1, for a non-text icon.
- Focus order: the bar (breadcrumb, forges, bell, More, previous, next, link, expand, page, close), the title, the status box, the arrow, the values, the branch chips, the plus, the tabs, the faces, the conversation.
- Motion: a 0.15s fade on the status's words only.

## Open points

Not decided on the canvas. The plan marks the milestones that wait on them.

1. *Decided 2026-10-30, see Decisions.* **The side column goes.** J-line 2 moves Fields, Labels, Development, Forges, Participants and Inbox out of it. `time-flow`'s README (row 6) kept "Fields keep the sidebar"; this supersedes it once confirmed. Spec and Linked still need a place: proposed, a line under the branch line while they have content.
2. *Decided 2026-10-31, see Decisions.* **The waits block is gone.** Each wait now sits on its source (*Decisions*). Read from "things live where they matter, always"; to confirm.
3. *Decided 2026-10-30.* **The branch's name in its folder.** Proposed: `[branches] name = "{kind}/{number}/{n}-{slug}"`, so `test/2771/1-implement-tests-to-validate-all-equations`, `test/2771/2-reference-examples`, and `2811/1-…` with no kind. `{n}` is counted, never stored: one more than the highest found among the issue's branches and the merge commits on the base that name one. The issue's number stays the first number in the name, which `branches.Names` needs; a branch named `test/2771` never exists, so git's folder rule holds. A deleted, never-merged branch's number can come back.
4. *Decided 2026-10-30: both, Create and Copy.* **git-board makes the branch.** Proposed: *Create* runs `git branch --no-track <name> <start>` and `git push -u origin <name>`, never `git switch` (it would change the checked-out branch of the clone the server runs in), only when the server listens on localhost, as `wtx` runs are guarded (`worktrees.go`); otherwise the popover keeps *Copy the commands*. The popover then gives `git switch <name>`.
5. *Decided 2026-10-30.* **The words.** Proposed: a branch's three steps are *Branch*, *Review*, *Merge*; the chips name the branch (2771/2), and the gate's number (#3155) moves into its popover as "The gate: GitHub #3155"; *Open PR into dev* becomes *Ask for review*, which opens the gate. *Mark ready for review* and *Start verification* (`cycle-protocol.md`) keep their names.
6. *Decided 2026-10-30, built.* **Several branches and the cycle: branched out.** `cycle.Gather` follows the newest branch (`gather.go`); with branch 2 in review and branch 3 started, which one is step 6's? guilbep asked, 2026-10-30: "I guess it should 'branched out'?" Proposed, branched out: Intent, Risk and Plan stay the issue's, once; from Tests on, each branch of the issue's folder runs its own lane, Tests, Code, Review (branch, review, merge), drawn as one row per branch (2771/1, 2771/2) under steps 4 to 6 of the step bar; the lanes join again at Verify, which opens once every branch is merged or closed, and the sign-off closes the issue as today. The issue's own step, and the status the cycle proposes, is its least advanced open lane, so a new branch never hides behind a merged one. To confirm; then W3-5.
7. *Decided 2026-10-30.* **The labels key.** Proposed: `[labels] list = [...]`, after `[repos] list` and `[sprints] list`. It supersedes row 8a's `[issue] labels = true`.
8. *Decided 2026-10-31, built.* **"New on the forge".** The blue "↓3" needs a check (a dry run) to know; proposed only after a Refresh, never polled.
9. *Decided 2026-10-31: the PM read the arrow right; it stays.* **The arrow without words.** The blind check found icons alone misread on chips; the arrow sits on the status it moves and its tooltip names the target. A quick check with the PM before the words go.
10. *Decided 2026-10-31: later.* **Dark mode.** `dark.css` redefines the app's colors; the drawer uses only semantic tokens, but the tints' contrast in dark needs a pass once `dark.css` is on tokens.
11. *Decided 2026-10-31: all three.* **The position in the bar.** Today's crumb shows the view and the position ("3 / 12"); the boards show the view and the number. Keeping all three is proposed: *current-sprint 3/12 › #2771*.
