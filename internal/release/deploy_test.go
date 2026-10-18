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
	if b.Ref != "ghcr.test/app/frontend:"+c || b.Revision != c || filepath.Base(b.Context) != "frontend" || strings.HasPrefix(b.Context, fx.App) {
		t.Errorf("build = %+v, want ghcr.test/app/frontend:<sha> from a checkout of the commit", b)
	}
	if got := releasetest.Git(t, fx.App, "worktree", "list"); strings.Count(got, "\n") != 0 {
		t.Errorf("the build checkout is left behind: %s", got)
	}
	wantCopied := []string{"ghcr.test/app/frontend:" + c + " → quay.test/app/frontend:" + c, "ghcr.test/app/backend:" + c + " → quay.test/app/backend:" + c}
	if !slices.Equal(tools.Copied, wantCopied) {
		t.Errorf("copied %q, want %q", tools.Copied, wantCopied)
	}
	if len(tools.Tagged) != 4 || !slices.Contains(tools.Tagged, "quay.test/app/backend:"+c+" → "+dev) {
		t.Errorf("tagged %q, want each image on each registry tagged %s", tools.Tagged, dev)
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

	deploy(t, r, "stage", release.Target{Commit: c, Version: "v1.0.1-rc.1"})
	if len(tools.Built) != 2 || len(tools.Copied) != 2 {
		t.Errorf("stage built %d, copied %d; want the dev images reused", len(tools.Built), len(tools.Copied))
	}
	st := status(t, r)
	devImgs, stage := envOf(t, st, "dev"), envOf(t, st, "stage")
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
	tools.Images["ghcr.test/app/backend:"+c] = releasetest.FakeImage{Digest: "sha256:be", Revision: c}
	deploy(t, r, "dev", release.Target{Commit: c, Version: "1.0.1-dev.1.g" + c[:7]})
	if len(tools.Built) != 1 || tools.Built[0].Ref != "ghcr.test/app/frontend:"+c {
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
	tools.FailScan["ghcr.test/app/backend:"+c] = true
	before := heads(t, fx)
	err := r.Deploy(ctx, "dev", release.Target{Commit: c, Version: "1.0.1-dev.1.g" + c[:7]})
	if err == nil || !strings.Contains(err.Error(), "backend") {
		t.Fatalf("err = %v, want the scan failure naming backend", err)
	}
	if len(tools.Tagged) != 0 || !slices.Equal(heads(t, fx), before) {
		t.Errorf("after a failed scan: tagged %q, overlays written %v", tools.Tagged, !slices.Equal(heads(t, fx), before))
	}
	if e := envOf(t, status(t, r), "dev"); e.Commit != first {
		t.Errorf("dev = %+v", e)
	}
}

// R3: a target is written only the images its overlay lists; an image no
// target of the environment lists is an error.
func TestDeployOnlyImagesInTheOverlay(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	dropImage(t, fx, 1, "dev", "quay.test/app/frontend")
	dropImage(t, fx, 0, "stage", "ghcr.test/app/frontend")
	dropImage(t, fx, 1, "stage", "quay.test/app/frontend")
	adopt(t, fx, "dev")
	adopt(t, fx, "stage")
	c := fx.Commit(t, "feat")
	dev := "1.0.1-dev.1.g" + c[:7]
	deploy(t, r, "dev", release.Target{Commit: c, Version: dev})
	e := envOf(t, status(t, r), "dev")
	if imgs := e.Targets[1].Deployed.Images; len(imgs) != 1 || imgs["quay.test/app/backend"].Tag != dev {
		t.Errorf("dev on quay = %+v, want backend only", imgs)
	}
	if imgs := e.Targets[0].Deployed.Images; imgs["ghcr.test/app/frontend"].Tag != dev || imgs["ghcr.test/app/backend"].Tag != dev {
		t.Errorf("dev on ghcr = %+v", imgs)
	}
	built := len(tools.Built)
	err := r.Deploy(ctx, "stage", release.Target{Commit: c, Version: "v1.0.1-rc.1"})
	if err == nil || err.Error() != "image frontend is in no overlay of stage" {
		t.Errorf("err = %v", err)
	}
	if len(tools.Built) != built {
		t.Errorf("built before refusing")
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
