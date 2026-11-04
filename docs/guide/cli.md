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

`--delete <ref>` deletes a comment: see [Deleting](#delete-an-issue-or-a-comment).

## `delete`: an issue or a comment

```bash
git board delete a1b2c3d                        # asks first
git board delete a1b2c3d --yes
git board comment a1b2c3d --delete 9c8b7a6:1 --yes
```

A deleted issue leaves every list, search, page, export and sync; its id
answers not found (exit code 3). A deleted comment leaves its issue's
page, its log and search.

A delete hides; it doesn't erase. The text stays in the board's history,
in every clone that fetched it, and on the forges it was synced to: a
forge keeps its copy. **A secret pasted into an issue is leaked: revoke
it.** Deleting it keeps it from being shown, searched or synced again. A
pull never brings a deleted issue or comment back.
[ADR 0019](../adr/0019-deleting-is-a-tombstone.md) says why.

On a terminal it asks first. `--yes` doesn't ask, and is needed when no
one is at a terminal (a script, CI): without it, exit code 2. Deleting is
in the command line only, not yet in the web UI.

## `undo`: your last change

```bash
git board undo a1b2c3d              # your last change to it
git board undo                      # your last change, on any issue
git board undo a1b2c3d --force      # though someone wrote after you
```

It writes the inverse of your last change: a label added is removed and
one removed added back; a status, a title or a text goes back to what it
was; a new comment is deleted. A change is what one command wrote, so
`edit -e`'s title and description go back together. Undoing twice redoes.
Nothing is erased: the change and its undo are both in `git board log`.

Refused, exit code 2, with the reason:

- creating an issue: `git board delete` deletes it;
- a delete: never undone;
- a change of metadata (a review thread resolved, a Project field);
- someone else wrote to the issue after you, or a sync pulled into it:
  `--force` undoes yours anyway, over theirs.

Changes a sync pulled are not yours to undo here. `--json` prints
`{"issue", "title", "undone", "what"}`: the operations undone and a line
for each thing put back.

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

## `ls` and `view`

```bash
git board ls "assignee:@me is:open" --sort "prio asc"
git board ls --sort "updated desc" --limit 10
git board ls "login" --json
git board view backlog --md
```

`ls [filter]` lists the issues a filter matches; the grammar is in
[Configuration › Filters](configuration.md#filters). `--sort` takes `created`,
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
| `gh issue delete N --yes` | `glab issue delete N` | `git board delete N --yes` (hidden, kept in history) |
| `gh issue comment N --delete-last` | `glab issue note delete` | `git board comment N --delete <ref>` |
| | | `git board undo N` (your last change, inverted) |
| `gh issue status` | | `git board ls "involves:@me is:open"` |
| `gh issue develop N` | `glab mr create --related-issue N` | `git board branch N` (prints the branch and the command) |
| | `glab issue view N -s` (system notes) | `git board log N` (every change, text edits too) |
| `gh project item-edit …` | `glab issue board …` | `git board move N --next`, `git board project` |

`N` is any [name of an issue](#naming-an-issue). Not there yet: templates, pinning, locking (no permissions in a
board kept in git), `--web`, `--jq` and `--template`, shell completion of
ids and values.
