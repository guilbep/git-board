# git-board · UI review: one interface, said once PRD

Nov 1, 2026 · @Pierre Guilbert

A review of every page, menu, dialog and hover state of the web app, on this repository's own board (38 issues, the real `.git-board.toml`), at 1440 px, on a 390 px phone and in dark mode. It found 50 problems. The layout bugs are fixed on this branch; the rest is proposed here, each with what "done" means, for review before any of it is built.

The screenshots are in the session's scratchpad (`shots/`, `ui/`); a Playwright test pins each fix, as for the layout bugs (`web/e2e/layout.spec.ts`).

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
| 6 | The board on a phone showed one lane | **Not a bug**: the board scrolls sideways, as kanbans do. Proposed in D-19: one lane a screen, snapping. |
| 7 | Forge fields' "Written to" panel overflowed; "blue = an override" with nothing blue | **Fixed** (2a2a4e2): the panel wraps; the legend shows with an override. |
| 40 | The Columns menu cuts its list mid-row, no sign it scrolls | To do: the menu grows to the viewport, a fade at the cut. |

## B. Contradictions: proposals to validate

Each has a proposal and, where there is a real choice, the alternative.

**B-8. "1 to send", "2 changes", "1 issue to send".** The header and Sync count issues; Work counts changes (#51's risk and its description).
Proposal: count **issues** everywhere, the changes as detail: "1 issue to send (2 changes)" on Work, "1 issue to send" elsewhere. One function on the server answers both numbers; every page reads it.

**B-9. `doc` vs `docs`.** Done (your call, made): `[branches.kinds]` maps the kind `doc` to the branch prefix `docs` (`doc = "docs"`). Your issues carry `kind:doc`. In your uncommitted `.git-board.toml`, not committed.
Proposal for the code: Branches › Kinds offers only the kind field's values as keys (a select, not free text), so a key the field lacks can't be typed; an existing one shows "not a kind" in red, like lint.

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
Proposal: the Reference panel goes from every tab. Each tab keeps one line of intro (B goal 3) and a "Reference" link at its end that opens the section's reference in a side sheet (the same text, from `docs/guide/configuration.md`, the one source). The forms get the page's full width, which also fixes the three-column squeeze at 1440 px behind A-2/A-3.

**C-17. The open filter twice** (`is:open` in the box, "open" ticked in State), and the empty state suggesting `is:open` when it's already there.
Proposal: the facet panel is the filter's other face, so it stays; the box shows the same query. The duplicate is the empty state's advice: it suggests a filter one step broader than the current one (drop the last term), never the current one.

**C-18. "Board" is both a view (tab) and a layout (switch).**
Proposal: the default view is named for what it shows, not how: "Open issues" (the file's `[views.board]` keeps its key; `title` is the shown name). The layout switch keeps Table / Board / Roadmap.

**C-19. Sync has four entry points** (the header, the Sync tab's Check/Sync bar, Work's Send all, the issue popover), with different verbs per row (Fetch/Pull/Push vs Pull/Push/Sync).
Proposal: the header's **Sync** is the one entry: it opens a panel with the count (B-8), Check, and Sync. The Sync tab keeps the per-side rows for when one side needs a hand, each row with the same three verbs (Pull, Push, Sync; origin's Fetch becomes Pull's "check first" step), and drops its own bottom bar. Work's "Send all" goes; its "To send" box links to the header panel. The issue popover keeps "Send now" for that one issue (B-37).

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
Proposal: two: the status menu's last item ("done, and close"), and the comment box's "Close with comment" (GitHub's pattern: closing with a reason). The ✓ and the ⋯ item go.

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
Proposal: icon buttons get visible labels at ≥ 1024 px ("GitHub #17", "Following"); Fit/Fill and Comfortable/Compact move into the ⋯ menu under "Layout"; the ✓ goes (C-36); the avatar beside the tabs (the viewer) goes. The text column widens to 52 rem at "Fill".

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

**D-45. The focus ring is the brand red**, so every focused input reads as invalid.
Proposal: focus is the text color (as the forms already do with `outline: 2px solid var(--fg)`), red is for errors only. One rule in `gitboard.css`.

**D-48. Sync › GitHub Project › Change reflows the whole table.**
Proposal: Change opens the same small form in a popover anchored to the row.

**D-49. A lane's "⋯" is a "move every card to another lane" select.**
Proposal: the ⋯ is a menu (Move every card to…, Hide this lane, Set a limit); the select is its first item.

**D-19. The phone board** (from A-6): one lane a screen, swiped, the lane names as tabs above.

## Order

1. **The cheap, global ones** (one rule each): D-45 focus, D-25 placeholders, D-26 disabled reasons, D-33 labels, C-22, B-10, B-11, B-37, A-40.
2. **One home per thing**: C-16 (the Reference panel goes; unblocks the widths), C-34, C-35, C-36, C-21, C-43, then C-19 (Sync), the largest.
3. **The counts and names**: B-8, B-12, B-15, C-18, B-14, B-9's select.
4. **The pages**: D-23 Labels, D-29 Work, D-30 issue page, D-38 New issue, D-39 filter, D-31 release, D-28 Insights, D-32 frame, D-19 phone board.

Each step lands with its Playwright tests (fail first, collect page errors) and a screenshot pass at 390, 1440 and dark.

## Open questions

1. D-23's `[lint]` key to silence a rule: the one new key proposed. Or a per-field `required = false`?
2. C-18: rename the default view's shown name, or leave "Board"?
3. C-20: drop the Settings overview, or keep it as the page Settings opens on?
4. C-19: is Sync per side (the tab's rows) still needed once the header panel exists, or only behind "Advanced"?
