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

- **A token** comes from an environment variable, when it is set and the
  forge's host is the one the variable is for, as in CI. Otherwise it comes
  from git's credential helper for the forge's host, with the account as
  `username`:

    ```sh
    printf 'protocol=https\nhost=gitlab.epfl.ch\nusername=guilbep\n\n' | git credential fill
    ```

- **An environment token goes to its own host only,** because the
  settings live in the committed `.git-board.toml`, and a pull request
  that edits them must not send a token elsewhere:
    - `GITHUB_TOKEN` is for `github.com` and its API, `api.github.com`.
    - `GITHUB_ENTERPRISE_TOKEN` is for the host named by
      `GITHUB_ENTERPRISE_HOST`. If that is unset, it is never used.
    - `GITLAB_TOKEN` is for the host named by `GITLAB_HOST`, which is
      `gitlab.com` when unset.
    - Any other host gets its token from git's credential helper only.
    - `GH_TOKEN` is never read.
    - A base URL must be `https://`. `http://` is accepted only for
      `localhost`, `127.0.0.1` and `::1`; any other makes the adapter
      refuse to start.

- **The core keeps no token,** in git or anywhere else. git-board's
  Settings stores a new one with `git credential approve`.
- **git-board's own calls to a forge** (its images, who can be assigned,
  pull requests) take their token by the same rule, through each
  adapter's `Token`.

## GitHub

The adapter is `core/forge/github`: a project `owner/repo` on github.com
or a GitHub Enterprise host. It speaks GitHub's REST API,
`https://api.github.com` or `https://<host>/api/v3`, plus one GraphQL
query for the times REST doesn't give.

- **The token** follows [Accounts and tokens](#accounts-and-tokens):
  `GITHUB_TOKEN` or `GITHUB_ENTERPRISE_TOKEN` for their own host, else
  git's credential helper for the host, with the account's login as
  `username`, and never from a prompt. A person's token counts only when GitHub says it is theirs:
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
- **80 writes per minute and 500 per hour.** The adapter leaves at least a
  second between two writes, as GitHub asks, which keeps under 80 a minute,
  and makes at most 500 in any hour. When GitHub still turns a request away for a
  rate limit, a 403 or a 429, the adapter waits and tries again, three
  times at most. It waits for `Retry-After` when given, else until
  `x-ratelimit-reset` when none is left, else a minute: the secondary rate
  limit gives no reset time. When a limit stops a run, the next one resumes
  from there, as sync records every write as it makes it.
- **A read is tried again after a 5xx or a broken connection, a write never
  is.** GitHub may have made it (see [Echoes](sync.md#echoes)).
- **Errors.** A 404, a 410 and a redirect, as for an issue moved to
  another repository, are not found. A 403 and a 422 are refused, rate
  limits aside. A token GitHub rejects, a 401, stops the run when it is
  the posting account's; a person's is no token.
- **Labels are made as needed,** in GitHub's grey, before they are added.
  One write adds the labels; each removal is a write of its own. An added
  label GitHub leaves out, as it does without push access, is refused.
  Removing a label the issue no longer has is no error.
- **Assignees go as logins,** the whole set in one write. The adapter
  reads back what GitHub kept: GitHub leaves out a user without access to
  the repository.
- **Visibility** is the repository's: `internal`, on an Enterprise, when
  its `visibility` says so, else private by its `private` flag, else
  public.
- **A login can be renamed,** so people are matched by the forge's numeric
  user id, never by login.

## GitLab

`core/forge/gitlab` is the adapter, over GitLab's REST API v4.

- **A project** is named by its numeric id or its path, on a base URL like
  `https://gitlab.epfl.ch`. Its key is the host and the project as named:
  `gitlab.epfl.ch/42400`. A numeric id outlives a rename or a move. A path
  doesn't, and a new key is a new sync entity.
- **Tokens** follow [Accounts and tokens](#accounts-and-tokens). The
  posting account's is `GITLAB_TOKEN` for its own host, else
  the credential helper's for the host, with the posting account's login
  as `username`. Another account's comes from the credential helper only,
  and counts once GitLab's `/user` says it is theirs. A helper's token for
  someone else, or one GitLab turns away, is no token. Asking the helper
  never prompts.
- **An issue's id is GitLab's `id`, its number the `iid`.** The API reads
  an issue by its iid only. So the adapter keeps the iid of every issue it
  lists, reads or creates, and looks for an id it hasn't seen in the whole
  listing, once.
- **Text edits leave no event.** A title or description edited on GitLab
  shows in the issue itself, and in a system note: `changed title from …`,
  `changed the description`. Sync finds the edit by comparing it with last
  seen. For [the later change](sync.md#the-four-cases), a title or a
  description takes its time from the latest such note, and the status from
  the latest state event. A field without one takes the issue's last
  update.
- **`updated_at` moves late,** so sync lists the issues updated from 2
  minutes before the last sync. Measured on gitlab.epfl.ch: after a label
  or assignee change, `updated_at` updates about once a minute at most. It
  hadn't moved 26 s after a change, and had 71 s after. Closing an issue
  always moves it. Without the margin, an issue edited right after a sync
  could carry a time earlier than that sync, and no later sync would pick
  it up. The margin costs a handful of issues re-read per sync.
- **Listings are read page by page,** 100 at a time, oldest first, up to
  the last `X-Next-Page`. An issue or a note listed twice, as a page read
  after a change can repeat one, counts once.
- **Comments are the notes people wrote.** System notes are GitLab's own,
  and give only the change times above. Internal notes are for the
  project's members, so sync never reads them.
- **Line endings.** A web edit can send `\r\n`. Texts come back with `\n`,
  so that the blank lines around a header and a marker stay what sync
  wrote.
- **The free edition has one assignee per issue.** The others stay in
  git-board, with one warning. GitLab takes assignees only as the whole set,
  `assignee_ids`. So sync reads them, writes the set only when it changes,
  and reads back what GitLab kept. GitLab records an assignment only as a
  system note, so sync reads the assignees off the issue.
- **Labels go by name,** in `add_labels` and `remove_labels`. GitLab
  splits them at commas, and its labels hold none, so a label with a comma
  is refused.
- **Base URLs compare without their trailing slash.** `https://gitlab.epfl.ch`
  and `https://gitlab.epfl.ch/` are the same server, for projects and for
  tokens.
- **Visibility** is the project's `public`, `internal` or `private`;
  another value is an error. An internal project is seen by every user of
  the instance, not by anyone (see
  [Nothing private published](sync.md#nothing-private-published)).
- **Confidential issues are skipped.** GitLab lists them to the project's
  members, flagged `confidential`, and the adapter leaves them out. Reading
  one on its own is an error (see
  [Nothing private published](sync.md#nothing-private-published)). GitLab
  takes writes to one, so each write reads its issue first: an issue that
  turned confidential gets no more.
- **Writes go 250 ms apart,** under GitLab's default limit of 300 issues
  or notes created a minute. A 429 is tried again after its `Retry-After`,
  else at the limit's `RateLimit-Reset`, three times at most. A read that
  gets a 5xx is tried again after 1, 2, then 4 seconds. A write never is,
  as GitLab may have made it (see [Echoes](sync.md#echoes)). Once
  `RateLimit-Remaining` is 0, the next request waits for `RateLimit-Reset`.
- **Errors.** A 404, a 410 and a redirect are not found: a redirect is
  never followed, as Go would send `PRIVATE-TOKEN` along to any host. A
  400, 403, 409 or 422 is refused for good.

[609]: https://github.com/git-bug/git-bug/issues/609
[1638]: https://github.com/git-bug/git-bug/issues/1638
