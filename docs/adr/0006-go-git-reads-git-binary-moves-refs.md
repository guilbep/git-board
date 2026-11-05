# ADR 0006: go-git reads objects; the `git` binary moves refs and talks to remotes

- Status: accepted (recorded 2026-11-04, decided earlier)
- Date: 2026-10-24 (`docs/prd/core.md`: "Fetch and push go through the
  `git` binary"; `docs/prd/core-m1-plan.md`: go-git v5 plus the `git`
  binary).
- Sources: `docs/spec/storage.md` (intro, Updating a ref, Fetch and push);
  `docs/spec/decisions.md` › Trial and M2 results, "What was harder than
  planned".
- Alternatives below are reconstructed, except where the docs record why.

## Context

The git datastore (`core/gitstore`) writes many small objects (blobs,
trees, commits) and reads them back on every load. It also moves refs
under contention and fetches and pushes to whatever remotes a developer
has: SSH, HTTPS, credential helpers, `insteadOf` rewrites, push URLs. The
binary must stay a single static Go build (ADR 0012), so cgo is out.

## Decision

**Objects are read and written in process with go-git v5. Every ref
update, fetch, push and config read goes through the `git` binary, run in
the C locale.**

- `core/gitstore/store.go:67` `Open`: go-git with
  `EnableDotGitCommonDir`, so a linked worktree sees its main repository's
  refs and objects.
- `core/gitstore/store.go:169` `WriteCommit` and the blob and tree writers
  use go-git's storer.
- `core/gitstore/store.go:292` `UpdateRef` runs `git update-ref <ref>
  <new> <old>`: the compare-and-swap of ADR 0004. git's refusals are
  matched by their English text (`:305` `refused`), so every git runs
  with `LC_ALL=C` (`:333-337`).
- `core/gitstore/remote.go:25` `Fetch` and `:45` `Push` run `git fetch`
  and `git push` with one refspec per entity kind, so every remote, URL
  and credential helper git knows works.
- A cancelled git gets SIGINT first, so it removes its `.lock` files
  (`core/gitstore/store.go:338-343`; Windows has no SIGINT: killed).
- The code repository side does the same split: `internal/store/code.go`
  reads branches with go-git; `internal/store/diff.go` and
  `internal/ship/git.go` shell out to `git`. ADR 0001 § 8 shells out
  for notes, "go-git has no notes API, and its ref locks don't exclude
  git's".

Workarounds this choice brought, all in `gitstore`:

- go-git caches its list of pack files: after a fetch, `Reindex`
  (`core/gitstore/remote.go:38-40`), and once on a missing object.
- go-git's index isn't safe across goroutines: one mutex per `Store`
  (`core/gitstore/store.go:40-41`, a `ponytail:` note).
- go-git lists a `<ref>.lock` as a ref: gitstore reads loose and packed
  refs itself (`docs/spec/decisions.md` › M2 results).

## Alternatives considered

Reconstructed. Scored on: works with every remote a developer has (30),
one static binary (25), speed of many small reads (25), correctness under
contention with other git processes (20).

| Option | Score | Why not |
| --- | --- | --- |
| **go-git for objects, `git` for refs and remotes (this decision)** | 80 | |
| go-git for everything | 50 | Its transports don't honour every credential helper and `insteadOf`; its ref locks don't exclude git's; no notes API |
| `git` binary for everything | 60 | A process per object read; loading the board would spawn thousands |
| libgit2 (git2go) | 35 | cgo, so no single static cross-built binary; GPL-2.0 with a linking exception (`docs/prd/core-other-languages.md:95`) |

## Consequences

Good:

- Fetch and push behave as the developer's own `git` does.
- Reads are fast and in process.
- A ref update is atomic against any other git process.

Bad, and accepted:

- `git` must be on `PATH`, even for a binary that is otherwise static.
- **SHA-1 repositories only**: go-git v5 has no other object format
  (`docs/spec/storage.md`, intro).
- Two git implementations can disagree; each disagreement found so far
  cost a workaround (above).
- Error handling depends on git's English messages.

## Left open

- SHA-256 repositories wait for go-git, or for moving reads to the binary.
- The single mutex per `Store` is marked to revisit if it shows in a
  profile.
