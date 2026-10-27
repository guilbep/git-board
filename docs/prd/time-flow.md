# The time flow: one column, one issue, three readings

PRD, 2026-10-26. Builds on `cycle-protocol.md` (the current step is the
main screen; a comment carries its step) and on `core.md` and
`docs/spec/data-model.md` (the core's operations). The decisions below
were taken with the user on 2026-10-26, on the design canvas "git-board ·
one issue, one plane?" (row 6, artboards 6a to 6g). The handoff is
`docs/design/time-flow/`: `README.md` (the decisions, the screens'
criteria, the states), `protocol.md` (what is stored and how it syncs),
`brief.md` (the build order), and `screens/`, one HTML file per artboard.

Risk class: **structural**. The time flow gives a meaning to new
metadata keys (`thread:*`, `spec:*`) and changes what sync posts. So this
PRD and the M1 plan go through their own pull request first, and no code
is written before the user approves them.

## The problem

Today's issue page is row 5 of the canvas: the step's workspace, with the
discussion in a 340 px aside beside it. The aside is narrower than the
description, not what a GitHub or GitLab reader expects, and it hides
History and Changes, which the page showed at full width before the step
became its main screen and which are worth keeping (the History scrubber
above all). And the spec phase has no
collaboration beyond comments: the PM can't point at a section, propose a
wording, or say "this version is good". When the description is edited,
nothing says what the earlier comments were about.

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
| Steps are segments | The flow is cut into segments at the operations that opened each step. *This step* and *All* are filters of the flow, not rooms; so are *Open threads*, *On v*N, *Mine*, *Agent*. The step rail jumps to a segment. The composer carries the current step, as today (`cycle:step`). |
| Mid-cycle | From step 2 on, the current step's workspace (the plan, the test runs, the diff, the ticks) heads the column, above the folded description; its segment of the flow follows (6g). The workspaces are row 5's, unchanged inside. |
| Order and grouping | *Oldest first / Newest first*. *Flat / Threads*: in Threads, a thread folds to its root's first words plus a count and a state. A "new since your last visit" line, from what `lastvisit.ts` already knows. |
| GitHub stays readable | Every comment is posted flat, in time order. An anchored comment carries a quote header; a suggestion keeps its block; the keys ride in the marker sync already hides on the last line. GitLab gets a discussion per thread. Nothing on the forge needs git-board to be read. |
| Roles in the screens | Person A wrote the issue (PO, the body's author). Person B validates it (PM). Who may give a verdict is an open point below. |

## The rules that hold throughout

- No new entity, no new operation kind. Everything the time flow does is
  one of the core's seven issue operations, with `meta` keys where
  git-board reads it back.
- The core gives no key a meaning; git-board does.
- Versions and thread states are computed when the page renders, never
  stored.
- The step is never typed: it comes from the status and the evidence
  (`internal/cycle`), as today.
- GitHub #37 stays readable without git-board.
- `review:*` comments stay local, as `docs/spec/sync.md` says.
- History's scrubber keeps today's behaviour.
- `cycle:step` stays as it is: its name and its values.

## The protocol

`docs/design/time-flow/protocol.md` is the written form; it says which
key is set by what, the anchor grammar (`body@v1/expected#3`), how
versions and ticks are counted, how carried, outdated and resolved are
computed, and the three things sync adds. In short:

| Key | On | Milestone |
| --- | --- | --- |
| `cycle:step` | a `comment` | exists; M1 changes only when the composer sets it (call 2) |
| `thread:anchor`, `thread:reply-to`, `thread:kind` | a `comment` | M2 |
| `thread:resolved`, `thread:applied` | a root `comment`, by `set-meta` | M2 |
| `spec:verdict`, `spec:on` | a `comment` | M3 |

Sync (M5) posts a thread comment flat with a quote header, keeps a
suggestion's block, and appends the keys to the marker it already writes
on the last line (`<!-- git-board comment <id> step=7 anchor=… -->`); pull
strips the header and the marker and restores the keys as `meta`.

## Calls taken

The decisions leave these to the builder. They are mine, of 2026-10-26;
calls 2, 5, 6 and 9 need the user's eye.

1. **`cycle:step` keeps today's values, "1" to "7".** `protocol.md`
   writes `"01"` … `"07"`, in its table and its example. The rule "keep
   `cycle:step` as it is" wins: "01" is how the gutter and the dividers
   print a step, not what is stored. M5's marker writes `step=7`.
   `protocol.md` is left as it was handed; this call overrides its two
   "01"s.
2. **The composer always carries the issue's step.** Today it writes the
   step shown under *This step* (a done step picked on the bar writes that
   step) and nothing under *All*. Once *This step* and *All* are filters,
   not rooms, the composer writes `cycle:step` = the issue's current step
   (`cycle.step`) whatever is filtered or scrolled to; nothing when the
   cycle hasn't loaded, as today. The key and its values are unchanged;
   every new comment carries one. Revise in the plan workspace keeps
   writing 3, which is the issue's step when Revise shows.
3. **Segments are computed in the browser, from the timeline.** The
   server computes the step from today's evidence only; it keeps no
   history of steps. An entry's segment:
   - a comment with `cycle:step`: that step;
   - a review line comment (`review:*`) without one: 6, as `forStep` reads
     it today;
   - anything else (a comment without the key, a label, status or title
     change): the step running at its time. A `status:` label sets it: a
     status before ready 1, ready 2 (3 when a `risk:` label is on),
     in-progress 4, in-review 6, a validation or the closing status 7;
     closing the issue sets 7. A `risk:` label added at ready raises it to
     3; a comment's `cycle:step` raises it until the next status change.
     A step the cycle skips (4 for a doc or a chore, 3 at low risk) moves
     it up to the next step not skipped. After the last status change, it
     is the issue's step.

   Every step from 1 to the issue's step has a segment, empty ones
   included, minus the skipped ones; so has any later step a comment
   carries (an issue sent back from Verify keeps its 06 and 07). The
   current one's divider says *current*. Segments go in step order,
   entries in time order inside. Before the cycle loads, or when it
   fails, the flow is one list in time order with no dividers. The
   ceiling: a step opened by evidence alone (criteria written, a test
   committed, a PR opened) shows only through the comments that carry it.
   The upgrade is the server replaying `internal/cycle` over the issue's
   operations.
4. **The step rail's entries are links.** A done or current step is an
   `<a href="#/issues/<id>?step=N">`; a click scrolls to the segment's
   divider and moves focus to it (a click with a modifier key opens the
   link as the browser does), and the link opened elsewhere lands on the
   page scrolled there. A locked or skipped step stays an entry with its
   reason, not a link, as today.
5. **A done step's workspace stays one click away.** Today a done step
   picked on the bar shows its workspace (the approved plan, the sign-offs).
   The rail now jumps to the segment, so a done segment's divider carries
   *Show the workspace*, which brings that step's workspace to the head of
   the column with today's "Back to step N".
6. **The workspace heads the column at step 7 too.** 6c (step 7) draws the
   column without a workspace because it is about the flow; the Mid-cycle
   decision names "the ticks", which are step 7's. At step 1 the
   description is the head, with Intent's criteria under it, as today.
7. **The tabs are Conversation and History *n* in M1.** History is
   today's `HistoryView`, unchanged, at the column's width with the
   sidebar beside it, as 6d draws it. *Changes n* comes with M4. Today's
   *This step / All / History* switch above the discussion goes.
8. **M1's threads are the review threads.** Body threads need
   `thread:*` keys (M2). In M1, *Threads* folds each review thread
   (`threadsOf`) to one line: root author, `path:line`, first words, count,
   open or resolved; the `path:line` still opens the line in step 6's
   workspace. *Open threads* counts the unresolved ones. `ReviewAside`
   goes: Threads says the same thing in the flow.
9. **The flow's defaults are All, Threads, Oldest first** (6c), on a
   first visit. After that the page remembers each person's reading (open
   point 6): the tab, the filters, the order, the grouping and whether the
   description is unfolded, kept in the browser as `lastvisit.ts` keeps
   the visit, one reading for every issue.
10. **Agent is a comment with `ai:model`**, the only mark git-board writes
    today (`suggestRisk`). No new key. What an agent posts through the
    person's identity is the person's.
11. **"New since your last visit" reuses `lastvisit.ts`**, keyed
    `issue:<id>`: the separator stands before the first entry, not mine,
    after the visit before this one. None on a first visit, none when
    nothing is new. The inbox's jump to the first new event stays.
12. **The sync state moves to the comment's header**: "on GitHub #37 ✓",
    "not on GitHub yet, next sync", or "stays local" for a review
    comment, one per forge.
13. **Send to is the issue's agent, when it has a worktree**: a checkbox
    in the composer naming it ("Send to Claude Code"); the comment is
    posted, then briefed to the agent, as Revise does today. One agent per
    worktree in wtx, so no list; no agent, no checkbox.
14. **The folded description, in M1**: the author, the criteria count,
    when it was last edited, and *Read the description*, which unfolds it
    in place. Versions, threads and the verdict join it in M2 and M3.

## Open points

Questions for the user. The first six are the README's, each with what I
proposed; the last three are mine. Settled on 2026-10-27: the user took
every proposal.

1. **Who may give a verdict.** A role from `[roles]`, as the plan role and
   the reviewer are, or anyone who is not the body's author? I propose a
   gate role, `[cycle] spec = "pm"`, as `risk`, `plan` and `review` are;
   unset, anyone but the body's author. (M3)
2. **A version after approval.** v3 published after v2 was approved: the
   bar says "approved v2, v3 awaits". Does the status move back, or does
   the next step's gate simply wait? The README proposes: it flags and
   waits, the status stays. I agree: moving the status back would undo
   step 2's risk and step 3's plan for a typo. (M3)
3. **When thread comments reach the forge.** As written, like every
   comment today (some noise on GitHub during spec), or at the verdict? I
   propose as written: GitHub then reads in the same order as git-board,
   and holding comments until a verdict is a queue nobody sees. (M5)
4. **Narrow width.** Not drawn; the README's rule is a proposal. M1
   builds what is drawn (≥ 1280 px, 900–1280 px) and the sidebar stacking
   under 900 px; the rules under 720 px come with the sections (M2), which
   are what they change.
5. **Anchors on other texts.** 6g anchors a comment on the plan (`plan v1 ›
   Steps`); the grammar allows it, the first milestones don't need it. I
   propose it stays out of this PRD: its own issue after M5.
6. **A mode switch (Issue / Cycle) for people who read differently.** A PM
   who reads only titles and moves cards. The README proposes no second
   page; instead the page remembers each person's reading (tab, filters,
   whether the description is folded), a *Milestones* filter shows the
   flow without comments, and the board's panel stays that PM's door:
   title, fields, move. I agree. Settled: yes, the remembered reading and
   the *Milestones* chip join M1 (a task more, no new data). *Milestones*
   narrows the flow to what is not a comment, as *Mine* and *Agent*
   narrow it to an author; the three are a union.
7. **A done step's workspace** (call 5): the divider's *Show the
   workspace*, or should a rail click also bring it to the head, as
   today? Settled: the divider's button; the rail only jumps.
8. **The design folder.** It was handed over untracked, one level too
   deep. Settled: the plan's pull request carries it as
   `docs/design/time-flow/` (one level, as the brief and this PRD name
   it), without its `.claude` folders.
9. **The default filter.** 6c shows *All* at step 7; 6g shows *This step*
   at step 3. Settled: *All* everywhere on a first visit (call 9), then
   the person's own.

## Milestones

Each ships on its own and leaves the app working, in `brief.md`'s order.
This branch builds M1 after the plan's approval. M1 and M4 need nothing
new; M2 and M3 write `meta` through what the core offers at the time; M5
waits for forge sync on the core (core M5's second phase).

### M1: the column

The aside goes. Conversation at full width: the description first, the
flow after it with segments, folded threads, order, the filters, the
folded description from step 2 on with the step's workspace heading the
column, the composer's step chip, "new since your last visit"; the
person's reading remembered, and the *Milestones* filter (open point 6).
Screens 6c, 6g. Plan: `time-flow-m1-plan.md`.

Done when:
- 6c's five criteria hold, and 6g's first two (the third is open point 5);
- the reading picked on one issue is the one the next issue opens with,
  after a reload too; *Milestones* shows no comment;
- the tabs read Conversation and History *n*, and History's scrubber,
  lanes and snapshot behave as today;
- every new comment carries the issue's step;
- the side panel on the board shows the same column, with the sidebar
  under it;
- `cd web && npm run typecheck && npm test`, then `make ci`, pass.

### M2: sections and threads

The body rendered as sections with gutters; a comment anchored from a
gutter; threads under their section or in the flow, with the toggle;
replies; suggested edits with Apply / Decline (Apply is an `edit-text`
then a `set-meta`). Screens 6a, 6f.

Done when:
- 6a's five criteria hold (the verdict bar's are M3's);
- a suggestion applied leaves two operations, the edit and the root's
  `thread:resolved` and `thread:applied`;
- a body with no heading is one section; a repeated heading gets an
  indexed slug.

### M3: versions and the verdict

Version counting; the version switcher with *Changes since*; carried,
outdated and resolved; the verdict bar and its status move; the *On vN*
filter; the Spec block in the sidebar. Screen 6b.

Done when:
- 6b's five criteria hold, and 6a's verdict bar;
- a tick is not a version;
- Approve vN moves the status to ready by the path of Move to ready, and
  a later version shows "approved vN, vN+1 awaits" (open point 2).

### M4: History and Changes

Two more lanes (Versions and ticks, Steps), the snapshot with the version
and the threads as of then, the log cut at the cursor; the Changes tab
with the versions as diffs and the threads on them, then the code.
Screens 6d, 6e.

Done when:
- 6d's and 6e's criteria hold;
- the scrubber, the arrows and *Now* behave as today.

### M5: sync

Thread comments posted with the quote header; the marker carries the
keys; pull restores them; GitLab discussions. Screen 6f.

Done when:
- 6f's comment, posted to a scratch repository and pulled back, is the
  same comment with the same `meta`;
- a `review:*` comment is never posted;
- the issue on GitHub reads, in order, without git-board.

## Out of scope

- Anchors on the plan or any text but the body (open point 5).
- Voting, or approving sections one by one.
- A second page or an Issue / Cycle switch (open point 6).
- Posting `review:*` comments to a pull request.

## Constraints

- The workspaces (`Cycle*.vue`) are mounted in the column, unchanged
  inside.
- Live checks go to scratch repositories only: guilbep/git-board-test,
  guilbep-sandbox. Never co2-calculator.
- No real person's name in mocks, fixtures or this file: Person A, B, C.
- EPFL tokens from `web/epfl-design`, not hex values.
- No new dependency.
