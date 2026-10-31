# The issue head, finished

PRD, 2026-10-30, after the redraw of the design screens from the build
(`docs/design/issue-head/README.md`, rows 20 and 21 of the canvas). The
first pass is `docs/prd/issue-head-plan.md`, its *Progress* says what landed
on 2026-10-30, all of it still uncommitted on `main`
(`docs/prd/issue-head-handover.md`). This document says what is left to
call the head done, and takes a call on each point the design left open.
The user asked for the plan and the PRD and took no question, so every
decision marked **(call)** is this document's, open to change; what the
user said before is marked **(user)**.

## Why

The head v2 runs: the bar holds the issue's identity, one block of fields
and branches sits under the title, the conversation shares the column, Fit
and Fill, Comfortable and Compact are kept per browser, a value opens its
own editor, a branch is named by its place in the issue's folder and the
cycle branches out with it. Three things stop it from being *the* head:

- **The bar breaks between 30 and about 56rem.** *Fit · Fill* and
  *Comfortable · Compact* leave the bar only under 30rem (the container
  query); above it they stay, and from about 56rem down they push the crumb
  (`flex: 1; min-width: 0`) to nothing while the forge icons draw over it.
  A side panel at its default width, 64rem, is fine; dragged to 704px, or
  the wide panel on a 1280px screen, is not. Found by the redraw (20c, 20e).
- **Two rows of the README were never built.** The red dot under the active
  tab (decision *EPFL to the letter*, row 15b): the app still draws its 2px
  dark underline. Three faces then +N on a phone (decision *Participants*):
  `PeopleButton.vue` shows four at every width.
- **The old head is still there.** v1 and its side column live behind
  More › *The previous head* (`gb-head = v1`); `IssueSide.vue`,
  `IssueBranches.vue` and the meta line carry code the new head replaced.
  Until they go, every fix is made twice or not at all, and the team's
  board has two heads.

Behind those, the first plan's leftovers: the forge badges for *out of
reach* and *changed on both sides* exist in `forgestate.ts` and `head.css`
but `ForgeIcons.vue` never shows them (W2-2); *new on the forge* is not
built (W3-6, open point 8); the cycle's own pages still say PR where the
head says branch, review, merge (W3-4); the value colors sit in
`gitboard.css`'s `:root` where the adherence linter counts them (W0-2); the
linter has no Makefile target (W0-9); `gitboard.css`, `dark.css` and
`app.css` keep their old problems (E-*); the folder form of branch names
has no test for the names it must not link (W0-10); the guide has its
*Branches* and *Labels* but `docs/implementation-plan.md` has no status
update for the pass (D-1). And five open points of the README wait for a
word: 2, 8, 9, 10, 11, with the default spacing.

Done means: one head, the README's criteria all ticked or struck with a
decision, the bar whole at every width, the open points closed, `make ci`
green and `make docs` green.

## Decisions

| Topic | Taken |
| --- | --- |
| The bar between 30 and 56rem | Under 56rem of drawer (a second `@container` step), *Fit · Fill* and *Comfortable · Compact* leave the bar and go into More as two items, *Fill the panel* and *Compact spacing*, checkbox items that say their state; *Fit* and *Comfortable* are the unchecked states. The crumb keeps `min-width: 10rem` at every width, so no row of icons can take it to nothing; the view's name shortens first, as under 30rem. The 30rem step stays as it is. **(call)** |
| The red dot | Built, for the new head's tabs only: `.is-head-v2 .gb-tab[aria-selected="true"]::after` becomes EPFL's dot, 0.4375rem by 0.375rem, `--primary`, centred under the tab, in place of the 2px underline; the tab's text keeps its weight. The board's other tabs keep their underline until a pass of their own. **(call)** |
| The faces on a phone | Four faces then +N at every width, as built: at 416px four faces and "+2" fit beside the tabs (20c), and a second rule for one width is a rule to maintain. The README's *Participants* row changes to say four. **(call)** |
| The old head goes | W3-1 as the first plan has it, now: the switch `gb-head`, More › *The previous head*, the meta line's *The new head*, `IssueSide.vue`'s Fields, Labels, Development, On the forges, Participants and Inbox, `IssueBranches.vue`, and the v1 branches of `IssuePanel.vue`'s template go. `IssueSide.vue` stays for Spec and Linked issues, under the branch line. The class `is-head-v2` stays as a name, set always, so `head.css` changes nowhere. **(call)** |
| Forge badges | W2-2 as the first plan names the sources: *syncing* from `forgeStatus.job` running and not a dry run; *out of reach* from the `left` for that forge of the newer of this issue's own last run and the server's job; *changed on both sides* from this issue's last check in the popover (*Refresh*'s dry run: that forge's tally has `conflicts`), kept per issue until its next run; *not on it yet* from a configured bridge the issue has no forge for. Precedence as `forgestate.ts` has it: out of reach, both sides, syncing, the count. **(call)** |
| New on the forge | Open point 8, closed: after *Refresh from forges* in the popover, each forge whose dry run would bring something in shows a blue "↓N", N the count of what comes from it, until the next run of any kind on that issue; never polled, never after a full sync from Settings. The badge's words: "GitLab #1: 3 new there since the last sync". Precedence: after the count to send. When both apply, the blue count sits in the icon's bottom corner, the yellow one at the top (2026-10-31: as one pill, "↑6 ↓3" hid the forge's shape). **(call)** |
| The waits block | Open point 2, closed: it stays gone. Each wait sits on its source: a hold in the status box, a held flag on its field, a forge's count on its icon, a branch's state on its chip. **(user, 2026-10-29: "things live where they matter, always")** |
| The arrow without words | Open point 9: the arrow stays, its tooltip and name saying the target. One check with the PM, five minutes, before the milestone that retires v1 lands: the PM is shown 20d cold and asked what the arrow does. If they say "it moves the status on", done; if not, the arrow gains its target's words at 64rem and wider (`Move to backlog →`), the icon alone below. **(call)** Checked 2026-10-31: the PM read it right; the arrow stays as it is. **(user)** |
| Dark mode | Open point 10: not in this pass. `dark.css`'s values move to `tokens/dark.css` with E-*, nothing else; the tints' contrast in dark is a pass of its own once that file is on tokens. **(call)** |
| The crumb's three parts | Open point 11, closed: the view, the position and the number stay, as built: *current-sprint / 3 of 12 › #2771*; under 30rem the view's name shortens first. **(call)** |
| The default spacing | Comfortable stays the default; Compact stays, opted into. Neither goes in this pass: the user has not used both yet. **(user, 2026-10-30: "once Pierre has used both")** |
| The cycle's words | W3-4 finished: on the cycle's pages, "PR #N" and "Draft PR #N" become "GitHub #N" and "GitHub #N, a draft", as the head's popovers say it; a sentence that explains says "the gate", "in review", "merged". *Mark ready for review* and *Start verification* keep their names (decision 5 of the README). **(call)** |
| The linter and the old problems | `make web-lint` reports, not in `make ci`, until the count is 0. In this pass the files the head touched go to zero: `gitboard.css` (its value colors to `tokens/values.css`, the rest to tokens and rem), `dark.css` (to `tokens/dark.css`), `app.css` (the hex fallback), `LabelsSettings.vue`. The other 46 files wait for a pass of their own. **(call)** |
| The CSS-only fixes have no DOM test | The web tests run in Node, without a DOM: the task lists as text, the browser margins, the loose checklist's boxes get no regression test. The screens `V1`, `V7` and `V8` are their reference; a change to `gitboard.css`'s `.gb-md` rules is checked against them by eye. Said here so it is not a surprise at review. **(call)** |
| Dead rules | `gitboard.css`'s `.gb-md ul.gb-tasks` rules (four lines) have no producer since the server's task lists dropped the class: removed. **(call)** |

## Out of this pass

- A dark theme for the head's tints (open point 10, after `tokens/dark.css`).
- The 46 other files `make web-lint` names (E-*, the rest).
- `[issue] top`, a key for which fields the line shows first (the first
  plan's *Calls taken* 1 stands: assignee, prio, sprint, kind).
- co2-calculator's `.git-board.toml`: if it sets `[branches] name`, it needs
  `{n}` for the folder numbering. Its repository, not this one; a note for
  the user.

## Open after this

Nothing of the README's *Open points*; the pass closes 2, 8, 9 (with its
check), 10 (deferred by decision), 11. What stays open is the default
spacing, by the user's word, and the dark pass.

## The plan

`docs/prd/issue-head-m2-plan.md`: eleven milestones in three waves, each a
day or less on its own branch, the retirement of v1 last but for the docs.
