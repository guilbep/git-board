# Header and help: the account card, keys you can see, filter help, tips

Status: implemented 2026-11-02 (go given), not yet committed. Plan: `header-help-plan.md`.
Canvas (explorations, then the "Decided" row):
https://claude.ai/artifact/Fpo7pBejtJDvPtce4AjQPY
Depends on: `avatars.md` for the picture in the avatar button and the
card (initials until it lands).

## Why

- The avatar opens a menu of four unrelated things: you ("You", in red,
  reads as an error), Settings, the app's keyboard shortcuts, and Sync's
  last check ("Not checked yet").
- `?` opens every shortcut, but nothing on screen says so. `⌘K` is now
  shown in the search field; `?` isn't shown anywhere.
- The filters panel's `?` opens the app's shortcut list, not help on
  filtering. The hard half of filtering, the words in the Filter box
  (`-status:done`, `no:assignee`, `@me`…), is explained nowhere in the
  page.
- Shortcuts are learned by being told once and seeing them again. The
  list behind `?` is read once, if at all.

## Already done (uncommitted in the main checkout, lands as Task 1)

- Sync in the header is drawn as an issue's forge button (16e): the sync
  icon, a yellow up-count to send, a blue down-count to get, a blue badge
  while syncing, a red one for conflicts. `.gb-fbadge.is-conflict` was
  missing in `head.css` (conflicts showed grey on issues too): fixed.
- The header's search is a field (Zensical's): icon, "Search", `⌘K`.
- The folded filters rail keeps a `?`.

## Decisions

| Area | Decision (canvas) |
|---|---|
| Avatar | opens an **account card** (A2) without Settings: picture, name, login; "Your profile" (Settings › You); "Switch identity"; Appearance (Light / Dark / Auto). No shortcuts, no Sync state in it |
| Settings | a **cog at the end of the main nav**, after Inbox (A3, the text replaced by the icon); `aria-label="Settings"`; the red dot when on any `/settings` page |
| Search | the field shows **two keys, `⌘K` and `?`** (C2); `?` is its own button at the end of the field, opening the shortcut list |
| Palette | **every command shows its key**; issue-panel commands (next, previous, wide) are commands too; a footer: `↑↓` move · `↵` open · `Tab` complete · `?` every shortcut (C3) |
| Foot line | a **thin line at the bottom of the window** with the keys for where you are: the board, the issue panel, the filters, a settings file (C5) |
| Tips | **one tip at a time, chosen for the screen you're on** (D3), never a shortcut you already use; in the **header from 80rem (1280px)** (D1), in a **30px strip under the header from 48rem to 80rem** (D2), none below |
| Filter help | the Filter box offers **examples on focus** (B3), **completions as you type** with counts, and a `?` at its end; the panel's `?` and the rail's `?` open the same **"How filtering works"** popover: Ticks and Words tabs, words in two columns, each with Try (B1 + B2). Neither `?` opens the app's shortcut list any more |

## 1. One list of shortcuts

Today the shortcuts are written three times: the handlers
(`AppHeader.vue`, `IssuePanel.vue`, `IssueBoard.vue`, `IndexPage.vue`),
the list (`KeyboardHelp.vue`'s `groups`), and the palette's hints. The
list, the foot line, the palette and the tips all read **one registry**
(`web/src/keys.ts`): each shortcut's keys (Mac and others), what it does,
the contexts it works in, and its tip sentence. The handlers stay where
they are; each one records a use (`noteUse(id)`).

**Contexts:** `board`, `panel` (an issue open beside or over the board),
`filters` (the pointer or the focus in the filters panel), `settings-file`
(Settings › The file), `any`. The page sets the current context; the
most specific wins (`filters` over `panel` over `board`).

## 2. The account card

- The avatar button (44px square, the picture or the initials) opens a
  card under it, 300px, bordered, the popover shadow. Esc and a click
  outside close it; focus goes back to the button.
- **Who:** picture (44px), name, login (mono, muted). No identity yet:
  "Choose who you write as" (the existing `IdentityChooser`) in its place.
- **Your profile** → Settings › You. **Switch identity** → Settings › You,
  on "Who you write as" (`#set-me`); inline switching is later.
- **Appearance:** Light · Dark · Auto, the same `themePref` Settings › You
  sets (`theme.ts`), applied at once.
- The Sync check time leaves the card: it is in Sync's tooltip already.

## 3. The keys you can see

- **Search field:** `⌘K` (Ctrl K off a Mac) and `?` as keycaps. The field
  is a group of two buttons: the search (most of the width, accessible
  name "Search: filter or jump to") and the `?` keycap (its own 28px
  button, "Keyboard shortcuts"). Under 48rem the field folds to the
  search icon; `?` still works from the keyboard.
- **Palette:** each entry with a shortcut shows it right-aligned as
  keycaps (today: plain text hints for two of them). The footer row is
  always there.
- **Foot line:** right-aligned, 12.5px, muted, at most five keys, the
  last always `?` every shortcut. Hidden under 48rem, and while another
  bar holds the bottom of the window (the settings file's save bar, `ConfigSaveBar`; the board's
  picked-cards bar, `.board-picked`): it never covers one.

## 4. Tips

- **Pool:** the registry's shortcuts that work in the current context,
  minus the **retired** ones: used 3 times (counted per browser, in
  `localStorage`), or the tip's own × (one tip), or "Hide tips" (all).
- **Rotation:** one tip, 12 s, then the next in the pool; hover or focus
  pauses it; a click opens the shortcut list on that shortcut's group.
  The pool empty: nothing shows, no placeholder.
- **Place:** header from 80rem, strip from 48rem to 80rem, none below.
  The header's tip never squeezes the nav: when it doesn't fit beside
  it, it goes to the strip.
- **Motion and screen readers:** a 200ms fade; with
  `prefers-reduced-motion`, no fade and **no rotation** (one tip per page
  load). `role="note"`, never `aria-live`: a screen reader doesn't hear
  each change. Pausing on hover and the × meet WCAG 2.2.2
  (pause, stop, hide).
- **Off for good:** "Hide tips" in the tip and in Settings › You
  (Appearance), stored per browser; Settings can turn them back on and
  reset the retired ones.

## 5. Filter help

- **Focus, box empty:** a listbox under the box, "Try", up to five
  examples **from this board**: `assignee:@me` (a person field and your
  login), `is:closed`, `no:<the status field>`, `sprint:@current` (a
  sprint field), `involves:@me`; each with a few words. Footer: `↑↓`
  choose · `Tab` complete · "How filtering works".
- **Typing:** the listbox becomes completions (`suggest`, as now), each
  with its count from the facets; its footer teaches the next trick for
  the word being typed (`-status:` for "not", `a,b` for either).
- The listbox replaces the `<datalist>`: the combobox pattern
  (`aria-expanded`, `aria-activedescendant`), `↑↓` move, `Tab` and `↵`
  take the highlighted value, `↵` with nothing highlighted applies the
  filter as today, `Esc` closes the list (a second `Esc` behaves as
  today).
- **"How filtering works":** a popover from the panel's `?`, the rail's
  `?`, the `?` at the end of the Filter box, and the listbox's footer
  link. Ticks tab: click, `⌥`-click, Shift-click, Space. Words tab: the
  grammar in two columns (`internal/filter/filter.go`'s list), each with
  Try (writes the example into the box, focuses it). Footer: words are
  AND'ed; `Tab` completes; every shortcut `?`; the guide's link.

## Out of scope

- Switching identity inside the card (it links to Settings › You).
- Shortcuts that don't exist yet (no new keys in this work).
- Tips about anything but shortcuts and the filter words.
- The avatars themselves (`avatars.md`).

## Open questions

1. Narrow screens (48–80rem) carry two bands: the tip strip under the
   header and the foot line. If that is one too many, the tip can sit at
   the left end of the foot line instead. This draft keeps the strip, as
   decided; to look at once built.
2. Tips' retirement is per browser (`localStorage`), not per person: a
   second computer shows them again. Acceptable for tips; say so if not.
