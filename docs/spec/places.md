# Where a value lives

The board is the one truth. Every forge and the GitHub Project are
mirrors, and every write a mirror accepts ends in a recorded agreement.
This page states the model those words name: the three decisions that
place a value, the agreement that makes a mirror reversible, and the
rule that replaces each behaviour the config leaves unwritten. [Sync](sync.md)
gives the run's rules; [Forges](forges.md) gives what each forge needs;
[Datastores](datastores.md) gives where the board is kept.

## One truth, many mirrors

```
                     the board — issues, people, fields as ns:value
                     labels — in one datastore: git, PostgreSQL or
                     Markdown files (board.datastore)
                          |
         ┌────────────────┼────────────────┐
    the forge mapper  the forge mapper  the project mapper
    board ↔ GitHub    board ↔ GitLab    board ↔ the GitHub
    issues            issues            Project ([github-project])
```

- **The board's value is canonical.** A field's value is a namespaced
  label in the board (`status:ready`, ADR 0007), whatever the mirrors
  show. A mirror never becomes the truth by being written to: a pull
  writes the mirror's value into the board, and from then on the board
  holds it as its own.
- **The datastores are not levels.** git, PostgreSQL and Markdown are
  alternative homes of the *same* board; a clone has exactly one
  (`board.datastore`). Two clones converge through git — origin, a
  fetch and a push — not through a forge. The forges alone mode
  (`store/forgecopy`) is the one exception: a board kept only as the
  forge's mirror, with no datastore of its own.
- **The forges are peers, not stages.** GitLab has no Project, so a
  field that lives in the Project on GitHub lives in a label on GitLab.
  Neither is "closer to" the board; each mirror shows what it offers.
- **The Project bridge reads no forge.** It compares the board's label
  with the Project's column, and is written to by no forge step. In one
  run (`git board sync`) the steps order git pull → the forges → the
  Project → git push, and each compares the board with its own mirror.

## The three decisions

Where a value is, and where a change goes, follows from three decisions
and nothing else. Each has a place in `.git-board.toml`, and each is
stated where it is decided.

| Decision | Where it lives | Unwritten |
|---|---|---|
| where the board is kept | `board.datastore` (git config) | git |
| which forges, and which way | `[forges.<name>]` tables | those `origin` names, both ways |
| where each field lives on a forge | `[forges.<name>.fields.<field>]` | the type's default, on that forge |

- **Which forges.** The forges are the tables written, else — with no
  table at all — the projects `origin` names, its fetch URL and every
  push URL. A table turns the finding off. Discovery is a default, not
  a hidden channel: adding a remote never silently starts a sync that
  writes, because creations on a forge never synced with, or 50 or more
  in one run, wait for consent (`--allow-create`, the Sync page's
  checkbox). A forge found in the remotes syncs both ways, with no
  login of its own.
- **Which way.** A run is a pull, a push, or both, chosen each time the
  run starts (`git board forge pull|push|sync`, the Sync page's Check
  and Sync). There is no per-forge direction in the config today; the
  way a mirror may write is [a proposed key](#the-implicit-states-and-their-rules)
  (`way = "push"`), not a built one.
- **Where a field lives.** On each forge, a field's home is its
  mapping's `to`, else the type's default for that forge's kind:
  a single select is a label, a date a due date on GitLab, a person the
  forge's assignees, an issue-typed field sub-issues on GitHub, a
  milestone the forge's own. Each target a forge offers — GitLab's
  scoped label, the Project column, the weight — is a mapping's `to`
  away. On GitHub with a Project, the Project's own fields (`status`,
  `prio`, `mod`, `est`, `sprint`, `moscow`, `milestone`) live in the
  Project, and a field a `project` mapping names lives there too.
  Settings › Fields, **Where it lives**, shows the resolved home of
  every field on every forge; a forge's row in Settings › Sync says the
  same, in one line.

## The agreement

Every mirror write ends in a record of what both sides last agreed on.
That record is what makes a mirror reversible: nothing is ever moved
again because it was moved once.

- **A forge's record** is a `seen` operation in the forge's sync entity,
  per field (title, body, status, labels, assignees, each comment).
  [Last seen](sync.md#last-seen) gives its format.
- **The Project's record** is the issue's metadata,
  `git-board:<owner>/<number>:<field>` — the base.
- **Both serve the same four cases.** For each field, the board's value
  (G), the mirror's (M) and the agreement (S) are compared: neither
  changed, nothing; the mirror changed, it is pulled; the board changed,
  it is pushed; both changed, the later change wins on a forge, and on
  the Project the field is **held on both sides** until
  `git board project resolve` keeps one. The table is
  [the four cases](sync.md#the-four-cases); the bridge's statement is
  `tools/ghproject`'s header.
- **Labels are sets, compared per label,** on both the forges and the
  Project, so changes on both sides merge and cannot conflict.
- **A write that the mirror accepts is recorded at once** — a run that
  stops then loses nothing, and never writes the same change twice
  ([Crash recovery](sync.md#crash-recovery)).

### The reversibility guarantees

Four rules follow from the agreement, and they are the whole of the
model's reversibility:

1. **An agreement survives its mirror.** Unconfigure a forge or the
   Project, and its records stay. Reconfigure it — the same project, or
   a new one — and the records still say what both sides agreed on; a
   sync moves only what changed since.
2. **A held label comes back.** When a field's home on a forge stops
   being the Project (or any non-label target), the field flows to the
   forge as a label again. Its old labels are read as a fallback until
   a migration moves them (`git board sync --migrate <field>`, with a
   preview of what it would set and remove).
3. **A home's move is explicit and previewed.** Changing where a field
   lives moves nothing by itself; the old target is read until the
   migration runs, and the migration says what it would write and
   remove before it does.
4. **A conflict is never auto-won.** Both sides changed, differently:
   the field is held on both, named on the Conflicts page, and moved
   only by a resolve that a person asked for.

## The implicit states, and their rules

Nothing in the config should decide something the config doesn't say.
Each behaviour below was reached without being written; each now has a
rule that states it, and where the rule is stated. Where a rule is not
enough — where the state itself should become written — the page says
so, with its state.

| Implicit state today | Its rule | Written down where | To write |
|---|---|---|---|
| A forge exists because `origin` names it | the forges are the tables, else origin's projects; a table turns the finding off | [Which forges](#the-three-decisions), the guide's Adding a forge | — |
| A found forge syncs, both ways, with no login | discovery's default; creations wait for consent | [Which forges](#the-three-decisions) | `way` per forge, for a mirror that only receives (a backup) or only sends |
| A field lives in the Project on GitHub because `[github-project]` is there, not because a mapping says so | the Project's own fields live there; `Held` keeps their labels off GitHub | [Where a field lives](#the-three-decisions), `forge.Target` | configure the Project by writing its fields' mappings into `[forges.<name>.fields]`, so no table's presence decides another table's behaviour |
| A host no table names still gets the type's defaults | a synthetic table, its kind from the host's first label | `forge.ForgeOf` | — |
| The Project bridge reads the token of the first GitHub table's posting login, else yours on the host, else any account | the fallback chain, in order, each step named | [app/project.go](../../app/project.go)'s header, the guide's Tokens | — |
| A run's direction is how it was started | pull, push and sync are the three ways; nothing persists one | [Which way](#the-three-decisions) | the same `way` key |
| The Project is read until `write = true` | the default, in the table's docs | [ConfigureProject](../../app/project.go), the guide | — |
| The numbers' counter is on origin, or in the datastore | ADR 0016 states where, and how a clone finds it | [ADR 0016](../adr/0016-project-numbers-are-a-counter-on-origin.md) | — |

The one state the model should outgrow is the Project's. Today,
configuring `[github-project]` changes where seven fields live on
GitHub — and which labels GitHub holds — while `[forges.gh]` says
nothing of it. The rule is stated and the pages show the resolved
homes, but the elegant shape is the written one: adding the Project
writes the mappings it implies, and from then on the file says every
home, the Project's presence among them.

## The use cases

Each case below is stated as the config that expresses it, and what the
model answers. Where today's config is enough, the row says so; where a
key from the table above is wanted, it names it.

| A user wants | The config | The model's answer |
|---|---|---|
| 1. to back its issues up in `github.com/org/repo` | a `[forges.backup]` table naming the repo, run `forge push` | works, by running the push alone; nothing pulls back. A persisted `way = "push"` would make the backup a property of the forge, not of the command typed |
| 2. in `github.com/org/x`, issues in `github.com/org/issue-repo` | a `[forges.issues]` table naming `org/issue-repo` | works: a table names any project, `origin` aside |
| 3. in `x`, `y`, `z`, issues shared in one `issue-repo` | each repo's table names `org/issue-repo` | works, as a shared bus: each clone's board links to the same forge issues; a change either side pulls or pushes by the four cases; two boards changed the same field, the later wins |
| 4. `x`, `y`, `z` sharing one GitHub Project | each repo's `[github-project]` names the same Project | works per issue: Project items name their repository, and each board bridges its own; fields shared through the Project's columns reach every repo's board. A Project's field on an issue another repo owns is not that board's to write |
| 5. from a GitHub Project into a PostgreSQL board | `board.datastore = postgres`, `[github-project]` set, `project pull` | works: the engine is datastore-agnostic; the Project step runs on PostgreSQL as on git |
| 6. code in GitHub, issues in GitLab | `origin` GitHub, a `[forges.gl]` table naming the GitLab project | works; without the table, discovery would mirror to GitHub too — the table is the explicit no |
| 7. code in GitHub, issues in a PostgreSQL or S3 remote | `board.datastore = postgres` | PostgreSQL works; the issues travel with the datastore's address, not with git. S3 is not built: the `Datastore` interface (heads, load, add, versions) is what a store on S3 would implement |

The cases the model answers *as configured* share one property: they
change none of the three decisions' meanings. The ones that want a new
key — the backup's `way`, an S3 store — want a decision written that
today is reached by not writing one.
