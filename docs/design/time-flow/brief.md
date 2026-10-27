# Brief: handing the time flow to an agent

This folder is the Intent step of the loop (`docs/prd/dev-cycle.md`): what
to build and when it is done. The agent's first deliverable is not code,
it is the PRD and the plan, in the house style, for a person to approve.

## What to paste to the agent

```text
Read docs/design/time-flow/README.md, then protocol.md, then open the
seven files in docs/design/time-flow/screens/ (plain HTML; read the
markup, it is the reference for structure, copy and states). Skim
exploration/index.md for why the earlier directions were dropped; don't
build anything from exploration/.

Then, in plan mode:

1. Write docs/prd/time-flow.md in the style of docs/prd/cycle-protocol.md
   (the problem, a Decisions table that repeats README.md's decisions
   verbatim, the open points as questions for me, the milestones). Risk
   class: structural, since it gives meaning to new metadata keys and
   changes what sync posts; so the plan goes through its own PR first.
2. Write docs/prd/time-flow-m1-plan.md for the first milestone below, in
   the style of the other *-m1-plan.md files, task by task with checkboxes.
3. Stop and ask me to approve the plan before writing code.

Rules that hold throughout: no new entity or operation kind; the core
gives no key a meaning; versions and thread states are computed, never
stored; the step is never typed; GitHub #37 must stay readable without
git-board; review:* comments stay local; keep today's History scrubber
behaviour; keep cycle:step as it is. When a screen and this brief
disagree, the screen's structure wins and the README's rule wins over the
screen's sample data.
```

## Build order

Each milestone is reviewable on its own and leaves the app working.

| Milestone | What ships | What it needs from the core | Screens |
| --- | --- | --- | --- |
| M1 · the column | The aside goes. Conversation at full width: the description first, the flow after it with segments, folded threads (Flat / Threads), order, the filters (This step, All, Open threads, Mine, Agent), the folded description from step 2 on with the step's workspace heading the column, the composer's step chip, "new since your last visit". | nothing new | 6c, 6g |
| M2 · sections and threads | The body rendered as sections with gutters; a comment anchored from a gutter; threads under their section or in the flow; replies; suggested edits with Apply / Decline (Apply = `edit-text` + `set-meta`). | `thread:*` keys on `comment`; `set-meta` | 6a, 6f |
| M3 · versions and the verdict | Version counting; the version switcher with *Changes since*; carried / outdated / resolved; the verdict bar and the status move; *On vN* filter; the Spec block in the sidebar. | `spec:*` keys | 6b |
| M4 · History and Changes | Two more lanes, the cursor over the flow, the snapshot with the version and the threads as of then, the log cut at the cursor; the Changes tab with the versions as diffs and the threads on them, then the code. | nothing new | 6d, 6e |
| M5 · sync | Thread comments posted with the quote header; the marker carries the keys; pull restores them; GitLab discussions. | sync (core M5's second phase) | 6f |

M1 and M4 need no new data and can start before the core's M5 lands;
M2 and M3 write `meta` through whatever `internal/gitbugio` or the core
offers at the time; M5 waits for forge sync on the core.

## Where things are

- The page: `web/src/pages/IssuePage.vue`, `web/src/components/IssuePanel.vue`.
- The conversation today: `IssueTimeline.vue`, `TimelineEvent.vue`, `CommentCard.vue`, `IssueComposer.vue`; the sidebar `IssueSide.vue`; the step rail `IssueStep.vue`; the workspaces `CycleIntent.vue` … `CycleVerify.vue`; review threads `ReviewThread.vue`, `ReviewAside.vue`.
- History today: `HistoryView.vue`, `web/src/history.ts`, `web/src/lanes.ts`; last visit: `web/src/lastvisit.ts`.
- The step: `web/src/cycle.ts`, `web/src/gate.ts`; the rules of the cycle: `docs/prd/cycle-protocol.md`.
- The data model and sync: `docs/spec/data-model.md`, `docs/spec/sync.md` (markers: "### Markers"), `docs/prd/core.md`, `docs/prd/core-m5-inventory.md` (comment metadata is kept over the core).
- The palette: `web/epfl-design`.

## How a person checks a milestone

Open GB-31 (or any issue with a description that has headings and a list)
and tick the acceptance criteria of the milestone's screens in
`README.md`. Then the usual: the package's tests while developing,
`make ci` once at the end, as `CLAUDE.md` says.
