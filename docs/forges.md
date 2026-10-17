# Forges

## Three ways your issues go

Your issues travel with git, like your code. Forges and the GitHub Project are mirrors, for the people who work there.

- **1 · Remotes** · git. Everything: code, branches, issues and people, to and from any git remote (GitHub, GitLab or Gitea as git servers, a shared server, a colleague). Push and Pull: `git bug push`, `git bug pull`.
- **2 · Forges** · their API. A mirror of the issues, comments, labels and assignees on GitHub, GitLab or Gitea, through a git-bug bridge. Check (what would change, nothing written), Get, Send, Sync; and on one issue, in its panel, Send now and Refresh.
- **3 · GitHub Project** · fields. The Project's fields (status, priority, sprint) as `ns:value` labels here, and back when writing is on. Its conflicts are on the Conflicts page.

A forge doesn't replace a remote: the people who use git-board pull and push the issues with git; a forge is for the people who stay on GitHub, GitLab or Gitea.

## Tokens

A forge is read and written with a token of yours, kept in git-bug's credential store in this repository and never shown again. Adding a forge checks the token against the project before anything is kept.

- **GitHub**: a classic personal access token (github.com/settings/tokens) with the `repo` scope, or `public_repo` for a public repository. Fine-grained tokens (`github_pat_…`) don't work.
- **GitLab**: a personal access token of the instance (`/-/user_settings/personal_access_tokens`) with the `api` scope; `read_api` is enough to only get. Reporter on the project to read, Developer to send.
- **Gitea**: an access token of the instance (`/user/settings/applications`) with read and write on issues and repositories, and read on users.

The GitHub Project isn't read with a forge's token but with the `gh` command, logged in on its own (`gh auth login`): the `read:project` scope to read it, `project` to write it.

## Adding a forge

Settings › Forges › **+ Add a forge**, in four steps:

1. **Forge**: GitHub, GitLab or Gitea.
2. **Project**: its address, as in your browser (`https://github.com/owner/repo`, `https://gitlab.epfl.ch/group/project`); a short name, `gh`, `gl` or `gitea` when left empty; and whether it syncs both ways.
3. **Token**: one already stored for this forge, or a new one pasted.
4. **Check**: git-board asks the forge whether the token sees the project. Nothing is kept when it doesn't.

Adding syncs nothing: the first sync is yours to start, from the Sync page, after its check. The address `#/settings/forges?add=1` opens the steps.

## Sync on or off

With sync on, the forge takes part in the Sync page's sync: each forge is read first, then every change is sent to each, so an issue, comment or change made on one forge reaches the others in one run.

Off, the forge is kept but stays out of it, until you switch it on again in the Forges table. `git bug bridge pull <name>` still gets its issues, on the command line.

## Removing a forge

**Remove**, in the Forges table, drops the forge's settings. The issues it brought stay, still linked to it, and so does the stored token, for the next forge (`git bug bridge auth rm <id>` deletes it).

## On the command line

The same, while git-board-server isn't running (it holds the repository):

```sh
git bug bridge new --name gh --url https://github.com/owner/repo --sync
git bug bridge new --name gl --target gitlab --url https://gitlab.epfl.ch/group/project --sync
git config git-bug.bridge.gh.sync false   # sync off
git bug bridge rm gh
```

`git bug bridge new --sync` adds a forge as Settings does: it checks the token and the project, takes the stored token of that forge when only one fits (`--credential <id>` picks one, `--token` gives a new one), and turns sync on. Without `--sync`, the forge stays out of the sync.

`git bug bridge sync` then runs the sync of every forge with sync on; `--dry-run` says what it would do and changes nothing, locally or on the forges.
