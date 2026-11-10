# How v2 came to be

This page keeps the history that ADR 0020 and ADR 0021 carried before
they merged into
[ADR 0006: One binary, built in layers](../adr/0006-go-git-reads-git-binary-moves-refs.md).
The decisions are in ADR 0006. The paths in the first part are v0.11's:
`core/` and `internal/` are gone since v0.12.

## The core in layers, in v0.11 (ADR 0020)

On 2026-11-04 Pierre decided that releases and deploys belong in the
core, and asked for a kernel. `core/` then held the operation log, the
datastores, people, the issue kind, the index, the forges and the sync.
The release's fold and rules lived in `internal/store/releases.go` and
`internal/release`, next to the web server. The cycle lived in
`internal/cycle`, `internal/work`, `internal/proof` and `internal/wtx`.

The split named a kernel (`core/oplog`, the datastores, `core/people`,
`core/watch`, `core/forge`) and three domains on it: issues, releases and
the cycle. `core/layers_test.go` gave every core package a layer. The
four steps were done on 2026-11-04.

### What the three-way split missed

1. The kernel. Without a named kernel, each domain grows its own copy of
   the log, or the issue domain becomes the kernel by accident.
2. People. Who wrote what is shared by all three domains.
3. Sync with forges. Releases went to GitHub on their own path
   (`internal/ship/binaries.go`).
4. Proof. Deploys and the cycle both read the signed check results.
5. Configuration. Each domain owns and reads its own table, so a broken
   `[cycle]` cannot break issues.
6. The read model. The index and the inbox belong with the domain they
   answer for.

### Step 2: the issue domain's satellites

- `core/model` took `Issue`, `Thread`, `Event`, `Person` and the other
  read types from `internal/store`, with names and JSON tags unchanged.
- `core/fields` took the schema from `internal/config` and the
  assignments from `internal/fields`.
- `core/filter` and `core/inbox` read a `model.Issue` and a schema.
- Fields are the issue domain's, not the kernel's: a field is a label,
  and a release has none.

### Step 3: the release domain

- `core/release` took the record and its rules, versions and tags, the
  GitOps overlay and the `[release]` table.
- `core/proof` took the proof from `internal/proof`.
  `config.AtBase` read the base branch's file into a `proof.Base`.
- `internal/ship` kept the `Runner`, which cuts, promotes, deploys, rolls
  back and adopts. It stayed outside the core, because it keeps no data
  and drives tools: docker, crane, trivy, helm and gh.
- A deploy is a commit to an app's configuration repository, not an
  operation of the board. The configuration repository is the deploy
  log.

### Step 4: the cycle

- `core/cycle` took the computation and the `[cycle]` table.
- `Gather` fetches nothing. It computes the evidence from the thread and
  three seams: `Repo` (five reads of git), `Pulls` (a branch's PR) and
  `Render` (the plan's HTML).
- `core/cycle/work` took the Work page's rows.
- `core/forge.PR` took the pull request's shape from `internal/ghpr`.
- The switch stayed the application's. `core/cycle` computes when it is
  asked, and the application decides whether anyone asks.
- `internal/branches` and `internal/markdown` stayed in the application.

### What was left in internal/

| Package | Why it was the application's |
|---|---|
| `store` | the wiring: opens a board, writes as you, pushes and pulls with git |
| `config` | reads `.git-board.toml` and checks each table into the core's types |
| `server`, `tui`, `board`, `cmd/git-board` | the surfaces |
| `evidence`, `branches`, `ghpr` | the cycle's outside world: git's branches, GitHub's pull requests |
| `ship` | the release runner |
| `wtx`, `ai` | agents, their worktrees, a chat client |
| `ghbridge`, `ghlabels`, `fromforge`, `migrate`, `lint` | tools over a board |
| `uploads`, `avatar`, `markdown`, `insights` | attachments, pictures, HTML, a report |
| `hermetic` | the tests' seal |

### Open when v0.11 froze

- Releases reached GitHub alone. v0.12 sends them through the sync
  engine (`domain/releases/releasesync`).
- `forge.PR` speaks GitHub's words. A GitLab merge request maps to it
  when the cycle runs on GitLab.
- The `[release]` and `[cycle]` checks were still in `internal/config`.
  v0.12 gives each domain its own table.

## v2: new code, the same data (ADR 0021)

On 2026-11-04 Pierre asked for a rewrite from scratch, "with elegance,
modularity and speed in mind". The [design](design.md) was written that
night. Milestones M0 to M4 were built in `next/` by the morning: the
kernel, the git, PostgreSQL and Markdown datastores, the view, writes,
the issue commands, forges and sync. 225 recorded cases matched v1's
outputs.

The design left eight questions open. Pierre answered them on
2026-11-05:

| Question | Answer |
|---|---|
| Freeze v1? | Yes. v1 took fixes only from then. |
| The HTTP API | Reshaped, with the web UI: paged lists, one error shape, fewer routes. The CLI's commands, flags, `--json` fields and exit codes stay. |
| What v2 drops | `html`. Agents and `wtx` move to their own binary. The GitHub Project bridge stays. |
| The version | v0.12. The data is the same. |
| When | As soon as v2 is done, and v1 is deleted then. |
| GitLab replies | Fixed. v1 posted a reply as a top-level comment. v2 posts it as a reply. |

Taken as the design proposed:

- `next/` stays in this module, and becomes the root at the switch.
- v2 reads objects through `git cat-file` processes, not go-git. The
  first build is about five times faster.
- Writes are allowed during a sync. The web app no longer sees v1's 409
  `syncing`.

Two additions, asked the same morning: the schema version of
`.git-board.toml`, and the server's logs at a level the user chooses.

On 2026-11-05 Pierre dropped v1 entirely, with no v0.11.7. v2 shipped as
v0.12.0. `next/` became the root, and `make compat` holds v2 to v1's
recorded outputs ([compat](compat.md)). v2 does not port `switch` and
`prune`: git-board v0.11 migrates a git-bug repository.

### Open items

- The agents' repository name. Proposed: `github.com/guilbep/board-agents`,
  its binary `git-board-agents`. It lives in `contrib/board-agents/` until
  that repository exists ([Agents and wtx](design.md#agents-and-wtx-their-own-binary)).
  What its client still lacks from v2's API (a comment's `cycle:step`,
  line comments located on the branch) is in its README.
- Whether the GitHub Project bridge ships in v0.12 or v0.13.
