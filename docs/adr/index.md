# The decisions

git-board keeps a team's issues in the team's own repository, and shows
them on the forges the team uses. Nine decisions make it what it is. Read
them in this order.

[ADR 0003: The board is the team's](0003-own-core-instead-of-git-bug.md).
The team owns its issues, their history and the people who wrote them.
git-board owns its data model, depends on no forge, and loses nothing
when the team leaves a forge or git-board itself.

[ADR 0002: Where the board is kept](0002-datastores.md). A board lives in
git, in PostgreSQL, in a folder of Markdown files, or on the forges
alone. Every datastore keeps the same packs, so a board keeps its ids
when it moves. A disposable view answers the queries, attachments go to
S3, and project numbers come from one counter.

[ADR 0004: Issues are operation logs, fields are labels](0004-entities-are-operation-logs.md).
An issue is a log of operations that every clone orders the same way, so
clones merge with no server. Every field is a label, so a new field
needs no code. A delete hides an issue or a comment, and keeps every id.

[ADR 0011: Shared settings in the file, your own in git config](0011-shared-settings-in-git-board-toml.md).
The team's configuration is `.git-board.toml`, reviewed like code and
versioned by its schema. Who you are, where your clone keeps the board,
and what you try alone stay in your git config.

[ADR 0008: One truth, many mirrors, and how they sync](0008-sync-state-in-the-data-model.md).
The board is the only truth, and every forge and the GitHub Project are
mirrors of it. The sync records each agreement as operations, compares
three values per field, and moves only the side that changed.

[ADR 0010: No secret is stored, and the server answers on localhost](0010-tokens-from-env-or-git-credential-helper.md).
Tokens come from the environment or from git's credential helper, never
from a file. On github.com, a person logs in in the browser, and the
token still goes to git's credential helper. The local server has no
login, so it answers only this machine's pages, and runs code only for
them.

[ADR 0006: One binary, built in layers](0006-go-git-reads-git-binary-moves-refs.md).
One static binary is the CLI, the terminal UI, the server and the web
app. Its packages sit in layers that a test enforces, and it reads git
through `git cat-file`.

[ADR 0014: Tests on real repositories, few licences](0014-hermetic-tests-on-real-repositories.md).
Tests run on real repositories and stateful fakes, sealed off the
network and the user's tokens. git-board is MIT, builds no GPL module,
and adds a dependency only when a few lines cannot do the work.

[ADR 0001: Proof in git notes, and the opt-in cycle](0001-check-results-are-signed-git-notes.md).
A check's result is a signed line in a git note, keyed by the content it
checked, so the proof travels with the code. The development cycle that
reads it is opt-in.

Merged and kept as stubs: 0005, 0012, 0020 and 0021 into 0006, 0007 into
0004 and 0008, 0009, 0015 and 0016 into 0002, 0013 into 0010, 0017 into
0014, 0018 into 0003 and 0001, 0019 into 0004, 0022 into 0008.
