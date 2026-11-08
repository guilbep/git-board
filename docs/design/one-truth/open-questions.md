# One truth, many mirrors: the open questions

This page prepares the session of 2026-11-08. On 2026-11-07 the spec
gained the page [Where a value lives](../../spec/places.md), and
[ADR 0022](../../adr/0022-one-truth-many-mirrors.md) accepted the
invariant (a rule that holds at every moment). This page holds the
questions that the invariant does not answer yet. Each question lists
its options and a proposal. The session can then decide, and does not
derive the model again.

This page builds on ADR 0007 (each field is a label), and on ADR 0008
(sync state lives in the data model). It also builds on ADR 0016
(project numbers come from one counter), and on the
[four cases](../../spec/sync.md#the-four-cases) of the sync spec.

## Decided on 2026-11-07

- ADR 0022 states the invariant. The board is the only source of truth.
  Each forge and the GitHub Project are mirrors (a mirror is a copy of
  the values of the board, on GitHub or GitLab). Each write that a
  mirror accepts ends in a recorded agreement (the value that both
  sides last recorded as equal). The configuration states each
  behavior of a mirror.
- The spec gained the page Where a value lives. The lead of the Sync
  page and of the Project page states the model in one sentence.
- The Project bridge reads tokens in this order. First the posting
  account of the first GitHub forge table. Then your own account on
  the host. Then any account on the host. The code and the guide state
  this order.
- ADR 0022 commits to three changes that are not built. The changes
  are: a direction key for each forge, the Project field homes written
  into the forge tables, and an S3 datastore.

## Q1. What the invariant says, word for word

The phrase is the headline of ADR 0022. People will quote it alone, so
it must hold alone. Proposal for a definition:

> A field value lives where the board last recorded an agreement about
> it. A write to a mirror is the translation of a value that the board
> holds. Or a pull makes the value of the mirror a value that the board
> holds. No mirror is the truth for the board. A mirror can be the
> source of a change first, and the board holds that change at the next
> pull. Before the pull, the board is behind. It is not wrong.

The definition commits to three statements. Confirm each tomorrow.

- A board that does not pull holds a stale value, and not a false
  value. The agreement makes the difference measurable, because the
  four cases compare against the agreement, and not against the age of
  the board. Nothing overwrites a newer value of the mirror.
- The word board means the board of your clone (a copy of the
  repository on one machine), and not one board of the world. Use case
  3 has three boards that share one forge. Each board is the truth of
  its clone, and the forge is the meeting point. Write this meaning
  into ADR 0022 if the session confirms it.
- Authorship is not truth. A value pulled from the Project is first the
  value of the Project. After the pull, the board holds it as its own,
  and the base (the agreement of the board with the Project) records
  it. The Project is the source of the change. It is never the source
  of truth.

Proposal: add the definition to ADR 0022, in a section with the name
What the invariant says. State the per-board meaning in the same
section.

## Q2. Three conflict laws, one model

The model resolves a conflict (a field that two sides changed) with
three laws. Between clones, the order wins, and the order is the same
on every clone. Between the board and a forge, the later change wins.
Between the board and the Project, the field waits on both sides until
a person keeps one side.

- The Project waits, and the forge does not. A write to a forge is
  already public. A waiting forge change leaves a notification with no
  value change. A write to the Project is internal, and waiting costs
  only a line on the Conflicts page. Keep this difference, and state it
  at the four cases: the law follows from the audience of the mirror.
- The Project never moves to later-wins. Options: keep wait-only, or
  add a setting `conflict = "later"` for a field or a board. The
  setting removes the guarantee that the model never auto-wins a
  conflict. Proposal: keep wait-only, and revisit when the Conflicts
  page shows real conflicts.
- The tie of the later change: at the same second, the forge wins,
  because its change is already public. The Project gives no time for a
  field change, because a Check run reads values, and reads no events.
  The wait rule needs no tie-break, because it compares no times.

## Q3. A mirror that only receives: the direction key

Use case 1 wants a backup (a forge that receives the issues, and sends
nothing). The design proposes a forge table with `way = "push"`. A push
creates and updates issues. Nothing pulls from the backup. An edit made
on the backup is drift (a change on the mirror that the board did not
make). The run reports drift, and does not take it as truth.

The key opens these questions. Answer them before the build.

- Is a backup mirror a link like the others? A link records a seen
  value for each write. A push-only forge then holds complete records,
  and the model needs no new kind of record. Proposal: yes. The key
  `way` filters the directions of the run, and changes nothing else.
- Does a forge without pulls still record a run? A push records a run
  today. The forge then lists its issues from the last run. The missing
  pull is missing by design, and not by an omission in the journal. No
  change.
- What does the Sync page show for a push-only forge? Options: no Check
  button, or a Check that counts the sends only. The row shows the
  words push only. Decide these words with the design of the Sync page.
- The value `way = "pull"` gives a read-only mirror: the sync reads its
  issues, and writes nothing back. Use case 6 with private-forge limits
  needs this value. The same milestone carries both values.

Proposal: build `way` as one key with the values `push`, `pull`, and
`both`. Add the words of the Sync page in the same milestone. This
change removes the last state that the configuration does not state:
the direction of a run.

## Q4. Write the Project field homes into the forge tables

Today, `[github-project]` decides where seven fields live on GitHub.
The forge table `[forges.gh]` says nothing about it. This is the one
implicit state that ADR 0022 commits to remove. The proposal: the
configuration of a Project writes the field mappings
(`to = "project"`) into the tables of the GitHub forges. After that,
the code reads the mappings alone, and the special Project clause goes.

The change opens these questions.

- Who writes the mappings of an old board? Three candidates: the next
  `project configure`, a `sync --migrate`, or a save in Settings.
  Never write them at the open of a board. Until the migration, the
  rule of today holds, and the pages show the resolved homes. Decide
  who writes. State one consequence with it: a new forge table turns
  the discovery of forges off.
- The key `write = true` is not a home. The Project holds the field
  with or without `write`. The key `write` decides the direction of
  the bridge only. Keep the two apart in the words of the file.
- GitLab has no Project, and the defaults stay.

Proposal: the command `git board project configure`, and its API
route, write the mappings. Show a preview of the move, as for each
move of a field home.

## Q5. Shared mirrors: filters, and who resolves

Use cases 3 and 4 share one mirror across repositories. The model
answers them per issue, and does not answer these questions yet.

- A board can want only its own issues from the shared forge. A filter
  on the forge (for example `issues = "label:x"`) makes the sync pull
  and push only the issues of the filter. This is not built, and
  ADR 0022 does not commit to it. Decide if the want is real before
  the design. The alternative: each repository keeps its own issue
  repository, and the repositories share only through the Project.
- Two boards, one Project, one conflict: each board records its own
  base, holds its own conflict, and resolves its own. Two boards can
  keep different sides, and the Project takes the last resolve. The
  Project holds one column, and the four cases decide per board, in
  turn. Confirm that this is acceptable.
- Deletions are never propagated (a delete hides the issue here, and
  keeps it on the forge). On a shared forge, three teams see this
  behavior. Confirm that it is still the want.

## The goals, freshened on 2026-11-08

Three inputs arrived after the session of 2026-11-07. Each fits the
invariant, and each raises the order of one piece.

- People sync between GitHub and GitLab from the command line, and the
  way must be easy. GitLab is the backup. This is use case 1 and use
  case 6 together: GitHub runs both ways, and the GitLab mirror
  receives with `way = "push"`. The easy path is one command that adds
  the backup forge and sets its way, plus the Tokens section for the
  token. `git board forge status` says what is missing, in words.
- People choose where the board is kept when they open the app for the
  first time. Today the choice is `board.datastore` in git config, and
  the Datastore page states it. The want is a question at the first
  open, with git as the default. This is decision one of the three,
  made visible.
- The long-term goal is the hosted service of
  [issue 1](https://github.com/guilbep/git-board/issues/1): one server
  serves every repository of the organization, behind EPFL login, with
  git as the only store for issues, and no issue database. The hosted
  server runs one board per repository on the same app, so the model of
  this page holds unchanged, and the datastore choice above stays
  open per board. The pilot of issue 1 lists its first steps.

## Q6. What is built, in what order

| Piece | Serves | Size |
|---|---|---|
| The definition section in ADR 0022 | the wording of the invariant | an edit |
| The key `way` for each forge | the GitLab backup, use case 1 and 6 | the configuration, the engine, the words of the Sync page |
| One command that adds a backup forge | the easy command line of 2026-11-08 | a command on the forge commands |
| The datastore question at the first open | decision one, made visible | a first-run step in the app, git as the default |
| The Project homes written into the forge tables | the last implicit state | the configure command, the migration, and the removal of the Project clause |
| A filter for the issues of a forge | the shared forge | the configuration and the engine, only if Q5 finds the want |
| An S3 datastore | use case 7, and the hosted service | an implementation of the `Datastore` interface, a project of its own |

Proposal for the session: decide the words of Q1 first. Then build the
`way` key and the one command, because the GitLab backup waits for
them. Then the datastore question at the first open. The hosted
service of issue 1 runs on its own plan, and the pilot uses what this
page decides.
