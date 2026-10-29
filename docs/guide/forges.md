# Forges

## Where your issues go

Your issues travel with git, like your code. Forges and the GitHub Project are mirrors, for the people who work there. One sync moves them all, from Settings › Sync or with `git board sync`, in this order:

1. **git pull** from `origin`: the issues and people, as code goes, and the numbers origin hands out.
2. **The forges** whose sync is on, over their API: a mirror of the issues, comments, labels and assignees on GitHub or GitLab, with the forges of `.git-board.toml` (`[forges]`), or with none there, those its git remotes are on.
3. **The GitHub Project**, with `[github-project]`: its fields (status, priority, sprint, est. hours) as `ns:value` labels here, and back when `write = true`. Its conflicts are on the Conflicts page. Those labels stay off GitHub, the Project being their home there; GitLab, which has no Project, gets them as labels. Fields naming issues (`parent:`, `blocked-by:`) and `milestone:`, a forge's own, stay off every forge.
4. **git push** to `origin`, which numbers the issues origin now has.

A failed pull stops the steps after it. A forge or the Project that fails is told, and the push still runs: what the forge step linked must reach origin, or another clone creates it again. A forge that can't be read is left out of the run, told, and the others go on. Without an `origin`, the git steps are skipped.

**Check** runs the same steps and writes nothing: a fetch from origin and a compare, the forges' dry run, the Project's check. **Sync** needs a clean check of the last 15 minutes, and asks first, with the check's counts. On one issue, in its panel: Send now and Refresh.

A forge doesn't replace a remote: the people who use git-board pull and push the issues with git; a forge is for the people who stay on GitHub or GitLab.

## Tokens

A forge is read and written with a token, never kept in git-board's files: `.git-board.toml` is committed, and names only the forge's posting account, `login`. The token comes from the environment when it is set for that forge's host (`GITHUB_TOKEN` for github.com, `GITHUB_ENTERPRISE_TOKEN` for the host `GITHUB_ENTERPRISE_HOST` names, `GITLAB_TOKEN` for the host `GITLAB_HOST` names, gitlab.com by default), as in CI; else from git's credential helper for the host, with the login as user name (`git credential fill`). A token pasted in Settings goes to that helper (`git credential approve`), and is never shown again.

- **GitHub**: a classic personal access token (github.com/settings/tokens) with the `repo` scope, or `public_repo` for a public repository.
- **GitLab**: a personal access token of the instance (`/-/user_settings/personal_access_tokens`) with the `api` scope; `read_api` is enough to only get. Reporter on the project to read, Developer to send.

Someone else with a token of their own on the forge, in their own credential helper, has their comments posted as themselves when they sync; the others' are posted by the posting account, under a line naming them.

The GitHub Project isn't read with a forge's token but with the `gh` command, logged in on its own (`gh auth login`): the `read:project` scope to read it, `project` to write it.

## Adding a forge

With no `[forges.<name>]` table, git-board needs none: it finds the forges in `origin`, its fetch URL and every push URL (`git@host:path.git`, `ssh://[user@]host[:port]/path`, `https://host/path`, with or without `.git`, and aliases git resolves through `insteadOf`). Each project counts once. Their sync is on, with no login: git's credential helper picks the account.

Only `origin` is read: a fork's `upstream`, or a colleague's remote, isn't where your team's issues go, and every forge found is pulled from. For one more forge, give `origin` a push URL there (`git remote set-url --add --push origin git@gitlab.example.com:group/project.git`, which also pushes your code there), or write the forges as tables.

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

The check runs the forge sync on a copy of the repository, and answers the forges' writes without making them: it first merges what the pull brings in, so what it counts is what the run then does. A real run needs a check of the last 15 minutes, and syncs only the forges that check read. The Work page's Send all sends alone, after a check.

Each run, the check too, also asks each forge who can be assigned on its project, and says how many it found. The assignee boxes (every person field) then offer them, with the people git-board knows. A forge that doesn't answer is a warning: the boxes keep its last list.

Off (`sync = false`), the forge is kept but stays out of it, until you switch it on again in Settings › Sync. A forge found in the remotes always syncs: to leave one out, give the forges tables.

## Removing a forge

**Remove**, in Settings › Sync, drops the forge's table from `.git-board.toml`. The issues it brought stay, still linked to it, and so does the token in git's credential helper (`git credential reject` removes it).

## One issue

In an issue's panel, On the forges lists each forge it is on, and its timeline marks what hasn't reached each forge yet:

- **Send now** sends its waiting changes, **Refresh from forges** gets the forges' own. Both check first, and ask before writing to a forge.

An issue is linked to a forge issue once, by the sync, and stays so: linking by hand, unlinking and taking a forge's version are out of git-board's first version.

## On the command line

A sync moves your issues to three places, and each has its own commands with git's verbs:

| Side | What it holds | Commands |
| --- | --- | --- |
| The git remote (origin) | the board itself, merged without loss | `git board fetch`, `pull`, `push` |
| The forges | a translation of it, both ways: a write there emails people | `git board forge pull`, `push`, `sync` |
| The GitHub Project | field values, matched to labels | `git board project pull`, `push`, `sync`, `resolve`, `configure` |

Each acts at once and takes `--check`, which says what it would do and writes nothing. `pull` takes the other side's changes, `push` sends this clone's, `sync` does both.

`git board sync` runs the whole sync, as Settings › Sync does, each step in a line; `--check` is its dry run, and `--git` syncs with git alone: a pull, then a push. It has no check to wait for: `--check` first is yours to run. Creations wait for consent all the same: on a forge never synced with, or 50 or more on one in a run, none is made without `--allow-create`, and the sync says how many wait. It and `git board switch` print the forges found in the remotes, one line each: name, host and project, a GitLab one's with its id, and each URL they skip.

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


## Leaving git-bug

A repository whose issues were git-bug's moves to git-board in this order:

1. Stop git-bug's bridge sync wherever it runs: CI, cron, other clones.
2. `git board switch`, read its report, then `git board switch --apply`.
3. `git board sync`. It pushes `refs/board` and the switch's mark to
   origin.
4. Every other clone runs `git board sync`, which switches it too. No one
   else runs the switch.
5. Once no clone or automation uses git-bug, `git board prune` removes
   git-bug's data from the clone: its refs (`refs/bugs`, `refs/identities`
   and the old release records in `refs/releases`), its cache
   `.git/git-bug` and its `git-bug.*` config keys. `-n` says what it would
   remove. The refs go to a bundle in the git directory first, which it
   names: `git fetch <bundle> 'refs/*:refs/*'` brings them back.
6. Last, `git board prune --remote origin` deletes git-bug's refs on
   origin too. Nothing backs those up: a clone still on git-bug loses its
   shared issues.

## Starting over

A board can start over from what the forges hold, on the git side alone: no forge and no GitHub Project is touched. Every clone needs this version of git-board or a later one first: an earlier one doesn't know the guard below.

1. `git board reset --hard` empties this clone's board: the issues, the people, the sync links, the release records, the fetched copies and `board.me`. The switch's mark and the numbers' counter stay, so no number is handed out twice.
2. `git board push --prune` makes origin's board this clone's: what origin has and this clone doesn't is deleted there. It refuses on a clone that didn't start over, and keeps a numbers' counter a teammate moved on meanwhile.
3. Every other clone stops its automated syncs, then runs `git board reset --hard origin`, which makes its board origin's.
4. `git board forge pull` refills the board from the forges: on a big project, several minutes (see [Long syncs](#long-syncs)).

Each writes what it deletes to a bundle in the git directory and names it, with the `git fetch` that brings it back.

**What a forge pull brings back.** Every issue and comment that came from a forge. Not yet what git-board itself posted to a forge: that text carries git-board's marker, which names an issue the empty board no longer has, and the pull leaves it. A repository where git-board never posted to a forge misses nothing.

**The guard.** A reset writes a mark, and a pull or a push between two boards whose marks differ refuses: merged, the old board and the new one would hold every issue twice, both linked to the same forge issue. It says which side started over and how to settle it:

- a clone that missed the start-over: "origin's board was started over on 2026-10-28 15:04: git board reset --hard origin takes it";
- the clone that started over, before its `push --prune`: "this clone's board was started over on …, origin's wasn't: git board push --prune makes origin match, git board reset --hard origin goes back to it".

A new clone, with no board yet, takes origin's mark as it pulls.

Settings › Sync, Start over, runs the same. On the command line the resets refuse while `git board serve` runs on the repository.

## Long syncs

The first sync with a forge, or the first after starting over, reads every issue the forge holds. A thousand issues take several minutes, mostly for their comments: one request per issue that has any. The listing comes in within seconds, and the bar then has an end: "GitHub (gh) · pulling 412 of 1,034 · #412 Export CSV · about 6 min left". With 200 issues or more to read, it says the sync takes minutes. The command line says so too, and where it is every hundred issues.

You can close the page: the sync runs in git-board's server, not in the page. Open Settings › Sync again to follow it.

**When a sync is cut.** A shutdown, a crash or a stopped server breaks nothing:

- **What it pulled stays.** Each issue is written as the pull reaches it, and nothing is made twice.
- **The next sync isn't blocked.** The lock a run holds is the system's, and it goes with the process.
- **The next run goes on from where it stopped.** Its journal, in `board/sync.lock` in the git directory, lists the issues it read. The next sync, or check, skips those not updated since.
- **You're told where it stopped.** Settings › Sync says: "The last sync stopped today 14:02, at 612 of 1,034 issues from GitHub (gh)…". The command line says it as well.

**When the connection drops.** A read waits and tries again, after 1, 2, 4 seconds and so on, up to 30 seconds between tries and for 5 minutes in all: long enough for a Wi-Fi drop or a laptop asleep. The progress says so: "GitHub doesn't answer: trying again in 30 s". It says a rate limit's wait the same way. A write is never tried again, as the forge may have made it: the sync stops, and the next one goes on.
