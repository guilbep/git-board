# Issues from the command line

Everything you do to an issue on a forge, you can do with `git board`,
offline: create one, read it with its comments, comment, edit, set its
fields, close and reopen it. Every change is written to the board at once
and reaches the forges with the next `git board sync`. No command but a
sync touches the network.

`git board X` and `git-board X` are the same command: the binary is a git
subcommand.

## Naming an issue

Every command taking `<id>` takes any of:

| Form | Example | Note |
|---|---|---|
| a prefix of its id | `a1b2c3d` | 7 characters or more, as `ls` shows them |
| its project number | `CO2-3075` | once the project numbers its issues (`git board numbers`) |
| a forge's number | `"#2528"` | quoted: `#` starts a shell comment |

A prefix matching two issues is refused (exit code 3); give more of it.

## Who you write as

Every change is signed with a person of the board. A fresh board has none
chosen, and every write fails until you choose (exit code 4):

```bash
git board me                 # who you write as
git board me --new           # create you from git user.name and user.email
git board me --new --name "Ada Lovelace" --email ada@example.com
git board me --ls            # everyone on the board, * marks you
git board me 3f2a9c1         # write as someone already there (a forge sync made them)
```

`--new` refuses to create a second person with your name and email: it
names the one there, to choose with `git board me <id>`. The choice is
kept in the clone's git config (`board.me`).

| Flag | |
|---|---|
| `--new` | create a person and write as them |
| `--name`, `--email` | with `--new`: instead of git's `user.name`, `user.email` |
| `--ls` | list everyone on the board |
| `--json` | `{"id", "name", "login", "avatar_url"}`; with `--ls` an array, `"me": true` on you |

## Texts: `-b`, `-F`, `-e`

`new`, `comment` and `edit` take a text the way `gh` does:

| Flag | |
|---|---|
| `-b, --body <text>` | the text itself |
| `-F, --body-file <file>` | read it from a file; `-` reads stdin |
| `-e, --editor` | write it in your editor: git's (`GIT_EDITOR`, `core.editor`, `VISUAL`, `EDITOR`) |

With `-e`, `new` and `edit` put the title on the first line and the
description below it. A text saved empty writes nothing. When a write is
refused after you wrote the text, the error names a file keeping it:
nothing you typed is lost.

## `new`: create an issue

```bash
git board new "Login fails on Safari"
git board new "Login fails on Safari" prio=p1 assignee=+@me sprint=@current
git board new "Export to CSV" -b "As ls --md, but CSV."
git board new "Crash report" -F crash.md label=+bug
git board new -e prio=p2
id=$(git board new "Follow-up")
```

The title is the first argument (or `-t, --title`), then `field=value`
assignments, read and checked as [`set`](#set-fields) reads them, written
in the same change as the issue. A first argument that reads as
`field=value` is an assignment: a title like `a=b` goes with `--title`.

It prints the new issue's id alone on its line. `--json` prints the issue
as `ls --json` prints each one. `--force` writes values the config
doesn't allow; `--me` is `@me`.

## `show`: an issue, with its comments

```bash
git board show a1b2c3d
git board show CO2-3075 --md
git board show "#2528" --json
git board show a1b2c3d --no-comments
```

Title; number, id, state, author, date and comment count; the fields, the
plain labels, the inherited ones and the forges; the description; then
each comment, headed by its author, date, `(edited)` and its ref
`[abc1234:1]`, which `comment --edit` takes.

| Flag | |
|---|---|
| `--md` | as Markdown: a heading, a fields table, a section per comment |
| `--json` | the whole thread (below) |
| `--no-comments` | leave the comments out |
| `--at <when>` | as it was then (see [The board as it was](#the-board-as-it-was)) |

`get <id>` stays what it was: the issue's labels, one per line, for
scripts.

## `comment`: comment, or edit a comment

```bash
git board comment a1b2c3d -b "Fixed in 4e5f6a7"
git board comment a1b2c3d                       # the editor, as git commit
git log -1 --format=%B | git board comment a1b2c3d -F -
git board comment a1b2c3d --edit-last
git board comment a1b2c3d --edit 9c8b7a6:1 -b "Fixed in 4e5f6a7, in v1.2"
```

With no text flag the editor opens. `--edit <ref>` edits a comment by the
ref `show` prints (a prefix of its hash is enough when it names one
comment); `--edit-last` edits your last comment on the issue. The editor
opens on the comment's text.

An edit carries the text you started from. If someone changed the
comment meanwhile, here or through a sync, your edit is refused with exit
code 5 instead of replacing theirs, and your text is kept in a file the
error names.

It prints the comment's ref; `--json` prints `{"id", "ref", "issue"}`.
The description is not a comment: `edit` changes it.

## `edit`: title and description

```bash
git board edit a1b2c3d --title "Login fails on Safari 17"
git board edit a1b2c3d -b "Steps: ..."
git board edit a1b2c3d -e                       # title and description, one buffer
```

Refused, exit code 5, when the title or the description changed since
you read it; your text is kept. Every earlier version stays in the
issue's history (`git board log`).

## `set`: fields

```bash
git board set a1b2c3d status=ready prio=p1
git board set a1b2c3d assignee=+@me             # add yourself
git board set a1b2c3d assignee=-guilbep         # remove
git board set a1b2c3d prio=                     # clear the field
git board set a1b2c3d label=+regression         # a plain label
git board set a1b2c3d milestone=v1 --force      # a field nothing configures yet
git board set --filter "sprint:@current status:ready no:assignee" assignee=+@me --apply
```

| Assignment | |
|---|---|
| `field=value` | a single field (`status`, `prio`) swaps to value; a multi field (`assignee`) adds it |
| `field=+value` | a multi field adds value (refused on a single field) |
| `field=-value` | removes value, from either |
| `field=` | clears the field: every value it holds goes |
| `label=+name`, `label=-name` | adds or removes the plain label `name` |

Values resolve first: `@me` (`--me`, `GIT_BOARD_ME`, else who you write
as: their forge login, else their name, so `assignee=+@me` works on a
board that never met a forge), `@current`, `@next`, `@previous`
(sprints), and a configured alias.

Every assignment is read before anything is written. One that is
malformed (`=x`, `status=-`, a space around the value) is refused. One the
config doesn't allow is refused too: a value outside the field's
`options`, not a number for a number field, not a `YYYY-MM-DD` date for a
date field, not a sprint for a sprint field, a plain label outside
`[labels]` when the project lists them, a field neither configured nor on
any issue. Nothing is written then, and the exit code is 2. `--force`
writes it anyway. A removal or a clear is never refused: it is how a
wrong value goes.

All the assignments land in one change. Setting `status` to its last
option closes the issue, as `close` does; another option reopens a closed
one: the state and the status never disagree.

`--filter <filter>` sets the fields on every issue the filter matches
(`ls`'s grammar). It prints what each issue would get and writes nothing
until `--apply`.

## `close`, `reopen`, `move`

```bash
git board close a1b2c3d -c "Fixed in 4e5f6a7"
git board reopen a1b2c3d -c "Still fails on Safari"
git board move a1b2c3d --next
git board move a1b2c3d --prev
```

`close` sets the status to its last option and closes the issue.
`reopen` opens it and, when its status is that last option or none, sets
the first option; another status stays. Both take `-c, --comment`, added
first. `move --next` and `--prev` step the status through its options;
onto the last one it closes the issue, back from it it reopens.

## `log`: everything that happened to an issue

```bash
git board log a1b2c3d
git board log a1b2c3d --field status            # its path through the workflow
git board log CO2-3075 --since 7d
git board log a1b2c3d --json
```

The board keeps every operation, with who made it and when: the
creation, each comment, each edit of the description or a comment (with
the text), each title, state and label change, in the order they were
made. A forge shows label events at best, and keeps no earlier text.

| Flag | |
|---|---|
| `--field <name>` | only that field's changes: who set `prio:p0`, when, what it was before |
| `--since <when>` | from a date (`2026-10-01`) or a while ago (`36h`, `7d`) |
| `--json` | the entries (below) |

`history` is the same command.

## The board as it was

```bash
git board show a1b2c3d --at 2026-10-01          # the issue at the end of that day
git board ls "sprint:@current" --at 2026-10-01  # the board that day
git board diff a1b2c3d --since 2026-10-01       # what changed since
git board diff CO2-3075 --since 7d --json
```

The board keeps every operation with its time, so it can show what it
was. `show --at` and `ls --at` fold only the operations made by then.

- **Each operation counts at its own time**, its writer's clock, to the
  second. The log still applies them in its own order (by clock), so the
  past is what the people writing saw. When clocks disagree, an edit of a
  comment made "before" the comment waits for it.
- **`<when>`** is a date, a time (`2026-10-01T14:30`, or with seconds, or
  RFC 3339 with a zone), or a while ago (`36h`, `7d`, `2w`). For `--at`, a
  date or a minute means its end: `--at 2026-10-01` is that evening.
- **`ls --at`** lists the issues created by then, each as it was. `@today`
  and `@current` mean that day and that day's sprint. Free text looks in
  the texts as they were. Who you are, what you follow and who is
  mentioned are as now.
- **An issue not created yet** is not found (exit code 3).
- **What waits for a forge** isn't marked: that is now's.

`diff <id> --since <when>` compares the issue before `--since` (a date
from its start) with now: the title, the state, the labels added and
removed, the description as a line diff (`- ` removed, `+ ` added), the
comments written, the comments edited. An issue created since says so,
and shows everything as new. `--json` prints:

| Field | |
|---|---|
| `id`, `since` | the issue, and from when |
| `created` | true when the issue is newer than `since` |
| `title`, `status`, `description` | `{"was", "now"}` when they changed; `description` with its `diff` |
| `added`, `removed` | labels |
| `comments` | the comments written since: `{"id", "author", "at", "body"}` |
| `edited_comments` | the comments edited since, with `was`, their text before |

## `ls` and `view`

```bash
git board ls "assignee:@me is:open" --sort "prio asc"
git board ls --sort "updated desc" --limit 10
git board ls "login" --json
git board view backlog --md
```

`ls [filter]` lists the issues a filter matches; the grammar is in
[Configuration › Filters](configuration.md#filters): fields, people,
words (`in:title`), dates (`created:>=@today-7d`, `closed:2026-10-01..2026-10-31`)
and counts (`comments:>5`). `--at <when>` lists the board as it was. `--sort` takes `created`,
`updated`, `title` or a field, then `asc` or `desc`; any other key or
direction is refused (exit code 2). `-L, --limit` keeps the first N after
sorting. `--group <field>` groups, `--md` and `--json` render. `view
<name>` runs a view from `.git-board.toml`.

## JSON output

The field names below are stable: new ones may come, these keep their
names and meanings. Times are RFC 3339.

**An issue** (`ls --json`, `view --json`, `new --json`, and the top of
`show --json`):

| Field | |
|---|---|
| `id` | the whole id, 40 hex characters |
| `title` | as stored; `shown_title` without a trailing `(#n)` of its own forge number |
| `status` | `open` or `closed` |
| `labels` | its own labels: `status:ready`, `assignee:ada`, `bug` |
| `inherited` | labels it takes from its project's roles (when any) |
| `number` | its project number, `CO2-3075` (when numbered) |
| `forges` | `[{"name", "url", "number"}]`, where it is on forges (when any) |
| `github_url` | its GitHub issue (when any) |
| `created_at`, `updated_at` | |
| `closed_at` | when it was last closed (closed issues only) |
| `comments` | how many comments it has |

**A thread** (`show --json`) is the issue plus `author` (a person),
`edited_at`, `participants` (people) and `events`. Each event has `kind`
(`created`, `comment`, `labels`, `status`, `title`), `id`, `author` and
`at`; then as its kind has them `body`, `edited` and `history` (the
text's versions, `[{"id", "author", "at", "body"}]`), `added` and
`removed` (labels), `status`, `title` and `was`, `meta`. A person is
`{"id", "name", "login", "avatar_url"}`.

**A log entry** (`log --json`): `at`, `author`, `kind` (`created`,
`comment`, `edit`, `title`, `state`, `labels`), `id`; then `target` (an
edit's text: the `created` entry's id for the description), `title` and
`was`, `state` (`open`, `closed`), `added` and `removed`, `body` (the text
written).

## Exit codes

| Code | |
|---|---|
| 0 | done; an empty `ls` too |
| 1 | an error not below |
| 2 | a wrong command line: a flag, an argument, an assignment malformed or not allowed by the config |
| 3 | no such issue, comment or person, or a prefix naming several |
| 4 | no one chosen to write as: `git board me` |
| 5 | someone changed the text meanwhile: your edit is refused, your text kept |

Errors go to stderr, prefixed `error:`.

## Coming from gh / glab

| gh | glab | git board |
|---|---|---|
| `gh auth status` | `glab auth status` | `git board me` (who you write as; forge tokens only matter to `sync`) |
| `gh issue create -t T -b B -l bug -a @me` | `glab issue create -t T -d B -l bug -a @me` | `git board new T -b B label=+bug assignee=+@me` |
| `gh issue create -e` | `glab issue create` | `git board new -e` |
| `gh issue list -a @me -l bug -S text` | `glab issue list -a @me -l bug --search text` | `git board ls "assignee:@me label:bug text"` |
| `gh issue list -L 10 --json …` | `glab issue list -P 10 -O json` | `git board ls -L 10 --json` |
| `gh issue list -S "created:>2026-10-01 comments:>5"` | | `git board ls "created:>2026-10-01 comments:>5"` |
| `gh issue list -S "crash in:title"` | | `git board ls "crash in:title"` |
| `gh issue view N -c` | `glab issue view N -c` | `git board show N` |
| `gh issue view N --json …` | `glab issue view N -F json` | `git board show N --json` |
| `gh issue comment N -b B` | `glab issue note N -m B` | `git board comment N -b B` |
| `gh issue comment N --edit-last` | | `git board comment N --edit-last` |
| `gh issue edit N -t T -b B` | `glab issue update N -t T -d B` | `git board edit N --title T -b B` |
| `gh issue edit N --add-label x --remove-label y` | `glab issue update N -l x -u y` | `git board set N label=+x label=-y` |
| `gh issue edit N --add-assignee @me` | `glab issue update N -a @me` | `git board set N assignee=+@me` |
| `gh issue edit N -m v1` | `glab issue update N -m v1` | `git board set N milestone=v1` (a field), or `sprint=@next` |
| `gh issue edit 1 2 3 --add-label x` | | `git board set --filter "<filter>" label=+x --apply` |
| `gh issue close N -c C` | `glab issue close N` | `git board close N -c C` |
| `gh issue reopen N` | `glab issue reopen N` | `git board reopen N` |
| `gh issue status` | | `git board ls "involves:@me is:open"` |
| `gh issue develop N` | `glab mr create --related-issue N` | `git board branch N` (prints the branch and the command) |
| | `glab issue view N -s` (system notes) | `git board log N` (every change, text edits too) |
| | | `git board show N --at 2026-10-01`, `git board ls --at 2026-10-01` (the board as it was) |
| | | `git board diff N --since 7d` (what changed: fields, a text diff, new comments) |
| `gh project item-edit …` | `glab issue board …` | `git board move N --next`, `git board project` |

`N` is any [name of an issue](#naming-an-issue). Not there yet: deleting
an issue or a comment, templates, pinning, locking (no permissions in a
board kept in git), `--web`, `--jq` and `--template`, shell completion of
ids and values.
