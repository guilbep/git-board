# Questions

## "choose who you write as first"

Every change is signed. Run `git board me --new` (you, from git's
`user.name` and `user.email`) or `git board me <id>` to write as someone
already on the board (`git board me --ls`). The web UI asks the same in
Settings › You.

## "@me needs your forge login"

`@me` in a filter or an assignment means your login on a forge, and a
board that never synced with one doesn't know it. Pass `--me <login>`, set
`GIT_BOARD_ME`, or write the login itself: `assignee=+ada`.

## `set` refuses a value

The project's `.git-board.toml` lists each field's options; a value outside
them is refused, and nothing is written (exit 2). Check the spelling
(`git board get <id>` shows the current values), add the option in
Settings › Fields, or pass `--force` to write it anyway.

## A command says an edit was made meanwhile

Someone changed the same comment or description since you started. Your
text is kept in the file the error names; run the command again on the
new text (exit 5).

## My teammate doesn't see my issues

Issues travel with `git board push` (or `git board sync`), not with `git
push`. They then `git board pull`. In the web UI, the header says what
waits to be sent.

## The sync wants to create many issues

A first sync with a forge, or one that would create a lot at once, waits
for consent: run `git board sync --check`, read what it would do, then
`git board sync --allow-create`.

## Where is the cycle, the Work page, the agents?

They are experimental, off unless the project's `.git-board.toml` turns
them on (`[experimental] features = ["cycle", "agents", "worktrees"]`) or
your clone does (`git config board.experimental cycle`). See
[Configuration › Experimental features](../guide/configuration.md#experimental-features).

## The index looks wrong, or a list is slow the first time

The index is a cache in `.git/board/index.db`, built again from the board
whenever it's missing. Deleting it is safe.

## Exit codes, for scripts

| Code | Meaning |
| --- | --- |
| 0 | done |
| 1 | an error |
| 2 | usage, or input refused (a bad value) |
| 3 | no such issue, or an ambiguous id |
| 4 | no one chosen to write as |
| 5 | changed meanwhile |

The [CLI reference](../guide/cli.md#exit-codes) has every command's.
