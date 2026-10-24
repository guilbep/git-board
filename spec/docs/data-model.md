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
- the id is lowercase hex, and unique within its kind (see [Ids](#ids)).

An entity's ref names both: `refs/board/issue/3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5`
is the issue `3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5`.

## Operations

An operation is one change to an entity, as a JSON object:

| Field | Type | What it holds |
|---|---|---|
| `kind` | string | what the operation does, from its entity's list below |
| `author` | string | who made it |
| `time` | integer | when it was made, in Unix seconds |
| `origin` | object | where it came from, on an operation pulled from a forge; absent otherwise (see [Origins](#origins)) |
| `data` | object | the operation's own fields, which depend on its `kind` |

```json
{
  "kind": "comment",
  "author": "guilbep",
  "time": 1792836300,
  "data": {
    "body": "It finds them with is:closed."
  }
}
```

- `kind` is read in the context of the entity: `create` is an issue's
  operation, `link` is a sync entity's.
- `author`, in M1, is a name the caller gives, or `forge:<login>` for an
  operation pulled from a forge. From M2, it is a person's id.
- `time` is for people to read. It never orders operations: the
  [clock](storage.md#clock) does.
- An operation holds no id. Its id comes from where it is stored (see
  [Ids](#ids)).

Reading an operation:

- a field the reader doesn't know, in the operation or in its `data`, is
  ignored;
- a field missing from `data` is its empty value: an empty string, or an
  empty list;
- a `kind` the reader doesn't know is an error, which names the kind. The
  reader never drops an operation it can't apply.

## Ids

An entity's operations are stored in packs, one per git commit (see
[Packs](storage.md#packs)). A pack's hash is the git object id of its
blob, in lowercase hex.

- **An entity's id.** An entity created without an id gets its first
  pack's hash. An entity created with an id keeps it: a sync entity's id is
  derived from its forge project (see [Sync](#sync)).
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
  "author": "guilbep",
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
  "author": "guilbep",
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
  "author": "guilbep",
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
  "author": "guilbep",
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
  "author": "guilbep",
  "time": 1792836600,
  "data": {
    "status": "closed"
  }
}
```

### `change-labels`

Either list can be empty or absent.

```json
{
  "kind": "change-labels",
  "author": "guilbep",
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

### State

An issue's state follows from its operations, applied in order:

1. The first operation must be `create`. A log that starts with anything
   else is an error, and so is a second `create`.
2. `create` sets the title and the body, and the status to `open`. The
   issue's author, creation time and origin are the `create` operation's.
3. `comment` adds a comment after the others. The comment's id is the
   operation's id, and its author, time and origin are the operation's.
4. `edit-text` replaces the text of its target. A target that isn't the
   `create` operation, nor a `comment` met earlier in the order, is an
   error.
5. `set-title` replaces the title.
6. `set-status` sets the status. A status other than `open` or `closed` is
   an error.
7. `change-labels` adds the labels in `add`, then removes the labels in
   `remove`. Labels are a set: adding a label already there, or removing one
   that isn't, changes nothing. Operations apply in order, so the later one
   wins.
8. Any other `kind` is an error, which names the kind.

The state's labels are sorted and unique. The state isn't stored; shown as
JSON, the issue above, closed and with its labels changed, is:

```json
{
  "id": "3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5",
  "author": "guilbep",
  "created": 1792834200,
  "title": "Search hides closed issues by default",
  "body": "Searching for a closed issue finds nothing, unless the query says is:closed.",
  "status": "closed",
  "labels": ["assignee:guilbep", "bug"],
  "comments": [
    {
      "id": "8c04e5b1d97f2a63c18e0b4d5f6a7928e3b1c0d4:0",
      "author": "guilbep",
      "time": 1792836300,
      "body": "It finds them with is:closed."
    }
  ]
}
```

## People

Planned for M2. A person will have a name, and an account per forge server,
with the forge's numeric id and login. Its operations aren't defined yet,
and M1 writes no `person` entity. Until then, an operation's `author` is a
plain string.

## Sync

A sync entity records what the core knows of one forge project: which issue
is which forge issue, and what the forge last showed of each field. There is
one per forge project.

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
| `seen` | `issue`, `field`, `value` | what the forge last showed of one field of a linked issue |

### `link`

`issue` is the issue's id. `forge_id` is the forge's own id for the issue,
`number` its number in the project, and `url` its web page. All four are
strings.

```json
{
  "kind": "link",
  "author": "guilbep",
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

```json
{
  "kind": "seen",
  "author": "guilbep",
  "time": 1792836800,
  "data": {
    "issue": "3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5",
    "field": "title",
    "value": "Search hides closed issues by default"
  }
}
```

### State

A sync entity's state follows from its operations, applied in order:

- Its links, each joining an issue to a forge issue. The first link in the
  order wins. For an issue linked twice, and for one forge issue (one
  `forge_id`) linked from two issues, the later link is ignored.
- A link to an issue not present locally is kept, as the issue may arrive
  with the next fetch. It is skipped until then.
- For each linked issue and field, the value of the last `seen`. A `seen`
  for an issue without a link at that point in the order, or with a field
  not listed above, is ignored.
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
  "author": "forge:octocat",
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
