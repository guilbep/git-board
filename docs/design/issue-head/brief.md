# Brief: handing the issue head to agents

This folder is the Intent step of the loop (`docs/prd/dev-cycle.md`): what to
build and when it is done. The plan is already written,
`docs/prd/issue-head-plan.md`, cut into milestones of a day or less, most of
which can run at once on their own branches. Before a milestone that waits
on an open point starts, the point is settled with guilbep and the README's
*Decisions* table gains a row.

## What to paste to an agent

```text
Read docs/design/issue-head/README.md, then open the screens it lists in
docs/design/issue-head/screens/ (plain HTML; the markup is the reference
for structure, copy and states; drawer.css is the stylesheet as drawn).
Then read docs/prd/issue-head-plan.md and take milestone <ID>: check that
every milestone in its "Needs" has landed and that its "Decision" column
is settled; if not, stop and say which.

In plan mode, write the milestone's tasks as checkboxes in the plan file,
each naming the files it touches and its test, then stop for approval.

Rules that hold throughout: the EPFL design system as web/README.md says
it (tokens only, no hex, no px, Feather icons, borders not shadows); run
the adherence linter from web/README.md on the files you touch; no real
person's name in mocks, fixtures or tests; a pure function in web/src/*.ts
gets its node --test file and joins the test script in web/package.json;
git-board runs only commands from the toml or the code, never input from
the page; the package's own tests while working, make ci once at the end.
When a screen and the README disagree, the README's rule wins over the
screen's sample data.
```

## Build order

Four waves; inside a wave, everything can run at once. The plan's table has
the exact needs.

| Wave | Milestones | Why first |
| --- | --- | --- |
| 0 | the icons, the value tokens, the pure state functions (next step, branch state, forge state, bell, people), the unicode marks, the linter target, each file's EPFL fixes | nothing depends on anything; each unblocks the rest |
| 1 | the bar, the forge icons, the bell, the title row, the stepper, the fields line, the branch line, the people in the tab row | each needs only wave 0 |
| 2 | the popovers (forges, inbox, branch, create, labels), the forge states | each needs its wave 1 control |
| 3 | the side column retired, the branch words, git-board making branches, the folder names, the docs | wait on open points |

## Where things are

- The drawer: `web/src/components/IssuePanel.vue` (`.gb-ptop` is the bar, `.gb-ihead` the head), `web/src/pages/IssuePage.vue`.
- The side column today: `IssueSide.vue`; Development: `IssueBranches.vue`; the forges' sync: `IssueSync.vue`, `WaitingChanges.vue`, `web/src/issuesync.ts`, `web/src/forges.ts`.
- Icons: `GbIcon.vue` (paths), `ValueMark.vue` and `web/src/valueicons.ts` (value marks).
- The inbox's follow: `web/src/inboxcheck.ts` (`followLine`), `internal/inbox/rows.go` (`Follows`, what reaches the inbox), `internal/server/inbox.go` (`handleSetFollow`).
- Branches: `internal/branches/branches.go` (`Name`, `Slug`, `Suggest`, `Names`, `Linked`), `internal/config/config.go` (`Branches`), `internal/config/default.git-board.toml` (`[branches]`).
- The gate: `internal/ghpr/ghpr.go` (`Find`, the waits in `Gate`, `Open`, `Merge`), `internal/server/pulls.go`, `web/src/gate.ts`.
- Participants: `internal/store/thread.go` (`Participants`: the author, then each commenter).
- The cycle's branch: `internal/cycle/gather.go` (`gatherCode`: the newest branch not under `plan/`).
- Colors: `web/src/css/gitboard.css` (`--gb-<color>-*`), `web/src/colors.ts`; the EPFL tokens: `web/epfl-design/` (`web/README.md` says how it is vendored and linted).
- The canvas: "git-board · one issue, one plane?", rows 8 to 19 (private to guilbep; ask for access).

## How a person checks a milestone

Open #2771-like issues on a scratch clone (`CONTRIBUTING.md`, *Live tests*)
and tick the README's acceptance criteria of the milestone's screens; run
the adherence linter on the changed files and expect no new problem. Then
the usual: the package's tests while developing, `make ci` once at the end,
as `CLAUDE.md` says.
