# ADR 0011: Shared settings in the file, your own in git config

- Status: accepted; merged 2026-11-09 from ADR 0011 (decided before
  2026-09-02, extended 2026-10-24 and 2026-11-04, amended 2026-11-05)
- Date: 2026-09-02, 2026-10-24, 2026-11-05
- Absorbs no other ADR.
- Guide: [`.git-board.toml`](../guide/configuration.md) and
  [The file's schema version](../guide/config-schema.md).

## Context

git-board replaces a GitHub Project whose fields, views and automations
lived in GitHub's database and drifted unseen. A project's fields, their
order, its sprints, views, forges, checks and releases are the team's.
The team reviews them as it reviews code. Other choices belong to one
person or one machine: who writes, where this clone keeps its board, who
signs the proof notes.

## Decision

### 1. The project's configuration is `.git-board.toml`, committed

- One TOML file at the root of the repository. It is optional: with no
  file, every `ns:value` label is a field.
- It holds the fields and their types, labels, colors, views and roles,
  sprints, the GitHub Project, the forges and `publish-private`, the
  uploads' addresses, branches, the cycle's tests and checks, numbers and
  releases.
- Settings › Project file edits it in the web app. A hand edit applies
  when the server starts again.
- The kernel and the datastores never read it. Each domain declares and
  reports the problems of its own table (`conf/`, `app/config.go`).
- A gate reads it from the base branch, never from the branch under
  review (`app/verify.go`, `AtBase`;
  [ADR 0001](0001-check-results-are-signed-git-notes.md), section 5).
- It never holds a secret: no token
  ([ADR 0010](0010-tokens-from-env-or-git-credential-helper.md)), no S3
  key and no datastore URL ([ADR 0002](0002-datastores.md)).

### 2. The file has a schema version

The file states its format's version in a top-level `schema = N`. A file
with no `schema` is version 1. git-board migrates the file between
versions, up and down, and edits only the lines that change.

- An older file is read migrated in memory, and written upgraded on the
  next explicit write.
- A newer file is read for what git-board knows, and never written.

### 3. Each clone's own configuration is local git config

| Key | What it holds |
| --- | --- |
| `board.me` | who writes: a person's id (`git board me`) |
| `board.datastore` | `git`, `forge`, `markdown:<dir>` or a `postgres://` URL. `GIT_BOARD_DATASTORE` wins over it |
| `board.experimental` | a comma list that replaces the file's `[experimental] features` in this clone |
| `board.avatars` | `forge`, `gravatar` or `none`, from any level of git config |
| `git-board.writer` | this clone's writer of proof notes |

- These keys are never shared, and are not the board. `board.me` stays
  in the clone's git config, whatever datastore keeps the board.
- No key chooses the issue page. The cycle's page shows when the cycle is
  on: `[experimental] features` holds `cycle` in the file, or
  `board.experimental` does in the clone
  ([ADR 0001](0001-check-results-are-signed-git-notes.md), section 10).

## Consequences

Good:

- A change to the fields or the cycle is a reviewed diff.
- Each branch or worktree can try its own configuration.
- Personal choices do not churn the file.
- A file of an older git-board opens in a newer one, and the reverse.

Bad, and accepted:

- The configuration differs per branch. The server reads the checkout it
  started in.
- A hand edit applies only after a restart.
- A value that the team shares but keeps private, such as the S3 keys or
  the datastore URL, lives in each server's environment.
- git config has two namespaces: `board.*` for the board and
  `git-board.writer` for the proof notes.
