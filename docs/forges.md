# Forges

## Three ways your issues go

Your issues travel with git, like your code. Forges and the GitHub Project are mirrors, for the people who work there.

- **1 · Remotes** · git. Everything: code, branches, issues and people, to and from any git remote (GitHub or GitLab as git servers, a shared server, a colleague). Push and Pull: `git board sync`.
- **2 · Forges** · their API. A mirror of the issues, comments, labels and assignees on GitHub or GitLab, with the forges of `.git-board.toml` (`[forges]`). Check (what would change, nothing written), Get, Send, Sync; and on one issue, in its panel, Send now and Refresh.
- **3 · GitHub Project** · fields. The Project's fields (status, priority, sprint) as `ns:value` labels here, and back when writing is on. Its conflicts are on the Conflicts page.

A forge doesn't replace a remote: the people who use git-board pull and push the issues with git; a forge is for the people who stay on GitHub or GitLab.

## Tokens

A forge is read and written with a token, never kept in git-board's files: `.git-board.toml` is committed, and names only the forge's posting account, `login`. The token comes from the environment when it is set for that forge's host (`GITHUB_TOKEN` for github.com, `GITHUB_ENTERPRISE_TOKEN` for the host `GITHUB_ENTERPRISE_HOST` names, `GITLAB_TOKEN` for the host `GITLAB_HOST` names, gitlab.com by default), as in CI; else from git's credential helper for the host, with the login as user name (`git credential fill`). A token pasted in Settings goes to that helper (`git credential approve`), and is never shown again.

- **GitHub**: a classic personal access token (github.com/settings/tokens) with the `repo` scope, or `public_repo` for a public repository.
- **GitLab**: a personal access token of the instance (`/-/user_settings/personal_access_tokens`) with the `api` scope; `read_api` is enough to only get. Reporter on the project to read, Developer to send.

Someone else with a token of their own on the forge, in their own credential helper, has their comments posted as themselves when they sync; the others' are posted by the posting account, under a line naming them.

The GitHub Project isn't read with a forge's token but with the `gh` command, logged in on its own (`gh auth login`): the `read:project` scope to read it, `project` to write it.

## Adding a forge

Settings › Forges › **+ Add a forge**, in four steps:

1. **Forge**: GitHub or GitLab.
2. **Project**: its address, as in your browser (`https://github.com/owner/repo`, `https://gitlab.epfl.ch/group/project`); a short name, `gh` or `gl` when left empty; and whether it syncs.
3. **Token**: one git's credential helper holds for that host already, or a new one pasted.
4. **Check**: git-board asks the forge whose token it is, the posting account, and whether it sees the project. Nothing is kept when it doesn't. A GitLab project is kept by its numeric id, which a renamed group doesn't change.

Adding writes a `[forges.<name>]` table, and syncs nothing: the first sync is yours to start, from the Sync page, after its check. The address `#/settings/forges?add=1` opens the steps.

## Sync on or off

With sync on, the forge takes part in the Sync page's sync: each forge is read first, then every change is sent to each, so an issue, comment or change made on one forge reaches the others in one run. Line comments, replies and reviews (Changes tab) stay in git-board: they never go to a forge.

The check runs the whole sync on a copy of the repository, and answers the forges' writes without making them: what it counts is what the run then does. A real run needs a check of the last 15 minutes. Send alone is refused where the check found issues on the forge by their marker, which a run stopped before linking: only Get or Sync links them.

Each run, the check too, also asks each forge who can be assigned on its project, and says how many it found. The assignee boxes (every person field) then offer them, with the people git-board knows. A forge that doesn't answer is a warning: the boxes keep its last list.

Off (`sync = false`), the forge is kept but stays out of it, until you switch it on again in the Forges table.

## Removing a forge

**Remove**, in the Forges table, drops the forge's table from `.git-board.toml`. The issues it brought stay, still linked to it, and so does the token in git's credential helper (`git credential reject` removes it).

## One issue

In an issue's panel, On the forges lists each forge it is on, and its timeline marks what hasn't reached each forge yet:

- **Send now** sends its waiting changes, **Refresh from forges** gets the forges' own. Both check first, and ask before writing to a forge.

An issue is linked to a forge issue once, by the sync, and stays so: linking by hand, unlinking and taking a forge's version are out of git-board's first version.

## On the command line

The forges are `.git-board.toml`'s, edited in Settings or by hand:

```toml
[forges.gh]
kind = "github"
project = "owner/repo"
login = "you"
sync = true

[forges.gl]
kind = "gitlab"
host = "gitlab.epfl.ch"
project = "42400"
login = "you"
sync = true
```

The sync itself runs from the Sync page: there is no forge sync command yet.
