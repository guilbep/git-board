# User manual

git-board keeps your project's issues in your repository, as your own
data. GitHub, GitLab and GitHub Projects become mirrors that a sync keeps
in step: you can leave any of them and lose nothing, work offline, and see
every change ever made to an issue.

This manual is for the people who use a board day to day. The
[guide](../guide/configuration.md) is for who configures one, and the
[spec](../spec/data-model.md) for who builds on the core.

## In five minutes

```sh
cd your-repository
git board init                      # a .git-board.toml with sensible fields
git board me --new                  # you, from git's user.name and user.email
git board new "Login fails on Safari" prio=p1 -b "Steps: ..."
git board ls                        # the open issues
git board                           # the same board in your browser
```

The issues live under `refs/board/` in the repository: `git board push`
shares them with your team through `origin`, and `git board sync` mirrors
them to GitHub or GitLab when you want that too.

## The manual

1. [Install](install.md): the binary, and what else a feature needs.
2. [First steps](first-steps.md): a board, who you are, your first issue.
3. [Everyday work](everyday.md): finding, triaging, moving and closing
   issues, from the command line and the web UI.
4. [Working as a team](together.md): sharing the board, forges, numbers.
5. [Owning your issues](ownership.md): where the board is kept, leaving a
   forge, exporting, history.
6. [Questions](faq.md): what goes wrong, and what to do.

Every command's flags, its JSON output and its exit codes are in the
[CLI reference](../guide/cli.md).

## Why git-board

- **Your issues are yours.** They are operations in your repository (or
  your PostgreSQL, or Markdown files in your checkout): not rows in a
  forge's database you rent access to.
- **Nothing is lost.** Every change is kept, by whom and when (`git board
  log`). A forge that deletes, rewrites or locks you out takes nothing with
  it.
- **Any forge, or none.** One issue can live on GitHub and GitLab at once;
  a board can live on neither.
- **Offline.** Everything but the forge sync works without a network.
- **Config as code.** Fields, views and sprints are `.git-board.toml`,
  reviewed in pull requests like the rest of the repository.
