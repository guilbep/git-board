# Releases: one trunk, one commit, one image, promoted by tags

PRD, from the discussion of 2026-10-17/18 (branch `prd/release`). It replaces
the discussion stub that started this session. Decisions marked **(user)**
were made by the user. Those marked **(call)** are this document's, open to
change. Work-unit's milestones are in `docs/prd/work-unit.md`:
- M1 numbers, M2 roles and M3 the PR gate are built;
- M4, review in git-board, is not built yet.

## Why

ENAC-IT4R apps release by merging long-lived branches on GitHub: `dev` →
`stage` (pre-release PR), then `stage` → `main` (release PR). A GitHub
Actions workflow then reads `package.json`, tags `main` and creates a GitHub
Release. Three things are wrong with that:

- **The same code gets a new identity at every step.** Each merge mints a
  new commit, and each pushed ref builds its own images. co2 1.4.19 ran
  images `d26ab4…`/`0f6eb3…` in stage (enack8s-app-config c17f73959) and
  `33b43e…`/`64939d…` in prod (cb7628dc1): same code, different
  artifacts. What was tested in stage is not, byte for byte, what runs in
  prod.
- **The forge is a single point of failure.** Build, tag, changelog,
  release page and deploy trigger all run in GitHub Actions or GitLab CI.
  GitHub Actions was blocked for the whole org on 2026-09-22 (wiki,
  `Continuous Deployment.md`), and the GitLab fallback is manual.
- **It's a hand-run checklist.** The lead dev runs every step
  (`co2-calculator/docs/src/architecture/release-runbook.md`): bump the
  version, PR, merge, rewrite the changelog, sync back, PR, merge, sync
  back twice, check ArgoCD. That was six releases in ten days for
  1.4.14-1.4.19.

The direction is the one git-bug already took for issues: **decentralized
first**. The work happens on the developer's machine (theirs or their
agent's), records live in git, and forges are optional mirrors.

## Decisions

- **Promote, don't re-merge** **(user)**: one commit and one built image
  move through the environments.
- **A release is a git-bug entity** **(user)**, stored in git, replacing the
  GitHub Releases page.
- **The whole pipeline runs on the developer's machine, builds included**
  **(user)**. No forge CI is needed to merge, build, release or deploy.
- **The dev owns the merge** **(user)**: they merge their validated branch,
  make sure dev isn't broken, and make sure their work is deployed. Agents
  do most of it.
- **The gate on dev is review in git-board** **(user)**, meaning work-unit's
  M4 Approve. On repos that opt in, the `dev` ruleset drops the GitHub PR
  and required-checks rules and keeps no-force-push and no-delete. This
  replaces two earlier rules: work-unit's "the GitHub PR is the gate" and
  the wiki's "the lead dev merges all PRs".
- **Anyone with push may promote, prod included** **(user)**. There is no
  role check. Records say who did what.
- **Environments are marked by semver tags, not branches** **(user's idea,
  researched)**. `stage` and `main` go away.
- **The most resilient design** **(user)**: no daemon, no server that must
  be up, tolerant of a laptop closing halfway and of two people acting at
  once.
- **One remote is the authority** **(call)**: `origin`, as M1 decided for
  the issue-number counter. Its refusal of a second push is what makes
  `dev`, tags and `refs/releases/*` safe. Other remotes are mirrors (co2
  already mirrors GitHub to GitLab). If `origin` is down for long, the team
  moves the authority on purpose (`[release] remote`). It never happens
  automatically, because two writable authorities would let two clones
  both win the same tag.
- **v1 records are attributed, not signed** **(call)**. Each operation
  carries its git-bug author and each overlay commit its git author. Adding
  signing keys to identities today makes entities unreadable in several
  cases:
  - a second machine without the key writes unsigned packs, and clones
    reject them;
  - so does an agent in a container;
  - a teammate's bridge import, written under the keyed identity, does the
    same.

  The reviewer of this PRD traced each case in `entity/dag/entity.go` and
  `operation_pack.go`. Signing needs its own design for keys across
  machines, agents and bridges.

## How deploys work today

Facts the design builds on, checked in the repositories:

- **Building**: `epfl-enac-build-push-deploy-action` (v3.11.0) builds on a
  push of `dev`, `test` or `stage`, or of a `v*` tag (`deploy.yml`
  L665-672). It pushes `ghcr.io/<owner>/<org>/<repo>/<name>:<sha>`, adds
  the ref name with `crane tag`, and `crane copy`s the same digest to other
  registries (co2: quay-its.epfl.ch). It also packages the helm chart
  (co2: `./helm`, version `1.0.<run>`, `-rc` on stage, `-dev` elsewhere) and
  pushes it to `oci://ghcr.io/epfl-enac/co2-calculator/helm`. Tag pipelines
  always rebuild (`docs/deploy.md` L226).
- **co2 has two CI paths**: GitHub Actions for stage and `v*`, and
  gitlab.epfl.ch's pipeline for dev (`.gitlab-ci.yml`, since 2026-09-22).
  Both use the same action contract and write the same overlays.
- **Deploying is already a git commit.** The action sends a
  `repository_dispatch` to the config repo. A bot rewrites
  `<org>/<app>/overlays/<env>/kustomization.yaml` (`images[].newTag`,
  `images[].digest`, `helmCharts[].version`) and pushes to `main`, and
  Argo CD syncs. Of the last 200 commits of enack8s-app-config, 160 are
  these.
- **co2 deploys to two config repos**: `EPFL-ENAC/enack8s-app-config` (ghcr
  images) and `EPFL-ENAC/openshift-app-config` (quay images). Both have
  `epfl/co2-calculator/overlays/` dev, stage and prod.
- **Environments vary per app.** Of 110 apps with overlays in
  enack8s-app-config:

  | Environments | Apps |
  |---|---|
  | dev + prod | 46 |
  | dev + stage + prod | 23 |
  | prod only | 20 |
  | dev only | 15 |
  | with test | 2 |

  GitHub Pages sites (prod only) and legacy VMs (enacit-ansible,
  enac-cd-app) deploy elsewhere.
- **co2's images don't depend on the environment.** Runtime settings are
  injected at container start (`frontend/docker/entrypoint.sh` writes
  `injectEnv.js`). The one exception is the backend's `APP_VERSION` build
  arg, which gets a different suffix per ref: `-dev-<stamp>`, `-rc`, or
  none (`scripts/app-version.sh`). The frontend reports the git SHA.
- **Push rights**:
  - co2's `dev` has a ruleset: a PR with one code-owner approval, the merge
    method only, six required Actions checks, no force-push, no deletion.
  - enack8s-app-config's `main` only forbids force-push and deletion, and
    24 accounts can push to it.
  - The quay robot credential (`svc1751+ghaction`) lives in Actions
    secrets.

## Goals

1. One commit, one set of image digests and one chart per release, the same
   in every environment.
2. Merge, build, release and deploy run from git-board on the developer's
   machine, as a command (for agents) and a button (for people).
3. Every step can be interrupted and finished later by anyone, from any
   clone.
4. A release page that comes from git: version, commit, images, the issues
   carried, notes, and where it runs now.
5. Each issue shows which environments it has reached.
6. Works for any set of environments: dev only, prod only, dev + prod, dev
   + stage + prod, with test.

## Non-goals

- Mirroring releases to GitHub Releases or GitLab Releases. A bridge can
  come later, like the issue bridges.
- Merge queues and speculative CI (bors, GitHub merge queue, Uber
  SubmitQueue): the compare-and-swap retry below is enough for a team this
  size.
- Feature-flag tooling.
- Renaming `dev` to `main`.
- Signed records (see Decisions), GitHub Pages and legacy VMs in v1.

## Anti-goals: it fails if…

- What runs in prod is not the digest that was in stage.
- A slower or older laptop can overwrite a newer deploy.
- A hotfix that went to prod can be silently lost by the next release.
- A release depends on any one machine or daemon being up, or on a forge's
  CI.
- Git and the cluster disagree about what runs where, with no way to tell.

## The model: three things, one job each

| Thing | Job | Moves? | Lives in |
|---|---|---|---|
| **Tags** | name a candidate or a release | never (git-tag(1), "On Re-tagging") | the app repo |
| **Overlays** | say what runs in each environment | forward, and back on rollback | the config repos |
| **Release records** | hold the page: digests, chart, notes, issues, comments | notes and comments only | `refs/releases/*` in the app repo |

**Every step means "make it so", not "do this".** A step reads the desired
state from git, does nothing if it already holds, and can be finished by
anyone. The only locks are git's own: a push without `--force` is refused
unless the remote ref is still what you fetched, which makes it a
compare-and-swap. The authority remote is the one whose refusal counts.

## Ship: the dev merges

`dev` is the only branch that matters. It is the trunk, and the repo's
default branch. Work lands on it with `--no-ff` merges, as decided in
work-unit. **Ship** is one git-board action, a button on the issue and
`git-board ship <issue>` for agents:

1. **Fetch.** Merge the issue's branch onto a fresh `origin/dev`, in a
   scratch worktree, so the developer's checkout is untouched.
2. **Check.** Run `[release] checks` (e.g. `make ci`) on that exact tree.
   This is the "not rocket science rule": `dev` only ever receives a tree
   that passed.
3. **Commit** the merge with trailers:
   - `Reviewed-by:` the approvals from the issue's M4 review;
   - `Checked-by: <identity> <checks command>`.

   They travel with the code, appear on every forge, and need no extra
   storage. They are self-declared, like Basecamp's gh-signoff.
4. **Push `dev`** to the authority remote. If someone pushed first, the push
   is refused. Ship then re-fetches, re-merges, re-checks and retries.
   After three tries it stops and says why.
5. **Converge dev** (below).

**The approval Ship requires** **(user)** is one M4 Approve from the
issue's `reviewer` or `lead-dev` (M2 roles: the issue's own, else the
project's). An approval at an older commit still counts and shows "at
d4e5f6a, 2 commits ago", as co2's code-owner rule works today (it doesn't
dismiss stale approvals). **The branch's author can't approve their own
branch** **(call)**, as GitHub's code-owner rule refuses it today. Agents
working under the author's identity count as the author. `--no-review`
skips the check, which is the "Ship" of Ship/Show/Ask and the way through
on a one-person project, and is recorded in the trailer.

**Ship depends on M4** and on the `dev` ruleset change. Where `dev` still
requires a GitHub PR, Ship refuses and says so: "dev requires a GitHub PR:
merge it from the issue (M3, `internal/ghpr`), or change the ruleset".
Converge and releases
don't care how commits reached `dev`, so they work on such repos too, and
come first (see Migration).

## Converge: make an environment so

One primitive, `converge <env> [<release>]`, serves both cases:

- **eagerly**: Ship ends by converging dev;
- **on demand, from anywhere**: git-board shows drift ("dev runs 2 merges
  behind `dev` — Deploy"), and anyone can converge.

That's the middle between "deploy on merge" and "deploy on demand". It
needs no daemon. A machine *may* run `git-board converge --watch`, but none
has to.

The target commit is `dev`'s head for dev, and a release's commit for any
other environment.

1. **Ensure the images.** For each `[release] images` entry and each
   registry:
   - If the image tagged `:<sha>` exists **and** its
     `org.opencontainers.image.revision` label is that SHA, skip the build.
     The action's `reuse_unchanged_images` can leave a `:<sha>` tag on an
     image built for an older commit, which is why the label is checked.
   - Otherwise build with `docker buildx build --platform linux/amd64`. The
     build gets the revision label, a Trivy scan (the gate the action has
     today), a push to the first registry, and a `crane copy` of the same
     digest to the others.

   Two machines building the same SHA is harmless: the overlay records the
   digest it was given.
2. **Ensure the chart**, if `[release] chart` is set. `helm package` it with
   the version of what is being deployed:
   - the dev version for dev;
   - `1.0.1-rc.2` for a candidate;
   - `1.0.1` for a release.

   Then `helm push` it to its OCI registry. A candidate's chart is packaged
   once; promoting to the final re-pushes the same chart content under the
   final version. Its version and digest are pinned on the release record.
3. **Ensure each overlay.** For each `[[release.deploy]]` entry, write into
   the overlay:
   - each `images[]` entry, matched by name, with its digest and `newTag`;
   - `helmCharts[].version`;
   - the source commit, as an annotation, so anyone can read which commit
     runs;
   - `APP_VERSION` as an environment value.

   Commit (`chore(manifest): <app> (<env>) <version> [by <identity> via
   git-board]`) and push. If the push is refused because the config repo is
   shared, fetch, replay the one-file change, re-check, retry.
4. **Monotonic rule.** An environment only moves to a commit that
   *descends* from the one it runs (`git merge-base --is-ancestor`). An
   older build can never overwrite a newer deploy. Two moves are the
   explicit exceptions, and each is recorded: `rollback`, and a hotfix into
   an environment whose running commit it doesn't descend from (see Tags).

**Across repos**: app repo, config repo A and config repo B are separate
pushes, with no atomicity between them. Each is converged and checked on
its own, so a half-done converge is visible ("prod: enack8s on v1.0.1,
openshift on v1.0.0") and is finished by the next converge.

**Adopt, once per app.** No overlay carries a source-commit annotation
today, and dev's `newTag` is the literal `dev`, so the monotonic rule has
nothing to compare against. `git-board release adopt` reads each overlay
and proposes the commit it runs:
- the commit of the tag in `newTag` (`v1.4.19`);
- for dev, the commit whose image matches the digest, by revision label.

The user confirms each proposal, and adopt writes the annotations.
Converge refuses an overlay that hasn't been adopted.

## Tags

| Event | Git tag | Images | Overlays |
|---|---|---|---|
| Ship into dev | none; the version is derived, e.g. `1.0.1-dev.12.gabc1234` | built once, `:<sha>` | dev → that digest |
| Candidate | annotated `v1.0.1-rc.N` on commit C | same digest, plus `:v1.0.1-rc.N` | stage (or test) → same digest |
| Release | annotated `v1.0.1` on the **same** commit C | same digest, plus `:v1.0.1` | prod → same digest |
| Candidate fails | `rc.N` stays as history; fix on dev, then `rc.N+1` on the new commit | built for the new commit | — |
| Rollback | no tag moves | — | prod → the previous release's digests |
| Hotfix | branch `hotfix/<v>` from `v1.0.1`, fix, then `v1.0.2` | built for the hotfix commit | prod → hotfix digest |

Rules:

- **The tag names the candidate, not the environment.** `v1.0.1-rc.3` can
  go to test and then to stage. Every environment between dev and the last
  takes candidates. The last environment takes final releases.
- **A final is promoted from a candidate** in apps that have one: the final
  tag goes on the commit of a candidate that ran in the environment before
  last.
- **Apps with only two environments, or one, skip the rc.** dev + prod
  releases straight to `v1.0.1`. Prod-only releases from `dev`'s head.
  An app whose one environment follows the trunk tags what it runs, a
  final, and deploys nowhere; it has nothing to promote.
- **Dev's version is derived, never stored.** It is the next patch, plus the
  number of commits since the last tag, plus the SHA, in the style of Go
  pseudo-versions. SemVer ranks it below every rc: `1.0.1-dev.12.gabc1234
  < 1.0.1-rc.1 < 1.0.1`. It uses no `+`, because Docker tags can't contain
  one.
- **The tag is pushed first**, to the authority remote, before anything else
  is written. It acts as the compare-and-swap on the version name: of two
  clones cutting `rc.2`, one push is refused and that clone takes `rc.3`.
  Offline (`release cut --offline --no-deploy`), the tag waits as a ref
  outside `refs/tags`; the next online cut, promote, rollback or deploy
  checks it again, pushes it and builds it, or drops it and says why.
- **Hotfixes skip the candidate step.** co2's release policy already says a
  hotfix may "skip staging (emergency only)" (`release-management.md`).
  - A hotfix descends from the commit running in prod, so prod takes it
    under the monotonic rule.
  - Converging it into stage, whose running commit is a newer `dev` commit,
    is an explicit override, like rollback.
- **No lost hotfix.** A new candidate or release must descend from the
  commit running in prod. The next release is therefore refused until the
  hotfix branch is merged into `dev`.
- **The next version is proposed, not imposed.** The proposal is a minor
  bump if any issue it carries is `kind:feat`, a major bump if any is
  marked breaking, a patch otherwise. It is editable before the tag is
  pushed.

`stage` and `main` are frozen at the switch, then deleted once nothing
reads them (co2: `deploy.yml`, `changelog.yml`, `release-please.yml`, and
the docs' links to `main`). No mirror branches are kept, because tags carry
what they carried.

## The release record

A new entity type defined in git-board on git-bug's `entity/dag` (see
`git-bug/entity/dag/example_test.go`), in its own namespace
`refs/releases/<id>`. The git-bug fork doesn't change.

- **One record per released commit**, frozen at creation:
  - commit;
  - image digests per image and registry;
  - chart version and digest;
  - previous final release.

  In Kargo's terms it is the Freight.
- **Its names are the tags on its commit**, read from git, never stored
  **(call)**. A record whose commit carries `v1.0.1-rc.2` shows `v1.0.1`
  as soon as that tag is pushed. A failed `rc.1` is another commit, so
  another record.
- **Creating a record is itself a convergent step** **(call)**. Any clone
  that sees a release tag with no record for its commit creates it,
  reading the digests from the registries by revision label. A laptop that
  dies between the tag push and the record leaves nothing to repair by
  hand. If two clones race, the record with the earliest Lamport time
  counts and the other is ignored.
- **Operations**:
  - create (commit, digests, chart, previous)
  - set notes
  - comment
  - yank (with a reason; a yanked release can't be promoted)
- **Notes** are drafted from the issues merged since the previous final
  release. git-board walks the first-parent merges on `dev` between the two
  commits and maps branch names to issues, with the same code as
  "merged" (`internal/branches`, `Issue.BareNumber`/`HasNumber` in
  `internal/gitbugio`). They are grouped by
  `kind`, editable, and an agent can rewrite them, as the 1.4.19 notes
  were.
- **Attributed**: each operation carries its git-bug author. No signing in
  v1 (see Decisions).
- **Travels with git**: `git bug push` only carries git-bug's own
  namespaces, so git-board's push and pull add `refs/releases/*`.
- **Not stored again**: who promoted what, and when, is read from the config
  repos' history of each overlay, which has the author, date and digest.
  The overlay stays the single truth for what runs where, because Argo
  reads it.

**The release page** is git-board's Releases view, also written by the
static `html` export for people without git-board. It lists finals with
their candidates folded under them. Each release shows:
- version, date, commit;
- images with registry links, and the chart;
- issues carried and notes;
- **where it runs now**, read from the overlays: "prod (enack8s,
  openshift) · stage".

`CHANGELOG.md` is no longer committed at release time. If a repo wants
one, `git-board release notes --all` writes it, and it lands with the next
Ship like any other change.

## Where an issue has got to

An issue has reached an environment when its merge commit is an ancestor
of the commit that environment runs, read from the overlay annotation.
This is the existing "merged" computation run against more commits. It
appears as one line in the issue's Development section: "in dev · in stage
· not in prod". A filter qualifier (`env:prod`) can follow.

## Configuration

```toml
[release]
remote = "origin"                         # the authority
environments = ["dev", "stage", "prod"]   # ordered; any subset
follow = "dev"                            # optional: dev follows the trunk
checks = "make ci"
images = [
  { name = "frontend", context = "./frontend" },
  { name = "backend",  context = "./backend" },
  { name = "docs",     context = "./docs" },
]
chart = { path = "./helm", registry = "oci://ghcr.io/epfl-enac/co2-calculator/helm" }
version_values = ["backend.env.APP_VERSION"]  # under the chart's valuesInline
builder = ""   # optional: a remote buildx builder instead of the local one

[[release.deploy]]
repo = "git@github.com:EPFL-ENAC/enack8s-app-config.git"
path = "epfl/co2-calculator/overlays/{env}"
registry = "ghcr.io/epfl-enac/epfl/co2-calculator"

[[release.deploy]]
repo = "git@github.com:EPFL-ENAC/openshift-app-config.git"
path = "epfl/co2-calculator/overlays/{env}"
registry = "quay-its.epfl.ch/svc1751"
```

Exact key names are the builder's. The shape is: the authority remote,
ordered environments, checks, images, the chart, and deploy targets, each
with its registry. An overlay's existing `images[]` entries are updated by
name, so a target that doesn't deploy an image (enack8s has no docs image)
is left alone.

## Failures and concurrency

| What happens | What the design does |
|---|---|
| Two devs Ship at once | One `dev` push wins. The other re-merges, re-checks, retries. |
| Laptop closes after the `dev` push, before the overlay | dev's overlay runs behind `dev`. git-board shows it, and the next converge, by anyone, finishes it. |
| Two converges of dev at once | Both build the same SHA (harmless). The overlay push is a compare-and-swap, and the monotonic rule stops the older one. |
| Two clones cut `rc.2` | The second tag push is refused, so that clone takes `rc.3`. |
| Laptop dies after the tag push, before the record or the overlays | The tag stands. The next git-board session, anyone's, creates the record for its commit, and converge finishes the environments. |
| Another app's bot pushes the config repo meanwhile | The push is refused. Replay the one-file change, re-check, retry. |
| One config repo updated, the other not | Visible per repo. The next converge finishes it. |
| The authority remote is down | Read-only work goes on. Converge to an already-cut release still works, because the config repos and registries are elsewhere. Ship and new tags wait, or the team moves `[release] remote` on purpose. |
| GitHub Actions blocked (2026-09-22) | Nothing is affected. No step uses it. |
| Bad release in prod | `rollback prod` points the overlays back at the previous release's digests and chart. No tag moves. |

## Trust

- **Checks are self-declared**, attributed to who ran them. This is the
  gh-signoff model: it trusts the team, which the PR gate also did.
- **Records are attributed**, not signed, in v1.
- **A laptop build is SLSA Build L1 at most.** buildx provenance and SBOM
  are attached (`--provenance=mode=max --sbom=true`) and labelled as such.
- **Anyone with push may promote.** The rights are the ones that already
  exist: config-repo write (24 accounts on enack8s-app-config) and registry
  write. No role check is added.

## The stub's five questions, answered

1. **Is a release an issue, a config entry, or only tags?** Neither an issue
   nor a config entry. It is a git-bug entity of its own (one record per
   candidate), next to an immutable tag that names it.
2. **How does an issue show which environment it has reached?** By
   ancestry against the commit each environment runs, read from the
   overlays. It shows as a line in Development.
3. **Where does the changelog come from, and who edits it?** From the issues
   between two releases, drafted by git-board. Whoever cuts the release
   edits it, or their agent. It lives on the release record, not in a
   commit.
4. **Who opens the pre-release and release PRs?** Nobody: they're gone. A
   candidate and a release are a tag and a converge, done by whoever has
   push.
5. **What stays GitHub's?** Git hosting (`origin`, the authority while the
   team keeps it there), ghcr, and the config repos Argo reads. Actions,
   GitHub Releases and the PR gate on `dev` no longer take part.

## Dependencies outside git-board

- **Work-unit M4** (review in git-board): needed by Ship, not by converge
  or releases.
- **co2**:
  - `APP_VERSION` read at runtime (the overlay sets it), not baked in per
    ref;
  - the chart's version given at package time, not from the CI run number;
  - `deploy.yml`, `changelog.yml`, `release-please.yml` and the GitLab dev
    pipeline retired at the switch;
  - the `dev` ruleset changed as decided;
  - the docs' release runbook rewritten.
- **The deploy action**, only if it stays as the fallback: re-tag `:<sha>`
  when that image exists, instead of rebuilding on a tag.
- **Laptops**:
  - `docker buildx` and an amd64 builder, `crane`, `helm`, `trivy`;
  - `docker login` to ghcr (a token with `write:packages`) and to quay;
  - the quay robot credential leaves Actions secrets.
- **Argo**: unchanged. It keeps reading the overlays.

## Migration: co2 first

1. The co2 changes that keep CI working: runtime `APP_VERSION`, and the
   chart version given at package time.
2. git-board's converge, tags, records and adopt, tested on the scratch
   repos (below).
3. **Pilot on co2, releases first**:
   - `release adopt`;
   - switch off both CI deploy paths (GitHub Actions and GitLab), so two
     writers don't fight over the overlays;
   - cut the next release with git-board.

   Merging into `dev` still goes through the GitHub PR. The old CI paths
   stay available as the fallback, because they write the same overlays.
4. **Ship**, once M4 is built: change the `dev` ruleset and merge with git-board.
5. Freeze `stage` and `main`, make `dev` the default branch, delete them
   once nothing reads them.
6. Other apps opt in by adding `[release]`.

## Risks

- **Cross-build time.** amd64 images built under emulation on an arm64 Mac,
  three images for co2. Measure it first. A remote buildx builder (e.g.
  BuildKit on enack8s) is the fallback, and `builder` is the knob.
- **No daemon, so no schedule.** co2's scheduled image scan is an Actions
  cron today. It needs another home, or a `git-board converge --watch` host
  that also scans.
- **Credentials on laptops.** Registry write moves from Actions secrets to
  each person who deploys.
- **Two writers during the switch.** CI and git-board both writing
  overlays. The pilot switches both CI deploy paths off.
- **Discoverability.** `refs/releases/*` is invisible on forges. The static
  export is what non-git-board readers get.

## Left to the builder

- Command and key names; the Releases view's layout.
- How the source commit is written into an overlay (annotation key) and
  where `APP_VERSION` goes in it.
- The derived dev version's exact format, within SemVer and Docker's tag
  rules.
- Whether Ship's scratch worktree is reused or made per run.
- How images and charts are built: the docker, crane and helm CLIs, or their
  Go libraries.

## Verification

On the scratch forges (guilbep/git-board-test, Project #1) and a scratch
config repo with a dev/stage/prod overlay:

1. `release adopt` on the scratch overlays writes annotations, and converge
   refuses an overlay that wasn't adopted.
2. Two clones Ship two branches at the same time. One push wins, the other
   re-merges and lands, and `dev` passed checks at every step.
3. A converge is killed after the `dev` push. The other clone shows the
   drift and finishes it.
4. `v0.1.0-rc.1` is cut and fails, `rc.2` is cut and promoted to `v0.1.0`.
   The prod overlay's digests and chart equal rc.2's, and the tag points at
   rc.2's commit.
5. Rollback from `v0.1.1` to `v0.1.0`: the overlay goes back, and no tag
   moves.
6. A hotfix goes straight to prod. The next release is refused until the
   hotfix branch is merged into `dev`.
7. A second clone pulls `refs/releases/*` and shows each record with its
   author.
8. An issue's Development line reads "in dev · in stage · not in prod" at
   the right moments.

## Prior art

- **Promote one artifact**: Kargo's Freight and Stages
  (docs.kargo.io/user-guide/core-concepts), Heroku Pipelines' promote of the
  same slug.
- **Signed ref log in git, checked by clients**: gittuf
  (github.com/gittuf/gittuf, design document). Where signing would go next.
- **Reconcile, don't react**: Kubernetes controllers, Argo CD, Flux.
- **Local CI**: DHH, "We're moving continuous integration back to
  developer machines" (world.hey.com/dhh); Rails 8.1 `bin/ci`;
  basecamp/gh-signoff.
- **Trunk-based development**: release from a tag on trunk, roll forward
  (trunkbaseddevelopment.com).
- **Environment branches considered harmful**: "Stop using branches for
  deploying to different GitOps environments" (Codefresh, Octopus).
- **Tags**: git-tag(1) "On Re-tagging"; semver.org precedence; npm
  dist-tags (immutable versions, moving pointers); Go pseudo-versions
  (go.dev/ref/mod).
- **Records in git**: git-appraise (reviews and CI in git notes), Radicle
  CI and its COBs, Fossil tickets.
- **Not it**: Anthropic's Scion ("Redesigning source control",
  anthropic.com/engineering, 2026-10). Internal, not to be released, and
  built for always-online clients, the opposite of this direction.

## Constraints

- co2-calculator stays local (no sync); GitHub Project 40 is read-only.
- Live tests only on the scratch forges: guilbep/git-bug-sync-test,
  gitlab.epfl.ch project 42400, guilbep/git-board-test, Project #1.
