# The classic issue page by default

PRD, 2026-10-28, from the discussion of the same day. Decisions marked
**(user)** were taken by the user; those marked **(call)** are this
document's, open to change.

## Why

Since the cycle protocol (`docs/prd/cycle-protocol.md`, then
`time-flow.md` and `project-cycle.md`), every issue page is the cycle's
current step:
- the step rail;
- the step's workspace over the description;
- the flow, cut into segments;
- the review screen;
- the verdict bar.

The cycle is still a work in progress, and a board that doesn't work that
way, such as co2-calculator's, gets it all the same. **(user)**

**The plain issue page of 17 October comes back as the default. The cycle
becomes something a board, or a person, turns on.**

## Decisions

| Topic | Taken |
| --- | --- |
| The default | The classic page. **(user)** |
| Classic | The plain page from before 18 October (before `03fc93b`): <ul><li>the title, with Edit and Close or Reopen;</li><li>the meta line: status, author, comment count, forge links, id;</li><li>two tabs, Conversation (the description, then every comment and event in order, then the composer) and History;</li><li>the side column of fields.</li></ul> **(user)** It's built from today's components, not the old file, so nothing that landed since for every issue is lost. **(call)** |
| The Conversation tab | The plain list, as on 17 October: the description, then the comments and the label, status and title changes, in order. There is no flow, no filters, no version milestones, no sections to comment on, and no description versions. Threads, replies and suggestions that people on the cycle's page wrote show as ordinary comments in that order; resolving or applying them is the cycle page's. A comment's step chip doesn't show. **(call: "the plain one from before 17 October", user)** |
| Tabs | Conversation and History. The Changes tab, which reads the cycle for its code part, is the cycle page's, so the classic page never asks for the cycle. **(call)** |
| The team's switch | `.git-board.toml`: a board with a `[cycle]` table walks the cycle, one without gets the classic page. Every key of `[cycle]` configures the cycle, so writing one means the board uses it. **(user, as proposed)** |
| A person's switch | `git config board.view classic` or `cycle`, read at every level git reads (`--global` for every repository, or this one's alone), never a new file. It changes what that person sees, not the board's file. **(user, as proposed)** |
| Which wins | `board.view` when set, else the file. Any other value of `board.view` counts as unset, and the page says so. **(call)** |
| Off means off | With the classic page:<ul><li>the issue page has no step, flow, review or verdict;</li><li>the header has no Work;</li><li>Settings drops the hint about the cycle's guessed stages.</li></ul>The cycle's data stays: comments keep their `cycle:step`, and the verdicts and sign-offs stay comments. Turning it back on loses nothing. **(user)** |
| The server | It reports the view; the web decides what to draw. The cycle's endpoints keep working, because a person with `board.view cycle` on a board without `[cycle]` walks the default cycle. No endpoint gates a status or a field today, so there is no gate to turn off. **(call)** |
| Settings › You | Three choices: As the board says, Classic, The cycle's. They write `board.view` in this repository's git config (`--local`); As the board says unsets it there. A global `board.view` is left alone, and stays a command. **(user, 2026-10-29; the level: call)** |
| The issue page's switch | On the cycle's page, a Cycle \| Classic switch at the end of the meta line shows the issue as the classic page does, for that issue and the next ones in the tab, until switched back or the page reloads. It writes nothing. Someone on the classic page has no switch. An unsent comment or edit is asked about first: the two pages have their own composers. **(user, 2026-10-29; how long it lasts: call)** |
| Breaking | A board with no `[cycle]` table, this repository and co2-calculator among them, opens on the classic page after this release. The release notes say how to keep the cycle: `[cycle]` in the file, or `git config board.view cycle`. **(user, accepted)** |

## What the server says

`GET /api/me` gains two fields:
- `view`, either `"classic"` or `"cycle"`;
- `view_from`, which says where the view came from:
  - `"board.view"`: the person's git config;
  - `".git-board.toml"`: the board's `[cycle]` table;
  - `"default"`: neither is set.

When `board.view` holds another value, `view_from` is still `"default"` or
`".git-board.toml"`, and a third field, `view_ignored`, holds the value
that was ignored. `view_scope` is the git config level that set
`board.view` (`"local"`, `"global"`…).

`PUT /api/me/view` takes `{"view": "classic" | "cycle" | ""}`: it sets
this repository's `board.view`, or unsets it for `""`, and answers with
`/api/me`.

The config keeps, beside its `[cycle]` values, whether the file has the
table at all: `Cycle.On`, read with the TOML decoder's `IsDefined("cycle")`.

## The web

- **The issue panel and the issue page** draw the classic layout unless
  the view is `cycle`. Until `/api/me` has answered, they show the loading
  skeleton they show today, so the page never flashes one layout and then
  the other.
- **The header** lists Work only with the cycle view.
- **Settings › You** says which page your issues open on and why, and
  lets you pick: As the board says, Classic, The cycle's. It names
  `[cycle]` in the file for everyone and `git config --global` for every
  repository.
- **The issue page**, on the cycle's page, has the Cycle | Classic switch.
- **Settings › Project file's** hint about the guessed stages shows only
  with the cycle view.

## Not in this PRD

- **A saved per-issue choice of layout.** The switch lasts for the tab.
- **Removing any cycle code.**

## Left to the builder

- How the panel's template splits, and whether the classic layout is a
  component of its own.
- The wording on Settings › You, in the page's voice.
