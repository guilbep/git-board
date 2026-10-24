# Queries and search

Lists and search read a local index of the issues and people, kept in
sqlite. This page says where it lives, how it stays fresh, what a query
asks, and how a program hears that entities changed.

## Where it lives

The index is one sqlite file, `<git common dir>/board/index.db`. The git
common dir is what `git rev-parse --git-common-dir` gives, so every
worktree of a repository shares one index. The core creates the `board`
directory when it is missing.

- **It is never the source of truth.** It holds only what the refs hold.
  Deleting it loses nothing: the next query builds it again.
- **It is local.** It lives in the git directory, so git never commits,
  fetches nor pushes it.
- **Two processes can use it at once,** like git-board's server and a
  command. The file is in WAL mode, so a read doesn't wait for a write, and
  a write waits up to 5 seconds for another (sqlite's `busy_timeout`).

## What it holds

Issues and persons. Sync entities aren't indexed.

- **For each issue:** its id, title, status, author, `created`, `updated`,
  labels and number of comments, from its [state](data-model.md#issue-state).
  For search, the text of its title, body and comments.
- **For each person:** its id, name and accounts (`server`, `id`,
  `login`), from its [state](data-model.md#person-state).
- **For each entity:** its head, the commit its ref points at.
- **For each broken entity:** its head and its error (see
  [Freshness](#freshness)).

## Freshness

A refresh brings the index up to the refs:

1. It lists `refs/board/issue/*` and `refs/board/person/*`. A ref whose
   name isn't a valid id isn't an entity, and is ignored (see
   [Refs](storage.md#refs)).
2. An entity that is new, or whose head differs from the stored one, is
   loaded again, and its rows replaced.
3. An entity whose ref is gone is dropped.

Every query refreshes first. So it answers from the refs as they are,
whoever moved them: this process, another one, or a fetch.

An entity that fails to load is broken. The refresh drops its rows, so no
query or lookup answers it, and still keeps the rest of its changes.

- **The index records it,** with its head and its error. `Broken()` lists
  the broken entities, each with its kind, id and error, after a refresh
  like every query.
- **It is loaded again** when its head moves, or at a rebuild. An entity
  whose ref is gone is dropped, broken or not.
- **A query never fails because of one broken entity.** It answers without
  it. A refresh that can't list the refs, or can't write the index, still
  fails the query.

## Rebuild

The index holds its schema version in `PRAGMA user_version`. Opening it,
the core deletes the file and builds it again from the refs when:

- the file is missing;
- sqlite can't read it as a database;
- its version isn't the code's.

Its `-wal` and `-shm` files go with it.

## Queries

A query asks for issues. Each field is optional: an empty one filters
nothing.

| Field | Matches |
|---|---|
| `status` | `open`, `closed`, or empty for any |
| `labels` | issues that have every label listed, compared byte for byte |
| `author` | issues whose author is that person id |
| `text` | issues whose text matches (see [Text](#text)) |

An issue must match every field given. The answer is ordered and cut by
three more:

| Field | What it does |
|---|---|
| `sort` | `created` or `updated`, the default |
| `desc` | true sorts newest first; false, the default, oldest first |
| `limit` | the most issues answered; 0, the default, answers all |

- Issues with the same time go by id, in the same direction.
- Another `status` or `sort`, or a negative `limit`, is an error.
- Each issue comes as its summary: what the index holds for it.

## Text

The text is searched in each issue's title, body and comments, as their
last edits left them.

- **Words.** The text splits on whitespace into words. An issue matches
  when every word matches, each anywhere in its title, body or comments.
- **Prefixes.** A word matches the start of a word: `crash` matches
  `Crashes on save`.
- **Case and diacritics don't count:** `cafe` matches `Café`. Words are cut
  as FTS5's default tokenizer, `unicode61`, cuts them: letters and digits
  make up a token, and anything else separates tokens.
- **A word of several tokens,** like `title:crash`, matches those tokens in
  a row, the last as a prefix.
- **A word with no token,** like `-`, is left out. A text with no word left
  matches every issue.
- **FTS5's own syntax is never read.** Each word is quoted, so `"`, `*`,
  `-`, `:`, `AND`, `OR`, `NOT` and parentheses are plain text.

## Accounts

A lookup by account, a `server` and an `id`, answers the person who owns
it, by the rule in [Who owns an account](data-model.md#account-owner). When
no person holds the account, it answers none.

## Change events

`Watch` tells a program, like git-board's server or its TUI, that entities
changed.

- **It polls.** Each poll lists `refs/board/<kind>/*` for each kind, and
  compares each head with the one it saw last. It ignores a ref whose name
  isn't a valid id.
- **The first poll is the baseline.** It runs at once, when `Watch`
  starts, and reports nothing.
- **Each later poll** runs one interval after the last, and reports what
  changed since. An interval of 0 or less means 1 second.
- **A poll whose listing of the refs fails is skipped:** it reports nothing,
  and the next interval tries again. A skipped baseline leaves the next
  poll as the baseline.
- **A poll's changes come together:** each entity that appeared, moved, or
  was removed, with its kind, id and new head. A removed one has the zero
  hash as its head. They are sorted by kind, then
  id. A poll with no change reports nothing.
- **It reports heads, not writes.** An entity written several times between
  two polls is reported once, with its head at that poll. One created and
  removed between two polls isn't reported.
- **It sees every change,** whoever made it: this process, another one, or
  a fetch.
- **It stops** only when its context ends, never on its own.
