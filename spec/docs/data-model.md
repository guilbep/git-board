# Data model

The core keeps one kind of record, the entity. This page says what an
entity is, what its operations hold, and how each kind of entity turns its
operations into a state. [Storage](storage.md) says how entities live in
git.

## Entities

An entity is a log of operations. Its state is what those operations add
up to, applied in the [order](storage.md#order) every clone agrees on.
Nothing else is kept in git: no snapshot of the state, no cache.

An entity has a kind and an id:

- the kind is `issue`, `person` or `sync`;
- the id is 40 lowercase hex characters, `^[0-9a-f]{40}$`, and unique
  within its kind. An issue's or a person's id is a pack hash, and a sync
  entity's is the SHA-1 of its forge key (see [Ids](#ids)).

An entity's ref names both: `refs/board/issue/3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5`
is the issue `3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5`.

Any read or write with another kind, or with an id that doesn't match, is
an error. So no id is a name git refuses for a ref.

## Operations

An operation is one change to an entity, as a JSON object:

| Field | Type | What it holds |
|---|---|---|
| `kind` | string | what the operation does, from its entity's list below |
| `author` | string | the id of the person who made it (see [People](#people)) |
| `time` | integer | when it was made, in Unix seconds |
| `origin` | object | where it came from, on an operation pulled from a forge; absent otherwise (see [Origins](#origins)) |
| `data` | object | the operation's own fields, which depend on its `kind` |

```json
{
  "kind": "comment",
  "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
  "time": 1792836300,
  "data": {
    "body": "It finds them with is:closed."
  }
}
```

- `kind` is read in the context of the entity: `create` opens an issue in
  an issue, and makes a person in a person. `link` is a sync entity's.
- `author` is a person's id. Each kind's rules reject an empty one, except
  in a person's first pack (see [People](#people)). They check nothing else:
  not its form, nor that the person exists, as it may arrive with the next
  fetch. The trial's `forge:<login>` authors are gone: a pull authors its
  operations as persons (see [Authors](sync.md#authors)).
- `time` is for people to read. It never orders operations: the
  [clock](storage.md#clock) does.
- An operation holds no id. Its id comes from where it is stored (see
  [Ids](#ids)).

Reading an operation:

- a field the reader doesn't know, in the operation or in its `data`, is
  ignored;
- a field missing from `data` is its empty value: an empty string, or an
  empty list. So is an absent `author`;
- an operation whose `kind` is empty or absent, or whose `time` is 0 or
  absent, makes the reader reject its commit (see
  [What a reader rejects](storage.md#what-a-reader-rejects));
- a `kind` the reader doesn't know is an error, which names the kind. The
  reader never drops an operation it can't apply;
- keys are case-sensitive, and an object holds each key once: `Kind` isn't
  `kind`. This holds for packs too. The core's reader checks neither: Go's
  decoder takes a key in any case, and keeps the last of two equal keys.
  It is a known ceiling, marked in `oplog`.

## Ids

An entity's operations are stored in packs, one per git commit (see
[Packs](storage.md#packs)). A pack's hash is the git object id of its
blob, in lowercase hex.

- **An entity's id.** An entity created without an id, an issue or a
  person, gets its first pack's hash. An entity created with an id keeps
  it: a sync entity's id is derived from its forge project (see
  [Sync](#sync)).
- **An operation's id** is `<pack hash>:<index>`, where the index is the
  operation's position in its pack, from 0.

An issue whose first pack has the hash
`3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5` has that id. Its `create`
operation, the first in that pack, is:

```json
"3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5:0"
```

A comment appended later, alone in a pack whose hash is
`8c04e5b1d97f2a63c18e0b4d5f6a7928e3b1c0d4`, is:

```json
"8c04e5b1d97f2a63c18e0b4d5f6a7928e3b1c0d4:0"
```

An operation's id is known once its pack is written. A write retried on a
new head writes a new pack, so its operations get new ids (see
[Updating a ref](storage.md#updating-a-ref)).

## Issues

An issue has a title, a body, a status, labels and comments. It has six
operations:

| `kind` | `data` | What it does |
|---|---|---|
| `create` | `title`, `body` | opens the issue |
| `comment` | `body` | adds a comment |
| `edit-text` | `target`, `body` | replaces the body, or a comment's body |
| `set-title` | `title` | replaces the title |
| `set-status` | `status`: `"open"` or `"closed"` | opens or closes the issue |
| `change-labels` | `add`, `remove`: lists of labels | adds and removes labels |

### `create`

```json
{
  "kind": "create",
  "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
  "time": 1792834200,
  "data": {
    "title": "Search ignores closed issues",
    "body": "Searching for a closed issue finds nothing."
  }
}
```

### `comment`

```json
{
  "kind": "comment",
  "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
  "time": 1792836300,
  "data": {
    "body": "It finds them with is:closed."
  }
}
```

### `edit-text`

`target` is an operation's id: the `create` operation's for the issue's
body, or a `comment` operation's for that comment's body. As `create` is
the first operation of an issue's first pack, the body's target is always
`<issue id>:0`.

```json
{
  "kind": "edit-text",
  "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
  "time": 1792836400,
  "data": {
    "target": "3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5:0",
    "body": "Searching for a closed issue finds nothing, unless the query says is:closed."
  }
}
```

### `set-title`

```json
{
  "kind": "set-title",
  "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
  "time": 1792836500,
  "data": {
    "title": "Search hides closed issues by default"
  }
}
```

### `set-status`

```json
{
  "kind": "set-status",
  "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
  "time": 1792836600,
  "data": {
    "status": "closed"
  }
}
```

### `change-labels`

Either list can be empty or absent. A label is a string that isn't empty
and holds no line feed (`\n`), as sync writes a set of labels one per line.

```json
{
  "kind": "change-labels",
  "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
  "time": 1792836700,
  "data": {
    "add": ["bug", "assignee:guilbep"],
    "remove": ["triage"]
  }
}
```

### Labels

Every field of an issue besides its title, body, status and comments is a
label, written `ns:value`: `priority:high`, `type:bug`. Assignees are
labels too, `assignee:<name>`. A label without a colon, like `bug`, is a
plain label.

```json
["assignee:guilbep", "bug", "priority:high"]
```

The core keeps labels as strings and gives no namespace a meaning: git-board
reads them. Assignees are the one field a forge holds as its own: sync makes
`assignee:` labels the forge's assignees, and says how on the Sync page.

### State {#issue-state}

An issue's state follows from its operations, applied in order:

1. The first operation must be `create`. A log that starts with anything
   else is an error, and so is a second `create`.
2. `create` sets the title and the body, and the status to `open`. The
   issue's author, creation time and origin are the `create` operation's.
3. `comment` adds a comment after the others. The comment's id is the
   operation's id, and its author, time and origin are the operation's.
   A `comment` whose `origin` equals an earlier comment's, in both `forge`
   and `event`, is that same comment: it adds none, and its id becomes
   another id of the earlier comment. A `comment` without an `origin` is
   never the same as another.
4. `edit-text` replaces the text of its target. A target that isn't the
   `create` operation, nor the id of a `comment` met earlier in the order,
   is an error.
5. `set-title` replaces the title.
6. `set-status` sets the status. A status other than `open` or `closed` is
   an error.
7. `change-labels` adds the labels in `add`, then removes the labels in
   `remove`. Labels are a set: adding a label already there, or removing one
   that isn't, changes nothing. Operations apply in order, so the later one
   wins. An empty label, or one holding a line feed, in either list, is an
   error.
8. An operation whose `author` is empty is an error.
9. Any other `kind` is an error, which names the kind.

Two clones that pull the same forge comment before fetching each other each
write a `comment` for it, with the same `origin`. Once they merge, the first
in the order is the comment, and an `edit-text` of either id edits it. The
first wins because it follows the order every clone shares, so every clone
keeps the same one, as with a sync entity's [links](#sync).

An issue's state holds:

| Field | What it holds |
|---|---|
| `id` | the issue's id |
| `author` | the `create` operation's author |
| `created` | the `create` operation's time |
| `updated` | the time of the issue's last operation in the order, whatever it does |
| `origin` | the `create` operation's origin; absent if it has none |
| `title`, `body`, `status` | as the operations left them |
| `labels` | the labels, sorted and unique |
| `comments` | the comments, in the order they were added |

A comment holds:

| Field | What it holds |
|---|---|
| `id` | the `comment` operation's id |
| `author` | the `comment` operation's author |
| `time` | the `comment` operation's time |
| `edited` | the time of the last `edit-text` of the comment in the order, by any of its ids; 0 if it was never edited |
| `origin` | the `comment` operation's origin; absent if it has none |
| `body` | its text, as the last `edit-text` left it |

Like every `time`, `updated` and `edited` are for people to read. They are
the last operation's time, not the highest: when wall clocks disagree, an
operation can carry an earlier time than one before it.

The state isn't stored. Shown as JSON, the issue above, closed and with its
labels changed, is:

```json
{
  "id": "3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5",
  "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
  "created": 1792834200,
  "updated": 1792836700,
  "title": "Search hides closed issues by default",
  "body": "Searching for a closed issue finds nothing, unless the query says is:closed.",
  "status": "closed",
  "labels": ["assignee:guilbep", "bug"],
  "comments": [
    {
      "id": "8c04e5b1d97f2a63c18e0b4d5f6a7928e3b1c0d4:0",
      "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
      "time": 1792836300,
      "edited": 0,
      "body": "It finds them with is:closed."
    }
  ]
}
```

It was made in git-board, so it shows no `origin`. A pulled issue's state
does (see [Origins](#origins)).

## People

A person is someone who makes operations: a name, an email, and the
accounts they have on forges. Every operation's `author` is a person's id.
A person is created without an id, so its id is its first pack's hash (see
[Ids](#ids)).

That pack makes the person's id, so no operation in it can name the person
itself. The operations of a person's first pack, the pack whose hash is
its id, may have an empty author: a person created by a pull, or the first
person of a repository, has no one else to name. Every later operation's
author is a person's id.

An account is one user on one forge server:

- `server` is the forge's host, like `github.com` or `gitlab.epfl.ch`;
- `id` is the forge's own id for that user, numeric or opaque, as a string.
  It stays when the user renames their login (see [GitHub](forges.md#github));
- `login` is the user's login there.

An account is named by its `server` and `id`, compared byte for byte.

A person has five operations:

| `kind` | `data` | What it does |
|---|---|---|
| `create` | `name`, `email` | makes the person |
| `set-name` | `name` | replaces the name |
| `set-email` | `email` | replaces the email |
| `add-account` | `server`, `id`, `login` | adds an account, or changes its login |
| `remove-account` | `server`, `id` | removes an account |

### `create` {#person-create}

`create` is the first operation of a person's first pack, so its author
may be empty. A pull writes a person's `create` and its `add-account` in
that one pack (see [Authors](sync.md#authors)). A person created in
git-board names the person who created it.

The first person of a repository:

```json
{
  "kind": "create",
  "author": "",
  "time": 1792830000,
  "data": {
    "name": "guilbep",
    "email": "guilbep@example.com"
  }
}
```

Its first pack's hash, `b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40`, is its
id, and the author of its later operations.

### `set-name`

```json
{
  "kind": "set-name",
  "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
  "time": 1792830100,
  "data": {
    "name": "Pierre"
  }
}
```

### `set-email`

An empty `email` clears it.

```json
{
  "kind": "set-email",
  "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
  "time": 1792830200,
  "data": {
    "email": "pierre@example.com"
  }
}
```

### `add-account`

An account the person already has, with the same `server` and `id`, takes
the new `login`. That is how a renamed login is followed.

```json
{
  "kind": "add-account",
  "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
  "time": 1792830300,
  "data": {
    "server": "github.com",
    "id": "4815162",
    "login": "guilbep"
  }
}
```

### `remove-account`

```json
{
  "kind": "remove-account",
  "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
  "time": 1792830400,
  "data": {
    "server": "gitlab.com",
    "id": "9910331"
  }
}
```

### State {#person-state}

A person's state follows from its operations, applied in order:

1. The first operation must be `create`. A log that starts with anything
   else is an error, and so is a second `create`.
2. `create` sets the name and the email.
3. `set-name` replaces the name.
4. `set-email` replaces the email. An empty one clears it.
5. `add-account` adds the account. If the person already has one with the
   same `server` and `id`, it replaces that one's login instead.
6. `remove-account` removes the account with that `server` and `id`.
   Removing one the person doesn't have changes nothing.
7. An empty `name`, `server`, `id` or `login`, in any operation that holds
   it, is an error. The email may be empty.
8. An operation whose `author` is empty is an error, except in the person's
   first pack.
9. Any other `kind` is an error, which names the kind.

A person's state holds its `id`, `name`, `email` and `accounts`. The
accounts are sorted by (`server`, `id`), in byte order, and unique by both.
The state isn't stored. Shown as JSON, the person above, with accounts on
GitHub and gitlab.epfl.ch, is:

```json
{
  "id": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
  "name": "Pierre",
  "email": "pierre@example.com",
  "accounts": [
    {
      "server": "github.com",
      "id": "4815162",
      "login": "guilbep"
    },
    {
      "server": "gitlab.epfl.ch",
      "id": "2342",
      "login": "guilbep"
    }
  ]
}
```

### Who owns an account {#account-owner}

Two persons can hold the same account. Two clones that pull the same forge
user before fetching each other each create a person for it. The account
then belongs to the person whose id sorts first, in byte order. A lookup by
account answers that person.

- **Every clone agrees** once it holds both persons, as the rule reads only
  their ids.
- **The other person keeps the account** in its state. Only lookups pass it
  over.
- **Operations keep their author.** Those written with the other person's
  id still name it.

The GitHub account `583231`, held by the persons
`4a9d2e7f1b0c6e38d5a2f9b4c7e1d0a36b8f2c95` and
`d03f6a1c8e5b2f7940a3c6e9d1b4f8a27c5e0b63`, belongs to the first.

### Me

Who "I" am is the local git config key `board.me`, holding a person id:

```sh
git config --local board.me b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40
```

git never fetches nor pushes config, so it is never shared: each clone says
who it is. Unset, it names no one, as in CI.

## Sync

A sync entity records what the core knows of one forge project: which issue
is which forge issue, and the value of each field that git-board and the
forge last agreed on. There is one per forge project.

Its id is the SHA-1 of the project's forge key, in hex. A forge key names one
project on one forge server, like `github.com/guilbep/git-board-test`, whose
sync entity is:

```json
"56f7a8e819f90e968812bac54ae4b06142d8e735"
```

The adapters (M4) fix each forge's form of key.

A sync entity has two operations:

| `kind` | `data` | What it records |
|---|---|---|
| `link` | `issue`, `forge_id`, `number`, `url` | an issue and the forge issue it mirrors |
| `seen` | `issue`, `field`, `value` | the value of one field of a linked issue that git-board and the forge last agreed on |

### `link`

`issue` is the issue's id. `forge_id` is the forge's own id for the issue,
`number` its number in the project, and `url` its web page. All four are
strings.

```json
{
  "kind": "link",
  "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
  "time": 1792836800,
  "data": {
    "issue": "3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5",
    "forge_id": "2791543210",
    "number": "12",
    "url": "https://github.com/guilbep/git-board-test/issues/12"
  }
}
```

### `seen`

`field` is one of `title`, `body`, `status`, `labels`, `assignees`, or
`comment:<forge comment id>` for a comment. `value` is a string. The Sync
page says how each field's value is written, and how sync uses it.

A `seen` is the base sync compares both sides with. When git-board and the
forge both changed a field, its `seen` stays as it was, so the conflict
stays visible to a later rule. Labels are the exception: they merge per
label, and so can't conflict.

```json
{
  "kind": "seen",
  "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
  "time": 1792836800,
  "data": {
    "issue": "3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5",
    "field": "title",
    "value": "Search hides closed issues by default"
  }
}
```

### State {#sync-state}

A sync entity's state follows from its operations, applied in order:

- Its links, each joining an issue to a forge issue. The first link in the
  order wins. For an issue linked twice, and for one forge issue (one
  `forge_id`) linked from two issues, the later link is ignored.
- A link to an issue not present locally is kept, as the issue may arrive
  with the next fetch. It is skipped until then.
- For each linked issue and field, the value of the last `seen`. A `seen`
  for an issue without a link at that point in the order, or with a field
  not listed above, is ignored.
- An operation whose `author` is empty is an error.
- Any other `kind` is an error, which names the kind.

The first link wins because it follows the order every clone shares, so
every clone keeps the same one.

Two clones that pull the same new forge issue before fetching each other
each create an issue for it, and link it. Once they merge, the first link
holds and the other issue is left unlinked. Joining such duplicates is
settled in M3.

## Origins

An operation pulled from a forge has an `origin`, which says where it came
from:

- `forge` is the forge key of its project;
- `event` names what on the forge it came from.

Until sync reads the forges' events (M3), `event` is `issue:<forge issue
id>` for an issue's own operations, and `comment:<forge comment id>` for a
comment.

```json
{
  "kind": "create",
  "author": "4a9d2e7f1b0c6e38d5a2f9b4c7e1d0a36b8f2c95",
  "time": 1792834200,
  "origin": {
    "forge": "github.com/guilbep/git-board-test",
    "event": "issue:2791543210"
  },
  "data": {
    "title": "Search ignores closed issues",
    "body": "Searching for a closed issue finds nothing."
  }
}
```

An operation made in git-board has no `origin`.

The state of the issue that `create` opens shows its origin:

```json
{
  "id": "6e2b9f4a1d7c03e85b6a2f9c4d1e7b0a3c8f5d29",
  "author": "4a9d2e7f1b0c6e38d5a2f9b4c7e1d0a36b8f2c95",
  "created": 1792834200,
  "updated": 1792834200,
  "origin": {
    "forge": "github.com/guilbep/git-board-test",
    "event": "issue:2791543210"
  },
  "title": "Search ignores closed issues",
  "body": "Searching for a closed issue finds nothing.",
  "status": "open",
  "labels": [],
  "comments": []
}
```
