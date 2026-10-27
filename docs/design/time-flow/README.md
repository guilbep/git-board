# The time flow: one column, one issue, three readings

Design handoff, 2026-10-26. From the design canvas "git-board · one issue,
one plane?", row 6 (artboards 6a to 6g), decided with guilbep on 2026-10-26.
Builds on `docs/prd/cycle-protocol.md` (the current step is the main screen;
a comment carries its step) and on `docs/prd/core.md` and
`docs/spec/data-model.md` (the core's operations). The screens are in
`screens/`, as plain HTML that opens in a browser; the earlier rows of the
canvas, the exploration that led here, are in `exploration/`, with the
canvas notes in `exploration/index.md`. `protocol.md` says what is stored
and how it syncs; `brief.md` says how to start.

## The problem

Row 5 of the canvas put each step's workspace on the issue page with the
discussion in a 340 px aside beside it. The aside is narrower than the
description, not what a GitHub or GitLab reader expects, and it hides
History and Changes, which today's issue page shows at full width and
which are worth keeping (the History scrubber above all). And the spec
phase has no collaboration beyond comments: the PM can't point at a
section, propose a wording, or say "this version is good". When the
description is edited, nothing says what the earlier comments were about.

**One column, GitHub-wide. The description is its first entry and
everything after it is the flow. A thread anchors to a section of a
version; the verdict is on the version. The same flow reads as
Conversation, as History (with a cursor) and as Changes (as diffs).**

## Decisions

Taken with the user on 2026-10-26. Not reopened here.

| Topic | Decision |
| --- | --- |
| One column | The description is the first entry of one column, as wide as GitHub's; everything after it is the flow: comments, threads, milestones. Fields keep the sidebar. The aside of row 5 goes. |
| Three readings | Conversation is the flow. History is the flow with a cursor: today's scrubber, lanes and "as of then", at full width. Changes is the body's versions as diffs, with the threads written on those lines, then the code. The tabs stay as today: Conversation, History *n*, Changes *n*. |
| Threads anchor to a section of a version | A comment may carry an anchor: the body's version, a section (its heading, slugged), an item of its list. A thread shows under its section while the spec is open and in the flow once the issue has moved on; a toggle (*Threads: under their section / in the flow*) switches. In the flow, an anchored comment quotes its anchor. |
| The verdict is on the version | PR-style. The PM comments sections and suggests edits; then one verdict per version: *Approve vN* or *Request changes*. Approving moves the status to ready, which opens step 2. A new version asks for a new verdict. Sections are never approved one by one; there is no vote. |
| A version is an edit that changes text | The body's versions are counted, never stored: the n-th `edit-text` on the body that changes text is v*n*. An edit that only flips `- [ ]` to `- [x]` (a tick, step 7) is not a version. |
| What becomes of a thread when a version is published | Carried: its section is unchanged, the thread stays open on the new version. Outdated: its section changed and nobody resolved it; it stays open, marked "written on v1, §4 changed in v2". Resolved: by Apply (a suggested edit applied) or by a person. All three are shown in 6b. |
| Suggested edits | A comment may carry a replacement for its anchored line (a ```suggestion block). The author of the body may *Apply in v*N+1 (writes the next version, resolves the thread) or *Decline*. |
| Steps are segments | The flow is cut into segments at the operations that opened each step. *This step* and *All* are filters of the flow, not rooms; so are *Open threads*, *On v*N, *Mine*, *Agent*. The step rail opens a step at the head of the column. The composer carries the current step, as today (`cycle:step`). |
| Mid-cycle | From step 2 on, the current step's workspace (the plan, the test runs, the diff, the ticks) heads the column, above the folded description; its segment of the flow follows (6g). The workspaces are row 5's, unchanged inside. |
| Order and grouping | *Oldest first / Newest first*. *Flat / Threads*: in Threads, a thread folds to its root's first words plus a count and a state. A "new since your last visit" line, from what `lastvisit.ts` already knows. |
| GitHub stays readable | Every comment is posted flat, in time order. An anchored comment carries a quote header; a suggestion keeps its block; the keys ride in the marker sync already hides on the last line. GitLab gets a discussion per thread. Nothing on the forge needs git-board to be read. |
| Roles in the screens | Person A wrote the issue (PO, the body's author). Person B validates it (PM). Who may give a verdict is an open point below. |

## The screens

Each file in `screens/` is one state of GB-31 at 1440 px. Read them in
this order; the acceptance criteria are what a reviewer ticks.

### 6a `F1-spec-v1.html` · Spec: v1 under discussion, threads under their section

Step 1, status backlog. The description v1 with four sections and a gutter
per section (§1 intro, §2 Releases page, §3 Expected, §4 Workaround). §3's
thread is open under the section: a question, an answer, a suggested edit
with Apply / Decline. The verdict bar under the description. The flow below
holds the opening, one-line pointers to the anchored comments ("13:40 ·
Person B commented on §3 › 3 ↑"), a plain comment, the composer.

- [ ] The description renders as sections: the prose before the first heading is §1; each heading opens a section; a section's gutter shows its anchor and a chip (none: "Comment on §n"; open threads: "n comments · open"; a suggestion counts).
- [ ] A thread under its section shows its comments in order, the anchor on the root, a suggestion as a two-line diff with Apply / Decline for the body's author only.
- [ ] The verdict bar names the version, who reviews, how many threads are open, and what Approve does; Approve moves the status to ready.
- [ ] With *Threads: under their section*, the flow shows an anchored comment as a one-line pointer, in time order with everything else.
- [ ] The composer carries the step and the version as chips, and says where the comment goes ("Saved in git-board now, on GitHub at the next sync").

### 6b `F2-spec-v2.html` · Spec: v2 published, what v1's threads became

Person A published v2 after Person B requested changes. The version
switcher (v1 | v2), *Changes since v1* on: added text green, removed text
struck. §1 changed (a sentence added), §2 unchanged with its thread carried,
§3's thread resolved by v2 (the suggestion applied), §4 rewritten with its
thread outdated and still open. The flow filtered *On v2 · 4* with the
seven v1 entries folded; the "published v2" milestone names the changed
sections and the resolved thread.

- [ ] Versions are numbered by counting `edit-text` operations on the body that change text; ticks don't count.
- [ ] *Changes since v*N-1 renders the body as a diff per section; off, it renders v*N* plain.
- [ ] A thread's state is computed at render: carried (section unchanged), outdated (section changed, not resolved), resolved (`thread:resolved` set). Each has its chip and, for outdated, a notice in the thread.
- [ ] The verdict bar shows the previous verdict and that the new version waits for one.
- [ ] *On v*N keeps the entries written on that version and the version's own milestone; the folded entries say how many are hidden and offer *show all*.

### 6c `F3-flow.html` · The whole life as one flow

Step 7. The description folded to two lines. Controls: filters, order,
grouping, thread placement (*in the flow*). Seven segments with dividers
("01 Intent · Oct 21 – 22 · 12 entries"); threads folded to their first
words; milestones as plain lines with the step number in the gutter; a
"new since your last visit" line; the two Verify comments with their sync
state; the composer carrying 07.

- [ ] The flow is cut into segments at the operations that opened each step; the current step's divider says *current*.
- [ ] *Flat* lists every comment in time order; *Threads* folds each thread to one line (root author, anchor, first words, count, state) that unfolds in place.
- [ ] *Newest first* reverses the order of entries and of segments.
- [ ] Every entry shows its sync state when it is not on the forge yet ("not on GitHub yet, next sync"), as today's events do.
- [ ] The step rail's entries are links that open their step.

### 6d `F4-history.html` · History: the flow with a cursor

Today's History at full width: four lanes (Texts, Versions and ticks,
Status, Steps) over the issue's time, a cursor, a scrubber ("As of Oct 22
10:40 · change 13 of 31"), the issue "as of then" with what the change
touched outlined, and the change log cut at the cursor, later rows dimmed.
Each row: who, operation, what, op id, what GitHub #37 has.

- [ ] The lanes are today's, plus Versions (a diamond per version, a hollow one per tick) and Steps (one bar per segment).
- [ ] The cursor moves with the scrubber, the arrows and *Now*; the snapshot and the log follow it.
- [ ] The snapshot shows the body's version as of then, its threads' states as of then, and the fields, with the changed ones outlined.
- [ ] The log's operations are the core's (`create`, `comment`, `edit-text`, `change-labels`, `set-meta`, `set-status`), one row each, with their forge state.

### 6e `F5-changes.html` · Changes: the description's versions, then the code

The version strip (v1 → v2, plus the ticks, listed, not numbered), the
diff v1 → v2 rendered section by section with the threads written on those
lines, then the branch, its three commits with CI, the files, the plan.

- [ ] Changes lists the body's versions, each with who, when, the sections changed, the verdict it got; ticks are listed apart.
- [ ] The diff shows the threads anchored on changed lines, folded, with their state; a resolved suggestion shows *applied in v*N.
- [ ] The code part is today's: branch, commits, PR, files, the plan file.

### 6f `F6-protocol.html` · One comment, three readings

The same suggested edit as the core stores it (a `comment` with `meta`),
as git-board shows it (under its section), and as sync posts it to
GitHub #37 (a flat comment with a quote header and the marker). Then the
table of where each thing lives. `protocol.md` is the written form.

### 6g `F7-step3.html` · Step 3 in the flow: the workspace heads the column

Mid-cycle. The plan workspace of row 5's C3 heads the column (version
switcher, worktree strip, the plan, Approve), then the folded description,
then the flow filtered *This step*, with the agent's thread and a composer
that can send to the agent.

- [ ] From step 2 on, the current step's workspace is the first block of the column; the description folds under it with a *Read the description* button.
- [ ] *This step* shows only the current segment and says how many entries are folded.
- [ ] A comment on a plan carries an anchor on the plan's version (`plan v1 › Steps`): the same anchor grammar, on another text.

## Layout

| Region | Rule |
| --- | --- |
| Page | Header, title block and step rail as row 5's C-boards. Then a wrapping row: the column `flex: 999 1 640px; min-width: 0`, the sidebar `flex: 1 1 260px`, so the sidebar stacks under the column below about 900 px. |
| Column | Padding 24 px 40 px 40 px; content `max-width: 1000px`; blocks stacked with a 18–20 px gap. |
| Description | A bordered card: a header line (author, role, version, time, version switcher, *Changes since*, Edit), then the sections. A section is a two-column grid, content `minmax(0, 1fr)` and gutter 164 px, 16 px vertical padding, a hairline between sections. The criteria section keeps row 5's teal ground and inset bar. |
| Thread | Spans both columns of its section (`grid-column: 1 / -1`), a 1 px grey-300 border, comments separated by hairlines, the reply row on the subtle ground. |
| Flow entry | A grid `44px minmax(0, 1fr)` with a 12 px gap; the gutter holds the step number of a milestone, in mono teal; a comment is a bordered card with a header line and a body; an event is one line; a pointer is one muted line. |
| Segment divider | Small caps, teal, with a hairline running to the right; the current one in black. |
| Composer | Today's: *Commenting as*, Write / Preview, the hint, the textarea, a subtle footer with the sync note, the carried chips, *Close issue*, *Comment*. |
| History lanes | Label column 110 px; tracks with a dashed baseline; markers positioned as a percentage of the issue's span; the cursor is a 2 px red line across the lanes. The scrubber is an `<input type="range">` with prev / next buttons. |

## Design tokens

The boards use the EPFL palette already in `web/epfl-design`; use those
tokens, not the hex values. For reference, what the boards used:

| Token | Value | Used for |
| --- | --- | --- |
| red | #FF0000 | the EPFL mark, the active tab underline, the History cursor, the "new since" line |
| red-dark | #B51F1F | primary buttons, focus rings, an illustration mark |
| canard (teal) | #007480 | anchors, step numbers, segment dividers, open-thread chips, "Open" |
| canard-bg | #EAF4F5 | the criteria section, the History snapshot's changed field, a ready bar |
| add / add-fg | #E6F4EA / #1E5B2E | added text, PASS, "resolved by v2", "✓ Saved" |
| del / del-fg | #FBE9EA / #80000C | removed text, FAIL |
| warn-bg / warn-fg | #FFF4E0 / #7A4B00 | an outdated thread's chip and notice |
| blue-bg / fg, purple-bg / fg | #E8EEF8 / #2B4C8C, #F1E8F8 / #5B2E86 | in-progress and in-review bars of the Status lane |
| g200 / g300 / g600 | #D5D5D5 / #C1C1C1 / #707070 | rails, borders, muted text (4.7:1 on white) |
| black, subtle, hair | #212121, #F5F5F5, rgba(33,33,33,.1) | text, grounds, hairlines |
| type | Suisse Int'l, Arial fallback; mono ui-monospace | h1 28 px, h2 24–26, body 15, UI 14, meta 13, labels 12 uppercase .12em, mono 12–13 |
| radius | 2 px | buttons, chips |

## Components

| Component | Today | What changes |
| --- | --- | --- |
| `IssuePanel.vue`, `pages/IssuePage.vue` | the page | the column replaces row 5's workspace + aside layout; the current step's workspace becomes the column's first block from step 2 on |
| `IssueTimeline.vue`, `TimelineEvent.vue` | the conversation | becomes the flow: segments, folded threads, pointers, filters, order, grouping |
| `CommentCard.vue` | a comment | gains the role chip, the anchor chip (quoting its line when in the flow), the sync state line it already has, and the suggestion block with Apply / Decline |
| `MarkdownBody.vue` | the body | splits into sections by heading, each with a gutter; needs stable slugs and item numbers |
| `ReviewThread.vue` | a review thread | reused for body threads: same card, another anchor |
| `IssueComposer.vue` | the composer | carries chips (step, version, agent) and an anchor when started from a gutter |
| `IssueSide.vue` | the fields | a *Spec* block: version, verdict, open threads |
| `HistoryView.vue`, `lanes.ts`, `history.ts` | History | two more lanes, the snapshot gains the body's version and the threads' states, the log cut at the cursor |
| new `SectionGutter`, `ThreadCard`, `VerdictBar`, `VersionSwitcher`, `FlowControls` | — | as drawn; the version switcher exists in `CyclePlan.vue` already |
| `Cycle*.vue` | the workspaces | unchanged inside; mounted at the head of the column |

## States and interactions

| Element | State | Behaviour |
| --- | --- | --- |
| Gutter chip | no thread | dashed "Comment on §n"; opens the composer under the section with the anchor set |
| Gutter chip | open threads | teal border, "n comments · open ▸"; toggles the thread under the section; `aria-expanded` |
| Gutter chip | resolved | grey, "n comments · resolved by v2 ▸" |
| Gutter chip | outdated | amber, "1 comment · open · written on v1 ▾" |
| Thread | folded / unfolded | one line in the flow: root author, anchor, first words, count, state; unfolds in place |
| Suggestion | pending | diff block; Apply / Decline shown to the body's author only; others see the block |
| Suggestion | applied | "applied in v2", the thread resolved |
| Suggestion | declined | a reply by the author; the thread stays open until resolved |
| Verdict bar | awaiting | "Person B, PM, reviews vN", the counts, Request changes / Approve vN |
| Verdict bar | changes requested | the previous verdict named with its time, the bar waits for the next version |
| Verdict bar | approved | "approved vN by …"; a later version shows "approved v2, v3 awaits" |
| Version switcher | vN selected | renders that version; *Changes since* diffs against the one before |
| Flow filters | This step / All / Open threads / On vN / Mine / Agent | one active at a time except Mine and Agent, which combine; counts in the chips |
| Order | Oldest / Newest first | reverses entries and segments |
| Grouping | Flat / Threads | Threads folds replies under their root |
| Threads placement | under their section / in the flow | moves thread bodies; the flow keeps a pointer line when they are under their section |
| New since | a separator | at the first entry after the last visit; absent when nothing is new |
| Sync state | posted / not yet / local | "on GitHub #37 ✓", "not on GitHub yet, next sync", "stays local" (as today's `review:*` comments) |
| Composer | from a gutter | carries the anchor chip; Comment posts a `comment` with `thread:anchor` |
| Composer | in a step with an agent | the *Send to* checkbox and the agent list, as row 5's C3 |
| History scrubber | moved | the cursor, the snapshot and the log follow; *Now* returns to the last change |

## Responsive behaviour

| Width | Changes |
| --- | --- |
| ≥ 1280 px | as drawn: column plus sidebar |
| 900–1280 px | the column narrows; the gutter keeps 164 px; the flow controls wrap to two rows |
| < 900 px | the sidebar stacks under the column (flex-wrap) |
| < 720 px (not drawn) | a section becomes one column: the gutter row goes under the section's content; the step rail scrolls horizontally; the History lanes keep their labels at 80 px |

## Edge cases

- **A body with no heading**: one section, §1, the whole text.
- **Repeated headings**: the slug gets an index (`expected`, `expected-2`); anchors use it.
- **An anchored item that disappears in a later version**: the thread is outdated; its quote keeps the old line.
- **A version published while a thread is being written**: the comment anchors to the version it was written on, as its `thread:anchor` says; the UI shows it carried or outdated at once.
- **Two people edit the body at the same time**: two `edit-text` operations; the core's order decides v2 and v3; each thread anchors to the version it names.
- **A long flow (100+ entries)**: segments before the current one fold to their divider with a count; *show all* unfolds.
- **A comment pulled from the forge** with no keys: a plain comment on the issue, in the step current at pull. A marker in a stranger's text is just text, as `docs/spec/sync.md` says.
- **No agent configured**: no *Send to* in the composer; the plan step's workspace says so, as today.
- **New issue**: the description, its gutters with dashed chips, the verdict bar, an empty flow, the composer.
- **Loading / error**: today's "Loading #… the title shows at once" and "Couldn't load #…" states.

## Accessibility

- Every control is drawn as the real element: `<button>` with `aria-pressed` for toggles and `aria-expanded` for gutter chips, `<input type="range">` with a label for the scrubber, `<label>` on every field and textarea, `<a href>` for the step rail.
- Focus order: title, step rail, tabs, the description (gutter chips in reading order, then an unfolded thread's reply row), the verdict bar, the flow controls, the entries, the composer, then the sidebar.
- The "new since your last visit" line is a `role="separator"` with an `aria-label`.
- Text contrast is at least 4.5:1 as drawn: muted #707070 on white is 4.7:1, teal #007480 is 5.0:1, amber #7A4B00 on #FFF4E0 is 7.5:1. Carried, outdated and resolved differ in words and in lightness, not in hue alone.
- No motion is required; unfolding a thread may take 120 ms ease-out, nothing else moves.

## Open points

Not decided on the canvas. The brief asks the agent to settle them in the
PRD, with the user.

1. **Who may give a verdict.** A role from `[roles]`, as the plan role and the reviewer are, or anyone who is not the body's author?
2. **A version after approval.** v3 published after v2 was approved: the bar says "approved v2, v3 awaits". Does the status move back, or does the next step's gate simply wait? Proposed: it flags and waits, the status stays.
3. **When thread comments reach the forge.** As written, like every comment today (some noise on GitHub during spec), or at the verdict?
4. **Narrow width.** Not drawn; the rule above is a proposal.
5. **Anchors on other texts.** 6g anchors a comment on the plan (`plan v1 › Steps`); the grammar in `protocol.md` allows it, the first milestones don't need it.
6. **A mode switch (Issue / Cycle) for people who read differently.** Raised on 2026-10-26: a PM who reads only titles and moves cards. Proposed: no second page (rows 1 to 3 and `cycle-protocol.md` dropped it because nobody knew which to open); instead the page remembers each person's reading (tab, filters, whether the description is folded), a *Milestones* filter shows the flow without comments (the old Cycle reading), and the board's panel stays that PM's door: title, fields, move. To settle with the user.
