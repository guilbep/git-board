# ADR 0018: You own your issues; work in progress is opt-in

- Status: accepted (2026-11-04)
- Date: 2026-11-04
- Builds on [ADR 0003](0003-own-core-instead-of-git-bug.md) (git-board's
  own core) and [ADR 0002](0002-datastores.md) (where the board is kept).

## Context

git-board grew two kinds of features:

1. **What makes the issues yours**: issues, people and their history kept
   as operations in your repository (or your PostgreSQL, or a folder of
   Markdown files), read and written from the command line, a TUI and a web
   UI, and synced with GitHub and GitLab, which become mirrors.
2. **A development workflow on top**: the cycle (an issue's steps from
   intent to verified, with signed check results, ADR 0001), coding agents
   started on an issue, and worktrees through `wtx`.

The second kind is younger, changes fast, and needs tools many users don't
have (wtx, an agent, a signing key). Shown to everyone, it hides what the
product is for, and a broken `[cycle]` table broke every command
(`internal/config` validated it on every load). The code review of
2026-11-04 found no code dependency of the first kind on the second.

## Decision

### 1. The core is ownership

git-board exists so that a team owns its issues: independent of GitHub,
GitLab or any forge, with every change kept, offline, and nothing lost when
a forge goes away or changes its rules. A feature belongs to the core when
it serves that:

- the issue's data and history are the team's, in a store the team chooses
  (ADR 0002), with the same ids everywhere;
- a forge is a mirror the sync keeps in step, never the source of truth
  (ADR 0008);
- everything a forge's CLI or UI does for issues, git-board does without
  it: the CLI aims past `gh issue` and `glab issue`
  ([CLI reference](../guide/cli.md));
- leaving a forge, or git-board itself, loses nothing: export to Markdown,
  copy between datastores, the operations readable without git-board.

### 2. Work in progress is opt-in

The cycle, the agents and the worktrees are **experimental**, off by
default, behind one switch:

```toml
[experimental]
features = ["cycle", "agents", "worktrees"]
```

in `.git-board.toml` (the team opts in), replaced in one clone by
`git config board.experimental cycle,agents` (someone trying it alone).

Off, a feature is absent, not broken:

- its commands are hidden and refuse with a sentence naming the switch
  (`git board verify`);
- its API routes answer 404 `{"code": "experimental"}`;
- the web UI hides its pages, tabs, menu entries and controls (`GET
  /api/me` lists what is on);
- its configuration isn't checked: a broken `[cycle]` is a warning, not an
  error for every command.

On, everything works as before. The switch is
`internal/config/experimental.go`; what it holds back is listed in
[Configuration › Experimental features](../guide/configuration.md#experimental-features).

### 3. Leaving experimental

A feature leaves `[experimental]` when it has: a user guide page, no open
high-severity issue, the core working the same with it on or off, and a
team that used it for a release. Leaving is one commit that drops the
feature's name from the switch's list; the `[experimental]` entry for it
then does nothing, and the config check warns of the unknown name.

## Alternatives considered

Scored on: the product says what it is for (35), nothing breaks for the
teams using the features (30), cost (20), reversibility (15).

| Option | Score | Why not |
| --- | --- | --- |
| **One switch in the project file, a clone may override (this decision)** | 85 | |
| Remove the features until ready | 40 | Breaks the teams already using the cycle; the code rots on a branch |
| Build flags (separate binaries) | 45 | Two binaries to ship and test; a team can't try a feature without a rebuild |
| A switch per clone only (git config) | 60 | Each teammate turns it on alone: a board half the team sees with the cycle and half without |
| Leave them visible, marked "beta" | 50 | The core is still hidden behind them, and a broken `[cycle]` still breaks every command |

## Consequences

Good:

- A new user meets issues, views, sync and the CLI: what git-board is for.
- A team using the cycle adds one line and loses nothing.
- A broken experimental table no longer stops `ls` or a sync.

Bad, and accepted:

- Two paths to test: experimental tests turn the switch on in their
  fixtures.
- A board that used the cycle before 2026-11-04 shows the classic issue
  page until its file names the feature.

## Left open

- Releases and deploys (`[release]`) stay on, and belong in the core:
  decided 2026-11-04 (Pierre). A release is what the issues ship in; which
  issue went out in which version, and where it is deployed, is part of
  owning them. How the core is split so this holds is
  [ADR 0020](0020-the-core-in-layers.md), proposed.
- The GitHub Project bridge (`git board project`, `internal/ghbridge`)
  depends on `gh` and on GitHub's Project model: a forge-specific feature
  in the core. It stays, as GitHub Projects are where many teams come from.
