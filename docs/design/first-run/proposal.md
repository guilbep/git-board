# Named states, a first run, and fewer ADRs

- Status: accepted (2026-11-09, Pierre). Built on 2026-11-09: `app.State`,
  `git board state`, `GET /api/state`, the Start page, and the ADR merge.
  The named states replace the old checks only where this page says so.
- Scope: the implicit state machines, the first run in the CLI and the
  web app, what is missing, and a merge of the ADRs.

## The decision: one computed state

An implicit state machine is a lifecycle that no type names. The code
infers its state from booleans, files, refs and git config keys. The
survey found eleven. Each one is inferred in two to five places, and
no command or route tells a person or an agent which state the board
is in.

The board gets one function, `app.State`, that computes every machine
once and names each state. Three views read it, and nothing else
derives a state:

- `git board state`, with `--json` and `--jq` as the other commands
  have them. `git board status` is the inbox, as in `gh status`.
- `GET /api/state`.
- The Start page of the web app.

Each machine also lists its next actions. A next action names the CLI
command and the API route that does it. A person reads the sentence.
An agent reads the command. The web app shows a button, never a
command.

The old call sites change to read `app.State`. The state must not be
a sixth copy of the same tests. This is what makes it a fix, not a
feature.

| Machine | Named states | Inferred today in |
|---|---|---|
| Sync run | idle, running here, running in another process, stopped mid-run, done | `surface/http/sync.go:136`, `sync/lock.go:120`, `sync/journal.go:216` |
| Forge | found in origin, in a table and off, pull, push, both, first sync, no token, token for another owner | `app/forges.go:62`, `forge/conf.go:147`, `app/tokens.go:82` |
| Datastore | git, Markdown, PostgreSQL, the forges alone, set by the environment | `app/datastore.go:31`, `app/numbers.go:32` |
| Project file | no repository, no file, current, older schema, newer schema | `app/app.go:176`, `app/init.go:140`, `app/config.go:226` |
| GitHub Project | none, broken table, configured, fields stated, write on, fields held | `tools/ghproject/session.go:124`, `app/project.go:82`, `surface/cli/tui.go:58`, `surface/http/project.go:127`, `:167` |
| Identity | none, chosen | `domain/people/service.go:17` |
| Start over | equal, ours newer, theirs newer, both started over | `app/reset.go:102` |
| Old git-bug refs | none, switched, not switched | `app/forges.go:291` |
| Serve | none, running, stale | `surface/http/serve.go:260` |
| Dev cycle | off, on, broken table | `app/config.go:87`, `app/cycle.go:31` |

### Defects that the states expose

Each fix keeps the v1 output, which the golden recordings
(`golden/compat_test.go`) fix byte for byte.

- A run that stopped mid-run is never reported. `sync.Cut` and
  `sync.Running` have no caller outside tests. The status reports the
  stopped run, and the next sync says that it resumes.
- A CLI sync is not visible in `GET /api/sync`, because the server
  reads only its own memory. The status reads the OS lock.
- One value has three names: `both` in the table, `sync` in
  `bsync.Both` and in the API. The API accepts both names and answers
  `both`. The table keeps refusing `sync`.
- `git board config set` accepts `board.view`, but no code reads it.
  `board.experimental` or `[experimental]` turns the cycle on. The key stays accepted, and the command
  says that it has no effect.
- A broken `[github-project]` table looks like no table in the TUI,
  because `tui.go:58` drops the error. The status names it broken.
- The datastore's display name is computed twice, and a forge copy is
  "git" in one place and "the forges alone" in the other. One function
  names it.
- `refs/board/switched` is written as a literal in two files, and no
  code in this repository writes it. One constant holds it, and the
  error names the v1 command that writes it.
- A reset or a drop deletes the sync entity, so the next sync is a
  first sync again and asks for consent. The status shows this before
  the run. A test proves it.

## The project file on a first run

The file stays optional, and the app does not ask for it at start.
A board with no file already works: every `ns:value` label is a field,
and the forges are the projects that `origin` names. A question at
start interrupts a person who only wants to read the issues.

The file is written when a person first changes a shared setting. The
Fields page, the Project page and the Sync page already do this
through `POST /api/config/init` or a write route. The page says, "This
writes `.git-board.toml`. Commit it to share it with the team."

The global file keeps only personal values. Today it holds one key,
`template`. It does not hold shared settings, because then two clones
of the same repository read different boards (ADR 0011). A personal
default `way` for new forge tables is the only candidate key, and it
waits for a real need.

The Start page offers the file as one step that you can skip, with a
choice of template: the default, `enac`, a URL or a path. Today only
the CLI can choose a template.

## The first-run flow

The Start page is at `#/start`. A board is new if no identity is
chosen, or no forge has run and the board holds no issue. A new board
with no issue offers the Start page in its empty state. The page does
not open by itself, because a redirect at start breaks every e2e test
that opens an empty board.

1. Who you are. Choose or create your identity. If you skip this
   step, the board is read-only.
2. What you want. Select one goal. You can add the others later.
3. Log in to the forge that the goal needs. Today you paste a token,
   on localhost only. Later the OAuth device flow (#101) replaces this
   step and keeps the same place in the flow.
4. Look at the first run. The app runs a dry run and shows its counts:
   issues to pull, issues to create, fields to write.
5. Run it. The Open the board button then opens the board.

| Goal | What the app writes | Step 4 asks for consent |
|---|---|---|
| See my forge's issues here | `way = "pull"` on a forge table; a forge found in `origin` needs a table first, which Add a forge writes | no, because nothing is created on the forge |
| Work here, and keep the forge in step | `way = "both"` | yes, when the board holds issues the forge does not have |
| Keep a backup on another forge | a `[forges.backup]` table, `way = "push"` | yes |
| Keep the issues as Markdown files | `board.datastore = markdown:<dir>`, then the copy | no |
| Plan in a GitHub Project | `[github-project]` with `fields`, and `write` if you select it | no |

Each step writes one decision. The CLI gets the same flow without a prompt: `git board state`
prints the next actions in order, and `git board` with no argument
stays as it is.

## What is missing

These gaps make the web app send you to a terminal, which the rule
from ADR 0022 forbids:

- The Fetch, Pull and Push buttons show CLI commands in their tooltips
  (`SyncSettings.vue:225`).
- The token field says to set `GITHUB_TOKEN`. That needs a restart
  from a shell.
- The Project form cannot turn on `write = true`. You must edit the
  file by hand.
- Markdown export (`git board export --md`) has no route.
- A file edited by hand needs a restart of the server.

These gaps are in the product itself:

- An empty board says "No issues match this filter." The Start page
  replaces this on a new board.
- No end-to-end test covers a first run. The flow gets one Playwright
  test in `web/e2e`.
- `docs/guide/configuration.md:9` names "Start from the default in
  Settings › Fields". The button is "Write the default file".

## The ADRs, merged

The 22 ADRs contradict each other in six places, and five of them
describe code that is gone. The merge keeps nine. Each group goes
into its lowest number. Each absorbed file becomes a stub of three
lines, "Merged into ADR 00NN", and leaves the navigation. No number
changes, so the 374 references outside `docs/adr/` keep their target.

A new page, `docs/adr/index.md`, reads the nine as one story, in this
order:

| Order | ADR | Title | Absorbs |
|---|---|---|---|
| 1 | 0003 | The board is the team's | 0018 (ownership) |
| 2 | 0002 | Where the board is kept | 0009, 0015, 0016 |
| 3 | 0004 | Issues are operation logs, fields are labels | 0007, 0019 |
| 4 | 0011 | Shared settings in the file, your own in git config | none |
| 5 | 0008 | One truth, many mirrors, and how they sync | 0022, the mappings of 0007 |
| 6 | 0010 | No secret is stored, and the server answers on localhost | 0013 |
| 7 | 0006 | One binary, built in layers | 0005, 0012, 0020, 0021 |
| 8 | 0014 | Tests on real repositories, few licences | 0017 |
| 9 | 0001 | Proof in git notes, and the opt-in cycle | 0018 (the cycle) |

The merge removes the six contradictions:

- 0007 says that the forges get labels and never their own fields.
  0022 puts fields in the Project's columns. The merged 0004 and 0008
  say that a field is a label in the board, and that a mirror holds it
  where its mapping says.
- 0005 names `core/`, which is gone.
- 0006 says that go-git reads objects. `git cat-file` reads them
  (0021).
- 0008 says that there is no delete operation. 0019 adds one.
- 0018 keeps the agents in the repository. 0021 moves them out.
- 0002 makes a board kept on the forges alone a copy of the forges.
  0022 says that the board is the only truth. The merged 0002 says
  that this datastore keeps no board of its own.

Content that is not a decision moves out of the ADRs:

- The line and key format of 0001, its evidence and prior art, to
  `docs/spec/`.
- The file names and the cache note of 0002, to
  `docs/spec/datastores.md`.
- The forge defaults of 0007 and the defaults of 0015, to
  `docs/guide/configuration.md`.
- The step logs of 0020 and the open items of 0021, to `docs/rewrite/`.

The merged ADRs keep the format of ADR 0022: Status, Date, Context,
Decision, Consequences. Each says which ADRs it absorbs.

## What was built on 2026-11-09, and what was not

Built, each with its test:

- `app.State` and its views, `git board state` and `GET /api/state`
  (`TestStateOfANewBoard`, `TestStateOfAForge`, `TestState`).
- A run held by another process shows as `running`, and the web app
  refuses a second run with 409. The server reads the OS lock through
  `Board.Syncing`.
- The API takes `both` for a two-way run.
- `git board config set board.view` says that v2 reads it nowhere.
- The Start page, with the e2e test `web/e2e/start.spec.ts`.
- The Fetch, Pull and Push buttons say what they do. The Add a forge
  dialog no longer names an environment variable. The Project page
  turns `write` on and off (`web/e2e/project.spec.ts`).

Changed after a closer look at the code:

- A broken `[github-project]` table stops the board from opening, so
  the TUI never sees one. The `broken` state stays for completeness.
- A pull or a push alone records no run, so the state reads "first"
  from the sync entity existing, as the consent to create does.
- An older file is no setup step. The only migration comments out two
  keys that git-board does not read, and every upgrade adds a line.
- A GitLab project that the remotes name by its path is keyed by its
  numeric id, which only the forge says. Its state is `on` until the
  first sync.

Not built yet:

- A state chip in the header.
- The datastore's display name in one function. (`refs/board/switched`
  is now one constant, `switchedRef`.)
- A template choice on the Start page.
- The line that each step writes, shown on the page.
- Making a forge found in `origin` read-only in one click: today the
  page tells you to add it as a forge with the way Read only.
- Markdown export from the web app, and a reload of a file edited by
  hand.

