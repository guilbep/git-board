# The file's schema version

`.git-board.toml` says which version of its format it is written in: a
top-level key, before the first table.

```toml
# the board's opening comment

schema = 2

[fields.status]
…
```

A file without the key is **version 1**: what git-board v0.11 and every
earlier release write. git-board v0.12 (v2) writes **version 2**. Each
version is a step from the one before, with its way back, as Alembic's
migrations are for a database: git-board upgrades a file, and downgrades
it for an older git-board, editing the lines that change and nothing
else. Comments, order and every other line stay as they were.

## The versions

| Version | Written by | What it changed | Down |
| --- | --- | --- | --- |
| 1 | v0.11 and earlier | The format [Configuration](configuration.md) describes, no `schema` key. | |
| 2 | v0.12 | Sets aside `[sprints] start_weekday` and `[repos] assignees`, which no git-board reads, in a comment each: `# start_weekday = "monday"  # schema 2: git-board doesn't read this key`. `git board init` wrote the first in every file; the second is dead since the assignees live where their field's mapping says ([Forges](configuration.md#forges)). At version 2 both are keys git-board doesn't know, and warns of. | lossless |

`git board config schema` lists the versions between the file and the
binary, with what each changes.

## Reading and writing

**A file older than git-board** is read as if it were upgraded: the
upgrade runs in memory, and the file is left as it is. The first
explicit write (`config set`, `config unset`, a save in Settings) writes
it upgraded, and says so once:

```
.git-board.toml upgraded from schema 1 to 2 (git board config schema says what changed)
```

A write that changes nothing writes nothing, the upgrade included.

**A file newer than git-board** is read for the tables it knows, with a
warning, and never written:

```
warning: .git-board.toml is schema 3, newer than this git-board's 2: it reads what it knows, writes nothing to it, and ignores the rest; upgrade git-board
error: .git-board.toml is schema 3, and this git-board knows schema 2 at most: it writes nothing to it. Upgrade git-board, or have a git-board that knows schema 3 run git board config downgrade --to 2
```

Reads go on because a read can't harm the file, and because an older
git-board already reads a newer file that way: a key it doesn't know is
named and ignored, as a misspelled one is. Which tables a newer version
changed only the newer git-board knows, so the older one can't pick the
tables it would trust. A write could undo a newer version's edit, or
write a form that version no longer reads: it is refused, for every
table. A migration that changes what a key means renames the key, so an
older git-board sees a key it doesn't know rather than a value it reads
wrong.

**git-board v0.11 on a version 2 file** reads it. It doesn't know the
`schema` key: each command warns
`schema: git-board doesn't know this key, ignored (git board lint)` once,
and `git board lint` counts it as an `unknown-key` finding, so
`lint --strict` fails. The keys version 2 set aside are ones v0.11
doesn't read either. To hand the file to a team still on v0.11 without
the warning, downgrade it: `git board config downgrade --to 1`.

## Upgrading and downgrading

```bash
git board config schema                  # the file's version, git-board's, the steps between
git board config schema --json           # {"file", "binary", "pending"}
git board config upgrade --dry-run       # the diff an upgrade would write, as diff -u prints it
git board config upgrade                 # to git-board's version
git board config upgrade --to 2
git board config downgrade --to 1 --dry-run
git board config downgrade --to 1        # for git-board v0.11
```

`upgrade` without `--to` goes to git-board's version. `downgrade` needs
`--to`. Both refuse a version git-board doesn't know, and a file newer
than git-board (exit code 2). Commit the change as any other: it is a
diff to review.

## What is lossy

No step is lossy yet: from what an upgrade wrote, a downgrade gives the
file back byte for byte. What a downgrade can't give back is what was
done at the newer version:

- A line version 2 set aside and then edited by hand, its comment
  changed, stays a comment: the downgrade no longer recognizes it.
- A key set again at version 2 (`[repos] assignees`, beside the line
  set aside) is refused by the downgrade, which would set it twice: keep
  one of the two by hand.

A lossy step says what it loses in `config downgrade`'s output and in
this table's last column.

## Adding a migration

A change to the file's format is a new version. The checklist:

1. **Code.** Append a `Migration` to `Migrations` in
   `conf/schema.go`, `From` the current version, `To` the next, a
   one-line `Desc`, and raise `Current`. Build `Up` and `Down` from the
   edits in `conf/migrate.go`: `RenameKey`, `MoveKey`,
   `RenameTable`, `MapValue`, `CommentKey` and `UncommentKey`, `DropKey`,
   joined with `Chain`. Each takes a value over several lines (an array,
   a `"""` string) whole: `CommentKey` comments every line of it and
   `UncommentKey` gives them all back; `MapValue` refuses one. Each checks
   the file it gives back, so a key in a form it doesn't edit (an inline
   table, a dotted key) is an error the user reads, never a key left
   behind.
2. **The domain.** The table's domain reads the new form alone: an older
   file reaches it migrated. Don't keep a reader for the old form.
3. **Both directions.** `Down` undoes `Up`. When it can't, keep what it
   would lose in a comment (`CommentKey`), or say what is lost in
   `Lossy`. Keep the lines where they are when you can: a read migrates
   in memory, and an error message names a line.
4. **A changed meaning renames the key.** An older git-board then warns
   of a key it doesn't know, instead of reading a value wrongly.
5. **Tests.** Add a file to `conf/testdata/schema/<name>.v1.toml`
   that has the old form, run
   `GIT_BOARD_GOLDEN_UPDATE=1 go test ./conf -run TestMigrateBothWays`,
   and review the `<name>.v<N>.toml` it writes: `TestMigrateBothWays`
   then checks every fixture up to each version and back, byte for byte
   when no step is lossy. Test the forms the edit refuses beside it.
6. **This page.** A line in the table of versions, and the form in
   [Configuration](configuration.md).
7. **Compatibility.** `make compat`.

## For Settings and the HTTP API

`app` gives the versions: `Config.SchemaVersion` is the file's (0
without a file), `app.CurrentSchema` git-board's, `app.ConfigSchema` both
and the pending steps, `app.MigrateConfig` an upgrade or a downgrade,
and `conf.ForWrite` the text a write starts from (upgraded, or refused
when newer). `app.SetConfig` returns the write's `ConfigWrite`, whose
`Notice` is the line to show.
