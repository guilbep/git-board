package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/release"
	"github.com/guilbep/git-board/internal/release/releasetest"
)

// releaseRepo points the CLI at a releasetest fixture: its app repo as the
// working directory, its config as .git-board.toml, its fake tools. The
// fixture's own backend is closed, git-bug's lock being the CLI's to take.
func releaseRepo(t *testing.T) (*releasetest.Tools, *releasetest.Fixture) {
	t.Helper()
	r, tools, fx := releasetest.NewRunner(t)
	if err := r.B.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(fx.App, ".git-board.toml"), []byte(fx.Config(t)), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(fx.App)
	old := releaseTools
	releaseTools = func() release.Tools { return tools }
	t.Cleanup(func() { releaseTools = old })
	return tools, fx
}

func both(t *testing.T, fx *releasetest.Fixture, env string, d release.Deployed) {
	t.Helper()
	for i := range fx.ConfigBare {
		fx.SetOverlay(t, i, env, d)
	}
}

func first(t *testing.T, fx *releasetest.Fixture) string {
	return releasetest.Git(t, fx.App, "rev-list", "--max-parents=0", "HEAD")
}

// squash collapses runs of spaces, so a layout isn't what a test pins.
func squash(s string) string { return strings.Join(strings.Fields(s), " ") }

func run(t *testing.T, args ...string) string {
	t.Helper()
	out, err := execute(t, args)
	if err != nil {
		t.Fatalf("git-board %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func heads(t *testing.T, fx *releasetest.Fixture) []string {
	var hs []string
	for _, b := range fx.ConfigBare {
		hs = append(hs, releasetest.Git(t, b, "rev-parse", "main"))
	}
	return hs
}

func TestReleaseStatusPrints(t *testing.T) {
	_, fx := releaseRepo(t)
	c := first(t, fx)
	fx.Tag(t, c, "v1.0.0")
	both(t, fx, "stage", release.Deployed{Commit: c, Version: "v1.0.0-rc.2"})
	fx.SetOverlay(t, 0, "prod", release.Deployed{Commit: c, Version: "v1.0.0"})
	fx.SetOverlay(t, 1, "prod", release.Deployed{Commit: fx.Commit(t, "older"), Version: "v0.9.0"})
	both(t, fx, "dev", release.Deployed{Commit: c, Version: "1.0.0-dev.0.g" + c[:7]})
	fx.Merge(t, "a", "")
	fx.Merge(t, "b", "")

	got := squash(run(t, "release"))
	for _, want := range []string{
		"dev 1.0.0-dev.0.g" + c[:7] + " " + c[:7] + " 2 merges behind dev",
		"stage v1.0.0-rc.2 " + c[:7],
		"prod v1.0.0 " + c[:7] + " split: " + "config (quay.test)" + " on v0.9.0",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("status lacks %q:\n%s", want, got)
		}
	}
	if got != squash(run(t, "release", "status")) {
		t.Error("release and release status differ")
	}
}

func TestDeployDev(t *testing.T) {
	_, fx := releaseRepo(t)
	c := first(t, fx)
	fx.Tag(t, c, "v1.0.0")
	both(t, fx, "dev", release.Deployed{Commit: c, Version: "v1.0.0"})
	head := fx.Commit(t, "feat")

	out := run(t, "deploy", "dev")
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "  ") {
		t.Errorf("want step lines, got:\n%s", out)
	}
	if last := lines[len(lines)-1]; last != "dev runs 1.0.1-dev.1.g"+head[:7] {
		t.Errorf("last line = %q", last)
	}
	if _, err := execute(t, []string{"deploy", "stage"}); err == nil || !strings.Contains(err.Error(), "--release") {
		t.Errorf("deploy stage without --release: %v", err)
	}
}

func TestCutNoDeploy(t *testing.T) {
	_, fx := releaseRepo(t)
	c := first(t, fx)
	fx.Tag(t, c, "v1.0.0")
	both(t, fx, "stage", release.Deployed{Commit: c, Version: "v1.0.0"})
	both(t, fx, "prod", release.Deployed{Commit: c, Version: "v1.0.0"})
	m := fx.Merge(t, "fix/12-a", "")
	both(t, fx, "dev", release.Deployed{Commit: m, Version: "1.0.1-dev.1.g" + m[:7]})
	before := heads(t, fx)

	out := run(t, "release", "cut", "--no-deploy")
	if !strings.Contains(out, "v1.0.1-rc.1") {
		t.Errorf("no tag in:\n%s", out)
	}
	if !slices.Equal(heads(t, fx), before) {
		t.Error("an overlay moved with --no-deploy")
	}
	if got := releasetest.Git(t, fx.AppBare, "tag", "-l", "v1.0.1-rc.1"); got == "" {
		t.Error("tag not pushed")
	}
}

func TestRollbackNeedsReason(t *testing.T) {
	releaseRepo(t)
	_, err := execute(t, []string{"release", "rollback", "prod", "v1.0.0"})
	if err == nil || err.Error() != "--reason is required" {
		t.Errorf("err = %v", err)
	}
	_, err = execute(t, []string{"release", "yank", "v1.0.0"})
	if err == nil || err.Error() != "--reason is required" {
		t.Errorf("yank err = %v", err)
	}
}

func TestAdoptNeedsYes(t *testing.T) {
	_, fx := releaseRepo(t)
	c := first(t, fx)
	fx.Tag(t, c, "v1.0.0")
	for i, reg := range []string{"ghcr.test/app", "quay.test/app"} { // runs v1.0.0, not adopted
		fx.SetOverlay(t, i, "prod", release.Deployed{Version: "v1.0.0", Images: map[string]release.Image{reg + "/frontend": {Tag: "v1.0.0"}}})
	}
	before := heads(t, fx)

	out := run(t, "release", "adopt")
	if !strings.Contains(out, "prod") || !strings.Contains(out, "v1.0.0") || !strings.Contains(out, c[:7]) || !strings.Contains(out, "--yes") {
		t.Errorf("proposals:\n%s", out)
	}
	if !slices.Equal(heads(t, fx), before) {
		t.Error("adopt without --yes wrote")
	}
	run(t, "release", "adopt", "--yes")
	if slices.Equal(heads(t, fx), before) {
		t.Error("adopt --yes wrote nothing")
	}
	if !strings.Contains(squash(run(t, "release")), "prod v1.0.0 "+c[:7]) {
		t.Error("prod not adopted")
	}
}

func TestReleaseNotesAll(t *testing.T) {
	_, fx := releaseRepo(t)
	c := first(t, fx)
	fx.Tag(t, c, "v1.0.0")
	both(t, fx, "stage", release.Deployed{Commit: c, Version: "v1.0.0"})
	both(t, fx, "prod", release.Deployed{Commit: c, Version: "v1.0.0"})
	m := fx.Merge(t, "fix/12-a", "")
	both(t, fx, "dev", release.Deployed{Commit: m, Version: "1.0.1-dev.1.g" + m[:7]})
	run(t, "release", "cut", "--no-deploy")
	if out := run(t, "release", "notes"); strings.Contains(out, "rc.1") {
		t.Errorf("notes without --all lists a candidate:\n%s", out)
	}
	if out := run(t, "release", "notes", "--all"); !strings.Contains(out, "## v1.0.1-rc.1 (") {
		t.Errorf("notes --all:\n%s", out)
	}
	if _, err := execute(t, []string{"release", "cut", "--version", "1.0.1-rc.2"}); err == nil || !strings.Contains(err.Error(), "final version") {
		t.Errorf("cut --version rc: %v", err)
	}
}
