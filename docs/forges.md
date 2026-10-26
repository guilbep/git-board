# Forges

## Where your issues go

Your issues travel with git, like your code. Forges and the GitHub Project are mirrors, for the people who work there. One sync moves them all, from Settings › Sync or with `git board sync`, in this order:

1. **git pull** from `origin`: the issues and people, as code goes, and the numbers origin hands out.
2. **The forges** whose sync is on, over their API: a mirror of the issues, comments, labels and assignees on GitHub or GitLab, with the forges of `.git-board.toml` (`[forges]`), or with none there, those its git remotes are on.
3. **The GitHub Project**, with `[github-project]`: its fields (status, priority, sprint) as `ns:value` labels here, and back when `write = true`. Its conflicts are on the Conflicts page.
4. **git push** to `origin`, which numbers the issues origin now has.

A step that fails stops the steps after it, and the sync says which. A forge that can't be read is told, and the others go on. Without an `origin`, the git steps are skipped.

**Check** runs the same steps and writes nothing: a fetch from origin and a compare, the forges' dry run, the Project's check. **Sync** needs a clean check of the last 15 minutes, and asks first, with the check's counts. On one issue, in its panel: Send now and Refresh.

A forge doesn't replace a remote: the people who use git-board pull and push the issues with git; a forge is for the people who stay on GitHub or GitLab.

## Tokens

A forge is read and written with a token, never kept in git-board's files: `.git-board.toml` is committed, and names only the forge's posting account, `login`. The token comes from the environment when it is set for that forge's host (`GITHUB_TOKEN` for github.com, `GITHUB_ENTERPRISE_TOKEN` for the host `GITHUB_ENTERPRISE_HOST` names, `GITLAB_TOKEN` for the host `GITLAB_HOST` names, gitlab.com by default), as in CI; else from git's credential helper for the host, with the login as user name (`git credential fill`). A token pasted in Settings goes to that helper (`git credential approve`), and is never shown again.

- **GitHub**: a classic personal access token (github.com/settings/tokens) with the `repo` scope, or `public_repo` for a public repository.
- **GitLab**: a personal access token of the instance (`/-/user_settings/personal_access_tokens`) with the `api` scope; `read_api` is enough to only get. Reporter on the project to read, Developer to send.

Someone else with a token of their own on the forge, in their own credential helper, has their comments posted as themselves when they sync; the others' are posted by the posting account, under a line naming them.

The GitHub Project isn't read with a forge's token but with the `gh` command, logged in on its own (`gh auth login`): the `read:project` scope to read it, `project` to write it.

## Adding a forge

With no `[forges.<name>]` table, git-board needs none: it finds the forges in the repository's git remotes, every fetch and push URL of each (`git@host:path.git`, `ssh://[user@]host[:port]/path`, `https://host/path`, with or without `.git`). Each project counts once, whichever remote names it. Their sync is on, with no login: git's credential helper picks the account.

- `github.com` is GitHub, its project `owner/repo` as GitHub spells it: each run reads the repository's `full_name` once, as a URL's case is only your typing and the sync keys its links on the name (`git@github.com:epfl-enac/wiki.git` is `EPFL-ENAC/wiki`, as a table naming it). One GitHub doesn't answer for is left out of that run, with why.
- `gitlab.com`, or a host whose first label holds `gitlab`, is GitLab. The web host drops a first label `ssh`, or a first label's `-ssh`: `git@gitlab-ssh.epfl.ch:EPFL-ENAC/wiki.git` is `gitlab.epfl.ch`, project `EPFL-ENAC/wiki`. Each run reads the project's numeric id once, as its links are keyed on it; a project whose id can't be read is left out of that run, with why, and the others go on.
- Any other host is skipped, and Settings › Sync, `git board switch` and `git board sync` say which URL.

They are named by their kind, `github`, `gitlab`, then `github-2`… when a kind repeats. One table or more turns this off, and so does `discover = false` in `[forges]`.

Settings › Sync › **+ Add a forge**, for a forge no remote names, in four steps:

1. **Forge**: GitHub or GitLab.
2. **Project**: its address, as in your browser (`https://github.com/owner/repo`, `https://gitlab.epfl.ch/group/project`); a short name, `gh` or `gl` when left empty; and whether it syncs.
3. **Token**: one git's credential helper holds for that host already, or a new one pasted.
4. **Check**: git-board asks the forge whose token it is, the posting account, and whether it sees the project. Nothing is kept when it doesn't. A GitLab project is kept by its numeric id, which a renamed group doesn't change.

Adding writes a `[forges.<name>]` table, and syncs nothing: the first sync is yours to start, from Settings › Sync, after its check. While the forges come from the remotes, adding one writes their tables too, as a table turns the finding off; one the remotes name already is refused. The address `#/settings/sync?add=1` opens the steps.

## Sync on or off

With sync on, the forge takes part in the sync's forge step: each forge is read first, then every change is sent to each, so an issue, comment or change made on one forge reaches the others in one run. Line comments, replies and reviews (Changes tab) stay in git-board: they never go to a forge.

The check runs the forge sync on a copy of the repository, and answers the forges' writes without making them: what it counts is what the run then does. A real run needs a check of the last 15 minutes. The Work page's Send all sends alone, after a check.

Each run, the check too, also asks each forge who can be assigned on its project, and says how many it found. The assignee boxes (every person field) then offer them, with the people git-board knows. A forge that doesn't answer is a warning: the boxes keep its last list.

Off (`sync = false`), the forge is kept but stays out of it, until you switch it on again in Settings › Sync. A forge found in the remotes always syncs: to leave one out, give the forges tables.

## Removing a forge

**Remove**, in Settings › Sync, drops the forge's table from `.git-board.toml`. The issues it brought stay, still linked to it, and so does the token in git's credential helper (`git credential reject` removes it).

## One issue

In an issue's panel, On the forges lists each forge it is on, and its timeline marks what hasn't reached each forge yet:

- **Send now** sends its waiting changes, **Refresh from forges** gets the forges' own. Both check first, and ask before writing to a forge.

An issue is linked to a forge issue once, by the sync, and stays so: linking by hand, unlinking and taking a forge's version are out of git-board's first version.

## On the command line

`git board sync` runs the whole sync, as Settings › Sync does, each step in a line; `--check` is its dry run, and `--git` syncs with git alone: a pull, then a push. It has no check to wait for: `--check` first is yours to run. It and `git board switch` print the forges found in the remotes, one line each: name, host and project, a GitLab one's with its id, and each URL they skip.

Otherwise the forges are `.git-board.toml`'s, edited in Settings or by hand:

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

