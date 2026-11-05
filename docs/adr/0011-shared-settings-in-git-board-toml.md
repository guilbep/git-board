# ADR 0011: Shared settings live in a committed `.git-board.toml`; each clone's own in git config

- Status: accepted (recorded 2026-11-04, decided earlier)
- Date: before 2026-09-02 for the file (`docs/implementation-plan.md` › Why
  this exists: "sprints/views are `.git-board.toml`, reviewed and diffed
  like the rest of the repo"); 2026-10-24 for forges in it and "who I am is
  local" (`docs/prd/core.md`); 2026-11-04 for `board.datastore` (ADR 0002
  As built).
- Sources: `docs/guide/configuration.md`; `internal/config/config.go`;
  `docs/spec/data-model.md` › Me.
- Alternatives below are reconstructed, except `board.datastore`'s, which
  ADR 0002 records.

## Context

git-board replaces a GitHub Project whose fields, views and automations
lived in GitHub's database and drifted unseen. A project's ontology (its
fields, their order, sprints, views, forges, cycle checks, releases) should
be reviewed like code. Some settings are not the project's but one
person's or one machine's: who I am, which page I prefer, where this clone
keeps its board, which writer signs proof notes.

## Decision

### 1. The project's settings: `.git-board.toml`, committed

- One TOML file at the repository root, found from any subdirectory
  (`internal/config/config.go:309` `FileName`, `:315` `Path`). Optional:
  with none, every `ns:value` label is a field (`:343` `Load`).
- It holds fields and types, labels, colors, views and roles, sprints,
  the GitHub Project, `[forges.<name>]` and `publish-private`, uploads'
  addresses, branches, the cycle's tests and checks, numbers, releases
  (`docs/guide/configuration.md`).
- Settings › Project file edits it in the web app; edits by hand need a
  server restart.
- The core never reads it: git-board passes what sync needs as options
  (ADR 0005).
- A gate reads it from the base branch, never from the branch under review
  (`internal/config/cycle.go:65-82`, `AtBase`, ADR 0001 § 5).
- It never holds a secret: no token (ADR 0010), no S3 key (ADR 0015), no
  datastore URL (ADR 0002).

### 2. Each clone's own settings: local git config

| Key | What | Where read |
| --- | --- | --- |
| `board.me` | who writes, a person's id | `core/people/ops.go:105-121` |
| `board.view` | classic page or the cycle's | `internal/store/store.go:619`, `:669` |
| `board.datastore` | `git`, `forge`, `markdown:<dir>`, a `postgres://` URL; `GIT_BOARD_DATASTORE` wins | `internal/server/datastore.go:13-20` |
| `git-board.writer` | this clone's proof-notes writer | `core/proof/notes.go:19-24` |

- Never shared, never the board. For a datastore with no clone, `board.me`
  is the file `<folder>/me` (ADR 0002).
- `board.view` may also be set `--global`, for every repository
  (`docs/guide/configuration.md:419`).

### Amendment (2026-11-05): the file has a schema version

[ADR 0021](0021-git-board-v2-same-data-new-code.md) asked for it: the
file says its format's version in a top-level `schema = N`, none being
version 1 (v0.11's), and git-board migrates it between versions, up and
down, editing only the lines that change. An older file is read
migrated in memory and written upgraded on the next explicit write; a
newer one is read for what git-board knows, and never written. The
versions, the commands and how a migration is added are in
[The file's schema version](../guide/config-schema.md).

## Alternatives considered

Reconstructed. Scored on: reviewed and versioned with the code (35), no
secret committed (25), per-person choices stay personal (20), cost (20).

| Option | Score | Why not |
| --- | --- | --- |
| **Committed TOML for the project, git config for the clone (this decision)** | 85 | |
| Settings as entities in the board (a `settings` kind) | 50 | Versioned and synced, but not reviewed in pull requests, and a gate couldn't read them "at the base branch" |
| Everything in git config | 35 | Not shared through the repository; each clone set up by hand |
| Everything in `.git-board.toml`, `me` included | 30 | A committed file can't say who each reader is; a datastore URL may hold a password |
| YAML or JSON instead of TOML | 60 | Workable; TOML was already a dependency (`BurntSushi/toml`) and reads well by hand |

## Consequences

Good:

- A change to the fields or the cycle is a reviewed diff.
- Each branch or worktree can try its own settings.
- Personal choices don't churn the file.

Bad, and accepted:

- Settings differ per branch: the server reads the checkout it was started
  in.
- A hand edit applies only after a restart.
- A setting the team wants shared but private (S3 keys, the datastore URL)
  has to live in each server's environment.

## Left open

- **Two namespaces in git config:** `board.*` for the board and
  `git-board.writer` for proof notes. Nothing records why they differ.
- `docs/guide/configuration.md:5` still says git-board "works on any
  git-bug repository" (ADR 0003).
- `NumbersRemote` is hard-coded to `origin`, "a setting when a project's
  isn't" (`internal/store/numbers.go:33-36`, a `ponytail:` note; ADR 0016).
