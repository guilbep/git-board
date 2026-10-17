# Inbox: a feed you check, not mail you receive

Replaces the email-notifications investigation (`email-notifications.md`).
Status: design approved, plan next. Mockup (private to the author until
shared): https://claude.ai/artifact/Xh5BK2s82ysTfaMw9a72TX

## Why not email

Grilled before designing:

- Where a forge mirror exists, the forge already emails followers and turns
  replies into comments, which the sync pulls back. What it can't see is
  what git-board adds: person fields as labels (`assignee:alice` notifies
  nobody), comments that stay local, sprints.
- Sending needs an always-on server that doesn't exist yet (no hosted, no
  auth), and identities often carry no email.
- Reply-by-email from Outlook (HTML, top posting, "Le … a écrit :", Safe
  Links) is the swamp patches by email already sank in.
- One email per event gets filtered into a folder within a week.

Five paths were weighed: let the forge do it, an inbox you check, the
writer announces in Teams, a CI digest, email for outsiders (a helpdesk).
The inbox won: it needs no sender, no addresses and no privacy rules, and
"who follows what, and what's new for them" is the core every other path
would need anyway. Teams, a digest or email can deliver it later.

## Decisions

| Question | Decision |
|---|---|
| Who it's for | developers first; a lead or PM if they open git-board; outsiders out (a helpdesk is another product) |
| Feed or triage | **feed**: an issue shows when something new happened; opening it clears it |
| Where | web first; state in a file in the clone, so the CLI/TUI can join later |
| What counts | conversation and what changes your relation to the issue; field changes optional (a personal setting) |
| Follow | automatic, plus Follow/Unfollow per issue, plus following a whole view |
| One query language | following is filter terms, `involves:` and `follows:@me`, for the inbox, the board and ⌘K alike (agreed with the main session) |
| Freshness | opening the inbox checks (fetches) the git remotes and offers "N new · Pull" as one click; no automatic pull, no timer, forges stay manual (agreed with the main session) |
| Where computed | the server, on request (`internal/inbox`) |

## 1. Data

**Me** is the identity git-board writes as (`Backend.Me()`). Authorship is
matched by its identity ID; person fields and mentions by its login,
found the way `@me` finds it (`MyLogin()`, `--me` first), ignoring case.
**You are also every identity carrying that login**: old bridges and the
sync can leave several for one person (the fork's `resolveAll`), and your
own forge comments come back under one of them; your own events, `author`
and `commented` match any of them. No identity set: the inbox says so
(`no-identity`) and links to Settings. An identity without a login (no
bridge tagged it, no `--me`): person fields and mentions can't match, so
the inbox says so (`no-login`, as `fields.go`'s "@me needs your forge
login") instead of looking thin.

**One file per identity**, in git-bug's local storage
(`Backend.Storage()`): `git-board/inbox/<identity-id>.json`. Shared by the
clone's worktrees, never pushed. Switching identity switches inbox; hosted
later has one file per person in the same shape.

```json
{
  "seen":     { "<issue id>": ["<event id>", "..."] },
  "follow":   ["<issue id>"],
  "unfollow": ["<issue id>"],
  "views":    ["backend"],
  "fields":   false
}
```

- **`seen` holds event IDs, not a time**: a comment written offline
  yesterday and pushed today carries yesterday's time, so a seen time would
  hide it. Every timeline event has an ID (`CombinedId`).
- **A 14-day window**: only issues whose `UpdatedAt` is within 14 days are
  loaded, and only events whose own time is within 14 days count (an issue
  updated yesterday doesn't bring back its comments from March). Entries
  of `seen` for issues outside the window are pruned on save. First open:
  two weeks of what concerns you, cleared with Mark all read.
  ponytail: a change arriving more than 14 days late never shows.
- Written through a temporary file and a rename; an unreadable file is an
  error naming it, never silently reset (it holds your follows).

## 2. Rules

**You follow an issue** when any of these holds, unless you unfollowed it:

| Reason (as shown) | From |
|---|---|
| `author` | the create event's author is you |
| the field's name (`assignee`, `reviewer`, `pm`…) | a current label `field:yourlogin` on a field whose `TypeOf` is person |
| `commented` | a comment of yours |
| `mentioned` | `@yourlogin` in the description or a comment (the fork's `bridge/core/render.go` `mentionRe`, which is unexported: copied, as `web/src/richtext.ts` already does) |
| `followed` | you clicked Follow |
| `view: <name>` | the issue matches a followed view's filter without its `is:` terms (`filter.Without`, as `insights.go` does), so a close still shows |

**The same rules are filter terms** (`internal/filter`), so the board and
⌘K can ask them too:

- `involves:<login>` or `involves:@me`: author, any person field holding
  the login, a comment by it, a mention of it. Works for anyone
  (`involves:alice`).
- `follows:@me`: what your inbox follows: `involves:@me`, plus Follow, plus
  followed views, minus Unfollow. Only `@me`: the follows are yours.
- One function, `filter.Involvement(…, login) []string`, returns the
  reasons (`author`, a field's name, `commented`, `mentioned`); the term is
  "any", the inbox's chips are those plus `followed` and `view: <name>`.
  It takes a small struct both an `Issue` and a `Thread` can fill, so the
  list and the inbox can't disagree.
- `Issue` gains `CommenterLogins` from the excerpt's `Participants`
  (cheap, as `AuthorLogins`) and `MentionLogins`. Mentions need the text:
  the git-bug cache's full-text index narrows to the issues containing the
  login, the mention pattern confirms on their text. **Verify first** in
  the plan how the index tokenizes `@login`; if it can't narrow, the term
  loads the texts and we measure.
- `follows:@me` needs your file: `Matches` gets it from its caller as plain
  sets (Follow, Unfollow, the followed views' issues; no import of
  `internal/inbox`). Followed views are matched without their `follows:`
  terms, so a view can't follow itself.

The inbox still loads every issue in the window (§3), not only
`follows:@me`'s: an event addressed to you can sit on an issue you no
longer follow (you were just removed as assignee).

**What shows**, among unseen events in the window that aren't yours:

- On a followed issue: a comment; the issue's creation; closed or reopened.
- With `fields` on: also any label change on an issue where you're
  currently in a person field.
- **Addressed to you, followed or not, unfollowed included**: a label
  change adding you to or removing you from a person field; a comment or
  description mentioning you. Unfollow mutes the issue's activity, not
  what's addressed to you.
- Never: your own events, title edits, description and comment edits, and
  (with `fields` off) other label changes. ponytail: a comment edited to
  add `@you` keeps its event ID, so it isn't seen again.

**A row per issue**: number, title, status; why you follow it (the reasons);
what's new as counts ("2 comments · closed · added you as reviewer ·
mentioned you · new issue · 3 field changes"); who acted (the events'
distinct authors); the newest event's time (rows sort by it, newest first);
the oldest of those events' ID (the panel scrolls there).

**Clearing**: opening an issue marks every event it has now as seen,
counted or not; Mark all read does it for every row.

## 3. API

| | |
|---|---|
| `GET /api/inbox` | `{ me, rows, fields, views, views_missing }` |
| `POST /api/inbox/seen` `{"issues": [...]}` | marks every current event of those issues seen |
| `GET /api/inbox/follow/{issue}` | `{ following, reasons, set }`; `set` is `true`/`false` once clicked, `null` when automatic |
| `PUT /api/inbox/follow/{issue}` `{"follow": true\|false\|null}` | Follow, Unfollow, back to automatic |
| `PUT /api/inbox/settings` `{"views": [...], "fields": bool}` | |

- **No identity**: `409`, code `no-identity` (`writeCoded`); no login:
  `409`, code `no-login`. Unreadable
  file: `500` naming it. A followed view gone from `.git-board.toml`:
  listed in `views_missing`, not an error.
- **`internal/inbox`**: the file (load, save) and one pure function,
  `Rows(me, state, threads, inViews) []Row`, holding every rule of §2.
- **The server** gathers its inputs: `ListIssues` within the window,
  `Thread` for each, `board.LoadMatching` for each followed view. One mutex
  around each read-change-write of the file. The last result is kept, per
  identity (switching who you write as changes git config, not refs),
  until `RefsFingerprint`, the file or the config changes, so the header's
  count doesn't recompute on every event.
- **`eventFor`**: `/api/inbox` gets its own `inbox` kind, so marking an
  issue seen in one tab updates the count in the others.

**Checking and pulling** are the page's, not the server's. Opening the
inbox: `GET /api/remotes`; `POST /api/remotes/{name}/fetch` for each remote
not checked in the last 5 minutes (its `checked`), all at once: a fetch,
nothing local changes; then `GET /api/inbox`. A remote with
`issues.receive` > 0 shows "3 issues new on origin · **Pull**": one click,
`POST /api/remotes/{name}/pull`, then the rows reload. Refresh checks
again regardless of the 5 minutes. No automatic pull: a pull merges into
the local issues, and the Sync page's rule is that a run which changes
something is one you asked for. A remote that fails, or answers `409`
while a forge run holds the write lock, shows why in the status line
("a forge sync is running: Pull when it ends"), and the page carries on.

## 4. The page

Mockup: the link at the top.

- **Header** (`AppHeader.vue`): an **Inbox** link after Conflicts, with its
  count in the same `nav-count`; the tab title becomes "(3) git-board". The
  header reads `GET /api/inbox` on load and on SSE `issues` or `inbox`
  events, and never pulls.
- **`/inbox`** (`InboxPage.vue`):
  - A status line: "Checked origin 4 s ago · 3 issues new · **Pull**",
    or "upstream: couldn't check: …"; **Refresh** (checks within the 5
    minutes too, and drops read rows) and **Mark all read**.
  - A settings menu: "Also field changes on issues I'm in" (`fields`); the
    config's views as checkboxes; a view gone shows "(no longer exists)"
    with a remove ×.
  - The rows (§2): an unread dot, status icon, `#number` and title, what's
    new, reason chips, avatars, how long ago.
  - Opening a row opens `IssuePanel` in a dialog at `?issue=<id>`, as
    `IndexPage` does (links, j/k), scrolled to the oldest unseen event (an
    optional `at` on `IssueTimeline`), and posts `seen`. The row stays,
    dimmed, until the next refresh: the list doesn't jump while you read.
  - Empty: "Nothing new for you", and what you follow ("Following 14
    issues · view backend"). No identity: "Choose who you write as". No
    login: "Tell git-board your forge login", pointing at `git bug bridge
    new` or `--me`.
- **`IssueSide.vue`**: an Inbox block: "Following · because you're
  assignee · commented" with **Unfollow**, or "Not following" with
  **Follow**; once clicked, **Back to automatic**.
- **`commands.ts`**: "Go to inbox".

## 5. Tests

- **`internal/inbox`, one table test of `Rows`**: someone's comment on your
  assigned issue shows, yours doesn't; a late arrival (old time, unseen ID)
  shows after you opened the issue; added to or removed from a person field
  and a mention show unfollowed, and through an Unfollow; `fields` off
  hides a sprint change, on shows it; title and description edits never
  show; Unfollow hides activity, Follow brings an issue in; a closed issue
  still shows for an `is:open` view; a missing view lands in
  `views_missing`; outside the window is ignored and pruned; a comment by
  a second identity carrying your login is yours (hidden, and counts as
  `commented`).
- **The file**: a round trip; an unreadable file is an error.
- **The server, one `httptest` flow on a `gitbugiotest` repo**: another
  identity comments, `GET /api/inbox` has the row; `seen` clears it;
  `follow: false` keeps a later comment out, `null` brings it back; no
  identity is `409 no-identity`, an identity without a login `409
  no-login`.
- **`internal/filter`**: `involves:` cases (author, a person field, a
  comment, a mention, someone else's login) and `follows:@me` (Follow adds,
  Unfollow removes, a followed view adds, a view's own `follows:` term is
  ignored).
- **Web, one vitest**: which remotes to check (checked within 5 minutes:
  skip, unless Refresh), and which offer Pull (`issues.receive` > 0).
- Not tested: the header count, the tab title (display).

## For the plan

- `IssueSide.vue` and `issuesync.go` have work in flight on main (branch
  `issuelink`: Link / Unlink / Take the forge's version): merge main into
  this branch after it lands, before touching `IssueSide.vue`.
- First task: the full-text index spike for mentions (§2).

## Not now

- Email, Teams or Slack, a digest: deliveries of the same rows, when
  someone asks.
- The CLI and TUI: `git-board inbox` on the same package.
- Browser notifications; automatic pulls, or a timer in the background
  (hosted, where a machine is always on); forge Get on open (rate limits).
- An inbox card on Settings' Overview tab.
- Outsiders writing in (a helpdesk); replying by email.
