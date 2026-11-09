# v2: the HTTP API

- Status: built in milestone M5 (2026-11-05), in `surface/http`.
  A route marked *later* waits for the command it mirrors (`datastore
  set`, `numbers status`); one marked *M6* comes with
  its domain. Both answer 404 `not-found` until then.
- Decided: [ADR 0021](../adr/0021-git-board-v2-same-data-new-code.md),
  "The HTTP API: reshaped, with the web UI". Builds on [the
  design](design.md) (section 3: the application, errors, change events),
  [ADR 0012](../adr/0012-one-go-binary-web-app-embedded.md) (one binary, the
  web app inside) and [ADR 0013](../adr/0013-local-server-without-auth.md)
  (no login: loopback, host and origin checked).

v1's API grew one route per button: 117 routes, 18,000 lines with their
tests. v2's is designed from what the web app reads and writes, in
nouns the CLI already prints. It has 60 routes.

The server is `surface/http`. Each route decodes a request, calls
one method of `app`, and encodes the answer. No rule lives in the
server.

## The rules every route follows

**JSON in, JSON out.** A write's body is `application/json`, or the
server answers 415 (ADR 0013: a page of another site can't send JSON
without a preflight). An upload is the one exception (`multipart/form-data`
with the page's `X-Git-Board-Upload: 1`). Times are RFC 3339.

**The same nouns as the CLI.** An issue is what `ls --json` prints, a
thread what `show --json` prints, a log entry `log --json`'s, an inbox
`status --json`'s, a person `me --json`'s, a label count `labels
--json`'s, a key `config list --json`'s ([JSON output](../guide/cli.md#json-output)).
The API adds one field to an issue and a thread: `version`, below.

**Lists are paged.** A list answers one page:

```json
{"items": [...], "total": 1234, "next": "MTAwOmE5ZjM..."}
```

`limit` is the page's size: 100 by default, 1,000 at most, `0` for every
item. `cursor` is the `next` of the page before; `next` is absent on the
last page. A cursor is opaque: it names the place of the last item seen
in the list's order (what the list sorts by, as that item was, and its
id), and the next page starts at the first item after that place. Issues
added before it meanwhile don't come again, and the item leaving the list
(closed, moved to another lane) or moving skips nothing after it. A
cursor is the sort's it was given with: another `sort` is `invalid`. `total`
counts every item the request matches.

**Filters are the CLI's grammar.** `filter` is what `ls` takes
([Filters](../guide/configuration.md#filters)). Absent, it is `is:open`, as
`ls`; empty (`filter=`), it matches every issue.

**Versions and compare-and-swap.** Every entity has a version: its head
commit in git, its head in PostgreSQL or a folder. A read answers it, in
the body (`version`) and in the `ETag` header (`"<version>"`). A write
may send it back in `If-Match`. The write then happens only if the
entity is still at that version, checked where the datastore writes it,
so nothing can slip in between; else it answers **412** `stale`, and
nothing is written. Without `If-Match` a write applies to the entity as
it is now (the last writer wins, field by field). A write answers the
new version. The web app always sends `If-Match`. `.git-board.toml`
works the same way: its version is a hash of its text.

`If-None-Match` on a `GET` of an issue answers **304** when the version
is the same.

**One error shape.** Every error, whatever the route:

```json
{"code": "stale", "message": "the issue changed since you read it", "field": "title"}
```

`message` is a sentence for a person. `field` names the request's field
the error is about, when there is one. `code` is one of a fixed list,
decided by the error's class in `app` (`app.Error`): the same class
that gives the CLI its exit code.

| `code` | HTTP | Class | CLI exit code | When |
|---|---|---|---|---|
| `invalid` | 400 | Usage | 2 | a wrong request: a value, a field, a filter, a key `.git-board.toml` doesn't read |
| `not-json` | 415 | Usage | | a write whose body isn't JSON |
| `not-found` | 404 | NotFound | 3 | no such issue, comment, person or route, or a prefix naming several |
| `experimental` | 404 | Off | 1 | a feature behind `[experimental]` that is off |
| `no-identity` | 409 | NoMe | 4 | no one chosen to write as |
| `changed` | 409 | Changed | 5 | a comment's text changed since the edit started (its `base`) |
| `stale` | 412 | Stale | 5 | `If-Match` names a version the entity has moved from |
| `busy` | 409 | Busy | 1 | a sync, a copy or a release runs; try again |
| `missing` | 409 | Missing | 1 | the datastore can't do it (ADR 0002 §3): numbers on a folder |
| `forbidden` | 403 | Forbidden | 1 | the host or the origin checks (ADR 0013); a cycle's step that waits for someone else |
| `internal` | 500 | Internal | 1 | anything else; v1 answered 400 |

v1's other codes are gone. `syncing` is the one a page met most: v2
writes during a sync (compare-and-swap keeps it safe, ADR 0021).

**Who writes.** The person the clone's `board.me` names, as the CLI.
`X-Git-Board-Client` names the page that writes; its change events carry
it as `by`, so the page can ignore its own.

**The checks of ADR 0013.** Every request's `Host` must name this
machine (`localhost`, `127.0.0.1`, `[::1]`, or what `--addr` exposes).
A write's `Origin`, when sent, must be this server's own (or the dev
server's, `localhost:9000`). Both answer 403 `forbidden`, and are logged
as warnings. Clients without an `Origin` (curl, scripts) are the user's.

**The code gate** (ADR 0013, Amendment 2026-11-05). Reaching the port
writes the board; it must not run code as the user. A request that makes
this machine run code, or changes what code a later run runs, is refused
(403 `forbidden`) when the server listens beyond loopback (`--addr`), and
otherwise asks a same-host `Origin`, the board's own page (or the dev
server's): a script without an `Origin` is refused too. It gates:

- a cut (44), a promotion and an adoption (46: `{state: "promoted"}`,
  `{adopt}`), a deploy and a rollback (48): they build (`sh -c` of
  `release.build`, `go build`, `docker build`, `helm`) and push the
  overlays that say what runs;
- a config write (24, 25) whose file changes a key that holds a command
  or says what a run builds, deploys or trusts: `[release]` (any key),
  `[cycle.tests]`, `[cycle.checks]` and `cycle.signers`. The same write
  without such a change, and every dry run, pass.

Notes, yanks and comments on a release are records, and pass. The
datastore's copy (29) answers on loopback alone, as before, and so do
the other routes that write a secret or say where the board goes: 28,
33 and 55 (a token to git's credential helper), 57 (a git remote).
`localRoutes` (`surface/http/guard.go`) lists them, and
`TestLocalRoutes` refuses each from another address.

## Routes

`{id}` is an issue's name as the CLI takes it: its id, a prefix of it,
or its project number (`CO2-3075`).

### The board

| | Route | Request | Response |
|---|---|---|---|
| 1 | `GET /api/board` | | `{git_dir, dir, version, datastore, experimental, first_pull}` |
| 2 | `GET /api/events` | | the change events, Server-Sent Events (below) |

`/api/board` is what a `git board` reads to find the board running for
its repository (`git_dir`), and what a page reads first. `datastore` is
`{name, where}`: `git`, `Markdown files` or `PostgreSQL`, and where it
is (never a password). `experimental` lists the features on.
`first_pull`, there only when the server's first open of a fresh clone
pulled origin's board or couldn't, is `{remote, issues, error}`: how many
issues came, or why none did ([A clone shows its
board](https://github.com/guilbep/git-board/blob/main/docs/prd/clone-shows-board.md)). The pull runs once the server
serves; a page already open hears it as a `me` event, and an `issue`
event for each issue pulled.

### Issues

| | Route | Request | Response |
|---|---|---|---|
| 3 | `GET /api/issues` | `filter`, `sort`, `limit`, `cursor`, `at` | a page of issues |
| 4 | `POST /api/issues` | `{title, body, labels, set, force}` | 201, the thread |
| 5 | `GET /api/issues/{id}` | `at` | the thread |
| 6 | `PATCH /api/issues/{id}` | `If-Match`; `{title, body, base, status, reason, duplicate_of, comment, labels, set, force}` | the thread |
| 7 | `DELETE /api/issues/{id}` | `If-Match` | 204 |
| 8 | `GET /api/issues/{id}/log` | `field`, `since` | `{"items": [log entry]}` |
| 9 | `POST /api/issues/{id}/comments` | `If-Match`; `{body}` | 201, the thread; `Location` names the comment |
| 10 | `PATCH /api/issues/{id}/comments/{comment}` | `If-Match`; `{body, base}` | the thread |
| 11 | `DELETE /api/issues/{id}/comments/{comment}` | `If-Match` | the thread |
| 12 | `POST /api/issues/{id}/undo` | `{force}` | `{issue, title, undone, what}`, as `undo --json` |
| 13 | `GET /api/issues/{id}/follow` | | `{following, reasons, set}`, as `follow --json` |
| 14 | `PUT /api/issues/{id}/follow` | `{follow: true, false or null}` | the same |

**List (3).** `sort` is `ls --sort`'s: `created`, `updated`,
`title` or a field, then `asc` or `desc` (`prio desc`); absent, oldest
first. `at` (RFC 3339 or a
date) lists the board as it was then, as `ls --at`. An item is an issue
plus its `version`. No `group`: a board's column is its own list
(`filter=<the view's filter> status:ready`), each paged on its own, and
counted by `GET /api/labels`.

**Create (4).** `labels` are labels to start from (a template's); `set`
is `set`'s assignments, `["status=ready", "assignee=@me"]`, checked
against the config unless `force`. It answers the new thread, its
`ETag`, and `Location: /api/issues/<id>`.

**Show (5).** The thread, as `show --json`, with each text's `html`
rendered, and its `version`. `at` shows it as it was.

**Change (6).** Every change to an issue is this one route. Each field
is optional; those sent are applied in this order, each one a write:

- `title`, `body`: `edit`. `base` is the description the edit started
  from: `changed` if it moved since (v1's 409).
- `labels`: `{"add": [...], "remove": [...]}`, raw labels.
- `set`: assignments, as create's.
- `status`: `open` or `closed`. To close, `reason` (`completed`,
  `not-planned`, `duplicate`), `duplicate_of` and `comment` (a comment
  first, "Close with comment"), as `close`; to reopen, `comment`, as
  `reopen`.

`If-Match` holds for the whole request: the first write checks it, and
the next ones check the version the first wrote. A request that fails
halfway answers the error, and keeps what was written (the change events
say so).

**Delete (7).** A tombstone (ADR 0019): the issue is hidden, its history
kept.

**Comments (9 to 11).** A comment's id is its event's `id` in the thread.
`base` is the text the edit started from: `changed` if someone edited it
since. The cycle's and the review's metadata on a comment (`meta`) come
with their domain (*M6*).

**Undo (12).** Your last change to this issue, as `undo`: refused
(`invalid`) when someone wrote after it, unless `force`.

**Follow (13, 14).** `null` goes back to following automatically (you
opened it, you are assigned).

### Labels, fields, people

| | Route | Request | Response |
|---|---|---|---|
| 15 | `GET /api/labels` | `filter` | `{"items": [label count]}`, as `labels --json` |
| 16 | `GET /api/fields` | | `{name: field}` |
| 17 | `GET /api/people` | | `{"items": [person]}`, as `me --ls --json` |
| 18 | `GET /api/me` | | `{me, avatars}` |
| 19 | `PUT /api/me` | `{id}`, or `{name, email}`, or `{avatars, global}` | the same as 18 |
| 54 | `POST /api/fields/{name}/migrate` | `dry_run` | `{plans, error}`, as `sync --migrate` |

**Labels (15)** are the labels in use, each with its open and closed
count, among the issues `filter` matches (every issue when absent): the
filter panel's counts, and every value a filter box suggests.

**A field's migration (54)** is the forge-mapping migration (`sync
--migrate <field>`): once a field's home on a forge changes (its
`[forges.<name>.fields.<field>]` mapping) and a sync ran, it takes the
old labels off the forge's issues, and sets the assignees when they are
the new home. With `dry_run=1` it is the preview Settings › Fields shows
after a mapping changed, and writes nothing. A plan per declared forge:
`{forge, repo, target, issues, labels, note}`. A forge that refuses
keeps its note and the others go on: `error` is the first refusal, and
the plans say what landed. A field nothing configures is `invalid`.

**Fields (16)** are the fields with their types: the configured ones and
those the labels show (`date`, `person`, `issue`…). Each is `{type,
single, numeric, options, unit, current, inherits, people, colors}`:
`current` an iteration's sprint today, `inherits` a role's project
value, `people` who a person field offers, `colors` each value's color
as `.git-board.toml` sets it. v1's `/api/colors` and `/api/sprints` are
in it. v1 also gave a color to every value the file doesn't color, and
dates red once past: the new web app colors those itself.

**Me (18, 19).** `me` is the person who writes, or `null`. `{id}` writes
as that person (`me <id>`), `{name, email}` makes one and writes as
them (`me --new`), `{avatars}` sets `board.avatars` (`forge`,
`gravatar`, `none`) in the clone's own config; with `global: true` at
git's global level, so every repository of the user reads it, a local
one read over it.

### The inbox

| | Route | Request | Response |
|---|---|---|---|
| 20 | `GET /api/inbox` | | as `status --json`: `{me, assigned, opened, follows}` |
| 21 | `POST /api/inbox/seen` | `{issues}` | `{}` |
| 22 | `PUT /api/inbox/settings` | `{views, fields}` | `{}` |

### Settings: `.git-board.toml`

| | Route | Request | Response |
|---|---|---|---|
| 23 | `GET /api/config` | | `{schema_version, path, version, text, keys, problems}` |
| 24 | `PUT /api/config` | `If-Match`; `dry_run`; `{text}` | the same as 23 |
| 25 | `PATCH /api/config` | `If-Match`; `dry_run`; `{set: {key: value}}` | the same as 23 |
| 26 | `GET /api/docs/{page}` (*later*) | | `{"items": [{title, html}]}` |
| 56 | `POST /api/config/init` | `{template}` (none: the default) | 201, `{path, lines}` |

`schema_version` is the file's schema (1 for a file without one; the
versioned schema and its upgrades are built beside this). `keys` are
the keys it sets, `[{key, value}]`, as `config list --json`. `problems`
are what is wrong, each `{table, message}`: a broken table is its own
domain's problem, and the rest answers.

**Whole text (24)** is the file as a person writes it. **Keys (25)** are
dotted keys, as `config set` takes them (`fields.prio.options`,
`views.mine.filter`); a value is JSON (a string, a number, a list, an
object), `null` removes the key. An object is the key's whole table: its
keys set in place, those it leaves out removed, its sub-tables likewise
(`fields.prio`, or a top table, `cycle`), and `null` removes a table with
its sub-tables; a list of objects is an array of tables (`sprints.list`).
A string where the file holds a date stays a date. Only those keys' lines
change: comments and order stay. Both refuse (`invalid`, nothing written) a key git-board
doesn't read or a file left broken. With `dry_run=1` they check and
answer the file they would write, and write nothing: v1's
`/api/config/check`. Views are `[views]`: saving one is a `PATCH`.

**Init (56)** writes the default `.git-board.toml`, as `git board init`
does: the template `template` names (a built-in one, a file, or a URL;
none: the default), seeded with origin's labels and its Project's fields
when it can read them. It never overwrites one: a file there already is
an error, 409. `lines` are what reading the forge said.

**Docs (26)** are the guide's sections the settings show beside a form:
`configuration`, `forges`.

### The datastore

| | Route | Request | Response |
|---|---|---|---|
| 27 | `GET /api/datastore` | | `{name, where, replica, counter}` |
| 28 | `PUT /api/datastore` | `{value}`; loopback only | the same as 27 |
| 29 | `POST /api/datastore/copy` | `{from, to}` | `{to, entities, counter}` |

`replica` says the board can be pulled and pushed (git), `counter` that
it hands out project numbers. `PUT` (28) points the clone at another
datastore: `board.datastore` in its own git config, `""` or `git` for
git. The value is opened first, so a name no datastore understands is
refused and the clone's board still opens; the board then opens again
and the pages hear a `config` event. `dry_run` (open it and say what it
holds, v1's `/api/datastore/test`) comes later. A copy
(`from` and `to` are `board.datastore` values: `""` is git, and the
datastore the clone names) and `PUT` answer only on loopback (ADR 0013).

### Sync: forges and remotes

| | Route | Request | Response |
|---|---|---|---|
| 30 | `GET /api/sync` | | `{forges, remotes, run}` |
| 60 | `GET /api/state` | | the board's state, as `git board state --json` |
| 31 | `POST /api/sync` | `{scope, way, forges, remote, dry_run, allow_create}` | 202, the run |
| 32 | `POST /api/remotes/{name}/{action}` | `action`: `fetch`, `pull`, `push` | `{remote, action, note, issues, people}` |
| 57 | `POST /api/remotes/{name}` | `{url}`; loopback only | 204 |
| 33 | `POST /api/forges` | `{kind, url, name, login, token, sync, way}`; loopback only | 201, `{name}` |
| 34 | `PATCH /api/forges/{name}` | `{sync, way}` | `{name, sync, way}` |
| 35 | `DELETE /api/forges/{name}` | | 204 |
| 36 | `GET /api/forges/credentials` | | `{"items": [{host, login, source}]}` |
| 37 | `GET /api/numbers` (*later*) | | `{prefix, next, unnumbered}` |
| 38 | `POST /api/numbers` | `{prefix, start}` or `{}` | `{where, numbered}` |
| 59 | `GET /api/forges/auth?check=` | | `{"items": [forge status]}` |
| 55 | `POST /api/forges/auth` | `{forge, login, token}`; loopback only | 201, `{forge, host, login}` |

**Status (30).** `forges` are the forges the board syncs with: its
`[forges]` tables, or those found in origin's URLs (`discovered`), each
`{name, kind, project, host, login, sync}`. `remotes` are the git
remotes' names; `run` the run in progress or the last one (`null` before
any).

**The state (60)** names the state of each part of the board, and the
next actions, each with its command and route: `{phase, file, me,
datastore, forges, project, run, issues, cycle, next}`. It is
`app.State`, which `git board state` prints too
([the proposal](../design/first-run/proposal.md)). Its `run` is
`running` while this server's run, the CLI's or another server's holds
the lock, and `stopped` when a run stopped before it went through. A
run (31) started while another process holds the lock answers 409.

**A run (31)** is one sync, in the background; its steps come as
`progress` events, and `GET /api/sync` answers it at any time. `scope`
is `forges` (`forge sync`), or `all` (fetch, the forges, push: `sync`).
`way` is `sync` (or `both`, the forge table's word), `pull` or `push` (`forge pull|push|sync`); `forges`
limits it to those named; `remote` is the whole sync's git remote
(`origin`); `dry_run` says what a forge run would do (`--check`); a
whole sync goes both ways and has none. One issue's run (v1's
`/api/issues/{id}/sync`, an `issue` here) waits for the sync engine to
take one issue (*later*). A run while another runs answers 409 `busy`.
A run is `{id, scope, way, dry_run, started_at, ended_at, state, steps,
report, error}`: `state` is `running`, `done` or `failed`; a step is a
whole sync's (`{name, state, line}`: `pull`, `forges`, `project`,
`push`) or a forge's event (`{forge, way, what, issue, line}`, `what`
one of `created`, `updated`, `linked`, `posted`, `pushed`, `warning`,
`waiting`, `left`, `held`, `note`); `report` counts what the forges'
run did, `{created, updated, unchanged, posted, pushed}`.

**Remotes (32)** run at once. A fetch answers what a pull would
receive and a push would send, of `issues` and `people`
(`{receive, send}`); a pull and a push answer their `note` (the numbers
handed out). Adding one (57) is the `git remote add` the sync steps
want, done from the web app: a name in the path and its URL in the
body; a name the clone has already is refused. **Forges (33 to 36)** are `[forges]` tables, written as
`PATCH /api/config` writes a key: the board opens again and the pages
hear a `config` event. The token goes to git's credential helper (`git
credential approve`, as `forge login`), never the file (ADR 0010), and is
never answered nor logged. Adding one (33): `kind` is `github` or
`gitlab`, `url` the project's address (`https://github.com/o/r`, `.git`
and GitLab's `/-/…` taken off); `name` defaults to `gh`, `gl`, then
`gh2`… the first free; `sync` to on; the token is for `login`, else
your own account on the host; `way` is the mirror's direction, `push`
(a backup), `pull` (read-only) or `both`, the default. A GitLab project
is written by its id, read
from the forge with that token. With no table yet, the forges found in
the remotes get theirs first (a table turns the finding off), read once
through the forge as a sync reads them; one that already names the
project refuses it, 400 `invalid`. 34 and 35 change a table the file
has: a forge found in the remotes has none, 404. 34 takes `sync`, or
`way` (one of the three, `both` removing the key — the file says a
non-default way alone), or both, and answers the state it left. 36 lists
the tokens
`forge status` finds, with nothing asked of a forge; never one's value.

**The tokens (59, 55)** are `forge status`'s, for the web app's Tokens
section: 59 answers each forge, and for each of its accounts (the
posting login, yours on the host) whether a token is found and where,
as `{name, kind, host, project, sync, way, discovered, tokens}` with a
token `{login, role, found, source, owner, error}` — `role` is
`posting` or `you`; with `check=1` the forge is asked whose each token
is, `owner` the login it says (or `error` what it answered instead).
55 stores a token (`forge login`): `forge` is a forge's name, `login`
the account it is for ("" : yours on the host, else the posting
account), the token on the body alone; it goes to git's credential
helper (`git credential approve`, ADR 0010), and is never answered,
kept nor logged. 201 with the forge, its host and the login it was
stored for.

**Numbers (38)** are the project's numbers (ADR 0016). `{prefix, start}`
starts numbering, and answers where the counter is (`on origin`, `in
PostgreSQL`); `{}` numbers the issues that have none, and answers how
many.

### Texts and files

| | Route | Request | Response |
|---|---|---|---|
| 39 | `POST /api/markdown` | `{body}` | `{html}` |
| 40 | `GET /api/uploads` | | `{enabled, where, max_bytes}` (built) |
| 41 | `POST /api/uploads` | multipart, `X-Git-Board-Upload: 1`, the file in `file` | `{url, name, type, markdown}` (built); `missing` without a bucket |
| 42 | `GET /api/media` | `url` | the picture, through this server (built): a forge's alone, with its token, kept in the board's folder |

`POST /api/markdown` previews a text as the thread renders it, cleaned.
Uploads are `[uploads]`'s, a tool's (`tools/uploads`, v1's, ADR
0015): the keys from the server's environment. Media (42) are the
forges' pictures of issue text (GitHub's attachments, GitLab's uploads),
which a private repository's page can't load without the forge's token:
a thread's and a preview's HTML point them at this route, which fetches
them with the token, from a forge's picture addresses alone: GitHub's
(`github.com`'s attachments, `*.githubusercontent.com`, and the one S3
bucket its attachments redirect to, by its exact name), and GitLab's
uploads on the board's GitLab hosts. They are kept in the board's folder,
256 MB at most: a picture unused for 30 days goes, then the least
recently used.

### Domains that come with M6

These routes keep v1's meaning in v2's shapes, and are built with their
domain (`domain/releases`, `domain/cycle`, `tools`).
Until then they answer 404 `not-found`.

| | Route | What |
|---|---|---|
| 43 | `GET /api/releases` | `local`: `{enabled, remote, trunk, environments, binaries, releases, proposals, job}`, as `release status` reads it (built) |
| 44 | `POST /api/releases` | `{version, final, hotfix, commit, notes, deploy}`: cut one; 202, the job, its steps as `release` events (built) |
| 45 | `GET /api/releases/{tag}` | one release, `{row, issues, candidates, history}`; the tag `next` is the draft (`final`), v1's `/releases/draft` (built) |
| 46 | `PATCH /api/releases/{tag}` | `{notes}`, `{state: "yanked", reason}`, `{adopt: [proposal]}`: `{}`; `{state: "promoted"}`: 202, the job (built) |
| 47 | `POST /api/releases/{tag}/comments` | `{body}`: a comment on its record (built) |
| 48 | `POST /api/environments/{env}/deploy` | `{tag}` (none: the trunk's head, to the followed environment); a rollback is a deploy of an older tag with `reason`; 202, the job (built) |
| 49 | `GET /api/issues/{id}/cycle` | the cycle (experimental): its step, gate and evidence (branches, plan, tests, PRs), and the environments, as one read (built); its list, `GET /api/cycle` (`who`: `me`, `team`), is the Work page's `{needs, waiting}` (built) |
| 50 | `POST /api/issues/{id}/cycle/{action}` | a step's action: the risk, a status move, approve the plan, the spec's verdict, review and resolve, open, edit or merge a pull, verify, check, sign off (built; below) |
| 51 | `GET /api/insights` | `field`, `days`, `view`: flow, throughput, cycle time, burn-up, as `insights --json` (built); `sprint` (`@current` or a slug): `{sprint, title, markdown}`, its report |
| 52 | `GET /api/github-project` | the GitHub Project bridge: its last check, and where each field lives (*built*) |
| 53 | `POST /api/github-project/{action}` | `check`, `apply` (`side`), `resolve` (*built*) |
| 58 | `PUT /api/github-project` | `{owner, number}`: `[github-project]` written as `project configure` writes it, the fields it holds stated with it; owner `""` removes it; answers as 52 (*built*) |

**The GitHub Project (52, 53)**, built with the bridge
(`tools/ghproject`). Only an action reads the Project: a read
answers the last run, which the server keeps. They answer `{project,
write, fields, stated, last, places, problem}`: `fields` are those the
Project holds as its columns, `stated` whether `[github-project] fields`
names them (else they are the default seven); `project` is `owner/number`, `""` when
`[github-project]` names none (and `problem` when the table doesn't
read); `last` the last run, `null` before any, `{project, title,
checked_at, side, applied, write, truncated, items, skipped, columns,
counts, conflicts, unmatched, changes}`. `columns` are the Project's
fields by name, with their type (`SINGLE_SELECT`, `ITERATION`,
`NUMBER`, `DATE`, `TEXT`, GitHub's own). `counts` is `{matched,
label_changes, project_writes, conflicts, warnings, unmatched}`. A
conflict is `{issue, number, url, title, field, base, has_base, local,
project}`: `local` git-board's value, `project` the Project's, `""`
unset. A change is `{issue, number, url, title, added, removed, writes,
warnings}`.

`places` say **where each field lives**, on each forge of the board
(its `[forges]` tables, else GitHub's): `[{field, forge, kind, to,
default, column, on_project}]`. `to` is the field's target (its
mapping's, else the Project's own columns once `[github-project]` names
one, else its type's `default`); `column` the Project's column of a
field that lives there, and `on_project` whether the last run found it
there (absent before any). Settings › Fields shows them.

`check` plans a sync both ways, and writes nothing. `apply` runs one:
`side` is `labels` (`project pull`), `project` (`project push`) or
`both` (`project sync`, the default); writing to the Project needs
`write = true`. `resolve` is `{issue, field, keep}`: `issue` is its
number on GitHub, `owner/repo#n` or its URL; `keep` is `git-board`
(written to the Project, with `write`) or `project` (set on the label);
it answers `kept`, the value both now hold, after checking again. A
field in no conflict is `not-found`; a value the Project has no option
for, or a number two repositories have in conflict, `invalid`. One
action at a time: another is `busy`.

### The cycle (49, 50)

Experimental (ADR 0018): off, both answer 404 `experimental`. Built in
`app` (`cycle.go`, `cycleact.go`, `pulls.go`) on
`domain/cycle`.

**The read (49)** is `{step, at, of, name, stage, done, gate, risk,
skipped, note, waits, mine, actions, evidence, id, version,
body_version, pr_checked_at, environments}`, with the issue's `version` as `ETag`: an
action's `If-Match`. `evidence` is `cycle.Input`: the status, the
criteria, the people holding each gate, the lanes (each branch's step,
tests and PR chip), the plan (its `html` rendered and cleaned as a
thread's texts: a plan comes from any branch), the runs, checks and sign-offs, and `pr`,
the evidence branch's pull request as the forge gates it (`{number, url,
state, draft, title, body, base, head, merge_commit, author, decision,
mergeable, merge_state, checks, reviews, requested, can_merge, checking,
waits}`, GitHub's words; a GitLab merge request in them). `actions` are
what the viewer can press now. `body_version` is the description's
version, what `spec-verdict`'s `on` names. `pr_checked_at` is when the PR was last
read from the forge. `environments` are `[release]`'s, `[{name, version,
in}]`: `in` when the environment runs one of the issue's merged
branches (the trunk's records, no fetch); `[]` without `[release]`.

**The PRs** are read through the forge layer (`forge.Puller`: GitHub's
GraphQL and REST, GitLab's merge requests) on origin's forge, with the
token of the person who writes, and kept by the server for its life
(`app.PullCache`): a read asks the forge again for a branch's PR once its
answer is 5 minutes old, within 10 seconds, and offline the last answer
stands. A sync's end and a fetch or pull of a remote make every answer
stale; `refresh` asks now. The Work page reads the cache, and asks
nothing.

**An action (50)** is `POST` with `If-Match` (the issue's writes
compare-and-swap from it; 412 `stale`), and a body with the fields its
action reads. It answers the read (49) after it, and its `ETag`.

| Action | Body | Writes |
|---|---|---|
| `no-criteria` | | accepts the issue without criteria (`intent:criteria=none`): its author's, the spec role's, or anyone's without one |
| `spec-verdict` | `{verdict: approve or request-changes, on: "body@v<n>", body}` | the spec role's verdict on the description's version (`changed` on another); an approval moves the status to `ready`. The author doesn't approve their own |
| `set-risk` | `{risk: low, normal or structural}` | the `risk` field, by the risk role when one holds it |
| `move-ready`, `start`, `to-validation` | | the status to `ready`, the first in-progress, the first validation status, when `actions` has it |
| `approve-plan` | | `plan:approve` at the head; at `ready`, the status to in-progress. A structural plan is approved on the forge |
| `ready-for-review` | | the status to in-review, when the signed check results at the head prove it (ADR 0001; `invalid` with the reason when not) |
| `review` | `{state: APPROVE, REQUEST_CHANGES or COMMENT, body}`, or a line comment `{path, line, side: old or new, body}`, or a reply `{reply_to, body}`; `commit` (the head by default; none without a branch) | a comment with `review:*` metadata, `review:pr` the PR's, which the sync mirrors |
| `resolve` | `{comment, resolved}` | `review:resolved` on a thread's first line comment |
| `open-pull` | `{branch}` | a pull request from one of the issue's branches on origin into its base, titled from the issue, `Refs: <number>` |
| `edit-pull` | `{number, title, body}` | one of the issue's PRs' title and description |
| `merge` | `{number, sha}` | merges it with a merge commit if its head is still `sha` |
| `verify` | `{output, commit}` | a run's output, at the head by default (`verify:run`) |
| `check` | `{index}` | one criterion checked (`verify:criterion`), its box ticked in the description |
| `signoff` | `{why}` | the sign-off of the validation status (`signoff:status`), then the next one or the closing status, which closes the issue. Refused (`invalid`) without a run at the head, or once merged on a base declaring checks, without their proof at the merge, unless `why` overrides |
| `refresh` | | nothing: the PRs asked of the forge now |

A step that waits for someone else is `forbidden`, naming them; a step
the issue isn't at, or a wrong value, `invalid`; an unknown action
`not-found`. The forge's refusal (a moved head) is `invalid`, its
failure `internal`.

Not ported: v1's `suggest-risk` (an AI's class: the agents are their
own binary), the tests' run and their log (`run-tests`, signing the
results: `git board verify` and the proof commands), the changes' diff
(`?changes=`, `?file=`).

The release setup, the overlays' adoption, the checks' setup, the
changes and the diff of an issue's branch, the review, the label sync
with GitHub and lint go under these same nouns when M6 builds them:
`/api/releases/setup` is `[release]` (settings), a review comment is a
comment with `meta`, a diff is the cycle's.

## Change events

`GET /api/events` is one Server-Sent Events stream. A page holds it
open, and reloads what an event names. Each event is a `data:` line of
JSON:

```
retry: 3000

data: {"kind":"issue","id":"a9f3…","version":"5c1e…","by":"k3j2h1"}

: ping
```

| `kind` | When | `id` |
|---|---|---|
| `issue` | an issue was written (a delete too: a tombstone is a version), here or by anyone else (a command, a fetch, a sync) | the issue, with its `version`; none when its ref is gone |
| `person` | a person was written | the person |
| `config` | `.git-board.toml` changed | |
| `me` | who writes, or `board.avatars`, changed | |
| `inbox` | what you read or follow | |
| `sync` | a run started or ended; a remote moved | the run |
| `progress` | a run's step, in `step` | the run; `issue` when it is one issue's |
| `release` | a release job's step, its end, or a write on a record | the tag, when known |

`by` is the `X-Git-Board-Client` of the request that wrote it, when one
did. The server sees every change, whoever made it: a write through the
API is told at once, by the kernel's observer, and the server compares
the datastore's heads every second for the others, which it tells a
second later (an API write lands in the datastore just before its
observer runs: the wait lets the observer tell it, with its `by`).
`.git-board.toml` written by hand is seen the same way: the board opens
again on it, and `config` is told. A write of it through the API (24,
25, 33 to 35) opens the board once and is told once, with its `by`: the
watcher knows the server's own write. A page that falls 64 events behind is dropped: its stream closes,
it reconnects (`retry`) and reloads. A comment line every 25 seconds
keeps proxies from closing the stream.

## Logs

The server writes its logs to stderr, through Go's `log/slog`: one line
an event, as text (`key=value`) or JSON.

| Flag | Variable | Default |
|---|---|---|
| `--log-level debug, info, warn, error` | `GIT_BOARD_LOG_LEVEL` | `info` |
| `--log-format text, json` | `GIT_BOARD_LOG_FORMAT` | `text` |

The flag wins over the variable.

| Level | What |
|---|---|
| `info` | the start: `version`, `board`, `datastore`, `where`, `url`; one line a request: `method`, `path`, `status`, `duration`, `bytes`, and `code` when it failed; a run's start and end |
| `warn` | a request refused by the host or origin checks (`host` or `origin`, `why`); a request slower than a second; a sync's warnings (a conflict, a forge left out, creations held), and a whole sync's step that failed; what a read leaves aside (an entity broken), as the CLI tells it |
| `error` | a request that failed on the server's side (500), with its `code` and message; a run that failed, with its `code` |
| `debug` | a request's query string, redacted; each change event sent; each port tried taken |

A request's line never holds a header, a cookie, a token or a body: the
path is logged without its query. At `debug` the query is logged, every
value whose key holds `token`, `key`, `secret`, `password`, `pass`,
`auth` or `credential` replaced by `REDACTED`. A datastore's URL is
logged without its password, and a forge's error without the
credentials of a URL.

## v1's routes, and where they went

117 routes in v1 (`internal/server/server.go`); 54 in v2: 32 built in
M5, 7 *later* and 14 with their domain in M6 (the GitHub Project's two
built), and the forge-mapping migration (54), ported for v0.12. Five
more came with the web app's settings forms (2026-11-07 and 08): the
tokens (55, 59), init (56), a git remote (57), and the Project named
(58). The board's state (60) came on 2026-11-09.

| v1 | v2 |
|---|---|
| `GET /api/events` | `GET /api/events` |
| `GET /api/repository` | `GET /api/board` |
| `GET /api/datastore` | `GET /api/datastore` |
| `PUT /api/datastore` | `PUT /api/datastore` (*later*) |
| `POST /api/datastore/test` | `PUT /api/datastore?dry_run=1` (*later*) |
| `POST /api/datastore/copy` | `POST /api/datastore/copy` |
| `GET /api/views` | `GET /api/config`: views are `[views]` keys |
| `PUT /api/views/{name}` | `PATCH /api/config` |
| `GET /api/config/fields` | `GET /api/fields` |
| `GET /api/config/labels` | `GET /api/labels` |
| `GET /api/colors` | `GET /api/fields`: each field's `colors` |
| `GET /api/issues` | `GET /api/issues`, paged; `group` dropped: a column is a list |
| `GET /api/facets` | `GET /api/labels?filter=` |
| `GET /api/issues/{id}` | `GET /api/issues/{id}` |
| `PATCH /api/issues/{id}` | `PATCH /api/issues/{id}` |
| `POST /api/issues` | `POST /api/issues` |
| `GET /api/issues/{id}/thread` | `GET /api/issues/{id}`: an issue is its thread |
| `POST /api/issues/{id}/sync` | `POST /api/sync` with `issue` (*later*) |
| `GET /api/issues/{id}/refs` | `GET /api/issues/{id}/cycle` (*M6*) |
| `GET /api/issues/{id}/branches` | `GET /api/issues/{id}/cycle` (*M6*) |
| `POST /api/issues/{id}/branches` | `POST /api/issues/{id}/cycle/branch` (*M6*) |
| `GET /api/issues/{id}/environments` | `GET /api/issues/{id}/cycle` (built) |
| `GET /api/issues/{id}/pulls` | `GET /api/issues/{id}/cycle` (built) |
| `POST /api/issues/{id}/pulls` | `POST /api/issues/{id}/cycle/open-pull` (built) |
| `POST /api/issues/{id}/pulls/{number}/merge` | `POST /api/issues/{id}/cycle/merge` (built) |
| `PATCH /api/issues/{id}/pulls/{number}` | `POST /api/issues/{id}/cycle/edit-pull` (built) |
| `GET /api/issues/{id}/cycle` | `GET /api/issues/{id}/cycle` (built) |
| `POST /api/issues/{id}/cycle/{action}` | `POST /api/issues/{id}/cycle/{action}` (built) |
| `GET /api/issues/{id}/tests` | `GET /api/issues/{id}/cycle` (*M6*) |
| `POST /api/issues/{id}/tests/run` | `POST /api/issues/{id}/cycle/run-tests` (*M6*) |
| `GET /api/issues/{id}/tests/log/{hash}` | `GET /api/issues/{id}/cycle?log=<hash>` (*M6*) |
| `GET /api/checks/setup` | `GET /api/config`: the checks are a table (*M6*) |
| `POST /api/checks/setup` | `PATCH /api/config` (*M6*) |
| `GET /api/issues/{id}/changes` | `GET /api/issues/{id}/cycle?changes=<range>` (*M6*) |
| `GET /api/issues/{id}/changes/file` | `GET /api/issues/{id}/cycle?file=<path>` (*M6*) |
| `POST /api/issues/{id}/review` | `POST /api/issues/{id}/cycle/review` (built) |
| `POST /api/issues/{id}/review/{comment}/resolve` | `POST /api/issues/{id}/cycle/resolve` (built) |
| `GET /api/issues/{id}/agent` | dropped: agents move to their own binary (ADR 0021), which calls this API |
| `POST /api/issues/{id}/agent/{action}` | dropped: the same |
| `POST /api/issues/{id}/comments` | `POST /api/issues/{id}/comments`; its `status` is `PATCH` with `comment` |
| `PUT /api/issues/{id}/comments/{comment}` | `PATCH /api/issues/{id}/comments/{comment}` |
| `POST /api/issues/{id}/comments/{comment}/meta` | `PATCH /api/issues/{id}/comments/{comment}` with `meta` (*M6*) |
| `PUT /api/issues/{id}/title` | `PATCH /api/issues/{id}` with `title` |
| `PUT /api/issues/{id}/status` | `PATCH /api/issues/{id}` with `status` |
| `POST /api/issues/{id}/labels` | `PATCH /api/issues/{id}` with `labels` |
| `GET /api/me` | `GET /api/me` |
| `POST /api/me` | `PUT /api/me` |
| `PUT /api/me/view` | dropped: the classic and cycle pages are one in the new web app; `board.view` is the cycle's (*M6*) |
| `PUT /api/me/avatars` | `PUT /api/me` with `avatars` |
| `GET /api/inbox` | `GET /api/inbox` |
| `GET /api/work` | `GET /api/cycle` (`who`): the cycle's rows; its agents dropped, the forges' pending its Sync |
| `POST /api/inbox/seen` | `POST /api/inbox/seen` |
| `GET /api/inbox/follow/{id}` | `GET /api/issues/{id}/follow` |
| `PUT /api/inbox/follow/{id}` | `PUT /api/issues/{id}/follow` |
| `PUT /api/inbox/settings` | `PUT /api/inbox/settings` |
| `GET /api/media` | `GET /api/media` (*M6*) |
| `POST /api/markdown` | `POST /api/markdown` |
| `GET /api/uploads` | `GET /api/uploads` (*M6*) |
| `POST /api/uploads` | `POST /api/uploads` (*M6*) |
| `GET /api/insights` | `GET /api/insights` (*M6*) |
| `GET /api/report` | `GET /api/insights?sprint=` (*M6*) |
| `GET /api/sprints` | `GET /api/fields`: an iteration's `options` and `current` |
| `GET /api/settings` | `GET /api/config`, `GET /api/sync` |
| `GET /api/config` | `GET /api/config` |
| `POST /api/config/check` | `PUT /api/config?dry_run=1` |
| `GET /api/config/docs` | `GET /api/docs/configuration` (*later*) |
| `GET /api/config/model` | `GET /api/config`: `keys` |
| `GET /api/fields/usage` | `GET /api/labels`: each value's count |
| `GET /api/fields/migrate` | `POST /api/fields/{name}/migrate?dry_run=1`: the forge-mapping migration's preview |
| `POST /api/fields/migrate` | `POST /api/fields/{name}/migrate`: the forge-mapping migration (`sync --migrate`), not a renaming of values (that is `set --filter`) |
| `PUT /api/config/section/{section}` | `PATCH /api/config` |
| `PUT /api/config` | `PUT /api/config` |
| `PUT /api/bridges/{name}/sync` | `PATCH /api/forges/{name}` |
| `GET /api/credentials` | `GET /api/forges/credentials` |
| `POST /api/bridges` | `POST /api/forges` |
| `DELETE /api/bridges/{name}` | `DELETE /api/forges/{name}` |
| `GET /api/forges` | `GET /api/sync`: `forges` and `run` |
| `GET /api/forges/docs` | `GET /api/docs/forges` (*later*) |
| `POST /api/forges/sync` | `POST /api/sync` with `scope: forges` |
| `GET /api/remotes` | `GET /api/sync`: `remotes` |
| `POST /api/remotes/{name}/{action}` | `POST /api/remotes/{name}/{action}` |
| `GET /api/numbers` | `GET /api/numbers` (*later*) |
| `POST /api/numbers/init` | `POST /api/numbers` with `{prefix, start}` |
| `POST /api/numbers/run` | `POST /api/numbers` with `{}` |
| `GET /api/labels` | dropped from the core: GitHub's labels against the fields is `lint`'s, a tool (*M6*); `/api/labels` now counts the board's |
| `POST /api/labels/send` | dropped: the same (*M6*, `lint --create-labels`) |
| `POST /api/labels/adopt` | dropped: the same; adopting a color is `PATCH /api/config` |
| `POST /api/labels/delete` | dropped: the same |
| `GET /api/lint` | dropped: `lint` is a tool (*M6*) |
| `GET /api/worktrees` | dropped: `wtx` moves to its own binary (ADR 0021) |
| `POST /api/worktrees/preview` | dropped: the same |
| `POST /api/worktrees/init` | dropped: the same |
| `GET /api/sync` | `GET /api/github-project`: v1's `/api/sync` was the Project's |
| `POST /api/sync/check` | `POST /api/github-project/check` |
| `POST /api/sync/apply` | `POST /api/github-project/apply` |
| `POST /api/sync/resolve` | `POST /api/github-project/resolve`: `keep` the same, `issue` for `number` |
| `PUT /api/github-project` | the same route, 58: `{owner, number}` |
| `DELETE /api/github-project` | `PATCH /api/config` with `null` |
| `GET /api/start-over` | dropped: starting over is the CLI's (`git board reset`), a rare and destructive act |
| `POST /api/start-over` | dropped: the same |
| `GET /api/sync/all` | `GET /api/sync` |
| `POST /api/sync/all` | `POST /api/sync` with `scope: all` |
| `GET /api/releases` | `GET /api/releases` (*M6*) |
| `GET /api/releases/draft` | `GET /api/releases/next` (*M6*): the tag `next` |
| `POST /api/releases` | `POST /api/releases` (*M6*) |
| `GET /api/releases/job` | `GET /api/releases` (the job in it) and `progress` events (*M6*) |
| `GET /api/releases/setup` | `GET /api/config`: `[release]` |
| `POST /api/releases/setup` | `PATCH /api/config` |
| `GET /api/releases/adopt` | `GET /api/releases`: the proposals in it (*M6*) |
| `POST /api/releases/adopt` | `PATCH /api/releases/{tag}` (*M6*) |
| `GET /api/releases/{tag}` | `GET /api/releases/{tag}` (*M6*) |
| `POST /api/releases/{tag}/promote` | `PATCH /api/releases/{tag}` with `state` (*M6*) |
| `PUT /api/releases/{tag}/notes` | `PATCH /api/releases/{tag}` with `notes` (*M6*) |
| `POST /api/releases/{tag}/comments` | `POST /api/releases/{tag}/comments` (*M6*) |
| `POST /api/releases/{tag}/yank` | `PATCH /api/releases/{tag}` with `state` (*M6*) |
| `POST /api/environments/{env}/deploy` | `POST /api/environments/{env}/deploy` (*M6*) |
| `POST /api/environments/{env}/rollback` | `POST /api/environments/{env}/deploy` with `reason` (*M6*) |

What the table drops, in short:

- **One route per field** (`/title`, `/status`, `/labels`): one `PATCH`.
- **A thread apart from its issue**: an issue's read is its thread.
- **Checks apart from writes** (`/config/check`, `/datastore/test`):
  `dry_run` on the write.
- **Settings forms** (`/config/section`, `/views/{name}`,
  `/github-project`, `/releases/setup`, `/checks/setup`): keys of
  `.git-board.toml`, one `PATCH`.
- **Status pages** (`/settings`, `/forges`, `/remotes`, `/sync/all`):
  one `GET /api/sync`.
- **Agents and `wtx`**: their own binary, a client of this API.
- **Start over and the field migration**: the CLI's.

## Serving

`git board serve` serves the API and the web app, embedded in the
binary (ADR 0012: `web`'s build, [the web app](web.md)), until
Ctrl-C. A bare `git board` opens the board in the browser: the one
already running for this repository if there is one, else it starts one
here. It listens on `localhost`, port 8080 or the next one free on both
loopback addresses; `--addr` chooses (an address other machines reach
exposes the board and the forges' tokens to them). The running board is
found by `<git dir>/git-board/server.json` (its URL and pid), checked
against the `git_dir` its `/api/board` answers.

The web app is `web/` ([the web app](web.md)); v1's was deleted at the
switch (M7).
