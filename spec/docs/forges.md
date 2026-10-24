# Forges

An adapter speaks to one forge's API for sync (M4). This page says what
GitHub and GitLab need, as checked live for the git-bug fork that came
before the core. [Sync](sync.md) gives the rules they serve.

An adapter implements the core's `forge.Forge` and passes its conformance
suite, `core/forge/conformance`, over a fake forge served as that forge's
API. The adapter, not the engine, applies its listing margin, paces its
writes and retries a rate limit. It maps the forge's answers to the core's
errors: refused for good, not found, confidential, and no token.

## Accounts and tokens

Sync acts on a forge through accounts: the posting account, from
git-board's settings, and the people who have a token of their own there
(see [Posting for people without a token](sync.md#posting-for-people-without-a-token)).

- **A token** comes from an environment variable, when it is set, as in
  CI. Otherwise it comes from git's credential helper for the forge's host,
  with the account as `username`:

    ```sh
    printf 'protocol=https\nhost=gitlab.epfl.ch\nusername=guilbep\n\n' | git credential fill
    ```

- **The core keeps no token,** in git or anywhere else. git-board's
  Settings stores a new one with `git credential approve`.

## GitHub

The adapter is `core/forge/github`: a project `owner/repo` on github.com
or a GitHub Enterprise host. It speaks GitHub's REST API,
`https://api.github.com` or `https://<host>/api/v3`, plus one GraphQL
query for the times REST doesn't give.

- **The token** is `GITHUB_TOKEN` when it is set, or
  `GITHUB_ENTERPRISE_TOKEN` for an Enterprise host. Otherwise it comes from
  git's credential helper for the host, with the account's login as
  `username`, and never from a prompt. A github.com token never goes to
  another host. A person's token counts only when GitHub says it is theirs:
  `/user` must give their account's numeric id.
- **A forge issue's id is GitHub's numeric `id`.** REST names an issue by
  its number, so the adapter remembers each id's number from its listings.
  It looks for an id it hasn't seen, one recorded by an earlier run, once,
  in the listing of every issue.
- **The listing** is every issue updated since the last run, oldest first,
  a hundred a page. It starts a minute early: the last run's start is the
  local clock's, and `updated_at` is GitHub's. An issue or a comment that
  two pages both list counts once.
- **Pull requests are never synced.** GitHub's listing of issues holds them;
  the adapter leaves them out. GitHub has no confidential issues, so reading
  a pull request alone is the core's confidential error: sync never reads
  it.
- **Status and labels come from the issue,** not from its timeline. Some
  issues GitHub closes leave no closed event in the timeline
  ([git-bug#609][609]). And a timeline read page by page can miss items
  ([git-bug#1638][1638]). The status is `state`, whatever its
  `state_reason`: an issue closed as not planned is closed.
- **When a field last changed,** for [the later change](sync.md#the-four-cases),
  comes from one GraphQL query per page of issues: the body's
  `lastEditedAt`, the last rename and the last reopening. A closed issue's
  status changed at its `closed_at`. A field GitHub shows no change of
  changed at the issue's creation. A comment's edit is its `updated_at`,
  when that is after its creation. An issue whose comments or times fail to
  load is an error, never half an issue.
- **`\r\n` is read as `\n`.** A web edit leaves `\r\n`, which would break
  the blank lines around sync's header and marker.
- **80 writes per minute and 500 per hour.** The adapter writes a second
  apart at most, as GitHub asks, which keeps under 80 a minute, and makes
  at most 500 in any hour. When GitHub still turns a request away for a
  rate limit, a 403 or a 429, the adapter waits and tries again, three
  times at most. It waits for `Retry-After` when given, else until
  `x-ratelimit-reset` when none is left, else a minute: the secondary rate
  limit gives no reset time. When a limit stops a run, the next one resumes
  from there, as sync records every write as it makes it.
- **A read is tried again after a 5xx or a broken connection, a write never
  is.** GitHub may have made it (see [Echoes](sync.md#echoes)).
- **Errors.** A 404, a 410 and a redirect, as for an issue moved to
  another repository, are not found. A 403 and a 422 are refused, rate
  limits aside. A token GitHub rejects, a 401, stops the run.
- **Labels are made as needed,** in GitHub's grey, before they are added.
  One write adds the labels; each removal is a write of its own. An added
  label GitHub leaves out, as it does without push access, is refused.
  Removing a label the issue no longer has is no error.
- **Assignees go as logins,** the whole set in one write. The adapter
  reads back what GitHub kept: GitHub leaves out a user without access to
  the repository.
- **Public** is the repository's `private` flag, negated.
- **A login can be renamed,** so people are matched by the forge's numeric
  user id, never by login.

## GitLab

- **Text edits leave no event.** A title or description edited on GitLab
  shows only in the issue itself. Sync finds the edit by comparing it with
  last seen. For [the later change](sync.md#the-four-cases), a description
  edit takes its time from GitLab's system note "changed the description",
  else from the issue's last update.
- **`updated_at` moves late,** so sync lists the issues updated from 2
  minutes before the last sync. Measured on gitlab.epfl.ch: after a label
  or assignee change, `updated_at` updates about once a minute at most. It
  hadn't moved 26 s after a change, and had 71 s after. Closing an issue
  always moves it. Without the margin, an issue edited right after a sync
  could carry a time earlier than that sync, and no later sync would pick
  it up. The margin costs a handful of issues re-read per sync.
- **The free edition has one assignee per issue.** The others stay in
  git-board, with one warning. GitLab takes assignees only as the whole set,
  `assignee_ids`. So sync reads them, writes the set only when it changes,
  and reads back what GitLab kept. GitLab records an assignment only as a
  system note, so sync reads the assignees off the issue.
- **Base URLs compare without their trailing slash.** `https://gitlab.epfl.ch`
  and `https://gitlab.epfl.ch/` are the same server, for projects and for
  tokens.
- **Confidential issues are skipped.** GitLab's listing omits them, and
  reading one on its own is an error (see
  [Nothing private published](sync.md#nothing-private-published)).

[609]: https://github.com/git-bug/git-bug/issues/609
[1638]: https://github.com/git-bug/git-bug/issues/1638
