# Brief: the cycle is the project's own

A lighter handover than `docs/design/time-flow/`: the PRD is drafted
(`docs/prd/project-cycle.md`), the two screens are here, and the agent's
first deliverable is the M1 plan, after the user has confirmed the PRD's
proposed decisions.

## What to paste to the agent

```text
Read docs/prd/project-cycle.md, then open the two files in
docs/design/project-cycle/screens/ (plain HTML; read the markup). Read
docs/prd/cycle-protocol.md and docs/design/time-flow/README.md for what
this builds on; web/src/cycle.ts, internal/server/cycle.go and the
[cycle] reading in the config package for what exists today.

Then, in plan mode:

1. Go through the PRD's "Decisions, proposed" table and its open points
   with me, one line at a time, and rewrite the table as taken decisions
   (dated today) with my answers. Keep the file's style; don't add
   sections.
2. Write docs/prd/project-cycle-m1-plan.md for milestone M1 (the list,
   the three reads, the subsequence rule in lint, the rail and the
   segments from the list, cycle:step ids with the "1" to "7" read), in
   the style of the other *-m1-plan.md files, task by task with
   checkboxes, each task naming the files it touches and its test.
   Risk class: structural, since it changes what cycle:step stores and
   how [cycle] is read; the plan goes through its own PR first.
3. Stop and ask me to approve the plan before writing code.

Rules that hold throughout: every existing .git-board.toml keeps working
unchanged (the three reads are not optional); a project with no steps key
behaves exactly as today; gates are the kind's and never move with the
list; the ordinal is never stored; the step is never typed; nothing here
touches sync. When a screen and the PRD disagree, the PRD wins.
```

## Where things are

- The step and its gates today: `web/src/cycle.ts` (`steps`, `gate`,
  `LOCKS`, `forStep`), `web/src/gate.ts`, `internal/server/cycle.go`
  (writes `cycle:step`), `internal/server/issues.go` (the comment
  endpoint writes `strconv.Itoa(req.Step)`).
- The rail and the workspaces: `web/src/components/IssueStep.vue`,
  `CycleIntent.vue` … `CycleVerify.vue`.
- `[cycle]` today: `docs/guide/configuration.md` (the `[cycle]`,
  `[cycle.kinds]`, `[cycle.signoffs]`, `[cycle.tests.*]`,
  `[cycle.checks.*]` tables), and wherever the config package reads
  them.
- The rail and segments of the time flow: `docs/design/time-flow/README.md`
  and its screens 6c and 6g.

## How a person checks M1

A test project with `steps = ["code", "review"]` shows two cells and two
segments; co2-calculator's file, untouched, shows the seven; `git board
lint` refuses `["review", "code"]` and says why; a comment written in
review carries `cycle:step = review`; an issue with old comments carrying
`"6"` shows them in the review segment. Then `make ci` once at the end.
