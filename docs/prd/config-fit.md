# Onboard an existing GitHub project

PRD, 2026-11-11. A team that already has a GitHub repository, its
issues and a GitHub Project starts using git-board. On the first check,
git-board suggests the `.git-board.toml` that fits what the team already
has, so the first sync does not collide with it. **(user)** The team
also chooses what that first sync does, on one page. **(user)** Every
other decision is marked **(call)**: this document's, open to change.
The plan is [config-fit-m1-plan.md](config-fit-m1-plan.md).

No backward compatibility **(user)**: an older git-board does not need
to read a file this one writes, and there is no schema step for it.

## Why

A check (`git board project sync --check`, the GitHub Project tab's
Check) tells what a sync will do, and warns where the file and the
Project disagree. The fix is left to the person, who has to know which
key holds what. On co2-calculator the check of Project 40 said:

- **82 warnings in 915 issues.** 80 were `category value "SPECS" is
  none of [branches] kinds, skipped`: six Category values (SPECS 27,
  TASK 18, FIX 14, DATA 13, DOC 5, TEXT 3) that are no kind. 2 were
  `status value "Tests Metier" from the Project has no matching option,
  field left alone`, and the same for "Comité Spécialisé".
- **The file lost the team's statuses.** The default file had replaced
  co2's. It lacked three of the Project's nine statuses, 23 sprint
  dates and seven views. The status warnings came from that loss.
- **Fixing it by hand went wrong twice.** An agent took the
  `autorelease: pending` labels' leading space for a typo (it is
  release-please's label). It also took the `Must have` labels for a
  duplicate of the empty `moscow` field. Both "fixes" were reverted.

git-board holds what the right file is: the Project's options in their
order, its iterations and their dates, the kinds, and the labels the
issues carry. It suggests the changes and shows them as a diff. When
the person says so, it writes them.

## Part 1: the suggested file

### Decisions

| Topic | Taken |
| --- | --- |
| When | Every Project check computes the proposals. On the first check, before any issue has a sync base, they show as a step before Sync: "The file and the Project disagree: 12 changes clear 82 warnings." On a later check, they are one line that opens them. Sync is never blocked: "Sync anyway" runs it with the warnings. **(user)** for the first check, **(call)** for the rest |
| What a proposal can do | Add a value, a key or a mapping. It never removes an option, a label, a kind, a sprint, a view or a table, and it keeps the file's order. A one-line `options = [...]` with one more value is a changed line, so the rule is this: every value the old file reads is in the new file, in the same order. **(call)** |
| How it writes | The fit builds the new text with the `conf` editors and calls `app.SaveConfig` with the file's version at the check as `base`. The comments and the other lines stay. If the file moved since the check, the write is refused as stale. `SetConfigKeys` is not used: `conf.SetValue` writes a key twice under an inline table, sorts inline tables, and rewrites every `[[sprints.list]]` block. **(call)** |
| Ticked or asked | If git-board can derive a proposal with no guess, it is ticked. Otherwise it is a question with its choices and no default. Without an answer, nothing is written for it. **(call)** |
| Leaving a value unmapped | The answer goes in the file, so the whole team stops seeing it: `[github-project.unmapped] category = ["TASK"]`. The sync skips those values without a warning, and the check says "4 Category values unmapped by choice" once. **(call)** |
| Command line | `git board project fit` prints the diff of the ticked proposals, then each question with the flag that settles it, and writes nothing. `--apply` writes the ticked and the answered ones. `--map category:TASK=feat`, `--map 'status:QA Validation=qa'`, `--add-kind category:DATA` and `--unmapped category:SPECS` settle a question. With a terminal, `--apply` asks each open question. When there are proposals, `project sync --check` and `sync --check` end with `config: 12 changes clear 82 warnings: git board project fit`. **(call)** |
| Where on the web | The GitHub Project tab, where the Project's check is. It gets a panel: the proposals with checkboxes, the questions with their choices, and **Apply to .git-board.toml**, which shows the diff, writes, and checks again. The Sync tab gets one line that links to it until Part 2 joins the two pages. **(call)** |

### The proposals

| What git-board sees | The proposal | Ticked or asked |
| --- | --- | --- |
| An option of a Project single select that matches no option of the file, with or without a warning | Add the value to `[fields.<field>] options`, after the nearest option the Project lists before it. If `Slug(name)` is not the value, the Project's name goes in the field's mapping, in the form the field already uses, else `project`. Never a new `[forges.<name>]` table: one turns off finding the forges in origin's URLs. | The field's **stray values** are the values its labels carry on the board that are no option of the file. If `Slug(name)` is a stray value, or the field has none, the proposal is ticked with `Slug(name)`. Otherwise it is asked, with `Slug(name)` first, then the stray values. |
| A Category value that is no kind | An alias in `[fields.kind.aliases]`, `FIX = "bug"`. For a plain-label kind, the kind field's options are the keys of `[branches] kinds`. `checkAliases` reads them, and `planKind` reads the kinds, then the aliases, ignoring case. | A known synonym is ticked: FIX and BUG for bug, FEATURE for feat, DOC and DOCUMENTATION for docs, PERFORMANCE for perf, TESTS for test. A kind with or without a final "s" is ticked too. Otherwise it is asked, with three choices: map to a kind, add as a kind, or leave unmapped. |
| An iteration of the Project with no `[[sprints.list]]` entry | Add the entry: its title, its start date, and as slug the `sprint:` label its issues carry. Entries go in start-date order. | If the issues carry one slug for it, or none, the proposal is ticked. If they carry several, it is asked, and says how many labels it renames. |

Three things are not proposed. **(call)**

- A fold of a Project value into an existing option. The Project
  mapping does not read aliases.
- A numeric field's values. Any number is a known value, so nothing
  warns.
- A fix for a file that does not load. An old format is not supported.

On co2 the fit gives twelve proposals. Six are ticked: `tests-metier`,
FIX, DOC and three iterations. Six are asked: Comité Spécialisé, QA
Validation, SPECS, TASK, DATA and TEXT.

## Part 2: the first sync, chosen

Built after Part 1. **(user)** for the three needs, **(call)** for the
shape.

- **One page.** The GitHub Project tab and the Sync tab are one page,
  Sync. Its Check reads the forges and the Project together, and shows
  the forges' changes, the Project's changes and the proposals in one
  report.
- **Choose what the sync does.** The check's report counts what each
  part does: issues to pull that changed, new issues to pull, issues to
  create on the forge, changes to send. The person picks the parts the
  sync runs: pull the changes, pull the new issues, push nothing, or
  all. Each count opens its list of issues, so the choice is made on
  what happens. A part left out is left for the next sync.
- **Stop.** A Project check and a Project sync can be stopped, as a
  forge sync can (`DELETE /api/sync`).

### Decisions

| Topic | Taken |
| --- | --- |
| The parts | `pull-changes`, `pull-new`, `create` and `send`. The sync engine skips a part left out: its issues are not written and not journaled. The run records no run, so the next sync lists them again. **(call)** |
| A creation | What a sync sends after it creates an issue (its labels, its comments) is part of `create`. It counts there alone. When `send` is left out, it still goes with the creation. **(call)** |
| The Project's side | The Project reads its labels with a pull part. It writes to the Project with `create` or `send`. Without `create` and `send`, the sync writes nothing to the Project. The git pull and push are not parts: they always run. **(call)** |
| A release | A release of binaries is published with `send`. Without `send`, the sync publishes none. **(call)** |
| Command line | `git board sync --only pull-changes,pull-new`. One list flag, and no `--no-push`, because "push" already names the git push. `sync --check` ends with the count of each part. **(call)** |
| The web page | Check is the whole sync's dry run. Once it ends, it shows a box for each part with its count, and the issues under it. Sync runs the parts ticked. The GitHub Project is a section of the Sync page, with the fit and the conflicts. **(call)** |
| The Project's routes | The Project's own `check` and `apply` routes go. The whole sync checks and runs the Project, so Stop stops it. A resolve and a fit read the last check, and check nothing again. After a fit writes, the page asks for a new check. **(call)** |

### What the code forced

- **A new issue in a check has no number here.** The check runs on a
  copy of the board, so its new issues are on the copy alone. Each
  issue of a part is named by its forge number and title.
- **A creation that waits for consent has no list.** On a first sync
  without "Create issues", the forge holds the creations and sends one
  count. The page shows that count under the parts, with no issues.
- **The first check of a board with no issue matches nothing on the
  Project.** The forges' check runs on a copy, and the Project's check
  reads the board. Pull the new issues first, then check again: the
  Project's changes show then.

## Not in this PRD

- **Labels on issues that the file does not list** (lint's
  `label-outside`). Lint already says "add it there".
- **Changing the Project**, such as renaming its Category options.
- **Titles** with `[BUG]` (`kind-title`).
- **GitLab.** It has no Project.
- **A Category column with another name.** `Category` is a fixed name
  (`tools/ghproject/plan.go:534`).

## The tests

Part 2 has one test, `TestTheChosenSync`. It checks a board through the
forge fake and `projectfake`, then syncs the new issues alone. A second
check still counts the other parts. A Project check and a Project sync
then stop, and nothing is written to the Project.

Part 1 has one test, the co2 case end to end, through `projectfake`. The Project
has the nine statuses, the six Categories and three dated iterations.
The file is the default file. The test checks the twelve proposals, then
applies them with the six questions answered, then asserts four things:

- Every option, kind, sprint, view and comment of the old file is in the
  new file, in order.
- A second check has no warning, and says "2 Category values unmapped by
  choice".
- A write on a file that moved after the check is refused as stale.
- `git board project fit` without `--apply` writes nothing.
