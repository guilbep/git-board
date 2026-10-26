package release_test

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/release"
	"github.com/guilbep/git-board/internal/release/releasetest"
	"github.com/guilbep/git-board/internal/store"
)

// adopt makes env's overlays run the app's first commit and returns it.
func adopt(t *testing.T, fx *releasetest.Fixture, env string) string {
	t.Helper()
	first := releasetest.Git(t, fx.App, "rev-list", "--max-parents=0", "HEAD")
	both(t, fx, env, release.Deployed{Commit: first, Version: "v0.1.0"})
	return first
}

func deploy(t *testing.T, r *release.Runner, env string, tg release.Target) {
	t.Helper()
	if err := r.Deploy(ctx, env, tg); err != nil {
		t.Fatalf("Deploy %s: %v", env, err)
	}
}

// heads is each config repo's main.
func heads(t *testing.T, fx *releasetest.Fixture) []string {
	t.Helper()
	var hs []string
	for _, b := range fx.ConfigBare {
		hs = append(hs, releasetest.Git(t, b, "rev-parse", "main"))
	}
	return hs
}

// dropImage removes the images[] entry name from target's overlay of env,
// as the fixture wrote it.
func dropImage(t *testing.T, fx *releasetest.Fixture, target int, env, name string) {
	t.Helper()
	dir := cloneOf(t, fx.ConfigBare[target])
	file := filepath.Join(dir, "app/overlays", env, "kustomization.yaml")
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	entry := "  - name: " + name + "\n    newTag: latest\n"
	if !strings.Contains(string(data), entry) {
		t.Fatalf("%s has no %s", file, name)
	}
	if err := os.WriteFile(file, []byte(strings.Replace(string(data), entry, "", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	releasetest.Git(t, dir, "commit", "-qam", "drop "+name)
	releasetest.Git(t, dir, "push", "-q", "origin", "HEAD:main")
}

func TestDeployBuildsOnceAndCopies(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	adopt(t, fx, "dev")
	adopt(t, fx, "stage")
	c := fx.Commit(t, "feat")
	var steps []release.Step
	r.Step = func(s release.Step) { steps = append(steps, s) }
	dev := "1.0.1-dev.1.g" + c[:7]
	deploy(t, r, "dev", release.Target{Commit: c, Version: dev})

	if len(tools.Built) != 2 {
		t.Fatalf("built %+v, want frontend and backend once each", tools.Built)
	}
	b := tools.Built[0]
	if b.Ref != "ghcr.test/app/frontend:scan-"+c || b.Revision != c || filepath.Base(b.Context) != "frontend" || strings.HasPrefix(b.Context, fx.App) {
		t.Errorf("build = %+v, want ghcr.test/app/frontend:scan-<sha> from a checkout of the commit", b)
	}
	if got := releasetest.Git(t, fx.App, "worktree", "list"); strings.Count(got, "\n") != 0 {
		t.Errorf("the build checkout is left behind: %s", got)
	}
	fe, be := tools.Images["ghcr.test/app/frontend:"+c].Digest, tools.Images["ghcr.test/app/backend:"+c].Digest
	wantCopied := []string{"ghcr.test/app/frontend@" + fe + " → quay.test/app/frontend:" + c, "ghcr.test/app/backend@" + be + " → quay.test/app/backend:" + c}
	if !slices.Equal(tools.Copied, wantCopied) {
		t.Errorf("copied %q, want %q", tools.Copied, wantCopied)
	}
	if len(tools.Tagged) != 6 || !slices.Contains(tools.Tagged, "ghcr.test/app/frontend:scan-"+c+" → "+c) ||
		!slices.Contains(tools.Tagged, "quay.test/app/backend@"+be+" → "+dev) {
		t.Errorf("tagged %q, want each scanned build tagged <sha>, each image on each registry tagged %s", tools.Tagged, dev)
	}
	if tools.Charts["oci://ghcr.test/app/helm "+dev] == "" {
		t.Errorf("charts %v, want the chart pushed at %s", tools.Charts, dev)
	}
	var names []string
	for _, s := range steps {
		names = append(names, s.Name+" "+s.State)
	}
	wantSteps := []string{"frontend running", "frontend done", "backend running", "backend done", "chart running", "chart done",
		"dev on " + fx.ConfigBare[0] + " running", "dev on " + fx.ConfigBare[0] + " done",
		"dev on " + fx.ConfigBare[1] + " running", "dev on " + fx.ConfigBare[1] + " done"}
	if !slices.Equal(names, wantSteps) || !strings.HasPrefix(steps[1].Detail, "built sha256:") {
		t.Errorf("steps = %+v", steps)
	}

	e := envOf(t, status(t, r), "dev")
	if e.Commit != c || e.Version != dev || e.Split {
		t.Errorf("dev = %+v", e)
	}
	for i, reg := range []string{"ghcr.test/app", "quay.test/app"} {
		d := e.Targets[i].Deployed
		for _, img := range []string{"frontend", "backend"} {
			want := release.Image{Tag: dev, Digest: tools.Images["ghcr.test/app/"+img+":"+c].Digest}
			if d.Images[reg+"/"+img] != want {
				t.Errorf("dev on %s: %s = %+v, want %+v", reg, img, d.Images[reg+"/"+img], want)
			}
		}
		if d.Chart != dev {
			t.Errorf("dev on %s: chart %s", reg, d.Chart)
		}
		msg := "chore(manifest): update app (dev) " + dev + " [by dev via git-board]"
		if got := releasetest.Git(t, fx.ConfigBare[i], "log", "-1", "--format=%s", "main"); got != msg {
			t.Errorf("config commit %q, want %q", got, msg)
		}
	}
	if raw := releasetest.Git(t, fx.ConfigBare[0], "show", "main:app/overlays/dev/kustomization.yaml"); !strings.Contains(raw, `APP_VERSION: "`+dev+`"`) {
		t.Errorf("APP_VERSION not written:\n%s", raw)
	}

	deploy(t, r, "stage", release.Target{Commit: c[:7], Version: "v1.0.1-rc.1"}) // a short sha is resolved
	if len(tools.Built) != 2 || len(tools.Copied) != 2 {
		t.Errorf("stage built %d, copied %d; want the dev images reused", len(tools.Built), len(tools.Copied))
	}
	st := status(t, r)
	devImgs, stage := envOf(t, st, "dev"), envOf(t, st, "stage")
	if stage.Commit != c {
		t.Errorf("stage commit %q, want the full sha %s", stage.Commit, c)
	}
	for i := range stage.Targets {
		for name, img := range stage.Targets[i].Deployed.Images {
			if img.Digest != devImgs.Targets[i].Deployed.Images[name].Digest || img.Tag != "v1.0.1-rc.1" {
				t.Errorf("stage %s = %+v, want dev's digest", name, img)
			}
		}
	}

	before := heads(t, fx)
	deploy(t, r, "stage", release.Target{Commit: c, Version: "v1.0.1-rc.1"})
	if after := heads(t, fx); !slices.Equal(after, before) {
		t.Errorf("deploying what runs wrote to the config repos")
	}
}

func TestEnsureImageRebuildsOnWrongRevision(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	adopt(t, fx, "dev")
	c := fx.Commit(t, "feat")
	// a :<sha> tag left on an older build (reuse_unchanged_images)
	tools.Images["ghcr.test/app/frontend:"+c] = releasetest.FakeImage{Digest: "sha256:old", Revision: "another"}
	// built for c, but its copy to quay never happened
	be := releasetest.FakeImage{Digest: "sha256:be", Revision: c}
	tools.Images["ghcr.test/app/backend:"+c], tools.Images["ghcr.test/app/backend@sha256:be"] = be, be
	deploy(t, r, "dev", release.Target{Commit: c, Version: "1.0.1-dev.1.g" + c[:7]})
	if len(tools.Built) != 1 || tools.Built[0].Ref != "ghcr.test/app/frontend:scan-"+c {
		t.Errorf("built %+v, want frontend only", tools.Built)
	}
	if len(tools.Copied) != 2 {
		t.Errorf("copied %q, want frontend and backend to quay", tools.Copied)
	}
	e := envOf(t, status(t, r), "dev")
	for i, reg := range []string{"ghcr.test/app", "quay.test/app"} {
		imgs := e.Targets[i].Deployed.Images
		if imgs[reg+"/frontend"].Digest == "sha256:old" || imgs[reg+"/backend"].Digest != "sha256:be" {
			t.Errorf("dev on %s = %+v", reg, imgs)
		}
	}
}

func TestDeployRefusesOlder(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	c1 := fx.Commit(t, "one")
	c2 := fx.Commit(t, "two")
	both(t, fx, "dev", release.Deployed{Commit: c2, Version: "v1.0.2"})
	before := heads(t, fx)
	err := r.Deploy(ctx, "dev", release.Target{Commit: c1, Version: "v1.0.1"})
	if !errors.Is(err, release.ErrOlder) || !strings.Contains(err.Error(), "dev on "+fx.ConfigBare[0]) {
		t.Fatalf("err = %v, want ErrOlder naming dev and the config repo", err)
	}
	if len(tools.Built) != 0 || !slices.Equal(heads(t, fx), before) {
		t.Errorf("a refused deploy built %d images or wrote an overlay", len(tools.Built))
	}
	deploy(t, r, "dev", release.Target{Commit: c1, Version: "v1.0.1", Rollback: true})
	if e := envOf(t, status(t, r), "dev"); e.Commit != c1 || e.Version != "v1.0.1" {
		t.Errorf("after rollback dev = %+v", e)
	}
}

// A hotfix may leave the line an environment runs, except the last one's.
func TestDeployHotfixOnlyBeforeLast(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	c1 := fx.Commit(t, "one")
	c2 := fx.Commit(t, "two")
	both(t, fx, "stage", release.Deployed{Commit: c2, Version: "v1.1.0-rc.1"})
	both(t, fx, "prod", release.Deployed{Commit: c2, Version: "v1.1.0"})
	releasetest.Git(t, fx.App, "checkout", "-q", "-b", "hotfix/1.0.2", c1)
	h := releasetest.Commit(t, fx.App, "hotfix")
	releasetest.Git(t, fx.App, "checkout", "-q", "dev")
	if err := r.Deploy(ctx, "prod", release.Target{Commit: h, Version: "v1.0.2", Hotfix: true}); !errors.Is(err, release.ErrOlder) {
		t.Errorf("hotfix into prod: err = %v, want ErrOlder", err)
	}
	deploy(t, r, "stage", release.Target{Commit: h, Version: "v1.0.2-rc.1", Hotfix: true})
	if e := envOf(t, status(t, r), "stage"); e.Commit != h {
		t.Errorf("stage = %+v", e)
	}
}

func TestDeployRefusesNotAdopted(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	first := releasetest.Git(t, fx.App, "rev-parse", "HEAD")
	fx.SetOverlay(t, 0, "dev", release.Deployed{Commit: first, Version: "v0.1.0"})
	c := fx.Commit(t, "feat")
	err := r.Deploy(ctx, "dev", release.Target{Commit: c, Version: "1.0.1-dev.1.g" + c[:7]})
	if !errors.Is(err, release.ErrNotAdopted) || !strings.Contains(err.Error(), fx.ConfigBare[1]) {
		t.Fatalf("err = %v, want ErrNotAdopted naming the second config repo", err)
	}
	if len(tools.Built) != 0 {
		t.Errorf("built %+v before refusing", tools.Built)
	}
	if d := envOf(t, status(t, r), "dev").Targets[0].Deployed; d.Commit != first {
		t.Errorf("the adopted overlay moved: %+v", d)
	}
}

func TestDeployFinishesSplitEnvironment(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	adopt(t, fx, "prod")
	c := fx.Commit(t, "fix")
	hook := filepath.Join(fx.ConfigBare[1], "hooks", "pre-receive")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	tg := release.Target{Commit: c, Version: "v1.0.0"}
	if err := r.Deploy(ctx, "prod", tg); err == nil {
		t.Fatal("Deploy succeeded with the second config repo refusing pushes")
	}
	if e := envOf(t, status(t, r), "prod"); !e.Split || e.Targets[0].Deployed.Commit != c {
		t.Fatalf("prod = %+v, want split", e)
	}
	if err := os.Remove(hook); err != nil {
		t.Fatal(err)
	}
	deploy(t, r, "prod", tg)
	if e := envOf(t, status(t, r), "prod"); e.Split || e.Commit != c || e.Version != "v1.0.0" {
		t.Errorf("prod = %+v, want both targets on %.7s", e, c)
	}
}

func TestScanStopsDeploy(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	first := adopt(t, fx, "dev")
	c := fx.Commit(t, "feat")
	tools.FailScan["ghcr.test/app/backend:scan-"+c] = true
	before := heads(t, fx)
	tg := release.Target{Commit: c, Version: "1.0.1-dev.1.g" + c[:7]}
	err := r.Deploy(ctx, "dev", tg)
	if err == nil || !strings.HasPrefix(err.Error(), "backend: ") {
		t.Fatalf("err = %v, want the scan failure naming backend", err)
	}
	// the image that failed never counts as built: a retry scans again
	if err := r.Deploy(ctx, "dev", tg); err == nil || !strings.HasPrefix(err.Error(), "backend: ") {
		t.Fatalf("retry: err = %v, want the scan failure again", err)
	}
	if _, ok := tools.Images["ghcr.test/app/backend:"+c]; ok {
		t.Error("the image that failed its scan got :<sha>")
	}
	for _, tag := range tools.Tagged {
		if strings.Contains(tag, "backend") {
			t.Errorf("after a failed scan backend was tagged: %s", tag)
		}
	}
	if !slices.Equal(heads(t, fx), before) {
		t.Error("an overlay was written after a failed scan")
	}
	if e := envOf(t, status(t, r), "dev"); e.Commit != first {
		t.Errorf("dev = %+v", e)
	}
}

// R3: a target is written only the images its overlay lists, and an image
// goes only to the registries whose overlays list it (built on the first);
// an image no target of the environment lists is an error.
func TestDeployOnlyImagesInTheOverlay(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	dropImage(t, fx, 1, "dev", "quay.test/app/frontend")
	dropImage(t, fx, 0, "dev", "ghcr.test/app/backend")
	dropImage(t, fx, 0, "stage", "ghcr.test/app/frontend")
	dropImage(t, fx, 1, "stage", "quay.test/app/frontend")
	adopt(t, fx, "dev")
	adopt(t, fx, "stage")
	c := fx.Commit(t, "feat")
	dev := "1.0.1-dev.1.g" + c[:7]
	deploy(t, r, "dev", release.Target{Commit: c, Version: dev})
	var built []string
	for _, b := range tools.Built {
		built = append(built, b.Ref)
	}
	if want := []string{"ghcr.test/app/frontend:scan-" + c, "quay.test/app/backend:scan-" + c}; !slices.Equal(built, want) || len(tools.Copied) != 0 {
		t.Errorf("built %q, copied %q; want %q and no copy", built, tools.Copied, want)
	}
	e := envOf(t, status(t, r), "dev")
	if imgs := e.Targets[0].Deployed.Images; len(imgs) != 1 || imgs["ghcr.test/app/frontend"].Tag != dev {
		t.Errorf("dev on ghcr = %+v, want frontend only", imgs)
	}
	if imgs := e.Targets[1].Deployed.Images; len(imgs) != 1 || imgs["quay.test/app/backend"].Tag != dev {
		t.Errorf("dev on quay = %+v, want backend only", imgs)
	}
	n := len(tools.Built)
	err := r.Deploy(ctx, "stage", release.Target{Commit: c, Version: "v1.0.1-rc.1"})
	if err == nil || err.Error() != "image frontend is in no overlay of stage" {
		t.Errorf("err = %v", err)
	}
	if len(tools.Built) != n {
		t.Errorf("built before refusing")
	}
}

// record makes c's release record, with a digest per image and registry
// that the fake registry holds, and returns the digests by <registry>/<image>.
func record(t *testing.T, r *release.Runner, tools *releasetest.Tools, c string, chart store.ChartArtifact) map[string]string {
	t.Helper()
	rec, digests := store.ReleaseRecord{Commit: c, Chart: chart}, map[string]string{}
	for _, img := range []string{"frontend", "backend"} {
		for _, reg := range []string{"ghcr.test/app", "quay.test/app"} {
			d := "sha256:" + c[:6] + reg[:4] + img
			digests[reg+"/"+img] = d
			rec.Artifacts = append(rec.Artifacts, store.Artifact{Image: img, Registry: reg, Digest: d})
			tools.Images[reg+"/"+img+"@"+d] = releasetest.FakeImage{Digest: d, Revision: "built by the old action"}
		}
	}
	if _, err := r.B.CreateRelease(rec); err != nil {
		t.Fatal(err)
	}
	return digests
}

// R15: a commit's release record has its digests; a record without a chart
// (a release from before git-board) leaves the chart as it is.
func TestDeployUsesRecordedDigests(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	adopt(t, fx, "stage")
	adopt(t, fx, "prod")
	legacy := fx.Commit(t, "legacy")
	// reuse_unchanged_images left :<sha> on another commit's build
	tools.Images["ghcr.test/app/frontend:"+legacy] = releasetest.FakeImage{Digest: "sha256:wrong", Revision: "another"}
	digests := record(t, r, tools, legacy, store.ChartArtifact{})
	deploy(t, r, "prod", release.Target{Commit: legacy, Version: "v1.4.19"})
	if len(tools.Built) != 0 || len(tools.Copied) != 0 || len(tools.Charts) != 0 {
		t.Errorf("built %+v, copied %q, charts %v; want the record's digests only", tools.Built, tools.Copied, tools.Charts)
	}
	e := envOf(t, status(t, r), "prod")
	for _, tg := range e.Targets {
		for name, img := range tg.Deployed.Images {
			if img.Digest != digests[name] || img.Tag != "v1.4.19" {
				t.Errorf("prod %s = %+v, want %s", name, img, digests[name])
			}
		}
		if tg.Deployed.Chart != "v0.1.0" {
			t.Errorf("prod chart %q, want it left at v0.1.0", tg.Deployed.Chart)
		}
	}
	for _, b := range fx.ConfigBare { // R17: the version values follow every deploy
		if raw := releasetest.Git(t, b, "show", "main:app/overlays/prod/kustomization.yaml"); !strings.Contains(raw, `APP_VERSION: "v1.4.19"`) {
			t.Errorf("APP_VERSION not moved to v1.4.19:\n%s", raw)
		}
	}
	if e.Commit != legacy || e.Version != "v1.4.19" {
		t.Errorf("prod = %+v", e)
	}

	cut := fx.Commit(t, "cut")
	record(t, r, tools, cut, store.ChartArtifact{Version: "v1.5.0-rc.1", Digest: "sha256:chart"})
	deploy(t, r, "stage", release.Target{Commit: cut, Version: "v1.5.0-rc.1"})
	if len(tools.Built) != 0 || len(tools.Charts) != 0 {
		t.Errorf("built %+v, charts %v; want the recorded chart not pushed again", tools.Built, tools.Charts)
	}
	if got := envOf(t, status(t, r), "stage").Targets[1].Deployed.Chart; got != "v1.5.0-rc.1" {
		t.Errorf("stage chart %q, want the recorded v1.5.0-rc.1", got)
	}
}

func TestDeployFollowed(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	first := adopt(t, fx, "dev")
	fx.Tag(t, first, "v1.0.0")
	fx.Commit(t, "a")
	head := fx.Commit(t, "b")
	if err := r.DeployFollowed(ctx); err != nil {
		t.Fatalf("DeployFollowed: %v", err)
	}
	if e := envOf(t, status(t, r), "dev"); e.Commit != head || e.Version != "1.0.1-dev.2.g"+head[:7] {
		t.Errorf("dev = %s %.7s, want 1.0.1-dev.2.g%.7s", e.Version, e.Commit, head)
	}
	fx.Tag(t, head, "v1.0.1-rc.1", "not-a-release")
	head = fx.Commit(t, "c")
	if err := r.DeployFollowed(ctx); err != nil {
		t.Fatalf("DeployFollowed: %v", err)
	}
	if e := envOf(t, status(t, r), "dev"); e.Commit != head || e.Version != "1.0.1-dev.1.g"+head[:7] {
		t.Errorf("dev = %s, want 1.0.1-dev.1.g%.7s: counted since the candidate, past the final", e.Version, head)
	}
}

func TestDeployFollowedNeedsAFollowedEnv(t *testing.T) {
	r, _, _ := releasetest.NewRunner(t, "stage", "prod")
	if err := r.DeployFollowed(ctx); err == nil {
		t.Error("DeployFollowed succeeded with no environment following the trunk")
	}
}
