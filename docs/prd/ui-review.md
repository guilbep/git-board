# git-board · UI review: one interface, said once PRD

Nov 1, 2026 · @Pierre Guilbert

A review of every page, menu, dialog and hover state of the web app, on this repository's own board (38 issues, the real `.git-board.toml`), at 1440 px, on a 390 px phone and in dark mode. It found 46 problems, numbered as in the review (1–33 from the pages, 34–46 from the menus, dialogs and hovers). The layout bugs are fixed on this branch; the rest is proposed here, each with what "done" means, for review before any of it is built.

A Playwright test pins each fix, as for the layout bugs (`web/e2e/layout.spec.ts`). A proposal that undoes an earlier PRD's decision says so on an **Overrides** line.

## Status, Nov 2, 2026: built

Built overnight on this branch, in the order below; each step's Playwright tests are in `web/e2e/review.spec.ts` and `layout.spec.ts`, the Go ones beside their package. Where the plan left a choice, the call made is marked **(call)**, for review.

| # | Done | How |
| --- | --- | --- |
| A-1…7, 40 | yes | 2a2a4e2, e29c551, 6575f67 (above). The release page's two columns were never a grid either: fixed (abd1973). |
| B-8 | partly | Work's box is **For the forges** and links to Sync. The header's count is unchanged: it adds the git push to the last check's tallies, and a board-wide count of what waits for the forges needs a server count (`ways.ts` notes it). |
| B-9 | yes | `doc = "docs"` in your uncommitted file; Branches marks a kind the kind field lacks, and says how to fix it. |
| B-10, B-11, B-37 | yes | The release summary counts binaries; Attachments leads with the server switch; the forge popover's note follows its buttons. |
| B-12 | yes | Work names an issue by GB-N; the table's column is **On the forge**, "GitHub #18". |
| B-13 | yes | New lint check **near-value** (`risk:-normal: did you mean risk:normal (2 issues)?`), and unknown-value names the option a typo meant. README updated. |
| B-14 | yes | The flow says "closed ones included"; an open issue at done is "1 open issue says done: close it?". |
| B-15 | yes | The tab is **The file**, its button **Save…** (the review follows). |
| C-16 | light **(call)** | The reference keeps its place and drops its pointer line; each tab's intro is one sentence that no longer repeats it. |
| C-17 | yes | The empty board suggests the filter one term shorter. |
| C-18 | no **(call)** | "Board" kept: renaming the default view touches every board's file for a word. |
| C-19 | **(call)** | The Sync tab is the one home: the header links to it, Work's Send all and Check first are gone, the issue popover keeps Send now for one issue. No header panel (the tab is one click away). |
| C-20 | yes | No Overview: Settings opens on Sync, and each menu item says where it stands (origin…, 23 labels, off on this server, as guilbep). |
| C-21, C-22, C-35, C-43 | yes | |
| C-34 | **(call)** | The header's 🔍 stays (every page has a header); the sidebar's copy went. The opposite of the proposal, for one header everywhere. |
| C-36 | yes | The ⋯ menu's Close went; the ✓ advance (W0-3) and the comment box's Close stay. |
| D-23 | yes | Labels in three parts: Plain labels, On GitHub (both GitHub tables), Lint (a missing value or a dead option one line per field, "Show them" filtering the board). No `[lint]` key **(call)**: grouped, the lines no longer bury the page. The missing-token error says what to do. |
| D-24, D-27 | yes | One-sentence intros; the guide's examples are `acme/app`; `start_weekday` and `[repos] assignees` left the reference. The EPFL upload defaults stay, named as EPFL's. |
| D-25, D-26, D-44 | yes | Placeholders "e.g." or "empty:", italic and lighter; a disabled Save says why; Mark all read shows only with something unread; focus in the text color. The forge rows' disabled buttons keep their hover reason. |
| D-28 | yes | Lead time leaves out issues opened and closed in the same minute and says how many (on this board 8: the median goes from 1 h to 2 h). The time machine spans the row; the table's toggle says "Show". |
| D-29, D-41 | yes | Work grouped by step and what it waits for, five a group; Open only when it is not the title's link; Start an agent on hover or focus (always on touch); a row's result on the row. |
| D-30 | partly **(call)** | The forge number shows beside its logo. The switches already have words and labels: unchanged. The avatar beside the tabs is the conversation's people (people.ts), not the viewer: the proposal was wrong, it stays. |
| D-31 | yes | Empty sections left out; the Cut dialog's checks only with checks. |
| D-32 | yes | Conflicts in the page frame; Releases' eyebrow is the section, its title a sentence; settings rows at one input size; the inbox menu with the others' shadow. |
| D-33, D-38, D-39, D-42, D-45, D-6 | yes | Field types in words; New issue's inputs by type, its actions in reach; a half-typed term not applied; a declared preset opens its field; the Project's number set under its row; the phone board one lane a screen. |
| D-46 | **(call)** | The lane's control stays a select (one action: a menu would hold one item) and says **Move all…** instead of ⋯. |

Open questions, as answered by the calls: 1 no key; 2 kept; 3 dropped; 4 kept (the tab is the home); 5 light; 6 neither (they were labelled).

## Problem

The app grew page by page, and each page explains itself, offers its own way to do the common things, and styles its own controls. Four things follow:

1. **Things break at ordinary widths.** Names cut to `stat…`, an editor slid under its neighbour, a table that breaks an address into syllables on a phone.
2. **Pages contradict each other.** "1 to send" in the header, "2 changes" on Work. "no releases" in Settings, 18 on the Releases page.
3. **The same thing is offered or said two or three times.** Each settings tab's intro is repeated in its Reference panel; Sync is a header link, a tab's button, a Work button and an issue popover; an issue closes from three places.
4. **Some pages can't be used.** Labels is a 5,000-pixel page where 50 lint lines bury the table; the issue page has six unlabelled controls; disabled buttons don't say why.

## Goals and non-goals

Goals:

1. Nothing cut or overlapped from 390 px to 1920 px, light and dark.
2. One number, one name, one place for each thing: a count reads the same on every page, an action has one home.
3. A page says what it is in one line; the reference is one click away, not beside every form.
4. Every control says what it does (label or visible text), and every disabled control says why.

Non-goals: a visual redesign (the EPFL tokens and type stay), new features, the CLI.

## A. Bugs and broken layout: fixed on this branch

| # | Problem | Status |
| --- | --- | --- |
| 1 | The save bar covered the last item (Views, Branches, Release, phone Fields) | **Not a bug**: an artifact of full-page screenshots, which freeze a sticky bar at the first screen's bottom. Scrolled in a real viewport, the bar sits after the last item. |
| 2 | Field names cut in the Fields list (`stat…`) | **Fixed** (2a2a4e2): the name keeps its width, the summary shrinks first. |
| 3 | The Fields editor's head squeezed, Remove over the reference; the Roles × over it | **Fixed** (2a2a4e2, e29c551): the card head wraps; a card never grows past its column (a select's per-value table pushed the whole editor under the reference); a row's inputs shrink with it. |
| 4 | Hints broke around code chips (". One login each") | **Fixed** (2a2a4e2): `.gb-hint` was `display:flex`, so every text run and `<code>` wrapped on its own; it is a paragraph now. One rule, every hint. |
| 5 | The Sync table broke words into syllables on a phone | **Fixed** (2a2a4e2): the table keeps a minimum width and scrolls sideways. |
| 6 | The board on a phone showed one lane | **Not a bug**: the board scrolls sideways, as kanbans do. Proposed in D-6: one lane a screen, snapping. |
| 7 | Forge fields' "Written to" panel overflowed; "blue = an override" with nothing blue | **Fixed** (2a2a4e2): the panel wraps; the legend shows with an override. |
| 40 | The Columns menu cut its list mid-row, no sign it scrolls | **Fixed** (6575f67): the menu takes the screen's height (every column at 900 px), a shade at the cut past it. Checked on this board; not pinned by a test (the e2e board has no issues, so no table). |
| — | A release cut from the current branch | **Not a bug**: the review ran on a clone whose `origin/HEAD` is this branch; the trunk is origin's default branch. |

## B. Contradictions: proposals to validate

Each has a proposal and, where there is a real choice, the alternative.

**B-8. "1 to send" in the header, "2 changes" on Work.** Not the same thing: the header counts issues waiting for **origin** (a git push, `ways.ts` `waiting()`); Work counts changes waiting for **the forges** (`/api/work`'s `pending`, #51's risk and its description). Neither page says which.
Proposal: every count names where it goes. The header: "1 to push · 2 for GitHub" (it reads `pending` too, the board-wide forge count `ways.ts` notes it lacks); Work: "For GitHub: 2 changes on 1 issue"; Sync's origin row: "1 issue to push". Touches: `ways.ts`, `work.ts` `sendWords`, the header, `/api/work`.

**B-9. `doc` vs `docs`.** Done (your call, made): `[branches.kinds]` maps the kind `doc` to the branch prefix `docs` (`doc = "docs"`); your issues carry `kind:doc`. In your uncommitted `.git-board.toml`, not committed. It doesn't clear lint's "no-kind": that one is GB-9 and GB-22, which carry no kind at all.
Proposal for the code: Branches › Kinds marks a key the kind field doesn't hold: "not a kind: no issue can carry it" (the input stays free text, with the kind field's values suggested). In this file that marks `design`, `perf`, `refactor`, `style` and `test` (the kind field holds feat, bug, doc, chore): either they go, or the kind field gains them. Touches: `BranchesSettings.vue`.

**B-10. Release settings says "no releases", the Releases page lists 18.** The summary counts environments only; a `binaries` table makes releases with none.
Proposal: the summary says what is released: "1 binary · finals, published on GitHub", "3 environments · 2 images", "off: no environments and no binaries".

**B-11. Attachments "Off" on the overview, "the defaults" on the tab.** Both true: the server has no S3 keys in its environment (off), and the file has no `[uploads]` table (the defaults).
Proposal: the tab leads with the switch that decides, as a status line: "Off on this server: set ACCESS_KEY_ID, ACCESS_KEY_SECRET and BUCKET_NAME where git-board runs." Under it, the file's keys, titled "Where the files go, once on". The overview says the same first line.

**B-12. Two numbers per issue** (`#29` and `GB-22` side by side on Mine and Work).
Proposal: the project number (`GB-22`) is the issue's name everywhere; a forge number shows only where it links to the forge, spelled with its forge: "GitHub #29". Mine's "Forge" column becomes that link; Work's rows lead with `GB-22`.

**B-13. `risk:-normal` and `risk:normal`.** A data typo nothing surfaces outside Labels.
Proposal: lint's "unknown value" for an undeclared field whose values differ by punctuation only says "did you mean `normal`?", with Fix (rewrites the label on the issue). Undeclared fields are where typos live; declaring `risk` (B-14) makes it plain lint.

**B-14. "5% of the open issues are done"; `done 16` in the flow, `done 1` in the donut.** The flow counts every issue (closed ones keep `status:done`); the donut counts open ones, and one open issue says done.
Proposal: the flow says so ("every issue, closed included"), and an open issue at the last status is a nudge, not a statistic: the donut's sentence becomes "1 open issue says done: close it?", linking to it. Alternative: closing on the last status automatically (`git board close` already sets it; the reverse is new behaviour, so not proposed).

**B-15. "Advanced", "Project file", "Project configuration", "Review changes".** One page, four names; one button unlike every Save.
Proposal: the tab is **"The file"** (the side menu's hint stays "`.git-board.toml`"), the page title the same, the toml's header comment updated; its button is **Save**, the review step a dialog Save opens (as now), so every tab saves the same way.

**B-37. The issue's forge popover: "Waiting changes go out at the next Sync, or now:" then "Refresh from forges".** The text promises a send; the button pulls.
Proposal: two buttons, each saying its side: "Send now" (when there is something to send) and "Refresh from GitHub".

## C. Things shown twice: removed, not reworded

The rule: **one home per thing, and links to it.** A page that needs a thing it doesn't own links to the owner's page; it doesn't copy its text or its button.

**C-16. The intro repeated in the Reference panel** (Fields, Views, Sprints, Roles, Labels, Branches, Cycle, Repos).
Proposal, light: the panel stays and loses its first paragraph, the one that repeats the tab's intro; nothing else moves.
Proposal, full: the Reference panel goes from every tab. Each tab keeps one line of intro (goal 3) and a "Reference" link at its end that opens the section's reference in a side sheet (the same text, from `docs/guide/configuration.md`, the one source). The forms get the page's full width, which also fixes the three-column squeeze at 1440 px behind A-2/A-3.
Overrides (full only): `settings-forms-plan.md` › The reference, per tab.

**C-17. The open filter twice** (`is:open` in the box, "open" ticked in State), and the empty state suggesting `is:open` when it's already there.
Proposal: the facet panel is the filter's other face, so it stays; the box shows the same query. The duplicate is the empty state's advice: it suggests a filter one step broader than the current one (drop the last term), never the current one.

**C-18. "Board" is both a view (tab) and a layout (switch).**
Proposal: the default view is named for what it shows, not how: "Open issues" (the file's `[views.board]` keeps its key; `title` is the shown name). The layout switch keeps Table / Board / Roadmap.

**C-19. Sync has four entry points** (the header, the Sync tab's Check/Sync bar, Work's Send all, the issue popover).
Proposal: the header's **Sync** is the one entry: it opens a panel with the counts (B-8), Check, and Sync. The Sync tab keeps the per-side rows for when one side needs a hand, with their verbs as sync-sides set them (git's fetch, pull, push for origin; pull, push, sync for a forge: different sides, different verbs, on purpose), and drops its own bottom bar. Work's "Send all" goes; its "To send" box links to the header panel. The issue popover keeps "Send now" for that one issue (B-37).

**C-20. The overview cards repeat the side menu, for some tabs.**
Proposal: Settings opens on the first tab (Sync); the overview's sentence on what git-board is moves to the side menu's head, its per-tab state to each menu item's hint ("1 forge · Project #40", "7 fields", "off").

**C-21. Advanced's `+ View / + Sprint / + Field` buttons** repeat the forms' Add buttons.
Proposal: they go; the file editor is for the file.

**C-22. "github · github · guilbep/git-board".** The forge's name and kind are both "github".
Proposal: the title says the kind once when the name is the kind: "GitHub · guilbep/git-board"; the name shows only when it differs ("work (GitHub) · EPFL-ENAC/x").

**C-34. Two buttons open the same command palette** (the header's 🔍 and the sidebar's "Filter or jump ⌘K").
Proposal: the header's 🔍 goes; the box in the page is the one (⌘K everywhere). On pages without the box (settings, releases), the header shows the box, not an icon.

**C-35. The avatar menu shows the name twice** (guilbep / guilbep) and repeats Sync and Settings.
Proposal: the menu is "You" things only: who you write as (once, "guilbep" with the identity's name if it differs), your issue page, appearance, keyboard shortcuts. Settings and Sync are already in the header.

**C-36. An issue closes from three places** (✓ beside the status, ⋯ › Close the issue, Close issue under the comment box).
Proposal: two. The ✓ is the status box's advance arrow (`issue-head-plan.md` W0-3, W1-6: on the last status it closes), and the comment box's "Close issue" stays (closing with a reason). The ⋯ menu's "Close the issue" goes. The ✓ gets a visible tooltip and label at its last step: "Close as done".

**C-43. "How forges work" is the Sync intro, a dialog, and the Add-a-forge wizard's side panel.**
Proposal: one guide page section; the Sync tab links it once; the wizard keeps only the step's own help ("Which forge: GitHub or GitLab. GitHub was found in your remotes: guilbep/git-board", which it also doesn't say today).

## D. Confusing or unusable: proposals

**D-23. Labels: one endless page.** Today: the plain labels form, a 23-row table of field labels, "On the issues, not declared", 50 lint lines one per issue per field, then GitHub's other labels, then the reference.
Proposal: three tabs inside Labels, each a list that fits a screen:

- **Plain labels** (the form; the only thing saved to the file here).
- **On GitHub**: one table, every label the project defines or GitHub has, with one state column (same / color differs / missing / GitHub only) and a filter on that state; Adopt and Delete per row, Send to GitHub for the whole. Today's two GitHub tables become this one.
- **Lint**: grouped by **field, then issue count**, not one line per issue per field: "est: 32 open issues have none · Show them" (links the board filtered `no:est is:open`), "moscow: 3 values no issue carries (musthave, couldhave, wonthave)", "status: 1 issue carries ready-for-validation, not an option · Add it / Migrate". A rule the project doesn't want ("every issue has est") gets "Don't check this" (a `[lint]` key, new: the one new key in this PRD; to validate).

The token error reads as a sentence: "GitHub can't be read: no token for github.com. Set GITHUB_TOKEN or log in with git's credential helper." (`no token for ""` today: an empty login in the message.)

**D-24. Long explanations** (the Sync intro is nine lines; Cycle's, Branches' and Repos' references are essays; keys documented as unused).
Proposal: one sentence of intro per tab, written for the decision the tab asks; the rest goes to the guide (C-16). Keys the code doesn't read (`start_weekday`, `[repos] assignees`) leave the reference; `git board lint` warns on them in the file.

**D-25. Placeholders that look like values** (Release: `make ci`, `origin`, `chart`; Views: `status`).
Proposal: a placeholder starts with "e.g." and is lighter than the value text; an empty optional input with a default says it beside it: "empty: origin". One style in `gitboard.css`, used by every form.

**D-26. Disabled buttons don't say why** (the main Sync, Mark all read, Save; the forge row's reason is a hover title only).
Proposal: a disabled button carries its reason as visible text beside it or under the group ("Run Check first", "Nothing to save", "Nothing new"), and as `aria-describedby`. A `title` alone doesn't reach touch or keyboard users.

**D-27. EPFL-specific text in a generic tool** (S3 and CDN defaults, `registry.epfl.ch/enac`, "the ten IT4R kinds", co2-calculator in examples).
Proposal: placeholders and examples use `example.org` and `acme/app`; the EPFL defaults stay where they are configuration (the uploads defaults are behaviour: they stay, and the Attachments tab names them as EPFL's).

**D-28. Insights' "median lead time 1 h"** (imported issues open and close at import), the time machine alone on its row, "The numbers, as a table" with nothing under it.
Proposal: lead time leaves out issues opened and closed within a minute of each other (an import), and says how many it left out. The time machine spans the row. The table heading is a disclosure ("Show the numbers as a table").

**D-29. Work.** 19 rows, all "01 Intent", each with Start an agent and Open; "No current sprint: [sprints] is the retro's clock"; Work isn't in the header.
Proposal: rows grouped by step ("01 Intent · 17 issues wait for acceptance criteria"), collapsed past five; the row is the link (Open goes); Start an agent on hover and focus. The retro box says "No sprint is running. Set sprints in Settings › Sprints" with the link. Work joins the header when the cycle is on (it is the cycle's page).

**D-30. The issue page's unlabelled controls** (GitHub icon, bell, ⋯, Fit/Fill, Comfortable/Compact, ✓, a floating avatar).
Proposal: icon buttons get visible labels at ≥ 1024 px ("GitHub #17", "Following"); Fit/Fill and Comfortable/Compact keep their place and gain a group label each ("Width", "Spacing"); the ✓ is labelled (C-36); the avatar beside the tabs (the viewer) goes. The text column widens to 52 rem at "Fill".
Alternative: Fit/Fill and Comfortable/Compact move into ⋯ › Layout. Overrides: `issue-head-plan.md` (the bar's switches, `headswitch.ts`).

**D-31. The release page.** Empty "Runs in" and "Images" headings; the comment box between Conversation and the release's details; headings in two cases. The Cut dialog's empty "Before it is cut".
Proposal: a section with nothing shows nothing (or one muted line, "No image"); the order is the record, then the conversation, the comment box last; one heading style (the h2 of Notes). The Cut dialog lists its checks under "Before it is cut", or drops the heading.

**D-32. Pages built differently** (margins 32 vs 48 px; the eyebrow is the page on some pages, "GIT-BOARD" on Releases; Worktrees and You with their own layout and a red primary instead of the save bar; inputs in two sizes; popovers in two styles).
Proposal: one page frame component (gutter, eyebrow = the section, title, one-line intro) used by every page; settings forms use one input size (`.875rem`) and one save bar (Worktrees' "Write wtx.toml" is its Save; You saves at once and says so); one popover style.

**D-33. Small labels.** Field names in red mono in tables (reads as an error); "guessed · one"; `due` defaulting to several.
Proposal: names in the text color, mono; "guessed" becomes the type's word ("a select", "a date"); a new date field, or an issue field named `parent`, defaults to one value. The Fields form already follows the type (7e564ba); the default cardinality follows it too.

**D-38. The New issue dialog isn't typed** (Due and Parent ask "comma-separated, or @me"; Risk has no hint), its Create button falls below a 900 px screen, and the title is red before anything is typed.
Proposal: each field's input follows its type, as the Fields form does: a date picker, an issue picker (`#` search), a person picker, a select. The dialog's body scrolls and its footer stays; required shows on submit, not on open.

**D-39. The filter applies each keystroke**: typing `prio:` shows 0 issues and offers "Save view" for a half-typed query.
Proposal: an incomplete term (a field with no value) isn't applied; the suggestions list its values. The unsaved-view bar appears once the query parses.

**D-41. Work: an action's error shows at the top**, far from the row clicked.
Proposal: an action's result shows on its row (inline, as Settings' rows do); the page-level line is for the page's own load.

**D-42. "+ Add a field" offers fields that exist** (status, prio).
Proposal: a preset the file has shows as "status · already declared" and opens that field.

**D-44. The focus ring is the brand red**, so every focused input reads as invalid.
Proposal: focus is the text color (as the forms already do with `outline: 2px solid var(--fg)`), red is for errors only. One rule in `gitboard.css`.

**D-45. Sync › GitHub Project › Change reflows the whole table.**
Proposal: Change opens the same small form in a popover anchored to the row.

**D-46. A lane's "⋯" is a "move every card to another lane" select.**
Proposal: the ⋯ is a menu (Move every card to…, Hide this lane, Set a limit); the select is its first item.

**D-6. The phone board** (from A-6): one lane a screen, swiped, the lane names as tabs above.

## Order

1. **The cheap, global ones** (one rule each): D-44 focus, D-25 placeholders, D-26 disabled reasons, D-33 labels, C-22, B-10, B-11, B-37.
2. **One home per thing**: C-16 (the Reference panel goes; unblocks the widths), C-34, C-35, C-36, C-21, C-43, then C-19 (Sync), the largest.
3. **The counts and names**: B-8, B-12, B-15, C-18, B-14, B-9's select.
4. **The pages**: D-23 Labels, D-29 Work, D-30 issue page, D-38 New issue, D-39 filter, D-31 release, D-28 Insights, D-32 frame, D-6 phone board, D-45, D-46.

Each step lands with its Playwright tests (fail first, collect page errors) and a screenshot pass at 390, 1440 and dark.

## Open questions

1. D-23's `[lint]` key to silence a rule: the one new key proposed. Or a per-field `required = false`?
2. C-18: rename the default view's shown name, or leave "Board"?
3. C-20: drop the Settings overview, or keep it as the page Settings opens on?
4. C-19: is Sync per side (the tab's rows) still needed once the header panel exists, or only behind "Advanced"?
5. C-16: the light proposal (drop the repeated paragraph) or the full one (the panel becomes a sheet)?
6. D-30: label the issue bar's switches, or move them into ⋯?
