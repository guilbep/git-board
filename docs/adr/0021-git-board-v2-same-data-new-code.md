# ADR 0021: git-board v2: new code, the same data

- Status: accepted (2026-11-05, Pierre)
- Date: 2026-11-05
- Builds on the design, [docs/rewrite/design.md](../rewrite/design.md),
  and [ADR 0020](0020-the-core-in-layers.md) (the layers).

## Context

On 2026-11-04 Pierre asked for a rewrite from scratch, "with elegance,
modularity and speed in mind". The design was written that night, and
milestones M0 to M4 were built in `next/` by the morning: the kernel,
the git, PostgreSQL and Markdown datastores, the view, writes, the
issue commands, forges and sync. 225 recorded cases match v1's outputs;
v1 and v2 write, sync and push one board in turn and agree.

The design left eight questions open (its section 6). Pierre answered
them on 2026-11-05.

## Decision

**v2 replaces v1. It keeps the data, byte for byte, and changes
everything else where that makes it better.**

| Question | Answer |
|---|---|
| Freeze v1? | **Yes.** v1 takes fixes only, from now. |
| The HTTP API | **Reshaped, with the web UI.** It is a v2: paged lists, one error shape, fewer routes. The CLI's commands, flags, `--json` fields and exit codes stay. |
| What v2 drops | **`html`.** Agents and `wtx` move to their own binary (below). The GitHub Project bridge stays (below). |
| The version | **v0.12.** The data is the same. |
| When | **As soon as v2 is done**, and v1 is deleted then. |
| GitLab replies | **Fixed.** v1 posts a reply as a top-level comment, because the wrapper that records a forge's writes hides GitLab's `Reply`. v2 posts it as a reply. |

**Agents and `wtx` get their own binary.** They are a proof of concept
worth keeping, and they keep no data: they drive tools. They talk to a
board through git-board's CLI and HTTP API only, so they can live in
another repository and release on their own.

**The GitHub Project bridge stays in v2.** Some project managers keep
their priorities in GitHub Projects, and the field settings page says
where each field lives. Those PMs are a segment git-board is for. The
bridge becomes a sync target like the forges, a second mapper on the
sync engine, after the release mapper.

**Taken as the design proposed, unless Pierre says otherwise:**

- `next/` stays in this module, and becomes the root at the switch.
- v2 reads objects through `git cat-file` processes, not go-git. This
  amends ADR 0006 for reads: the first build is about 5 times faster.
- Writes are allowed during a sync. Compare-and-swap keeps them safe, so
  the web app no longer sees v1's 409 `syncing`.

**Two additions, asked the same morning:**

- **`.git-board.toml` has a schema version**, with documented upgrades
  and downgrades between versions, as Alembic has for a database.
- **The server logs.** `git board` (the server and its web UI) writes
  error, warning and info lines, with a level the user chooses.

**Amended 2026-11-05:** v2 does not port `switch` and `prune` (git-bug
repositories). git-board v0.11 migrates them: a repository that still holds
git-bug data is refused, with a message that names v0.11's `git board switch`.

## Consequences

- v1 is deleted in the pull request that ships v0.12. Until then a v1
  fix is also made in `next/`, or proven not needed there by `make
  compat`.
- The web app is rewritten against the new API. Its browser tests are
  the acceptance.
- A board kept by v0.11 opens in v0.12 unchanged. A `.git-board.toml`
  without a version is version 1, and is upgraded when written.

## Left open

- The new repository's name for agents and `wtx`. Proposed, and built in
  `contrib/board-agents/` until that repository exists:
  `github.com/guilbep/board-agents`, its binary `git-board-agents` (`git
  board-agents`). See the design's
  [Agents and wtx](../rewrite/design.md#agents-and-wtx-their-own-binary).
- Whether the bridge is in v0.12 or v0.13: it is planned for v0.12, and
  ships in v0.13 if it would hold the release back.
