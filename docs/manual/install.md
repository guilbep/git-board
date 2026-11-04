# Install

git-board is one binary: the command line, a terminal UI, a web server and
its web app inside. Installed as `git-board` on your `PATH`, git also runs
it as `git board`.

## From a clone

```sh
git clone https://github.com/guilbep/git-board.git
cd git-board
make install-latest          # the latest release's binary, checksum checked (needs gh)
# or
make install                 # build it here: needs Go 1.26+ and Node 22.12+
git-board --version
```

`make install` builds the web app first, as its pages are embedded in the
binary. `make` alone lists every target.

## What it needs

- **git**, on the `PATH`. That's all for issues, the web UI, the views,
  the sync with GitHub and GitLab, and Markdown export and import.
- **A token** for each forge you sync with, kept by git's credential
  helper or in `GITHUB_TOKEN` / `GITLAB_TOKEN`: see
  [Working as a team › Forges](together.md#forges).

Some features need more, and only those features:

| Feature | Needs |
| --- | --- |
| GitHub Projects (`git board project`) | `gh`, logged in, with the `project` scope (`gh auth refresh -s project`) |
| A board kept in PostgreSQL | a PostgreSQL database, tested with 16 ([Owning your issues](ownership.md#where-the-board-is-kept)) |
| Attachments in comments | an S3 bucket, set where the server runs ([Configuration › Attachments](../guide/configuration.md)) |
| The experimental cycle, agents and worktrees | see [Configuration › Experimental features](../guide/configuration.md#experimental-features) |

## Shell completion

```sh
git-board completion zsh > "${fpath[1]}/_git-board"   # zsh
git-board completion bash > /etc/bash_completion.d/git-board
git-board completion fish > ~/.config/fish/completions/git-board.fish
```
