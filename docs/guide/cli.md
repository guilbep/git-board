# Issues from the command line

Everything you do to an issue on a forge, you can do with `git board`,
offline: create one, read it with its comments, comment, edit, set its
fields, close and reopen it. Every change is written to the board at once
and reaches the forges with the next `git board sync`. No command but a
sync touches the network.

`git board X` and `git-board X` are the same command: the binary is a git
subcommand.

`-C <dir>` runs any command as if started in `<dir>`, as `git -C` does:
`git-board -C ~/src/co2-calculator ls`. `git -C <dir> board ls` is the
same.

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
git board new "Login fails" --template "Bug report"
git board new                                   # on a terminal: asks
id=$(git board new "Follow-up")
```

The title is the first argument (or `-t, --title`), then `field=value`
assignments, read and checked as [`set`](#set-fields) reads them, written
in the same change as the issue. A first argument that reads as
`field=value` is an assignment: a title like `a=b` goes with `--title`.

It prints the new issue's id alone on its line. `--json` prints the issue
as `ls --json` prints each one. `--force` writes values the config
doesn't allow; `--me` is `@me`.

### Templates

`-T, --template <name>` starts from one of the repository's issue
templates, by its name or its file's, without case:

| Where | Read |
|---|---|
| `.github/ISSUE_TEMPLATE/*.md` | the front matter's `name`, `title`, `labels`, `assignees`; the text below it |
| `.github/ISSUE_TEMPLATE/*.yml` | an issue form: `name`, `title`, `labels`, `assignees`, and a `### label` heading per field, as GitHub writes the issue; markdown elements, dropdown choices and validations are left out |
| `.gitlab/issue_templates/*.md` | named by the file; `/label ~x` and `/assign @x` lines are labels and assignees, the rest the text |

The template's text is the description, unless `-b` or `-F` gives one.
Its labels and assignees (`assignee:<login>`) are set first, without
the config's check; the `field=value` arguments go over them. Its title
starts the title you are asked for, or the editor's first line; a title
you give wins. A name no template has is exit code 2, and the error
lists the names. `config.yml` is GitHub's chooser, not a template.

### Prompts

With no title on a terminal, `new` asks. A template first, when the
repository has some; then the title, after the template's title; then
whether to write the description in your editor. Questions go to
stderr: the id still prints alone on stdout. `-e` asks nothing: the
editor holds the title.

It never asks when stdin is not a terminal, with `--no-prompt`, or when
`GIT_BOARD_NO_PROMPT` is set. No title is exit code 2 then, as in a
script.

## `show`: an issue, with its comments

```bash
git board show a1b2c3d
git board show CO2-3075 --md
git board show "#2528" --json
git board show a1b2c3d --no-comments
git board show a1b2c3d --web                    # in the web UI
git board show a1b2c3d --forge                  # on GitHub or GitLab
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
| `-w, --web` | open it in this repository's web UI |
| `--forge` | open it on its forge |
| `--at <when>` | as it was then (see [The board as it was](#the-board-as-it-was)) |

`--web` opens the issue's page where the repository's board already
runs; with none running, it starts one here, as a bare `git board` does,
until Ctrl-C. `--forge` opens it on the first forge it is on; an issue on
no forge yet is exit code 3. Both print the URL.

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

## `labels`: what is in use

```bash
git board labels                         # every label, with counts
git board labels --plain --json
git board labels rename regresion regression
git board labels rename prio:p0 prio:p1
```

`labels` lists every label on the board's issues, with how many open and
closed issues have it: the plain labels first, then each field's values,
by field. Deleted issues don't count. `--json` prints `[{"label",
"field", "value", "open", "closed"}]`.

`labels rename <old> <new>` renames a label on every issue that has it,
one change each. A field's value is a label too. It doesn't check the new
name against `.git-board.toml`: `git board lint` does.

## `config`: settings

```bash
git board config list                               # the file's keys, then this clone's
git board config get views.backlog.filter
git board config set views.backlog.filter "is:open prio:p1"
git board config set sprints.length_days 14
git board config unset github-project.write
git board config set board.view classic             # this clone's git config
```

A dotted key, like `github-project.owner` or `views.backlog.filter`, is
in `.git-board.toml`, which the team shares and commits. `set` changes
that key's line alone: every other line and comment stays. It refuses
(exit code 2) a key git-board doesn't read, and a value that leaves the
file broken. A value is TOML when it is one (`true`, `14`, `["a", "b"]`),
else a string: quote `'"14"'` for a string of digits. A key inside an
array of tables (`[[sprints.list]]`) is edited in the file.

A key `board.<name>` is this clone's own, in git config: `board.view`,
`board.avatars`, `board.datastore`, `board.experimental`, and `board.me`,
which `git board me` sets. `set` writes it with `--local`; `get` reads it
at every level git does. A datastore URL is printed without its password.
`get` of a key not set exits with code 3.

## `forge status`, `forge login`: tokens

```bash
git board forge status                 # each forge, and whether a token is found
git board forge status --check         # and whose each token is, asking the forge
git board forge login github           # a hidden prompt
gh auth token | git board forge login github --login guilbep
```

`forge status` lists the forges (those of `.git-board.toml`, else those
found in the git remotes) and, for each, whether a token is found for its
posting account and for your own account on that host, and where: an
environment variable or git's credential helper. No token is printed.
It doesn't touch the network, unless `--check`. `--json` prints it.

`forge login <forge>` stores a token in git's credential helper (`git
credential approve`), as Settings › Sync does: git-board keeps no token
of its own ([ADR 0010](../adr/0010-tokens-from-env-or-git-credential-helper.md)).
The token is read at a hidden prompt on a terminal, else from the first
line of stdin; never from the command line. It is for `--login`, by
default your own account on that host, else the forge's posting account.

## `close`, `reopen`, `move`

```bash
git board close a1b2c3d -c "Fixed in 4e5f6a7"
git board close a1b2c3d --reason not-planned -c "Out of scope"
git board close a1b2c3d --duplicate-of "#2518"
git board reopen a1b2c3d -c "Still fails on Safari"
git board move a1b2c3d in-progress
git board move a1b2c3d --next
git board move a1b2c3d --prev
```

`close` sets the status to its last option and closes the issue.
`reopen` opens it and, when its status is that last option or none, sets
the first option; another status stays. Both take `-c, --comment`, added
first.

`close -r, --reason` says why it closed, as GitHub does: `completed`
(the default), `not-planned` or `duplicate`. The reason is a field like
any other, the label `closed-as:not-planned`, so `ls
"closed-as:not-planned"` finds them; `completed` writes none.
`--duplicate-of <id>` closes as `duplicate`, sets the issue field
`duplicate-of` to the other issue (its forge number `#2518` when both
are on that forge, else its id), and comments `Duplicate of #2518`,
which GitHub reads as well. `reopen` takes `closed-as` away and leaves
`duplicate-of`. The forges get `closed-as` as a label, not as their own
close reason.

`move <id> <status>` is `set <id> status=<status>`, checked the
same way. `move --next` and `--prev` step the status through its
options. Onto the last one the issue closes; back from it, it reopens.

## `branch`: the issue's branch

```bash
git board branch a1b2c3d                # the name, and the command
git board branch a1b2c3d --create       # create it from the base, switch to it
```

The name comes from `[branches]` in `.git-board.toml`: by default the
issue's folder, then the branch's place in it and the title,
`2518/1-login-fails-on-safari`. `branch` prints it and the `git switch`
command creating it from the base, and creates nothing. `--create` runs
that command. A second `--create` makes the issue's next branch,
`2518/2-…`.

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

## `status`, `follow`, `unfollow`: what needs you

```bash
git board status                        # assigned to you, then your inbox
git board status --mark-read            # and mark that inbox read
git board status --json
git board follow a1b2c3d                # every change reaches your inbox
git board unfollow a1b2c3d              # only what is addressed to you
git board follow a1b2c3d --auto         # back to automatic
```

`status` prints three parts. The open issues assigned to you. Then your
inbox, as the web UI's Inbox has it: what someone else changed since you
last looked, in the last 14 days. It is split in two: the issues you
opened, and the others you follow or are mentioned in. Each line says
what is new (`2 comments`, `closed`, `mentioned you`), by whom, and when.

Nothing is marked read: the Inbox still shows the same. `--mark-read`
marks what `status` printed read. `--json` prints `{"me", "assigned",
"opened", "follows"}`: `assigned` holds issues, the others inbox rows
(below).

You follow an issue automatically while you are in it: you opened it,
are assigned, commented, are mentioned, or hold a role. `follow` and
`unfollow` override that, as the web UI's bell does; `follow --auto`
goes back to it. Each prints whether you follow the issue now, and why;
`--json` prints `{"following", "reasons", "set"}`. Your follows stay in
this clone, as the web UI's do. `ls "follows:@me"` lists what you
follow.

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
sorting. `--group <field>` groups, `--md` renders as Markdown, and
`--json`, `--jq` and `--template` as [JSON](#json-output). `view
<name>` runs a view from `.git-board.toml`.

## `insights`: flow, throughput, cycle time

```bash
git board insights                         # the last 90 days, by status
git board insights --days 30 --field prio
git board insights --view backlog --json
```

What the web UI's Insights page shows, as text: the headlines, the open
issues by the field's value today and `--days` ago, the issues opened and
closed each week (the last `--weeks`, 8), the cycle time's median and
85th percentile, and the current sprint's burn-up. `--view` takes a
view's issues alone, open and closed. `--json` prints what the page reads
(`GET /api/insights`).

## JSON output

```bash
git board ls --json                              # every field
git board ls --json id,title,labels              # only these
git board ls "assignee:@me" --jq '.[].title'
git board ls --template '{{range .}}{{.title}} ({{timeago .updated_at}}){{"\n"}}{{end}}'
git board show a1b2c3d --jq '.events[] | select(.kind == "comment") | .body'
```

Every command that prints JSON takes the same `--json`: `ls`, `view`,
`show`, `log`, `diff`, `status`, `me`, `new`, `comment`, `follow`,
`unfollow`, `undo`, `labels`, `insights`, `config list`, `forge status`.
Alone, it prints every field. With a list, `--json id,title`, it keeps
those: in a list, of each item. A field an item lacks is `null`. A name
the command doesn't print is refused (exit code 2), with the list of
those it does, before anything is written. `--json=id,title` says the
same; the word after `--json` is taken as fields when it has a comma or
is one of them, so `ls --json title` prints titles and `ls title --json`
searches for "title".

`--jq <expression>` filters the JSON with jq (gojq, built in: no `jq` to
install). A string prints as it is, anything else as JSON on one line.
`--template <template>` prints it through a Go template
([text/template](https://pkg.go.dev/text/template)), with gh's helpers:
`join <sep> <list>`, `pluck <field> <list>`, `timeago <time>`, `timefmt
<layout> <time>` (a Go layout, `2006-01-02`), `truncate <n> <text>`. Each
implies `--json`, and goes with its fields: `--json id,title --jq ...`.
A wrong expression or template is refused (exit code 2). `new` has no
`--template`: it is kept for an issue template, as gh's.

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

**An inbox row** (`status --json`): `issue` (an issue), `reasons` (why
you follow it: `author`, `assignee`, `followed`, `view: <name>`; none when
it is only addressed to you), `news` (`2 comments`, `closed`, `mentioned
you`, `added you as reviewer`), `who` (people), `latest` and `first` (the
oldest new event's id).

## Shell completion

```bash
git board completion bash > ~/.local/share/bash-completion/completions/git-board
git board completion zsh > "${fpath[1]}/_git-board"
git board completion fish > ~/.config/fish/completions/git-board.fish
```

`git board completion <shell>` prints the script for bash, zsh, fish or
PowerShell; `git board completion <shell> --help` says where it goes. It
completes the `git-board` form: `git board ...` goes through git's own
completion, which doesn't know this command's words. Tab then completes:

| Where | What |
|---|---|
| `<id>` (`show`, `comment`, `edit`, `set`, `close`, `reopen`, `move`, `log`, `branch`, `get`) | the issues by short id or project number, each with its title |
| `set` and `new`'s `field=value` | the fields, then their options; `@current`, `@next` and the sprints for a sprint; `@me` for a person; else the values on the issues. After `+` or `-` too |
| `move`, after the id | the status options |
| a filter (`ls`, `tui`, `set --filter`) | the qualifiers, `is:` and the fields, then their values, for the filter's last word |
| `view` | the views |
| `me` | the people on the board |
| `--group`, `--sort`, `--field`, `--json=` | the fields |

A completion reads the board and `.git-board.toml`, writes nothing, and
offers nothing when it can't read them.

## Logs

The server behind the web app (`git board`, `git board serve`) writes its
logs to stderr: one line an event, its start, each request, what went
wrong. v2 writes them (`git-board-next`, [the HTTP API](../rewrite/api.md#logs));
v1 writes a few lines with no level.

```bash
git board serve --log-level debug                     # the queries, each change event
git board serve --log-format json 2> board.log        # one JSON object a line
GIT_BOARD_LOG_LEVEL=warn git board                    # only what went wrong
```

| Level | What |
|---|---|
| `error` | a request that failed on the server's side, with its code; a sync that failed |
| `warn` | a request refused (another site's page, a name that isn't this machine's); a request slower than a second; a sync's warnings; an issue that can't be read |
| `info` (the default) | the start (version, board, datastore, address); a line a request (method, path, status, duration, bytes); a sync's start and end |
| `debug` | a request's query, its secrets redacted; each change event |

`--log-level` and `--log-format` (`text`, the default, or `json`) win
over `GIT_BOARD_LOG_LEVEL` and `GIT_BOARD_LOG_FORMAT`. A line never holds
a token, a cookie, a header or a request's body.

## Exit codes

| Code | |
|---|---|
| 0 | done; an empty `ls` too |
| 1 | an error not below |
| 2 | a wrong command line: a flag, an argument, an assignment malformed or not allowed by the config; a config key git-board doesn't read; an undo refused; a delete with no terminal to ask and no `--yes` |
| 3 | no such issue, comment or person, or a prefix naming several |
| 4 | no one chosen to write as: `git board me` |
| 5 | someone changed the text meanwhile: your edit is refused, your text kept |

Errors go to stderr, prefixed `error:`.

## Coming from gh / glab

| gh | glab | git board |
|---|---|---|
| `gh auth status` | `glab auth status` | `git board me` (who you write as), `git board forge status` (the forges' tokens) |
| `gh auth login --with-token` | `glab auth login --stdin` | `git board forge login <forge>` (into git's credential helper) |
| `gh issue create -t T -b B -l bug -a @me` | `glab issue create -t T -d B -l bug -a @me` | `git board new T -b B label=+bug assignee=+@me` |
| `gh issue create -e` | `glab issue create` | `git board new -e` |
| `gh issue create` (asks) | `glab issue create` (asks) | `git board new` (asks on a terminal; `--no-prompt`) |
| `gh issue create -T name` | `glab issue create --template name` | `git board new -T name` |
| `gh issue list -a @me -l bug -S text` | `glab issue list -a @me -l bug --search text` | `git board ls "assignee:@me label:bug text"` |
| `gh issue list -L 10 --json …` | `glab issue list -P 10 -O json` | `git board ls -L 10 --json` |
| `gh issue list --json number,title` | | `git board ls --json number,title` |
| `gh issue list --json title --jq '.[].title'` | | `git board ls --jq '.[].title'` |
| `gh issue list --json title -t '{{range .}}…'` | | `git board ls --template '{{range .}}…'` |
| `gh issue list -S "created:>2026-10-01 comments:>5"` | | `git board ls "created:>2026-10-01 comments:>5"` |
| `gh issue list -S "crash in:title"` | | `git board ls "crash in:title"` |
| `gh issue view N -c` | `glab issue view N -c` | `git board show N` |
| `gh issue view N --json …` | `glab issue view N -F json` | `git board show N --json` |
| `gh issue view N -w` | `glab issue view N -w` | `git board show N -w` (the web UI), `show N --forge` (the forge) |
| `gh issue comment N -b B` | `glab issue note N -m B` | `git board comment N -b B` |
| `gh issue comment N --edit-last` | | `git board comment N --edit-last` |
| `gh issue edit N -t T -b B` | `glab issue update N -t T -d B` | `git board edit N --title T -b B` |
| `gh issue edit N --add-label x --remove-label y` | `glab issue update N -l x -u y` | `git board set N label=+x label=-y` |
| `gh issue edit N --add-assignee @me` | `glab issue update N -a @me` | `git board set N assignee=+@me` |
| `gh issue edit N -m v1` | `glab issue update N -m v1` | `git board set N milestone=v1` (a field), or `sprint=@next` |
| `gh issue edit 1 2 3 --add-label x` | | `git board set --filter "<filter>" label=+x --apply` |
| `gh issue close N -c C` | `glab issue close N` | `git board close N -c C` |
| `gh issue close N -r "not planned"`, `--duplicate-of M` | | `git board close N -r not-planned`, `--duplicate-of M` |
| `gh issue reopen N` | `glab issue reopen N` | `git board reopen N` |
| `gh status`, `gh issue status` | | `git board status`, or `git board ls "involves:@me is:open"` |
| | `glab issue subscribe N`, `unsubscribe N` | `git board follow N`, `git board unfollow N` |
| `gh issue develop N -c` | `glab mr create --related-issue N` | `git board branch N --create` (`branch N` prints the branch and the command) |
| | `glab issue view N -s` (system notes) | `git board log N` (every change, text edits too) |
| | | `git board show N --at 2026-10-01`, `git board ls --at 2026-10-01` (the board as it was) |
| | | `git board diff N --since 7d` (what changed: fields, a text diff, new comments) |
| `gh issue delete N --yes` | `glab issue delete N` | `git board delete N --yes` (hidden, kept in history) |
| `gh issue comment N --delete-last` | `glab issue note delete` | `git board comment N --delete <ref>` |
| | | `git board undo N` (your last change, inverted) |
| `gh -R owner/repo issue list` | `glab -R owner/repo issue list` | `git board -C <dir> ls` (a clone: the board is in it) |
| `gh config get/set` | `glab config get/set` | `git board config get/set/list` |
| `gh label list` | `glab label list` | `git board labels` (with counts) |
| `gh label edit old --name new` | `glab label edit` | `git board labels rename old new` (on every issue) |
| | `glab issue board view` (counts) | `git board insights` (flow, throughput, cycle time, burn-up) |
| `gh project item-edit …` | `glab issue board …` | `git board move N in-progress`, `move N --next`, `git board project` |
| `gh completion -s zsh` | `glab completion -s zsh` | `git board completion zsh` |

`N` is any [name of an issue](#naming-an-issue). Not there yet: pinning,
locking (no permissions in a board kept in git), `transfer` between
boards.
