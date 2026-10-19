package release_test

import (
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/release"
	"github.com/guilbep/git-board/internal/release/releasetest"
)

// runs makes target's overlay of env run tag (a first image tagged tag at digest "").
func runs(t *testing.T, fx *releasetest.Fixture, target int, env, tag, digest string) {
	t.Helper()
	reg := []string{"ghcr.test/app", "quay.test/app"}[target]
	fx.SetOverlay(t, target, env, release.Deployed{Version: tag, Images: map[string]release.Image{reg + "/frontend": {Tag: tag, Digest: digest}}})
}

func TestProposalsFromTagAndLabel(t *testing.T) {
	r, tools, fx := releasetest.NewRunner(t)
	c := fx.Commit(t, "feat")
	fx.Tag(t, c, "v1.0.0")
	runs(t, fx, 0, "prod", "v1.0.0", "")
	runs(t, fx, 1, "prod", "v1.0.0", "")
	d := "sha256:aaaaaaaaaaaaaaaaaaaa"
	tools.Images["ghcr.test/app/frontend@"+d] = releasetest.FakeImage{Digest: d, Revision: c}
	runs(t, fx, 0, "dev", "dev", d)
	runs(t, fx, 1, "dev", "dev", "sha256:bbbbbbbbbbbbbbbbbbbb") // not in the registry
	runs(t, fx, 0, "stage", "stage", "")
	both(t, fx, "stage", release.Deployed{Commit: c, Version: "v1.0.0"}) // adopted: left out

	ps, err := r.Proposals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]release.Proposal{}
	for _, p := range ps {
		got[p.Env+" "+p.Path+" "+p.Repo] = p
	}
	if len(ps) != 4 {
		t.Fatalf("proposals = %+v, want dev and prod on both targets", ps)
	}
	p := got["prod app/overlays/prod "+fx.ConfigBare[0]]
	if p.Commit != c || p.From != "tag" || p.Runs != "v1.0.0" {
		t.Errorf("prod = %+v", p)
	}
	p = got["dev app/overlays/dev "+fx.ConfigBare[0]]
	if p.Commit != c || p.From != "label" || p.Runs != "dev@aaaaaaaaaaaa" {
		t.Errorf("dev labelled = %+v", p)
	}
	p = got["dev app/overlays/dev "+fx.ConfigBare[1]]
	if p.Commit != "" || p.From != "" {
		t.Errorf("dev unlabelled = %+v", p)
	}
}

func TestAdoptWritesOnlyAnnotations(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	c := fx.Commit(t, "feat")
	fx.Tag(t, c, "v1.0.0")
	runs(t, fx, 0, "prod", "v1.0.0", "")
	before := cloneOf(t, fx.ConfigBare[0])
	ps, err := r.Proposals(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var mine []release.Proposal
	for _, p := range ps {
		if p.Env == "prod" && p.Repo == fx.ConfigBare[0] {
			mine = append(mine, p)
		}
	}
	if err := r.Adopt(ctx, mine); err != nil {
		t.Fatal(err)
	}
	after := cloneOf(t, fx.ConfigBare[0])
	if releasetest.Git(t, after, "log", "-1", "--format=%s") != "chore(manifest): adopt app (prod) [by dev via git-board]" {
		t.Errorf("message = %s", releasetest.Git(t, after, "log", "-1", "--format=%s"))
	}
	diff := releasetest.Git(t, after, "diff", "-U0", releasetest.Git(t, before, "rev-parse", "HEAD"), "HEAD")
	var changed []string
	for _, l := range strings.Split(diff, "\n") {
		if (strings.HasPrefix(l, "+") || strings.HasPrefix(l, "-")) && !strings.HasPrefix(l, "+++") && !strings.HasPrefix(l, "---") {
			changed = append(changed, l)
		}
	}
	want := "+commonAnnotations:|+  git-board/commit: " + c + "|+  git-board/version: v1.0.0"
	if strings.Join(changed, "|") != want {
		t.Errorf("changed lines = %q, want %q", changed, want)
	}
	if e := envOf(t, status(t, r), "prod"); e.Targets[0].Deployed.Commit != c || e.Targets[1].Deployed.Commit != "" {
		t.Errorf("prod = %+v", e)
	}
}

func TestAdoptRefusesUnknownCommit(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	c := fx.Commit(t, "feat")
	runs(t, fx, 0, "prod", "v1.0.0", "")
	before := heads(t, fx)
	ps := []release.Proposal{
		{Env: "prod", Repo: fx.ConfigBare[0], Path: "app/overlays/prod", Commit: c},
		{Env: "prod", Repo: fx.ConfigBare[1], Path: "app/overlays/prod", Commit: "deadbeef"},
	}
	if err := r.Adopt(ctx, ps); err == nil || !strings.Contains(err.Error(), "prod") {
		t.Fatalf("Adopt = %v, want an error naming the proposal", err)
	}
	if got := heads(t, fx); got[0] != before[0] || got[1] != before[1] {
		t.Errorf("heads moved: %v -> %v", before, got)
	}
}
