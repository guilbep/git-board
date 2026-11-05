# Related work

git-board keeps a project's issues as the team's own data, independent of
any forge. It is not the first tool to try. This page compares it with the
others: what each one keeps, how it merges, how it talks to forges, and
what it does better. Then it says what git-board did well, what it did
wrong, what it missed, and what to do next.

- Written 2026-11-05, the day v0.12 shipped. Every tool was checked that
  day: its repository, its docs, and its last commit or release.
- A claim about another tool links to its source. A claim about git-board
  links to its ADRs, spec or guide.
- [ADR 0003](adr/0003-own-core-instead-of-git-bug.md)'s clean room asks
  whoever implements the core not to read git-bug's code or spec. This page
  quotes git-bug's public design notes. It is for reading, not for
  implementing from.

## 1. The landscape

### The tools at a glance

| Tool | Kept in | Two clones edit the same field | Forge sync | Who wrote it | Surfaces | Alive (2026-11-05) |
|---|---|---|---|---|---|---|
| **git-board** | operation logs: git refs, PostgreSQL or a Markdown folder | a fixed order every clone agrees on (clock, then pack hash) | GitHub, GitLab (self-hosted too), GitHub Projects; both ways, field by field | person entities with forge accounts; not signed | CLI, TUI, local web app, HTTP API | v0.12.0, 2026-11-05 |
| [git-issue](#git-issue) | plain files in a git repository of their own | git's text merge; a conflict is the user's | GitHub, GitLab.com; import and export, whole issues | git's author name and email | CLI | 3 commits in 2025, the last 2025-10-18 |
| [git-bug](#git-bug) | operation logs in git refs | a fixed order (Lamport clock, then pack id) | GitHub, GitLab, Jira, Launchpad | identity entities; signing keys partly built | CLI, TUI, web UI, GraphQL | active; v0.11.0, 2026-09-22 |
| [bug](#bug) | a plain-text `issues/` folder | git's or hg's text merge | GitHub import only | none | CLI | last commit 2019-03-06 |
| [Bugs Everywhere](#bugs-everywhere) | files in a `.be` folder | the VCS's text merge | none | free-form names | CLI, web, email, HTML | last release 2013-06-28 |
| [deft](#deft) | plain-text files beside the code | the VCS's text merge | none | none | CLI | last commit 2011-08-20 |
| [Fossil](#fossil) | ticket change artifacts in the Fossil repository | replayed in timestamp order | none (Fossil is its own forge) | repository accounts | web UI, CLI | active; 2.28 packaged 2026-03 |
| [git-appraise](#git-appraise) | git notes, one JSON line each | `cat_sort_uniq` notes merge | mirror tools for GitHub and Phabricator | git's author | CLI, separate web UI | last commit 2023-08-11 |
| [git-dit](#git-dit) | one commit per message, under `refs/dit/` | messages form a tree; no field merge | none | git's author | CLI | last commit 2025-06-01 |
| [`gh`, `glab`](#gh-and-glab) | the forge's database | the forge's | they are the forge | the forge's accounts | CLI | active, official |
| [Perceval](#perceval) | JSON items it fetches | n/a: read only | reads GitHub, GitLab, Jira and 30 more | the source's | CLI, Python | 1.6.1, 2026-09-21 |
| [SD](#sd-simple-defects) | Prophet replicas (SQLite or files) | conflicts resolved, and the resolutions synced | RT, Trac, GitHub, Google Code, Hiveminder | replica ids | CLI, local web UI | page last edited 2013; Prophet 0.751, 2013-05-20 |
| [Radicle](#radicle) | collaborative objects (CRDTs) in git refs | CRDT merge | none; it replaces the forge | Ed25519 keys | CLI, web; a TUI in progress (2024) | active; 1.9.0, 2026-05-19 |

### git-issue

[git-issue](https://github.com/dspinellis/git-issue), by Diomidis
Spinellis, is "a minimalist decentralized issue management system based on
Git, offering (optional) bidirectional integration with GitHub and GitLab"
([README](https://github.com/dspinellis/git-issue/blob/e252789ab0cde61629ef394c61b25b0a7916b6a5/README.md)).
It is one shell script, `git-issue.sh`, and a library for the forges; it
needs `jq` and `curl`. The repository was created in January 2016
([repositorystats](https://repositorystats.com/dspinellis/git-issue)). 377 of
its 514 commits are from 2018 and 2019; the forge sync came from Google
Summer of Code 2019 (the README's build badge points at
`eellak/gsoc2019-git-issue`, and the GitLab import is dated July 2019 in
the history). Spinellis presented it at
[FOSDEM 2020](https://archive.fosdem.org/2020/schedule/event/git_issue_management/).
No paper was found: the repository has a
[CITATION.cff](https://github.com/dspinellis/git-issue/blob/e252789ab0cde61629ef394c61b25b0a7916b6a5/CITATION.cff)
for citing the software itself. GPL-3.0.

- **Storage.** A separate git repository in `.issues/` (its own `.git`;
  `init -e` uses the project's). Each issue is a folder
  `issues/xx/xxxxx…`, named by the hash of an empty commit made for it.
  One file per attribute: `description`, `tags`, `assignee`, `milestone`,
  `weight`, `duedate`, `timeestimate`, `timespent`, `watchers`, and a
  `comments/` folder. "Open" and "closed" are tags.
- **Merging.** `git issue pull` is `git pull` in `.issues`
  ([git-issue.sh, L1525-1532](https://github.com/dspinellis/git-issue/blob/e252789ab0cde61629ef394c61b25b0a7916b6a5/git-issue.sh#L1525-L1532)).
  No merge driver is set, so two clones that edit the same file meet git's
  text merge.
- **Forge sync.** `import` pages through every issue of a GitHub or
  GitLab project, comments included, and overwrites the local files with
  what the forge says. `export` sends every imported issue changed since
  its last import as a whole: title, body, labels, assignees and state, in
  one `PATCH`
  ([import-export.sh, L798-869](https://github.com/dspinellis/git-issue/blob/e252789ab0cde61629ef394c61b25b0a7916b6a5/lib/git-issue/import-export.sh#L798-L869)).
  There is no "last agreed" value, so a change made on the forge since
  the import is overwritten by an export. The hosts are `api.github.com`
  and `gitlab.com`, written in the code
  ([L380-414](https://github.com/dspinellis/git-issue/blob/e252789ab0cde61629ef394c61b25b0a7916b6a5/lib/git-issue/import-export.sh#L380-L414)).
  A `checkpoint` file records the last import "for merging", but nothing
  reads it yet
  ([L966-971](https://github.com/dspinellis/git-issue/blob/e252789ab0cde61629ef394c61b25b0a7916b6a5/lib/git-issue/import-export.sh#L966-L971)).
  Links to forge issues are files under `imports/<provider>/<user>/<repo>/`.
- **Identity.** git's author name and email; an assignee is an email, or a
  forge login after an import.
- **Surfaces.** The CLI, with `list -l` format strings, `filter-apply`
  (run a command in every issue's folder) and `dump` (the whole database
  as JSON). `attach` is documented but answers "Not implemented yet"
  ([L1489-1492](https://github.com/dspinellis/git-issue/blob/e252789ab0cde61629ef394c61b25b0a7916b6a5/git-issue.sh#L1489-L1492)).
- **Better than git-board.** Nothing to build: one script, packaged for
  Guix ([packages.guix.gnu.org](https://packages.guix.gnu.org/packages/git-issue)).
  The files read with `cat` and change with any editor. Time spent,
  time estimates and weight are fields, and map to GitLab's.
- **git-board does better.** Concurrent edits merge without a conflict;
  sync compares field by field and never overwrites a forge's change it
  hasn't seen ([Sync](spec/sync.md)); any GitLab or GitHub Enterprise
  host; numbers, boards, sprints, a web app.
- **Worth taking.** `export -e`: a field the forge doesn't have goes into
  the description as text. A `dump` of the whole board. GitLab's time
  tracking mapped to fields.

### git-bug

[git-bug](https://github.com/git-bug/git-bug) "is a bug tracker that is
fully integrated in git" and "bridges to other bug trackers"
([README](https://github.com/git-bug/git-bug/blob/trunk/README.md)).
git-board was built on it, then on a fork of it, until October 2026
([ADR 0003](adr/0003-own-core-instead-of-git-bug.md)). GPL-3.0.

- **Storage.** An entity is a chain of commits under `refs/<namespace>/<id>`.
  Each commit holds an "OperationPack", a JSON blob of operations, and
  Lamport clocks in its tree. Its id is the hash of its first operation
  ([data model](https://github.com/git-bug/git-bug/blob/trunk/doc/design/data-model.md)).
- **Merging.** Two heads are joined by a merge commit; operations are
  ordered by "the edition's lamport clock if not concurrent", then "the
  lexicographic order of the OperationPack's identifier" (same page).
- **Forge sync.** Bridges to GitHub, GitLab, Jira and Launchpad. Imports
  and exports are incremental; assignees and milestones are not synced
  ([feature matrix](https://github.com/git-bug/git-bug/blob/trunk/doc/feature-matrix.md)).
- **Identity.** Identities are entities too. Public keys, key management
  and "identity protection" are partly built in the core and not in the
  CLI (feature matrix). Commits are signed when the author has a key
  (`entity/dag/operation_pack.go`).
- **Surfaces.** CLI, terminal UI, web UI, GraphQL API. A board entity is
  partly built; comment deletion is in the core only (feature matrix).
- **Alive.** v0.11.0 on 2026-09-22; commits merged on 2026-11-05.
- **Better than git-board.** Four bridges; signed authorship designed in;
  backers and sponsors on Open Collective (README).
- **git-board does better.** Assignees, fields, boards, sprints and the
  GitHub Project; sync state as operations (ADR 0008), where the fork kept
  it as metadata strings that git-bug's own bridges misread; MIT.
- **Worth taking.** Signed packs and identity keys. Media kept in the
  entity's tree, so attachments travel with the board.

### bug

[bug](https://github.com/driusan/bug) "writes code
problem reports to plain text files": an `issues/` folder, one
"descriptive subdirectory per issue", versioned with git or Mercurial
([README](https://github.com/driusan/bug/blob/d7f4e7f33fea25010433ae651d629242abfcd276/README.md)).
Issues "branch and merge along with the rest of your code, and you can
resolve conflicts using your standard tools". It imports from GitHub and
from Bugs Everywhere (`bug-import/`). It lists issues as `Issue 1`, their
place in the list. Last commit 2019-03-06.

- **Worth taking.** The idea that a board read without the tool is a
  feature. git-board's [Markdown folder](guide/datastores.md#markdown-files)
  already does it.

### Bugs Everywhere

[Bugs Everywhere](https://pypi.org/project/bugs-everywhere/) (`be`) keeps
"the bug information" with the source code, in a `.be` folder, for Arch,
Bazaar, Darcs, Git, Mercurial and Monotone. Its ids are "globally unique"
because "bugs and comments can be created by several users in parallel"
([tutorial](https://bugs-everywhere.readthedocs.io/en/master/tutorial.html)).
It has a CLI, web and email interfaces, and `be html`, a static dump. Each
bug is a folder of `values` (JSON) and `body` files
([a review](https://www.cbaines.net/posts/bugs_everywhere_review/)).
The last release on PyPI is 1.1.1, of 2013-06-28, for Python 2.7. Its
homepage, `bugseverywhere.org`, redirected to an unrelated site on
2026-11-05.

- **Worth taking.** An email interface: people file and answer without an
  account anywhere.

### deft

[deft](https://github.com/npryce/deft) (2011) is a
"distributed feature tracker". Its principles include "absolute
prioritisation of features that have the same status", and "don't
re-implement functionality that is already in the VCS"
([README](https://github.com/npryce/deft/blob/307fc8d0bfd64da2a6586c0806c7e118589e6351/README.rst)).
Each status is an ordered file (`tracker/status/in-progress.index`), one
feature per line. Last commit 2011-08-20.

- **Worth taking.** A rank: the order of the issues in a column is data,
  not a sort. GitHub Projects has it too (below).

### Fossil

[Fossil](https://fossil-scm.org/) is a version control system with tickets,
a wiki and a web UI in one executable. A ticket is a series of "ticket
change artifacts"; the `TICKET` table is replayed from them "in time stamp
order", and "can always be reconstructed from the ticket change artifacts"
([tickets](https://www.fossil-scm.org/home/doc/trunk/www/tickets.wiki)).
"Tickets do not branch", and "casual passers-by on the internet should be
permitted to create tickets" ([bug theory](https://www.fossil-scm.org/home/doc/tip/www/bugtheory.wiki)).
Fields are added to the table's SQL, and the screens are TH1 scripts.
Fossil 2.28 reached FreeBSD's ports in
[March 2026](https://lists.freebsd.org/archives/dev-commits-ports-all/2026-March/195488.html).

- **Better than git-board.** Anyone can file a ticket on the project's web
  page, with no forge and no clone. One file to install.
- **git-board does better.** A logical clock, not wall clocks, orders
  edits; it works with git and the forges people already use.
- **Worth taking.** A public intake for people outside the team.

### git-appraise

[git-appraise](https://github.com/google/git-appraise) is "distributed code
review for git". Reviews, comments, CI results and robot comments are git
notes under `refs/notes/devtools/`, one JSON line each, merged with the
`cat_sort_uniq` strategy
([README](https://github.com/google/git-appraise/blob/be4751005fb49ddfc7b7f01259d1d4906aa288bf/README.md)).
Mirrors to GitHub pull requests and Phabricator are separate tools. Last
commit 2023-08-11. [ADR 0001](adr/0001-check-results-are-signed-git-notes.md)
already weighed its notes layout for check results.

### git-dit

[git-dit](https://github.com/neithernut/git-dit) stores each issue and
comment as a commit, "similar to threads in a mailing list"; an issue's
head is `refs/dit/<issue-hash>/head`, and status is a trailer such as
`Dit-status` ([data model](https://github.com/neithernut/git-dit/blob/d35a06cbca6721b5954d8ecd36d8f5d1e1c6f69e/doc/datamodel.md)).
Its [caveats](https://github.com/neithernut/git-dit/blob/d35a06cbca6721b5954d8ecd36d8f5d1e1c6f69e/doc/caveats.md)
name the problem every git-only tracker has: issues must be pulled from
the reporters' public repositories, "otherwise, issue reporters would
require push access". Last commit 2025-06-01, a dependency fix; the one
before is from 2020.

### `gh` and `glab`

[`gh`](https://cli.github.com/manual/gh_project) and
[`glab`](https://docs.gitlab.com/cli) are GitHub's and GitLab's own CLIs.
They keep nothing locally: each command is an API call. `gh project` edits
GitHub Projects' fields and items. git-board's CLI follows their flags
where it can ([Coming from gh / glab](guide/cli.md#coming-from-gh-glab)).

### Perceval

[Perceval](https://pypi.org/project/perceval/), part of CHAOSS's
GrimoireLab, fetches data from git, GitHub, GitLab, Jira, Bugzilla,
mailing lists, chats and more, for analytics. It reads; it doesn't write
back. 1.6.1 on 2026-09-21.

- **Worth taking.** Breadth of reading: a one-way import from a tracker
  git-board doesn't sync with (Jira, Bugzilla, Redmine) would let a team
  move its history in.

### SD (Simple Defects)

[SD](https://syncwith.us/sd/) is "a peer-to-peer bug tracker", with SQLite
and file backends, sync over the network or by "sneakernet", and bridges
to RT, Trac, GitHub, Google Code and Hiveminder. It runs on Prophet, a
distributed database in Perl. When a change "won't apply cleanly", Prophet
records a resolution, and resolutions are synced like other records
([Prophet::Replica](https://metacpan.org/release/JESSE/Prophet-0.71/view/lib/Prophet/Replica.pm)).
The page was last edited in 2013; Prophet's last release is 0.751, of
2013-05-20 ([metacpan](https://metacpan.org/pod/Prophet::App)).

- **Worth taking.** A conflict resolved once is resolved on every clone.

### The LWN article, 2008

Jonathan Corbet's [Distributed bug tracking](https://lwn.net/Articles/281849/)
(LWN.net, 2008-05-14) reviews Bugs Everywhere, ticgit, Scmbug, DisTract,
DITrack and Ditz. Its two warnings still hold:

- conflicts: "changing the severity of a bug in two branches and merging
  the result creates a conflict which can only be resolved by
  hand-editing the bug tracker's files";
- users: "any distributed bug tracking system which does not facilitate
  this wider conversation will, in the end, not be successful". Most
  users won't run git to file a bug.

git-board answers the first with operation logs. It answers the second
with the forges: people outside the team file on GitHub or GitLab. Without
a forge, it has no answer yet.

### Radicle

[Radicle](https://lwn.net/Articles/966869/) is a peer-to-peer forge on
git. Issues and patches are "collaborative objects", CRDTs stored in git
refs: "any node can append data to a COB, and changes from different nodes
can be merged without conflict". Nodes have Ed25519 keys, and refs are
signed (LWN, 2024-03-29). The
[radicle-cob](https://docs.rs/radicle-cob/latest/src/radicle_cob/lib.rs.html)
crate is at 0.21.1, and Radicle 1.9.0 came out on
[2026-05-19](https://hn.jakechampion.name/reader/https://radicle.dev/2026/05/19/radicle-1.9.0).
It replaces the forge rather than mirroring one.

- **Better than git-board.** Every change is signed by a key; no forge at
  all.
- **Worth taking.** Signed operations. A Radicle repository as a forge
  adapter, one day.

## 2. Where we did well

1. **Sync is the best of the lot.** git-issue overwrites whole issues
   both ways and knows two hosts; git-bug's bridges skip assignees. git-board
   compares each field with the value both sides last agreed on, never
   echoes what it pulled, leaves markers that make a cut run safe to run
   again, posts for people without a token under a header naming them, and
   defuses mentions and `#n` that would point elsewhere
   ([ADR 0008](adr/0008-sync-state-in-the-data-model.md), [Sync](spec/sync.md)).
   Every adapter, the fake included, passes one conformance suite
   (`forge/forgetest`). It reads only what changed since the last run, with
   a margin ([Forges](spec/forges.md)).
2. **The data outlives the code.** One pack format, the same bytes and ids
   in git, PostgreSQL and a Markdown folder
   ([ADR 0002](adr/0002-datastores.md)); a board copied between them stays
   the same board. v0.12 rewrote everything and kept the data byte for
   byte, proven by `make compat` on golden boards
   ([ADR 0021](adr/0021-git-board-v2-same-data-new-code.md),
   [compat](rewrite/compat.md)). No other tool here offers a choice of store
   with the same ids.
3. **It is for project managers too.** Fields as labels, configured in a
   committed file ([ADR 0007](adr/0007-every-field-is-a-label.md)), boards,
   sprints, a roadmap, insights, and a two-way bridge to GitHub Projects
   ([CLI › project](guide/cli.md#project-the-github-project)). The
   git-only trackers here have lists; git-bug's board is partly built
   ([feature matrix](https://github.com/git-bug/git-bug/blob/trunk/doc/feature-matrix.md)).
4. **Numbers people can say.** `CO2-3075`, forge-independent, from one
   counter in git ([ADR 0016](adr/0016-project-numbers-are-a-counter-on-origin.md)).
   git-issue, git-bug, Bugs Everywhere and git-dit show hashes; bug shows
   a place in the list.
5. **Honest deletes.** A tombstone that hides, keeps history, and tells you
   a pasted secret is leaked ([ADR 0019](adr/0019-deleting-is-a-tombstone.md)).
6. **Measured speed.** After v1, budgets and complexity guards on boards of
   2,000 to 20,000 issues ([Performance](guide/performance.md)). The index
   is disposable, as Fossil's ticket table is
   ([ADR 0009](adr/0009-queries-read-a-disposable-index.md)).
7. **A permissive licence, checked.** MIT, with a test that fails on a GPL
   dependency ([ADR 0017](adr/0017-permissive-licences-few-dependencies.md)).
   git-issue and git-bug are GPL-3.0.

## 3. Where we did wrong

1. **We built before we looked.** The core's PRD (2026-10-24) has no
   survey of prior art. git-issue had synced with GitHub and GitLab since
   2019; git-bug's data model, which we used for weeks, is the model the
   core kept. The README's related-work list came on 2026-11-05, the day of
   v0.12, and follows git-issue's own list entry by entry. The release PRD
   did look (`docs/prd/release.md`, "Records in git"), and ADR 0001 has a
   prior-art list: the habit exists, but not where it mattered most.
2. **We reinvented git-bug's format without saying so.** An entity as
   commits under one ref, each holding a JSON pack; a Lamport clock; order
   by clock, then pack hash; merge commits with no operation
   ([ADR 0004](adr/0004-entities-are-operation-logs.md),
   [Storage](spec/storage.md)). That is
   [git-bug's design](https://github.com/git-bug/git-bug/blob/trunk/doc/design/data-model.md).
   Keeping it was right. Not crediting it, and not listing it among ADR
   0004's alternatives, was not.
3. **The road through git-bug cost two migrations.** git-board stood on a
   git-bug library, then a fork, then its own core
   ([ADR 0003](adr/0003-own-core-instead-of-git-bug.md)). The fork kept
   sync state in metadata strings that git-bug's bridges misread, which
   duplicated issues on GitHub. The switch lost what lived only in
   git-bug: issues never sent to a forge, local labels, git-board's caches.
   v0.12 no longer migrates git-bug repositories at all: v0.11 must do it
   first ([Leaving git-bug](guide/forges.md#leaving-git-bug)).
4. **v1's code had to be thrown away a month later.** Caches on caches, a
   facade of 125 methods, domain rules in HTTP handlers, races found by
   luck, and speed never budgeted (`ls` at 77 s on 20,000 issues)
   ([design, section 2](rewrite/design.md)). The data held; the code
   didn't.
5. **Parts of the board aren't owned.** Attachments go to an S3 bucket
   with EPFL's defaults, and an export keeps the links, not the files
   ([ADR 0015](adr/0015-attachments-on-s3-not-in-git.md),
   [Owning your issues](manual/ownership.md#attachments)). Authors are ids
   nobody checks: anyone who can push can write as anyone
   ([Data model](spec/data-model.md#operations)).
6. **"Any forge" is two forges.** GitHub and GitLab. Gitea, Forgejo and
   Jira wait "until someone asks" ([ADR 0003](adr/0003-own-core-instead-of-git-bug.md)).
   git-bug has four bridges, SD had five.
7. **The docs drift.** ADR 0003, 0004 and 0007 each list texts that still
   describe git-bug, a commit identity the code no longer writes, or a
   decision since amended.

## 4. What we missed

Ranked by value for the vision (no forge needed, the issues owned) and for
its users: developers, and project managers who keep their priorities in
GitHub Projects.

1. **A self-hostable forge: Forgejo and Gitea.** The forge a team leaving
   GitHub moves to. ADR 0003 left it for later; adding a forge of kind
   `gitea` is refused today.
2. **A way in for people outside the team.** Fossil lets passers-by file
   tickets; Bugs Everywhere took email; LWN said a tracker without it fails.
   git-board relies on a forge for this, so leaving every forge closes the
   door.
3. **Rank, synced with GitHub Projects.** A Project's item position "represents
   the priority of an item"
   ([GitHub GraphQL, `updateProjectV2ItemPosition`](https://docs.github.com/graphql/reference/projects)),
   and deft made rank a principle. git-board sorts by fields; the bridge
   doesn't carry the order PMs drag.
4. **An install without a clone.** git-issue is one script in Guix; Fossil
   is one file. git-board needs a clone and `gh`, or Go and Node
   ([Install](manual/install.md)).
5. **Signed operations.** git-bug designed signing keys; Radicle signs
   every ref. git-board already signs check results with SSH keys
   ([ADR 0001](adr/0001-check-results-are-signed-git-notes.md)), but not
   the board.
6. **Attachments in the store.** git-bug keeps media in the entity's tree.
   git-board's export loses the files.
7. **Time tracking.** git-issue and GitLab have time spent and estimates;
   git-board has neither field nor mapping (git-time is a separate tool).
8. **Imports from the other trackers.** git-issue's `.issues`, git-bug's
   refs (now only through v0.11), Jira. The people most likely to come
   already use one of these.
9. **A field a forge lacks, as text.** git-issue's `export -e` writes such
   fields into the description. git-board keeps them off that forge
   (held), so forge users don't see them.

## 5. How to be better

### Process

- **Look for prior art before a PRD,** and write what was found in it:
  name, link, what to take, what to avoid. A day of reading is cheaper
  than a migration.
- **Credit what we keep.** When a design comes from another tool, say so
  in the ADR and the spec.
- **Read a dependency's data model before building on it,** and ask what
  its other users' tools will do with our data. The fork's duplicates came
  from there.
- **Check features against the vision,** once per release: does this keep
  the issues ours, without a forge? Attachments and authorship fail it
  today.
- **Read old warnings.** LWN named, in 2008, the two problems every tool
  here met: merges and outsiders.
- **Keep this page current.** Recheck each tool when a release touches
  its area.

### Actions

Each is small enough to be an issue.

| # | Action | Why |
|---|---|---|
| 1 | A Forgejo/Gitea adapter passing `forge/forgetest` | "Any forge" needs a forge a team can host |
| 2 | Sync GitHub Project item position as a rank field | It is how PMs keep priority |
| 3 | A rank field on the web board: drag sets it, views sort by it | Rank without a Project, for teams that left GitHub |
| 4 | Release archives and an install script that needs neither `gh` nor a clone | Adoption starts with one command |
| 5 | Sign entity commits with the person's SSH key; mark unsigned authors | Owned data should say who wrote it, provably |
| 6 | A public "new issue" form on `git board serve`, off by default | An intake without a forge |
| 7 | Attachments kept in the datastore, as an option; export copies files | The export should hold the whole issue |
| 8 | `git board import --git-issue <dir>` | git-issue's users are the closest to ours |
| 9 | Map GitLab time tracking (`time_estimate`, `total_time_spent`) to fields | Data GitLab users already keep |
| 10 | Credit git-bug in ADR 0004 and [Storage](spec/storage.md) | Honest lineage |
| 11 | A "Prior art" section in the PRD template | Makes the first process lesson a habit |
| 12 | Fix the drift ADR 0003, 0004 and 0007 list under "Left open" | The spec is the contract |
