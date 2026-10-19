# Work unit M4 — review in git-board (git-board part)

> **For agentic workers:** this plan is executed task by task with TDD
> (failing test → run → implement → run → commit). The first task copies
> this file to `docs/prd/work-unit-m4-plan.md`. The fork part (git-bug
> `review-sync`, released as v0.11.1-sync.6) is built by the main session
> `git-bug-95`: never change `../git-bug`, never commit a `replace` in
> go.mod; fork needs beyond the agreement go to that session as a message.

## Context

M1 (numbers), M2 (roles) and M3 (the PR gate) are on main (02c3152). An
issue page shows its branches and their PR gate (side panel, Development),
but reviewing the code still means leaving for GitHub. M4 brings the review
to the issue: a **Changes** tab (layout A) with the branch's diff computed
locally, line comments, replies, resolve, and Comment / Approve / Request
changes, all stored as git-bug comments with `review:*` metadata so the fork
mirrors them to the PR (and PR review comments back to the issue's lines).

Done when, on guilbep/git-board-test: a line comment made in git-board shows
on the PR's line; a PR comment shows on the issue's line; an approval made in
git-board satisfies the code-owner rule; the PR merges from the issue.

Spec: `docs/prd/work-unit.md` § M4 (lines 209–251) and `PROMPT.sent.md`
(the brief, repo root). Style reference: `docs/prd/work-unit-m3-plan.md`.

## Architecture

**Storage (fork agreement, sync.5 already accepts it):** every review
object is an AddComment op with metadata, written with
`cache.BugCache.AddCommentRaw(author, unix, body, nil, metadata)`:

| object | metadata |
|---|---|
| line comment | `review:path`, `review:line`, `review:side` (old\|new), `review:commit`, `review:pr` (`github.com/o/r#26`, "" when no PR yet) |
| reply | the parent's anchor + `review:reply-to` = parent comment id |
| review | `review:state` (APPROVE\|REQUEST_CHANGES\|COMMENT), `review:commit`, `review:pr`; summary as body |
| resolve | `SetMetadata` on the thread's first comment op: `review:resolved` = true\|false, **last op wins** |

git-bug's `SetMetadata.Apply` is first-wins in `AllMetadata()`
(`entity/dag/operation.go` `addExtraMetadataImmutable`), so git-board
computes `resolved` itself by scanning `snap.Operations` for
`*dag.SetMetadataOperation[*bug.Snapshot]` whose `Target` is the comment's
op id, as `onForges` in `internal/gitbugio/thread.go:159-214` already does
for sync keys. An imported PR comment carries origin key
`sync:github.com/o/r:pr-comment` and the same `review:*` anchor (verify at
sync.6).

**Diff:** git subprocesses on the repository, the pattern of
`ChangesIn` in `internal/gitbugio/code.go:117-124`
(`exec.Command("git", "--git-dir", b.dir, …)`; `b.dir == ""` on in-memory
backends → "no repository", callers degrade). Base = `git merge-base
<base> <branch>`, base name from `branches.Suggest(...).Base`
(`internal/branches/branches.go`). File list `git diff --numstat -z -M`,
one file `git diff -M <from> <to> -- <path>` (server truncates to a line
limit), generated files `git check-attr --source=<head> -z
linguist-generated --stdin` (git ≥ 2.40, we have 2.50), commits `git log
--format=… <from>..<to>`.

**Outdated and quoted lines:** computed server-side when a thread is
written (`writeThread`, `internal/server/issues.go:52`), for anchored
comments only, one `git diff -U0 <review:commit> <head> -- <path>` and one
`git show <review:commit>:<path>` per distinct (commit, path) pair. `<head>`
is the first branch of the issue that contains `review:commit` (`git
for-each-ref --contains`); `outdated` = a hunk's old range covers the line
(side=new only; `ponytail:` side=old never goes outdated: the merge base
only moves on a rebase).

**Web:** a `ChangesView.vue` behind a third tab in `IssuePanel.vue`
(`mode` ref at line 195, tabs at 101–106), a `DiffFile.vue` for one file,
and a pure, `node --test`-tested `web/src/review.ts` (groups, unified diff
parsing, threads, viewed storage, "N commits ago"). Conversation renders an
anchored comment in `CommentCard.vue` with its `path:line`, the quoted line
and an "outdated" badge. No new dependency, no syntax highlighter.

**Mirroring:** needs sync.6. git-board then passes
`SyncOptions.PullsOf` (PR numbers of an issue, from `ghpr.Find` over its
origin branches, i.e. `onOrigin` in `internal/server/pulls.go:46`) into
the sync calls in `internal/gitbugio/identities.go` (`SyncForges`,
`GetForges`, `SendForges`, `SyncIssue`) through whatever entry point
sync.6 exposes for it.

## Tech stack

Go (fork `github.com/guilbep/git-bug` pinned v0.11.1-sync.5 at
go.mod:116), method-pattern `http.ServeMux` (`internal/server/server.go`
newMux 94–158), `writeError/writeJSON/writeCoded`, `decode`,
`beginWrite` (409 while a forge sync runs). Vue 3 + TypeScript (vue-tsc),
hash routing, tests `node --test` with the explicit file list in
`web/package.json` (append new test files there). Ponytail mode: minimal
code, `// ponytail:` on deliberate ceilings.

## Global constraints

- Work on `prd/review` only; never push main, never tag; `make ci` and
  `cd web && npm run typecheck && npm test` before every commit.
- No `replace` bump; the main session bumps the pin to sync.6 on this
  branch. Until then **run no forge sync on a repository holding review
  ops**: sync.5 would export them as plain issue comments (the skip lands
  in sync.6). Tests use in-memory backends and temp repos only.
- Live tests only on guilbep/git-board-test (public, rulesets enforced).
  Each write to that forge (CODEOWNERS, ruleset, PR, review) is asked
  first. The second account is `guilbep-gbtest`: ask the user to add its
  token to the keyring when needed, never ask for the value. Never `git
  bug bridge auth`, never `fetch --prune` / `remote prune`.
- Keep `IssueBranches.vue` untouched if possible (prd/release edits it);
  the tab goes in `IssuePanel.vue`.
- `rtk` rewrites output: `rtk proxy <cmd>` for exact output. No `timeout`
  on macOS: `go test -timeout`. git-bug keyring tests set `HOME` and
  `XDG_CONFIG_HOME` to a temp dir (see `gateRepo`,
  `internal/server/pulls_test.go:93`).

## Amendments (2026-10-18, before Task 1)

A review of the approved plan changed these details inside Tasks 1–5; the
structure stays. Each wins over the text below where they differ.

1. **The diff is origin's branch.** A review mirrors to the PR and counts
   for the code-owner rule only at the PR's head, which is origin's copy;
   other reviewers only have origin. Default head: `origin/<branch>` when
   it exists, the local branch only when origin has none. Merge base
   against `Suggestion.Start` (`origin/<base>`), not the bare base name
   (local dev can be stale). `review:commit` is resolved to the full SHA
   on the server before it is written. (Replaces Review focus 9's "local
   preferred".)
2. **Outdated diffs to the branch's current head, not to a branch
   containing the commit.** After a force push no branch contains the old
   commit, and those comments are the ones most likely outdated. Head =
   the issue's branch head (amendment 1's choice); skip only when the
   commit object isn't in the repository (`git cat-file -e`), e.g. an
   imported comment on a commit never fetched. `BranchesContaining` goes.
3. **Lines are translated to head.** A comment at an older commit stores
   that commit's line. The `-U0` hunks give the shift: each hunk wholly
   above the line moves it by (new count − old count); a hunk covering it
   makes it outdated. `Review.HeadLine` carries the translated line
   (0 when outdated or unknown). The web keys a thread on `head_line`
   when the view ends at head, on `line` when the view ends at the
   comment's commit, and otherwise lists it in a file-level box above the
   diff. `ChangedOld` becomes `LineAt(from, to, path, line) (int, bool)`.
4. **Viewed marks store the file's blob.** `git diff --raw --numstat -z`
   gives each file's new blob id (`FileStat.Blob`); a mark is path → blob,
   so a push un-views only the files it touched. (Replaces Review focus
   8's "per head".)
5. **The Changes tab is on the full issue page only** (`IssuePanel`'s
   `page` prop); the board's side panel keeps Conversation and History.
6. **Contract questions sent to git-bug-95** (2026-10-18): how PullsOf is
   passed through `SyncAll`/`SyncIssue`; `review:reply-to` = the parent's
   CombinedId; `review:commit` a full SHA; what export does with an empty
   `review:pr`. Answered the same day:
   - Options are variadic: `core.SyncAll(…, opts ...Option)`,
     `core.SyncIssue(…, opts ...Option)`, `core.WithPullsOf(f)`, and
     `(*Bridge).Use(opts ...Option)` after `LoadBridge` for Get/Send.
     Names may still change before sync.6 is tagged.
   - reply-to = CombinedId and commit = full SHA: confirmed.
   - **No PR yet: omit `review:pr`** (never write ""; first value wins).
     Export trusts `review:pr` (SetMetadata included) and never
     recomputes it; an op without it waits. git-board may later stamp
     `review:pr` on that branch's earlier review ops with SetMetadata:
     Task 7 does it on Open PR.
   - Imported PR comments carry the same `review:*` keys, `review:pr`,
     and origin `sync:github.com/o/r:pr-comment`.
7. **sync.6 tagged** (git-bug-95, 2026-10-18; bridge-sync 3b0a745c, names
   as agreed; decisions in the fork's
   `doc/design/bridges/sync-decisions.md` § "Pull requests: git-board's
   review"). For Task 7:
   - Sync: `core.SyncAll(ctx, repo, names, dryRun, core.WithPullsOf(f))`;
     Send now: `core.SyncIssue(ctx, repo, id, nil, dryRun,
     core.WithPullsOf(f))`; Get: `br.Use(core.WithPullsOf(f))` after
     `LoadBridge`, before `ImportAllSince`. Send (`ExportAll`) needs none.
     Without a pull that had PullsOf, a crash-posted comment isn't matched
     back. One WithPullsOf serves every GitHub bridge of a run.
   - f returns bare numbers ("26"), OPEN pull requests only (three REST
     listings per pull each), cheap, ideally cached. It runs on the engine
     goroutine under the bridge lock, once per bug per pull, and must not
     call core sync functions.
   - Imported comments: `review:commit` = GitHub's original_commit_id,
     `review:line` = original_line (where it was written, kept when
     outdated), full SHAs. `review:pr` key lowercased
     (`github.com/owner/repo#7`): compare case-insensitively. An approval
     without text imports as an empty comment with `review:state=APPROVE`.
   - Records: sent = SetMetadata `sync:<key>:id`/`:url`/`:as`; pulled =
     the op's own `sync:<key>:pr-comment` (node id), `:url`, `:text`;
     resolution GitHub last saw = SetMetadata `sync:<key>:resolved`, last
     wins. Resolutions made on GitHub are not imported.
   - Review comments never get issue-comment records: don't show them as
     "not on GitHub yet" in the pending logic.
   - Approvals and change requests go only under the author's own token,
     else "missing author token: approval|change request by X not sent to
     pull request #N". COMMENT reviews and line comments may go via the
     default account, attributed.

## Review focus (risks → tests)

1. **numstat parsing**: renames (`-z` gives two paths), binaries (`-`),
   tabs in paths → `TestDiffStat` with a renamed and a binary file.
2. **Thread path cost**: git calls only for anchored comments, grouped per
   (commit, path), skipped when `b.dir == ""` → `TestAnnotateSkipsInMemory`,
   `TestOutdated` (line in a changed hunk, line outside it, side=old).
3. **resolved last-wins** across two SetMetadata ops → `TestResolveTwice`.
4. **Reply anchor** copied from the parent server-side, parent must be
   anchored → `TestReplyInheritsAnchor`, 400 on a non-anchored parent.
5. **Review validation**: state ∈ 3 values, commit required; a line
   comment needs path+line+side+commit; `review:pr` matches
   `github.com/o/r#N` or empty → table test.
6. **Groups and thresholds**: 999/1000/3000/3001 changed lines, generated
   wins over size, generated excluded from the viewed count → `review.test.ts`.
7. **Unified diff parsing**: hunk headers with and without counts, `\ No
   newline at end of file`, rename headers → `review.test.ts`.
8. **Viewed per head**: marks keyed `gb-viewed:<issue>:<branch>`, a mark
   made at another head doesn't count → `review.test.ts`.
9. **Branch choice**: `?branch=` must be one of the issue's branches
   (`branches.Linked`), local ref preferred over `origin/` → 400 otherwise.

## Tasks

### Task 0 — plan into the repo

- [x] Copy this file to `docs/prd/work-unit-m4-plan.md`, with the amendments below. Commit `docs(prd): M4 plan`.

### Task 1 — git diff helpers (`internal/gitbugio/diff.go`)

Files: `internal/gitbugio/diff.go`, `internal/gitbugio/diff_test.go`
(temp repo via `exec` git as `internal/server/branches_test.go` `git(t,
dir, …)` does — copy that tiny helper into a gitbugio test file).

Interfaces (all on `*Backend`, `ErrNoRepo` when `b.dir == ""`):

```go
type FileStat struct {
    Path      string `json:"path"`
    From      string `json:"from,omitempty"` // renamed from
    Added     int    `json:"added"`
    Deleted   int    `json:"deleted"`
    Binary    bool   `json:"binary,omitempty"`
    Generated bool   `json:"generated,omitempty"`
}
func (b *Backend) MergeBase(a, c string) (string, error)
func (b *Backend) RevParse(rev string) (string, error)
func (b *Backend) DiffStat(from, to string) ([]FileStat, error)   // --numstat -z -M, Generated filled by check-attr --source=to
func (b *Backend) DiffFile(from, to, path string, limit int) (diff string, truncated bool, err error)
func (b *Backend) Log(from, to string) ([]Commit, error)          // reuse Commit from code.go, newest first
func (b *Backend) BranchesContaining(sha string) ([]string, error) // for-each-ref --contains, refs/heads + refs/remotes/origin, "origin/" stripped
func (b *Backend) ShowLine(rev, path string, line int) (string, error)
func (b *Backend) ChangedOld(from, to, path string) ([][2]int, error) // old ranges [start, count] of git diff -U0
```

Steps:
- [ ] Write failing tests: a repo with `dev` and `fix/1-x` (two commits,
      one rename, one binary, a `.gitattributes` marking `gen/*
      linguist-generated`); assert MergeBase, DiffStat (paths, counts,
      From, Binary, Generated), DiffFile truncation at limit 3 →
      `truncated`, Log order, BranchesContaining, ShowLine, ChangedOld
      ranges. `TestDiffNoRepo` on `gitbugiotest.New(t)` → `ErrNoRepo`.
- [ ] Run, red. Implement. Run, green. `make ci`. Commit
      `feat(gitbugio): git diff helpers for the Changes tab`.

### Task 2 — review ops and `Event.Review` (`internal/gitbugio/review.go`)

Files: `internal/gitbugio/review.go`, `review_test.go`, `thread.go`
(Event struct at 57–81, comment closure in `event()`), `gitbugiotest`
(a `CommentWith` helper if tests need it).

Interfaces:

```go
// Review is a comment's review metadata (review:* keys), nil for a plain comment.
type Review struct {
    Path     string `json:"path,omitempty"`
    Line     int    `json:"line,omitempty"`
    Side     string `json:"side,omitempty"`     // old | new
    Commit   string `json:"commit,omitempty"`
    PR       string `json:"pr,omitempty"`       // github.com/o/r#26
    ReplyTo  string `json:"reply_to,omitempty"`
    State    string `json:"state,omitempty"`    // APPROVE | REQUEST_CHANGES | COMMENT
    Resolved bool   `json:"resolved,omitempty"` // last SetMetadata review:resolved wins
    Outdated bool   `json:"outdated,omitempty"` // filled by the server
    Text     string `json:"text,omitempty"`     // the anchored line at Commit, filled by the server
}
// on Event: Review *Review `json:"review,omitempty"`

func (b *Backend) CommentWith(idPrefix, body string, metadata map[string]string) (string, error) // AddCommentRaw as me, now; returns the comment id
func (b *Backend) SetCommentMetadata(idPrefix, commentID string, metadata map[string]string) error // SetMetadata on the comment's op
func (b *Backend) CommentReview(idPrefix, commentID string) (*Review, error) // the parent's anchor, for replies
```

Steps:
- [ ] Failing tests on `gitbugiotest.New(t)`: `CommentWith` with an anchor
      → `Thread()` event has `Review{Path, Line, Side, Commit, PR}`; a
      reply has `ReplyTo`; a review has `State`; a plain comment has
      `Review == nil`; `SetCommentMetadata` resolved=true then false →
      `Resolved` false (last wins); `review:line` not a number → ignored,
      not an error.
- [ ] Red → implement (`b.write` + `bug.AddCommentRaw(me, …)`, author from
      `b.c.GetUserIdentity()`; `thread.go` fills Review from the op's
      `AllMetadata()` and the SetMetadata scan) → green → commit
      `feat(gitbugio): review comments, replies, reviews and resolve as
      metadata`.

### Task 3 — routes (`internal/server/review.go`)

Files: `internal/server/review.go`, `review_test.go`, `server.go` (routes),
`issues.go` (`writeThread` calls `s.annotate(th)`).

Routes:

```
GET  /api/issues/{id}/changes?branch=&from=&to=
     → {branch, base_name, base, head, from, to, commits: [Commit], files: [FileStat]}
       defaults: branch = first of branches.Linked (local ref preferred over origin/),
       from = merge-base(base_name, branch), to = branch head
GET  /api/issues/{id}/changes/file?branch=&from=&to=&path=&limit=
     → {path, diff, truncated}
POST /api/issues/{id}/review   {body, path, line, side, commit, pr, reply_to, state}
     → thread (201); a reply copies the parent's anchor; validation per Review focus 5
POST /api/issues/{id}/review/{comment}/resolve {resolved} → thread
```

`annotate(th *gitbugio.Thread)`: for events with `Review.Path != ""`
and `Side == "new"`, group by (commit, path); head = first of
`BranchesContaining(commit)` that `branches.Names(name, is)` (else skip);
`Outdated` from `ChangedOld`, `Text` from `ShowLine`. No-op when the
backend has no repository or git fails (the thread still answers).

Steps:
- [ ] Failing handler tests: `newIssueServer`/`call` style
      (`internal/server/issues_test.go`) for POST review (line comment,
      reply inherits anchor, review, 400 table, 409 while syncing via
      `s.syncing.Store(true)`), resolve twice; a temp-repo server (like
      `gateRepo` but no token) for GET changes (defaults, `?branch=`
      400 on a foreign branch, generated flag), GET file (truncated),
      and annotate (outdated true/false, text, in-memory skip).
- [ ] Red → implement → green → `make ci` → commit `feat(server): the
      Changes API and review writes`.

### Task 4 — web helpers (`web/src/review.ts`)

Files: `web/src/review.ts`, `web/src/review.test.ts` (append to
`package.json` `test`), `web/src/api/board.ts` (types + `getChanges`,
`getFileDiff`, `postReview`, `resolveReview`; `IssueEvent.review?`).

```ts
export type Group = 'review' | 'large' | 'generated';
export function groupOf(f: FileStat): Group;            // generated > 3000 → large > rest
export function autoOpens(f: FileStat): boolean;        // review group and added+deleted < 1000
export const LARGE_LIMIT = 5000;
export function diffCommand(base: string, branch: string, path: string): string; // git diff dev...<branch> -- <path>
export type Row = { op: ' ' | '-' | '+' | '@'; old?: number; new?: number; text: string };
export function parseUnified(diff: string): Row[];
export type ReviewThread = { root: IssueEvent; replies: IssueEvent[]; resolved: boolean };
export function threadsOf(events: IssueEvent[]): Map<string, ReviewThread[]>; // key `${path}:${side}:${line}`
export function reviewsOf(events: IssueEvent[]): IssueEvent[];               // state set, newest last
export function lastReviewBy(events: IssueEvent[], who: string): IssueEvent | undefined;
export function commitsAgo(sha: string, commits: { sha: string }[]): number | null; // index in newest-first list
export function loadViewed(issue: string, branch: string): Record<string, string>; // path → head, try/catch
export function saveViewed(issue: string, branch: string, v: Record<string, string>): void;
export function viewedCount(files: FileStat[], viewed: Record<string, string>, head: string): [number, number]; // excluding generated
```

- [ ] Failing tests per Review focus 6–8 and `commitsAgo`, `threadsOf`
      (reply under its root, resolved from root), `parseUnified`.
- [ ] Red → implement → `npm test` green → `npm run typecheck` → commit
      `feat(web): review helpers`.

### Task 5 — Changes tab (`ChangesView.vue`, `DiffFile.vue`, `IssuePanel.vue`)

Files: `web/src/components/ChangesView.vue`, `web/src/components/DiffFile.vue`,
`web/src/components/IssuePanel.vue`, `web/src/components/CommentCard.vue`,
`web/src/css/gitboard.css` (a few `.gb-diff-*` rules; reuse
`.cfg-d*` colours from ConfigEditor where they fit).

ChangesView (props `id`, `thread`, `pulls`-free: it reads
`getPulls` itself only to fill `review:pr`, or receives `pr` strings per
branch from IssuePanel if cheaper — pick the smaller diff):
- header: branch `<select class="epfl-select">` when > 1 branch; commit
  picker `<select>`: "All changes (base…head)", "Since your last review"
  (when `lastReviewBy(me)` has a commit in the list), each commit (from =
  its parent, i.e. previous in the newest-first list, to = it); summary
  "N files, +a −d · viewed k/n"; approvals line from `reviewsOf`:
  "Approved by X at d4e5f6a, 2 commits ago" (`commitsAgo`), "Changes
  requested by Y".
- groups as `<details class="gb-dev-new">`-style sections: To review
  (open), Large (folded), Generated (folded).
- each file is a `DiffFile`: head (path, `+a −d`, Viewed
  `<label class="epfl-check">`), body loads `getFileDiff` when opened
  (auto for `autoOpens`), Large: "Load the first 5,000" button + Copy
  `git diff …` button (`copyText` pattern from `IssueBranches.vue:179`),
  rows from `parseUnified` in a `<table>` with old/new numbers; click a
  number → composer under the row (`MarkdownEditor` v-model, Comment /
  Cancel); threads under their row: `MarkdownBody` per comment, author +
  `when()`, Reply box, Resolve/Unresolve on the root; outdated badge.
- footer: review box — radios Comment / Approve / Request changes,
  summary `MarkdownEditor`, "Submit review" → `postReview({state, commit:
  head, pr, body})`.
- writes go through `useIssue`'s `write` (409 → wait for the sync) by
  adding `review(...)` and `resolve(...)` to `web/src/issue.ts`; a write
  re-reads the thread; the Changes list re-reads on `onLive` issue events
  for this id and when the branch head changes.

IssuePanel: `mode: 'talk' | 'history' | 'changes'`, third tab "Changes"
(hidden when the issue has no branch: `getBranches` is already fetched by
IssueBranches; simplest: always show the tab, the view says "No branch
names this issue yet" with the create-branch hint).

CommentCard: when `event.review?.path`, a line above the body:
`path:line` (button that switches to the Changes tab and scrolls to the
row, via an emit the panel handles), the quoted `review.text` in
`<pre class="gb-mono">`, "outdated" badge; when `event.review?.state`, a
header "approved" / "requested changes" / "reviewed" at `commit[:7]`.

Steps:
- [ ] Build; `npm run typecheck && npm test`; run `git board serve --addr
      localhost:8091` on a scratch clone in the scratchpad with a branch
      and an issue naming it; check in the Playwright tab (own tab only):
      tab, groups, open a file, comment a line, reply, resolve, submit a
      review, approval line, viewed count, Conversation rendering.
- [ ] Commit `feat(web): the Changes tab`.

### Task 6 — docs

- [ ] `docs/configuration.md` § Branches: a "Changes" paragraph (what the
      tab shows, groups and thresholds, where comments and reviews go,
      what the sync mirrors).
- [ ] `README.md` feature line; `docs/prd/work-unit.md` M4: "Built
      2026-10-xx" notes (what differs from the spec, e.g. side=old never
      outdated, viewed per head).
- [ ] Commit `docs: the Changes tab and review`.

### Task 7 — PullsOf and mirroring (after the pin is at sync.6)

Gate: `go.mod:116` says `v0.11.1-sync.6` (the main session bumps it).
Then read the fork's `bridge/core/sync.go` SyncOptions and how
`SyncAll`/`SyncIssue` take options.

- [ ] If sync.6 has no way to pass `PullsOf` through `SyncAll`/`SyncIssue`
      (today `syncRun` builds `SyncOptions` in `Bridge.newEngine`), message
      `git-bug-95` and wait; build nothing around it.
- [ ] `internal/gitbugio/identities.go`: the sync entry points take a
      `pullsOf func(id entity.Id) []string`; `internal/server/forges.go`
      and `issuesync.go` build it: issue by id → `onOrigin` names →
      `ghpr.Find` per name → numbers (errors → nil for that issue, logged).
      Test with the fake GitHub (`newFakeGitHub`) that the callback returns
      the PR numbers and nil when origin isn't GitHub.
- [ ] Imported PR comments: `Event.Review` already renders them (same
      keys); check an imported op's `review:*` keys match the agreement
      and that `pending`/`onForges` treat the `pr-comment` origin key as
      "on that forge" (`thread.go` `onForges`: a key `sync:<remote>:…`
      that doesn't end in `:id` must still count). Add a thread test.
- [ ] `make ci`, commit `feat(sync): PR numbers for the import, review
      comments mirrored`.

### Task 8 — live test on guilbep/git-board-test

Ask before each forge write. Steps, each reported:
- [ ] Scratch clone in the scratchpad; `git board serve --addr
      localhost:8092`; bridge configured for the scratch repo with the
      user's token (already in the keyring) — a sync here is fine once the
      pin is sync.6.
- [ ] Ask: add `CODEOWNERS` (`* @guilbep-gbtest`) and a ruleset on the
      default branch (required check "Tests", require code-owner review);
      ask the user to add `guilbep-gbtest`'s token to the keyring.
- [ ] Branch + issue + PR from git-board (Open PR, M3). Line comment in
      git-board → sync → shows on the PR line. PR review comment on
      GitHub (as gbtest, through the API with the keyring token, or ask
      the user to click) → sync → on the issue's line, outdated after a
      push. Approve as gbtest from git-board (identity switch) → the gate
      shows the code-owner review satisfied → Merge from the issue.
- [ ] Note results in the PRD's M4 "Live test" notes; commit.

### Task 9 — reviewer and fix pass

- [ ] Fresh reviewer agent on the most capable model, given the diff
      `main...prd/review`, the PRD M4 and the fork agreement, asked for
      bugs, missed spec points and the Review focus list.
- [ ] One fix pass: each fix starts with a failing test, then passes.
      Commit `fix(review): …`.

### Task 10 — ready

- [ ] `make ci`; `cd web && npm run typecheck && npm test`;
      `$(GOTOOLCHAIN=go1.26.0 go env GOROOT)/bin/gofmt -l .` and Go 1.27's
      gofmt both print nothing.
- [ ] Say it is ready: the user lands with `wtx land prd/review`.

## Verification

- Go: `go test ./internal/gitbugio ./internal/server -run 'Diff|Review|Changes|Annotate|Resolve' -timeout 120s`, then `make ci`.
- Web: `cd web && npm run typecheck && npm test`.
- Manual (Task 5, Task 8): a server on a scratch clone, own Playwright tab,
  `http://localhost:<port>/#/issues/<id>` → Changes tab → the flows above;
  Conversation shows the anchored comment with its quoted line.
- End-to-end (Task 8): the four "done when" points on guilbep/git-board-test.

