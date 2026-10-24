# Forges

An adapter speaks to one forge's API for sync (M4). This page says what
GitHub and GitLab need, as checked live for the git-bug fork that came
before the core. [Sync](sync.md) gives the rules they serve.

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

- **Status and labels come from the issue,** not only from its timeline.
  The pull reads the issue's `state` and `labels` with it, its labels paged
  past 100. Some issues GitHub closes leave no closed event in the
  timeline ([git-bug#609][609]). And a timeline read page by page can miss
  items ([git-bug#1638][1638]). The timeline gives the history and its
  times; the issue gives the values. An issue whose timeline fails to load is an
  error, never half an issue.
- **80 writes per minute and 500 per hour.** Sync paces its writes under
  both. When GitHub still answers with its secondary rate limit, which gives
  no reset time, sync waits a minute and tries again, three times at most.
  When a limit stops a run, the next one resumes from there, as sync
  records every write as it makes it.
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
