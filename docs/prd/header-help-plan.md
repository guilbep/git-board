# Header and Help Implementation Plan

> **For agentic workers:** steps use checkbox (`- [ ]`) syntax for tracking. One commit per task; each task leaves the app working.

**Goal:** The decided header (account card, cog, search with `⌘K` and `?`), the keys shown where they are used (palette, foot line), one contextual tip at a time, and filter help that explains filtering (examples, completions, "How filtering works").

**Architecture:** One registry of shortcuts, `web/src/keys.ts`, read by the shortcut list, the palette, the foot line and the tips; the existing handlers stay where they are and call `noteUse`. A `keyContext` ref says where you are. Tips are a pure function (`pickTip`) over the registry, the context and what's retired, plus one component placed by width. Filter help is a pure module (`filterhelp.ts`: the grammar, the examples for this board), one popover component and a combobox replacing the Filter box's `<datalist>`. Web only: no Go change.

**Tech Stack:** Vue 3 + Quasar + TypeScript; `node --test` for pure modules; Playwright e2e (`web/e2e/`).

**Spec:** `docs/prd/header-help.md`. Canvas: https://claude.ai/artifact/Fpo7pBejtJDvPtce4AjQPY ("Decided" row).

## Global Constraints

- No new dependency. EPFL tokens only (no raw hex or px in new CSS; `rem`); 2px radii; no new shadows except the popover's existing one.
- Pure modules (`keys.ts`, `tips.ts`, `filterhelp.ts`) import types relatively and are added to `package.json`'s `test` list.
- `localStorage` reads and writes go through try/catch helpers (as `commands.ts`'s `read`/`write`): a private window lasts until reload.
- Breakpoints: `48rem` (existing), `80rem` (new, the header tip).
- No shortcut changes: same keys, same handlers. A key typed in an input, textarea or contenteditable never triggers one (today's rule, kept).
- e2e test C-34 stays green: one button named "Filter or jump to"… (its accessible name contains it), none named "Filter or jump ⌘K".
- Commits: `feat(header): …`, `feat(keys): …`, `feat(tips): …`, `feat(filters): …`; Co-Authored-By line. Per task: typecheck, `npm test`, the task's e2e file; `make ci` once at the end.

## Review Focus

1. **The Filter box's `↵`** still applies the filter when nothing in the list is highlighted; `Esc` closes the list first, then does what it does today. Pinned in Task 8 (e2e).
2. **The foot line never covers** the settings file's save bar (`ConfigSaveBar`, sticky), the board's picked-cards bar (`.board-picked`, fixed), or the board's last row. Pinned in Task 4 (e2e, Settings › The file with a change pending).
3. **Tips never chatter**: no `aria-live`; with reduced motion, one tip per load. Pinned in Task 5 (unit, and e2e with `reducedMotion: 'reduce'`).
4. **A retired tip stays retired** across reloads; "Hide tips" hides all; Settings brings them back. Pinned in Task 5.
5. **The two `?`s mean two things, everywhere**: the filters' `?`s open "How filtering works", every other `?` (the key, the search field's keycap, the foot line, the palette) the shortcut list. Pinned in Task 7 (e2e).

---

## File Structure

| File | Responsibility |
|---|---|
| `web/src/keys.ts` (new) | the registry: `Shortcut`, `shortcuts`, `keyContext`, `inContext`, `noteUse`, `uses` |
| `web/src/tips.ts` (new) | pure: `pickTip(pool, retired, last)`; the retired and hidden state |
| `web/src/filterhelp.ts` (new) | pure: `GRAMMAR`, `TICKS`, `examples(fields, myLogin)`, `nextTrick(text)` |
| `web/src/components/AppHeader.vue` | cog, account card, search with two keys, the header tip |
| `web/src/components/AccountCard.vue` (new) | the card |
| `web/src/components/KeyLine.vue` (new) | the foot line |
| `web/src/components/TipLine.vue` (new) | a tip: rotation, pause, ×, the two placements |
| `web/src/components/KeyboardHelp.vue` | groups from the registry; open on a group |
| `web/src/components/CommandPalette.vue` | keycaps, panel commands, footer |
| `web/src/components/FilterHelp.vue` (new) | "How filtering works" |
| `web/src/components/FilterSidebar.vue`, `web/src/pages/IndexPage.vue` | the `?`s open FilterHelp; the Filter combobox; `keyContext` |
| `web/src/components/IssuePanel.vue`, `IssueBoard.vue`, `AppHeader.vue` | `noteUse` in the existing handlers |
| `web/src/pages/SettingsPage.vue` | Appearance: tips on/off, reset |
| `web/e2e/header.spec.ts` (new) | the header, keys, tips |
| `web/e2e/filterhelp.spec.ts` (new) | filter help and the combobox |
| `docs/implementation-plan.md` | a status line |

---

### Task 1: land what's done

**Files:** `AppHeader.vue`, `IndexPage.vue`, `css/head.css` (the uncommitted changes in the main checkout).

- [ ] **Step 1:** Move the three changed files onto this branch's worktree (a human: `git stash` in the main checkout, `git stash pop` in the worktree, or a patch).
- [ ] **Step 2:** e2e in `header.spec.ts`: Sync shows `↑1` when one issue waits to go out (the e2e board's fixture, or a route stub of `/api/remotes`); its accessible name says "1 to send"; the search button's name contains "Filter or jump to".
- [ ] **Step 3:** Commit `feat(header): sync drawn as the forge button, search as a field`.

### Task 2: the cog and the account card

**Files:** `AppHeader.vue`, create `AccountCard.vue`; `e2e/header.spec.ts`.

**Interfaces:** `<AccountCard :me />`, emits `close`. Consumes `themePref` (`theme.ts`), `GbAvatar` when `avatars-plan` Task 4 has landed, else the initials span.

- [ ] **Step 1: e2e first:** the nav's last link is named "Settings" and is active (`aria-current="page"`) on `/settings` and `/settings/labels`; the avatar button opens a card with the name, the login, "Your profile" (→ `/settings/you`), "Switch identity" (→ `/settings/you#set-me`), and three Appearance buttons; choosing Dark sets `<html data-theme="dark">`; Esc closes the card and focuses the avatar button; the card has no "Keyboard shortcuts" and no "checked" text.
- [ ] **Step 2:** The cog: a `router-link` at the end of `.epfl-nav`, `GbIcon name="settings"`, `aria-label="Settings"`, `title="Settings"`, active on `route.path.startsWith('/settings')` with the red dot the nav already draws. Remove the menu's Settings and the `v-else` gear.
- [ ] **Step 3:** `AccountCard.vue` as on the canvas: replaces the `<details>` menu (keep its Esc and outside-click closing, `close(menu)`), 18.75rem wide. Appearance: a group of three `aria-pressed` buttons bound to `themePref` (`'light' | 'dark' | 'system'`, labelled Auto). No identity: `IdentityChooser` instead of the who-block.
- [ ] **Step 4:** typecheck, e2e. Commit `feat(header): settings as a cog, the avatar opens your account card`.

### Task 3: the registry, the list and the palette

**Files:** create `keys.ts` (+ `keys.test.ts`); modify `KeyboardHelp.vue`, `CommandPalette.vue`, `IssuePanel.vue`, `IssueBoard.vue`, `AppHeader.vue`, `IndexPage.vue`.

**Interfaces:**
- `type Context = 'any' | 'board' | 'panel' | 'filters' | 'settings-file'`
- `interface Shortcut { id: string; keys: string[]; macKeys?: string[]; what: string; group: 'Board' | 'Issue panel' | 'Filters' | 'Settings › The file'; contexts: Context[]; tip?: string; command?: boolean }` (`command`: offered in the palette)
- `shortcuts: Shortcut[]` (today's `KeyboardHelp` groups, verbatim, with ids: `palette`, `complete`, `new`, `help`, `next`, `prev`, `wide`, `esc`, `send`, `click`, `only`, `range`, `toggle`, `save`)
- `keyContext: Ref<Context[]>` (most specific last); `inContext(ctx): Shortcut[]`; `noteUse(id)`, `uses(): Record<string, number>` (localStorage `gb-keys-used`)

- [ ] **Step 1: Unit tests:** every id unique; `inContext(['board','panel'])` includes `next` and `palette`, not `click`; `any` shortcuts are in every context; `noteUse` counts (with a stubbed storage).
- [ ] **Step 2:** `KeyboardHelp.vue` renders `groups` from the registry (same order, same text: the e2e and screenshots stay); `helpOpen` gains `helpGroup` to scroll to a group.
- [ ] **Step 3:** Each existing handler calls `noteUse(id)` when it acts (`⌘K` and `?` in `AppHeader`, `c` in `IssueBoard`/`IndexPage`, `j`/`k`/`f`/`Esc`/`⌘↵` in `IssuePanel`, Alt/Shift-click and Space in `FilterSidebar`, `⌘S` in the file editor).
- [ ] **Step 4:** The palette: its `hint` for an entry with a shortcut renders keycaps (`<kbd>`), not text; the `panel` commands (Next issue, Previous issue, Wide or side) are offered when a panel is open; a footer row `↑↓` move · `↵` open · `Tab` complete · `?` every shortcut (`?` clickable).
- [ ] **Step 5:** `IndexPage` sets `keyContext`: `['board']`; `['board','panel']` with an issue open; `filters` pushed while the pointer or focus is in the sidebar. Settings › The file sets `['settings-file']`.
- [ ] **Step 6:** `npm test`, typecheck, the existing e2e. Commit `feat(keys): one list of shortcuts, shown in the palette`.

### Task 4: the search's `?` and the foot line

**Files:** `AppHeader.vue`, create `KeyLine.vue`, mount it in `App.vue` (or the layout every page uses); `e2e/header.spec.ts`.

- [ ] **Step 1: e2e:** the search group has two buttons, "Search: filter or jump to" and "Keyboard shortcuts"; the second opens the list. The foot line on the board lists `⌘K`/Ctrl K, `c`, `?`; with an issue open, `j`, `k`, `f`, `Esc`, `?`; at 375px wide it is hidden; on Settings › The file with an unsaved change, the save bar is fully visible and clickable (`toBeInViewport()` and a click).
- [ ] **Step 2:** The search becomes `<div class="gb-search" role="group">` holding the search button (icon, "Search", `⌘K` keycap) and a 1.75rem `?` keycap button; same width, same look as now.
- [ ] **Step 3:** `KeyLine.vue`: `inContext(keyContext)` with `contexts` not `any`, last four, then `?`. Fixed to the bottom, `--bg-subtle`, top border; the page reserves its height (`padding-bottom` on the layout). Hidden under 48rem, and when a bottom bar is shown: `ConfigSaveBar` and `IssueBoard`'s `.board-picked` set a shared `bottomBarShown` ref (`commands.ts`) while visible.
- [ ] **Step 4:** typecheck, e2e. Commit `feat(keys): ? beside ⌘K, and a key line at the foot`.

### Task 5: tips

**Files:** create `tips.ts` (+ `tips.test.ts`), `TipLine.vue`; modify `AppHeader.vue` (placement), `SettingsPage.vue`; `e2e/header.spec.ts`.

**Interfaces:** `pickTip(pool: Shortcut[], retired: Set<string>, last?: string): Shortcut | null` (the next after `last` in pool order, skipping retired; `null` when none); `retired(): Set<string>` (used ≥ 3, or dismissed: `gb-tips-dismissed`); `tipsOff: Ref<boolean>` (`gb-tips-off`); `resetTips()`.

- [ ] **Step 1: Unit tests:** rotation order and wrap; a retired one skipped; all retired → `null`; a shortcut used 3 times is retired, 2 isn't; `resetTips` clears both.
- [ ] **Step 2:** `TipLine.vue`: `<p role="note">` "Tip", the keycaps, the sentence, four dots (position in the pool, `aria-hidden`), × ("Hide this tip"), and a "Hide tips" in its `title`/menu; 12 s timer, paused on `mouseenter`/`focusin`; a click on the text opens the list on the tip's group; 200ms opacity fade; `matchMedia('(prefers-reduced-motion: reduce)')`: no fade, no timer.
- [ ] **Step 3:** Placement: in `AppHeader`, a `ResizeObserver` on the header's free space between the nav and `.app-right`: the tip shows there when it fits its width and the viewport is ≥ 80rem; otherwise a 1.875rem strip under the header (≥ 48rem); none below. One `TipLine` instance at a time.
- [ ] **Step 4:** Settings › You, Appearance: "Tips" on/off (`tipsOff`) and "Show every tip again" (`resetTips`).
- [ ] **Step 5: e2e:** at 1440px a tip is in the header and has no `aria-live`; at 1100px it's in the strip; at 375px none; × on a tip, reload: that tip doesn't come back; "Hide tips": none; Settings turns them on again; with `reducedMotion: 'reduce'` the tip text doesn't change after 13 s (`page.clock`).
- [ ] **Step 6:** Commit `feat(tips): one shortcut at a time, for where you are`.

### Task 6: `filterhelp.ts`

**Files:** create `web/src/filterhelp.ts` (+ `filterhelp.test.ts`).

**Interfaces:** `GRAMMAR: { term: string; means: string; example: string }[]` (from `internal/filter/filter.go`'s package comment: field:value, a,b, -field:, no:field, @me, @current/@next/@previous, involves:, follows:@me, is:, title:*glob*, #2528 and its project number, "phrases"); `TICKS` (click, ⌥/Alt-click, Shift-click, Space); `examples(fields, myLogin): { term: string; means: string }[]` (≤ 5, only terms this board can answer: a person field and a login for `@me`; a status-like field for `no:`; a sprint field for `@current`); `nextTrick(text): string | null`.

- [ ] **Step 1: Unit tests:** `examples` with no person field omits `assignee:@me`; with no login omits every `@me`; never more than 5; uses the board's own field names (`lead-dev:@me` on a board whose person field is `lead-dev`). `nextTrick('status:r')` mentions `-status:` and `a,b`; `nextTrick('')` is null.
- [ ] **Step 2:** A Go-side guard against drift: a comment in `filter.go`'s package doc pointing at `filterhelp.ts` ("change one, change both").
- [ ] **Step 3:** Commit `feat(filters): the filter words, and examples for this board`.

### Task 7: "How filtering works"

**Files:** create `FilterHelp.vue`; modify `FilterSidebar.vue`, `IndexPage.vue` (rail and Filter box `?`); create `e2e/filterhelp.spec.ts`.

- [ ] **Step 1: e2e first:** the panel's `?` opens a dialog-like popover named "How filtering works" (not "Keyboard shortcuts"); its Words tab lists `-status:` and `no:`; Try on `is:closed` puts `is:closed` in the Filter box, focuses it, closes the popover; the rail's `?` (filters folded) and the Filter box's `?` open the same popover; the Ticks tab names ⌥ on a Mac user agent, Alt otherwise.
- [ ] **Step 2:** `FilterHelp.vue` on `GbPopover` (Esc, outside click, focus return), two tabs (`role="tablist"`), Words in two columns (one under 48rem), Try buttons emitting `try(term)`. Footer: AND'ed, `Tab` completes, `?` every shortcut (opens the list), a link to `docs/guide/configuration.md`'s filter section as served by the guide.
- [ ] **Step 3:** The three `?`s: `FilterSidebar`'s `filters-help` and the rail's (`IndexPage`) open it, replacing `helpOpen = true`; a `?` button inside the Filter box's right end.
- [ ] **Step 4:** Commit `feat(filters): the filters' ? explains filtering`.

### Task 8: the Filter box's examples and completions

**Files:** `IndexPage.vue` (the `<datalist id="filter-suggest">` and its input), possibly a `FilterBox.vue` extracted from it; `e2e/filterhelp.spec.ts`.

- [ ] **Step 1: e2e first:** focusing the empty box opens a listbox "Try" with ≤ 5 options; `↓` then `↵` puts the first example in the box; typing `status:r` lists `status:ready` with a count; `Tab` completes it; with nothing highlighted, `↵` applies the typed filter (the URL/board updates as today); `Esc` closes the list, a second `Esc` does today's thing; the listbox's "How filtering works" opens the popover.
- [ ] **Step 2:** The combobox: `role="combobox"`, `aria-expanded`, `aria-controls`, `aria-activedescendant`; options from `examples(...)` when empty, else `suggestions` with counts from the facets (`facetList`'s `[value, count]`); footer from `nextTrick`. Remove the `<datalist>`.
- [ ] **Step 3:** typecheck, both e2e files. Commit `feat(filters): examples and completions in the Filter box`.

### Task 9: docs and CI

- [ ] **Step 1:** `docs/implementation-plan.md`: a dated status line; the canvas link.
- [ ] **Step 2:** Screenshots, if the design docs keep them (`docs/design/`): the header wide and narrow.
- [ ] **Step 3:** `make ci`, once. Commit `docs(header): status`.

---

## Order and dependencies

Tasks 1–2 stand alone. 3 before 4 and 5 (the registry). 6 before 7 and 8. The avatar picture in the card follows `avatars-plan.md` Task 4 whenever it lands; the card works with initials until then.
