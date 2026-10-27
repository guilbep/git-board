# Time flow M5: sync, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** A thread comment goes to the forge flat, in time order, with a
quote header saying where it is anchored; the thread, spec and
`cycle:step` keys ride in the marker sync already writes and come back as
`meta` on pull; a published version gets a "Published v*N*" note; on
GitLab a reply joins its root's discussion. Screen 6f.

**Architecture:** Sync is the repository's own `core/syncer`, already
wired by `internal/store/forges.go` and `switch.go`: no change to the
git-bug fork. The core learns four generic things, each a callback in
`syncer.Options` beside `Local` and `Held`, so it gives no key a meaning:
which keys a marker carries and how pull restores them (`Carry`,
`Restore`), a comment's quote header (`Quote`), the note a published
version gets (`Published`), and a comment's thread root (`ReplyTo`). The
meanings live in `internal/store`: the short names, the quote's text and
the version and section rules, ported from `web/src/sections.ts` with its
test vectors.

**Tech Stack:** Go (`core/syncer`, `core/forge`, `internal/store`).

**Spec:** `docs/prd/time-flow.md`, "M5: sync" and call 1 (`step=7`);
`docs/design/time-flow/protocol.md`, "What sync does" and "The example of
6f, in full"; `screens/F6-protocol.html`, panel 3 and the table;
`docs/spec/sync.md`, the reference, which each task updates.

**Base:** `feat/time-flow-m2` at 1c78809 (M2, M3, M4 on it). The user
asked for continuity: no approval stop, no landing between milestones,
one `make ci` at the end.

## What exists today, found in the code

**The marker.** `core/syncer/text.go`: `marker(kind, id)` writes `<!--
git-board issue|comment <id> -->`; `markerRE` ends at the id, so a marker
with keys after it is not one today (crash recovery, `unwrap`, `Records`
all stop seeing it). `unwrap` splits a posted text into `wrapped{header,
body, marker…}`; every reader goes through it: the plan's text compare
(`wrapLike`), `Records` (`links.go:76`, then the store's `pending`, the
"edit waiting" mark), `same`, `plain`.

**What pull does with a posted comment.** It is matched by its `posted`
record, or by its marker from sync's accounts (crash recovery); a marked
comment whose id is not local is skipped (sync.md, "Markers"). So a
comment sync posted comes back as itself and its `meta` is never read
from the forge. The one path where a forge text reaches git-board is a
forge edit of that comment (`text()`'s `pullIt`): there the marker's keys
are restored (ruling 2).

**What stays local.** `Options.Local` is `isLocal`
(`internal/store/forges.go:315`): any `review:*` key. A thread comment on
the body has none, so it is posted today, without header or keys.

**The core issue has no body history.** `issue.Issue` has the current
body; the versions are the store's `Thread.History` (`Version{ID, Author,
At, Body}`), which the callbacks read through `b.Thread(id)`, once per
issue and run.

## Global Constraints

- No new entity or operation kind; the core gives no key a meaning:
  every key name and value rule is in `internal/store`. Versions and
  thread states are computed, never stored. `cycle:step` keeps "1" to
  "7": the marker writes `step=7` (PRD call 1, over protocol.md's "01").
- A `review:*` comment is never posted (`isLocal`, unchanged).
- GitHub reads without git-board: flat comments in time order, the quote
  header, the ```suggestion block kept, a verdict's first line bold.
- sync.md's rule holds: "The header and the marker a text has on a
  project stay the ones it was created with", except the marker's keys,
  rebuilt from the comment's `meta` each run (ruling 3).
- Live checks: guilbep/git-board-test only. No live call to GitLab
  (gitlab.epfl.ch is not a scratch forge here): GitLab is tested against
  `httptest`, as `gitlab_test.go` does.
- While developing: `go test ./core/syncer/ ./core/forge/... ./internal/store/
  -run …`. `make ci` once, at the end, through `rtk proxy make ci` into a
  log. A new Go test calls `t.Parallel()` first. No new dependency. No
  attribution trailer. `ponytail:` on deliberate shortcuts.

## Review Focus

1. A thread comment synced twice: the second run pushes nothing and its
   `Records` seen text is the local body, so no "edit waiting" (Task 2:
   `TestQuotedCommentSettles`).
2. Resolve after posting: the `meta` change alone edits the forge comment
   to the new keys, once (Task 1: `TestKeysFollowMeta`).
3. A forge edit that deletes the marker: the comment becomes plain, its
   thread and spec keys cleared, `cycle:step` kept, the forge's text kept
   (Task 1: `TestMarkerDeletedOnTheForge`; Task 5 `restore`).
4. A tick on the body is no version: no "Published" note (Task 5:
   `TestPublishedSkipsATick`); a version is noted once, even after a crash
   between the post and its record (Task 3: `TestPublishedOnce`).
5. A stranger's text with a git-board marker and keys is just text: pulled
   as a new comment with no `meta` (Task 1: `TestStrangerKeysAreText`).

---

### Task 0: the plan in the repository

- [ ] Commit this file, `docs(prd): the time flow's M5 plan`.

### Task 1: the marker carries keys

**Files:** `core/syncer/text.go`, `core/syncer/plan.go`,
`core/syncer/sync.go` (`Options`), `core/syncer/sync_test.go`,
`docs/spec/sync.md` ("Markers").

**Interfaces:**

```go
// Options gains:
// Carry is what a comment's marker carries after its id: "k=v" pairs,
// space-separated, each value without spaces or ">". Nil or "" carries none.
Carry func(meta map[string]string) string
// Restore is the meta a forge edit of a comment sync posted sets: from the
// marker's pairs (marked), or with the marker deleted (!marked). Keys
// mapped to "" are cleared. Nil restores nothing.
Restore func(carried string, marked bool, meta map[string]string) map[string]string

func marker(kind, id, keys string) string // keys "" writes today's marker
// markerRE: (issue|comment|version) <id>((?: [a-z][a-z-]*=[^\s>]*)*) -->
// wrapped gains keys string
// wrapLike(seen, body, keys string, posted bool): seen's header and marker
// id, the keys given
```

Push: a new comment's marker gets `Carry(c.Meta)`; a posted comment's
`g` is `wrapLike(seen, …, Carry(c.Meta), sent)`, so a `meta` change alone
edits the forge comment. Pull, `pullIt` of a posted comment: besides the
text, `Restore(w.keys, w.marker != "", c.Meta)` non-empty is one
`issue.SetMeta(ca, c.ID, …)` from the forge (`from(…, key, field, ft)`).

- [ ] **Step 1:** Tests on the fake forge: `TestMarkerKeys` (a comment
  with meta and a `Carry` of `"step=7 anchor=body@v1/expected#3"` is
  posted ending `<!-- git-board comment <id> step=7
  anchor=body@v1/expected#3 -->`; a second `Sync` reports `Pushed == 0`
  and no new local comment; a crash before the record (the fake's post
  kept, the record dropped) is found by its marker, not posted twice);
  `TestKeysFollowMeta` (a `SetMeta` locally, then `Sync`: the forge
  comment's marker ends with the new pair, one edit); `TestForgeEditRestores`
  (the forge comment edited to `resolved=v2` in its marker: the pull
  writes one `set-meta` with what `Restore` returned, the body without the
  marker); `TestMarkerDeletedOnTheForge` (the marker gone: `Restore`
  called with `marked` false, the body is the forge's text whole);
  `TestStrangerKeysAreText` (a forge comment by another account ending
  with a marker and keys: a new comment, its body as written, no `meta`).
- [ ] **Step 2:** Run `go test ./core/syncer/ -run 'TestMarker|TestKeys|TestForgeEdit|TestStranger'`. FAIL.
- [ ] **Step 3:** Implement; sync.md's "Markers" names the pairs, the
  `version` kind (Task 3) and the restore rule.
- [ ] **Step 4:** `go test ./core/syncer/` PASS.
- [ ] **Step 5:** Commit `feat(sync): a comment's marker carries keys, a forge edit restores them`.

### Task 2: the quote header

**Files:** `core/syncer/text.go`, `core/syncer/plan.go`, `sync.go`
(`Options`), tests, `docs/spec/sync.md` ("The forge's form").

**Interfaces:**

```go
// Options gains:
// Quote is the line a comment is posted under, after the header: a quote
// starting "> **On ". Nil or "" posts none.
Quote func(issueID string, c issue.Comment) string
// quoteRE = ^> \*\*On [^\n]*(?:\n\n|$), read by unwrap only inside a
// marker's text, after the header. wrapped gains quote; String() writes
// header, quote, body, marker.
```

A new comment's quote is `Quote(is.ID, c)`; a posted one keeps seen's, as
its header (sync.md: the form changes only when the text does).

- [ ] **Step 1:** Tests: `TestQuotedComment` (6f's comment, with a `Quote`
  and `Carry` giving 6f's line and keys, is posted exactly as
  protocol.md's "As sync posts it to GitHub #37", `step=1` for `step=01`);
  `TestQuotedCommentSettles` (a second `Sync`: `Pushed == 0`; `Records`
  seen for it equals the local body); `TestQuoteStrippedOnPull` (the forge
  edits the body under the quote: the pulled body has no quote and no
  marker); `TestQuoteOnlyInAMarkedText` (a stranger's comment starting
  `> **On v1 …**` keeps it).
- [ ] **Step 2:** FAIL. **Step 3:** Implement. **Step 4:** `go test ./core/syncer/` PASS.
- [ ] **Step 5:** Commit `feat(sync): a thread comment is posted under its quote header`.

### Task 3: the "Published vN" note

**Files:** `core/syncer/plan.go`, `sync.go` (`Options`), tests,
`docs/spec/sync.md`.

**Interfaces:**

```go
// Options gains:
// Published is the note sync posts when it pushes a new body: before is
// the forge's last body, after git-board's; id names the version (its
// edit's operation). id "" posts none.
Published func(issueID, before, after string) (id, text string)
```

`text()` returns its verdict; on `pushIt` of the body, after the write,
the note goes as a comment ending `<!-- git-board version <id> -->`, by
the body edit's author (`r.send`), recorded `posted` under
`[issue, id]`. Not posted when that record exists or `fi.Comments` holds
a comment from sync's accounts with that marker. Pull: a `version` marker
from sync's accounts is skipped, recorded when unrecorded. Never pulled as
a comment, never stored: M2's "Apply leaves two operations" holds.

- [ ] **Step 1:** Tests: `TestPublishedNote` (a body pushed with a
  `Published` returning `("e1:1", "**Published v2** · § Expected changed")`:
  one comment after the body edit, its text and marker); `TestPublishedOnce`
  (a second run, and a run after the record is dropped: still one);
  `TestPublishedNotPulled` (pull after: no new local comment).
- [ ] **Step 2–4:** FAIL, implement, `go test ./core/syncer/` PASS.
- [ ] **Step 5:** Commit `feat(sync): a published version gets its note on the forge`.

### Task 4: GitLab replies join their root's discussion

**Files:** `core/forge/forge.go`, `core/forge/gitlab/gitlab.go`,
`core/forge/fake/fake.go`, `core/syncer/plan.go`, `sync.go`, tests.

**Interfaces:**

```go
// forge.go: optional, GitHub doesn't implement it
type Replier interface {
	Reply(ctx context.Context, issue Ref, toCommentID, body string) (Comment, error)
}
// Options gains:
// ReplyTo is the git-board comment a comment replies to; "" for none.
ReplyTo func(meta map[string]string) string
```

Push: a comment whose `ReplyTo` root is posted on the project, through a
writer that is a `Replier`, is a `Reply` to the root's forge id; else a
`CreateComment`, as today. GitLab: `GET …/issues/:iid/discussions` finds
the discussion holding the note, then `POST
…/discussions/:discussion_id/notes`. Resolving: GitLab's API resolves
merge request threads, not issue threads, as its Discussions API lists
them (checked at this task; if it does resolve issue threads, `Resolve`
joins `Replier` and the root's `thread:resolved` drives it).

- [ ] **Step 1:** Tests: `TestGitLabReply` (`httptest`: the two calls,
  the note's id back); `TestReplyJoinsItsRoot` (the fake as a `Replier`:
  the reply goes to the root's forge id; with the root not posted, a
  plain comment).
- [ ] **Step 2–4:** FAIL, implement, `go test ./core/forge/... ./core/syncer/` PASS.
- [ ] **Step 5:** Commit `feat(sync): on GitLab a reply joins its root's discussion`.

### Task 5: git-board's meanings

**Files:** create `internal/store/body.go`, `body_test.go`; modify
`internal/store/forges.go`, `switch.go` (the options, one helper for
both), `internal/server/cycle.go` (the verdict's bold line),
`cycle_test.go`.

**Interfaces:**

```go
// body.go, the Go side of web/src/sections.ts, same vectors
type section struct{ slug, heading string; items []string; html string }
func sectionsOf(html string) []section // markdown.Render's HTML; items: each <li>'s text, nested lists excluded
func isTick(before, after string) bool
func bodyAt(h []Version, v int) (string, bool) // vK's text, ticks included
// the callbacks, built per run, caching each issue's thread
func (b *Backend) syncMeanings() (carry, quote, published, replyTo, restore)
```

- `carry`: `step`, `anchor`, `reply-to`, `kind`, `resolved`, `applied`,
  `verdict`, `on` for `cycle:step`, `thread:anchor`, `thread:reply-to`,
  `thread:kind`, `thread:resolved`, `thread:applied`, `spec:verdict`,
  `spec:on`, in that order; a value with a space or ">" is left out
  (ponytail: no escaping, none of these values has one).
- `restore`: marked, each carried key whose value differs, a missing one
  cleared; not marked, every `thread:*` and `spec:*` key cleared,
  `cycle:step` kept.
- `quote`: for `thread:anchor` `body@vN/slug[#k]`: `> **On vN · §
  Heading · k** · _item text_` (the intro "§ intro"; no item: `> **On vN ·
  § Heading**`); a slug vN lacks: `> **On vN · § slug**`.
- `published`: v*N* the version count after the edit, the sections whose
  HTML differs or that are new, "`**Published vN** · § intro, § Expected
  changed`"; a tick or an unchanged text: id "".
- `replyTo`: `thread:reply-to`.
- The verdict comment's body becomes "**Approved v1 of the description**"
  / "**Changes requested on v1 of the description**" (its first line bold).

- [ ] **Step 1:** `body_test.go` (parallel), sections.test.ts' vectors:
  `a heading's slug is its rendered text` (`## [Releases](https://x) page`
  → `releases-page`, `## A & B` → `a-b`), `a fence is not a heading`, `a
  repeated heading gets an index`, `a quoted heading opens none`, `no
  heading is one section`, `a tick is not a version`; `TestQuote` (6f's
  issue: the exact line); `TestPublishedSkipsATick`; `TestRestore` (the
  two cases); `TestThreadCommentSyncs` through `storetest`'s fake forge:
  a thread comment posted with its quote and keys, a `review:*` one not
  posted.
- [ ] **Step 2–4:** FAIL, implement, `go test ./internal/store/ ./internal/server/` PASS.
- [ ] **Step 5:** Commit `feat(sync): thread comments go to the forge with their quote and keys`.

### Task 6: verification and hand-off

- [ ] **Step 1:** Live, on guilbep/git-board-test: a scratch board with
  6f's issue (the description, Person B's question, Person A's answer,
  the suggestion, Apply), synced: `gh issue view` shows the comments flat
  in order, each quote, the block, "Published v2", no marker visible; a
  pull back: the thread JSON's comments have the same `meta` as before;
  the review comment absent from GitHub. If the sandbox stops the run,
  the script goes to the user.
- [ ] **Step 2:** `GOCACHE=$TMPDIR/go-build rtk proxy make ci > $TMPDIR/ci.log 2>&1`
  green (rerun `TestCutAndPromoteSameDigests` alone if it flakes).
- [ ] **Step 3:** Handover for M2–M5, then ready for `wtx land
  feat/time-flow-m2` with the HEAD. No push, no tag.

## Done when

The PRD's M5 list:
- 6f's comment, posted to a scratch repository and pulled back, is the
  same comment with the same `meta` (Tasks 1, 2, 6);
- a `review:*` comment is never posted (Task 5's test, Task 6);
- the issue on GitHub reads, in order, without git-board (Task 6).

## Rulings

This plan's calls, standing unless review says otherwise:

1. **No fork change.** Sync is `core/syncer`; the meanings are callbacks.
2. **Pull restores the keys on a forge edit.** A comment sync posted and
   not edited on the forge comes back by its record, unchanged: the same
   comment and `meta`. Edited there, its marker's keys become its `meta`;
   the marker deleted, it becomes a plain comment (thread and spec keys
   cleared, its step kept). A marked comment absent locally stays skipped,
   as sync.md says.
3. **The keys are rebuilt each run; the header and the quote are not.**
   Resolving edits the forge comment's marker; the quote stays the one
   posted.
4. **"Published vN" is sync's own note**, posted with the body edit it
   follows, under a `version` marker, never stored.
5. **A verdict's bold line is stored**, so git-board and the forge read
   the same text.
6. **GitLab: replies only.** A discussion's resolution is not built unless
   the API resolves issue threads.
7. **`step=7`**, per PRD call 1.

wtx-effort: high
