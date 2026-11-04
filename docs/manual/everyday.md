# Everyday work

The command line and the web UI read and write the same board: use either,
or both. Each section shows the command; the web UI does the same from an
issue's panel or the board.

## Fields

A field is a label `name:value`: `status:ready`, `prio:p1`,
`assignee:ada`. The project's `.git-board.toml` says which fields exist,
their options and their order; the web UI shows them as columns, lanes and
the side panel's values.

```sh
git board set 624a status=ready prio=p1          # single fields swap
git board set 624a assignee=+ada                 # multi fields add…
git board set 624a assignee=-ada                 # …and remove
git board set 624a prio=                         # clear
git board get 624a                               # its fields
```

`set` refuses a value the project doesn't allow (a misspelled option, a
date that isn't one) and writes nothing; `--force` writes it anyway. Several
assignments land as one change.

`assignee=+@me` needs your forge login, so it works once a forge sync knows
you (or with `--me ada`). On a board with no forge, assign by name:
`assignee=+ada`.

## Statuses

```sh
git board move 624a ready           # straight to a status
git board move 624a --next          # backlog → ready → in progress → …
git board move 624a --prev
git board close 624a -c "Fixed in 4e5f6a7"   # the last status, closed, with a comment
git board close 624a -r not-planned          # closed-as:not-planned
git board close 624a --duplicate-of 5f1e     # closed-as:duplicate, linked, commented
git board reopen 624a                        # open again
```

The issue's state follows its status: the last status option closes it,
any other opens it. On the web board, drag a card to another lane.

## Finding issues

```sh
git board ls                                  # open issues
git board ls "status:ready prio:p1"           # AND
git board ls "status:ready,qa -assignee:ada"  # either value; - negates
git board ls "sprint:@current assignee:@me" --group status
git board ls "involves:ada" --sort "updated desc" --limit 20
git board ls "safari login"                   # words in titles, descriptions, comments
git board view backlog --md                   # a named view from .git-board.toml
```

The same filter works in the web UI's Filter box, the filters panel and
⌘K. The whole grammar is in [Configuration › Filters](../guide/configuration.md#filters).

## Reading and discussing

```sh
git board show 624a                     # fields, description, comments
git board show 624a --json | jq .title
git board show 624a --web               # its page in the web UI
git board show 624a --forge             # its page on GitHub or GitLab
git board comment 624a -b "Reproduced on 17.2 too."
git board comment 624a -e               # in your editor
git board comment 624a --edit-last      # fix your last comment
git board edit 624a --title "Login fails on Safari 17"
git board edit 624a -e                  # title and description in your editor
```

An edit made by someone else meanwhile is never overwritten: the command
refuses, keeps your text in a file it names, and exits 5.

## History

```sh
git board log 624a                      # every change: who, when, what
git board log 624a --field status       # how its status moved
git board log 624a --since 2026-10-01 --json
```

Every operation is kept, so the history is complete: who set a field,
when, and what it was before. No forge shows you that.

## Many issues at once

```sh
git board set --filter "sprint:@current status:ready no:assignee" assignee=+ada          # what it would do
git board set --filter "sprint:@current status:ready no:assignee" assignee=+ada --apply  # do it
```

On the web board, Cmd- or Ctrl-click several cards, then drag them
together.

## Sprints

```sh
git board sprint ls                     # the configured sprints
git board ls "sprint:@current"          # this sprint's issues
git board set 624a sprint=@next         # into the next one
```

Sprints are `[sprints]` in `.git-board.toml`
([Configuration › Sprints](../guide/configuration.md)).

## The inbox

In the web UI, **Inbox** lists what changed in the issues you are in (you
opened them, are assigned, commented, are mentioned) and the ones you
follow. Follow an issue or a view from its menu.

```sh
git board status                  # assigned to you, and what changed for you
git board follow 624a             # every change to it reaches your inbox
git board unfollow 624a           # only a mention of you does
git board follow 624a --auto      # back to automatic
```
