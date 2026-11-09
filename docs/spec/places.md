# Where a value lives

The board is the one truth. Each forge and the GitHub Project is a
mirror of it: a copy of some of its values, on GitHub or GitLab. Every
write a mirror accepts ends in a recorded agreement, the value both
sides last held. This page says where each value lives, and what
happens when you switch a mirror on, off, or round. Each case names the
test that proves it. [Sync](sync.md) gives the run's rules, [Forges](forges.md)
what each forge needs, [Datastores](datastores.md) where the board is kept.

## The board and its mirrors

```
                     the board: issues, people, fields as ns:value
                     labels, in one datastore (board.datastore)
                          |
         ┌────────────────┼────────────────┐
     a GitHub forge    a GitLab forge    the GitHub Project
     its issues        its issues        its columns
```

- **The board's value is the truth.** A field's value is a label in the
  board (`status:ready`, ADR 0004). A pull writes a mirror's value into
  the board, and from then on the board holds it as its own. A mirror
  can be where a change starts; it is never the truth.
- **The board is your clone's.** Two clones meet through git (origin, a
  fetch, a push), not through a forge. Three repositories that share one
  forge have three boards; the forge is where they meet.
- **The datastores are homes, not levels.** git, PostgreSQL and Markdown
  hold the same board; a clone has one. The forge-only datastore is the
  exception: a board kept only as the forges' copy, with no store of its
  own (ADR 0002, section 4).
- **The mirrors are peers, not levels.** A forge and the Project each
  compare the board with themselves, never with each other. GitLab has
  no Project, so a field the Project holds on GitHub is a label on
  GitLab. One `git board sync` runs git pull, the forges, the Project,
  then git push.

## The three decisions

Where a value is follows from three decisions, and from nothing else.

| Decision | Where it is written | When it is not written |
|---|---|---|
| Where the board is kept | `board.datastore`, in the clone's git config (it can hold a password, so never in `.git-board.toml`) | git |
| Which forges, and which way | `[forges.<name>]` tables in `.git-board.toml`, with `way = "push"`, `"pull"` or `"both"` | the projects `origin` names, both ways |
| Where each field lives | `[forges.<name>.fields.<field>]` mappings, and `[github-project] fields` for the Project | the field type's default on that forge; for the Project, the default seven |

- **Which forges.** With no table, the forges are the projects `origin`
  names. Writing one table turns that off. A forge found this way never
  starts creating issues silently: a first sync with a forge, or 50
  creations or more, waits for consent (`--allow-create`, the Sync
  page's checkbox).
- **Which way.** `both` pulls and pushes. `pull` makes a read-only
  mirror: the run reads it, and writes nothing there. `push` makes a
  backup: the run writes to it, and pulls nothing; an edit made there is
  put back to the board's value at the next run, and counted.
- **Where a field lives.** On a forge, a field's home is its mapping's
  `to`, else its type's default: a label, the due date on GitLab, the
  assignees, sub-issues on GitHub, the milestone. On GitHub with a
  Project, the fields `[github-project] fields` names live in the
  Project's columns, not in labels. `project configure` and the Project
  page write that list with the Project. A table without it holds the
  default seven (`status`, `prio`, `mod`, `est`, `sprint`, `moscow`,
  `milestone`), and the Project page says the file does not state them
  yet. Settings › Fields shows the resolved home of each field on each
  forge.

## The agreement, and the two conflict rules

For each field, a run compares three values: the board's, the mirror's,
and the agreement. On a forge, the agreement is a `seen` record in the
forge's sync entity ([Last seen](sync.md#last-seen)); on the Project, it
is the issue's `git-board:<owner>/<number>:<field>` metadata.

- Neither side changed: nothing moves.
- One side changed: that side's value goes to the other.
- Both changed, differently, on a **forge**: the later change wins, the
  forge's at the same second. A forge's change is already public.
- Both changed, differently, on the **Project**: the field is held on
  both sides, named on the Conflicts page, until a person keeps one side
  (`git board project resolve`, or the page's buttons). The Project
  never wins by itself.
- Labels are sets, compared label by label, so two sides that add
  different labels both keep theirs.
- An accepted write is recorded at once, so a run that stops loses
  nothing and never writes the same change twice
  ([Crash recovery](sync.md#crash-recovery)).

### The reversibility guarantees

1. **An agreement survives its mirror.** Turn a forge or the Project off,
   and its records stay. Turn it on again, and a sync moves only what
   changed since.
2. **A held label comes back.** When a field stops living in the Project,
   it flows to GitHub as a label again, and the board's own change wins
   over a stale copy.
3. **A home moves only when you ask.** Changing a field's home moves no
   value by itself. `git board sync --migrate <field>` says what it would
   set and remove, then does it.
4. **A Project conflict is never won by itself.** It waits for a person.

## Switching, case by case

| You do | What moves | What stays | Proved by |
|---|---|---|---|
| Add a GitHub Project | its fields leave GitHub's labels and live in its columns; a pull fills the board from the columns | the board's labels; GitHub's old copies, neither pulled nor removed | `TestConfigureProject`, `TestHeldLabels`, `TestSyncFirstRunImports` |
| Hold fewer fields in the Project (`fields = ["status", "prio"]`) | the others are labels on GitHub again | the Project's records of every field | `TestConfigureProject` |
| Remove the Project | its fields flow to GitHub as labels again; the board's change since wins over a stale copy | the Project's records, for when it comes back | `TestHeldAfterSeen` |
| The board and the Project change one field differently | nothing: the field is held on both sides | both values, on the Conflicts page, until you keep one | `TestSyncConflictIsHeld`, `TestResolveKeepsOneSide`, `TestProjectRun` |
| The board and a forge change one field differently | the later change; the forge's at the same second | the agreement, updated | `TestPullKeepsLocalChange` |
| Move a field's home on a forge (a label to the assignees, say) | nothing, until the migration runs; then the old labels go | the old target, read until then | `TestMigrateField` |
| Make a forge a backup (`way = "push"`) | from then on, only the board's values go there; an edit made there is put back | its records | `TestWayPushOnly`, `TestWayBackupPutBack`, `TestWayBackupNeverFeedsTheBoard` |
| Make a backup both ways again | what is edited there from then on is pulled; no issue is created twice | its records, so it is not a first sync | `TestWayBackupBecomesBoth` |
| Make a forge read-only (`way = "pull"`) | its issues are pulled; nothing is written there | its records | `TestWayPullOnly`, `TestWayRuns` |
| Keep the board in Markdown, from git | Copy, then Keep the board here: every issue, person and record, ids kept | the board in git, untouched | `TestDatastoreCopy`, `TestDatastoreChoose` |
| Keep the board in git again, from Markdown | the key is unset, and the clone reads git | the Markdown folder; what was written only there is not copied back | `TestDatastoreChoose`; the copy back is **not proven** |

## The use cases

| You want | The config | What happens |
|---|---|---|
| 1. A backup of the issues in `github.com/org/repo` | `[forges.backup]` with `way = "push"`, or Settings › Sync's way select | the run writes there, pulls nothing, and puts back what is edited there |
| 2. Code in `org/x`, issues in `org/issue-repo` | a `[forges.issues]` table naming `org/issue-repo` | works: a table names any project |
| 3. Repositories `x`, `y`, `z` sharing one issue repository | each repository's table names `org/issue-repo` | works: each board links to the same forge issues; between two boards, the later change wins |
| 4. `x`, `y`, `z` sharing one GitHub Project | each `[github-project]` names the same Project | works per issue: each board bridges its own repository's items |
| 5. A GitHub Project into a PostgreSQL board | `board.datastore = postgres://…`, `[github-project]` set | works: the bridge does not depend on the datastore |
| 6. Code on GitHub, issues on GitLab | `origin` on GitHub, a `[forges.gl]` table naming the GitLab project | works; the table also stops `origin` being found as a forge. `way = "pull"` makes GitLab read-only |
| 7. Issues in PostgreSQL or S3, not in git | `board.datastore = postgres://…` | PostgreSQL works. S3 is **not built**: a store would implement the `Datastore` interface (heads, load, add, versions) |
