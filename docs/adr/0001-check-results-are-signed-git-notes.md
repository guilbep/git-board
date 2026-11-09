# ADR 0001: Proof in git notes, and the opt-in cycle

- Status: accepted; merged 2026-11-09 from ADR 0001 (decided 2026-10-22)
  and the cycle part of ADR 0018 (decided 2026-11-04)
- Date: 2026-10-22, 2026-11-04
- Absorbs [ADR 0018](0018-own-your-issues-work-in-progress-is-opt-in.md),
  sections 2 and 3 (work in progress is opt-in). Its ownership part is in
  [ADR 0003](0003-own-core-instead-of-git-bug.md).
- Spec: [Proof notes](../spec/proof.md) gives the line, the key, the
  signature, the refs, the evidence and the prior art. Guide:
  [Configuration › Tests and checks](../guide/configuration.md#tests-and-checks).

## Context

The proof that a change works usually lives at the forge. CI runs the
checks again on a shared runner, after a push, and the pull request shows
the result. That proof is slow, metered, online only, lost when the
project moves forge, and kept away from the issue and the branch that it
proves. Several agents work in parallel worktrees on one machine, and
most of what remote CI runs already ran there.

The goals are local first, speed, proof kept in git, and async first. A
reviewer pulls the repository, reads the status, and trusts the check.
Every commit carries its own proof.

The development cycle, the agents and the worktrees came after the core.
They change fast and need tools that many users do not have.

## Decision

### 1. A result is one signed line in a git note

The line names the check, the result (`PASSED` or `FAILED`), the content
key, the runner's platform and tools, the commit, the time, the log's
hash and the signature. [Proof notes](../spec/proof.md#the-line) gives
its format.

### 2. Notes live in one ref per writer

- Each clone writes only `refs/notes/ci/<writer>`, and its worktrees
  share it. `git config git-board.writer` names the writer.
- A clone pushes its own ref, which only fast-forwards, so no merge step
  is needed.
- A reader takes the union of every writer's ref, its own and those under
  `refs/notes/remotes/<remote>/ci/*`.

### 3. The note's address is the content key

The key is a git hash over the check's name, its command, its variables,
and every tracked file that its `files` globs match. Anyone can compute
it again from the commit and the base branch's `.git-board.toml`.

- A rebase or a merge of main that leaves those files alone keeps the
  key, and so keeps the proof.
- Two agents with the same content share one result.
- A check's `files` lists all that the check reads, not only its sources.
  For the Go tests, that includes every file that a `//go:embed` pulls in.

### 4. A proof run happens in a clean checkout

- git-board runs a check for the record in a fresh
  `git worktree add --detach <commit>`.
- Only the command from the base branch's file runs, as an argv.
- The environment holds only the variables that the check declares, and
  the tool caches.
- A run in the live worktree is feedback only, and is never recorded.

### 5. Each line is signed with SSH

Each line is signed with `ssh-keygen -Y sign -n git-board-ci`. The signer
is whoever `ssh-keygen -Y find-principals` names. The signers file
(`[cycle] signers`) and the checks are read from the base branch, never
from the branch under review. Otherwise a branch can add its own key, or
narrow a check's `files`.

### 6. When a commit is proven

A check is proven for a commit when all of these hold:

1. Its key is computed from the commit, with the base branch's file.
2. The note at that key is read from every writer's ref. A line whose
   signature fails is ignored.
3. In each writer's ref, the last verified line is that writer's verdict.
   A run again after a flaky failure clears it.
4. At least one writer's verdict is PASSED, and none is FAILED.

Every `[cycle.checks.*]` is required, and every `[cycle.tests.*]` with
`where = "worktree"`, except for an issue whose kind has the rule
`none`. A suite with `where = "ci"` stays the forge's to prove.

A commit is proven when every required check is proven, and every path
that it changes is in some required check's `files`. Otherwise it says
which path no check covers.

### 7. What a signature proves

Now, a valid signature means that this account claimed to run the check.
The agents run under the developer's account, so an agent can sign a
pass without a run. By convention, only git-board's runner writes lines.

Later, a runner under its own user holds its own key, and the agents can
only ask it for a run. Each line names its signer, so both kinds can live
side by side.

### 8. git-board writes the notes, and anyone can verify their signatures

- `domain/releases/proof` holds the record, the key, the signature, the
  run and the gate. It reads no `.git-board.toml`: `app/verify.go`
  (`AtBase`) reads the base branch's checks and signers file for it.
- git-board runs git for every write, fetch and push of these refs.
- Every write is a compare-and-swap through a private ref and
  `git update-ref`.
- `notes.rewriteRef` is never set, so a rebase never copies a proof onto
  content that it did not prove.
- Verifying needs only git and `ssh-keygen`.
  `git board verify [commit]` runs the gate in any clone.

### 9. How the cycle reads it

Verdicts are per key, so a FAILED blocks only the commits whose content
gives that key.

- A bug fix reads two proofs: the suite FAILED at the commit that adds
  the test, and PASSED at the fix.
- A feature's touched suites are proven at the branch's last commit.
- The `[cycle.checks.*]` proven at the branch's last commit gate Mark
  ready for review.
- The land uses the full rule of section 6 at the merged head.

### 10. The cycle is opt-in

The cycle is experimental, and off by default. One switch turns it on:
`[experimental] features = ["cycle"]` in `.git-board.toml` for the team,
or `git config board.experimental cycle` in one clone, which replaces the
file's list there (`app/config.go`).

- Off, the cycle is absent, not broken. Its commands are hidden and
  refuse with a sentence that names the switch. Its API routes answer
  404 `{"code": "experimental"}`. The web app hides its pages and
  controls.
- Off, a broken `[cycle]` table is a warning, never an error that stops
  every command. On, it is refused.
- `domain/cycle` computes whenever someone asks. The application decides
  whether anyone asks.
- A feature leaves `[experimental]` when it has a guide page, no open
  high-severity issue, the core working the same with it on or off, and
  a team that used it for a release.

## Consequences

Good:

- The proof travels with `git fetch`, and a reviewer can trust it offline.
- A rebase or a merge of main keeps every proof whose files it did not
  touch, so only what changed runs again.
- Parallel agents share results.
- No new data model: git's own notes, OpenSSH's own signatures.
- A new user meets the issues first. A team on the cycle adds one line.

Bad, and accepted:

- A reviewer configures one refspec and runs `git board verify`.
  `git notes show` displays a line, but does not verify its signature.
- No forge shows or protects `refs/notes/*`. A signature stops a forged
  line, not a deleted or rewound ref. A missing proof reads "not proven".
- Until a guarded runner exists, a signature proves a claim, not a run.
- `git board sync` does not push or fetch these refs yet.
- Two paths to test: the cycle's tests turn the switch on in their
  fixtures.
