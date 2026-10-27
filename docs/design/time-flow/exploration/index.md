# The exploration: rows Today to 5 of the canvas

The canvas "git-board · one issue, one plane?" before row 6, as plain HTML, with its notes. These are the directions tried on 2026-10-21 and 2026-10-22; row 5 (C1 to C7) was built, and its aside is what row 6 replaces. Read for the reasoning, not as a spec.

| File | Title on the canvas |
| --- | --- |
| `Main.html` | Today · the Issue view of GB-31 |
| `T2-cycle.html` | Today · the Cycle view of GB-31 |
| `P1-spine.html` | 1 · The spine beside the issue |
| `P2-story.html` | 2 · The cycle in the issue's story |
| `P3-seams.html` | 3 · Two views, one document |
| `S1-steps.html` | 4a · Step by step: where each step lives |
| `S2-protocol.html` | 4b · A protocol: one schema, two readings |
| `C1-intent.html` | Step 1 · Intent: the issue itself |
| `C2-risk.html` | Step 2 · Risk class |
| `C3-plan.html` | Step 3 · Plan: read, iterate with the agent, approve |
| `C4-tests.html` | Step 4 · Tests first: the bug's failing test |
| `C5-code.html` | Step 5 · Code: the branch, then the PR |
| `C6-review.html` | Step 6 · Review: the code review is the screen |
| `C7-verify.html` | Step 7 · Verify: the criteria, ticked and signed |

## The notes on the canvas, top to bottom

### Today · one issue, two planes

The Issue and the Cycle are two pages of one issue: in the Cycle you can't see the description or the conversation. The cycle reads criteria only under a heading named exactly "Acceptance criteria". GB-31's four, under Expected, weren't seen, so writing one from the Cycle appended a second list.

### 1 · One page: the spine beside the issue

Drop the switch. The spine becomes a rail beside the issue, and a step's workspace opens above the description. The seven steps stay visible at a glance; three columns are tight at 900px.

### 2 · One page: the cycle is the issue's story

Drop the Cycle view. Each step leaves an entry in the conversation, the next-step card stays under the title, and the criteria are the description's checklist, ticked at Verify. It reads as it would on GitHub; the separate workspaces go.

### 3 · Keep the switch, fix the seams

The Intent workspace is the description's own list, edited in place; Acceptance criteria, Done when or Expected all count. Each other step shows its slice of the conversation. The smallest change, but still two pages.

### 4 · Step by step, then a protocol

Going step by step: only 01 and 07 live in the description, 02 is a field, 03 to 06 live in the code (plan file, failing test, branch, pull request), and sign-offs are comments. The cycle is the issue plus its branch. A protocol that names where each step lives lets both views render from the same data.

### 5 · The cycle, one step at a time: each step is the main screen

### 5 · Steps 5 to 7: the code, its review, its proof

Back is always open: a done step reopens with its discussion. Ahead is locked until its evidence exists: the branch and a draft PR start at step 3 with the plan (wtx), Review needs that PR marked ready, Verify a merged one. A comment written in a step carries that step; the full conversation keeps every comment in order, as GitHub shows it. GB-31 is followed through all seven; from step 2 on it is an illustration.

## Row 6, for completeness

### 6 · Time flow: one column, the issue's whole life, three readings

Drop the aside. The description is the first entry of one GitHub-wide column and everything after it is the flow. A thread anchors to a section of a version: shown under its section while the spec is open, in the flow once the issue moves on. The verdict is on the version; a new version asks again. Steps are segments of the one flow, so This step and All are filters, not rooms. Mid-cycle (6g), the current step's workspace heads the column and its segment follows. Roles here: Person A wrote the issue (PO), Person B validates it (PM).

### 6 · The same flow as History, as Changes, and as GitHub sees it

History keeps today's scrubber, at full width: the lanes, the cursor, the issue as of then, the flow cut at that change. Changes holds the description's versions as diffs with the threads written on them, then the code. Nothing new is stored: a version is the core's edit-text on the body, a tick an edit that flips checkboxes only; anchor, reply, verdict and step are meta keys on the comment, like cycle:step today. GitHub #37 gets every comment flat, the anchor quoted, the keys appended to sync's marker and restored on pull; GitLab gets a discussion per thread.
