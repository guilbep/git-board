# `.git-board.toml`

## Overview

The file is optional. Without it, every `ns:value` label is a field and git-board works on any git-bug repository. The file adds what labels can't say: the order of a field's values, single or multiple values, colors, sprint dates, saved views, the GitHub Project to sync with, and where attachments go.

It lives at the root of the repository. Edit it in **Settings → Project file**: it is checked as you type and applies as soon as you save. Editing it by hand works too; restart `git board` (or `git board serve`) after.

A repository without one starts from the default: **Start from the default** in Settings → Project file, or `git board init` in a terminal (it never overwrites a file). The default declares a status workflow (`backlog`, `ready`, `in-progress`, `in-review`, `ready-for-validation`, `validated`), priorities `p0` to `p3`, an on-hold field beside the status (`hold:blocked`, `hold:paused`), kinds that are GitHub's plain labels (`bug`, `feat`…) and name branches (`fix/12-login-fails`), estimates in hours, sprints of 14 days, and four views (all, backlog, board, mine), each explained in a comment.

| Table | What it sets |
| --- | --- |
| `[fields.<name>]` | a field's type, values, order, single or multiple: together, the project's ontology |
| `[fields.<name>.colors]` | the color of each value |
| `[fields.<name>.descriptions]` | what each value means, as its GitHub label says it |
| `[views.<name>]` | a saved view: layout, filter, grouping, sort |
| `[sprints]`, `[[sprints.list]]` | sprint length and dates, for `sprint:@current` |
| `[github-project]` | the GitHub Project to sync fields with |
| `[uploads]` | where attached files go |
| `[branches]` | the branch an issue suggests: its base and its name |
| `[cycle]`, `[cycle.signoffs]` | the development cycle: which statuses are which stage, where plans live, which files are tests, who holds each gate |
| `[repos]` | the GitHub repositories whose labels Settings › Labels and `git board lint --create-labels` keep, and how the forges hold assignees |

## Fields

A field is a label namespace: `status:ready` is the value `ready` of the field `status`. Every namespace found on the issues is already a field; declare one here to give it an order, a cardinality or colors.

```toml
[fields.status]
single = true
options = ["backlog", "ready", "in-progress", "in-review", "validated"]

[fields.est]
single = true
numeric = true
options = ["0.5", "1", "2", "4", "8"]

[fields.assignee]
single = false
```

| Key | Type | Default | What it does |
| --- | --- | --- | --- |
| `options` | list of strings | none | The values, in workflow order: board columns, sort order, and the next status for `git board move <id> --next`; the last one is what `git board close` sets. A value outside the list still works; `git board lint` reports it. |
| `single` | true / false | false | One value at a time: setting a new one replaces the old. Multiple values (`false`) suit assignees or modules. |
| `numeric` | true / false | false | Sorts as numbers. A field named `est` shows its values in hours (`3h`). |
| `project` | inline table | none | The GitHub Project's option name for a value, when it isn't the value: `project = { comite = "Comité Spécialisé" }`. |
| `type` | one of the types below | guessed | What the values are; see Types. |
| `unit` | string | none (`h` for `est`) | Written after a number field's values: `unit = "pt"`. |

## Types

The fields and their types are the project's **ontology**: the kinds of things an issue can say about itself, and how they relate. An assignee is a person, a sprint is a span of dates, a parent is another issue. The type decides how a field sorts, filters, shows and is edited; its values stay labels (`due:2026-10-30`, `parent:#12`), so they travel to GitHub and GitLab like any label.

| Type | For | What it gives | Guessed from |
| --- | --- | --- | --- |
| `select` | status, priority, module | ordered values (`options`), colors, board columns | `options`; any field nothing else fits |
| `iteration` | sprint | the dates of `[[sprints.list]]`, `@current`, sorting by Dates, today's sprint tagged | the names `sprint`, `iteration` |
| `person` | assignee, reviewer | logins, `@me`, several at once | the names `assignee`, `assignees`, `reviewer`, `owner` |
| `number` | estimate, points | numeric sorting, a `unit` after the values | `numeric = true` |
| `date` | due date, deadline | a date picker, red once past, amber within a week, "3 days late" | the names `due`, `deadline`; values that are all `YYYY-MM-DD` |
| `issue` | parent, blocked-by | a link to the other issue, and on it the reverse: Sub-issues, Blocks | the names `parent`, `blocked-by`, `blocks`, `depends-on`, `duplicate-of`; values that are all `#12` or git-bug ids |
| `text` | anything free | a plain text box | only when set |

Set a type when the guess is wrong, or for a field no issue uses yet:

```toml
[fields.due]
type = "date"

[fields.points]
type = "number"
unit = "pt"

[fields.parent]
type = "issue"
```

An issue field names the other issue by its forge number, `#12`, or by its git-bug id, at least its first 7 characters.

On hold is a field beside the status, never a status: the default's `hold` (`blocked`, `paused`) leaves a blocked issue at its place in the workflow. An issue without one isn't missing a value.

## Colors

Every field value already has a color, picked from words in it: done, validated or closed are green; review or qa purple; validation pink; in-progress, doing or wip blue; ready or feat teal; waiting, comite or p2 amber; test, p1 or high orange; blocked, bug, fix or p0 red; backlog, todo, p3 or low grey. Other values take blue, purple, teal and pink in turn. Numbers and assignees stay plain.

Pick a color yourself for any value:

```toml
[fields.status.colors]
"in-review" = "purple"
"tests-metier" = "orange"
validated = "green"
```

The colors are grey, blue, purple, amber (or yellow), orange, green, red, teal and pink, or a hex `"#d73a4a"`, as a GitHub label has it. A hex color paints the value's dot; its text stays the page's. Another name stops the file from loading, and the editor says which line. Quote a value that has spaces or other punctuation than `-` and `_`.

A value's description is what its label says on GitHub, 100 characters at most (GitHub's limit):

```toml
[fields.kind.descriptions]
bug = "Something isn't working"
```

Settings › Labels shows each label's color and description beside GitHub's, and writes GitHub's into these tables (see Repos).

## Views

A view is a saved page: it appears in the header, in alphabetical order, and its address can be bookmarked.

You rarely need to write one by hand. On the board, change the filter (its box, or a tick in the filters panel), the grouping or the sort, and a bar offers **Save view** (into this view, for everyone) or **Save as new view…**; **Discard** goes back to the saved one. Only the view's table changes in the file, comments included.

```toml
[views.current-sprint]
layout = "board"
filter = "sprint:@current is:open"
group = "status"

[views.backlog]
layout = "table"
filter = "is:open no:sprint"
sort = "prio asc"

[views.roadmap]
layout = "roadmap"
filter = "is:open"
group = "status"
```

| Key | Type | Default | What it does |
| --- | --- | --- | --- |
| `layout` | `"board"`, `"table"` or `"roadmap"` | `"table"` | A board has a column per value of `group`, and cards move between columns by drag and drop. A table has a row per issue. A roadmap puts each issue on a time line: a bar over its sprint's dates, a diamond on its date fields (`due`), today marked; issues with neither are listed below it, unscheduled. |
| `filter` | string | `is:open` | Which issues show; see Filters. |
| `group` | field name | none | The board's columns, or the table's sections: `status`, `sprint`, any field. |
| `sort` | `"<field> asc"` or `"<field> desc"` | none | Order within a column or section: by the field's `options` order, numerically for a numeric field, else alphabetically. |
| `columns` | list of strings | the default ones | A table's columns, in order: `id`, `title` (without a trailing `(#2518)` that is the issue's own number: `links` shows it), `is` (open or closed), any field, `links` (the issue on each forge), `other` (labels without a namespace). Without it, the table shows id, forge links, title, state, every field and other, so a new field gets its column. Set from the table's **Columns** button and Save view. |
| `lanes` | list of values | every value | A board's columns: which values of `group` it shows, in this order; `"(none)"` for the issues without one. The other issues are left out, and the board says how many. Set from the board's **Lanes** button. |
| `limits` | inline table | none | At most this many cards in a lane, work in progress: `limits = { in-progress = 5 }`. The lane shows "4 / 5", amber at the limit, red over it. |

### Boards per role

The same issues, a board for each way of working: each view keeps the
lanes its people move cards between.

```toml
[views.dev]
layout = "board"
group = "status"
lanes = ["ready", "in-progress", "in-review", "ready-for-validation"]
limits = { in-progress = 5 }

[views.pm]
layout = "board"
filter = "is:open pm:@me"
group = "status"
lanes = ["(none)", "backlog", "comite", "ready", "qa"]

[views.qa]
layout = "board"
group = "status"
lanes = ["comite", "ready", "qa", "tests-metier", "validated"]
```

On a board, Cmd- or Ctrl-click picks several cards (Shift-click a range
in a lane, or the box on a card's corner), and dragging one of them moves
them all; a lane's ⋯ menu moves every card it holds.

## Roles

A project's people by role, as the team's roles page names them: the lead developer, the project manager, the code reviewer. Every issue takes them, unless it sets its own.

```toml
[roles]
lead-dev = "bob"
pm = "claire"
reviewer = "alice"
```

A role is a `person` field of the same name: `lead-dev:dan` on an issue overrides the project's `bob` for that issue, and removing it gives `bob` back. Filters, boards and the filters panel see the inherited value: `lead-dev:@me` finds the issues that take you as lead dev, the PM's board above (`pm:@me`) every issue while the project names them PM, and `no:lead-dev` finds none while the project names a lead dev. The side panel shows an inherited value greyed, "bob · project". `assignee` is the contributing developer, set per issue.

The inbox brings a role you hold for the project only the project's new issues and closes, not their discussions: the PM of a project isn't in every comment. A role set on the issue itself (`lead-dev:dan`) follows it like an assignee does. `involves:@me` and `follows:@me` count the issue's own roles, not the project's.

| Key | What it is |
| --- | --- |
| a role (`lead-dev`, `pm`, `reviewer`, …) | One login. The name is a field's: lower-case letters, digits and `-`; declared in `[fields]`, it is a `person`. |

## Filters

One syntax everywhere: a view's `filter`, the board's **Filter** box, ⌘K, and the command line (`git-board ls`, `git-board tui` and its `f`). Conditions combine with AND, as in GitHub's search; a leading `-` negates any of them.

| Syntax | Matches |
| --- | --- |
| `status:ready` | issues with the label `status:ready` |
| `-status:ready` | issues without it |
| `status:ready,qa` | either value, as in GitHub's search; works with `is:`, `label:` and `author:` too |
| `sprint:@current`, `@next`, `@previous` | the sprint of today, the next or the previous one, from `[[sprints.list]]` |
| `assignee:@me` | you: your git-bug identity's GitHub or GitLab login |
| `is:open`, `is:closed` | open or closed issues |
| `title:*spec*` | a title matching the pattern |
| `author:guilbep`, `author:@me` | who opened the issue |
| `involves:guilbep`, `involves:@me` | issues someone is in: the author, a person field (assignee, reviewer…), a comment, an @mention |
| `follows:@me` | what your inbox follows: `involves:@me`, the issues you followed and the views you follow, without those you unfollowed |
| `label:bug` | a plain label, or a full one: `label:status:ready`; `label:bug` also finds the kind `bug` when the kinds are plain labels (see Branches) |
| `no:assignee` | no value for that field; `no:label`: no plain label (a plain kind is the kind field, not one) |
| `#2528` | the issue with that number on a forge |
| `travel`, `"car sharing"` | words in the title, the description or a comment; a number (`2528`), or the first 7 characters or more of a git-bug id, also finds that issue |

An empty filter is every issue, open or closed. A view without one shows the open issues (`is:open`), as `git-board ls` does without one.

On the board, the Filter box holds the view's filter until you change it; the address then keeps it (`?view=backlog&filter=…`), so a link shows the same issues. The filters panel counts each field's values against the rest of the filter, and a tick writes into the box: ready and qa ticked is `status:ready,qa`, Not set `no:status`, Not set with values the values left out (`-status:validated`). ⌘K takes the same filter on every issue, the open ones first, and its first row, **Filter the board by**, puts it into the box; `#2528` or an id opens that issue at once. Typing `status:` in either offers the field's values. On the command line, quote a filter: `git-board ls "#2528"` (the shell takes an unquoted `#` for a comment).

## Sprints

Sprints are a field like any other (`sprint:s12`). The list gives them dates, so that `sprint:@current` finds today's sprint.

```toml
[sprints]
length_days = 14

[[sprints.list]]
slug = "s12"
title = "Sprint 12"
start = 2026-10-05

[[sprints.list]]
slug = "s13"
title = "Sprint 13"
start = 2026-10-19
```

| Key | Type | What it does |
| --- | --- | --- |
| `length_days` | number | The length of the last sprint, whose end has no next start. Also what **+ Sprint** adds to the last start. |
| `start_weekday` | string | Read, not used yet. |
| `slug` | string | The label's value: `sprint:s12`. |
| `title` | string | The name shown. |
| `start` | date, `YYYY-MM-DD`, no quotes | First day. A sprint lasts until the next one starts. |

**+ Sprint** adds the next one after the last: its start plus `length_days`, its number plus one.

The `sprint` field follows these dates: in the filters (sorted by **Dates**, today's sprint tagged *current*), on a board grouped by sprint, in a table's sections and when sorting. In an issue, the sprint is picked from the list. Give `[fields.sprint]` its own `options` to choose another order.

## GitHub Project

The GitHub Project whose fields (status, priority, sprint…) `git board bridge pull` and `sync` turn into labels and back.

```toml
[github-project]
owner = "guilbep"
number = 1
write = true
```

| Key | Type | Default | What it does |
| --- | --- | --- | --- |
| `owner` | string | none | The user or organization owning the Project. |
| `number` | number | none | The Project's number, as in its address. |
| `write` | true / false | false | Lets a sync write git-board's changes to the Project. Off, the Project is only read. |

The token needs the `project` scope: `gh auth refresh -s project`.

## Attachments

Files attached to comments go to an S3 bucket that a CDN serves; the comment gets a link to them. The keys stay out of this file: set `ACCESS_KEY_ID`, `ACCESS_KEY_SECRET` and `BUCKET_NAME` where the git-board server (`git board`, `git board serve`) runs. Without them, attaching is off.

```toml
[uploads]
prefix = "git-board/co2-calculator"
```

| Key | Default | What it does |
| --- | --- | --- |
| `endpoint` | `https://s3.epfl.ch` | The S3 service. |
| `public_url` | `https://enacit4r-cdn.epfl.ch` | Where the bucket is served. |
| `prefix` | `git-board/<repository>` | The folder in the bucket. |
| `region` | `us-east-1` | The S3 region. |

Files are public to anyone with the link, even for a private repository, and stay cached on the CDN for up to a year after a delete.

## Branches

An issue's **Development** section lists the branches whose name names it: *merged into dev* when the branch's last commit is in `dev`, *in dev, squashed* when its changes are there by a squash or a rebase (what `git merge-tree` makes of the two is `dev` as it is), else *not in dev*. Merging with merge commits (`--no-ff`) keeps the first one exact. **Create branch** gives the command creating its branch (`git board branch <id>` prints the same). git-board creates and pushes no branch.

When `origin` is on GitHub, each of these branches that origin has carries its **gate**: the pull request whose head it is, found on GitHub by the branch's name (never stored), its checks (the base's required ones first, from its rulesets, a required check that hasn't reported yet included), who approved and at which commit, and GitHub's review decision. **Merge into dev** is enabled when GitHub says the pull request can merge, and otherwise says what it waits for ("Frontend Tests, an approval from alice"); it merges with a merge commit, and only if the head is still the commit the page showed. **Open PR** opens one into the base, titled `CO2-3075 <title>`, its body naming the issue and, when the issue is one of origin's GitHub issues, `Closes #<its number>` (GitHub closes it when the pull request merges into the default branch). A branch only here shows the `git push -u origin <name>` to copy. The page asks GitHub again every 15 seconds while a check runs.

Open PR and Merge act on GitHub as you: with the token in git-bug's keyring whose login is your identity's GitHub login (its `github-login`, else the GitHub bridge's link, else its own login; add one with **Add a forge** in Settings › Forges). Without one, the gate says `no GitHub token for <login>` and uses no one else's. It is the account the bridge posts your comments under, not the `gh` CLI's, which can be logged in as someone else: GitHub refuses an approval from a pull request's author, so who opens and who approves must be told apart.

The full issue page has a **Changes** tab: the branch from where it left the base (the merge base with `origin/<base>`) to its head, origin's copy when origin has the branch, since that is what the pull request and its reviewers see. A picker shows all of it, what changed since your last review, or one commit. Files come in three groups: *To review* (those under 1,000 changed lines open by themselves), *Large* (over 3,000 changed lines, folded, the first 5,000 lines when opened, and the `git diff` to copy), and *Generated* (`linguist-generated` in `.gitattributes`, as on GitHub: folded and left out of the viewed count). **Viewed** marks are kept in this browser, per file version: a push un-views only the files it changed. Click a line number to comment on that line; a comment shows under its line, quoted in the conversation with its `path:line`, and *outdated* once a later commit changed that line (it then stays above the file's diff). Replies and **Resolve** work on the thread; **Comment**, **Approve** and **Request changes** submit a review at the head, and each person's last verdict shows with how many commits ago it was. All of these are issue comments carrying `review:*` metadata (path, line, side, commit, the pull request, the comment answered, the verdict, resolved), so they are in git-bug at once. When the branch has an open pull request on origin, the sync sends them to it: line comments as review comments on their line, reviews as GitHub reviews; approvals and change requests go only under their author's own token, so they count for a code-owner rule. Comments made on the pull request come back to the issue on their line. Resolving on GitHub doesn't come back.

```toml
[branches]
base = "dev"
name = "{kind}/{number}-{slug}"
kinds = { bug = "fix", feat = "feat", perf = "perf", task = "chore" }
```

| Key | Default | What it does |
| --- | --- | --- |
| `base` | the remote's default branch, else `main`, `master` or `trunk` | The branch a new branch starts from (`origin/<base>`, or `<base>` without a remote), and the one "in dev" and ahead/behind compare with. |
| `name` | `"{number}-{slug}"` (GitHub's) | The branch's name. `{number}`: the issue's project number with its prefix, in lower case (`vm-62`), once the project numbers its issues (see Numbers), else its number on its first forge, else its git-bug id's first 7 characters. `{kind}`: its kind, through `kinds`; without one, `{kind}/` is dropped. `{slug}`: its title in lower-case words, without a leading `[BUG](Scope)` or a trailing `(#1234)`, cut at about 50 characters. |
| `kinds` | none | The project's kinds, each to the `{kind}` of its branch. They are the `kind` field (a column, a group, a filter, one per issue) in either of two forms. Without `options` in `[fields.kind]`, the default, a kind is GitHub's plain label (`bug`), shown and filtered as `kind:bug`, in `kinds` order; `[fields.kind.colors]` and `descriptions` still apply. With `options` in `[fields.kind]`, the kinds are `kind:bug` labels like any field's. `bridge pull` and `sync` fill a missing kind from the Project's Category (`FEAT` → `feat`); `git board lint` reports an issue with two kinds, a title whose `[BUG]` names another kind than its label (the label wins), and how many have none. |

`[BUG](Equipment) Other equipment shows the wrong example (#2518)`, labeled `bug`, gives `fix/2518-other-equipment-shows-the-wrong-example`. What git doesn't take in a branch name becomes `-`.

A branch names an issue when its first number is the issue's number, with or without the project's prefix (`fix/vm-62-…`, `fix/62-…`, `2518-…`: its project number once the project numbers its issues, else a forge's), or when a word in it starts the issue's git-bug id (7 characters or more). Local branches and the remotes' (as last fetched) are listed.

## Cycle

An issue's page is its **step** in the development cycle (docs/prd/cycle-protocol.md). Under the title, the step bar runs through the seven steps, Intent, Risk, Plan, Tests, Code, Review and Verify: a done step reopens when clicked, a later one is locked and says why ("opens when the PR is ready for review"), and a skipped one says so. The current step is the page's main section: at Intent the description, its criteria and Move to ready; at Risk the three classes; at Plan the plan file, who drafted it, Approve the plan or Ask the agent to revise; at Code Mark ready for review; at Review the PR and its changes; at Verify the full run, the criteria to tick, the sign-off tiles, Sign off or Send back to in-progress. A status bar under it gives the branch and its head, the PR and its checks, and what waits for the forges. The right column is the discussion: **This step** (the comments posted on that step), **All** (every comment and event in order) and **History**, with the fields under it. Each step's button moves the status, as the next table says. Under 900px the step bar shows numbers only, with the current step's name, and the discussion goes under the step. The step is never stored: the server computes it on every read from the status and the evidence. The status is the floor (its stage gives the first step it can be at), and the evidence moves the step within that stage, never below it. Evidence that disagrees with the status shows after the line ("plan not approved"), it never makes another step.

```toml
[fields]
risk = { options = ["low", "normal", "structural"], single = true }

[cycle]
ready = "ready"
in-progress = ["in-progress"]
in-review = ["in-review"]
validation = ["ready-for-validation", "qa", "tests-metier"]
criteria = ["Acceptance criteria", "Done when", "Expected"]
plans = "docs/src/implementation-plans"
plans-in-pr = true
risk = "pm"
plan = "lead-dev"
review = "reviewer"

[cycle.kinds]
bug = "failing-test"
feat = "per-criterion"
doc = "none"
chore = "none"

[cycle.signoffs]
ready-for-validation = "lead-dev"
qa = "pm"
tests-metier = "metier"
```

Every key has a default, so a project writes only what the guess got wrong, and Settings › Project file says what was guessed. The stages are guessed from the status options, in their order, on the words of each name (split at `-`, `_` and spaces). The last option is the closing status. A word among `validation`, `qa`, `uat`, `acceptance`, `test`, `tests`, `metier`, `verify` or `staging` makes a validation status; `review`, `reviewing` or `pr` makes in-review; `progress`, `doing`, `wip`, `dev`, `development`, `implementing` or `coding` makes in-progress; the first with `ready`, `todo`, `planned` or `selected` is ready. Every other status is before ready. With co2-calculator's nine statuses the guess is the table above. The roles are unset by default, and a gate whose role is unset, or names nobody, is anyone's turn.

| Key | Default | What it does |
| --- | --- | --- |
| `ready` | guessed | The status an issue with acceptance criteria waits at: steps 1 to 3 (intent, risk class, plan). |
| `in-progress` | guessed | Steps 4 and 5: the failing test, then the code and the PR. |
| `in-review` | guessed | Step 6: the review. |
| `validation` | guessed | Step 7, one sign-off per status, in the options' order. Signing off the last one moves the issue to the closing status and closes it. With none, Verify happens in in-review once the PR is merged, and its sign-off closes the issue. |
| `criteria` | `"Acceptance criteria"`, `"Done when"`, `"Expected"` | The headings a description's acceptance criteria are listed under, any heading level or a bold line, in any case, with or without a colon. The first one the description has counts. |
| `plans` | `"docs/src/implementation-plans"` | Where plan files live: Markdown whose front matter's `issue:` names the issue (`2518`, `"#2518"`, `CO2-2518`, or its git-bug id's first 7 characters), and whose `status:` (`accepted`, `in-progress` or `delivered`) approves the plan. |
| `plans-in-pr` | `true` | The agent started at step 3 commits its plan and opens a draft PR holding it; Review opens once that PR is marked ready. `false` keeps plans out of PRs: the plan is committed on the branch and the PR opens at step 5. |
| `tests` | `*_test.go`, `*.test.ts`, `*.spec.ts`, `test_*.py`, `tests/**` | Which changed files are tests, by path or base name; `dir/**` is everything under `dir`. |
| `risk` | none | The role (`[roles]`) that sets the risk class. |
| `plan` | none | The role that approves the plan. |
| `review` | none | The role that reviews the PR. |
| `signoffs` | none | Each validation status to the role that signs it off. |
| `kinds` | `bug = "failing-test"`, `feat = "per-criterion"`, `doc = "none"`, `chore = "none"` | Step 4's rule by the issue's kind (the `kind:` field, or a flat label in `[branches] kinds`): a test that fails before the fix; the suites the diff touches green and each criterion naming its test; or no tests, which skips step 4. A file's entries go over the defaults. No kind, or one the table doesn't list, is `per-criterion`. |

Coming on later branches, and a warning in Settings until then: `[cycle.tests.<suite>]` and `[cycle.checks.<check>]`, the suites step 4 runs and the checks that gate Mark ready for review (M4).

Each step's button moves the status, through the same edit as the status field:

| Button | Who | Moves the status to |
| --- | --- | --- |
| Move to ready | the author, once the description lists a criterion | `ready` |
| Approve the plan | the `plan` role | the first `in-progress` |
| Start work | the assignee, at low risk or once the plan is approved | the first `in-progress` |
| Mark ready for review | the assignee, with a test, or at once when step 4 is skipped | the first `in-review` |
| Start verification | the assignee, once the PR is merged | the first `validation` |
| Sign off | the status's role | the next `validation`, then the closing status |

A card dragged on the board still moves the status: the step follows it, with a note when the evidence disagrees.

The step reads:
- **The status** (the `status` field).
- **The acceptance criteria:** the list under the first of the `criteria` headings the description has (the format is below). With none, the issue is at step 1.
- **The risk class:** the `risk` field, `low`, `normal` or `structural`. Low skips step 3.
- **The plan file**, read from the evidence branch, then the `plan/` branch, then the base.
- **The tests:** the files the evidence branch changed (`git diff base...branch`) that match `tests`. The evidence branch is the newest branch naming the issue that isn't a `plan/` branch.
- **The PR,** read from GitHub only when the issue is at in-review or later (or structural, for the plan branch's PR), at most every 5 minutes per branch. When GitHub can't be reached, the step still computes, with the last PR it read.
- **The kind**, for step 4's rule.
- **The comments with metadata** that the step writes. Each carries its step as `cycle:step` (`1` to `7`), as does a comment posted on a step's discussion; a comment without it is step 1's.

The criteria in a description:
- **The heading** is one of `criteria`, as a Markdown heading of any level (`## Expected`) or a line in bold (`**Done when:**`), in any case, with or without a colon. The whole line is the name: `## Expected results` isn't `Expected`. The first one in the description counts.
- **The list** starts at the first line under it that isn't blank. Each item is a line starting with `-`, `*`, `+`, `1.` or `1)`; a checkbox (`- [ ]`, `- [x]`) is dropped from its text. Blank lines between items are fine.
- **The list ends** at its first line that is neither blank nor an item: a paragraph, or the next heading.

```markdown
### Expected

- every row of the file is imported
- [ ] an empty file says so
```

Intent warns, naming the line, where the description reads differently from how it looks. The warnings never block a step:
- a second criteria heading: it and its list are ignored;
- a line that stops the list while items follow before the next heading: those items don't count;
- a sub-item: counted as a criterion of its own;
- a criteria heading with no list right under it.

The card writes:
- **Set the risk:** a field edit.
- **Approve the plan:** a `plan:approve` comment. A structural plan is approved on its PR on GitHub instead.
- **Post the run, and tick a criterion:** `verify:run` and `verify:criterion` comments, with the commit they were made at.
- **Sign off:** a `signoff:status` comment, then the next status. A sign-off needs a run at the branch's head, or a reason, written as `signoff:why`.

A comment is the viewer's when git-board's identity wrote it, since the server writes as that identity. Anyone else's comment counts under its author's forge login. The server refuses a card action from anyone but the gate's person with `waits for the <role> role: <login>`.

### The Work page

**Work** (`/work`, first in the header) is what needs a person, computed from every open issue's step. **Mine** is the viewer's: **Needs you** lists the issues whose gate waits for the viewer, or where another person asked them something, oldest wait first, with the step, what it waits for, since when, and the action (the three risk buttons, Answer, Read the plan, Review, Sign off, or Open); **Waiting on others** lists the issues the viewer is involved in (author, or named by a field such as `pm:alice`, on the issue or inherited from `[roles]`) whose gate waits for someone else. **Team** is every open issue: anyone's gates join Needs you, and the rest join Waiting on others. An issue is in one group at most. **Agents** is one row per linked git worktree (`git worktree list`): its branch, its last commit, and the issue whose evidence branch it is, with that issue's step and state. **To send** counts what waits for the forges, as the Sync page does, with Send all and Check first. **Since the last retro** counts the comments with `correction` metadata on open issues since the current sprint's start (`[sprints]` is the clock; without one it says so).

A question is a comment by someone else than the viewer with `ask` metadata, or, failing that, whose text ends in a question mark; the viewer's next comment answers it. Since is the issue's last state change (a status, label, title or card comment), not a plain comment. `GET /api/work?who=me|team` returns the groups; it needs an identity with a login, as the Inbox does.

## Worktrees

git-board starts agents with [wtx](https://github.com/EPFL-ENAC/wtx): one agent, one worktree, one branch, one PR. wtx, `wt` and `tmux` run on the machine where `git board serve` runs, and git-board calls wtx there, with an argument list, never a shell line.

**Settings › Worktrees** says whether wtx, wt and tmux are on that machine (`wtx doctor --json`), then **Set up wtx**: what wtx read in the repository (`wtx init --edit --no-notes`, the existing `wtx.toml` merged with a fresh scan), to correct before writing. The base branch, the protected branches, the agent and its plan and build models are edited; the ports and panes are shown as wtx read them. **Who commits** is a deny rule in `[permissions] deny`: *A person commits and opens the PR* denies `Bash(git commit *)`, `Bash(git push *)` and `Bash(gh pr create *)`; *The agent may commit; a person pushes* denies `Bash(git push *)` alone. **Never allowed** is the other deny rules, added to wtx's own (force pushes, pushes to the protected branches, changing the remote). **Preview the files** runs `wtx init --from-json` in a scratch repository and shows `wtx.toml` and `.wt.toml`; **Write wtx.toml** runs the same command in the main checkout with `--force`, so the written files are the preview byte for byte; wtx also adds its lines to `.gitignore`. A missing who-commits rule is written as *A person commits*, the radio's default. They are left uncommitted: you commit them. git-board writes no TOML itself.

**Start an agent**, under an issue's step and on the Work page for an issue without a worktree, writes a prompt file and runs `wtx go <branch> --prompt <file> --no-attach`. The branch is the issue's evidence branch, else the `[branches]` name for it. **Brief the agent**, once the worktree exists, runs `wtx brief <branch> --prompt <file>`; on the plan step, **Ask the agent to revise** posts its comment and sends it as a brief too. The prompt file holds git-board's words first (the issue, its step, the gate, what is asked now, the branch, the plan file, and what you typed), then the issue's title and description under the heading "The issue, as the forge holds it", every line quoted with `> `: anyone who can write an issue on the forge writes them, so they are data for the agent, not instructions. The agent's permissions come from `wtx.toml` alone; git-board passes no agent, model or permission flag.

wtx runs only when the server listens on loopback alone (not `--addr 0.0.0.0:…`), `wtx` is on its `PATH`, and, for a write (Preview, Write, Start, Brief), the request carries this board's own Origin. Otherwise the page says why: `exposed`, `no-wtx`, `origin`; without a `wtx.toml` the page says "No wtx.toml in this repository yet". On the Work page, **Agents** adds each worktree's session from one `wtx status --json`: running, asks permission, waits for a brief, done, or down. `GET /api/worktrees`, `POST /api/worktrees/preview` and `/init`, `GET /api/issues/{id}/agent` and `POST /api/issues/{id}/agent/go|brief` are the endpoints.

## Numbers

A project can number its issues itself, `CO2-3075`, instead of leaning on a forge's numbers. The counter is in git, `refs/board/seq` on `origin` (never a mirror): no key here. Start it once, for everyone, from **Settings › Numbers** (`#/settings/numbers`): it suggests a prefix from the repository's name (`git-board` → `GB`, `co2-calculator` → `CO2`) and a start above every number it knows (your issues' forge numbers, and GitHub's newest issue or pull request when a GitHub forge answers through `gh`), shows what your issues become, and names what it pushes and where before **Start numbering**. Once started, it says how many issues wait for a number, and **Number them now** numbers those origin has. Or from a terminal:

```bash
git board numbers init --prefix CO2 --start 3052
```

Choose the prefix once: issues carry it wherever a forge could read the number, in branches (`fix/co2-3075-…`), pull request titles, commits and messages. A bare number collides with the forge's own: GitHub links `#3075` or a branch's `3075` to its issue 3075. `start` is one more than GitHub's highest issue or pull request number (`gh api 'repos/OWNER/REPO/issues?state=all&per_page=1' -q '.[0].number'`). GitHub numbers below it stay the issues' own: `#2518` is `CO2-2518`, even for an issue imported later. Every other issue, made here or brought in from a forge, gets the next number with the next push to `origin` you ask for: **Push** on the Sync page's Remotes, **Number them now** in Settings › Numbers, `git board sync`, or `git board numbers`. The counter only names issues origin has, so other clones never see a number for an issue they can't get: `git board numbers` numbers what was pushed already, and a sync or Push pushes the issues first. Until then an issue shows its short git-bug id. Nothing pushes the counter on its own: not creating an issue, not a forge sync. Pushing the counter is the lock: when two clones number at once, git refuses the second push and that clone takes the next number.

After the switch, a bare number (`3075`, `fix/3075-…`) and `CO2-3075` are the project's number; `#3075` is always a forge's. git-board always writes a project number with its prefix; `#N` is only ever a forge's. Cards, rows and the issue page show the number before the title.

## Release

`[release]` is how the project's code reaches its environments: the ordered `environments`, the `checks` a merge must pass, what to build (`images`, `chart`) and the config repos holding each environment's overlay (`[[release.deploy]]`). Without `environments` or `binaries`, the project has no releases. See [the release design](prd/release.md).

A Go program with no deploy targets lists `binaries` instead: the main packages to build per platform and publish as archives on the forge's release. The Releases page writes these for you: **Enable binary release** infers them from the repo.

| key | meaning |
|---|---|
| `binaries` | main packages, `["./cmd/git-board"]`. A table with binaries and no environments is enabled: every cut is a final, deployed nowhere. |
| `platforms` | `os/arch` to build for; default `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64`. |
| `build` | a command run with `sh -c` in a clean checkout of the tag instead of the Go builds, `"make dist"`; it leaves the archives in `dist/`. |

The environments have roles, from their order. The one that **follows** the trunk takes its head (`dev` when listed, else none). The **last** takes final releases. Those **between** take candidates. One environment that is the followed one has no releases.

```toml
[release]
remote = "origin"
environments = ["dev", "stage", "prod"]
checks = "make ci"
images = [
  { name = "frontend", context = "./frontend" },
  { name = "backend", context = "./backend" },
]
chart = { path = "./helm", registry = "oci://ghcr.io/epfl-enac/co2-calculator/helm" }
version_values = ["backend.env.APP_VERSION"]

[[release.deploy]]
repo = "git@github.com:EPFL-ENAC/enack8s-app-config.git"
path = "epfl/co2-calculator/overlays/{env}"
registry = "ghcr.io/epfl-enac/epfl/co2-calculator"

[[release.deploy]]
repo = "git@github.com:EPFL-ENAC/openshift-app-config.git"
path = "epfl/co2-calculator/overlays/{env}"
registry = "quay-its.epfl.ch/svc1751"
```

| Key | Default | What it does |
| --- | --- | --- |
| `remote` | `origin` | The authority: tags and release records are pushed there, and a refused push is the lock. Other remotes are mirrors. |
| `environments` | none | The environments in order, each once. The last takes finals, the ones between `follow` and the last take candidates. |
| `follow` | `"dev"` when listed | The environment that follows the trunk's head. It must be one of `environments`; `follow = ""` means none does. |
| `checks` | none | The command run on a merge's exact tree before it reaches the trunk (`make ci`). |
| `images` | none | The images to build, each a `name` and a docker build `context`. |
| `chart` | none | The helm chart: its `path` and the OCI `registry` it is pushed to. |
| `version_values` | none | Dotted paths under the chart entry's `valuesInline` that get the version as a string (`backend.env.APP_VERSION`). |
| `builder` | the local buildx builder | A remote buildx builder to build on. |
| `deploy` | none | One `[[release.deploy]]` per config repo: its `repo`, the overlay `path` (with `{env}`, replaced by the environment) and the `registry` of its images. |

A deploy edits the overlay's `kustomization.yaml` and nothing else (comments, key order and other entries stay): the `images[]` entry named `<registry>/<image>` gets `newTag` and `digest`; the `helmCharts[]` entry whose `repo` is the chart's `registry` gets `version`, and each `version_values` path under its `valuesInline` gets the version; `commonAnnotations` `git-board/commit` and `git-board/version` record what runs.

## Repos

```toml
[repos]
list = ["EPFL-ENAC/co2-calculator"]
assignees = "native"
```

`list` is the GitHub repositories whose labels the project keeps; without `[repos]`, origin's when it is on GitHub. They are reached through the `gh` CLI, logged in as you.

**Settings › Labels** lists each label the project defines (every field's `options`, the sprints, the kinds) with its color, description and open issues, and its state on each repository: *same*, *color differs*, *description differs* or *missing*.

- **Adopt** writes GitHub's color and description of a label into `[fields.<name>.colors]` and `[fields.<name>.descriptions]`, and opens the Project file tab to review and save. An organization's labels keep their colors this way.
- **Send to GitHub** first lists, per repository, each label it creates and each change, then creates the missing labels and updates those whose color or description the file sets (`gh label create … --force`). A label outside the project's is never touched, and one the file gives no color or description of its own keeps GitHub's.
- **Lint** shows `git board lint`'s findings, then the GitHub labels outside the project's (a `bug` beside `kind:bug`, GitHub's old defaults) with their issues. **Delete** removes one from its repository, after a confirmation that says how many issues lose it.

`git board lint --create-labels` prints the same plan, and `--apply` carries it out.

`assignees` is how the forges hold an issue's `assignee:<login>` values: `native` (the default), as the forge's own assignees, or `labels`, as `assignee:<login>` labels. Before each real forge sync, git-board writes it into this clone's git config, `git-bug.bridge.<name>.assignees` for each bridge, so a plain `git bug bridge sync` here agrees. When it changes, the check says so, and the sync after it pulls every issue again, so the assignees already there move to the new way. Every clone must use the same mode, or their syncs undo each other on every run: CI, or another clone running `git bug bridge sync` without git-board, must set the same key.
