package releasetest

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
	"github.com/guilbep/git-board/internal/release"
)

// Fixture is the repositories behind a NewRunner Runner.
type Fixture struct {
	App        string   // the app's work tree, on dev: the Runner's Dir and B
	AppBare    string   // App's origin, the authority: tags and release records go there
	ConfigBare []string // one bare config repo per Runner.Cfg.Deploy target, default branch main
	cfg        config.Release
}

// NewRunner returns a Runner for a co2-like app, the fake tools it runs and
// its repositories. environments default to dev, stage, prod; dev, when
// listed, follows the trunk, dev.
//
// The app builds frontend (./frontend) and backend (./backend) and has a
// chart (./helm, oci://ghcr.test/app/helm) whose valuesInline
// backend.env.APP_VERSION carries the version. It deploys to two targets,
// local bare config repos with an overlay per environment in
// app/overlays/{env}: target 0 pulls from ghcr.test/app, target 1 from
// quay.test/app. Each overlay lists <registry>/frontend and
// <registry>/backend and the chart, and isn't adopted (no git-board
// annotations). B opens App with a user identity; CacheDir is a temp dir.
//
// It sets the git environment for the test (no user or system config, a
// fixed identity), so tests using it can't be parallel.
func NewRunner(t *testing.T, environments ...string) (*release.Runner, *Tools, *Fixture) {
	t.Helper()
	if len(environments) == 0 {
		environments = []string{"dev", "stage", "prod"}
	}
	for k, v := range map[string]string{
		"GIT_CONFIG_GLOBAL": os.DevNull, "GIT_CONFIG_SYSTEM": os.DevNull,
		"GIT_AUTHOR_NAME": "dev", "GIT_AUTHOR_EMAIL": "dev@example.com",
		"GIT_COMMITTER_NAME": "dev", "GIT_COMMITTER_EMAIL": "dev@example.com",
	} {
		t.Setenv(k, v)
	}
	app, bare := NewRemote(t)
	// git-bug's identity comes from the repo's config, not the environment
	Git(t, app, "config", "user.name", "dev")
	Git(t, app, "config", "user.email", "dev@example.com")
	fx := &Fixture{App: app, AppBare: bare, cfg: config.Release{
		Environments:  environments,
		Images:        []config.Image{{Name: "frontend", Context: "./frontend"}, {Name: "backend", Context: "./backend"}},
		Chart:         &config.Chart{Path: "./helm", Registry: "oci://ghcr.test/app/helm"},
		VersionValues: []string{"backend.env.APP_VERSION"},
	}}
	for _, reg := range []string{"ghcr.test/app", "quay.test/app"} {
		files := map[string]string{}
		for _, env := range environments {
			files["app/overlays/"+env+"/kustomization.yaml"] = overlay(reg, fx.cfg.Chart.Registry)
		}
		b := NewConfigRepo(t, files)
		fx.ConfigBare = append(fx.ConfigBare, b)
		fx.cfg.Deploy = append(fx.cfg.Deploy, config.Target{Repo: b, Path: "app/overlays/{env}", Registry: reg})
	}
	b, err := gitbugio.Open(app)
	if err != nil {
		t.Fatalf("gitbugio.Open: %v", err)
	}
	t.Cleanup(func() { _ = b.Close() })
	if _, err := b.NewIdentity("dev", "dev@example.com"); err != nil {
		t.Fatalf("NewIdentity: %v", err)
	}
	tools := NewTools()
	r := &release.Runner{Cfg: fx.cfg, Base: "dev", Dir: app, B: b, Tools: tools, CacheDir: t.TempDir(), Who: "dev"}
	return r, tools, fx
}

func overlay(registry, chart string) string {
	return fmt.Sprintf(`apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - ../../base
images:
  - name: %[1]s/frontend
    newTag: latest
  - name: %[1]s/backend
    newTag: latest
helmCharts:
  - name: app
    releaseName: app
    repo: %[2]s
    version: 0.1.0
    valuesInline:
      backend:
        env:
          APP_VERSION: "latest"
`, registry, chart)
}

// SetOverlay writes d into target's overlay of env, straight into its bare
// repo through a clone of its own, as a deploy would: d.Images keys are
// <registry>/<image>; the chart and version values are written only when
// d.Version is set, the chart's version being d.Chart, else d.Version; the
// commit's subject names d.Version, for History.
func (fx *Fixture) SetOverlay(t *testing.T, target int, env string, d release.Deployed) {
	t.Helper()
	ctx := context.Background()
	c, err := release.OpenConfigRepo(ctx, t.TempDir(), fx.ConfigBare[target])
	if err != nil {
		t.Fatal(err)
	}
	chart := ""
	if d.Version != "" {
		chart = fx.cfg.Chart.Registry
		if d.Chart == "" {
			d.Chart = d.Version
		}
	}
	path := strings.ReplaceAll(fx.cfg.Deploy[target].Path, "{env}", env)
	msg := fmt.Sprintf("chore(manifest): update app (%s) %s [by fixture via releasetest]", env, d.Version)
	if _, err := c.Update(ctx, path, msg, func(o *release.Overlay) (bool, error) {
		return o.Set(d, chart, fx.cfg.VersionValues)
	}); err != nil {
		t.Fatalf("SetOverlay %d %s: %v", target, env, err)
	}
}

// Commit makes an empty commit on the trunk, pushes it and returns it.
func (fx *Fixture) Commit(t *testing.T, msg string) string {
	t.Helper()
	c := Commit(t, fx.App, msg)
	Git(t, fx.App, "push", "-q", "origin", "dev")
	return c
}

// Merge merges a new branch holding one commit into the trunk, --no-ff,
// with subject ("": git's "Merge branch '<branch>' into dev"), pushes it
// and returns the merge commit.
func (fx *Fixture) Merge(t *testing.T, branch, subject string) string {
	t.Helper()
	if subject == "" {
		subject = "Merge branch '" + branch + "' into dev"
	}
	Git(t, fx.App, "checkout", "-q", "-b", branch)
	Commit(t, fx.App, branch)
	Git(t, fx.App, "checkout", "-q", "dev")
	Git(t, fx.App, "merge", "-q", "--no-ff", "-m", subject, branch)
	Git(t, fx.App, "push", "-q", "origin", "dev")
	return Git(t, fx.App, "rev-parse", "HEAD")
}

// Tag puts annotated tags on commit and pushes them.
func (fx *Fixture) Tag(t *testing.T, commit string, names ...string) {
	t.Helper()
	for _, n := range names {
		Git(t, fx.App, "tag", "-a", n, "-m", n, commit)
		Git(t, fx.App, "push", "-q", "origin", "refs/tags/"+n)
	}
}

// Config is the fixture's [release] table as a .git-board.toml, for a CLI
// run in App.
func (fx *Fixture) Config(t *testing.T) string {
	t.Helper()
	var b bytes.Buffer
	if err := toml.NewEncoder(&b).Encode(map[string]any{"release": fx.cfg}); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// GoMain writes a Go module with a main package at pkg (cmd/app) and a
// LICENSE, commits and pushes them, and returns the commit.
func (fx *Fixture) GoMain(t *testing.T, pkg string) string {
	t.Helper()
	for p, c := range map[string]string{
		"go.mod":         "module app\n\ngo 1.22\n",
		pkg + "/main.go": "package main\n\nimport \"fmt\"\n\nfunc main() { fmt.Println(\"hi\") }\n",
		"LICENSE":        "MIT\n",
	} {
		full := filepath.Join(fx.App, p)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	Git(t, fx.App, "add", ".")
	return fx.Commit(t, "a main package")
}
