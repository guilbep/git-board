# v2: the web app

- Status: built 2026-11-05, in `next/web`, served by `git-board-next`.
  v1's `web/` stays, served by v1, until the switch; it is deleted with
  v1 ([ADR 0021](../adr/0021-git-board-v2-same-data-new-code.md)).
- Builds on [the HTTP API](api.md) and [ADR 0012](../adr/0012-one-go-binary-web-app-embedded.md)
  (one binary, the web app inside).

`next/web` started as a copy of `web/`: the pages look and work as they
did. What changed is underneath, where v1's app worked around v1's API,
and what v2 dropped.

## One client, the API's shape

`next/web/src/api` is the one typed client, a module per part of
[api.md](api.md): the board and its events, issues, labels and people,
the inbox, the file, sync, texts. No component calls `fetch()`.

- **One error shape.** Every refusal is `{code, message, field}`
  (`api/error.ts`); the pages test the code (`stale`, `not-found`,
  `no-identity`), never a message.
- **Every write from the version read.** A thread, an issue of a list and
  the file carry their version; each write sends it (`If-Match`). A 412
  `stale` writes nothing, and the page keeps the write and asks: **Reload**
  (the issue or the file as it is now, the write dropped) or **Overwrite**
  (the same write without a version: the last writer wins, field by
  field). v1 had a version for comments alone (`changed`), and a write
  during a sync waited for its end (`syncing`); v2 writes during a sync.
- **One stream of changes.** `GET /api/events` tells every write, the
  page's own (`by`) and anyone else's: a command, a fetch, a sync. A page
  that falls behind reconnects and reloads (`resync`). v1's polling of a
  sync's job is gone: a run's steps come as `progress` events.

## The board, paged

v1's page asked for every issue at once, grouped by the server, and
sorted and filtered the answer in the page. On 20,000 issues that is 8 MB
of JSON before the first row.

- A list is pages of 100 (`boardlists.ts`). Ungrouped, one list; grouped,
  one list per value of the field: the lane's term added to the filter
  (`status:ready`, `no:status` for the issues without one), each loading
  its first page (40 cards a lane) and the next as its end comes near.
  The server sorts. A board asks its options' lanes and the counts at
  once, then a lane for each value in use no option names.
- A table past 300 loaded rows renders those on screen and a screen
  around them (`windowrows.ts`); spacers hold the rest's height.
- The filters panel counts with `GET /api/labels`: each facet against the
  rest of the filter, which the page now reads itself (`filterterms.ts`:
  v1's server answered the panel whole). It asks only what it shows.
- The list waits for the file alone (the views); the fields come beside
  it, the values' colors after it, the header's inbox later.

Measured on the 20,000-issue board (2 CPUs, headless Chromium, the server
on the same machine): the first rows 280 ms after navigation (median of
five; 260 ms as a board by status); typing a field term, the list 200 to
230 ms after the last key, each key handled in under 40 ms; scrolled
100,000 px, 5,300 issues loaded and 25 rows in the DOM, frames at 17 ms
(p95 33 ms). The server builds the words' index in the background once
it serves, after the rows (`app.IndexTexts`; the CLI still builds it on
the first query that searches): on a fresh copy of the 20,000-issue
board the first rows come 3.0 s after the start, as before, the index is
built 8.8 s after it, and a first free-text filter then takes 0.12 s
(6.4 s before); one asked earlier waits for what is left of the index,
never for a second build of it.

## Settings, as keys of the file

v1's server answered the file as a model (`/api/config/model`) and took
a form's section whole (`/api/config/section/<name>`). v2 answers the
text, its version and its keys:

- the page parses the text (`configparse.ts`, smol-toml) into the forms'
  model, with the forges `GET /api/sync` finds;
- a form saves its section as keys of `PATCH /api/config`
  (`sectionpatch.ts`): a field or a view a whole table, the other
  sections their keys alone, so a key the form doesn't show stays; a new
  order of the tables is the text's, put whole after the patch
  (`tomlorder.ts`);
- the file's editor checks the TOML here, with its line, then asks the
  server's dry run (`PUT /api/config?dry_run=1`).

v1's value colors (`/api/colors`) are the page's (`valuecolors.ts`), as
api.md says: the file's, a word's (done green, blocked red), then a
spread. The settings' reference reads the guide built into the page
until `GET /api/docs` answers.

## What v2 dropped, and what waits

Dropped, as [ADR 0021](../adr/0021-git-board-v2-same-data-new-code.md)
and [api.md](api.md#v1s-routes-and-where-they-went) say: agents and
worktrees (their own binary), the HTML export, Start over (the CLI's
`git board reset`), the classic
and cycle pages as a choice (one page), v1's per-issue sync (until the
sync engine takes one issue), Labels' comparison with GitHub and lint
(a tool), the clone's first-pull note (until v2 pulls on a first open).

Insights is v1's page on route 51 (`GET /api/insights`, the replay in
`next/tools/insights`, v1's, and `insights` on the CLI). Releases and a
release's page are v1's on routes 43 to 48 (`next/tools/ship`): the
adoption's proposals come with the list, a job's steps as `release`
events; Enable binary release (v1's `/api/releases/setup`) is Settings ›
Release now, a table of the file. The Work page (experimental: its link
shows with the cycle on) is v1's on `GET /api/cycle`, without v1's
agents and its aside (what waits for the forges is Sync's). It reads
every open issue's thread at once and keeps each row by its version,
the viewer and the code's refs: 2.7 s for the 12,000 open issues of the
20,000-issue board the first time (71 s before; v1 took over 400 s),
230 ms after, and it pages as the lists do (`limit`, `group`, `cursor`;
performance.md, The Work page). An issue's
History draws the cycle's step bars from `GET /api/issues/{id}/cycle`.
Attaching files (paste, drop, pick in a text, as v1) works on routes 40
and 41; a forge's pictures load through route 42. An issue with the
cycle on shows its cycle above the description (`CyclePanel.vue`, the
controls from `cycleact.ts`): the steps, the gate and whose turn it is,
the branches, the pull request with its checks (Open, Edit, Merge, Check
again), the environments, and the controls the server offers the viewer,
each one action of route 50 sent from the version read: Accept without
criteria, the spec's verdict, the risk, the status moves, Approve the
plan, Mark ready for review, the review, and at Verify the criteria's
checks, a pasted run and the sign-off. The Work page's Review and Sign
off open it. v1's line comments on the diff and the tests' run wait for
the changes' and the runs' reads. Waiting for their routes (M6, api.md
routes 51 to 53): the GitHub Project and its conflicts. Their v1 pages stay in `web/` until then; the cycle's
settings form (a table of the file) and the experimental switch stay.

## Tests

- `npm test` (`make next-web-check`): the pure modules, Node's runner.
- `make next-web-e2e`: 86 browser tests on `git-board-next serve` over a
  git board and a Markdown board (`next/web/e2e/serve.mjs`, with a fake
  GitHub and, for the Markdown board, a fake S3), ported from v1's;
  `stale.spec.ts` (Reload, Overwrite, a form's), `live.spec.ts` (the
  change events), `markdown.spec.ts` (an issue's life on the Markdown
  board, in its file), `views.spec.ts` (a view saved from the board),
  `lanes.spec.ts` (a lane's and a table's pages), `insights.spec.ts`,
  `releases.spec.ts` (a cut to a bare authority, its record's writes),
  `work.spec.ts` (the cycle on and off), `cycle.spec.ts` (the panel's actions, a pull request opened and merged on the fake GitHub, Work's Review and Sign off) and `uploads.spec.ts` (pick and
  drop in a comment) are new. Dropped with their features: the clone's
  first pull, the ENAC template's labels, Labels' lint, a field's GitHub
  Project column, Start over.
