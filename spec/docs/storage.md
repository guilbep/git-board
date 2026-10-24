# Storage

An entity lives in git as a chain of commits under one ref. Each commit
holds one pack of operations and a clock. This page gives the exact format,
the order of the operations, and how heads merge, refs move, and clones
fetch and push.

The core works only in SHA-1 repositories, as go-git v5, which it reads git
with, has no other object format. Every hash here is 40 lowercase hex
digits.

## Refs

An entity's ref is `refs/board/<kind>/<id>`, for a kind and an id as
[Entities](data-model.md#entities) gives them. It points at the entity's
head commit.

```text
refs/board/issue/3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5
refs/board/sync/56f7a8e819f90e968812bac54ae4b06142d8e735
```

- Entities live only there. Listing a kind's entities reads the refs under
  `refs/board/<kind>/`, and gives their ids sorted.
- The kinds, `issue`, `person` and `sync`, are one list in the core. The
  name check, fetch and push all read it.
- `refs/board/seq` is git-board's own counter of project numbers. It isn't
  an entity, and the core leaves it as it is.

## Commits

An entity commit is:

- **its tree:** one entry, `pack`, a blob (mode `100644`);
- **its parents:** none for the entity's first commit, the head it was
  written on for an append, and both heads for a merge;
- **its author and committer:** `git-board <git-board@localhost>`;
- **its message:** `git-board`;
- **unsigned,** whatever git's configuration asks for.

```text
$ git cat-file -p refs/board/issue/3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5
tree <tree>
parent <parent>
author git-board <git-board@localhost> <time>
committer git-board <git-board@localhost> <time>

git-board

$ git cat-file -p <tree>
100644 blob 8c04e5b1d97f2a63c18e0b4d5f6a7928e3b1c0d4	pack
```

The core never reads a commit's author, dates or message. Only its tree and
its parents carry meaning.

## Packs

A pack is a JSON blob:

```text
{"clock": N, "nonce": "<hex>", "ops": [...]}
```

- **`clock`:** the commit's clock, an integer from 1 (see [Clock](#clock)).
- **`nonce`:** at least 16 random bytes, in hex. It is present only in an
  entity's first pack. Two entities created with the same operations at
  the same second still get different ids.
- **`ops`:** the operations, in the order they were written (see [Data
  model](data-model.md#operations)). An entity's first pack holds at least
  one. A merge commit's pack holds none: `"ops": []`.

An issue's first pack:

```json
{
  "clock": 1,
  "nonce": "9b1e4f27c03a5d86e2f1a0b7c4d3e5f6",
  "ops": [
    {
      "kind": "create",
      "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
      "time": 1792834200,
      "data": {
        "title": "Search ignores closed issues",
        "body": "Searching for a closed issue finds nothing."
      }
    }
  ]
}
```

An append, on that first commit:

```json
{
  "clock": 2,
  "ops": [
    {
      "kind": "comment",
      "author": "b7e41c09d2a85f36e1c4a0d7b9f2e5c83a6d1f40",
      "time": 1792836300,
      "data": {
        "body": "It finds them with is:closed."
      }
    }
  ]
}
```

A merge, of two heads whose clocks are 2:

```json
{"clock": 3, "ops": []}
```

A pack's hash is the git object id of its blob, as stored. Ids and the order
come from that hash, so a reader never encodes a pack again to hash it. Any
JSON encoding of a pack is valid. The core writes packs compact, with the
keys in the order above.

## Clock

A commit's clock is 1 + the highest of its parents' clocks, and 1 for a
commit with no parent. It is stored in the commit's pack.

So a commit's clock is higher than the clock of every commit it descends
from.

## What a reader rejects

A reader rejects, with an error naming the commit:

- a commit whose tree has no `pack` entry;
- a pack that isn't JSON;
- a clock other than 1 + the highest of the parents' clocks, or 1 for a
  commit without parents;
- an `ops` that is empty or absent, in a commit with fewer than two
  parents;
- an operation whose `kind` is empty or absent, or whose `time` is 0 or
  absent.

An empty `author` is for each kind's rules to judge, as a person's first
pack may have one (see [Data model](data-model.md#operations)).

It ignores other entries in the tree, pack fields it doesn't know, and the
`nonce`, which only names an entity when it is created.

The reader is strict where corrupt data would reorder the history, and
lenient where a newer version may add things.

## Order

An entity's operations are every reachable commit's pack, sorted by (clock
ascending, pack blob hash ascending), with each pack's ops in their stored
order.

- A reachable commit is the head, or any commit found from it by following
  parents.
- Hashes compare as lowercase hex strings.
- Two commits whose packs are the same blob, byte for byte, count as one
  pack. The same bytes are the same change, and their operations would have
  the same ids.
- The operations' `time` plays no part.

Two clones, A and B, start from the same first commit. Offline, A adds a
comment and B sets the title. Then they merge:

| Commit | Clock | Pack hash | Operations |
|---|---|---|---|
| first | 1 | `3f7a9c2e41b6d08a5e1f93c7b2d4a6e8f0c1b3d5` | `create` |
| B's | 2 | `1d9e6b0a4c3f8e2d7b5a9c1e0f4d6b8a2c3e5f70` | `set-title` |
| A's | 2 | `8c04e5b1d97f2a63c18e0b4d5f6a7928e3b1c0d4` | `comment` |
| merge | 3 | `e5a0c7d2b9f4e1a83c6d0b5f2e7a9c4d1b8e3f06` | none |

Both clones then read `create`, `set-title`, `comment`, in that order. B's
pack comes before A's because, at the same clock, its hash is lower.

### Why every clone agrees

- The order depends only on the packs reachable from the head: their clocks
  and their hashes. Both are in the objects themselves. Which clone wrote a
  commit, when, and in which order the commits arrived play no part.
- Once two clones have merged each other's heads, they reach the same packs.
  So they read the same operations in the same order, and get the same state.
- An operation written after another was seen comes after it: its commit
  descends from the other's, so its clock is higher. Wall clocks can
  disagree between machines; the order doesn't read them.
- Two operations made without seeing each other are ordered by clock, then
  by hash. The result is arbitrary, but the same everywhere.

## Merging two heads

Merging an entity's local head with another head, from a fetch:

- **The local head contains the other** (they are equal, or the other is
  reachable from the local head): nothing changes.
- **The other contains the local head:** the ref fast-forwards to the
  other head. No commit is made.
- **Neither contains the other:** a commit whose parents are the local
  head, then the other, and whose pack holds no operation and a clock of
  1 + the higher of the two heads' clocks.
- **There is no local ref:** it is created at the other head.

A merge writes no operation. It only joins the two logs.

An entity created with a given id, like a sync entity, can be created on two
clones before either fetches the other. It then has two first commits, each
with a clock of 1 and a nonce. Neither contains the other, so they merge
like any two heads.

Two clones that merge each other's heads at the same moment each write a
merge commit. Neither pack holds an operation, so the two heads differ until
the next merge joins them, but their operations, and so their states, are
the same.

Known point, not settled yet: two clones that exchange directly and merge
at the same time can keep doing so, each writing a merge commit for the
other's, so their refs keep diverging while their states agree. Through one
origin, with unforced pushes, they converge: the second push is refused, and
its writer fetches and merges onto the first's head.

## Updating a ref

A write moves a ref only if the ref still points where the write read it:

```sh
git update-ref <ref> <new> <old>
```

- `<old>` is the head the write was built on. For a ref that must not exist
  yet, it is the zero hash, `0000000000000000000000000000000000000000`.
- git refuses when the ref has moved, when it exists and must not, or when
  another process holds its lock file at that moment. Each one means the
  ref moved.
- When the ref moved, the writer loads the entity again and writes its pack
  again on the new head, with the new head's clock + 1. A merge merges
  again, with the new local head. It makes at most 5 attempts in total,
  then fails.
- A write that lands doesn't read the entity again. Its commit's clock is
  one more than the head's, which is the highest of every reachable commit.
  So its pack sorts last, and its operations come after every one the
  writer read, with the ids `<pack hash>:<index>`.

There are no lock files. Two processes appending to the same entity at once
both land, the second after a retry. A retry writes a new pack, so the
retried operations get new ids.

Planned for M3: a sync run takes a lock, so that one machine doesn't post
the same thing twice.

## Fetch and push

Fetch and push run the `git` binary, so every remote, push URL and
credential helper git knows works. Both name the three entity kinds, one
refspec each, in one call. `refs/board/seq` and any other ref under
`refs/board/` belong to git-board, not to the core, and the core never
fetches nor pushes them.

Fetch:

```sh
git fetch <remote> \
  +refs/board/issue/*:refs/board-remotes/<remote>/issue/* \
  +refs/board/person/*:refs/board-remotes/<remote>/person/* \
  +refs/board/sync/*:refs/board-remotes/<remote>/sync/*
```

Then each `refs/board-remotes/<remote>/<kind>/<id>` is
[merged](#merging-two-heads) into `refs/board/<kind>/<id>`, which is created
when absent.

Fetch never propagates a deletion. An entity whose ref was deleted on the
remote keeps its ref here, and its fetched head in `refs/board-remotes/`:
the core never runs `git fetch --prune`. Push deletes nothing on the remote
either.

The fetched heads stay out of `refs/remotes/`, so they don't show as remote
branches in `git branch -r` or in an editor's branch list, `git fetch
--prune` doesn't delete them, and a remote branch named `board` can't
collide with them.

Push:

```sh
git push <remote> \
  refs/board/issue/*:refs/board/issue/* \
  refs/board/person/*:refs/board/person/* \
  refs/board/sync/*:refs/board/sync/*
```

The push isn't forced. When a ref moved on the remote, git refuses it:
fetch, merge, then push again.
