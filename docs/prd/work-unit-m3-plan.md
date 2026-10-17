# Work unit M3: the PR gate, implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** An issue's Development section shows, for each branch on origin,
the GitHub PR whose head it is (or an **Open PR** button), its checks with
the ruleset's required ones first, who approved at which commit, GitHub's
review decision, and **Merge into dev**, enabled when GitHub says the PR
can merge and otherwise saying what it waits for.

**Architecture:** A small GitHub client, `internal/ghpr`, plain
`net/http`: one GraphQL query finds a branch's PR with its checks, reviews
and merge state; the REST rules endpoint names the base's required checks;
REST opens and merges. `ghpr.Gate` turns those into `CanMerge` and
`Waits`. The server answers `/api/issues/{id}/pulls` apart from
`/branches`, so the branch list stays local and offline-proof; the web's
`IssueBranches.vue` renders it (layout B, mocked and chosen 2026-10-17:
every check listed, required first).

**GitHub access, and why:** git-bug's keyring token whose login is the
user identity's GitHub login, not the `gh` CLI. It is the account
the fork's bridge posts that person's comments and, in M4, approvals
under; `gh` can be logged in as someone else on the same machine, and a
PR opened by one account and approved by another under one person breaks
"GitHub refuses an approval from a PR's author". No match is an error,
"no GitHub token for <login>", never another token. Plain HTTP also
replays in tests (`httptest`) without faking a binary on PATH.

**Tech Stack:** Go (`net/http`, `encoding/json`), git-bug's `bridge/core/auth`, Vue 3.

**Spec:** `docs/prd/work-unit.md`, section M3, and the Decisions table.

## Global Constraints

- The PR is found on GitHub by head ref, never stored. Only PRs whose head
  repository is origin's repository count (a fork's same-named branch is
  another PR).
- Origin only, and only when origin is on github.com. Any other origin: no
  gate, Development as today.
- Title `CO2-3075 <title>` (`is.Number`, then `is.ShownTitle()`; before the
  switch, the shown title alone). Body: the number (else the git-bug id's
  first 7 characters), and `Closes #N` when one of the issue's forges is
  `https://github.com/<origin's repo>/issues/N` (owner and name compared
  case-insensitively, `Gone == ""`).
- Merge: `PUT /repos/{repo}/pulls/{n}/merge` with `merge_method: "merge"`
  and `sha` set to the head commit the page showed.
- Merge is enabled only when GitHub says so: `state OPEN`, not a draft,
  `mergeable MERGEABLE`, `mergeStateStatus` one of `CLEAN`, `HAS_HOOKS`,
  `UNSTABLE`.
- Required checks come from `GET /repos/{repo}/rules/branches/{base}`
  (`required_status_checks`), never hardcoded (co2's own ruleset spells
  "Frontent").
- git-board still creates and pushes no branch. A branch only local shows
  the `git push -u origin <name>` command to copy, not Open PR.
- Nothing calls GitHub when the issue has no branch on origin.
- No new dependency. No live GitHub call in `make ci`.
- **git-bug's keyring is one file for every repository**, in the user's
  config directory (`os.UserConfigDir()/git-bug/keyring`). A test stores
  tokens only on a `gitbugiotest` backend (repotest's in-memory keyring),
  or, on a `gitbugio.Open(dir)` backend, after `t.Setenv("HOME", tmp)`
  and `t.Setenv("XDG_CONFIG_HOME", tmp)`, set before `Open`. A stray token
  in the real keyring breaks the user's syncs: the fork's bridge queries
  `viewer` with every GitHub token it holds.
- `Closes #N` closes the issue only when the PR merges into the
  repository's default branch: co2's is `dev` (checked 2026-10-17).
- Plain HTTP needs no preview header: `mergeStateStatus` answered over
  `curl` without one (checked 2026-10-17).

## Review Focus

1. **A fork's PR with the same branch name** is ignored; the same-repo one
   shows. (Task 2: `TestFindSameRepoOpenFirst`.)
2. **The head moved after the page loaded**: GitHub refuses the merge
   (`sha` mismatch) and the page shows GitHub's message, nothing merges.
   (Task 3: `TestMergePull`, fake answers 409.)
3. **A keyring holding only someone else's token**: the gate says "no
   GitHub token for <me>" and never uses it. (Task 1: `TestGitHubToken`.)
4. **A required check that never reported** (absent from the rollup) is
   listed as expected and named in Waits. (Task 2: `TestGateMissingRequired`.)
5. **`mergeable UNKNOWN`** right after a push or open: "Checking…", and
   the page re-reads, not a dead disabled button. (Task 2:
   `TestGateChecking`; Task 4: the re-read timer.)

---

### Task 1: my GitHub token, origin's GitHub repository

**Files:**
- Modify: `internal/gitbugio/forges.go`, `internal/gitbugio/remotes.go`
- Test: `internal/gitbugio/forges_test.go` (create if absent), `internal/gitbugio/remotes_test.go`

**Interfaces:**
- Produces:
  - `func (b *Backend) GitHubToken() (login, token string, err error)`:
    the user identity's GitHub login, and the github-target token whose
    `auth.MetaKeyLogin` is it (case-insensitive). The login, the way the
    fork's bridge maps an author to an account: the identity's
    `github-login` metadata, else a GitHub bridge's sync link
    (`git-bug.bridge.<name>.login-<identity id>` where that bridge's
    `target` is `github`), else the identity's own login. (`MyLogin` takes
    the own login first: on a repository also bridged to GitLab, that can
    be a GitLab username.) No login: error `no GitHub login: choose who
    you are in git-board first`. No token: error `no GitHub token for
    <login>`.
  - `func (b *Backend) GitHubRepo(remote string) string`: `"owner/name"`
    when the remote's URL is on github.com
    (`https://github.com/o/r(.git)`, `git@github.com:o/r.git`,
    `ssh://git@github.com/o/r(.git)`), else `""`.

- [ ] **Step 1: Write the failing tests**

`TestGitHubToken`, on `gitbugiotest.New(t)` (in-memory keyring): store
two github tokens with `auth.Store` on `b.TestCache()`
(`auth.NewToken("github", "tok-alice")` with
`SetMetadata(auth.MetaKeyLogin, "alice")`, same for bob) and one gitlab
token with login `carol`. Then, as the user identity:
- an identity with own login `gl-user` and metadata `github-login: Bob`
  (`b.TestCache().Identities().NewRaw(…)`, then `SetUserIdentity`) gets
  `("Bob", "tok-bob")`;
- one with own login `carol` and no metadata errors `no GitHub token for carol`;
- one with no login anywhere errors with `no GitHub login`.

`TestGitHubRepo`: a table setting `remote.origin.url` (via `git config` in
a temp repo, as `branches_test.go` does) to each of the forms above, plus
`https://gitlab.com/o/r.git` and a bare path, expecting `o/r` or `""`.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/gitbugio -run 'TestGitHubToken|TestGitHubRepo'`
Expected: FAIL, `b.GitHubToken undefined`.

- [ ] **Step 3: Implement both** next to `ForgeToken` and `Remotes`.
`GitHubRepo` reads the URL through `b.repo.GetRemotes()`.

- [ ] **Step 4: Run them to see them pass**, then `go test ./internal/gitbugio`.

- [ ] **Step 5: Commit** `feat(gate): my GitHub token, and origin's repository`

---

### Task 2: `internal/ghpr`, the gate read from GitHub

**Files:**
- Create: `internal/ghpr/ghpr.go`, `internal/ghpr/ghpr_test.go`,
  `internal/ghpr/testdata/pr.json`, `internal/ghpr/testdata/rules.json`

**Interfaces:**
- Produces:

```go
// Client is GitHub's API as one person: their token.
type Client struct {
	API   string // "" is https://api.github.com; tests: an httptest server
	Token string
	HTTP  *http.Client // nil: a client with a 30 s timeout
}

type Check struct {
	Name     string `json:"name"`
	State    string `json:"state"` // success, failure, pending, expected (required, never reported)
	Required bool   `json:"required"`
	URL      string `json:"url,omitempty"`
}

type Review struct {
	Login  string `json:"login"`
	State  string `json:"state"`  // APPROVED, CHANGES_REQUESTED, COMMENTED
	Commit string `json:"commit"` // the commit it was made at
}

type PR struct {
	Number     int      `json:"number"`
	URL        string   `json:"url"`
	State      string   `json:"state"` // OPEN, MERGED, CLOSED
	Draft      bool     `json:"draft"`
	Title      string   `json:"title"`
	Base       string   `json:"base"`
	Head       string   `json:"head"` // its head commit
	Author     string   `json:"author"`
	Decision   string   `json:"decision"`    // reviewDecision: APPROVED, REVIEW_REQUIRED, CHANGES_REQUESTED, "" (no review rule)
	Mergeable  string   `json:"mergeable"`   // MERGEABLE, CONFLICTING, UNKNOWN
	MergeState string   `json:"merge_state"` // mergeStateStatus
	Checks     []Check  `json:"checks"`
	Reviews    []Review `json:"reviews"`   // latestReviews
	Requested  []string `json:"requested"` // requested reviewers: logins, team slugs
	// set by Gate
	CanMerge bool     `json:"can_merge"`
	Checking bool     `json:"checking"` // GitHub is still computing mergeability
	Waits    []string `json:"waits"`
}

func (c *Client) Find(ctx context.Context, repo, branch string) (*PR, error)       // nil, nil: none
func (c *Client) Required(ctx context.Context, repo, base string) ([]string, error)
func (c *Client) Open(ctx context.Context, repo, head, base, title, body string) (*PR, error) // Number and URL set
func (c *Client) Merge(ctx context.Context, repo string, number int, sha string) error
func Gate(p *PR, required []string)
```

- [ ] **Step 1: Capture the fixtures**

`testdata/pr.json`: the answer of `Find`'s query for co2-calculator's PR
#3082 (read-only, `gh api graphql`), with `EPFL-ENAC/co2-calculator`
rewritten `o/r`. The draft capture is
`/private/tmp/claude-502/-Users-guilbert-works-git-github-git-board/5338b72d-e4ce-4133-b176-5257794b1709/scratchpad/co2-pr.json`;
re-run it once `Find`'s query is final (drop `isRequired`: it needs the PR
number up front). It holds 13 check runs: Frontent Lint/Type Check and
Frontend Tests failed, the rest passed; `reviewDecision REVIEW_REQUIRED`,
`mergeable MERGEABLE`, `mergeStateStatus BLOCKED`, no reviews, team
`co2-calculator` requested.
`testdata/rules.json`: `gh api repos/EPFL-ENAC/co2-calculator/rules/branches/dev`,
kept to its `pull_request` and `required_status_checks` rules (the six
names).

- [ ] **Step 2: Write the failing tests**, each against an `httptest`
server that checks `Authorization: Bearer tok`:

- `TestFind`: serves `pr.json` at `POST /graphql`; asserts the query's
  variables (`owner o`, `name r`, `head fix/3075-x`), `Number 3082`,
  `Head c1ee2d98…`, `Decision REVIEW_REQUIRED`, 13 checks with
  `Frontend Tests` `failure`, `Requested == ["co2-calculator"]`.
- `TestFindSameRepoOpenFirst`: nodes newest first: a CLOSED same-repo PR,
  an OPEN PR from `fork/r`, an OPEN same-repo PR #7. `Find` returns #7.
  Without the open one it returns the closed one; with no node, `nil, nil`.
- `TestRequired`: serves `rules.json` at
  `GET /repos/o/r/rules/branches/dev`; returns the six names in order.
- `TestGate`: `pr.json` + the six: `CanMerge false`; the first six checks
  are the required ones; `Waits == ["Frontent Lint/Type Check", "Frontend
  Tests", "an approval from co2-calculator"]`.
- `TestGateMissingRequired`: required `["Docs Build", "E2E"]`, no `E2E`
  in the rollup: a check `{E2E, expected, required}` is added and `E2E`
  is in Waits.
- `TestGateClean`: OPEN, MERGEABLE, CLEAN, APPROVED, all checks success:
  `CanMerge true`, `Waits` empty. Same with UNSTABLE (a non-required
  failure): `CanMerge true`.
- `TestGateChecking`: `Mergeable UNKNOWN`: `Checking true`, `CanMerge
  false`.
- `TestGateWords`: CHANGES_REQUESTED by bob → `changes bob requested`;
  CONFLICTING → `conflicts with dev resolved`; BEHIND → `an update from
  dev`; draft → `ready for review (a draft)`; BLOCKED with nothing else to
  say → `GitHub's rules`; REVIEW_REQUIRED with no one requested → `an
  approval`.
- `TestOpen`: asserts `POST /repos/o/r/pulls` with JSON `{title, head,
  base, body}`; answers 201 `{"number": 9, "html_url": "…/pull/9"}`.
- `TestMerge`: asserts `PUT /repos/o/r/pulls/9/merge` with
  `{"merge_method":"merge","sha":"abc"}`. A 409 `{"message":"Head branch
  was modified. Review and try the merge again."}` becomes an error
  holding that message. A GraphQL answer with `errors` becomes an error
  holding the first message.

- [ ] **Step 3: Run them to see them fail**

Run: `go test ./internal/ghpr`
Expected: FAIL, package doesn't build.

- [ ] **Step 4: Implement** `ghpr.go`. The query, verified against
GitHub 2026-10-17:

```graphql
query($owner: String!, $name: String!, $head: String!) {
  repository(owner: $owner, name: $name) {
    pullRequests(headRefName: $head, first: 10, orderBy: {field: CREATED_AT, direction: DESC}) {
      nodes {
        number url state isDraft title baseRefName headRefOid
        headRepository { nameWithOwner }
        author { login }
        reviewDecision mergeable mergeStateStatus
        latestReviews(first: 20) { nodes { author { login } state commit { oid } } }
        reviewRequests(first: 10) { nodes { requestedReviewer { ... on User { login } ... on Team { slug } } } }
        commits(last: 1) { nodes { commit { statusCheckRollup { contexts(first: 100) { nodes {
          __typename
          ... on CheckRun { name status conclusion detailsUrl }
          ... on StatusContext { context state targetUrl }
        } } } } } }
      }
    }
  }
}
```

A check's state: a CheckRun not `COMPLETED` is `pending`; `SUCCESS`,
`NEUTRAL`, `SKIPPED` are `success` (GitHub lets those pass a required
check); any other conclusion is `failure`. A StatusContext: `SUCCESS` is
`success`, `PENDING`/`EXPECTED` `pending`, else `failure`.
`Gate` lists required checks first (the rules' order, adding `expected`
ones), then the rest in GitHub's order. `Waits` in this order: draft,
each required check not `success`, the review, conflicts, behind, and
`GitHub's rules` only when `MergeState` is `BLOCKED` and nothing else was
said.

- [ ] **Step 5: Run them to see them pass.** `go test ./internal/ghpr`

- [ ] **Step 6: Commit** `feat(gate): read a branch's PR, its checks and reviews from GitHub`

---

### Task 3: the server's `/pulls`

**Files:**
- Create: `cmd/git-board-server/pulls.go`, `cmd/git-board-server/pulls_test.go`
- Modify: `cmd/git-board-server/server.go` (routes; `githubAPI string` on `apiServer`)

**Interfaces:**
- Consumes: Task 1's `GitHubToken`, `GitHubRepo`; Task 2's `ghpr`;
  `branches.Names`, `branches.Suggest`, `Backend.Branches`.
- Produces, the JSON the page reads:
  - `GET /api/issues/{id}/pulls` → `{"repo": "o/r", "base": "dev", "login":
    "alice", "pulls": [{"branch": "fix/3075-x", "pr": PR|null}]}`: one
    entry per branch on origin naming the issue, in `Branches()` order.
    `repo ""` and `pulls []` when origin isn't on GitHub. No branch on
    origin: no GitHub call, no token needed.
  - `POST /api/issues/{id}/pulls` `{"branch": "fix/3075-x"}` → 201, the
    opened PR (`number`, `url`).
  - `POST /api/issues/{id}/pulls/{number}/merge` `{"sha": "<head>"}` → 200 `{}`.
  - A GitHub failure or a missing token: 502, `{"error": <its message>}`.
    A branch not on origin or not naming the issue: 400.

- [ ] **Step 1: Write the failing tests** in `pulls_test.go`, with the
repository setup of `TestBranchesOfAnIssue` (`git config remote.origin.url
https://github.com/o/r.git`; `update-ref refs/remotes/origin/…`), a token
for the identity's login stored with `auth.Store`, and `s.githubAPI` an
`httptest` server serving Task 2's fixtures and recording requests. A
helper builds it, and calls `t.Setenv("HOME", tmp)` and
`t.Setenv("XDG_CONFIG_HOME", tmp)` before `gitbugio.Open` (Global
Constraints: the keyring); it asserts `os.UserConfigDir()` is under
`tmp` before storing anything.

- `TestPulls`: the imported issue `#2518` (`https://github.com/o/r/issues/2518`)
  has `fix/2518-x` on origin and `2518-local` only locally: `pulls` holds
  `fix/2518-x` alone, its `pr.number 3082`, `pr.waits` as in `TestGate`.
- `TestPullsWithoutGitHub`: origin `https://gitlab.com/o/r.git`: 200,
  `repo ""`. An issue with no branch on origin: 200, the fake saw no
  request.
- `TestPullsNoToken`: no token stored: 502, error `no GitHub token for <login>`.
- `TestOpenPull`: posts `{"branch":"fix/2518-x"}`; the fake received
  `head fix/2518-x`, `base dev`, title `[BUG](Equipment) Crash on save`
  (`strings.TrimSpace(is.Number + " " + is.ShownTitle())`: no project
  number yet, and the shown title drops its own `(#2518)`), body
  `"<id7>\n\nCloses #2518"`. `{"branch":"2518-local"}` → 400.
  Without `Content-Type: application/json` → refused by the guard.
- `TestMergePull`: posts `{"sha":"c1ee2d9…"}` to `/pulls/3082/merge`; the
  fake received `merge_method merge` and that sha. The fake answering 409
  `Head branch was modified…` → 502 with that message.

- [ ] **Step 2: Run them to see them fail.** `go test ./cmd/git-board-server -run 'Pull'`

- [ ] **Step 3: Implement** `pulls.go`: `func (s *apiServer) github()
(*ghpr.Client, string, error)` (repo from `GitHubRepo("origin")`, token
from `GitHubToken()`, `API: s.githubAPI`), the three handlers,
`prTitle(is)` and `prBody(is, repo)`. The base is `branches.Suggest`'s
`Base`. Required checks are read once per GET, and only when a branch has
an open PR. A `ponytail:` comment marks one GraphQL request per branch,
in turn (an issue has one to three; aliases in one query if that grows).

- [ ] **Step 4: Run them to see them pass**, then `go test ./cmd/git-board-server`.

- [ ] **Step 5: Commit** `feat(gate): the server opens, reads and merges an issue's PRs`

---

### Task 4: the gate in Development

**Files:**
- Modify: `web/src/api/board.ts` (types and three calls), `web/src/components/IssueBranches.vue`

**Interfaces:**
- Consumes: Task 3's JSON. `getPulls(id)`, `openPull(id, branch)`,
  `mergePull(id, number, sha)` in `board.ts`, typed `IssuePulls`,
  `GatePR`, `GateCheck`, `GateReview` after Task 2's JSON tags.

Layout B (chosen on the mockup), per branch entry:

- **An origin branch with an open PR:** `#3081 Open · by bob` linking to
  GitHub. Then every check, required first, with ✓ / ✗ / ● (pending or
  expected) and a `required` tag. Then the approvals, one line per
  APPROVED review: `✓ alice approved at d4e5f6a`, then `(the head)` when
  its commit is `pr.head`, else `(an older commit)`. Then the decision:
  `review rule met`, `review required` or `changes requested`. Then
  **Merge into {base}**, enabled when `can_merge`. A `window.confirm`
  first (`Merge #3081 into dev on GitHub?`). Under it, `Waits for: …`
  (`waits` joined by `, `), or `Checking…` when `checking`.
- **A merged or closed PR:** `#3081 Merged` / `Closed`, linking to it. A
  closed, unmerged one also gets Open PR.
- **An origin branch with no PR:** **Open PR into {base}**, and under it
  `As {login}: {title}` (what Open sends, so whose token is plain).
- **A local branch:** when origin also has it, nothing more (the gate
  shows once, on the origin entry). Otherwise `Local only. Push it to open
  a PR:` and `git push -u origin <name>`, with the Copy button the Create
  branch panel has.
- **Errors:** `Couldn’t reach GitHub. {message}` under the list, with the
  branch list still shown.
- **Re-read:** the pulls are read after the branches, again after
  Open or Merge, and every 15 s while a PR is `checking` or has a pending
  or expected check and the page is visible. The timer stops on unmount.

- [ ] **Step 1: Add the types and calls to `board.ts`**, then run
`cd web && npm run typecheck`. Expected: passes.
- [ ] **Step 2: Render the gate in `IssueBranches.vue`** as above,
keeping its classes (`gb-dev-*`, `gb-state-ok`) and adding a few
(`gb-dev-check`, `gb-dev-req`).
- [ ] **Step 3: Verify** `cd web && npm run typecheck && npm test`. The
page is checked in the app against GitHub in Task 6 (no test-only flag
pointing the server at a fake).
- [ ] **Step 4: Commit** `feat(gate): the PR, its checks and approvals, and Merge, in Development`

---

### Task 5: docs

**Files:**
- Modify: `docs/configuration.md` (§ Branches), `README.md` (the
  Development paragraph), `docs/prd/work-unit.md` (M3: decided, done,
  and what is left)

- [ ] **Step 1:** § Branches: replace "git-board creates and pushes
nothing" with: git-board creates and pushes no branch; for a branch on
origin, when origin is on GitHub, it shows the PR whose head it is, its
checks (the base's required ones first), approvals and GitHub's review
decision; **Open PR** and **Merge into dev** act on GitHub with your
token, the one in git-bug's keyring whose login is your identity's (add
it on the Sync page); merge uses merge commits and the head commit shown.
Say why not `gh` in one sentence.
- [ ] **Step 2:** README's Development paragraph: one sentence on the gate.
- [ ] **Step 3:** PRD M3: the decisions this plan took (token, rules
endpoint, layout B), and move any left items to "Open, not blocking".
Add there: co2's `dev` rules include a `branch_name_pattern`,
`^(feat|fix|chore|docs|refactor)/.+`. If it constrains new branches, the
default `{number}-{slug}` and a `perf/` kind are refused on push, and such
a branch never gets a PR.
- [ ] **Step 4: Commit** `docs: the PR gate`

---

### Task 6: live test on guilbep/git-board-test

Every write to guilbep/git-board-test is listed to the user first and
done only on their go (decided 2026-10-17). Approval live is left to M4
(no second account): this test shows `review required` and merges with
the review rule off.

- [ ] **Step 1: Ask, then set up:** the base is git-board-test's default
branch, `main`, so `Closes #N` acts without changing the default; a
workflow with one quick job (`Tests`); a ruleset on `main` (PR required,
merge only, required check `Tests`; review count 0 for M3). The
repository is private: rulesets need it public or a paid plan; ask which.
- [ ] **Step 2:** In a clone of git-board-test with `[branches] base =
"main"` and a sync bridge to git-board-test, an issue bridged there
(so the body gets `Closes #N`), a branch `fix/<n>-gate` pushed to origin:
Development shows Open PR; open it; the check shows pending then passed,
required first; Merge into main waits, then merges; the GitHub issue
closes, and the next sync brings the close back; Development shows the PR
merged and, after a fetch, the branch merged into main.
- [ ] **Step 3:** Record the result in the PRD's M3 section and list what
was left on git-board-test for the main session.
