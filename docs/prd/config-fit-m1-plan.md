# Onboard an existing GitHub project: plan

**Spec:** docs/prd/config-fit.md. One branch, `feat/config-fit`, a
commit per task. Part 1 has one test, `TestTheCo2Fit`. Part 2 has one
test, `TestTheChosenSync`, and its own section at the end.

**Status:** built. Each task says what it changed.

## Global constraints

- `tools/ghproject` imports no `conf` and no `app` (`layers_test.go`).
  The proposals are computed there. The file's text is edited in `app`.
- A proposal adds. It removes no option, no label, no kind, no sprint,
  no view and no table, and it keeps the order of what the file has.
- Every write ends in `app.SaveConfig(dir, text, base, dry)`. It checks
  the file. If the file moved since `base`, it refuses the write as
  stale.
- A question has no default answer. Without an answer, nothing is
  written for it.
- No backward compatibility: no schema step, no fix for an old file.
- No live call at an EPFL repository. The test uses
  `tools/ghproject/projectfake`.

## Settled by the user

The user took every point the first plan raised against the first PRD.
The PRD now says them.

1. The writer is `SaveConfig` with the `conf` editors, not
   `SetConfigKeys`.
2. "Nothing removed" means every old value is in the new file, in order.
3. The panel is on the GitHub Project tab. The Sync tab links to it.
4. Sprint and status proposals come from the Project, not from a
   warning. A numeric field gets none.
5. No fold of a Project value into an existing option.
6. A status is ticked by the board's stray values.
7. A Category maps to a kind in `[fields.kind.aliases]`.
8. The co2 case is twelve proposals: six ticked, six asked.

## Tasks

### Task 1: typed warnings

- `tools/ghproject/plan.go`: `Warning{Kind, Field, Value, Text}` and the
  five kinds. `Change.Warnings` is `[]Warning`.
- The CLI prints `w.Text`. The API still answers the texts.

### Task 2: the Project's order

- `tools/ghproject/plan.go`: `Field.Choices`, in the Project's order,
  beside `Options`.

### Task 3: the iterations' dates

- `forge/github/projectv2.go`: the query reads `startDate duration`.
  `ProjectChoice` gains `Start` and `Days`.
- `projectfake`: `Choice` gains the same two.

### Task 4: the kind's aliases

- `domain/issues/check.go`, `checkAliases`: for a plain-label kind, the
  options are the keys of `[branches] kinds`.
- `tools/ghproject/plan.go`, `kindNamed`: the kinds, then the aliases,
  both in any case. `planKind` calls it.

### Task 5: values unmapped by choice

- `tools/conf.go`: `GithubProject.Unmapped`, key `unmapped`.
- `app/project.go`: `Config.Project` passes it on. `ConfigureProject`
  writes the table again, and keeps it.
- `tools/ghproject/plan.go`: `syncIssue` skips a listed value with no
  warning. `Plan.Unmapped` records it once.
- `Plan.UnmappedLines`: `2 Category values unmapped by choice`. The CLI
  prints it, and the API answers it as `unmapped_by_choice`.

### Task 6: the editors

`conf/sections.go`:

- `InsertInList`: a value in a one-line array, after another value or
  at the end. It refuses an array over several lines.
- `AddInline`: one entry more in an inline table, the other entries as
  they were. If the file has a `[table.key]` table, it writes a line there.
- `AddArrayTable`: a `[[key]]` block after the last one, else after its
  parent table. The comments over the next table stay with that table.

### Task 7: the proposals

`tools/ghproject/fit.go`:

- `Proposal`: its id (`status:Tests Metier`), what it says, whether it
  is ticked, its choices, the warnings it clears, and what it writes.
- `Fit(s State, plan Plan, c *Config) []Proposal`. It is pure.
  - **An option.** Each option of a Project single select that `toLabel`
    matches to no option of the file. It goes after the nearest option
    before it that the file has, or after a proposal written before it.
  - **A Category.** A synonym, or a kind with or without a final "s", is
    an alias. Any other is a question.
  - **An iteration.** Each one with a start date and no
    `[[sprints.list]]` entry, by date, with the slug its issues carry.
  - **An issue not pulled yet.** `Fit` plans each issue of the Project
    that the board lacks, and counts its warnings. The first check of an
    empty board gives the same proposals. An issue of another repository
    than the forge table's does not count.
- `Config.Answer(p, answer)`: `map=<value>`, `add-kind`, `unmapped`.
- `Report.Fit`, `Report.First` and `Report.FitLine`, filled by `Run`.

### Task 8: the write

`app/fit.go`:

- `Board.Fit(report, take, answers, dry) (Fitted, error)`. It answers
  the questions, builds the text with the editors of task 6, makes the
  diff, and calls `SaveConfig` with the check's file version.
- A Project name goes in the form the field has: the forge's `values`
  when the mapping has some, else `project`.
- `app/project.go`: `ProjectRun` sets `Report.ConfigVersion`.
  `ProjectNote` ends a check with the fit's line.

### Task 9: the command line

`surface/cli/fit.go`, `git board project fit`:

- Without `--apply`: the diff of the ticked proposals, each question
  with its flags, and `checked: nothing written`.
- `--apply`, with `--map`, `--add-kind` and `--unmapped`. With a
  terminal, it asks each open question.
- `project sync --check` and `sync --check` end with the `config:` line.

### Task 10: the API

`surface/http/project.go`:

- `GET /api/github-project` answers `fit`, `first` and `config_version`
  in `last`.
- `POST /api/github-project/fit`, `{take, answers}`, `dry_run`. It
  writes from the check the server keeps, through the code gate and
  `writeConfig`, then checks the Project again.
- With no check, or a file that moved, it answers 412.

### Task 11: the GitHub Project tab

- `web/src/components/ProjectFit.vue`: the proposals as checkboxes, the
  questions as menus with no answer chosen. **Apply to .git-board.toml**
  shows the dry run's diff, then writes.
- It is open on a first check, and one line on a later check.
- `web/src/components/SyncSettings.vue`: one line with a link to it.
- `web/src/projectview.ts`: `fitLine` and `answerWords`.

### Task 12: docs

- `docs/guide/cli.md`: `project fit`.
- `docs/guide/configuration.md`: `[github-project.unmapped]` and the
  kind's aliases.
- `docs/rewrite/api.md`: route 68.

## The test

`TestTheCo2Fit`, in `surface/cli/fit_test.go`, through `projectfake`.

- The forge: 80 issues with a Category and no kind, one on `Tests
  Metier`, one on `Comité Spécialisé`, and one off the Project. The last
  three carry `status:tests-metier`, `status:comite` and `status:qa`.
- The file: the default file, with a `[forges.gh]` table and
  `[github-project]`.
- It checks these, in order:
  1. With no issue on the board, `sync --check` counts 83 new issues
     and ends with `config: 12 changes clear 82 warnings`. Eight
     proposals are ticked and four are asked.
  2. Without `[github-project]`, `sync --only pull-new` pulls the 83
     issues with their status labels, and writes nothing to the forge.
     The next `sync --check` counts nothing in any part.
  3. The check has 82 warnings and ends with `config: 12 changes clear
     82 warnings: git board project fit`.
  4. The twelve proposals, six ticked and six asked, with what each
     clears.
  5. `project fit` prints the diff and the questions, and the file is
     byte for byte the same.
  6. A fit on a file that moved after the check is refused as stale.
  7. `project fit --apply` with the six answers writes twelve changes.
  8. Every line of the old file is in the new one, in order. The
     statuses, the kinds, the aliases, the sprints, the unmapped values
     and the views read as expected.
  9. The second check has no warning, and says `2 Category values
     unmapped by choice`.
  10. `project configure` writes the table again. The unmapped values
      stay, and the file still loads.

## Cut

- The fix for a file that does not load, with its flag, its API field
  and its button.
- A typed value in the web panel. A question offers its choices. The
  command line takes any value with `--map`.
- Tests of the web panel and of the route. One test is the rule.

## Risks that stay

- **The fit writes what lint flags.** A Project name in `[fields.<f>]
  project` is a `project-key` finding (`app/lint.go:348`).
- **An array over several lines.** The fit refuses it and writes
  nothing. The person adds the value by hand.
- **A new kind's label.** `--add-kind` writes the kind. Its label on the
  forge is `lint --create-labels --apply`'s to make.

## Part 2: the first sync, chosen

**Status:** built.

- **The engine.** `issuesync.Options.Only` holds the parts a run writes.
  A part left out skips its issues. The run records no run, so the next
  one lists them again. `Did` names each issue by its forge number and
  title.
- **The app.** `app.OpenForge` opens a forge, and a test gives it a
  fake. `ForgeRun.Only`, `Whole.Only` and `app.ParseParts` carry the
  parts. `Whole.Project` hears the Project's report. A Project apply
  stops between two issues once its context ends.
- **The command line.** `git board sync --only <parts>`. `sync --check`
  ends with the count of each part.
- **The API.** `POST /api/sync` takes `dry_run` with `scope: all`, and
  `only`. `GET /api/sync` answers each part's issues in `run.parts`. The
  Project's `check` and `apply` routes go. The resolve route stays. A
  resolve and a fit check nothing again.
- **The web.** `SyncSettings.vue` holds the whole page. `ProjectPanel.vue`
  replaces `ProjectSettings.vue` as its section. `SyncParts.vue` shows
  the parts with their issues. The `project` tab is gone, and its old
  links open the Sync page.

### The test

`TestTheChosenSync`, in `surface/http/chosen_test.go`:

1. A whole check counts 1 change to pull, 1 new issue, 2 creations and
   1 change to send. It reads the Project, and writes nothing.
2. A sync with `only: [pull-new]` pulls the new issue. It writes nothing
   to the forge or to the Project.
3. A second check still counts the change to pull, the creations and
   the change to send.
4. A sync with `only: [create]` creates the two issues. It writes to no
   issue that the forge already has. The next check counts no creation.
5. A sync with `only: [send]` sends the change. The title changed on the
   forge stays as the forge has it, and the next check still counts it.
6. A Project check and a Project sync each stop on `DELETE /api/sync`.
   Nothing more is written to the Project.
7. The stopped sync ran the forges whole. The forge still has five
   issues, and a last check counts nothing in any part.

### After the review

- **A release goes with `send`.** The release mapper ran whatever the
  parts were. It now runs with `send` alone.
- **A check goes on past a failed fetch.** `SyncWhole` stops after a
  failed pull in a sync alone.
- **The parts picked go with their check.** `SyncSettings.vue` sends
  `only` while the whole check shows its boxes. A new check ticks every
  part again.
- **Check and Sync wait for the status.** Before `GET /api/sync`
  answered, a click did nothing.
- **The browser.** `make web-e2e` runs, with `project.spec.ts` on the
  page's own Check. A manual pass covered the check, the fit's Apply,
  the choice of parts, Sync and Stop.

### Not exercised

- No test runs a release with `--only`. The rule is one line in
  `app.SyncForges`.
- The manual pass served the parts, the Sync and the Stop from a mocked
  `/api/sync`. The e2e fake GitHub answers no issue.
