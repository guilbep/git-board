# Owning your issues

git-board's reason to exist: your issues are your data, independent of
GitHub, GitLab or any forge, and of git-board itself.

## What you own

- **Every operation.** An issue is the list of every change made to it:
  created, retitled, labelled, commented, closed. Nothing is overwritten;
  `git board log <id>` reads it back.
- **In your store.** By default the operations are in your repository,
  under `refs/board/`: every clone has them all, and `git board push` backs
  them up wherever your remote is.
- **With stable ids.** An issue's id comes from its first operation, so it
  is the same in every clone and every store. Forge numbers come and go;
  the id stays.

## Where the board is kept

```sh
git board datastore             # where, and what it can do
```

| Kept in | When |
| --- | --- |
| git (default) | the board travels with the repository; offline; pull and push |
| PostgreSQL | one shared server: everyone sees each write at once, no pull or push |
| Markdown files | a folder of the checkout, one file per issue, reviewed in pull requests |
| the forges alone | GitHub or GitLab stays the record; git-board is a way to work on it |

Settings › Datastore chooses one, tests the connection and copies a board
in, every id kept. [Where the board is kept](../guide/datastores.md) has
the details.

## Leaving a forge

The forge is a mirror. To stop using one:

1. Pull everything it has: `git board sync` (or `git board forge pull`).
2. Turn its sync off in Settings › Sync, or remove it.

The issues, comments and history stay on the board. Links to the forge's
issues stay as text; nothing else depends on it. Moving to another forge is
adding it and syncing: its issues are created from the board.

## Leaving git-board

```sh
git board export --md issues        # one Markdown file per issue: fields, description, comments
```

The files are plain Markdown with YAML front matter: readable anywhere,
and `git board import --md issues` reads them back, as new issues on another
board or as edits to the same one. The operations themselves are JSON in
git objects: on a git board, each commit of `refs/board/issue/<id>` holds
one pack of them (`git cat-file -p refs/board/issue/<id>:pack` shows the
latest). The format is specified in [the spec](../spec/storage.md), for any
program to read.

## Coming from GitHub or GitLab

Add the forge (Settings › Sync, or nothing to add when `origin` is on it),
then:

```sh
git board sync --check              # what it would bring in
git board sync                      # every issue and comment, with their authors
```

From then on, work on the board, on the forge, or both. Coming from
git-bug: [Forges › Leaving git-bug](../guide/forges.md#leaving-git-bug).

## Attachments

Files attached to comments go to an S3 bucket behind a CDN, not into the
board: the board holds links. They are public to anyone with the link, and
an export or a copy keeps the links, not the files.
