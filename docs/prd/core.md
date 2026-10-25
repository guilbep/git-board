# The core: git-board's own data model and sync

PRD, 2026-10-24. Decided with guilbep, at the end of the session that rebased
the git-bug fork onto upstream and released v0.11.1-sync.10. The drawings
behind it are in the "The fork, drawn" artifact
(https://claude.ai/artifact/M6LeKg3bRWvp1H6wkEGP6n).

## The problem

git-board stands on a fork of git-bug, and the fork has become the problem:

- **Most of it isn't used.** git-board uses about 30,000 of git-bug's lines:
  the data model (~17,000) and the bridges (~13,000). It also carries
  git-bug's command line, GraphQL API, terminal UI, web UI, Jira and
  Launchpad, none of which it uses.
- **Its format was bent to sync.** The value git-board and the forge last
  agreed on lives in `sync:<remote>:*` metadata strings, read in a way git-bug's spec doesn't
  define, and git-bug's own bridges misread it. An original git-bug running
  `bridge pull` or `bridge push` on a synced repository duplicates issues,
  on GitHub too.
- **Upstream keeps moving.** Its author announced breaking changes "for some
  time" (git-bug#1628, 2026-10-22). Each rebase costs a session.
- **It is GPL-3.0.** git-board can't be MIT while it links git-bug.
- **It's hard to hold in one's head.** Part of that complexity is the
  problem's (two-way sync between forges), and part is ours (Gitea, review
  sync, three mode switches, our own metadata format).

## The decision

**Rewrite the data model and the sync, in Go, as a library inside
git-board. Keep git-board's own code and move it onto the library. Drop
git-bug.**

- **Go**, for this project: 2.7 s to rebuild after an edit, 9.2 s from
  scratch (measured on git-board, M4 Pro); one static binary for five
  platforms from one machine; git-board's 27,000 lines and 22,000 lines of
  tests stay. Rust came second, C and C++ far behind: the scored table is in
  the artifact. Plans for those languages, for later, are in
  [core-other-languages.md](core-other-languages.md).
- **Sync first.** Links to forges and "the value git-board and the forge last agreed
  on" are part of the data model, not metadata strings.
- **Not compatible with git-bug**, on purpose. The data lives under refs of
  its own, so git-bug never sees it and can't duplicate it.
- **One binary:** `git board`, for people and for CI.
- **MIT.** The code is guilbep's (his IP). git-board moves to MIT once
  nothing in it links GPL code (M6).
- **A schema, documented:** the spec is a Zensical site in `spec/`, and the
  code implements it.

## Rules

- **Clean room.** The spec is written in our own words, from our decisions
  and from observed behaviour. Sessions implementing the core read the spec,
  this PRD and the milestone plans, never git-bug's source, the fork's code,
  or git-bug/spec (which has no licence). `core/` must not import
  `github.com/git-bug/git-bug`: a test fails if it does.
- **The library is a library.** `core/...` is importable
  (`github.com/guilbep/git-board/core/...`) and never imports git-board's
  `internal/`.
- **Version 1 is what the problem forces, for GitHub and GitLab.**
  Everything else waits until someone asks.
- **The fork stays frozen at v0.11.1-sync.10** and keeps running until the
  new core passes the conformance suite and a live run on the scratch
  forges.

## The data model, in brief

The spec (M1) holds the details; these are the choices it starts from.

- **One kind of record: an entity**, a log of operations kept in a chain of
  git commits under `refs/board/<kind>/<id>`. Each commit holds one pack of
  operations and a clock one higher than its parents'. All of an entity's
  operations are ordered by (clock, pack hash): the same order on every
  clone, whatever the wall clocks say.
- **Four kinds** (the fourth from M5's inventory):
  - `issue`: created, commented, text edited, title, status, labels;
  - `person`: a name, and an account per forge server (numeric id and
    login);
  - `sync`, one per forge project: which issue is which forge issue, the
    value git-board and the forge last agreed on for each field, and what was
    sent;
  - `release`, one per released commit: what it shipped, its notes,
    comments and yank. The core stores and moves it; git-board reads it.
- **An operation pulled from a forge says where it came from** (forge
  project and event id), as a field.
- **Every field is a label** (`ns:value`), assignees included
  (`assignee:<name>`), as git-board has them today. The forges get labels,
  never their own fields (guilbep, 2026-10-24): nobody uses GitHub's issue
  types or priority, and the PMs' fields live in GitHub Projects, which
  git-board's Project sync handles.
- **One exception: assignees.** `assignee:<name>` labels become the forge's
  own assignees, always, with no switch: everyone uses them, and a forge
  notifies only its own assignees. GitLab's free edition takes one per
  issue; the others stay in git-board, with one warning.
- **Acceptance criteria stay text:** a task list under a heading in the
  description, which git-board reads today, and which every forge renders
  with its checkboxes. The core doesn't model them; git-board does.
- **Settings shared by the project live in `.git-board.toml`**, which
  git-board already has and edits in Settings: the forges, the posting
  account, `publish-private`. Who "I" am is local.
- **Tokens stay on each machine:** an environment variable when set (CI),
  else git's own credential helper for the forge's host, which already
  holds tokens on most machines (`git credential fill`, with the account as
  `username`). Settings stores a new one with `git credential approve`. No
  keyring library.
- **Writes check the ref they move.** No lock files: two processes writing
  at once both land, the second after a retry. Only a sync run takes a
  lock, so that one machine doesn't post the same thing twice.
- **Queries and search read a local sqlite index** (pure Go), rebuilt from
  the refs whenever it's missing or stale. It's never the source of truth.
- **Fetch and push go through the `git` binary**, with every remote, push
  URL and credential helper git knows.

## Version 1

**In:**
- issues (create, comment, edit a text, title, status, labels) and people;
- sync with GitHub and GitLab, both ways:
  - posting for people without a token there, under a header naming them;
  - mentions defused, and `#n` becoming the mirrored issue's number;
  - assignees as the forge's own;
  - a forge's refusal kept in git-board, with one warning;
- what sync.10 added: confidential GitLab issues never synced, nothing
  pulled from a project that isn't public sent to one that is (unless
  `publish-private`), and GitHub's state read from the issue itself;
- git-board on the core: server, web app, TUI, command line;
- the switch, repository by repository, from the forges (no converter).

**Out, until asked:**
- Gitea;
- mirroring git-board's review comments to GitHub pull requests (they stay
  in git-board);
- the forges' own fields besides assignees: GitHub's issue types and
  priority, GitLab's milestones, and the mode switches. They stay labels on
  every forge;
- `link`, `unlink` and `take`;
- Jira and Launchpad;
- any API besides git-board's own HTTP API.

## Milestones

| | Delivers | Proven by | Agent hours |
|---|---|---|---|
| **M1** | the spec (Zensical, `spec/`), and a trial core: git store, operation log, issues, a pull from a fake forge | the trial's tests; `zensical build --strict`; a size and concepts report. guilbep decides whether to go on | 15–25 |
| **M2** | the full core: people, every issue operation, the sqlite index and search, change events for the server and the TUI | tests for two processes, two clones and unknown operations; the import guard | 20–35 |
| **M3** | the sync engine, on a fake forge, with the conformance suite every adapter runs | the suite: mirror then stable, an issue opened on each side, a local edit survives, a forge edit is pulled, a conflict keeps the later edit, a crash after a write, labels merge, pushing alone keeps labels, a token owner acts directly, refs never mirrored, deletions not propagated, one issue synced alone, plus the guard and confidential issues | 15–30 |
| **M4** | the GitHub and GitLab adapters | the suite on fakes; one live two-way run on guilbep/git-board-test and gitlab.epfl.ch 42400 | 15–30 |
| **M5** | git-board on the core: `internal/gitbugio`'s Backend over the core, the server's forge sync over `core/sync`, the command line and the TUI; git-bug leaves go.mod; features carried over by use, not by inventory | `make ci`; git-board's tests as the checklist of behaviours to keep or drop | 15–30 |
| **M6** | the switch, without a converter: a last sync with sync.10 so that everything is on a forge, then the core pulls each repository from its forges. An issue posted by the fork is recognized as a mirror by its header's link to the original, so a mirrored pair becomes one issue. Project numbers are keyed again. Then MIT | a dry run on copies of the wiki and git-board's own board; then the real ones, with no duplicate on any forge and every issue kept; a licence scan of git-board's dependencies | 5–10 |

Each milestone gets its own plan, `core-mN-plan.md`, written when the one
before it is done and its part of the spec is approved.
[core-m1-plan.md](core-m1-plan.md) is the first.

M1 result, 2026-10-24: `gitstore`, `oplog` and `issue` are 895 lines of code
(target under 1,500); the pull is 208 (346 with its state; target under 300).
See the spec's Trial results. Awaiting guilbep's decision on M2.

M1 gate reached, 2026-10-24: the trial and its review are done, waiting for
guilbep's go or no-go on M2.

M2 result, 2026-10-24: the core is 2,466 lines of code and 3,273 of tests.
People, the sqlite index and search, and change events add 824 of them;
`gitstore`, `oplog` and `issue` are 1,113. The proof holds: two processes
(`TestIndexTwoProcesses`), two clones (`TestMergeConverges`), unknown
operations (`TestUnknownOpIsAnError`, `TestPersonUnknownOp`) and the import
guard pass. See the spec's M2 results.

M3 result, 2026-10-24: the sync engine is 1,684 lines of code (`syncer`),
on a forge interface of 86 (`forge`) and a fake of 449; the core is now
4,179 lines of code and 4,284 of tests. The conformance suite,
`core/forge/conformance`, runs the 14 scenarios of the M3 row, plus a
refusal warned once and assignees, and the fake passes all 16. "Refs never
mirrored" is read as references: no `#n` reaches another project as it is.
See the spec's M3 results.

## Carried to later milestones

What M1's tasks and reviews left for later. The execution ledger isn't kept
in git, so they are listed here.

- **Not planned yet:** the merge storm when clones exchange directly
  (storage.md's known point).
- ~~**M3:** the lock-file branch of the sync entity's first write has no
  test.~~ Done: `TestStateFirstWriteLocked`.
- ~~**M3:** a record of which git-board comment each posted comment is, a
  refusal record, the marker's format, and a last-run time; an issue pulled
  but not linked (a crash between the two writes) found by its origin and
  linked; tests of first link wins before M3 relies on it; the two conflict
  rules users will meet, stated in the spec: between clones by the order,
  between git-board and a forge by time.~~ Done: sync.md's Records, Markers,
  Conflicts and Crash recovery; `TestPullFindsUnlinkedByOrigin`,
  `TestStateFirstWins`.
- ~~**M3:** after two clones pull the same forge comment, the issue keeps the
  first copy's text (issue order), while `seen` takes the last value in the
  sync entity's order. They can disagree and hide a forge edit. Settle it
  with M3's conflict rules.~~ Done: what git-board took from a forge counts
  as last seen there (`TestTwoPullsOfOneComment`).
- **M6:** the sync entity is one log that grows forever, reloaded whole on
  every run. Plan a snapshot, or the index, before M6's mass first pull
  (a `ponytail:` note in `syncer/state.go`). M3 records a run only when it
  wrote, so a quiet repository's entity doesn't grow.
- **M4:** the adapters apply their own listing margin (GitLab's 2 minutes),
  pace their writes and retry rate limits; the engine has none of that.
- **M4:** a forge that rewrites line endings (`\r\n` from a web edit) breaks
  the blank line before the marker and after the header, so a posted text
  would be pulled back with them. Normalize in the adapter.
- **M4:** pulled operations carry the pull's time, not the forge's change
  time, so the later change between two forges is approximate. Use the
  forge's times when the adapters read them.
- **M5:** a run killed before its end leaves `board/sync.lock`, which must
  be removed by hand, like git's own locks.
- **M4:** a forge listing one comment id twice in one issue (paging).
- ~~**M5:** check git-board's own handling of `assignee:` in any case;
  validate kinds, ids and remote names before the HTTP API reaches them.~~
  Done in M5's first phase: `gitbugio` reads `Assignee:` as `assignee:`
  (`TestAssigneeInAnyCase`), and checks id prefixes, comment ids and remote
  names before the core (`TestInputsAreChecked`). No HTTP input names a
  kind.
- **M5, phase 2:** the push leaves out comments with `review:*` metadata;
  `Event.Pending`, `EditPending` and `EditWas` from the sync's records;
  the forges, tokens and the GitHub repository for numbers and pull
  requests from `.git-board.toml` and the credential helper, where the
  bridges' stubs answer "not wired yet". See
  [core-m5-inventory.md](core-m5-inventory.md).

## Success

- The core, the sync and the two adapters, tests aside, come to well under
  half of today's ~30,000 lines, with fewer concepts to learn.
- Nothing in git-board imports git-bug, and nothing in its dependencies is
  GPL.
- The wiki, co2 and git-board's own board run on the core, and their forges
  have no duplicate.

## Risks

- **The second-system effect.** Version 1 is the list above, nothing more.
- **Merging edits made in several clones** is the hardest part. The order
  rule is short, but its tests must be thorough: M1 and M2 carry them.
- **Forge quirks come back if forgotten.** The spec's forges page carries
  each one over from the fork's sync-decisions.md, which guilbep wrote.
- **Two systems for a while.** Until M6, the fork's sync.10 and today's
  git-board stay in use; the new core touches only scratch repositories.
- **What lives only in git-bug is lost at the switch:**
  - issues never sent to a forge;
  - local labels never pushed, like co2's from git-board;
  - comments the fork couldn't post.

  Before switching a repository, its last sync.10 run must report nothing
  left to send.

## Answered by guilbep, 2026-10-24

- No forge fields, labels everywhere, except assignees, which are the
  forge's own.
- No converter: the switch pulls from the forges (M6). One sync panel in
  git-board, no separate git-bug sync.
- Tokens: environment variables, else git's credential helper.
