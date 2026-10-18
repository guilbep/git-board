# Dev cycle M1: the step, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** With co2-calculator's nine statuses, an issue walks all seven
steps from status plus evidence, the Issue view says "Step N of 7 · <step>,
waits for <who> to <what>", and the person whose turn it is gets the card
(Set the risk, Approve the plan, Review, Sign off) and nobody else does.

**Architecture:** `internal/cycle` has two halves. `Compute(in Input)
Cycle` is pure: the seven rules, the gate texts, whose turn, in a table
test. `Gather` fills `Input` from the backend: the thread (status label,
description, comments with metadata), the linked branches (`internal/
branches.Linked`), four git calls in `gitbugio/code.go` (`diff
--name-only`, `log --diff-filter=A`, `grep -l`, `show`), and the server's
per-branch PR cache. Comments with metadata use git-bug's `AddCommentRaw`
through a new `Backend.AddCommentMeta`; the thread's `Event` gains `Meta`.
The web gets one component, `IssueCycle.vue`, and one pure module,
`cycle.ts`, for the words.

**Tech Stack:** Go (`os/exec` git, `regexp`), git-bug fork
`v0.11.1-sync.5` (`cache.BugCache.AddCommentRaw`, unchanged since sync.4),
Vue 3, `node:test`.

**Spec:** `docs/prd/dev-cycle.md`, "Decisions" and "M1: the step".

**Base:** main at `02c3152` or later, after the one-binary refactor: the
server is `internal/server` (package `server`), `git board serve` runs it,
the fork pin is `v0.11.1-sync.5`. Tasks 1 to 4 touch no file that refactor
changed; Tasks 5 to 9 need it merged.

## Global Constraints

- The step is computed, never stored. No label, no metadata holds it.
- Statuses are the project's own: `cfg.Fields["status"].Options`, in
  order; the closing status is the last option (as `cmd/git-board/close.go`
  has it). git-bug's open/closed is separate (`Issue.Status`).
- **The status is the floor.** `ready` spans steps 1–3, `in-progress` 4–5,
  `in-review` 6, a validation status 7. Evidence moves the step within the
  stage, never below it. Evidence that disagrees with the status is
  `note`, never another step.
- Seven steps per issue. Done is step 7 with `done: true`, when the status
  is the closing one or the issue is closed.
- Gate texts, verbatim from the deck's table: 1 `No issue, no agent`; 2
  `Class written on the issue`; 3 `Structural: plan PR approved first`
  (normal: `Plan approved by the plan role`, low: `Skipped, low risk`); 4
  `Every bug fix ships its test`; 5 `A human commits and opens the PR`; 6
  `Structural: a second human` (else `A human who can explain the diff`); 7
  `Green codes are not proof`.
- The risk field is `risk`, options `low`, `normal`, `structural`, single.
  M1 does not add a `[cycle]` key to rename it (YAGNI; noted in the PRD).
- Roles are logins (`[roles]`, `internal/config/roles.go`): the gate's
  person is the issue's own `<role>:x` label, else the project's. The
  viewer is `apiServer.me`, else `Backend.MyLogin()`; compared with
  `strings.EqualFold`. A gate whose role is unset or names nobody is
  anyone's: `anyone: true`, `mine: true`.
- Comment metadata keys are exactly: `plan:approve`, `plan:commit`,
  `signoff:status`, `signoff:commit`, `signoff:why`, `verify:run`,
  `verify:commit`, `verify:criterion`. `plan:approve`, `verify:run` hold
  `"1"`. Keys starting `sync:` are never exposed to the web.
- `POST …/cycle/{action}` refuses with 403 `{"error": "waits for the
  <role>"}` when the viewer is not the gate's person, and the gate has one.
- No live GitHub in `make ci`; the PR is read from GitHub only for an
  issue at `in-review` or later with a branch on origin, at most every 5
  minutes per branch, and a failure keeps the last answer (offline the step
  still computes).
- `../co2-calculator` is read-only: its statuses and plan front matter are
  copied into fixtures, never read by a test.
- No real person's name in fixtures: Person A, B, C, logins `alice`,
  `bob`, `carol` are already the repo's convention in tests.
- Everything git-bug writes goes through `Backend.write` (one commit per
  action). Every server write goes through `s.beginWrite(w)`.
- Commits are not made by the agent (GPG): stage everything at the end and
  give the commit messages. `GOCACHE=$TMPDIR/go-build` for Go tests in the
  sandbox; server tests bind `httptest` ports and may need a real terminal.

## Review Focus

1. **An issue with no status label at all** (a fresh git-bug issue, no
   `[fields.status]`): stage "before", step 1, no crash on an empty options
   list. (Task 3: `TestComputeNoStatus`.)
2. **A description with "Acceptance criteria" but no list under it**:
   zero criteria, step 1 at `ready`. (Task 3: `TestCriteriaCount`, the
   `heading, no list` row.)
3. **Two branches naming the issue plus a `plan/` branch**: the plan branch
   is never the evidence branch, and the newer head of the other two is.
   (Task 4: `TestEvidenceBranch`.)
4. **A sign-off by the wrong person over the API**: 403, no comment, no
   status move. (Task 6: `TestCycleSignoffRefused`.)
5. **A sign-off at the last validation status**: the status becomes the
   closing one and the issue is closed in the same request; the cycle
   answers `done`. (Task 6: `TestCycleSignoffCloses`.)

---

### Task 1: `[cycle]` in the config, with guessed stages

**Files:**
- Modify: `internal/config/config.go` (`raw`, `Config`, `parse`)
- Create: `internal/config/cycle.go`, `internal/config/cycle_test.go`
- Modify: `internal/config/testdata/git-board.toml` (add a `[cycle]`
  table only if a test needs the file; prefer inline TOML)

**Interfaces:**
- Produces:

```go
// Cycle is the [cycle] table: the project's statuses as the loop's stages,
// where plans live, which files are tests, and who holds each gate.
type Cycle struct {
	Ready      string            `toml:"ready"`
	InProgress []string          `toml:"in-progress"`
	InReview   []string          `toml:"in-review"`
	Validation []string          `toml:"validation"`
	Plans      string            `toml:"plans"` // default "docs/src/implementation-plans"
	Tests      []string          `toml:"tests"` // default *_test.go, *.test.ts, *.spec.ts, test_*.py, tests/**
	Risk       string            `toml:"risk"`  // the role that sets the risk class
	Plan       string            `toml:"plan"`  // the role that approves the plan
	Review     string            `toml:"review"`
	Signoffs   map[string]string `toml:"signoffs"` // validation status → role
}

// Stages is the [cycle] table with every default filled: what the server
// computes with, and what Settings shows as guessed.
type Stages struct {
	Before     []string // statuses before ready
	Ready      string
	InProgress []string
	InReview   []string
	Validation []string // in the status options' order
	Closing    string   // the last status option, "" without options
	Plans      string
	Tests      []string
	Guessed    bool // the stages came from the status names, not the file
}

func (c *Config) Stages() Stages
// StageOf is "before", "ready", "in-progress", "in-review", "validation",
// "done" or "" (unknown status) for a status value.
func (s Stages) StageOf(status string) string
```

Guessing (`guessStages(options []string) Stages`), on the words of each
option split at `-`, `_`, ` ` and lower-cased, in this order per option:
the last option is `Closing`; a word in `validation, qa, uat, acceptance,
test, tests, metier, verify, staging` → validation; `review, reviewing,
pr` → in-review; `progress, doing, wip, dev, development, implementing,
coding` → in-progress; `ready, todo, planned, selected` → ready (the first
such option; a later one is "before" too: never two readies); anything
else → before. A key set in the file replaces that part of the guess
(`Guessed` is false only when all four stage keys are set).

- [ ] **Step 1: Write the failing tests** in `cycle_test.go`:
  - `TestGuessStages`, a table over `options → Stages`:
    co2's nine → `Before [backlog comite]`, `Ready ready`, `InProgress
    [in-progress]`, `InReview [in-review]`, `Validation [ready-for-validation
    qa tests-metier]`, `Closing validated`; git-board's own `[draft backlog
    ready in-progress in-review done]` → `Validation` nil, `Closing done`;
    `[todo doing review done]` → `Ready todo`, `InProgress [doing]`; `nil`
    → every field empty, `Closing ""`.
  - `TestCycleFromFile`: `parse` of the PRD's M1 TOML (the `[fields]
    risk` and `[cycle]` block) plus co2's `[fields.status]`: `Stages()`
    returns the file's lists, `Guessed false`, `Plans
    "docs/src/implementation-plans"`, `Tests` the five defaults,
    `Cycle.Signoffs["qa"] == "pm"`; `md.Undecoded()` is empty (so `Check`
    gives no "doesn't know this key" warning).
  - `TestStageOf`: on co2's stages, `qa → validation`, `validated → done`,
    `comite → before`, `nope → ""`.
  - `TestCycleRolesChecked`: `[cycle] risk = "pm"` with no `pm` in
    `[roles]` parses fine (anyone's turn); `risk = "p m"` errors
    `[cycle] risk: a role name, not "p m"` (reuse `roleName`).
- [ ] **Step 2: Run them to see them fail.**
  Run: `GOCACHE=$TMPDIR/go-build go test ./internal/config -run 'Guess|Cycle|StageOf'`
  Expected: FAIL, `Cycle` undefined.
- [ ] **Step 3: Implement**: add `Cycle Cycle \`toml:"cycle"\`` to `raw`
  and `Config`, copy it in `parse`, `checkCycle(c Cycle)` beside
  `checkRoles`, `cycle.go` with the types, `Stages()`, `StageOf`,
  `guessStages` and the `defaultTests` slice.
- [ ] **Step 4: Run them to see them pass**, then `go test ./internal/config`.
- [ ] **Step 5:** Note the commit message `feat(cycle): [cycle] in .git-board.toml, stages guessed from the statuses`.

---

### Task 2: comments with metadata, and git for the evidence

**Files:**
- Modify: `internal/gitbugio/thread.go` (`Event.Meta`, `AddCommentMeta`),
  `internal/gitbugio/thread_test.go`
- Modify: `internal/gitbugio/code.go` (a `git` helper, four readers),
  create `internal/gitbugio/code_test.go`

**Interfaces:**
- Produces:

```go
// Event: new field
Meta map[string]string `json:"meta,omitempty"` // a comment's metadata, without the sync engine's sync:* keys

// AddCommentMeta is AddComment with operation metadata, as the user
// identity: what the cycle's sign-offs, approvals and runs are.
func (b *Backend) AddCommentMeta(idPrefix, body string, meta map[string]string) (string, error)

// ChangedFiles is `git diff --name-only base...branch`: the files branch
// changed since it left base. nil, nil when b has no git directory.
func (b *Backend) ChangedFiles(base, branch string) ([]string, error)
// FirstCommitAdding is the first commit of base..branch that added path, "".
func (b *Backend) FirstCommitAdding(base, branch, path string) string
// GrepFiles lists the files under dir at ref whose content matches the
// extended regexp (git grep -l -E), paths relative to the root.
func (b *Backend) GrepFiles(ref, dir, regexp string) ([]string, error)
// ReadFile is `git show ref:path`.
func (b *Backend) ReadFile(ref, path string) ([]byte, error)
```

All four run `git --git-dir b.dir …` through one `func (b *Backend)
git(args ...string) ([]byte, error)` that `ChangesIn` is moved onto;
`b.dir == ""` (a `NewForRepo` backend) answers `nil, nil` / `""`.

- [ ] **Step 1: Write the failing tests**:
  - In `thread_test.go`, `TestCommentMeta`: `AddCommentMeta(id, "Signed
    off qa at 4f2e9c1", {"signoff:status": "qa", "signoff:commit":
    "4f2e9c1"})`, then `Thread(id)`: the comment event's `Meta` equals
    that map; a plain `AddComment`'s event has `Meta` nil; an imported
    issue's events carry no `sync:` key (use `gitbugiotest.NewImportedIssue`
    and check the `created` event).
  - `code_test.go`, `TestGitReaders`: `git init -b dev` in a temp dir
    (copy `branches_test.go`'s `git(t, dir, …)` helper into the package),
    commit `docs/plans/42-x.md` on `dev` with front matter `issue: 42`,
    branch `fix/42-y` adding `a_test.go` then `b.go`; `gitbugio.Open(dir)`:
    `ChangedFiles("dev", "fix/42-y") == [a_test.go b.go]`;
    `FirstCommitAdding("dev", "fix/42-y", "a_test.go")` is the first
    branch commit's hash; `GrepFiles("dev", "docs/plans", "^issue: 42$")
    == [docs/plans/42-x.md]`, `GrepFiles("dev", "docs/plans", "^issue:
    7$")` is empty with no error; `ReadFile("dev", "docs/plans/42-x.md")`
    starts with `---`; on `gitbugiotest.New(t)` every reader answers
    empty, no error.
- [ ] **Step 2: Run them to see them fail.**
  Run: `GOCACHE=$TMPDIR/go-build go test ./internal/gitbugio -run 'CommentMeta|GitReaders'`
- [ ] **Step 3: Implement.** `Meta` is filled in `event`'s `comment`
  closure from `on.ops[item.CombinedId()].AllMetadata()`, dropping keys
  with the `sync:` prefix, nil when empty. `AddCommentMeta` is `write`
  with `author, err := b.c.GetUserIdentity()` then
  `bug.AddCommentRaw(author, time.Now().Unix(), body, nil, meta)`.
  `git grep` exits 1 on no match: not an error.
- [ ] **Step 4: Run them to see them pass**, then `go test ./internal/gitbugio`.
- [ ] **Step 5:** Commit message `feat(cycle): comment metadata in the thread, and git readers for the evidence`.

---

### Task 3: `internal/cycle`, the computation

**Files:**
- Create: `internal/cycle/cycle.go` (types, `Compute`, criteria),
  `internal/cycle/cycle_test.go`

**Interfaces:**
- Produces:

```go
// Names of the seven steps, index 1..7.
var Steps = [...]string{"", "Intent", "Risk class", "Plan", "Tests first", "Code", "Review", "Verify"}

type Plan struct {
	Path   string `json:"path,omitempty"`
	Ref    string `json:"ref,omitempty"`    // the branch it was read from
	Status string `json:"status,omitempty"` // its front matter status
}

type Approval struct {
	By     string    `json:"by"`
	Commit string    `json:"commit,omitempty"`
	At     time.Time `json:"at"`
}

type TestFile struct {
	Path   string `json:"path"`
	Commit string `json:"commit,omitempty"` // the commit that first added it
}

type Signoff struct {
	Status string    `json:"status"`
	By     string    `json:"by"`
	Commit string    `json:"commit,omitempty"`
	Why    string    `json:"why,omitempty"` // an override's reason
	At     time.Time `json:"at"`
}

type Run struct {
	By     string    `json:"by"`
	Commit string    `json:"commit"`
	At     time.Time `json:"at"`
}

type Check struct {
	Index  int       `json:"index"`
	By     string    `json:"by"`
	Commit string    `json:"commit,omitempty"`
	At     time.Time `json:"at"`
}

// Input is everything Compute needs, gathered once per issue.
type Input struct {
	Stages    config.Stages
	Status    string   // the status field's value
	Closed    bool     // git-bug's closed
	Criteria  []string // the acceptance criteria, in order
	Risk      string
	Author    string   // login
	Assignees []string // logins
	// People is the login holding each gate for this issue, "" for
	// nobody: "risk", "plan", "review", and "signoff:<status>".
	People map[string]string
	Me     string // the viewer's login, "" unknown

	Branch       string // the evidence branch, "" none
	Head         string
	Base         string
	PlanBranch   string
	Plan         *Plan
	PlanApproval *Approval // the plan:approve comment, by the plan gate's person
	PlanPR       *ghpr.PR  // the plan branch's PR, structural
	Tests        []TestFile
	PR           *ghpr.PR
	PRAt         time.Time
	Signoffs     []Signoff
	Runs         []Run
	Checks       []Check
}

type Waits struct {
	Role   string `json:"role,omitempty"`  // "risk", "plan", "review", "assignee", "author", "signoff"
	Login  string `json:"login,omitempty"` // "" with Anyone
	What   string `json:"what"`            // "set the risk class", "approve the plan", "sign off qa"…
	Anyone bool   `json:"anyone"`
}

type Cycle struct {
	Step     int      `json:"step"` // 1..7
	Of       int      `json:"of"`   // 7
	Name     string   `json:"name"`
	Stage    string   `json:"stage"`
	Done     bool     `json:"done"`
	Gate     string   `json:"gate"`
	Risk     string   `json:"risk,omitempty"`
	Skipped  []int    `json:"skipped,omitempty"` // [3] for low risk
	Note     string   `json:"note,omitempty"`    // evidence that disagrees with the status
	Waits    Waits    `json:"waits"`
	Mine     bool     `json:"mine"`             // the viewer's turn (or anyone's)
	Actions  []string `json:"actions,omitempty"` // the card: set-risk, approve-plan, review, signoff, verify, check
	Evidence Input    `json:"evidence"`          // what it used; Me is omitted
}

func Compute(in Input) Cycle
// Criteria lists the items of the list under an "Acceptance criteria"
// heading (any level, or a bold line) in a Markdown description.
func Criteria(description string) []string
```

The rules, in `Compute`:
- stage = `in.Stages.StageOf(in.Status)`; `in.Closed` or stage `done` →
  step 7, `Done true`, `Name "Done"`, no `Waits`, no `Actions`.
- `before` or `""`: step 1. `ready`: step 1 if `len(Criteria) == 0`; else
  step 2 if `Risk == ""`; else step 3, with `Skipped [3]` and `Note "low
  risk, waits for the status in-progress"` when low, `Note "plan approved,
  waits for the status in-progress"` when approved.
- approved: `Plan.Status` in `accepted, in-progress, delivered` (the
  kit's lifecycle at or past accepted); or `PlanApproval != nil` (normal
  and low); or structural and `PlanPR` with `Decision APPROVED` or `State
  MERGED`.
- `in-progress`: step 4 when `len(Tests) == 0`, else 5. `Note "risk not
  set"` / `"plan not approved"` when so (the floor holds). A PR open on the
  branch: `Note "PR #N open, waits for the status in-review"`.
- `in-review`: step 6. `PR.State MERGED` → `Note "merged, waits for the
  status"`. `Gate` is the structural one when `Risk == "structural"`.
- `validation`: step 7. `Waits` is the sign-off for the current status.
  `Note "plan not marked delivered"` when `Plan != nil && Plan.Status !=
  "delivered"`; `"no plan file"` when `Plan == nil` and `Risk != "low"`.
- `Waits.Login` per step: 1 `Author` (role `author`, what `add acceptance
  criteria`); 2 `People["risk"]` (`set the risk class`); 3
  `People["plan"]` (`approve the plan`; structural: `approve the plan PR
  on GitHub`); 4, 5 the first assignee (role `assignee`; `write the failing
  test` / `commit and open the PR`); 6 `People["review"]` (`review the
  PR`); 7 `People["signoff:"+Status]` (`sign off <status>`). An empty
  login → `Anyone true`.
- `Mine`: `Anyone`, or `strings.EqualFold(Login, Me)` with both set.
- `Actions` when `Mine`: 2 `set-risk`; 3 `approve-plan` (not structural,
  not approved yet); 6 `review`; 7 `verify`, `check`, `signoff`.
  Step 1, 4 and 5 give none (the card shows nothing: those are edits and
  code).
- `Evidence` is `in` with `Me` blanked (`json:"-"` on `Me`).

`Criteria`: find the first line matching
`(?i)^\s*(#{1,6}\s*|\*\*)acceptance criteria:?(\*\*)?:?\s*$`; then the
following lines, skipping blank ones, while they match `^\s*([-*+]|\d+[.)])\s+(\[[ xX]\]\s*)?(.+)$`
→ group 3 trimmed; stop at the first other non-blank line.

- [ ] **Step 1: Write the failing tests**:
  - `TestWalk`: co2's stages (built with `config.Config{Fields:
    {"status": {Options: co2's nine}}}.Stages()`), `People {"risk":
    "alice", "plan": "bob", "review": "carol", "signoff:ready-for-validation":
    "bob", "signoff:qa": "alice", "signoff:tests-metier": ""}`, author
    `dan`, assignee `erin`. A table, one row per (risk class, situation),
    asserting `Step, Name, Waits.Login, Waits.What, Mine (Me "alice"),
    Note, Skipped, Done`:
    - `backlog` → 1, author dan.
    - `ready`, no criteria → 1. `ready`, criteria, no risk → 2, alice,
      `set the risk class`, Mine true, Actions `[set-risk]`.
    - `ready`, risk normal → 3, bob, Mine false. Same with
      `PlanApproval` → 3, Note `plan approved, waits for the status
      in-progress`. Same with `Plan.Status accepted` → approved too.
    - `ready`, risk low → 3, Skipped `[3]`, Note `low risk, …`.
    - `ready`, risk structural, `PlanPR {Decision: "APPROVED"}` →
      approved; `{Decision: "REVIEW_REQUIRED"}` → not, Waits.What
      `approve the plan PR on GitHub`.
    - `in-progress`, no tests → 4, erin, `write the failing test`;
      with `Tests [{a_test.go, c1}]` → 5; `in-progress` no criteria, no
      risk → 4 with Note `risk not set` (the floor).
    - `in-review` → 6, carol, `review the PR`; `Risk structural` → Gate
      `Structural: a second human`; `PR {State: "MERGED"}` → Note
      `merged, waits for the status`.
    - `qa` → 7, alice, `sign off qa`, Mine true, Actions `[verify check
      signoff]`; `tests-metier` → 7, Anyone true, Mine true.
    - `validated` → Done; `Closed true` at `qa` → Done.
    The three risk classes each cross steps 1–7 in the table (low skips 3).
  - `TestStatusAloneNeverTellsTwoFromThree`: two inputs at `ready` with
    criteria, differing only in `Risk` → steps 2 and 3.
  - `TestComputeNoStatus`: `Input{}` → step 1, no panic, `Waits.Anyone`.
  - `TestCriteriaCount`: table: `## Acceptance criteria` + three `- [ ]`
    items → 3 items with the texts; `**Acceptance criteria:**` + numbered
    list → 2; heading then a paragraph → 0; no heading → 0; list cut by a
    blank-then-paragraph → the items before it.
  - `TestCardShowsToRoleOnly`: at `qa`, Me `bob` → Mine false, Actions
    nil; Me `alice` → Mine true; `signoff:qa` set to `""` → Mine true for
    both.
- [ ] **Step 2: Run them to see them fail.**
  Run: `GOCACHE=$TMPDIR/go-build go test ./internal/cycle`
- [ ] **Step 3: Implement** `cycle.go`.
- [ ] **Step 4: Run them to see them pass.**
- [ ] **Step 5:** Commit message `feat(cycle): the step from the status and the evidence`.

---

### Task 4: gathering the evidence

**Files:**
- Create: `internal/cycle/gather.go`, `internal/cycle/gather_test.go`

**Interfaces:**
- Consumes: Task 1 `Config.Stages()`, `Config.Cycle`, `Config.RolesFor`;
  Task 2 readers and `Event.Meta`; `branches.Linked`, `branches.Suggest`
  (`internal/branches/branches.go:131,173`); `board.FieldValue`
  (`internal/board/board.go:238`); `Backend.ReadCommit`.
- Produces:

```go
// Pulls is the server's PR cache, read-only here.
type Pulls interface {
	// PR is the cached PR whose head is branch, and when it was read;
	// nil when none or never read.
	PR(branch string) (*ghpr.PR, time.Time)
}

// Gather fills an Input for the issue from git-bug, git and the PR cache.
// me is the viewer's login.
func Gather(b *gitbugio.Backend, cfg *config.Config, th gitbugio.Thread, me string, pulls Pulls) (Input, error)
```

How, in order:
- `th.Inherited = cfg.RolesFor(th.Labels)` first (the thread handler
  doesn't set it). `Status = board.FieldValue(th.Issue, "status")`,
  `Risk = FieldValue(…, "risk")`, `Assignees` from every `assignee:`
  label, `Author = th.Author.Login`, `Criteria = Criteria(created
  event's Body)`.
- `People`: for role names `cfg.Cycle.Risk`, `.Plan`, `.Review` and each
  `cfg.Cycle.Signoffs[status]`: `""` when the role name is `""`, else
  `FieldValue(th.Issue, role)` (the issue's own label first, then the
  inherited project role).
- Branches: `branches.Linked(b, th.Issue, start)` with `start` from
  `branches.Suggest`'s `Start` and `Base` from its `Base`. The plan branch
  is the one with prefix `plan/`; the evidence branch is the other with the
  newest `ReadCommit(hash).Date` (local over remote on a tie).
- Plan: `GrepFiles(ref, cfg.Stages().Plans, re)` for `ref` in evidence
  branch, plan branch, base, first hit wins, where `re` is
  `^issue: *["']?(CO2-|#)?<N>["']? *$` with `N` the issue's bare number,
  or `^issue: *["']?<id prefix 7>` when no number. Front matter: the
  lines between the first two `---` lines; `status:` value trimmed of
  quotes.
- `Tests`: `ChangedFiles(Base, Branch)` filtered by `cfg.Stages().Tests`
  with `matchTest(pat, path)`: `path.Match(pat, path)` or
  `path.Match(pat, path.Base(path))`, or `pat` ending in `/**` and
  `strings.HasPrefix(path, prefix+"/")`; each with
  `FirstCommitAdding`.
- Comments: every `comment` event with `Meta`: `plan:approve` →
  `PlanApproval` (only when `By` is `People["plan"]` or that is `""`);
  `signoff:status` → `Signoffs`; `verify:run` → `Runs`; `verify:criterion`
  → `Checks` (index `strconv.Atoi`). `By` is the author's login.
- `PR, PRAt = pulls.PR(Branch)`; `PlanPR, _ = pulls.PR(PlanBranch)`.
  `pulls` nil → none.

- [ ] **Step 1: Write the failing tests** on a real repo (the `git`
  helper of Task 2's test, `gitbugio.Open(dir)`, a user identity as
  `branches_test.go` sets one):
  - `TestGatherFromRepo`: `dev` with `docs/src/implementation-plans/
    2518-x.md` (`status: accepted`, `issue: 2518`), branch `fix/2518-x`
    adding `api/a_test.go` and `api/a.go` (it inherits dev's plan),
    branch `plan/2518-x` changing the plan to `status: draft`: the
    evidence branch is read first, so `Plan.Ref == "fix/2518-x"` and
    `Plan.Status == "accepted"`, the plan branch's draft loses. An imported issue `(#2518)`
    with labels `status:in-progress`, `risk:normal`, `assignee:erin`,
    description with two criteria; `AddCommentMeta` a `plan:approve` as
    the identity. `cfg` with co2's status options, `Roles {"lead-dev":
    "<identity login>"}`, `Cycle{Plan: "lead-dev"}`. Assert `Status
    in-progress`, `Criteria` 2, `Branch fix/2518-x`, `PlanBranch
    plan/2518-x`, `Tests == [{api/a_test.go, <hash>}]`, `Plan.Status
    accepted`, `PlanApproval.By` the login, `People["plan"]` the login,
    `People["risk"] == ""`.
  - `TestEvidenceBranch`: `2518-old` (older commit), `fix/2518-new`
    (newer), `plan/2518-p` → `Branch fix/2518-new`.
  - `TestMatchTest`: table over the five defaults: `internal/x/y_test.go`
    ✓, `web/src/a.test.ts` ✓, `tests/unit/t.py` ✓, `test_x.py` at the root
    ✓, `src/tests.ts` ✗, `web/src/a.ts` ✗.
  - `TestGatherNoBranch`: `gitbugiotest.New(t)`, an issue at `ready`:
    `Branch ""`, `Tests` nil, `Plan` nil, no error.
- [ ] **Step 2: Run them to see them fail.**
- [ ] **Step 3: Implement** `gather.go`.
- [ ] **Step 4: Run them to see them pass**, then `go test ./internal/cycle`.
- [ ] **Step 5:** Commit message `feat(cycle): gather the evidence from git-bug, git and the PR cache`.

---

### Task 5: `GET /api/issues/{id}/cycle`, and the PR cache

**Files:**
- Create: `internal/server/cycle.go`, `internal/server/cycle_test.go`
- Modify: `internal/server/server.go` (routes; `pulls pullCache` on
  `apiServer`)

**Interfaces:**
- Consumes: Task 3 `Compute`, Task 4 `Gather`, `s.github()`
  (`pulls.go:37`), `onOrigin` (`pulls.go:46`), `s.config()`,
  `s.b.Thread`, `inboxMe`'s login rule (`inbox.go:39`: `s.me`, else
  `s.b.MyLogin()`; extract `func (s *apiServer) viewer() string` and use
  it in both).
- Produces:
  - `GET /api/issues/{id}/cycle` → `cycle.Cycle` as JSON, plus
    `"stages": config.Stages` and `"pr_checked_at"` (RFC 3339, omitted
    when never) in a wrapper `cycleResponse{Cycle; Stages; PRCheckedAt}`.
    404 for an unknown issue (`writeIssueError`).
  - `pullCache`: `map[string]cachedPull{pr *ghpr.PR; at time.Time; err
    string}` under a mutex, with `func (c *pullCache) PR(branch string)
    (*ghpr.PR, time.Time)` (Task 4's interface). `func (s *apiServer)
    refreshPulls(ctx, is gitbugio.Issue, names []string)`: for each
    branch on origin (`onOrigin`), when its entry is older than 5
    minutes, `Find` it (and `Required` + `Gate` as `handlePulls` does);
    an error keeps the old entry and records `err`. Called from the
    cycle handler only when the stage is `in-review` or later (or a
    `plan/` branch exists and the risk is structural), with a 10 s
    context timeout; before `Gather`.

- [ ] **Step 1: Write the failing tests** in `cycle_test.go`, with the
  repository setup style of `TestBranchesOfAnIssue` and `newFakeGitHub`
  (`pulls_test.go:32`) for GitHub:
  - `TestCycleOfAnIssue`: an imported issue `(#2518)` at `status:ready`
    with criteria and no risk, `cfg` co2's statuses + `Roles {"pm":
    "guilbep"}` + `Cycle{Risk: "pm"}`; GET → 200, `step 2`, `waits.login
    guilbep`, `mine true`, `actions [set-risk]`, `stages.ready == "ready"`,
    `stages.guessed true`. Then `PATCH {"risk": "low"}` and GET → `step 3`,
    `skipped [3]`.
  - `TestCycleReadsThePR`: `status:in-review`, `fix/2518-x` on origin,
    origin `https://github.com/o/r.git`, a token for the identity (the
    keyring rule of `pulls_test.go`'s helper); GET → `evidence.pr.number
    3082`, `pr_checked_at` set, the fake saw one `POST /graphql`; a second
    GET sees no new request. At `status:ready` with the same branch: no
    request.
  - `TestCycleOffline`: the fake answers 500: GET is still 200 with
    `step 6` and no `evidence.pr`.
- [ ] **Step 2: Run them to see them fail.**
  Run: `GOCACHE=$TMPDIR/go-build go test ./internal/server -run Cycle`
- [ ] **Step 3: Implement** `cycle.go` and register the route.
- [ ] **Step 4: Run them to see them pass**, then `go test ./internal/server`.
- [ ] **Step 5:** Commit message `feat(cycle): GET /api/issues/{id}/cycle, with a per-branch PR cache`.

---

### Task 6: `POST /api/issues/{id}/cycle/{action}`

**Files:**
- Modify: `internal/server/cycle.go`, `cycle_test.go`, `server.go` (route)

**Interfaces:**
- Consumes: Task 2 `AddCommentMeta`; `fields.Apply`
  (`internal/fields/fields.go:27`) for the status move; `Backend.CloseBug`
  (`gitbugio.go:244`); `s.beginWrite`, `decode` (`issues.go:23,33`).
- Produces, `POST /api/issues/{id}/cycle/{action}`, JSON body, answers
  200 with the recomputed `cycleResponse`:
  - `approve-plan` `{}`: viewer must be `People["plan"]` (403 `waits for
    the plan role: <login>` otherwise; `""` → anyone). Refused 400 `the
    plan PR is approved on GitHub` when the risk is structural. Posts
    `Plan approved` with `plan:approve "1"`, `plan:commit <evidence head
    or plan branch head>` when known.
  - `verify` `{"output": "...", "commit": "<sha>"}`: anyone at step 7
    (400 `not at Verify` otherwise); empty output → 400. Posts the output
    in a fenced block under `Run at <commit>` with `verify:run "1"`,
    `verify:commit`.
  - `check` `{"index": 2, "commit": "<sha>"}`: anyone at step 7; index
    out of `Criteria` → 400. Body `Checked: <criterion text>`, metadata
    `verify:criterion "<index>"`, `verify:commit`.
  - `signoff` `{"why": ""}`: viewer must be `People["signoff:"+status]`
    (403 `waits for the <role>: <login>`); 400 `not at Verify` off step 7.
    Needs a `Run` whose `Commit == Head` (or any run when `Head == ""`),
    else 400 `no run at <head>; sign off with a reason to override` unless
    `why` is set. Posts `Signed off <status> at <head7>` (plus `\n\n<why>`)
    with `signoff:status`, `signoff:commit`, `signoff:why` when given. Then
    moves the status: the next of `Stages.Validation`, or when the current
    one is the last, `Stages.Closing` via `fields.Apply(s.b, cfg, id,
    "status", next, me, time.Now())` and `s.b.CloseBug(id)`.
  - Any other action → 404. Everything under `s.beginWrite(w)`.

- [ ] **Step 1: Write the failing tests** (same setup as Task 5):
  - `TestCycleApprovePlan`: at `ready`, `risk:normal`, `Roles {"lead-dev":
    "guilbep"}`, `Cycle{Plan: "lead-dev"}`: POST → 200, `step 3`, `note`
    `plan approved, …`; the thread's last event has `meta["plan:approve"]
    == "1"`. With `me: "bob"` on the server → 403 `waits for the plan
    role: guilbep`, and no new event.
  - `TestCycleSignoffRefused`: at `qa`, `Cycle{Signoffs: {"qa": "pm"}}`,
    `Roles {"pm": "alice"}`, `me guilbep` → 403, labels unchanged.
  - `TestCycleSignoffNeedsARun`: at `qa`, pm is guilbep, no run → 400
    containing `no run`; `{"why": "ran it by hand"}` → 200, the comment's
    `meta["signoff:why"]`; the status label is now
    `status:tests-metier`.
  - `TestCycleVerifyThenSignoff`: `verify {"output": "ok", "commit": ""}`
    on an issue with no branch → 200; `check {"index": 0}` → 200 with
    `verify:criterion "0"`; `check {"index": 9}` → 400; `signoff {}` →
    200.
  - `TestCycleSignoffCloses`: at `tests-metier` (no role: anyone),
    `signoff` → 200, `done true`, the issue is `closed` and labelled
    `status:validated`.
  - `TestCycleBadAction`: `POST …/cycle/dance` → 404. Without JSON
    content type → 415 (the guard).
- [ ] **Step 2: Run them to see them fail.**
- [ ] **Step 3: Implement** `handleCycleAction` and the four actions as
  one `switch`. A `ponytail:` comment on the run check: the head is the
  local branch head, not the merged commit on the base; M2 reads the
  merged head when the PR cache knows it.
- [ ] **Step 4: Run them to see them pass**, then
  `GOCACHE=$TMPDIR/go-build go test ./...` and `make ci`.
- [ ] **Step 5:** Commit message `feat(cycle): approve the plan, post a run, check a criterion, sign off`.

---

### Task 7: the progress line and the card in the Issue view

**Files:**
- Modify: `web/src/api/board.ts` (types, `getCycle`, `cycleAction`)
- Create: `web/src/cycle.ts`, `web/src/cycle.test.ts`,
  `web/src/components/IssueCycle.vue`
- Modify: `web/src/components/IssuePanel.vue` (mount under `.gb-imeta`,
  line 94), `web/package.json` (`test`: add `src/cycle.test.ts`)

**Interfaces:**
- Consumes: Task 5 and 6 JSON; `patchIssue` for the risk; `request` and
  `issuePath` in `board.ts`; `me` from `@/me` is not needed (the server
  decides `mine`).
- Produces, in `board.ts`:

```ts
export interface CycleWaits { role?: string; login?: string; what: string; anyone: boolean }
export interface Cycle {
  step: number; of: number; name: string; stage: string; done: boolean; gate: string;
  risk?: string; skipped?: number[]; note?: string; waits: CycleWaits; mine: boolean;
  actions?: string[]; evidence: { criteria?: string[]; head?: string; branch?: string; pr?: GatePR | null; runs?: { by: string; commit: string }[]; checks?: { index: number; by: string }[]; [k: string]: unknown };
  stages: { ready: string; in_progress: string[]; in_review: string[]; validation: string[]; closing: string; guessed: boolean };
  pr_checked_at?: string;
}
export function getCycle(id: string): Promise<Cycle>
export function cycleAction(id: string, action: string, body: Record<string, unknown> = {}): Promise<Cycle>
```

(Go side: give `config.Stages` json tags `ready`, `in_progress`,
`in_review`, `validation`, `closing`, `guessed`, `plans`, `tests`,
`before`.)

In `cycle.ts`, pure:

```ts
// "Step 7 of 7 · Verify, waits for alice to sign off qa"; "Step 3 of 7 · Plan, skipped (low risk)…";
// "Done" when done; "…, anyone's turn to set the risk class" when anyone.
export function progressLine(c: Pick<Cycle, 'step' | 'of' | 'name' | 'done' | 'waits' | 'skipped' | 'note'>): string
// The card's title and button labels for the actions: set-risk → "Set the risk" with
// ["low", "normal", "structural"]; approve-plan → "Approve the plan"; review → "Review";
// signoff → `Sign off ${status}`; verify/check are shown as one "Post the run" box in M1.
export function cardFor(c: Cycle, status: string): { title: string; gate: string; buttons: { label: string; action: string; body?: Record<string, unknown> }[] } | null
```

`IssueCycle.vue`, props `id: string`, `status: string` (the status field
value, from `labels`), `labels: string[]`; emits `changed` after a write.
Loads `getCycle` on `id` and `labels` change (the panel re-reads the
thread after every write, so labels moving re-reads the cycle). Renders:
- `<p class="gb-cycle" role="status">` with `progressLine`, and the `note`
  in `gb-hint` after it.
- When `cardFor` is not null: `<div class="gb-card gb-cycle-card">` with
  the title, the gate as `gb-hint`, and the buttons (`epfl-btn
  epfl-btn-primary epfl-btn-sm`; the three risk buttons
  `epfl-btn-secondary`, each `patchIssue(id, {risk})`); `review` is a
  link to `evidence.pr.url` when there is one, else to `#development`
  (the side's section); `signoff` opens a one-line reason input only
  when the server answered 400 with `no run`, then re-posts with `why`;
  the Verify box: a textarea and a commit input, **Post the run**
  (`verify`), and the criteria as checkboxes (`check` on tick, checked
  ones disabled with "by <login>").
- Errors: the `ApiError` message in `<p class="gb-save-error" role="alert">`.

- [ ] **Step 1: Write the failing tests** in `cycle.test.ts`:
  `progressLine` for the step-7 example above, the low-risk skipped one,
  `Done`, and anyone's turn; `cardFor` returns null when `mine` is false
  or `actions` empty, three risk buttons with bodies for `set-risk`, `Sign
  off qa` for `signoff` at status `qa`.
- [ ] **Step 2: Run** `cd web && npm test` (after adding the file to
  `package.json`). Expected: FAIL, module not found.
- [ ] **Step 3: Implement** `cycle.ts`, the `board.ts` types and calls,
  `IssueCycle.vue`; mount it in `IssuePanel.vue` after the `gb-imeta`
  div: `<IssueCycle :id="thread.id" :status="statusValue" :labels="thread.labels" @changed="load" />`
  where `statusValue` is the `status:` label's value (a computed from
  `thread.labels`).
- [ ] **Step 4: Verify** `cd web && npm run typecheck && npm test`.
- [ ] **Step 5:** Commit message `feat(cycle): the step under the title, and the next-step card`.

---

### Task 8: docs and the PRD

**Files:**
- Modify: `docs/configuration.md` (table in Overview; new `## Cycle`
  section after `## Branches`), `README.md` (one sentence in the issue
  page's paragraph), `docs/prd/dev-cycle.md` (M1: done, and the calls
  this plan took)

- [ ] **Step 1:** `## Cycle`: the M1 TOML from the PRD, then one
  paragraph: every key has a default, the stages are guessed from the
  status names (which words guess which stage), the plans directory is
  the kit's, the roles are unset so a gate is anyone's; a table of the
  keys. One paragraph on what the step reads (status, criteria heading,
  risk, plan front matter `issue:`/`status:`, test patterns, PR, comments
  with `plan:`/`signoff:`/`verify:` metadata) and what the actions write.
- [ ] **Step 2:** README: one sentence.
- [ ] **Step 3:** PRD: in the Decisions row "Worktrees and agents", the
  machine where `git-board-server` runs is now the one where `git board
  serve` runs (the one-binary rename on main). PRD M1: mark done; record the calls: the risk field is
  always `risk` (no rename key), structural plan approval is read from
  GitHub but not posted from git-board until M4's approvals exist (the
  card links to the plan PR), `verify` and `check` are M1 actions so step
  7 is reachable without M2's workspace, the sign-off's run check uses
  the branch head (merged head in M2), Settings showing the guessed stages
  is left to M2 (the API already returns `stages.guessed`). Move "rename
  the risk field" to "Open, not blocking".
- [ ] **Step 4:** Commit message `docs(cycle): [cycle], the step and its actions`.

---

### Task 9: verification and hand-off

- [ ] **Step 1:** `GOCACHE=$TMPDIR/go-build make ci`, `cd web && npm run
  typecheck && npm test`. If `httptest` can't bind in the sandbox, run
  `GOCACHE=$TMPDIR/go-build go test $(go list ./... | grep -v
  internal/server)` and list the server tests that need a real
  terminal (`cycle_test.go`'s, and the package's existing ones).
- [ ] **Step 2:** `gofmt -l .` is empty. `git add -A`, no commit, no
  push. Report the five commit messages above (or one squashed
  `feat(cycle): M1, the step from the status and the evidence`) and
  what needs a terminal.

## Self-review notes

- Spec coverage: computation ✓ (3, 4), shown ✓ (7), written ✓ (6: risk
  via PATCH as the PRD says; approval, sign-off, verify as comments),
  config with defaults ✓ (1), local-first PR cache ✓ (5), "Done when"
  table test ✓ (`TestWalk`, `TestStatusAloneNeverTellsTwoFromThree`,
  the in-progress floor row, `TestCardShowsToRoleOnly`).
- Not in M1, said in Task 8: renaming the risk field; posting a GitHub
  approval for structural plans; Settings › Project file showing the
  guess; the merged head for the run check.
