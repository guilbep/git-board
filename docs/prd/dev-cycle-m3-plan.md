# Dev cycle M3: the Work page, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `/work`, the page that lists what waits for a person: every
open issue's cycle grouped into Needs you, Agents, Waiting on others, To
send and Since the last retro, for the viewer (Mine) or anyone (Team).

**Architecture:** A pure package `internal/work` groups rows the way
`internal/inbox` does: the server computes every open issue's cycle
(`cycle.Gather` + `cycle.Compute`, M1) with one PR-cache refresh for the
whole page, reads the worktrees from git, and answers `GET /api/work`
with the groups. The web page is a copy of the Inbox page's shape: the
words in a pure `web/src/work.ts` with node tests, the markup in
`web/src/pages/WorkPage.vue` after the canvas's F1 artboard.

**Tech Stack:** Go 1.25 (`net/http`, `os/exec` for `git worktree list`),
go-git through `internal/gitbugio`, Vue 3 + Quasar + vue-router, node:test
through tsx. No new dependency.

**Spec:** `docs/prd/dev-cycle.md`, section "M3: the Work page" and the
Decisions rows "The Work page", "Whose turn", "Local first", "The
retro". The canvas's F1 artboard (`project/F1-work.dc.html` in
https://claude.ai/artifact/FSf4omtJyvbSFSa6bkBwQv).

**Base:** `main` at 3450d96 (M2 merged, the SSH-signed fix, prd/release),
branch `prd/dev-cycle`.

## Global Constraints

- Work on `prd/dev-cycle` only; never push, never tag. A human runs `wtx
  land prd/dev-cycle`.
- Before every commit: `GOCACHE=$TMPDIR/go-build make ci` and `cd web &&
  npm run typecheck && npm test`. Commits end with `Co-Authored-By: Claude
  Fable 5.1 <noreply@anthropic.com>`.
- The page is `/work`, the API `GET /api/work`; the groups are computed
  server-side; the Work page never calls GitHub once per issue (one
  `refreshPulls` for the page, only in-review-or-later issues with a
  pushed branch, at most every `pullsEvery`).
- The viewer is the server's identity (`--me`, else the identity's
  login). A gate whose role is unset or names nobody is anyone's: Team
  lists it under Needs you, Mine does not.
- An issue appears in exactly one of Needs you and Waiting on others; a
  done issue in neither.
- Words from the spec, verbatim: the lead "Human time is the bottleneck.
  Agents wait here, not in a terminal."; the groups "Needs you", "Agents",
  "Waiting on others", "To send", "Since the last retro"; the actions
  "Review", "Answer", "Read the plan", "Low", "Normal", "Structural",
  "Send all", "Check first", "Open the retro".
- Fixtures name nobody real: Person A/B/C, logins alice/bob/carol/dan/erin.
- Ponytail: a shortcut with a ceiling carries a `ponytail:` comment naming
  the ceiling and the upgrade path.
- `config.go` gained `[release]` on main: re-read it before touching it
  (M3 adds no config).

## Review Focus

1. A viewer without an identity or a login: `/api/work` answers 409
   `no-identity` / `no-login` like the inbox and the page says what the
   Inbox page says, instead of listing everything as nobody's. Task 3's
   `TestWorkNeedsWhoYouAre`.
2. A worktree on a detached head or a branch naming no open issue: still
   a row, with the head or the branch and no issue, never a crash or a
   wrong issue. Task 1's `TestWorktrees` and Task 3's `TestWorkAgents`.
3. An issue whose gate waits for the viewer and that also carries an
   agent's question: one row, under Needs you, with Answer. Task 2's
   `TestGroupQuestionWins`.
4. A question the viewer already answered (a comment by the viewer after
   it): no Answer row. Task 2's `TestQuestionAnswered`.
5. No `[sprints]`, or today before the first sprint: the retro line says
   there is no current sprint, the rest of the page is intact. Task 3's
   `TestWorkWithoutSprints`.

---

### Task 1: the worktrees, from git

**Files:**
- Modify: `internal/gitbugio/code.go`
- Test: `internal/gitbugio/code_test.go`

**Interfaces:**
- Produces:

```go
// Worktree is a linked worktree of the repository (git worktree list).
type Worktree struct {
	Path   string `json:"path"`
	Head   string `json:"head"`
	Branch string `json:"branch,omitempty"` // "" when detached
}
// Worktrees lists the linked worktrees, the main one left out.
func (b *Backend) Worktrees() ([]Worktree, error)
```

Parses `git worktree list --porcelain` through `b.git`: blocks separated
by a blank line, lines `worktree <path>`, `HEAD <sha>`, `branch
refs/heads/<name>` or `detached`; `bare` blocks and the first block (the
main worktree) are skipped. `Branch` is the name without `refs/heads/`.

- [ ] **Step 1:** In `code_test.go`, `TestWorktrees`: on the test repo
  (`newRepo` or whatever helper the file uses), `git worktree add
  <tmp>/wt fix/1-x` on a new branch and `git worktree add --detach
  <tmp>/d`; `Worktrees()` returns two entries: `{Path: wt, Branch:
  "fix/1-x", Head: <the branch's sha>}` and `{Path: d, Branch: ""}`, the
  main worktree absent. A repo with no linked worktree returns an empty
  slice and no error.
- [ ] **Step 2:** Run: `GOCACHE=$TMPDIR/go-build go test ./internal/gitbugio -run Worktrees`. Expected: FAIL, `Worktrees` undefined.
- [ ] **Step 3:** Implement `Worktrees` in `code.go`.
- [ ] **Step 4:** Run the test. Expected: PASS.
- [ ] **Step 5:** Commit `feat(gitbugio): the linked worktrees`.

### Task 2: the rows and the groups, pure

**Files:**
- Create: `internal/work/work.go`
- Test: `internal/work/work_test.go`

**Interfaces:**
- Consumes: `cycle.Cycle`, `cycle.Waits` (M1), `gitbugio.Thread`,
  `gitbugio.Event.Meta`.
- Produces:

```go
package work

// Row is one open issue's cycle, slimmed for a list.
type Row struct {
	Issue    gitbugio.Issue `json:"issue"`
	Step     int            `json:"step"`
	Of       int            `json:"of"`
	Name     string         `json:"name"`
	Risk     string         `json:"risk,omitempty"`
	Done     bool           `json:"done"`
	Waits    cycle.Waits    `json:"waits"`
	Mine     bool           `json:"mine"`
	Actions  []string       `json:"actions,omitempty"`
	Since    time.Time      `json:"since"`    // when the gate started waiting
	Involved bool           `json:"involved"` // the viewer's issue
	Question *Question      `json:"question,omitempty"`
	Branch   string         `json:"branch,omitempty"` // the evidence branch (agents match it)
}

// Question is an agent's question to the viewer: a comment with ask
// metadata, else the newest comment ending in ? the viewer hasn't
// answered.
type Question struct {
	ID   string          `json:"id"`
	Body string          `json:"body"`
	By   gitbugio.Person `json:"by"`
	At   time.Time       `json:"at"`
}

// Groups is Needs you and Waiting on others, oldest wait first.
type Groups struct {
	Needs   []Row `json:"needs"`
	Waiting []Row `json:"waiting"`
}

func RowOf(th gitbugio.Thread, c cycle.Cycle, me inbox.Me) Row
func Group(rows []Row, me string, team bool) Groups
func QuestionOf(th gitbugio.Thread, me inbox.Me) *Question
func Since(th gitbugio.Thread) time.Time
func Involved(th gitbugio.Thread, me inbox.Me) bool
```

Rules, as the spec and F1 word them:

- `RowOf` copies the cycle's fields, `Branch` from `c.Evidence.Branch`,
  `Involved`, `Question`, and `Since`: the question's `At` when there is
  one, else `Since(th)`.
- `Since`: the `At` of the thread's last event that changes the gate: any
  event whose kind isn't `comment`, or a comment with metadata (a
  sign-off, an approval, a run, a check); the creation otherwise.
  `// ponytail: the last state change, not the gate's own start; the step
  history (replaying the events through Compute) would give that.`
- `Involved`: the viewer is the author (`me.Is(th.Author)` or login
  equal, case-insensitive), or any label of `th.AllLabels()` is
  `<field>:<me.Login>` (assignee, pm, reviewer…).
- `QuestionOf`: among comments by someone else (`!me.Is(author)` and
  login not the viewer's), the newest with `Meta["ask"] != ""`, else the
  newest whose trimmed body ends with `?`; nil when a comment by the
  viewer is later than it (answered). `// ponytail: nothing marks an
  identity as an agent yet, so any other person's question counts; M5
  names the agent on the worktree.`
- `Group`: skips `Done` rows. A row needs the viewer when it has a
  `Question`, or `Waits.Login` equals `me` (case-insensitive), or `team`
  and `Waits.Anyone`. Otherwise it waits on others when `team` or
  `Involved`. Both groups sorted by `Since` ascending, ties by the
  issue's id.

- [ ] **Step 1:** Write the failing tests in `work_test.go` with a
  thread helper `th(author, labels, events...)` and a cycle helper
  `at(step, login string, anyone bool)`:

```go
func TestGroupMineAndTeam(t *testing.T)      // alice's gate → Needs; anyone's gate: Team Needs, Mine neither unless Involved → Waiting; bob's gate on alice's issue → Waiting; bob's gate on carol's issue → Mine nothing, Team Waiting; a Done row → nowhere; order by Since
func TestGroupQuestionWins(t *testing.T)     // a row with a Question whose gate waits for bob → Needs, once
func TestQuestionAsk(t *testing.T)           // ask metadata by bob → the question, even without a ?; the viewer's own ask → nil
func TestQuestionMark(t *testing.T)          // newest bob comment ending in ? → it; a later carol comment without ? doesn't hide it
func TestQuestionAnswered(t *testing.T)      // alice comments after bob's question → nil
func TestSinceLastStateChange(t *testing.T)  // created, comment, labels, comment → the labels' At; a signoff:status comment after → its At
func TestInvolved(t *testing.T)              // author; assignee:alice; pm:alice; none → false
```

- [ ] **Step 2:** Run: `GOCACHE=$TMPDIR/go-build go test ./internal/work`. Expected: FAIL, package missing.
- [ ] **Step 3:** Implement `work.go`.
- [ ] **Step 4:** Run the tests. Expected: PASS.
- [ ] **Step 5:** Commit `feat(work): the rows and the groups of the Work page`.

### Task 3: `GET /api/work`

**Files:**
- Create: `internal/server/work.go`
- Modify: `internal/server/cycle.go` (`cycleOf` split: `pullNames`)
- Modify: `internal/server/server.go` (the route, next to `/api/inbox`)
- Test: `internal/server/work_test.go`

**Interfaces:**
- Consumes: Task 1's `Worktrees`, Task 2's package, `inboxMe`,
  `refreshPulls`, `cycle.Gather`/`Compute`, `branches.Names`,
  `config.ResolveSprint`.
- Produces:

```go
// GET /api/work?who=me|team (default me)
type workResponse struct {
	Me      inbox.Me     `json:"me"`
	Needs   []work.Row   `json:"needs"`
	Waiting []work.Row   `json:"waiting"`
	Agents  []workAgent  `json:"agents"`
	ToSend  []sendLine   `json:"to_send"` // newest first, at most 20
	Pending int          `json:"pending"` // every change the forges wait for
	Retro   retroSummary `json:"retro"`
}
type workAgent struct {
	Path   string          `json:"path"`
	Branch string          `json:"branch"` // the head's short sha when detached
	Issue  *gitbugio.Issue `json:"issue,omitempty"`
	Step   int             `json:"step,omitempty"`
	Name   string          `json:"name,omitempty"`
	State  string          `json:"state"` // "Waits for you", "Waits for bob to review the PR", "No issue"
	At     time.Time       `json:"at"`    // the last commit
}
type sendLine struct {
	Issue gitbugio.Issue `json:"issue"`
	What  string         `json:"what"` // "a comment", "the status", "the risk", "the title", "closed", "reopened", "a new issue"
	At    time.Time      `json:"at"`
}
type retroSummary struct {
	Sprint      string    `json:"sprint,omitempty"` // "" when none is current
	Since       time.Time `json:"since,omitempty"`
	Corrections int       `json:"corrections"`
}
func (s *apiServer) handleWork(w http.ResponseWriter, r *http.Request)
func (s *apiServer) pullNames(th gitbugio.Thread, stages config.Stages) ([]string, error) // what cycleOf asked for, now shared
```

`handleWork`: `inboxMe` (409s as the inbox); `ListIssues` → open ones →
`Threads`; `Inherited = cfg.RolesFor(labels)` per thread; the union of
`pullNames` over the threads → one `refreshPulls`; then per thread
`cycle.Gather(s.b, cfg, th, me.Login, &s.pulls)`, `Compute`, `RowOf`;
`work.Group(rows, me.Login, r.URL.Query().Get("who") == "team")`.
`// ponytail: Gather lists the branches once per issue; cache Branches()
per request when a board with hundreds of open issues shows it.`

Agents: one per `Worktrees()` entry; the issue is the first open issue
whose row's `Branch` equals the worktree's branch, else the first that
`branches.Names(branch, issue)`; `State` from that row: "Waits for you"
when `Mine && !Waits.Anyone`, else "Waits for anyone to <what>" /
"Waits for <login> to <what>"; "No issue" without one; `At` from
`ReadCommit(head).Date`.

ToSend: every event of every thread with `len(Pending) > 0`, `What` by
kind: `created` → "a new issue", `comment` → "a comment", `title` →
"the title", `status` → "closed"/"reopened", `labels` → "the <field>"
from the first added or removed label's prefix before `:` ("the
labels" without one). Newest first, `Pending` the total, `ToSend` the
first 20.

Retro: `config.ResolveSprint(cfg.Sprints, "@current", now)`; on error
`Sprint` stays ""; else `Corrections` counts comment events with
`Meta["correction"] != ""` and `At` ≥ the sprint's start over the open
threads. `// ponytail: open issues only; M7 reads closed ones too.`

- [ ] **Step 1:** Write the failing tests in `work_test.go`, on
  `gitbugiotest.New` with `cycleConfig(map[string]string{"pm": "bob"},
  config.Cycle{Risk: "pm"})` and `newTestServer` (viewer `guilbep`):

```go
func TestWorkGroups(t *testing.T)         // issue A at ready, no risk (bob's gate) → Mine: Waiting (guilbep is author), Team: Waiting; issue B at ready with risk:low, assignee:guilbep (guilbep's gate) → Needs; issue C closed → nowhere; an issue is in one group only; ?who=team adds nothing here
func TestWorkAnyonesGate(t *testing.T)    // no [roles]: ready without risk is anyone's → Mine: Waiting on others (author), Team: Needs
func TestWorkQuestion(t *testing.T)       // bob's "Which tolerance?" comment (CommentAs) on an issue whose gate is bob's → Needs with question.body; after guilbep's reply (AddComment) → Waiting
func TestWorkNeedsWhoYouAre(t *testing.T) // no identity → 409 no-identity (the inbox test's shape)
func TestWorkAgents(t *testing.T)         // on gateRepo: `git worktree add` on fix/2518-x → one agent row with the issue, step, state; a detached worktree → branch = short sha, "No issue"
func TestWorkToSend(t *testing.T)         // with gitbugiotest.NewForge: a comment pending → to_send [{what: "a comment"}], pending 1; without a forge: empty, 0
func TestWorkWithoutSprints(t *testing.T) // retro.sprint "" and corrections 0; with a sprint and an AddCommentMeta correction → 1
func TestWorkAsksGitHubOnce(t *testing.T) // prRepo at in-review: one /api/work → GitHub asked for fix/2518-x once; a second /api/work within pullsEvery → not again
```

- [ ] **Step 2:** Run: `GOCACHE=$TMPDIR/go-build go test ./internal/server -run Work`. Expected: FAIL, 404.
- [ ] **Step 3:** Implement `work.go`, split `pullNames` out of
  `cycleOf`, register `GET /api/work`.
- [ ] **Step 4:** Run `go test ./internal/server`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(work): GET /api/work, the groups from every open issue's cycle`.

### Task 4: the words of the Work page

**Files:**
- Modify: `web/src/api/board.ts` (`WorkRow`, `WorkAgent`, `SendLine`,
  `WorkResponse`, `getWork`)
- Create: `web/src/work.ts`
- Test: `web/src/work.test.ts` (added to `package.json`'s `test` script)

**Interfaces:**
- Consumes: the JSON of Task 3, typed:

```ts
export interface WorkRow { issue: Issue; step: number; of: number; name: string; risk?: string; done: boolean; waits: CycleWaits; mine: boolean; actions?: string[]; since: string; involved: boolean; question?: { id: string; body: string; by: Person; at: string }; branch?: string }
export interface WorkAgent { path: string; branch: string; issue?: Issue; step?: number; name?: string; state: string; at: string }
export interface SendLine { issue: Issue; what: string; at: string }
export interface WorkResponse { me: { id: string; login: string }; needs: WorkRow[]; waiting: WorkRow[]; agents: WorkAgent[]; to_send: SendLine[]; pending: number; retro: { sprint?: string; since?: string; corrections: number } }
export function getWork(who: 'me' | 'team'): Promise<WorkResponse>
```

- Produces, in `work.ts`:

```ts
export function ago(iso: string, now?: Date): string   // "just now" under a minute, "12 min", "2 h", "3 d", "5 w"
export function waitsFor(r: WorkRow): string           // the Waits for cell
export type Action = { kind: 'risk' } | { kind: 'answer' } | { kind: 'open'; label: string; cycle: boolean }
export function actionFor(r: WorkRow): Action
export function stepLabel(r: { step: number; name: string }): string  // "06 Review"
export function sendWords(pending: number): string     // "3 changes", "1 change", "Nothing to send"
export function retroWords(r: WorkResponse['retro']): string // "5 agent corrections flagged in reviews.", "1 agent correction …", "No correction flagged this sprint.", "No current sprint: [sprints] is the retro's clock."
```

`waitsFor`, F1's column: a question → `Your answer: "${body, one line, cut
at 80}"`; then by `waits.what`: "set the risk class" → "A risk class";
"approve the plan" → "Your read of the plan"; "approve the plan PR on
GitHub" → "Your approval of the plan PR"; "review the PR" → "A second
human" when `risk === 'structural'`, else "Your review"; "sign off X" →
"Your sign-off: X"; "add acceptance criteria" → "Acceptance criteria";
"write the failing test" → "The failing test"; "commit and open the PR"
→ "The PR"; "move the status to X" → "The status: X". A row the viewer
doesn't own (`!mine || waits.anyone`) says `${login} to ${what}` or
`Anyone to ${what}`, with `login` capitalised nowhere (logins).

`actionFor`: a question → answer; `actions` has `set-risk` → risk;
`approve-plan` → open "Read the plan" (cycle); `review` → open "Review"
(cycle); `signoff` → open `Sign off ${status from waits.what}` (cycle);
else open "Open" (not cycle).

- [ ] **Step 1:** Write the failing tests in `work.test.ts`: one `test`
  per function above, `ago` with a fixed `now`, `waitsFor` for the
  question, the structural review, a sign-off, bob's gate and anyone's
  gate, `actionFor` for the five kinds, `retroWords` for the four
  sentences.
- [ ] **Step 2:** Run: `cd web && npx tsx --test src/work.test.ts`. Expected: FAIL, module missing.
- [ ] **Step 3:** Implement `work.ts` and the types; add the test file to `package.json`.
- [ ] **Step 4:** Run: `cd web && npm run typecheck && npm test`. Expected: PASS.
- [ ] **Step 5:** Commit `feat(work): the words of the Work page`.

### Task 5: the Work page, the route and the header item

**Files:**
- Create: `web/src/pages/WorkPage.vue`
- Modify: `web/src/router/routes.ts` (`/work`, before `/inbox`)
- Modify: `web/src/components/AppHeader.vue` (a `Work` link before
  Insights, measured in `fixed` like the other two)
- Modify: `web/src/components/IssuePanel.vue` (the initial `view` is
  `cycle` when the route's query says `view=cycle`)

**Interfaces:**
- Consumes: Task 4's words, `getWork`, `patchIssue`, `startForgeSync`,
  `IssuePanel` with `:at` (the Inbox passes `first`), `getViews` for
  the header, `onLive`/`debounced`, `me` from `@/me`.

The page, F1 top to bottom, in `main.gb-page` with a main column and
an aside (`flex: 999 1 45rem` / `flex: 1 1 17rem`, stacked under 60rem):

- eyebrow "Work", `h1.h3` "What needs a person", the lead, the Mine |
  Team segment (`.gb-seg`, `aria-pressed`, reloads with `who`).
- Needs you (count): a table Issue | Step | Waits for | Since | action.
  Issue: `#number · risk` muted mono over the bold title linking to the
  issue (the panel, `?issue=`); Step: the ticks (`.gb-prog`, copied
  from `IssueCycle.vue`) and `stepLabel`; Since: `ago(since)`; action:
  risk → three `epfl-btn-secondary epfl-btn-sm` buttons that `patchIssue(id,
  {risk})` and reload; answer → "Answer" primary, opens the panel at the
  question's comment (`:at="question.id"`); open → a primary link to
  `/issues/<id>` (`?view=cycle` when `cycle`).
- Agents (N worktrees): branch (mono) | step | state, "Last commit
  ${ago(at)}" muted. Absent when there is no worktree.
- Waiting on others (count): Issue | step | `waitsFor` | since.
- When the three are empty: "Nothing waits for you." (Team: "Nothing
  waits for anyone.") with the hint "Every open issue is at a step
  someone else owns, or nothing is open."
- Aside "To send": `sendWords(pending)`, the lines `#n what`, buttons
  "Send all" (`startForgeSync(false, 'send')`) and "Check first"
  (`startForgeSync(true)`), then a link to `/sync`. "Since the last
  retro": `retroWords`. `// ponytail: "Open the retro" waits for M7's
  route.`
- The identity errors as the Inbox page words them (`no-identity`,
  `no-login`).
- Reloads on `issues` and `sync` live events, debounced; j/k not needed.

- [ ] **Step 1:** The page, the route, the header link, the panel's
  query. Run `cd web && npm run typecheck && npm test`. Expected: PASS
  (the page has no unit test; Task 7 checks it in the browser).
- [ ] **Step 2:** `cd web && npm run build` and a look at the fixture
  (Task 7's) at 1440px against F1: the sections, the columns, the
  buttons; fix what differs.
- [ ] **Step 3:** Commit `feat(work): the Work page`.

### Task 6: the docs and the PRD

**Files:**
- Modify: `docs/configuration.md` (Cycle section: the Work page, the
  Mine/Team rule, the `ask` metadata)
- Modify: `README.md` (the paragraph naming the Cycle view)
- Modify: `docs/prd/dev-cycle.md` (M3: "Done: plan in
  `dev-cycle-m3-plan.md`." and "Calls taken building it:")

- [ ] **Step 1:** Write them. Calls taken: a question is any other
  person's (no agent identity yet); Since is the last state change;
  Agents show the worktree and the last commit, wtx's state is M5's; To
  send lists the forge-pending events, the git remotes stay on the Sync
  page; "Open the retro" waits for M7; corrections counted on open
  issues.
- [ ] **Step 2:** `make ci`, web typecheck + test. Commit `docs(work): the Work page`.

### Task 7: verification and hand-off

- [ ] **Step 1:** `GOCACHE=$TMPDIR/go-build make ci` and `cd web && npm
  run typecheck && npm test` green.
- [ ] **Step 2:** The browser check on the M2 fixture `$TMPDIR/fx`
  (rebuild it if gone: branch `dev`, the co2 statuses, `[branches] base
  = "dev"`, unsigned commits, `--me alice`), extended for F1: an issue
  at `ready` without a risk and `pm:alice` (the risk buttons); one at
  `in-progress` with a plan branch and `lead-dev:alice` ("Read the
  plan"); one at `in-review` with a comment by bob ending in `?`
  ("Answer"); one at `qa` whose sign-off role is bob (Waiting on
  others); a linked worktree on an evidence branch. One server per
  repository: `$TMPDIR/git-board serve --addr localhost:8099 --me
  alice` from the fixture. Playwright: `#/work` at 1440px and 900px
  against F1, Mine and Team; press Normal on the risk row and see it
  move; press Answer and see the panel at the comment. Screenshots
  under `.playwright-mcp/`, never committed.
- [ ] **Step 3:** Fix what the check shows, commit, `git log --oneline
  main..` lists the commits, no push. Say it is ready for `wtx land
  prd/dev-cycle`.

## Self-review notes

- Spec coverage: `/work` and the header item ✓ (5), Mine/Team ✓ (2, 5),
  Needs you with the five actions ✓ (3, 4, 5), Answer rows from `ask` or
  the `?` ✓ (2), Agents per worktree ✓ (1, 3, 5), Waiting on others ✓
  (2, 5), To send with Send all / Check first ✓ (3, 5), Since the last
  retro ✓ (3, 4, 5), `GET /api/work` server-side groups in the inbox's
  shape ✓ (3), one group per issue ✓ (2, 3), empty page says so ✓ (5),
  never GitHub once per issue ✓ (3), F1 at 1440/900 ✓ (7).
- Not in M3, said in Task 6: wtx's session state and the agent's name
  on the Agents rows (M5), "Open the retro" (M7), the git remotes in To
  send (the header and Sync say them), a Work badge in the header.

wtx-effort: high
