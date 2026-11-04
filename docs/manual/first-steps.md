# First steps

## A board in your repository

```sh
cd your-repository
git board init
```

`init` writes `.git-board.toml` at the repository's root: the fields a
board starts with (status, priority, kind, estimate…), a few views, and
comments saying what each key does. Commit it: it is your team's
configuration, reviewed like code. You can also start without it: every
`name:value` label is a field anyway.

A clone of a repository whose board already exists needs no `init`: the
first `git board` or `git board ls` pulls `origin`'s board.

## Who you are

Every change is signed with who made it. Say who you are once per clone:

```sh
git board me --new             # a person named as git's user.name, user.email
git board me                   # who you write as now
git board me --ls              # everyone on the board
git board me 14c21d8d          # write as someone already there (after a forge sync, your forge account)
```

Who you write as is kept in this clone's git config (`board.me`), never in
the board.

## Your first issues

```sh
git board new "Login fails on Safari" prio=p1 -b "Steps: open the login page on Safari 17."
git board new -e                          # write the title and description in your $EDITOR
git board ls                              # the open issues
git board show 624a                       # one issue: fields, description, comments
```

`new` prints the new issue's id. Any unique start of an id names the issue
(`624a`), and so does a forge number (`#12`) or a project number
(`CO2-3075`) once the project has them.

## The web UI

```sh
git board
```

opens this repository's board in your browser, starting a local server on
port 8080 or the next free one. Each repository gets its own. The server
only listens on this machine, and only its own pages may write.

- **Issues**: a table, a board (kanban) by any field, or a roadmap; the
  filters panel and the Filter box take the same syntax as the command line.
- **An issue** opens in a side panel: its fields, the description, the
  comments and every change, in order.
- **Insights**: flow, lead time and sprint reports.
- **Inbox**: what changed in the issues you are in or follow.
- **Settings**: fields, labels, views, sprints, forges, where the board is
  kept, and who you are, each saved to `.git-board.toml` or your git config.

`git board tui` is the same board in the terminal.

## Next

[Everyday work](everyday.md) covers fields, statuses, filters and
comments; [Working as a team](together.md) covers sharing the board and
the forges.
