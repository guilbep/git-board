# git-board

Issues, fields, sprints and views kept in git, next to your code: a board
for a repository's issues, as a GitHub Project is, synced with GitHub and
GitLab. Your issues are yours: independent of any forge, every change kept.

**Documentation: <https://guilbep.github.io/git-board/>**: the
[user manual](https://guilbep.github.io/git-board/manual/) (install, first
steps, everyday work, teams, owning your issues), the
[CLI reference](https://guilbep.github.io/git-board/guide/cli/), the
[configuration guide](https://guilbep.github.io/git-board/guide/configuration/),
the core's spec and the architecture decisions. The same pages are in
[docs/](docs/index.md).

## What it is

- **Issues in the repository.** Each issue is a log of operations under
  `refs/board/`: it travels with `git push` and `git pull`, works offline,
  and needs no server. A board can also be kept in PostgreSQL or in a
  folder of Markdown files ([Where the board is kept](docs/guide/datastores.md)).
- **Fields are labels.** A field's value is a namespaced label
  (`status:ready`, `prio:p1`, `sprint:delivery`), so it merges like any
  other operation, and every `ns:value` label is a field without
  configuring it.
- **Configuration as code.** Fields, sprints, saved views, roles, forges
  and releases are a committed `.git-board.toml`, reviewed and diffed like
  the rest of the repository ([Configuration](docs/guide/configuration.md)).
- **The forges are mirrors.** `git board sync` pulls from GitHub and
  GitLab and sends to them, found from `origin` with no configuration
  ([Forges](docs/guide/forges.md)). A GitHub Project's fields sync with the
  labels too.
- **One binary.** The command line, a terminal UI, and a local web server
  with the web app inside: tables, boards, a roadmap, the inbox, insights,
  releases and Settings.

git-board v0.12 is a rewrite of v0.11 that keeps its data byte for byte
([ADR 0021](docs/adr/0021-git-board-v2-same-data-new-code.md)): a board
kept by v0.11 opens unchanged. A repository whose issues are still
git-bug's migrates with git-board v0.11 first ([Leaving
git-bug](docs/guide/forges.md#leaving-git-bug)).

## Install

git-board embeds its web app, so it is installed from a clone:

```bash
git clone https://github.com/guilbep/git-board.git
cd git-board
make install-latest   # the latest release's binary, its checksum checked (needs gh)
# or
make install          # build it here: the web app, then the binary (Go 1.26+, Node 22.12+)
git-board --version   # git-board version <version>
```

Installed as `git-board` on your `PATH`, git also runs it as `git board`.
It needs `git` on the `PATH`, and a token for each forge it syncs with;
what other features need is in [Install](docs/manual/install.md).

## Quick start

```bash
cd your-repository
git board init                       # a .git-board.toml: fields, views, comments on each key
git board me --new                   # who you write as, from git's user.name and user.email
git board new "Login fails on Safari" prio=p1 -b "Steps: ..."
git board ls                         # the open issues
git board ls "assignee:@me is:open" --sort "prio asc"
git board show 624a                  # one issue: its fields, description and comments
git board set 624a status=ready assignee=+@me
git board comment 624a -b "Fixed in 4e5f6a7"
git board close 624a
git board                            # the same board in your browser
git board sync --check               # what a sync with origin and the forges would do
git board sync
```

Any unique start of an id names an issue, and so do a forge number
(`#12`) and a project number (`CO2-3075`). Every command, its flags, its
`--json` output and its exit codes, with a table for those coming from
`gh` and `glab`: [Issues from the command line](docs/guide/cli.md).

## Contributing

Setup, the layout of the module, the make targets and the conventions are
in [CONTRIBUTING.md](CONTRIBUTING.md). The rewrite's design, and what each
milestone built, is [docs/rewrite/design.md](docs/rewrite/design.md).

## Related work and inspiration

git-board is one of several tools that keep issues beside the code. The
comparison, and what git-board takes from each, is in [Related
work](docs/related-work.md).

- **[git-bug](https://github.com/git-bug/git-bug)**: issues as operation
  logs in git refs, with bridges to GitHub, GitLab, Jira and Launchpad.
  git-board started on it, then on a fork, then on a core of its own
  ([ADR 0003](docs/adr/0003-own-core-instead-of-git-bug.md)); that core
  keeps git-bug's model of packs ordered by a Lamport clock.
- **[git-issue](https://github.com/dspinellis/git-issue)**, by Diomidis
  Spinellis: one shell script that keeps each issue as plain-text files in
  a git repository of its own (`.issues`), with import from and export to
  GitHub and GitLab.
- **GitHub** (and its [`gh`](https://cli.github.com/) CLI) and **GitLab**
  (and [`glab`](https://gitlab.com/gitlab-org/cli)): the forges git-board
  syncs with. Its command line follows `gh`'s and `glab`'s where it can,
  and its boards follow GitHub Projects'.

Others, most no longer maintained:

- [bug](https://github.com/driusan/bug): an issue tracker in Go that keeps issues as plain-text files in an `issues/` directory, versioned with git or Mercurial.
- [Bugs Everywhere](https://github.com/kalkin/be): a distributed bug tracker in Python that keeps bugs in the repository, for several version control systems. Its repository is read-only since 2020.
- [deft](https://github.com/npryce/deft): a lightweight feature and issue tracker that keeps its database as plain-text files in the project's repository.
- [Fossil](https://fossil-scm.org/): a distributed version control system with tickets, a wiki and a web interface built in, in one executable.
- [git-appraise](https://github.com/google/git-appraise): distributed code review for git, its reviews and comments kept in git notes.
- [git-dit](https://github.com/git-dit/git-dit): a distributed issue tracker for git that records each issue and comment as a commit.
- [Perceval](https://github.com/chaoss/grimoirelab-perceval): part of CHAOSS's GrimoireLab, it retrieves data from git, GitHub, GitLab, issue trackers and mailing lists.
- [Radicle](https://lwn.net/Articles/966869/): a peer-to-peer forge on git, its issues CRDTs in git refs, its refs signed.
- [SD, Simple Defects](https://syncwith.us/sd/): a peer-to-peer bug tracker for online and offline use, which syncs with other SD replicas and with trackers such as RT, Trac and GitHub.
- [Distributed bug tracking](https://lwn.net/Articles/281849/), Jonathan Corbet, LWN.net, May 2008: an article on how bug tracking can work alongside distributed version control.

## License

git-board is under the MIT License, the text in [LICENSE](LICENSE).
