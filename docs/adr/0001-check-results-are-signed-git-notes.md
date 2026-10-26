# ADR 0001: Check results are signed lines in git notes, keyed by content

- Status: accepted
- Date: 2026-10-22
- Decided with the user. The research behind it is summed up under
  Evidence.
- Builds on `docs/prd/cycle-protocol.md`: its `[cycle.tests.*]` and
  `[cycle.checks.*]` are the only declaration of what runs. Its M4 (step 4's
  runner) and M6 (C7's run at the merged head) build on this record.
- Amends that PRD's step 4: a run that counts happens in a clean checkout,
  not in the worktree (section 9).

## Context

The proof that a change works lives at the forge today: GitHub Actions runs
the checks, and a pull request shows their result. git-board reads it from
there (`internal/ghpr`: check runs, commit statuses, and the checks a
ruleset requires). That proof is:

- slow: push, then wait minutes for a shared runner to do again what the
  developer's machine already did;
- metered: Actions minutes;
- online only, and lost when the project moves forge;
- kept outside the issue and the branch it proves.

Several agents now work in parallel worktrees on one machine. Remote CI
becomes the bottleneck, and most of what it runs has already run locally.

The goals, in the user's words: **local first, speed, proof kept in git,
async first.** A reviewer pulls the repository, checks the status, and
trusts the check. Online first is not a goal.

In one line: **every commit carries its own proof.**

## Decision

### 1. A result is one signed line in a git note

```
v1 go-tests PASSED key=3f1c…e9 platform=darwin/arm64 tools=sha256:5d2e… commit=9440305… at=2026-10-22T14:03:11Z dur=48s log=sha256:7a0b… sig=U1NIU0lH…
```

| Field | Meaning |
| --- | --- |
| `v1` | the format's version |
| `go-tests` | the check: a `[cycle.tests.*]` or `[cycle.checks.*]` name |
| `PASSED` / `FAILED` | the result, in the in-toto test-result vocabulary |
| `key=` | the content key (section 3), the note's address |
| `platform=` | the runner's OS and architecture |
| `tools=` | the sha256 of the output of the check's version commands, in order |
| `commit=` | the commit the run checked out. For reading only: the key is what counts |
| `at=`, `dur=` | when it ran and how long it took, by the runner's clock. For audit, not trusted |
| `log=` | the sha256 of the run's output. The log stays with the runner |
| `sig=` | the SSH signature, base64 on one line (section 5) |

The fields come in this order, and no value holds a space. A reader ignores
a line it can't parse and never depends on the order of lines from
different refs: the in-toto bundle rules. A standard in-toto statement can
be generated from a line later. The line isn't one now: the same fact in a
DSSE envelope is 930 bytes of base64, against 375 readable bytes.

### 2. Notes live in one ref per writer

- Each clone writes only `refs/notes/ci/<writer>`. Its worktrees share it.
  The writer's name is set once per clone and kept in its git config.
- A clone pushes its own ref, which only fast-forwards, so there is no
  merge step.
- It fetches the others' refs into `refs/notes/remotes/<remote>/ci/*`,
  never with a `+` into its own namespace, which would throw away lines not
  yet pushed.
- A reader takes the union of every writer's ref.
- Pushes go with git-board's sync, not one per run: async, and GitHub
  advises at most 6 pushes a minute.

### 3. The note's address is the content key

The key is `git hash-object --stdin` (without `-w`) over a canonical text,
so it has the repository's own hash format:

```
git-board-ci/key/v1
check go-tests
run ["go","test","./..."]
env CGO_ENABLED=0
file 100644 6e1f… go.mod
file 100644 b2c4… go.sum
file 100644 91d0… internal/cycle/cycle.go
…
```

- `file` lines are every tracked file of the commit that the check's
  `files` globs match, as mode, blob id and path, sorted by path.
- `env` lines are the variables the toml sets for the check, with their
  values.
- A check with no `files` covers the whole tree.

**The key holds only what anyone can recompute from the commit and the
toml.** A reviewer on another machine therefore finds the same notes. The
machine's side, its platform and tool versions, is in the signed line
(section 1):

- a runner reuses a PASSED line only when its platform and tools match its
  own;
- a reviewer accepts any of them, unless the toml says otherwise.

A rebase, or a merge of main, that leaves those files alone keeps the key,
and so keeps the proof. Two agents with the same content share one result.
Looking a result up is one read.

`git notes` accepts an address with no object behind it. Such a note
travels, merges and survives `gc`. `git notes prune` deletes it, so prune
is never run on these refs.

**A check's `files` lists everything it reads, not only its sources.** For
the Go tests that is:

- `**/*.go`, `go.mod` and `go.sum`;
- the testdata;
- every file a `//go:embed` pulls in: `docs/guide/configuration.md`,
  `docs/guide/forges.md`, `internal/config/default.git-board.toml`,
  `web/epfl-design/…`, `web/public/epfl-logo.svg`, and `web/dist`.

In a clean checkout `web/dist` holds only its tracked `.gitkeep`. A loose
list gives a false pass. The cycle protocol's pre-ticking of "the suites
the diff touches" reads the same field.

### 4. A proof run happens in a clean checkout

- git-board runs a check for the record in a fresh
  `git worktree add --detach <commit>`, as the release build does
  (`internal/release/deploy.go`).
- Only the command from the base branch's toml runs, as an argv.
- The environment is cleared to the variables the check declares, plus the
  tool caches (Go's, npm's), which are content-addressed by their own
  tools.
- A run in the live worktree is feedback only and is never recorded.
  Ignored and untracked files there make a pass false. `web/dist` there
  holds the last build, which the Go tests embed and which no key sees.
  `node_modules` may not match the lockfile.

A cold run pays `npm ci` and the web build. The content key makes cold runs
rare.

### 5. Each line is signed with SSH

- The payload is `git-board-ci/v1`, a newline, then the line up to
  ` sig=`.
- It is signed with `ssh-keygen -Y sign -n git-board-ci`.
- `sig=` is the signature blob's base64, unwrapped.
- The signer is whoever `ssh-keygen -Y find-principals` names. Nothing
  written in the line is trusted for that.

The signers file is in the repository, in OpenSSH's `allowed_signers`
format, with `namespaces="git-board-ci"`. **It is read from the base
branch, never from the branch under review**, and so is the toml.
Otherwise a branch could add its own key, or narrow a check's `files`.

A developer's personal `gpg.ssh.allowedSignersFile` plays no part.

### 6. When a commit is proven

A required check is proven for a commit when:

1. its key is computed from the commit, with the base branch's toml;
2. the note at that key is read from every writer's ref. A line whose
   signature doesn't verify against the base branch's signers file is
   ignored;
3. in each writer's ref, the last verified line is that writer's verdict.
   Lines in one ref are only ever appended, so their order holds, and a
   re-run after a flaky failure clears it;
4. lines in different refs have no order, so the check is proven when at
   least one writer's verdict is PASSED and none is FAILED.

Required, by default:
- every `[cycle.checks.*]`;
- every `[cycle.tests.*]` with `where = "worktree"`, except the tests of an
  issue whose kind has the rule `none` (doc, chore).

A suite with `where = "ci"` stays the forge's to prove.

The commit is proven when every required check is proven, and every path
the commit changes against its base is in some required check's `files`.
A check with no `files` covers everything, so a docs-only commit is
covered by the checks. Otherwise it says so: "not proven: Makefile is
covered by no check".

### 7. What a signature proves, now and later

The user chose to start with the developers' own keys and upgrade later.

**Now:** a valid signature means "this account claimed it ran". The
worktree agents run under the developer's account, and their commits are
already signed with the developer's key. An agent can therefore sign a pass
without running anything: the review showed one forged with only the SSH
agent's socket. Two things limit this, and only the second is a check:

- By convention, only git-board's runner writes lines; an agent asks for a
  run. Deny rules in the agents' settings can back the convention. wtx
  would write them, for `git notes --ref=…ci…`,
  `git update-ref refs/notes/ci…` and `ssh-keygen -Y sign`. An agent on the same account can still get
  around them.
- `wtx land` runs a random sample of the proven checks again, in a clean
  checkout, and appends the result. When the result disagrees with the
  verdict (PASSED claimed, FAILED now), the land stops and says so.

**Later:** a runner under its own macOS user, a launchd daemon, holds its
own key and runs the checks; agents can only ask it for a run. The signers
file then lists runner keys, and the toml says which checks only a runner
can prove. Each line names its signer, so both kinds live side by side, and
a reviewer sees which one proved what.

### 8. git-board writes the notes; anyone can verify them

- git-board shells out to git for every write, fetch and push of these
  refs, as `internal/release/git.go` does. go-git has no notes API, and its
  ref locks don't exclude git's.
- Every write is a compare-and-swap:
  1. copy the ref to a private ref;
  2. append there;
  3. run `git update-ref refs/notes/ci/<writer> <new> <old>`;
  4. on failure, start again.
- `notes.rewriteRef` is never set for these refs: a rebase must not copy a
  proof onto content it didn't prove.

Verifying needs only git and `ssh-keygen`: compute the key, read the lines,
verify, apply section 6. The procedure is documented in `docs/` as a short
script, so trusting a check never depends on trusting git-board.

### 9. How the cycle reads it

Verdicts are per key. A FAILED blocks only the commits whose content gives
that key.

- **Step 4, a bug.** The rule "a test that fails before the fix" reads two
  proofs:
  - the suite FAILED at the key of the commit that adds the test;
  - it PASSED at the fix.
  
  The earlier FAILED is evidence, never a block on the fix, whose key
  differs. If the fix changes nothing the suite reads, the key stays the
  same and the FAILED still blocks, rightly. In v1, whether the new test is
  what failed is the reviewer's judgment. Later, the line can carry
  in-toto's `failedTests`, for suites that emit a report (`go test -json`,
  a JUnit file).
- **Step 4, a feature.** The touched suites are proven at the branch's last
  commit.
- **Step 5, Mark ready for review.** The `[cycle.checks.*]` proven at the
  branch's last commit gate it, as the cycle PRD says. The tests and the
  coverage rule don't.
- **The land and step 7.** These use the full "commit proven" of section 6,
  coverage included, at the merged head. A merge that leaves a check's
  files alone carries its proof over.

**This amends the cycle PRD's step 4.**
- A run that counts happens in a clean checkout of the branch's last
  commit (section 4). The worktree's own changes must be committed first.
- Running in the worktree stays, as feedback, and is never a gate.
- The first gate on a branch pays `npm ci` and the web build, unless proofs
  of main's content already cover it.

## Alternatives considered

Scored on the goals (in git and async 30, a reviewer can trust it offline
30, speed 20, cost to build and to read 20), after the research.

| Option | Score | Why not |
| --- | --- | --- |
| **Git notes (this decision)** | 80 | |
| A git-bug entity, `refs/checks`, like `refs/releases` | 72 | One ref per record, and thousands of records. Unreadable without git-board. New entity code. |
| A signed commit trailer, `Checked: …` | 72 | Only the author can add proof, and a failed run leaves no trace. The checks would have to pass before the commit is final, slowing the agents. A rebase makes it stale. |
| Forge commit statuses (Basecamp's gh-signoff, Rails 8.1's `bin/ci`) | 46 | Not in git, online only, and nothing to verify offline. |

Also rejected:

- Notes on commits, carried over by `notes.rewriteRef`: the spike copied a
  pass onto a rebased commit whose covered file had changed.
- One shared notes ref merged with `cat_sort_uniq`: it works across clones
  (git-appraise's layout), but it needs a merge step that per-writer refs
  remove.
- DSSE envelopes: see section 1.
- A second key for the runner under the same user: an agent on that
  account reads it as easily.
- A hardware touch for each signature: a person at every run is not async.
- CI as an auditor that re-runs a sample online: online first is not a
  goal.
- act, Dagger, Earthly: they run pipelines, they don't keep proof. Earthly
  is no longer maintained. Dagger stays a candidate for e2e environments.

## Consequences

Good:

- The proof travels with `git fetch`, and a reviewer checks it offline.
- A rebase or a merge of main keeps every proof whose files it didn't
  touch, so only what changed runs again.
- Parallel agents share results.
- The cycle reads one record, whoever ran the check.
- No new data model: git's own notes, OpenSSH's own signatures.

Bad, and accepted:

- A reviewer configures one refspec and runs the verify step. Plain
  `git notes show` displays a line, but doesn't verify it.
- Forges display nothing, and no forge protects `refs/notes/*`: GitHub's
  rulesets cover branches and tags. A signature stops a forged line, not a
  deleted ref or a rewound one. A missing proof reads "not proven", so
  deleting hides passes. Rewinding a writer's own ref can hide that
  writer's failure. This is the same limit `refs/releases` has.
- A cold run is slower than a run in the live worktree.
- Until the guarded runner exists, a signature proves a claim, not a run.
- With `--filter=blob:none`, a partial clone fetches the notes' contents
  only when read, and fails offline.

Unchanged: GitHub Actions keeps running, and git-board keeps reading its
results.

## Left open

The cycle protocol's M4 brief carried these. M4 settled the ones marked
so; docs/guide/configuration.md › Tests and checks has the details. The decision
above is unchanged.

- The toml's shape:
  - the signers file's path: settled by M4, `[cycle] signers`, default
    `.allowed_signers`, read at the base branch;
  - a check's version commands and variables: settled by M4, a suite's
    `tools` (their output's hash is `tools=`) and `env` (part of the key);
  - required checks beyond the default, the platforms and tools a reviewer
    accepts, and which checks only a runner can prove: settled by M4 as
    none. Every `[cycle.checks.*]` is required, any platform counts, and
    `where = "ci"` marks the forge's suites, which never gate. A key comes
    with a second platform or a runner;
  - the land's sample rate: open, with the land's re-run (wtx).
- How a clone's writer is named: settled by M4, `git config
  git-board.writer`, set on the first run to `<user.name>-<host>-<4 hex>`.
- Per-test results in the line (in-toto `failedTests`), so the bug rule
  shows which test failed: open.
- The verify script, and git-board's verify command: settled by M4,
  `git board verify [commit]`, the server's gate in any clone.
- The UI: step 4's runs and "not proven" with its reason are settled by
  M4. Step 7's run at the merged head is open (M6).
- Mirroring a proof to the forge as a commit status, if anyone needs one:
  open.
- The merge queue: `wtx land` moves main only to proven commits (Graydon
  Hoare's "not rocket science" rule): open (wtx).
- Pushing and fetching `refs/notes/ci/*` with git-board's sync: open. The
  reader already takes `refs/notes/remotes/*/ci/*`.

## Evidence

Two reviews on 2026-10-22, with experiments on git 2.50.1 and OpenSSH 10.2
in throwaway repositories.

- **Parallel writes lose lines with no error.** Two worktrees each ran 100
  `git notes append` calls: 141 of 200 lines kept, every exit code 0.
  `commit_notes` updates the ref without checking its old value. With
  compare-and-swap: 200 of 200, after 107 retries.
- **A note on a hash with no object behind it** shows in a clone, survives
  `gc --prune=now`, and merges. `git notes prune` lists it for deletion.
- **`notes.rewriteRef`** copied a pass onto a rebased commit whose covered
  file had changed: a false proof.
- **Scale:** 2,000 notes took 904 KiB once packed, with a 19 ms lookup.
- **SSH:** the one-line signature verified. A changed payload, the wrong
  namespace, and a principal claimed by someone else were each refused.
  `-Overify-time` lets a signer choose the time, so time is not trusted, and
  rotating a key means revoking it and running the checks again.
- **Forges:**
  - GitHub keeps `refs/notes/*` but has displayed nothing since 2014, and
    its rulesets don't cover them;
  - GitLab displays nothing, and its pull mirroring drops them;
  - Gitea and Forgejo display only `refs/notes/commits`.
- **Prior art:**
  - mhagger's git-test keys results by tree in `refs/notes/tests/<name>`,
    and records nothing from a dirty tree;
  - git-appraise keeps CI results in `refs/notes/devtools/ci`, unsigned;
  - gittuf signs in-toto attestations with SSH under `refs/gittuf/`;
  - SLSA's source-tool stores one signed attestation per line of a git note,
    which is the closest precedent;
  - git-test keys by tree alone. Bazel's action key and Turborepo's hash
    also cover the command and the toolchain, as the key and the line do
    here.

Sources:

- https://git-scm.com/docs/git-notes
- https://man.openbsd.org/ssh-keygen
- https://github.com/mhagger/git-test
- https://github.com/google/git-appraise
- https://github.com/gittuf/gittuf/blob/main/docs/design-document.md
- https://github.com/slsa-framework/source-tool/blob/main/docs/DESIGN.md
- https://github.com/in-toto/attestation/blob/main/spec/predicates/test-result.md
- https://github.com/in-toto/attestation/blob/main/spec/v1/bundle.md
- https://github.com/basecamp/gh-signoff
- https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-rulesets/about-rulesets
- https://docs.github.com/en/repositories/creating-and-managing-repositories/repository-limits
- https://github.blog/news-insights/git-notes-display
- https://gitlab.com/gitlab-org/gitlab/-/issues/15029
- https://gitlab.com/gitlab-org/gitlab/-/issues/23519
- https://github.com/go-gitea/gitea/blob/main/modules/git/notes.go
- https://bazel.build/remote/caching
