# Time flow M2: sections and threads, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** The description renders as sections, each with a gutter (its
anchor "§n Heading" and a chip); a comment is anchored from a gutter; a
thread shows under its section or in the flow (*Threads: under their
section / in the flow*), with replies and *Resolve*; a suggested edit shows
as a two-line diff with *Apply in v*N+1 / *Decline* for the body's author.
Screens 6a and 6f.

**Architecture:** One new pure module, `web/src/sections.ts`, holds the
body's rules: the sections of the rendered HTML and of the Markdown
source, the slugs, the anchor grammar, the version count, the suggestion
block and its application. `flow.ts` learns that a body thread is a thread
(`thread:anchor`, `thread:reply-to`, `thread:resolved`) beside a review
thread, and `Reading` gains `where`. Two new components: `IssueSections.vue`
(the sections, their gutters, the composer under a section) and
`ThreadCard.vue` (a thread's rows, its reply row, *Resolve*). `CommentCard`
gains the role chip, the anchor chip, the version in the description's
header and the suggestion block; `IssueFlow` the pointer lines, the
*Threads* seg and body threads in the flow. The server accepts `anchor`
and `reply_to` on a comment, writes `thread:kind` when the body carries a
```suggestion block, and gains one route that writes `thread:resolved` and
`thread:applied` on a root through the store's `SetCommentMetadata`, which
exists. The core is untouched.

**Tech Stack:** Vue 3, `node:test`; Go (`net/http`, the store).

**Spec:** `docs/prd/time-flow.md`, "Decisions", "The protocol", "Calls
taken", "M2: sections and threads"; `docs/design/time-flow/README.md`, 6a,
"Components", "States and interactions", "Responsive behaviour", "Edge
cases", "Accessibility"; `docs/design/time-flow/protocol.md`, the
`thread:*` keys, the anchor grammar, "Versions and ticks", the 6f example;
`screens/F1-spec-v1.html` and `screens/F6-protocol.html` for the markup
and the copy. Where a screen and the PRD disagree, the screen's structure
wins; the README's rule wins over a screen's sample data.

**Base:** main at `394f434`, branch `feat/time-flow-m2`. M1 is on main
(`flow.ts`, `IssueFlow.vue`, `IssuePanel.vue`, `IssueStep.vue`); its
handover doc (rulings, fixture recipe) is the Claude Docs doc
https://claude.ai/artifact/AwbmCxZ5Qxh7AWS8j7VZ4W. This plan goes first,
at `docs/prd/time-flow-m2-plan.md` (Task 0), as the branch's first commit:
no pull request of its own, it lands with the code (ruling 1).

## What exists today, found in the code

**How a comment's meta is written after it is posted.** The core has
`issue.SetMeta(author, target, meta)` (`core/issue/ops.go:39`), a
`set-meta` operation that `thread()` replays by merging `meta` into the
target event's `Meta` (`internal/store/thread.go:166-174`), so every key
reaches the client as `IssueEvent.meta` with no store change. The store
has `Backend.SetCommentMetadata(idPrefix, commentID, metadata)`
(`internal/store/review.go:60`): generic, any keys, on a comment or the
body, through `text()`. What is missing is above the store: the only
route that calls it is `POST /api/issues/{id}/review/{comment}/resolve`
(`internal/server/review.go:301`), which refuses a root that is not a
review line comment and writes `review:resolved` only; and
`handleComment` (`internal/server/issues.go:132`) takes `body`, `status`,
`step` and writes `cycle:step` only. The client has no meta call beyond
`resolveReview`. Nothing exposes an `edit-text` operation's id: the
description's `history` is `[{author, at, body}]`, oldest first
(`thread.go:163`), while `thread:applied` wants the edit's id. Task 3 adds
`ID` to `Version`, the `anchor` and `reply_to` fields, and the meta route.

**How body threads join M1's fold and the Open threads count.** `fold`
(`flow.ts:146`) folds review threads only: a root is `e.review?.path &&
!e.review.reply_to`, a reply joins by `e.review.reply_to`, resolved is
`e.review.resolved`. `pick`'s Open set (`flow.ts:132`) is
`threadsOf(events)` (`review.ts`) minus the resolved ones. Task 2
generalizes both with one notion of root, reply and resolved that reads
`review:*` or `thread:*`: a body root is a comment with
`meta['thread:anchor']` and no `meta['thread:reply-to']`, a reply joins
by `meta['thread:reply-to']`, resolved is a non-empty
`meta['thread:resolved']`. `reviewLine` (`review.ts`, step 6's cell) stays
review-only; the sidebar's open-threads count is M3's Spec block.

**The role chip.** `[roles]` reaches the client as person fields whose
`inherits` is the role's login (`internal/server/server.go:327`,
`board.ts:54`); a person's roles are the fields that inherit their login.
Nothing new on the server.

**Fenced code.** goldmark renders a ```suggestion block as `<pre><code
class="language-suggestion">`; bluemonday keeps that class
(`internal/markdown/markdown.go:33`). Headings get no `id`.

## Global Constraints

- No new entity or operation kind; the core gives no key a meaning. M2
  writes `cycle:step`, `thread:anchor`, `thread:reply-to`, `thread:kind`
  (on a `comment`) and `thread:resolved`, `thread:applied` (on a root, by
  `set-meta`), with the protocol's values. Versions and thread states are
  computed, never stored. The step is never typed. `cycle:step` stays as
  it is.
- GitHub stays readable without git-board: a thread comment's body is
  plain Markdown (the quote header and the marker are M5's); `review:*`
  comments stay local.
- History's scrubber, lanes and snapshot keep today's behaviour.
- The copy is the screens' (F1, F6) with Person A, B, C in fixtures and
  tests; logins alice, bob, carol. EPFL tokens (`var(--…)`), not hex:
  teal for anchors and open chips, `--gb-done` as M1's open chip.
- Layout: a section is a grid `minmax(0, 1fr) 164px`, gap `10px 20px`,
  padding 16px 0, a hairline top border; the thread spans both columns;
  the gutter chip is 12px, 4px 8px, dashed when empty, teal when open,
  grey when resolved; the diff block is mono 13px, the struck line on
  `--del`, the added one on `--add`. Under 720 px a section is one
  column, its gutter row under its content.
- Accessibility: gutter chips are `<button aria-expanded>`, the Threads
  seg `<button aria-pressed>`, a `<label>` on the reply input. Focus
  order: the description's gutter chips in reading order, then an
  unfolded thread's reply row, then the flow.
- No Vue component tests exist: every component task is checked in a
  browser on a throwaway board in `$TMPDIR` (Task 10's recipe).
- While developing: `cd web && npx tsx --test src/sections.test.ts
  src/flow.test.ts`, `go test ./internal/server/ -run TestThread`; `cd
  web && npm run typecheck && npm test` before each commit (lefthook
  runs the typecheck). `make ci` once, at the end, through `rtk proxy
  make ci` into a log. A new Go test calls `t.Parallel()` first.
- Commit on `feat/time-flow-m2`, never push `main`, never tag. No
  attribution trailer. No new dependency. Deliberate shortcuts carry a
  `ponytail:` comment.

## Review Focus

1. The gutter's sections come from the HTML, Apply's lines from the
   Markdown: a `#` line inside a fence, a heading in a blockquote, a
   setext heading, a heading with a link or an `&` must give both the same
   sections and slugs, or Apply edits the wrong lines. One slug rule (the
   HTML's), Markdown headings paired by order, Apply refused when the
   counts differ. (Task 1: `a fence is not a heading`, `a heading's slug
   is its rendered text`, `a setext heading opens a section`, `a quoted
   heading opens none`, `mismatched headings refuse`.)
2. A thread whose slug matches no section of the current body (the
   heading was renamed after the comment): it must still show, in the
   flow, its anchor as the slug; carried / outdated are M3's. (Task 1:
   `an unknown slug keeps its name`; Task 2: `pick keeps a thread with
   no section`.)
3. A reply whose root the filters hide (*Mine* on, the root by someone
   else): it stays as an entry of its own, as M1's ruling 1 says for
   review replies. (Task 2: `a body reply without its root stays`.)
4. Apply while someone changed the description meanwhile: the edit is
   refused with 409 `changed`, nothing is written on the root, the
   thread stays open and the error shows. The guard is `apply()` in
   `issue.ts` stopping when the edit fails (Task 4); pinned by Task 8's
   two-tab browser step.
5. A person resolves, then unresolves: `thread:resolved` set to `""`
   reopens the thread and the Open threads count follows. (Task 2:
   `an emptied thread:resolved is open`; Task 3: `the meta route takes
   an empty value`.)

---

### Task 0: the plan in the repository

**Files:**
- Create: `docs/prd/time-flow-m2-plan.md` (this file, verbatim)

- [ ] **Step 1:** Copy this plan to `docs/prd/time-flow-m2-plan.md`.
- [ ] **Step 2:** Commit `docs(prd): the time flow's M2 plan` on the
  branch. No pull request of its own (ruling 1).

### Task 1: the body's rules

**Files:**
- Create: `web/src/sections.ts`
- Test: `web/src/sections.test.ts` (add it to `web/package.json`'s
  `test` list)

**Interfaces:**
- Consumes: `IssueEvent` (`api/board.ts`).
- Produces:

```ts
export interface Section { n: number; slug: string; heading: string; html: string }
// the rendered body cut at its h1–h6 outside any <blockquote>: the prose
// before the first heading is §1 "intro" (its heading ""); no heading:
// one section, intro, the whole text. The slugs are computed here, once,
// from the heading's text with its tags stripped and its entities
// decoded ("A &amp; B" → "a-b"); nothing else computes a slug
export function sectionsOf(html: string): Section[]
// lower case, runs of other characters as one "-", an index on a repeat:
// ["Expected", "Expected", "Releases page"] → ["expected", "expected-2", "releases-page"]
export function slugs(headings: string[]): string[]

export interface Anchor { text: 'body'; v: number; slug: string; item?: number }
export function anchorOf(s: string | undefined): Anchor | undefined   // body@v1/expected#3
export function anchorString(a: Anchor): string
// "§3 › 3", "§3"; a slug no section has: "§ expected" (Review Focus 2)
export function anchorLabel(a: Anchor, sections: Section[]): string

// the body's version: 1 + the edits in `history` that are not ticks
// (protocol, "Versions and ticks"); a text with no history is v1
export function version(created: IssueEvent): number
export function isTick(before: string, after: string): boolean // differs only by "- [ ]" ↔ "- [x]"

// the Markdown source's line spans of the same sections: its heading
// lines (ATX "# …" and setext "…" over "===" / "---", outside fences and
// "> " lines) paired with `sections`' headings by order, each span taking
// its slug from the paired Section. undefined when the two heading counts
// differ: then nothing is applied (Review Focus 1)
export interface MdSection { slug: string; start: number; end: number } // lines, end exclusive
export function mdSections(md: string, sections: Section[]): MdSection[] | undefined
// the anchored line: item n's text (marker stripped) in the section, or
// the section's heading line; undefined when the anchor names nothing
export function lineAt(md: string, sections: Section[], a: Anchor): string | undefined
// the ```suggestion block's text, undefined without one
export function suggestionOf(body: string): string | undefined
// the body with the anchored item's line replaced (its "- " / "1. "
// marker kept), or, with no item, the section's lines after its heading;
// undefined when the anchor names nothing or mdSections refuses (Apply
// refuses)
export function applySuggestion(md: string, sections: Section[], a: Anchor, replacement: string): string | undefined
```

`sectionsOf` works on the HTML string the server sends: split at
`/<h([1-6])(?:\s[^>]*)?>/` tags while no `<blockquote` is open (a depth
counter over the open and close tags). `mdSections` walks lines, toggling
a fence on "```"/"~~~" lines. Items are the section's `- `, `* `, `+ ` and
`n. ` lines at any indent, counted in order (ponytail: a nested item
counts; a sub-list picker is not in M2).

- [ ] **Step 1:** Write `sections.test.ts`: `no heading is one section`
  (`sectionsOf('<p>a</p><p>b</p>')` → one, n 1, slug `intro`);
  `each heading opens a section` (intro then §2 `releases-page`, §3
  `expected`, §4 `workaround` from the 6a body); `a repeated heading gets
  an index` (`slugs` → `expected`, `expected-2`); `anchorOf parses the
  grammar` (`body@v1/expected#3` → `{text:'body', v:1, slug:'expected',
  item:3}`, `body@v2/intro` without item, `plan@a1f0/steps#4` and
  `body@v0/x` → undefined); `anchorLabel` ("§3 › 3", "§3", "§ expected"
  for an unknown slug); `a tick is not a version` (`history` of three
  bodies where the second only flips `- [ ]` to `- [x]` → 2);
  `a fence is not a heading` (`mdSections` over a body whose fence holds
  "# not one" → sections unchanged); `a heading's slug is its rendered
  text` (`## [Releases](https://x) page` → `releases-page`, `## A & B` →
  `a-b`, in `sectionsOf` and in `mdSections`' paired spans alike);
  `a setext heading opens a section` ("Expected" over "---": the items
  under it count in Expected, not the section above); `a quoted heading
  opens none` (`> # Note` in either form); `mismatched headings refuse`
  (a heading count that differs → `mdSections` undefined,
  `applySuggestion` undefined); `lineAt` (item 3 of Expected in 6a's
  body); `suggestionOf` (6f's comment → its one line); `applySuggestion
  replaces the item and keeps its marker`; `applySuggestion without an
  item replaces the section`; `applySuggestion on a gone section is
  undefined`.
- [ ] **Step 2:** Run `cd web && npx tsx --test src/sections.test.ts`.
  Expected: FAIL, module not found.
- [ ] **Step 3:** Write `sections.ts` with the signatures above.
- [ ] **Step 4:** Run the test. Expected: PASS. Add it to `package.json`.
- [ ] **Step 5:** Commit `feat(issue): the body's sections, anchors and
  versions`.

### Task 2: body threads in the flow's rules

**Files:**
- Modify: `web/src/flow.ts` (`Reading`, `READING`, `ONE_OF`, `pick`,
  `fold`)
- Test: `web/src/flow.test.ts`

**Interfaces:**
- Consumes: `anchorOf` (Task 1), `threadsOf` (`review.ts`).
- Produces:

```ts
// Reading gains: where the threads show (6a's seg)
where: 'section' | 'flow'   // READING.where = 'section'
// a thread's root, in either protocol; a plain comment has none
export const rootOf = (e: IssueEvent): string | undefined  // review.reply_to ?? meta['thread:reply-to']
export const isRoot = (e: IssueEvent): boolean             // a review line root, or thread:anchor without thread:reply-to
export const isResolved = (e: IssueEvent): boolean         // review.resolved, or a non-empty thread:resolved
// the body threads, roots with their replies, in time order
export function bodyThreads(events: IssueEvent[]): Item[]
// the role a person holds in the project: the first person field that inherits their login, else undefined
export function roleOf(fields: Fields, login: string | undefined): string | undefined
```

`fold` keeps its shape: roots are `isRoot`, a reply joins by `rootOf`,
`resolved` is `isResolved`. `pick`'s Open set is every entry of an
unresolved thread, review or body (`[...threadsOf(events)…,
...bodyThreads(events)]`). `readingOf` validates `where` through
`ONE_OF`.

- [ ] **Step 1:** Add to `flow.test.ts`: `a body reply folds under its
  root` (`c7` with `thread:anchor` `body@v1/expected#3`, `c8` with
  `thread:reply-to: 'c7'` → one item, one reply); `a body reply without
  its root stays` (`c8` alone → an item of its own); `Open threads
  counts a body thread` (`show: 'open'` keeps c7 and c8, drops c6);
  `a resolved body thread is not open` (`thread:resolved: 'v2'` on c7 →
  dropped); `an emptied thread:resolved is open` (`''` → kept); `pick
  keeps a thread with no section` (c7's slug names nothing: still
  kept); `readingOf takes where` (`'flow'` kept, `'side'` dropped).
- [ ] **Step 2:** Run `cd web && npx tsx --test src/flow.test.ts`.
  Expected: FAIL on the new cases.
- [ ] **Step 3:** Implement. `roleOf` is one line over
  `Object.entries(fields)`.
- [ ] **Step 4:** Run the test. Expected: PASS. `npm run typecheck`
  passes (`IssueFlow`'s reads of `fold` are unchanged).
- [ ] **Step 5:** Commit `feat(issue): body threads fold and count as
  review threads do`.

### Task 3: the server writes the thread keys

**Files:**
- Modify: `internal/server/issues.go` (`handleComment`), new
  `handleThreadMeta` beside `handleResolve` in
  `internal/server/review.go`; `internal/server/server.go:158-163` (the
  route); `internal/store/thread.go` (`Version.ID`)
- Test: `internal/server/issues_test.go`, `internal/store/thread_test.go`

**Interfaces:**
- Produces, on `POST /api/issues/{id}/comments`: two optional fields,
  `anchor` (the grammar, validated by
  `^body@v[1-9][0-9]*/[^/#\s]+(#[1-9][0-9]*)?$`) and `reply_to` (a
  comment id). `anchor` is written as `thread:anchor`; `reply_to` as
  `thread:reply-to`, and the root's `thread:anchor` copied with it, as
  `handleReview` copies a reply's anchor; `reply_to` must name a comment
  with `thread:anchor` and no `thread:reply-to`, else 400 "reply to a
  thread's first comment". A body with a line "```suggestion" gets
  `thread:kind` = `suggestion` (ponytail: the server reads the fence so
  no client can forget the key). `cycle:step` as today, in the same map.
- Produces `POST /api/issues/{id}/comments/{comment}/meta` with a JSON
  object of keys: only `thread:resolved` and `thread:applied` are
  accepted (400 otherwise), on a comment that is a body root (400
  otherwise), through `SetCommentMetadata`; answers the thread. `""` is
  a value.
- Produces `Version.ID string` (`json:"id"`): the `edit-text` entry's id
  on each history version after the first (the original's is the text's
  own id). `thread()` has the entry at hand where it appends.
- Store helper: `CommentMeta(idPrefix, commentID) (map[string]string,
  error)` beside `CommentReview`, for the checks above.

- [ ] **Step 1:** Write `TestThreadComments` (parallel) in
  `issues_test.go`, after `TestCommentOnAStep`'s style: a comment with
  `anchor` `body@v1/expected#3` and `step` 1 has both keys; a reply with
  `reply_to` has `thread:reply-to` and the copied anchor; a body with a
  ```suggestion fence has `thread:kind`; `anchor` `plan@x/y` → 400;
  `reply_to` the reply's id → 400. Write `TestThreadMeta` (parallel):
  the meta route on the root with `{"thread:resolved": "v2",
  "thread:applied": "<the edit id>"}` → both on the root's `Meta`;
  `{"thread:resolved": ""}` → accepted, the key empty; `{"cycle:step":
  "2"}` → 400; the route on a plain comment → 400; `Apply leaves two
  operations`: the issue's op log (the `l` that `b.issue(id)` returns,
  through a store test helper if the server test cannot reach it) grows
  by exactly two across the body's `PUT …/comments/{body}` and the meta
  call, an `edit-text` then a `set-meta` whose target is the root. Write `TestHistoryHasEditIDs` (parallel) in the
  store's test: after `EditComment` on the body, `History[1].ID` is a
  40-hex id with `:n`, not the body's.
- [ ] **Step 2:** Run `go test ./internal/server/ ./internal/store/ -run
  'TestThread|TestHistoryHasEditIDs'`. Expected: FAIL.
- [ ] **Step 3:** Implement: the two fields and the kind in
  `handleComment` (one meta map, nil when empty), `handleThreadMeta`,
  the route, `CommentMeta`, `Version.ID`.
- [ ] **Step 4:** Run the tests. Expected: PASS. `go test
  ./internal/server/ ./internal/store/` green.
- [ ] **Step 5:** Commit `feat(server): comments carry an anchor and a
  reply, a root takes thread:resolved and thread:applied`.

### Task 4: the client's calls

**Files:**
- Modify: `web/src/api/board.ts` (`addComment`, new `threadMeta`,
  `Version.id`), `web/src/issue.ts` (`comment`, new `resolveThread`,
  `apply`)

**Interfaces:**
- Produces:

```ts
// board.ts
export interface ThreadOn { anchor?: string; reply_to?: string }
export function addComment(id: string, body: string, status?: string, step = 0, on?: ThreadOn): Promise<Thread>
export function threadMeta(id: string, commentId: string, meta: Record<string, string>): Promise<Thread>
// Version gains id?: string
// issue.ts, in useIssue's return
comment(body: string, status?: 'open' | 'closed', step = 0, on?: ThreadOn): Promise<boolean>
resolveThread(root: string, value: string): Promise<boolean>   // "" reopens; a person's id, or "v2"
// Apply: the edit, then the root's keys with the new version's id from
// the created event's last history entry (Task 3). Not atomic: a failed
// second call leaves the edit and an open thread the person can resolve
// by hand (ponytail: one server call if that bites)
apply(root: string, body: string, base: string, version: number): Promise<boolean>
```

`apply` goes through `write()` as `edit` does, rethrowing `changed`
(409) for the card to show. It calls `threadMeta` only once the edit has
succeeded: a refused edit writes nothing on the root (Review Focus 4).

- [ ] **Step 1:** Implement the three functions and the field.
- [ ] **Step 2:** Run `cd web && npm run typecheck && npm test`.
  Expected: PASS.
- [ ] **Step 3:** Commit `feat(issue): the client posts anchored
  comments, replies, resolves and applies`.

### Task 5: the description as sections

**Files:**
- Create: `web/src/components/IssueSections.vue`
- Modify: `web/src/components/CommentCard.vue` (a default slot for the
  body; the created verb and version; the role chip),
  `web/src/components/IssuePanel.vue` (`#intent`)

**Interfaces:**
- Consumes: `sectionsOf`, `version` (Task 1); `bodyThreads`, `roleOf`
  (Task 2).
- Produces `IssueSections`: props `{ html: string; events: IssueEvent[];
  meId: string; fields: Fields; forges?: Forge[] }`; emits
  `comment(anchor: string, body: string)` (Task 6), exposes
  `unfold(rootId: string)` (Task 7). One `.gb-sec` grid per section: the
  content (`MarkdownBody :html="s.html"`), the gutter with
  `<span class="gb-anchor">§n Heading</span>` ("§1 intro" for the intro)
  and the chip: no thread → `<button class="gb-tchip is-ghost">Comment on
  §n</button>`; threads → `n comments · open ▸` (`is-open`), `n comments ·
  resolved ▸`, `› 3 · 2 comments, 1 suggestion ▾` when one is unfolded (the
  screen's words: the item, the count, the suggestions, ▾ when open),
  `aria-expanded`. A section's threads are the body threads whose anchor's
  slug is its slug, any version (ponytail: M3 resolves the version). Under
  720 px: `grid-template-columns: minmax(0, 1fr)`, the gutter after the
  content.
- Produces on `CommentCard`: a `<slot>` where `MarkdownBody` renders the
  body, defaulting to it; a `role?: string` prop drawn as the screen's
  `.role` chip (uppercase, 11px, bordered) after the name; for the
  description, the verb reads "wrote the description, v*n*" with
  `version(event)` and the Edit button's label "Edit · publishes v*n+1*".
- `IssuePanel`'s `#intent` passes `IssueSections` into the created
  `CommentCard`'s slot with `:html="created.html"`, the events, `meId`,
  `fields`; the folded description keeps M1's behaviour.

- [ ] **Step 1:** Implement `IssueSections.vue` (sections and gutters
  only; the chip toggles a local `open` set and does nothing else yet),
  the slot, the verb, the role chip; wire `IssuePanel`. `IssueFlow`
  passes `:role="roleOf(fields, c.author.login)"` to its cards.
- [ ] **Step 2:** `cd web && npm run typecheck && npm test`. PASS.
- [ ] **Step 3:** Browser (Task 10's board): issue A's description shows
  §1 intro to §4 Workaround with "Comment on §n" chips; a body with no
  heading shows "§1 intro" alone; two "Expected" headings show §3 and §4
  both named Expected; the header reads "wrote the description, v1",
  "Edit · publishes v2"; alice's card carries "PO" when the toml's
  `[roles] po = "alice"`. At 700 px the gutter sits under the content.
- [ ] **Step 4:** Commit `feat(issue): the description renders as
  sections with gutters`.

### Task 6: a comment anchored from a gutter

**Files:**
- Modify: `web/src/components/IssueSections.vue`,
  `web/src/components/IssueComposer.vue` (`carries` chips),
  `web/src/components/IssuePanel.vue`

**Interfaces:**
- Consumes: `comment(body, status, step, on)` (Task 4), `carries`
  (`flow.ts`), `version` (Task 1).
- Produces: `IssueComposer`'s `carries?: string` becomes `carries?:
  string[]`, one chip each ("01 Intent", "v1", "§3 Expected"); its hint
  stays "Saved in git-board now, on GitHub at the next sync. Carries …".
  The main composer passes `[carries(step), `v${version(created)}`]`
  (6a's fifth criterion). "Comment on §n" opens an `IssueComposer` under
  that section (one at a time, in the thread's place, with Cancel) whose
  `comment` emits `comment(`body@v${version}/${slug}`, body)`;
  `IssuePanel` calls `comment(body, undefined, cycle?.step ?? 0, { anchor
  })` and the composer closes on success. The step chip is the issue's,
  as the main composer's.

- [ ] **Step 1:** Implement.
- [ ] **Step 2:** `cd web && npm run typecheck && npm test`. PASS.
- [ ] **Step 3:** Browser: "Comment on §3" opens the composer under §3
  with chips "01 Intent", "v1", "§3 Expected"; posting makes a comment
  whose `meta` (the thread JSON in the network tab, or `git bug`'s
  export) has `thread:anchor` `body@v1/expected` and `cycle:step` `1`;
  §3's chip now reads "1 comment · open ▸".
- [ ] **Step 4:** Commit `feat(issue): a comment anchored from a
  section's gutter`.

### Task 7: the thread under its section: replies and Resolve

**Files:**
- Create: `web/src/components/ThreadCard.vue`
- Modify: `web/src/components/IssueSections.vue`,
  `web/src/components/CommentCard.vue` (the anchor chip),
  `web/src/components/IssuePanel.vue`

**Interfaces:**
- Consumes: `Item` (`flow.ts`), `anchorOf`, `anchorLabel` (Task 1),
  `comment(…, { reply_to })`, `resolveThread` (Task 4).
- Produces `ThreadCard`: props `{ thread: Item; sections: Section[];
  meId: string; fields: Fields; forges?: Forge[]; busy: boolean; save;
  reply(root: string, body: string): Promise<boolean>; resolve(root:
  string, value: string): Promise<boolean>; apply? (Task 8) }`. The
  screen's `.thread`: a `CommentCard` per comment (root first, replies in
  time order) with the anchor chip on the root (`anchorLabel`, a
  `<button class="gb-anchor">` that scrolls to the section) and the role
  chip; then the reply row (`<label for>` "Reply", an input, *Reply*,
  *Resolve* while open, *Unresolve* once resolved; resolved shows "resolved
  by v2" or "resolved by Person B" from `thread:resolved`: a `v` value as
  is, else the participant's name, else "resolved"). Enter in the input
  replies. Exposes nothing.
- `CommentCard` gains `anchor?: string` (the label) and emits
  `anchor` on its click.
- `IssueSections` renders the unfolded threads of a section in the
  `.gb-thread` row under the content, one `ThreadCard` each; the gutter
  chip toggles them (`aria-expanded`). Props gain `busy`, `save`,
  `reply`, `resolve`; `IssuePanel` passes `comment(body, undefined,
  step, { reply_to })` and `resolveThread(root, meId)` / `(root, '')`.
  `unfold(rootId)` opens the section's threads and scrolls to the root.

- [ ] **Step 1:** Implement.
- [ ] **Step 2:** `cd web && npm run typecheck && npm test`. PASS.
- [ ] **Step 3:** Browser: §3's chip unfolds the thread with bob's
  question and alice's answer in order, "§3 › 3" on the root; a reply
  posted lands last with `thread:reply-to` the root and the copied
  anchor; *Resolve* greys the chip to "2 comments · resolved by Person A
  ▸" and *Open threads* no longer counts it; *Unresolve* reopens. Tab
  from the chip reaches the reply row before the flow.
- [ ] **Step 4:** Commit `feat(issue): threads under their section, with
  replies and Resolve`.

### Task 8: the suggested edit: Apply and Decline

**Files:**
- Modify: `web/src/components/CommentCard.vue` (the suggestion block),
  `web/src/components/ThreadCard.vue`, `web/src/components/IssuePanel.vue`

**Interfaces:**
- Consumes: `suggestionOf`, `lineAt`, `applySuggestion`, `version`
  (Task 1), `apply` (Task 4).
- Produces: a `CommentCard` whose event has `thread:kind` `suggestion`
  hides the rendered fence (`pre:has(> code.language-suggestion)`) and
  shows the `.gb-sug` block: the anchored line (`lineAt(created.body,
  sectionsOf(created.html), anchor)`) struck on `--del`, the suggestion's text on `--add`. Props
  gain `suggestion?: { line: string; text: string; author: boolean;
  resolved: boolean }` and the card emits `apply` and `decline`. For the
  body's author (`created.author.id === meId`) while the root is
  unresolved: *Apply in v*N+1 (N = `version(created)`), *Decline*, the
  note "Applying publishes v*N+1* with this line replaced." (ruling 3;
  the screen's note speaks of a draft); others see the block alone.
  Once `thread:applied` is on the root: "applied in v*N*" from
  `thread:resolved`, no buttons.
- *Apply*: `applySuggestion(created.body, sectionsOf(created.html),
  anchor, text)`; undefined →
  the error "The anchored line is gone; edit the description instead."
  shown in the card; else `apply(root, newBody, created.body, N + 1)`.
  409 `changed` shows CommentCard's existing "Someone changed this text
  meanwhile" row, without the "Save mine anyway" path (ponytail: reload,
  then apply again).
- *Decline*: focuses the thread's reply input prefilled with "Declined: "
  (README: a declined suggestion is a reply by the author, the thread
  stays open). After a reply by the body's author later than the
  suggestion, the block reads "declined" and keeps *Apply* only.

- [ ] **Step 1:** Implement.
- [ ] **Step 2:** `cd web && npm run typecheck && npm test`. PASS.
- [ ] **Step 3:** Browser, as alice on issue A with bob's suggestion on
  `body@v1/expected#3`: the diff shows the old item struck and the new
  line added; as bob no buttons; as alice *Apply in v2* rewrites item 3
  of Expected, the header reads "wrote the description, v2", §3's chip
  reads "3 comments · resolved by v2 ▸", and the issue's operations show
  exactly two new ones: the `edit-text` and the `set-meta` with
  `thread:resolved` `v2` and `thread:applied` the edit's id (`git bug`'s
  export, or the thread JSON: the created event's `history[1].id` equals
  the root's `thread:applied`; the operation count is Task 3's test).
  Edit the description in another tab, then Apply: 409, nothing written
  on the root, the thread still open (Review Focus 4). *Decline* prefills the reply.
- [ ] **Step 4:** Commit `feat(issue): a suggested edit applies in the
  next version, or is declined`.

### Task 9: threads in the flow and the Threads seg

**Files:**
- Modify: `web/src/components/IssueFlow.vue`,
  `web/src/components/IssuePanel.vue` (`reveal`)

**Interfaces:**
- Consumes: `Reading.where`, `isRoot`, `rootOf` (Task 2), `anchorLabel`
  (Task 1), `ThreadCard` (Task 7).
- Produces: the controls' second row gains the seg "Threads [under their
  section | in the flow]" (`role="group"`, `aria-label="Where threads
  show"`, `aria-pressed`), writing `reading.where`. A body thread's item:
  with `where === 'section'` and the description rendered, a pointer line
  `<li class="gb-fe gb-fptr">13:40 · Person B commented on <button
  class="gb-anchor">§3 › 3</button> · under the section ↑</li>` (verbs:
  "commented on" for a root, "replied on" for a reply, "suggested an edit
  on" for a suggestion), its button calling the sections' `unfold(root)`;
  the replies are pointer lines of their own, in time order, under *Flat*
  and *Threads* alike. Otherwise (`'flow'`, or the description folded)
  the thread shows as review threads do: the folded line with the anchor
  label in place of the path and the chip, unfolded a `ThreadCard`. The
  gutter chip, with `'flow'`, scrolls to the thread in the flow and
  unfolds it. `reveal(id)` on a body thread comment: with `'section'`,
  `reading.description = true` then `sections.unfold(root)`; else the
  flow's `unfold`.

- [ ] **Step 1:** Implement. `IssueFlow` gets the props it lacks
  (`created`, `sections`, `reply`, `resolve`, `apply`) and an `unfoldSection`
  emit.
- [ ] **Step 2:** `cd web && npm run typecheck && npm test`. PASS.
- [ ] **Step 3:** Browser: 6a's flow lists the opening, the five pointer
  lines in time order with the plain comment between them, the composer;
  *in the flow* moves the threads into the flow (folded lines, unfold to
  the cards and reply row) and the pointers go; the seg persists across a
  reload and across issues; at step 2 (description folded) the threads
  are in the flow whatever the seg; the inbox row of bob's reply lands
  on the thread under §3.
- [ ] **Step 4:** Commit `feat(issue): threads under their section or in
  the flow, with pointer lines`.

### Task 10: verification and hand-off

- [ ] **Step 1:** `cd web && npm run typecheck && npm test` green, then
  `GOCACHE=$TMPDIR/go-build rtk proxy make ci > $TMPDIR/ci.log 2>&1`
  green (`TestCutAndPromoteSameDigests` flakes under the full run: rerun
  it alone before blaming the branch).
- [ ] **Step 2:** The board: `cd web && npm run build`, `go build -o
  $TMPDIR/git-board ./cmd/git-board`; a throwaway repo under `$TMPDIR`
  with unsigned commits, the M1 handover's `.git-board.toml` plus
  `[roles] po = "alice"` and `pm = "bob"`; `$TMPDIR/git-board serve --addr
  localhost:18431 --me alice` (this repo's git-bug lock may be another
  checkout's: serve that repo, never this one). Seed through the API with
  the handover's script extended: issue A with 6a's four-section body and
  bob's thread on §3 (a question with `anchor`, alice's reply with
  `reply_to`, bob's suggestion with the ```suggestion fence), a comment on
  §2 and on §4, a plain comment; issue B with no heading; issue C with
  "Expected" twice; the writer switched with `POST /api/me`. Reload
  after a rebuild (hash navigation keeps the old bundle).
- [ ] **Step 3:** Playwright at 1440, 1100 and 800 px, the page and the
  side panel: tick 6a's first, second, fourth and fifth criteria (the
  third is M3's) and the PRD's M2 list: two operations per Apply, one
  section for B, indexed slugs on C; the Review Focus; History's
  scrubber, arrows and *Now* unchanged; M1's checks still hold (the
  segments, the filters, the reading remembered).
- [ ] **Step 4:** Fix what the check shows, commit. `git log --oneline
  main..` lists the commits; no push. Say it is ready for `wtx land
  feat/time-flow-m2`, with the HEAD and a handover (rulings taken, the
  seed script) as M1's.

## Done when

The PRD's M2 list:
- 6a's five criteria hold (the verdict bar's are M3's);
- a suggestion applied leaves two operations, the edit and the root's
  `thread:resolved` and `thread:applied`;
- a body with no heading is one section; a repeated heading gets an
  indexed slug;
- `cd web && npm run typecheck && npm test`, then `make ci`, pass.

## Rulings

Taken by the user on 2026-10-27 (1 to 4); 5 to 7 are this plan's
calls, left standing unless review says otherwise.

1. **No pull request of its own.** The plan is the branch's first commit
   and lands with the code.
2. **Apply is two client calls.** The edit, then the meta route, with
   the edit's id read from the new `history[].id`, which M3 and M4 need
   anyway for versions and lanes. No server apply route.
3. **Apply publishes at once.** The note reads "Applying publishes v2
   with this line replaced." The screen's draft is not built: nothing in
   the page keeps a draft.
4. **Section anchors only.** The gutter anchors a comment on the section
   (`§3`); `§3 › 3` is read and shown when a comment carries it (seeded,
   or later from a forge). No item picker. A suggestion without an item
   replaces the section's body.
5. **Threads placement.** The README's rule ("under its section while
   the spec is open, in the flow once the issue has moved on") is what
   M1's folded description already gives from step 2 on. One remembered
   seg, `where`, default *under their section*, applied whenever the
   description is rendered; no `auto` value.
6. **`thread:kind` is the server's.** It derives the key from the
   ```suggestion fence, so a comment from any client is a suggestion when
   it looks like one.
7. **Anyone may resolve**, as review threads today; `thread:resolved` is
   the person's id, or `v`N on Apply; "" reopens.

## Self-review notes

- Spec coverage: sections and gutters ✓ (1, 5); the anchor from a gutter
  ✓ (6); threads under their section ✓ (7); in the flow and the seg ✓
  (9); replies ✓ (7); suggested edits with Apply / Decline, two
  operations ✓ (3, 4, 8); no heading, repeated heading ✓ (1, 5); the
  role chip ✓ (2, 5); the version in the header and the composer's chips
  ✓ (5, 6); the pointer lines ✓ (9); < 720 px ✓ (5); focus order ✓ (7);
  the reading remembered ✓ (2, 9); GitHub readable: a thread comment's
  body stays plain Markdown ✓ (3 writes keys only).
- Not in M2, each with its milestone: the verdict bar, carried /
  outdated, the version switcher, *On vN*, the Spec block (M3); the
  Versions lane and the snapshot's threads (M4); the quote header and the
  marker's keys (M5); plan anchors (open point 5 of the PRD).
- The README's `SectionGutter` is inside `IssueSections.vue`, its only
  user, as M1 kept `FlowControls` inside `IssueFlow.vue`; `ReviewThread.vue`
  is not reused: its words are a pull request's, `ThreadCard` is 6a's.

wtx-effort: high
