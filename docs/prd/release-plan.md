# Releases: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** git-board cuts, promotes, deploys and rolls back releases from
the developer's machine: one commit and one set of image digests go
through the environments, tags name candidates and releases, the config
repos' overlays say what runs where, and a git-bug entity in
`refs/releases/` holds each release's page. A Releases page and an
"in dev · in stage · in prod" line on each issue show it.

**Architecture:** A new package `internal/release` holds everything
that isn't git-bug: versions, overlay editing, git plumbing for tags and
config repos, the external tools behind one interface, and the
orchestration (`Runner`: Status, Deploy, Cut, Promote, Rollback, Adopt,
Draft). The release record is a git-bug `entity/dag` type in
`internal/gitbugio/releases.go`, beside numbers.go, pushed and pulled
explicitly like the numbers counter. The CLI (`git-board release`,
`git-board deploy`) and the server (`/api/releases`, a background job
like the forge sync's) both drive a `Runner`. The web adds a Releases
page, a release page, the cut and rollback dialogs, and a line in
Development.

**Tech Stack:** Go (BurntSushi/toml, go.yaml.in/yaml/v3 node API,
os/exec for git, docker buildx, crane, trivy, helm), git-bug fork
`entity/dag`, Vue 3 + Quasar, node:test.

**Spec:** `docs/prd/release.md`. **Designs:**
https://claude.ai/artifact/WZbwifzZcgoM69xBgJ1Ymq (Releases, a release,
Cut a release, Ship from the issue, When things go wrong).

**Not in this plan:** Ship (merge from the issue). It needs work-unit's
M4 and gets its own plan. Signing records (PRD: v1 is attributed).

## Global Constraints

- **No new Go dependency.** yaml.v3 and toml are already in go.mod.
  External tools run through `release.Tools` only: `git`, `docker
  buildx`, `crane`, `trivy`, `helm` on PATH.
- **No live registry, cluster or forge call in `make ci`.** Tests use
  `releasetest.Tools` (fake) and local bare repos.
- **The authority remote** is `cfg.Release.RemoteName()`: `[release]
  remote`, default `origin`. Tags and `refs/releases/*` are pushed there;
  a refused push is the lock.
- **Tags**: annotated, `vX.Y.Z` (final) and `vX.Y.Z-rc.N` (candidate).
  Never moved, never deleted on a remote.
- **Dev version** (derived, never stored): `X.Y.(Z+1)-dev.N.g<sha7>`,
  with X.Y.Z the newest final tag reachable and N the commits since the
  newest release tag reachable. No `+` (Docker tags refuse it).
- **Image refs**: `<registry>/<image>:<full 40-char sha>` is the build;
  `crane tag` adds the version. An image counts as built for a commit
  only when its `org.opencontainers.image.revision` label is that sha.
- **Overlay edits** (`<path>/kustomization.yaml` in each config repo):
  - an `images[]` entry matches when `name == <target registry>/<image>`;
    it gets `newTag: <version>` and `digest: <digest>`;
  - the `helmCharts[]` entry whose `repo == [release] chart.registry`
    gets `version: <version>`;
  - each `[release] version_values` dotted path, under that chart
    entry's `valuesInline`, gets the version as a string (co2:
    `backend.env.APP_VERSION`);
  - `commonAnnotations` keys `git-board/commit` (full sha) and
    `git-board/version`;
  - nothing else changes: comments, key order and other entries stay.
- **Config-repo commit message**: `chore(manifest): update <app> (<env>)
  <version> [by <login> via git-board]`, `<app>` being the path without
  its last `/overlays/<env>` part. Pushed to the config repo's default
  branch, up to 3 tries on a refused push.
- **Monotonic rule**: an environment only moves to a commit that
  descends from `git-board/commit`. Exceptions, explicit: rollback, and
  a hotfix into a non-last environment.
- **Environments**: `[release] environments` in order. `follow` (default
  `"dev"` when listed, else none) follows the trunk's head. The last
  environment takes finals. Environments between `follow` and the last
  take candidates. One environment that is `follow`: no releases.
- Config-repo clones live in `os.UserCacheDir()/git-board/config-repos/
  <first 12 hex of sha1(url)>`.
- One release job at a time in the server: another answers 409
  `{"code":"releasing"}`.
- Copy follows the designs: sentence case, no emoji, plain verbs
  ("Deploy dev", "Cut a release", "Promote to prod as v1.4.20", "Roll
  back…").
- `make ci` and the web's `npm test`, `npm run typecheck` pass after
  every task. New web tests are appended to `web/package.json` "test".

## Review Focus

1. **An overlay full of other things** (Infisical patches, a postgresql
   chart, comments, `valuesInline` env) is rewritten with only the
   intended lines changed. (Task 3: `TestSetKeepsEverythingElse`.)
2. **The config repo moved under us** (another app's bot pushed): the
   push is refused, the change is replayed on the new head, the monotonic
   check runs again, the push lands. (Task 4:
   `TestUpdateReplaysOnRejectedPush`.)
3. **Two clones cut the same rc**: the second tag push is refused, and
   nothing else is written: no record, no overlay, no registry tag.
   (Task 9: `TestCutTagTakenWritesNothing`.)
4. **A deploy stopped between two config repos** is reported as split
   and finished by the next deploy. (Task 8:
   `TestDeployFinishesSplitEnvironment`.)
5. **A `:<sha>` tag on an image built for another commit**
   (`reuse_unchanged_images` did that) is rebuilt, not trusted. (Task 8:
   `TestEnsureImageRebuildsOnWrongRevision`.)

---

### Task 1: The `[release]` configuration

**Files:**
- Create: `internal/config/release.go`
- Modify: `internal/config/config.go` (`raw`, `Config`, `parse`)
- Test: `internal/config/release_test.go`, `internal/config/check_test.go`
- Modify: `docs/configuration.md` (new `## Release` after `## Numbers`),
  `docs/prd/release.md` (config example: quay path, `follow`,
  `version_values`)

**Interfaces:**
- Produces:
  ```go
  type Release struct {
      Remote        string   `toml:"remote"`
      Environments  []string `toml:"environments"`
      Follow        *string  `toml:"follow"`
      Checks        string   `toml:"checks"`
      Images        []Image  `toml:"images"`
      Chart         *Chart   `toml:"chart"`
      VersionValues []string `toml:"version_values"`
      Builder       string   `toml:"builder"`
      Deploy        []Target `toml:"deploy"`
  }
  type Image struct{ Name, Context string }   // toml "name", "context"
  type Chart struct{ Path, Registry string }  // toml "path", "registry"
  type Target struct{ Repo, Path, Registry string } // toml "repo", "path", "registry"
  func (r Release) Enabled() bool            // len(Environments) > 0
  func (r Release) RemoteName() string       // cmp.Or(Remote, "origin")
  func (r Release) Followed() string         // *Follow if set, else "dev" when listed, else ""
  func (r Release) Candidates() []string     // environments after Followed() and before the last
  func (r Release) Last() string             // last environment, "" when none
  func checkRelease(r Release) error
  ```
  `Config.Release Release`.

- [ ] **Step 1: Write the failing tests**

```go
func TestReleaseRoles(t *testing.T) {
    r := mustLoad(t, `[release]
environments = ["dev", "stage", "prod"]`).Release
    assert(t, r.Followed() == "dev" && r.Last() == "prod")
    assert(t, slices.Equal(r.Candidates(), []string{"stage"}))
    assert(t, r.RemoteName() == "origin")
    p := mustLoad(t, `[release]
environments = ["prod"]`).Release
    assert(t, p.Followed() == "" && p.Last() == "prod" && len(p.Candidates()) == 0)
    d := mustLoad(t, `[release]
environments = ["dev"]`).Release
    assert(t, d.Followed() == "dev" && d.Last() == "dev")
}

func TestCheckReleaseRefuses(t *testing.T) {
    for _, bad := range []string{
        `environments = ["dev", "dev"]`,                                 // "listed twice"
        `environments = ["dev"]
follow = "stage"`,                                                       // "isn't in environments"
        `environments = ["dev"]
[[release.deploy]]
repo = "x"
path = "overlays/dev"`,                                                  // "path needs {env}"
        `environments = ["dev"]
images = [{ name = "", context = "." }]`,                               // "image needs a name"
    } {
        _, err := parse([]byte("[release]\n" + bad))
        if err == nil { t.Errorf("accepted %q", bad) }
    }
}
```

In check_test.go, `TestCheckTakesRelease`: a full `[release]` (the PRD's
co2 example) gives no "doesn't know this key" warning.

- [ ] **Step 2: Run them to see them fail**

Run: `go test ./internal/config/ -run 'Release'`
Expected: FAIL, `cfg.Release undefined`.

- [ ] **Step 3: Implement** the types and methods in `release.go`; add
  `Release Release \`toml:"release"\`` to `raw` and `Config`, call
  `checkRelease` in `parse` next to `checkRoles`, copy it across. Error
  texts: `release: environment %q listed twice`, `release: follow %q
  isn't in environments`, `release: deploy %s: path needs {env}`,
  `release: image needs a name`.

- [ ] **Step 4: Document** `## Release` in docs/configuration.md in the
  Branches section's format (prose, the co2 toml example, a key/default/
  what table), covering the environment roles above and the overlay
  edits from Global Constraints. In docs/prd/release.md fix the config
  example: the openshift target's registry is `quay-its.epfl.ch/svc1751`,
  and add `version_values = ["backend.env.APP_VERSION"]`.

- [ ] **Step 5: Run** `go test ./internal/config/ ./cmd/git-board-server/`
  Expected: PASS (the Settings docs split picks up the new heading).

- [ ] **Step 6: Commit**

```bash
git add internal/config docs/configuration.md docs/prd/release.md
git commit -m "feat(release): the [release] configuration"
```

---

### Task 2: Versions

**Files:**
- Create: `internal/release/version.go`, `internal/release/version_test.go`

**Interfaces:**
- Produces:
  ```go
  type Version struct{ Major, Minor, Patch, RC int } // RC 0: a final
  func ParseTag(tag string) (Version, bool)          // "v1.4.20", "v1.4.20-rc.2"; anything else false
  func (v Version) Tag() string                       // "v1.4.20-rc.2"
  func (v Version) String() string                    // "1.4.20-rc.2"
  func (v Version) Final() Version                    // RC = 0
  func (v Version) Less(w Version) bool               // SemVer precedence: 1.4.20-rc.2 < 1.4.20 < 1.4.21-rc.1
  func Bump(last Version, kinds []string, breaking bool) Version
  func NextRC(final Version, taken []Version) Version
  func DevVersion(lastFinal Version, since int, sha string) string
  ```

- [ ] **Step 1: Write the failing tests**

```go
func TestParseTag(t *testing.T) {
    v, ok := ParseTag("v1.4.20-rc.2")
    assert(t, ok && v == Version{1, 4, 20, 2} && v.Tag() == "v1.4.20-rc.2")
    for _, no := range []string{"1.4.20", "v1.4", "v1.4.20-beta.1", "v1.4.20-rc.0", "chore-tag"} {
        if _, ok := ParseTag(no); ok { t.Errorf("parsed %q", no) }
    }
}
func TestLess(t *testing.T) {
    order := []string{"v1.4.19", "v1.4.20-rc.1", "v1.4.20-rc.2", "v1.4.20", "v1.5.0-rc.1"}
    // each Less than the next, never the reverse
}
func TestBump(t *testing.T) {
    last := Version{1, 4, 19, 0}
    assert(t, Bump(last, []string{"fix", "chore"}, false) == Version{1, 4, 20, 0})
    assert(t, Bump(last, []string{"fix", "feat"}, false) == Version{1, 5, 0, 0})
    assert(t, Bump(last, []string{"fix"}, true) == Version{2, 0, 0, 0})
}
func TestNextRC(t *testing.T) {
    taken := []Version{{1, 4, 20, 1}, {1, 4, 20, 2}, {1, 4, 19, 5}}
    assert(t, NextRC(Version{1, 4, 20, 0}, taken) == Version{1, 4, 20, 3})
    assert(t, NextRC(Version{1, 5, 0, 0}, taken) == Version{1, 5, 0, 1})
}
func TestDevVersion(t *testing.T) {
    got := DevVersion(Version{1, 4, 19, 0}, 7, "8c41d2e0f9b1aa")
    assert(t, got == "1.4.20-dev.7.g8c41d2e")
}
```

- [ ] **Step 2: Run** `go test ./internal/release/ -run 'Tag|Less|Bump|RC|Dev'` — FAIL (package missing).
- [ ] **Step 3: Implement** with `regexp` `^v(\d+)\.(\d+)\.(\d+)(?:-rc\.([1-9]\d*))?$`.
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat(release): versions, candidates and the dev version`

---

### Task 3: Overlays

**Files:**
- Create: `internal/release/overlay.go`, `internal/release/overlay_test.go`
- Create: `internal/release/testdata/co2-prod.yaml` (co2's
  enack8s prod overlay as of 2026-10-18, trimmed of nothing: patches,
  postgresql chart, valuesInline, comments)

**Interfaces:**
- Produces:
  ```go
  type Deployed struct {
      Commit  string           // git-board/commit, "" when not adopted
      Version string           // git-board/version
      Images  map[string]Image // key: the images[] entry's name
      Chart   string           // the chart entry's version, "" when none
  }
  type Image struct{ Tag, Digest string }
  type Overlay struct{ /* yaml.Node document */ }
  func ParseOverlay(data []byte) (*Overlay, error)
  func (o *Overlay) Deployed(chartRepo string) Deployed
  func (o *Overlay) Set(d Deployed, chartRepo string, versionValues []string) (changed bool, err error)
  func (o *Overlay) Bytes() ([]byte, error) // yaml.v3 encoder, indent 2
  ```
  `Set` writes every key in `d.Images` (an absent name is an error
  `overlay has no image %s`), the chart version when `chartRepo != ""`
  (absent entry: `overlay has no chart %s`), each version value
  (creating missing maps on the path), and both annotations
  (`commonAnnotations` created when absent). `d.Commit == ""` leaves the
  annotations alone.

- [ ] **Step 1: Write the failing tests**

```go
func TestDeployedReadsCo2(t *testing.T) {
    o := mustParse(t, "testdata/co2-prod.yaml")
    d := o.Deployed("oci://ghcr.io/epfl-enac/co2-calculator/helm")
    assert(t, d.Commit == "" && d.Chart == "1.0.1789")
    fe := d.Images["ghcr.io/epfl-enac/epfl/co2-calculator/frontend"]
    assert(t, fe.Tag == "v1.4.19" && strings.HasPrefix(fe.Digest, "sha256:33b43e11"))
}

func TestSetKeepsEverythingElse(t *testing.T) {
    before := read(t, "testdata/co2-prod.yaml")
    o := mustParse(t, "testdata/co2-prod.yaml")
    changed, err := o.Set(Deployed{
        Commit: "5e09a71" + strings.Repeat("0", 33), Version: "v1.4.20",
        Images: map[string]Image{
            "ghcr.io/epfl-enac/epfl/co2-calculator/frontend": {Tag: "v1.4.20", Digest: "sha256:aaaa"},
            "ghcr.io/epfl-enac/epfl/co2-calculator/backend":  {Tag: "v1.4.20", Digest: "sha256:bbbb"},
        },
    }, "oci://ghcr.io/epfl-enac/co2-calculator/helm", []string{"backend.env.APP_VERSION"})
    assert(t, err == nil && changed)
    after := bytes(t, o)
    // a line diff of before/after holds exactly these changes:
    // newTag ×2, digest ×2, the co2-calculator chart's version: v1.4.20,
    // APP_VERSION: "v1.4.20", and the added commonAnnotations block (3 lines).
    // The "# db-secret-crd" comment, the postgresql chart's version 13.2.23
    // and every other line are unchanged.
    assertOnlyChanged(t, before, after, wantChanged)
}

func TestSetUnknownImage(t *testing.T) { /* Set with "ghcr.io/x/docs" → error "overlay has no image ghcr.io/x/docs" */ }
func TestSetTwiceIsNoChange(t *testing.T) { /* second identical Set returns changed == false */ }
```

- [ ] **Step 2: Run** `go test ./internal/release/ -run 'Deployed|Set'` — FAIL.
- [ ] **Step 3: Implement** on `yaml.Node`: find mapping keys by value,
  edit scalar nodes in place (keeping their style), append new nodes only
  for absent keys. Quote APP_VERSION's value (`Style: yaml.DoubleQuotedStyle`).
- [ ] **Step 4: Run** — PASS. If yaml.v3 re-indents a block it didn't touch
  (e.g. the `patch: |-` literals), keep the test strict and fix the
  encoder settings, not the expectation.
- [ ] **Step 5: Commit** `feat(release): read and write a kustomize overlay, keeping the rest`

---

### Task 4: Git plumbing: tags and config repos

**Files:**
- Create: `internal/release/git.go`, `internal/release/git_test.go`
- Create: `internal/release/releasetest/repos.go`

**Interfaces:**
- Produces:
  ```go
  func Git(ctx context.Context, dir string, args ...string) (string, error) // trimmed stdout; error carries stderr
  func IsAncestor(ctx context.Context, dir, a, b string) (bool, error)      // a == b counts
  type Tag struct {
      Name, Commit, Message string
      Version Version
      Date    time.Time
  }
  func ReleaseTags(ctx context.Context, dir string) ([]Tag, error) // ParseTag-able tags only, newest version first
  func TagsAt(tags []Tag, commit string) []Tag
  func PushTag(ctx context.Context, dir, remote string, t Tag) (taken bool, err error)
  type Change struct{ Commit, Author, Version string; Date time.Time }
  type ConfigRepo struct{ URL, Dir string }
  func OpenConfigRepo(ctx context.Context, cacheDir, url string) (*ConfigRepo, error)
  func (c *ConfigRepo) Read(ctx context.Context, path string) (*Overlay, error)
  func (c *ConfigRepo) Update(ctx context.Context, path, message string, change func(*Overlay) (bool, error)) (commit string, err error)
  func (c *ConfigRepo) History(ctx context.Context, path string, n int) ([]Change, error)
  ```
  releasetest:
  ```go
  func Git(t *testing.T, dir string, args ...string) string // GIT_CONFIG_GLOBAL=/dev/null, a fixed identity
  func NewRemote(t *testing.T) (work, bare string)          // bare repo + clone with one commit on dev
  func Commit(t *testing.T, dir, msg string) string         // empty commit, returns full sha
  func NewConfigRepo(t *testing.T, files map[string]string) (bare string) // default branch main
  ```
- Behaviour: `PushTag` creates the annotated tag locally, pushes
  `refs/tags/<name>`; a refusal because the remote has it (`! [rejected]`
  / `already exists`) deletes the local tag and returns `taken == true,
  nil`. `Update` fetches, resets its clone to `origin/<default>`, reads,
  calls `change` (false: nothing to commit, returns ""), commits as the
  user's git identity, pushes; on a non-fast-forward refusal it starts
  over, 3 tries, then `config repo %s moved 3 times while writing: try
  again`. `History` reads `git log --format` of the path, the version
  from the commit subject.

- [ ] **Step 1: Write the failing tests**

```go
func TestPushTagTaken(t *testing.T) {
    work, bare := releasetest.NewRemote(t)
    other := cloneOf(t, bare)
    c := releasetest.Commit(t, work, "a")
    releasetest.Git(t, work, "push", "origin", "HEAD:dev")
    releasetest.Git(t, other, "fetch", "origin")
    taken, err := PushTag(ctx, other, "origin", Tag{Name: "v1.0.0-rc.1", Commit: c, Message: "x"})
    assert(t, err == nil && !taken)
    taken, err = PushTag(ctx, work, "origin", Tag{Name: "v1.0.0-rc.1", Commit: c, Message: "y"})
    assert(t, err == nil && taken)
    // work has no local v1.0.0-rc.1 left
}

func TestUpdateReplaysOnRejectedPush(t *testing.T) {
    bare := releasetest.NewConfigRepo(t, map[string]string{
        "epfl/app/overlays/dev/kustomization.yaml": devOverlay,
        "epfl/other/README": "x",
    })
    c, _ := OpenConfigRepo(ctx, t.TempDir(), bare)
    calls := 0
    commit, err := c.Update(ctx, "epfl/app/overlays/dev", "msg", func(o *Overlay) (bool, error) {
        calls++
        if calls == 1 { pushOtherCommit(t, bare, "epfl/other/README") } // another app's bot
        return o.Set(newDeployed, "", nil)
    })
    assert(t, err == nil && commit != "" && calls == 2)
    // bare's main has both the other commit and ours, ours on top
}

func TestIsAncestorEqual(t *testing.T) { /* IsAncestor(a, a) == true; older vs newer as expected */ }
func TestReleaseTagsSkipsOthers(t *testing.T) { /* tags v1.0.0, v1.1.0-rc.1, "docs" → two, newest first */ }
```

- [ ] **Step 2: Run** `go test ./internal/release/ -run 'PushTag|Update|IsAncestor|ReleaseTags'` — FAIL.
- [ ] **Step 3: Implement** with `exec.CommandContext("git", "-C", dir, ...)`. The default branch is `git symbolic-ref refs/remotes/origin/HEAD` in the clone (set by clone).
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat(release): tags pushed first, config repos written with retries`

---

### Task 5: The tools behind one interface

**Files:**
- Create: `internal/release/tools.go`, `internal/release/tools_test.go`
- Create: `internal/release/releasetest/tools.go`

**Interfaces:**
- Produces:
  ```go
  var ErrNoImage = errors.New("no such image")
  type Build struct{ Context, Ref, Revision, Builder string } // platform is always linux/amd64
  type Tools interface {
      Revision(ctx context.Context, ref string) (digest, revision string, err error) // ErrNoImage when absent
      Build(ctx context.Context, b Build) (digest string, err error)
      Scan(ctx context.Context, ref string) error
      Copy(ctx context.Context, src, dst string) error
      Tag(ctx context.Context, ref, tag string) error
      PackageChart(ctx context.Context, path, version, dir string) (file string, err error)
      PushChart(ctx context.Context, file, registry string) (digest string, err error)
  }
  func Exec() Tools
  ```
  Exec's commands:
  - Revision: `crane digest <ref>`, then `crane config <ref>`, label
    `.config.Labels["org.opencontainers.image.revision"]`. "MANIFEST_UNKNOWN"
    or "NAME_UNKNOWN" → ErrNoImage.
  - Build: `docker buildx build [--builder B] --platform linux/amd64
    --label org.opencontainers.image.revision=<rev> --provenance=mode=max
    --sbom=true --push --metadata-file <tmp> -t <ref> <context>`, the
    digest from the metadata file's `containerimage.digest`.
  - Scan: `trivy image --exit-code 1 --severity HIGH,CRITICAL
    --ignore-unfixed <ref>`.
  - Copy `crane copy`, Tag `crane tag`, PackageChart `helm package <path>
    --version V --app-version V -d <dir>` (file = last word of stdout),
    PushChart `helm push <file> <registry>` (digest from its `Digest:` line).
  releasetest:
  ```go
  type Tools struct {
      Images   map[string]FakeImage // by ref, digest refs included
      Built    []release.Build
      Scanned, Copied, Tagged []string
      FailScan map[string]bool
      Charts   map[string]string    // registry+version → digest
  }
  type FakeImage struct{ Digest, Revision string }
  func NewTools() *Tools
  ```
  Fake Build stores `Images[ref]` with digest `"sha256:" + first 12 of
  sha256(ref+rev)`; Copy and Tag register the same image under the new
  ref; Revision answers from `Images`.

- [ ] **Step 1: Write the failing tests**: Exec is checked with fake
  binaries on PATH (a temp dir of shell scripts echoing fixed output),
  one test per parse: `TestExecRevisionNoImage` (crane prints
  MANIFEST_UNKNOWN, exit 1 → ErrNoImage), `TestExecBuildReadsMetadata`
  (fake docker writes `{"containerimage.digest":"sha256:abc"}` to the
  `--metadata-file` path → "sha256:abc"), `TestExecPushChartDigest`
  (helm prints `Pushed: …\nDigest: sha256:def` → "sha256:def"),
  `TestExecScanFails` (trivy exit 1 → error naming the ref).
- [ ] **Step 2: Run** `go test ./internal/release/ -run Exec` — FAIL.
- [ ] **Step 3: Implement** Exec and the fake.
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat(release): docker, crane, trivy and helm behind release.Tools`

---

### Task 6: The release record

**Files:**
- Create: `internal/gitbugio/releases.go`, `internal/gitbugio/releaseops.go`
- Test: `internal/gitbugio/releases_test.go`
- Modify: `internal/gitbugio/history.go` (`RefsFingerprint` adds the
  `releases` namespace)

**Interfaces:**
- Consumes: git-bug `entity/dag` (`Definition`, `New`, `ReadAll`,
  `Push`, `Fetch`, `MergeAll`), following `git-bug/entity/dag/example_test.go`.
- Produces:
  ```go
  const ReleasesNamespace = "releases"
  type Artifact struct{ Image, Registry, Digest string }
  type ChartArtifact struct{ Version, Digest string }
  type ReleaseComment struct{ Author, Message string; Date time.Time }
  type ReleaseRecord struct {
      ID, Commit, Previous string // Previous: the previous final's tag
      Artifacts []Artifact
      Chart     ChartArtifact
      Notes     string
      Comments  []ReleaseComment
      Yanked    string            // the reason, "" when not
      Author    string            // the creating identity's name
      Created   time.Time
  }
  func (b *Backend) Releases() ([]ReleaseRecord, error) // one per commit
  func (b *Backend) ReleaseFor(commit string) (ReleaseRecord, bool, error)
  func (b *Backend) CreateRelease(r ReleaseRecord) (ReleaseRecord, error) // the commit's existing record when it has one
  func (b *Backend) SetReleaseNotes(id, notes string) error
  func (b *Backend) CommentRelease(id, message string) error
  func (b *Backend) YankRelease(id, reason string) error
  func (b *Backend) PushReleases(remote string) error
  func (b *Backend) PullReleases(remote string) error
  ```
  Ops (releaseops.go): `create` (commit, previous, artifacts, chart),
  `set-notes`, `comment`, `yank`; typename `release`, format version 1.
  Authored by `b.c.GetUserIdentity()`. Two records for one commit
  (made on two clones): the one with the earlier create Lamport time
  counts, then the smaller id; the other is never shown.

- [ ] **Step 1: Write the failing tests**

```go
func TestReleaseRoundTrip(t *testing.T) {
    mine, theirs := gitbugiotest.NewPair(t)
    r, err := mine.CreateRelease(ReleaseRecord{Commit: "5e09a71...", Previous: "v1.4.19",
        Artifacts: []Artifact{{"frontend", "ghcr.io/x", "sha256:aa"}}})
    must(t, err)
    must(t, mine.SetReleaseNotes(r.ID, "A fix release."))
    must(t, mine.PushReleases("origin"))
    must(t, theirs.PullReleases("origin"))
    got, ok, _ := theirs.ReleaseFor("5e09a71...")
    assert(t, ok && got.Notes == "A fix release." && got.Artifacts[0].Digest == "sha256:aa")
}
func TestOneRecordPerCommit(t *testing.T) {
    // both clones create a record for the same commit, then pull each other:
    // Releases() returns one, the same on both sides
}
func TestCreateReturnsExisting(t *testing.T) { /* second CreateRelease for a commit returns the first's ID */ }
func TestYankedRecord(t *testing.T) { /* YankRelease sets Yanked to the reason */ }
```

- [ ] **Step 2: Run** `go test ./internal/gitbugio/ -run Release` — FAIL.
- [ ] **Step 3: Implement.** Push/pull use `dag.Push`/`dag.Fetch` +
  `dag.MergeAll` with the release Definition: `RepoCache.Push` doesn't
  carry a new namespace (see numbers.go for the same reason).
- [ ] **Step 4: Run** `go test ./internal/gitbugio/ ./cmd/...` — PASS.
- [ ] **Step 5: Commit** `feat(release): the release record, a git-bug entity in refs/releases`

---

### Task 7: Status: what runs where

**Files:**
- Create: `internal/release/runner.go`, `internal/release/status.go`
- Test: `internal/release/status_test.go`, `internal/release/releasetest/runner.go`

**Interfaces:**
- Consumes: Tasks 1–6.
- Produces:
  ```go
  type Step struct{ Name, Detail, State string } // State: running, done, failed
  type Runner struct {
      Cfg      config.Release
      Base     string            // the trunk branch (Branches.Base, else "dev")
      Dir      string            // the app repo's work tree
      B        *gitbugio.Backend
      Tools    Tools
      CacheDir string
      Who      string            // login for commit messages
      Step     func(Step)        // may be nil
      Offline  bool              // no fetch of the app remote or config repos
  }
  type TargetState struct {
      Repo, Path string
      Deployed   Deployed
      Since      *Change // the newest commit of the overlay, nil when unknown
  }
  type EnvState struct {
      Name    string
      Role    string        // follow, candidate, last
      Targets []TargetState
      Commit, Version string // the targets' when they agree
      Split   bool           // targets run different commits
      Adopted bool           // every target has git-board/commit
      Behind  []string       // follow only: merge subjects on the trunk not deployed
  }
  type ReleaseRow struct {
      Tag     Tag
      Names   []string                  // every release tag on its commit
      Record  *gitbugio.ReleaseRecord
      RunsIn  []string                  // environments whose commit is its commit
  }
  type Status struct{ Envs []EnvState; Releases []ReleaseRow; Head string }
  func (r *Runner) Status(ctx context.Context) (Status, error)
  func (r *Runner) Reconcile(ctx context.Context) error
  ```
  releasetest: `func NewRunner(t *testing.T, cfg string) (*release.Runner, *Tools, *Fixture)`
  building an app remote, a config remote with dev/stage/prod overlays
  for two targets, a gitbugiotest backend, and the fake tools;
  `Fixture` exposes `App`, `AppBare`, `ConfigBare []string`.
- Behaviour: Status fetches the authority remote (branches and tags),
  opens every config repo, reads each overlay. `Reconcile` creates a
  record for each release tag whose commit has none, with artifacts read
  through `Tools.Revision` of `<registry>/<image>:<tag>` and `Previous`
  the newest final before it; it never fails on a missing image (the
  artifact is left out). It pushes what it creates. It runs at the start
  of every write (Deploy, Cut, Promote, Rollback) and in `git-board
  release status`; Status itself never writes.

- [ ] **Step 1: Write the failing tests**: `TestStatusAgreeingTargets`
  (both targets on one commit → Commit set, Split false),
  `TestStatusSplit` (prod enack8s on c2, openshift on c1 → Split true,
  Commit ""), `TestStatusBehind` (dev overlay on c1, trunk two merges
  ahead → Behind has their two subjects), `TestStatusNotAdopted` (no
  annotation → Adopted false), `TestReleaseRowsRunsIn` (v1.0.0 on the
  commit prod runs → RunsIn ["prod"]; `Names` lists rc.2 and the final
  on one commit), `TestReconcileCreatesMissing` (a tag without record →
  one record after Reconcile, a second Reconcile adds none).
- [ ] **Step 2: Run** `go test ./internal/release/ -run 'Status|ReleaseRows|Reconcile'` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat(release): what runs where, read from the overlays`

---

### Task 8: Deploy (converge)

**Files:**
- Create: `internal/release/deploy.go`, `internal/release/deploy_test.go`

**Interfaces:**
- Produces:
  ```go
  var (
      ErrNotAdopted = errors.New("not adopted yet")
      ErrOlder      = errors.New("would move back")
  )
  type Target struct {
      Commit, Version string
      Rollback, Hotfix bool
  }
  func (r *Runner) Deploy(ctx context.Context, env string, t Target) error
  func (r *Runner) DeployFollowed(ctx context.Context) error // trunk head, DevVersion
  func (r *Runner) ensureImages(ctx context.Context, commit string) ([]gitbugio.Artifact, error)
  ```
- Behaviour, in order, each step reported through `r.Step`:
  1. **Images**: for each image, the first deploy target's registry is
     the build registry. `Revision(<reg>/<img>:<sha>)`: built when the
     revision is the sha. Else `Build`, `Scan` (a failure stops the
     deploy and names the image), then `Copy` to every other registry's
     `<reg>/<img>:<sha>`. Then `Tag` each with the version.
  2. **Chart** (when configured): `PackageChart` at the version,
     `PushChart`.
  3. **Overlays**, one config repo after the other, each through
     `Update`: not adopted → `ErrNotAdopted` (wrapped with env and
     repo); equal commit and version → no change; not a descendant →
     `ErrOlder`, unless `Rollback`, or `Hotfix` and env isn't the last.
     Message per Global Constraints.
- An environment whose targets are split is finished by the next
  Deploy to the commit of its newest target.

- [ ] **Step 1: Write the failing tests**

```go
func TestDeployBuildsOnceAndCopies(t *testing.T) {
    r, tools, fx := releasetest.NewRunner(t, co2LikeConfig)
    c := releasetest.Commit(t, fx.App, "feat")
    must(t, r.Deploy(ctx, "dev", Target{Commit: c, Version: "1.0.1-dev.1.g" + c[:7]}))
    assert(t, len(tools.Built) == 2)                  // frontend, backend: once each
    assert(t, len(tools.Copied) == 2)                 // to the second registry
    must(t, r.Deploy(ctx, "stage", Target{Commit: c, Version: "1.0.1-rc.1"}))
    assert(t, len(tools.Built) == 2)                  // no new build
    // both config repos' stage overlays carry the dev digests
}
func TestEnsureImageRebuildsOnWrongRevision(t *testing.T) {
    // tools.Images["<reg>/frontend:<sha>"] = {Digest: "sha256:old", Revision: "another"}
    // Deploy builds frontend
}
func TestDeployRefusesOlder(t *testing.T) {
    // dev runs c2; Deploy(dev, c1) → errors.Is(err, ErrOlder); overlays unchanged
    // Deploy(dev, c1, Rollback: true) → ok
}
func TestDeployRefusesNotAdopted(t *testing.T) { /* overlay without annotation → ErrNotAdopted, nothing built */ }
func TestDeployFinishesSplitEnvironment(t *testing.T) {
    // the second config repo's Update fails once (its bare made read-only),
    // Status shows prod Split; restore; Deploy(prod, same target) → both agree
}
func TestScanStopsDeploy(t *testing.T) { /* FailScan[ref] → error names "backend", no overlay written */ }
```

Deploy reads every target's overlay and refuses a not-adopted or older
one before it builds anything: `TestDeployRefusesNotAdopted` checks
`len(tools.Built) == 0`.

- [ ] **Step 2: Run** `go test ./internal/release/ -run 'Deploy|EnsureImage|Scan'` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat(release): deploy one commit's images to an environment, forward only`

---

### Task 9: Cut, promote, roll back, notes

**Files:**
- Create: `internal/release/cut.go`, `internal/release/draft.go`
- Test: `internal/release/cut_test.go`, `internal/release/draft_test.go`

**Interfaces:**
- Consumes: `gitbugio.Backend.ListIssues()`, `branches.Names(branch,
  is)`, `config.Branches.Kind(labels)`.
- Produces:
  ```go
  var ErrTaken = errors.New("tag taken")
  type DraftIssue struct{ Number, Title, Kind, Merge string }
  type Check struct{ OK bool; Text, Detail string }
  type Draft struct {
      From, FromVersion string   // the followed env's commit, else the trunk head
      Undeployed        []string // trunk merges after From (follow only)
      Previous          *Tag     // newest final reachable from From
      Proposed          Version  // Bump of Previous, final form
      Tag               string   // what Cut would push: NextRC(...) or the final
      Issues            []DraftIssue
      Notes             string   // markdown, grouped by kind
      Checks            []Check
  }
  func (r *Runner) Draft(ctx context.Context, final bool) (Draft, error)
  type Cut struct {
      Version Version // final form; zero: Proposed
      Final, Hotfix, Deploy bool
      Commit string   // zero: Draft.From
      Notes  string   // zero: Draft.Notes
  }
  func (r *Runner) Cut(ctx context.Context, c Cut) (Tag, error)
  func (r *Runner) Promote(ctx context.Context, rcTag string) (Tag, error)
  func (r *Runner) Rollback(ctx context.Context, env, tag, reason string) error
  func Merges(ctx context.Context, dir, from, to string) ([]Merge, error) // first-parent merges, oldest first
  type Merge struct{ Commit, Subject, Branch string }
  ```
- Rules:
  - The issues: first-parent merges on `Commit` since `Previous.Commit`;
    the branch from the subject (`Merge branch '<b>'…` or `Merge pull
    request #N from <owner>/<b>`); the issue the branch names. Kinds from
    `Branches.Kind`; `breaking` when an issue has the label `breaking`.
  - Checks, in this order and wording (designs, Cut a release):
    "It contains what runs in prod, <tag> (<sha7>)", "Images for <sha7>
    exist on <registries>: <images>", "<env> moves forward".
  - Cut refuses (error, nothing written): a commit that doesn't descend
    from the last environment's commit (`it doesn't contain what runs in
    <env> (<version>): merge <hotfix branch> into <base> first` when that
    commit is on a `hotfix/` branch), unless `Hotfix`; `Final` when the
    app has candidates and `Commit` carries no candidate tag that ran in
    the environment before last (overlay history), unless `Hotfix`; a
    yanked record on that commit.
  - Order: push the tag (taken → `ErrTaken`, nothing else done), `Tag`
    images with the tag's version, chart, `CreateRelease` +
    `SetReleaseNotes` + `PushReleases`, then (`Deploy`) `Deploy` to the
    first candidate environment, or to the last for a final.
  - Promote: the rc's commit gets the final tag (`Cut{Final: true,
    Commit: rc commit, Version: rc.Final()}`), same digests, deploy to
    the last environment.
  - Rollback: `Deploy(env, Target{Commit: tag's commit, Version: tag
    version, Rollback: true})`, then a comment on the rolled-back
    release's record: `Rolled back from <env> to <tag>: <reason>`.

- [ ] **Step 1: Write the failing tests**

```go
func TestDraftProposesPatchAndRC(t *testing.T) {
    // v1.0.0 runs in prod; dev runs a commit with fix/12-a and chore/13-b merged
    d := draft(t, false)
    assert(t, d.Proposed == Version{1, 0, 1, 0} && d.Tag == "v1.0.1-rc.1")
    assert(t, len(d.Issues) == 2 && strings.Contains(d.Notes, "#12"))
}
func TestDraftFeatIsMinor(t *testing.T) { /* a feat/14-c merge → 1.1.0 */ }
func TestCutTagTakenWritesNothing(t *testing.T) {
    // another clone pushed v1.0.1-rc.1 first
    _, err := r.Cut(ctx, Cut{Deploy: true})
    assert(t, errors.Is(err, ErrTaken))
    assert(t, len(tools.Tagged) == 0)            // no registry tag
    recs, _ := r.B.Releases(); assert(t, len(recs) == 0)
    // stage overlay unchanged
}
func TestCutAndPromoteSameDigests(t *testing.T) {
    rc := cut(t, Cut{Deploy: true})               // v1.0.1-rc.1 → stage
    final, _ := r.Promote(ctx, rc.Name)           // v1.0.1 → prod
    assert(t, final.Commit == rc.Commit)
    // prod's overlay digests == stage's; tools.Built unchanged by Promote
}
func TestFinalNeedsACandidate(t *testing.T) { /* Cut{Final: true} on a commit never in stage → error */ }
func TestHotfixRules(t *testing.T) {
    // prod on v1.0.1 (P); hotfix/1.0.2 branch from P with one commit H:
    // Cut{Final: true, Hotfix: true, Commit: H} → v1.0.2 in prod
    // then Cut{} from dev (doesn't contain H) → error mentions "merge hotfix/1.0.2 into dev"
}
func TestRollbackComments(t *testing.T) { /* prod back to v1.0.0; v1.0.1's record has the comment */ }
func TestMergesParsesBothSubjects(t *testing.T) { /* "Merge branch 'fix/1-a' into dev", "Merge pull request #9 from org/feat/2-b" */ }
```

- [ ] **Step 2: Run** `go test ./internal/release/ -run 'Draft|Cut|Promote|Final|Hotfix|Rollback|Merges'` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat(release): cut, promote and roll back, notes drafted from the issues`

---

### Task 10: Adopt

**Files:**
- Create: `internal/release/adopt.go`, `internal/release/adopt_test.go`

**Interfaces:**
- Produces:
  ```go
  type Proposal struct {
      Env, Repo, Path string
      Runs   string // newTag, plus the first image's digest prefix when newTag isn't a release tag
      Commit string // "" when unknown
      From   string // "tag", "label", ""
  }
  func (r *Runner) Proposals(ctx context.Context) ([]Proposal, error) // adopted overlays left out
  func (r *Runner) Adopt(ctx context.Context, ps []Proposal) error    // every proposal needs a Commit that exists in Dir
  ```
  The commit: a release tag in `newTag` → its commit; else
  `Tools.Revision(<name>@<digest>)` of the first image → the label. Adopt
  writes only the two annotations (`Set` with `Images` empty), commit
  message `chore(manifest): adopt <app> (<env>) [by <login> via git-board]`.

- [ ] **Step 1: Write the failing tests**: `TestProposalsFromTagAndLabel`
  (prod newTag v1.0.0 → From "tag"; dev newTag "dev" with a labelled
  image → From "label"; an unlabelled one → Commit ""),
  `TestAdoptWritesOnlyAnnotations` (overlay line diff: the
  commonAnnotations block only), `TestAdoptRefusesUnknownCommit`.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat(release): adopt what runs today`

---

### Task 11: The command line

**Files:**
- Create: `cmd/git-board/release.go`, `cmd/git-board/deploy.go`
- Test: `cmd/git-board/release_test.go`
- Modify: `cmd/git-board/cmd.go` (`root.AddCommand`)

**Interfaces:**
- Consumes: `release.Runner` and all its methods.
- Produces:
  ```go
  var releaseTools = release.Exec            // tests swap in releasetest.NewTools()
  func newRunner() (*release.Runner, func(), error) // loads config, opens gitbugio, Who from the user identity
  func newReleaseCmd() *cobra.Command        // status (default), cut, promote, rollback, adopt, notes, yank
  func newDeployCmd() *cobra.Command         // deploy <env> [--release <tag>] [--watch]
  ```
  Flags: `cut --version 1.4.20 --final --hotfix --commit <sha>
  --no-deploy --notes-file <path>`; `rollback <env> <tag> --reason "…"`
  (required); `adopt` prints proposals, `adopt --yes` writes them, `--set
  <env>/<n>=<sha>` fills an unknown one (n = the target's index in
  `[release] deploy`, from 1); `notes [--all]` prints markdown, newest
  first (CHANGELOG form); `yank <tag> --reason`; `deploy dev --watch`
  fetches every 60 s and deploys when behind, until interrupted.
  Status output, one block per environment:
  ```
  dev     1.4.20-dev.7.g8c41d2e  8c41d2e  2 merges behind dev
  stage   v1.4.20-rc.2           5e09a71
  prod    v1.4.19                d5d5aea  split: openshift on v1.4.18
  ```

- [ ] **Step 1: Write the failing tests** with `execute(t, args)` and
  `releaseTools` swapped: `TestReleaseStatusPrints` (exact block above
  shape on the fixture), `TestDeployDev` (prints each step, ends `dev
  runs <version>`), `TestCutNoDeploy` (prints the tag; stage unchanged),
  `TestRollbackNeedsReason` (error `--reason is required`),
  `TestAdoptNeedsYes` (prints proposals, writes nothing).
- [ ] **Step 2: Run** `go test ./cmd/git-board/ -run 'Release|Deploy|Cut|Rollback|Adopt'` — FAIL.
- [ ] **Step 3: Implement.** Steps print as `  <name>  <detail>` lines.
- [ ] **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat(release): git-board release and git-board deploy`

---

### Task 12: The server

**Files:**
- Create: `cmd/git-board-server/releases.go`, `cmd/git-board-server/releases_test.go`
- Modify: `cmd/git-board-server/server.go` (routes, `apiServer` fields),
  `cmd/git-board-server/events.go` (nothing if `RefsFingerprint` covers it)

**Interfaces:**
- Produces, routes:
  ```
  GET  /api/releases                    → releasesResponse
  GET  /api/releases/draft?final=1      → release.Draft
  POST /api/releases                    → 202 releaseJob   {version, final, hotfix, commit, notes, deploy}
  GET  /api/releases/job                → releaseJob
  GET  /api/releases/adopt              → []release.Proposal
  POST /api/releases/adopt              → 200              {proposals}
  GET  /api/releases/{tag}              → releaseDetail
  POST /api/releases/{tag}/promote      → 202 releaseJob
  PUT  /api/releases/{tag}/notes        → 200              {notes}
  POST /api/releases/{tag}/comments     → 200              {message}
  POST /api/releases/{tag}/yank         → 200              {reason}
  POST /api/environments/{env}/deploy   → 202 releaseJob   {tag?}
  POST /api/environments/{env}/rollback → 202 releaseJob   {tag, reason}
  GET  /api/issues/{id}/environments    → []issueEnvironment
  ```
  ```go
  type releasesResponse struct {
      Enabled      bool               `json:"enabled"`
      Environments []release.EnvState `json:"environments"`
      Releases     []release.ReleaseRow `json:"releases"`
      Job          *releaseJob        `json:"job,omitempty"`
  }
  type releaseDetail struct {
      Row        release.ReleaseRow   `json:"row"`
      Issues     []release.DraftIssue `json:"issues"`
      Candidates []release.ReleaseRow `json:"candidates"` // same final, rc rows
      History    map[string][]release.Change `json:"history"` // env → overlay changes naming this version
  }
  type releaseJob struct {
      Kind, Subject, State, Error string // kind: deploy, cut, promote, rollback; state: running, done, failed
      Code    string // a failed job's: taken, older, not-adopted; "" otherwise
      Steps   []release.Step
      Started time.Time
      Ended   *time.Time
  }
  type issueEnvironment struct{ Name, Version string; In bool }
  func (s *apiServer) runner() (*release.Runner, error)
  ```
  `apiServer` gains `releaseTools release.Tools` (nil: `release.Exec()`),
  `releaseCache string` (tests: a temp dir), `releaseMu sync.Mutex`,
  `releaseRun *releaseJob`. JSON field names are lower camel case (`json:`
  tags on every exported field the web reads, release types included).
- Behaviour: a job runs in a goroutine, appends steps under
  `releaseMu`, publishes `event{Kind: "releases"}` on each step and at the
  end; a second job while one runs → 409 `releasing`. Without `[release]`,
  `GET /api/releases` answers `{"enabled": false}` and every write 404s
  with `no [release] in .git-board.toml`. A job that fails on `ErrTaken`,
  `ErrOlder` or `ErrNotAdopted` gets `Code` `taken`, `older` or
  `not-adopted`. An issue's environments: the issue's merged branch
  commits (as `/branches` finds them) against each environment's commit
  (`IsAncestor`); not merged → every `In` false.

- [ ] **Step 1: Write the failing tests**: `TestReleasesDisabled`,
  `TestReleasesStatus` (fixture → three environments, rows),
  `TestDeployJobRunsAndPublishes` (POST deploy → 202, poll job until
  done, steps non-empty, an event "releases" was published),
  `TestSecondJobConflicts` (a job held running → 409 code releasing),
  `TestCutTakenIsCoded` (tag pushed meanwhile → job failed with `Code`
  "taken"),
  `TestIssueEnvironments` (an issue whose branch was merged before the
  commit dev runs → dev In true, stage false).
- [ ] **Step 2: Run** `go test ./cmd/git-board-server/ -run 'Releases|DeployJob|SecondJob|CutTaken|IssueEnvironments'` — FAIL.
- [ ] **Step 3: Implement.**
- [ ] **Step 4: Run** `make ci` — PASS.
- [ ] **Step 5: Commit** `feat(release): the server's releases, jobs and an issue's environments`

---

### Task 13: The Releases page

**Files:**
- Modify: `web/src/api/board.ts` (types and calls for every route of Task 12)
- Create: `web/src/releases.ts`, `web/src/releases.test.ts`
- Create: `web/src/pages/ReleasesPage.vue`, `web/src/components/ReleaseJob.vue`
- Modify: `web/src/router/routes.ts` (`/releases`), `web/src/components/AppHeader.vue`
  (a `Releases` router-link before Inbox), `web/src/css/gitboard.css`
  (the release block), `web/package.json` ("test" gains `src/releases.test.ts`)

**Interfaces:**
- Consumes: Task 12's JSON.
- Produces (releases.ts):
  ```ts
  export type EnvMark = 'prod' | 'stage' | 'dev' | 'off';
  export function envMark(role: 'follow' | 'candidate' | 'last', here: boolean): EnvMark
  export interface VersionGroup { version: string; final?: ReleaseRow; candidates: ReleaseRow[] }
  export function groupReleases(rows: ReleaseRow[]): VersionGroup[] // newest first; candidates under their final's version
  export function behindText(env: EnvState): string                 // "Runs 2 merges behind the dev branch"
  export function splitText(env: EnvState): string                  // "prod runs two releases. enack8s is on v1.4.20, openshift still on v1.4.19."
  export function stepState(job: ReleaseJob | undefined, i: number): 'done' | 'now' | 'todo' | 'failed'
  ```
- The page follows the design's Releases board: eyebrow (repo name),
  title row with "Cut a release" (opens Task 14's dialog), the
  environment strip (`gb-envs`: per environment its mark, URL-less in v1,
  version, sha7, "deployed <when> by <who>" from `Since`, targets with
  ok marks, the behind or split alert, and the action: Deploy dev /
  Promote to prod as vX / Roll back…), then "All releases" grouped by
  `groupReleases`, yanked rows muted, rows before git-board (no record
  author) marked `before git-board`. A running job shows `ReleaseJob`
  (the `gb-plan` list) above the strip; the page reloads on the
  `releases` event. `enabled: false` shows the `## Release` snippet from
  docs and a link to Settings.
- CSS: copy the "releases" block of the canvas stylesheet (`gb-envs` …
  `gb-ship`) into gitboard.css, mapping `--epfl-black` → `--bg-dark`,
  `--epfl-canard` → `--tertiary`, `--epfl-gray-100` → the gray the file
  already uses for `.gb-skel`; dark.css needs nothing new when only
  these variables are used.

- [ ] **Step 1: Write the failing tests** (node:test):

```ts
test('groupReleases puts candidates under their version', () => {
  const g = groupReleases([row('v1.4.20-rc.2'), row('v1.4.20-rc.1'), row('v1.4.19')]);
  assert.deepEqual(g.map((x) => x.version), ['1.4.20', '1.4.19']);
  assert.equal(g[0].final, undefined);
  assert.deepEqual(g[0].candidates.map((c) => c.tag.name), ['v1.4.20-rc.2', 'v1.4.20-rc.1']);
});
test('behindText', () => {
  assert.equal(behindText(env({ name: 'dev', behind: ['a', 'b'] })), 'Runs 2 merges behind the dev branch');
});
test('splitText names each target', () => { /* exact sentence above */ });
test('stepState', () => { /* done before the running step, now on it, todo after, failed on a failed job's last */ });
```

- [ ] **Step 2: Run** `cd web && node --test src/releases.test.ts` — FAIL.
- [ ] **Step 3: Implement** releases.ts, the API calls, the page, the nav link, the route, the CSS.
- [ ] **Step 4: Run** `cd web && npm test && npm run typecheck && npm run build` — PASS.
- [ ] **Step 5: Commit** `feat(release): the Releases page`

---

### Task 14: A release, cutting, rolling back, adopting

**Files:**
- Create: `web/src/pages/ReleasePage.vue`, `web/src/components/CutReleaseDialog.vue`,
  `web/src/components/RollbackDialog.vue`, `web/src/components/AdoptPanel.vue`
- Modify: `web/src/router/routes.ts` (`/releases/:tag`), `web/src/pages/ReleasesPage.vue`
  (open the dialogs; AdoptPanel when an environment isn't adopted)
- Test: `web/src/releases.test.ts` (adds `cutLabel`, `checkIcon`)

**Interfaces:**
- Produces (releases.ts): `export function cutLabel(d: Draft, deploy: boolean, env: string): string`
  ("Cut v1.4.20-rc.3 and deploy to stage" / "Cut v1.4.20-rc.3").
- ReleasePage follows the design's release board: breadcrumb, title +
  badge (Candidate · in stage / Release · in prod / Yanked), Yank… and
  Promote to prod as vX (candidates of the last-but-one environment
  only), meta line, Notes (rendered markdown via the existing
  `MarkdownBody`, Edit notes → `MarkdownEditor`, saves with PUT notes),
  Issues it carries, Candidates for vX, Conversation (comments + a
  composer), side column: Runs in (per environment and target, with
  History's who/when), Images, Built, Record.
- CutReleaseDialog follows the Cut a release board: From (the followed
  environment's commit; the undeployed head disabled with "Deploy dev
  first to release them."), Version input + Goes to (candidate/final
  segmented), the hint line, Before it is cut (Draft.Checks), Notes
  textarea (Draft.Notes), What happens (four `gb-plan` items), foot
  with the "anyone finishes it" note and `cutLabel`. Submitting posts
  and closes into the page's ReleaseJob. A job failed with `code`
  `taken` shows the design's "The version was taken" alert with "Cut as
  rc.N+1".
- RollbackDialog: target release select (finals older than what runs),
  the two checks of the design, required reason input.
- AdoptPanel: the design's adopt table; a missing commit is an input;
  "Adopt these" disabled until every row has one.

- [ ] **Step 1: Write the failing test** for `cutLabel` (both forms).
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement** the page and components.
- [ ] **Step 4: Run** `cd web && npm test && npm run typecheck && npm run build` — PASS.
- [ ] **Step 5: Commit** `feat(release): a release's page, cutting, rolling back and adopting`

---

### Task 15: Where an issue has got to

**Files:**
- Modify: `web/src/components/IssueBranches.vue` (an Environments line
  under the branches), `web/src/releases.ts` (+ test)

**Interfaces:**
- Produces: `export function envLine(envs: IssueEnvironment[]): { text: string; mark: EnvMark }[]`
  → `in dev`, `in stage · v1.4.20-rc.2`, `not in prod`.
- Behaviour: fetched with `getIssueEnvironments(id)` only when
  `/api/releases` is enabled and the issue has a branch; hidden
  otherwise. Marks as in the design's "on its way to prod" card; the
  stage mark links to its release page.

- [ ] **Step 1: Write the failing test** for `envLine` (in, in with version, not in).
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** web checks — PASS.
- [ ] **Step 5: Commit** `feat(release): an issue's Development says which environments have it`

---

### Task 16: Releases in the static export

**Files:**
- Modify: `cmd/git-board/html.go`, `cmd/git-board/render.go`
- Test: `cmd/git-board/html_test.go`

**Interfaces:**
- Consumes: `release.Runner.Status` (no fetch: `Status` takes the local
  state when the export runs offline; add `Runner.Offline bool`, which
  skips the fetch of the app remote and the config repos).
- Produces: `releases.html` beside the export's pages: the environment
  strip and the releases table, each release's notes inline, as
  static HTML with the export's stylesheet.

- [ ] **Step 1: Write the failing test** `TestHTMLWritesReleases`: with
  `[release]` and a fixture, the export has `releases.html` containing
  `v1.0.1-rc.1`, `stage` and the notes' first line; without `[release]`,
  no such file.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** `make ci` — PASS.
- [ ] **Step 5: Commit** `feat(release): the releases page in the static export`

---

### Task 17: Docs and the live test

**Files:**
- Modify: `README.md` (Usage: `git-board release …`, `git-board deploy
  dev [--watch]`), `docs/prd/release.md` (a "Built" note with dates and
  what the live test showed)

- [ ] **Step 1: Write the README lines** in the Usage list's style.
- [ ] **Step 2: Live test, asking the user first** (it pushes images and
  tags): guilbep/git-board-test as the app (a two-image Dockerfile
  set), a scratch config repo with dev/stage/prod overlays as the
  config repo, `ghcr.io/guilbep/git-board-test/<image>` as registry.
  Run, and write down what each shows:
  1. `git-board release adopt --yes` writes the annotations.
  2. Two clones `git-board deploy dev` at once: one deploys, the other
     reports dev already runs it (no second build).
  3. `cut` → rc.1, `cut` again from a fixed commit → rc.2, `promote
     v0.1.0-rc.2`: the prod overlay's digests equal rc.2's, and the tag
     `v0.1.0` points at rc.2's commit.
  4. `rollback prod v0.0.x --reason test`: the overlay goes back, no tag
     moves.
  5. A hotfix released to prod, then `cut` from dev refused until the
     hotfix branch is merged.
  6. A second clone pulls `refs/releases/*` and shows each record with
     its author.
  7. The issue page's line reads "in dev · in stage · not in prod" at
     the right moments.
- [ ] **Step 3: Run** `make ci` and the web checks — PASS.
- [ ] **Step 4: Commit** `docs(release): usage, and what the live test showed`

---

### Task 18: Single-environment apps tag what runs

Settled on 54d0f401 (GB-17): an app whose followed environment is its
last (fonts: `environments = ["prod"]`, `follow = "prod"`) may cut.

**Files:**
- Modify: `internal/release/draft.go` (`devOnly` leaves Draft),
  `internal/release/cut.go` (Cut no longer refuses; Promote does)
- Test: `internal/release/cut_test.go` (replaces `TestDevOnlyNeverTags`)

**Behaviour:**
- Draft and Cut take what the followed environment runs, as a final
  (no candidate environments), and deploy nowhere: that environment
  already runs the commit, and a deploy would only roll its pods.
- Promote still refuses, worded for any position of `follow` (T9):
  `prod follows main and is the last environment: nothing to promote;
  release cut tags what it runs`.
- Rollback can target such a tag. The next `deploy prod` (or `--watch`)
  moves the environment forward again, as follow means.

- [ ] **Step 1: Write the failing tests** `TestSingleEnvironmentCutsWhatRuns`
  (tag on the running commit, a record, no overlay change),
  `TestSingleEnvironmentPromoteRefused` (the message),
  `TestSingleEnvironmentRollback`.
- [ ] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** — PASS.
- [ ] **Step 5: Commit** `feat(release): a single-environment app tags what it runs`

---

### Task 19: An offline cut, finished online

Settled on 54d0f401 (GB-17): `git-board release cut --offline
--no-deploy` works without a network; the next online command finishes it.

**Files:**
- Modify: `internal/release/cut.go`, `internal/release/runner.go`
  (Reconcile), `internal/release/git.go`, `cmd/git-board/release.go`
  (`--offline`)
- Test: `internal/release/cut_test.go`, `cmd/git-board/release_test.go`

**Behaviour:**
- `--offline` sets `Runner.Offline`. Offline, Cut refuses a deploy (it
  needs `--no-deploy`), notes (drafted once the cut is pushed, editable on
  the record after) and `--hotfix` (the pending ref doesn't carry it, so
  the online check would refuse it). A failed fetch's message names
  `--offline`; nothing guesses offline from a failed fetch (auth fails the
  same way).
- Offline, Cut runs the checks the local state allows (the config repos'
  caches, local tags and records), then writes a pending cut: the ref
  `refs/git-board/cuts/<tag>` on the commit. Nothing else. One pending cut
  at a time: a second offline cut is refused, naming it. `release status`
  lists it.
- A pending cut is not under `refs/tags`, so `git fetch --tags` never
  clashes with it, and Status and Reconcile never record it.
- Online, Reconcile fetches, then checks each pending cut again on what
  it fetched (the same checks a cut runs), then pushes it. `PushTag`
  makes the annotated tag then, so its date (Reconcile's grace clock)
  starts at the push. Taken, or no longer allowed: the ref is dropped and
  the step says `dropped v…, cut offline: <why>; cut again`, with nothing
  else written. Another push failure keeps the ref and returns the error.
  Pushed: the ref is dropped, and the second half of a cut runs: images,
  chart, record with the notes drafted then. That half is factored out
  of `cut()`, not copied.
- An online cut that pushed a pending cut stops there: it was that cut,
  deployed nowhere. Promote, rollback and deploy push it on their way.

- [x] **Step 1: Write the failing tests** `TestOfflineCutWritesOnlyPending`,
  `TestOnlineFinishesOfflineCut` (tag dated at the push, images, record),
  `TestOfflineCutTakenDropsIt` (another clone pushed the name: no tag,
  no record, the fetch succeeds), `TestOfflineCutCheckedAgainOnline`
  (prod took a hotfix meanwhile), the refusals in
  `TestOfflineRefusesWrites`, and the CLI's `TestCutOffline`.
- [x] **Step 2: Run** — FAIL. **Step 3: Implement.** **Step 4: Run** — PASS.
- [x] **Step 5: Commit** `feat(release): cut offline, finished by the next online command`

---

### Task 20: The whole-branch review

- [ ] **Step 1:** One review on the most capable model over every
  `(release)` commit (`git log --grep='(release)'`, from ba62d6c, the new
  ones too), pointed at the deferred minors T1–T14 in GB-17.
- [ ] **Step 2:** One round of fixes, each with its test.
- [ ] **Step 3:** One re-review of the fixes. What is left is adjudicated:
  each of T1–T14 is fixed, or stays deferred with its reason in
  `docs/prd/release.md`'s "Built" note.
- [ ] **Step 4: Commit** each fix as `fix(release): …`.
