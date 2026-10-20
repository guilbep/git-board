package release_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/release"
	"github.com/guilbep/git-board/internal/release/releasetest"
)

// binariesApp is a binaries-only app: no environments, a Go main package
// at cmd/app, the authority on GitHub as acme/app (through the Runner's
// seam: origin stays the local bare, for the push).
func binariesApp(t *testing.T) (*release.Runner, *releasetest.Tools, *releasetest.Fixture, string) {
	t.Helper()
	r, tools, fx := releasetest.NewRunner(t)
	r.Cfg = config.Release{Binaries: []string{"./cmd/app"}, Platforms: []string{runtime.GOOS + "/" + runtime.GOARCH}} // ponytail: the host only, a cross-compile per platform is slow
	r.Repo = "acme/app"
	return r, tools, fx, fx.GoMain(t, "cmd/app")
}

// stepNames collects the names of the steps r reports as done.
func stepNames(r *release.Runner) *[]string {
	var names []string
	r.Step = func(s release.Step) {
		if s.State == "done" {
			names = append(names, s.Name)
		}
	}
	return &names
}

func ext() string {
	if runtime.GOOS == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

func base(files []string) []string {
	var b []string
	for _, f := range files {
		b = append(b, filepath.Base(f))
	}
	return b
}

func TestBinariesCutBuildsPublishesRecords(t *testing.T) {
	r, tools, _, c := binariesApp(t)
	names := stepNames(r)
	tg := cut(t, r, release.Cut{Version: release.Version{Major: 1}, Final: true, Deploy: true})
	if tg.Commit != c || tg.Version.RC != 0 {
		t.Fatalf("cut %s on %.7s, want a final on %.7s", tg.Name, tg.Commit, c)
	}
	want := "app_v1.0.0_" + runtime.GOOS + "_" + runtime.GOARCH + ext()
	if len(tools.Published) != 1 || tools.Published[0].Tag != "v1.0.0" || tools.Published[0].Repo != "acme/app" ||
		!slices.Equal(base(tools.Published[0].Files), []string{want, "checksums.txt"}) {
		t.Errorf("published %+v, want %s and checksums.txt for v1.0.0 on acme/app", tools.Published, want)
	}
	rec, ok, err := r.B.ReleaseFor(c)
	if err != nil || !ok || len(rec.Assets) != 1 || rec.Assets[0].Name != want || len(rec.Assets[0].SHA256) != 64 {
		t.Errorf("record %+v, %v, %v", rec, ok, err)
	}
	if !slices.Equal(*names, []string{"tag", "build", "publish", "record"}) { // Deploy on a binaries-only app deploys nowhere
		t.Errorf("steps %v", *names)
	}
}

// The archive holds a directory named like it with the binary and LICENSE
// (no README.md: the fixture has none), and checksums.txt names it.
func TestBinariesArchiveLayout(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("tar.gz only")
	}
	r, tools, _, _ := binariesApp(t)
	tools.CopyPublished = t.TempDir()
	cut(t, r, release.Cut{Version: release.Version{Major: 1}, Final: true})
	name := "app_v1.0.0_" + runtime.GOOS + "_" + runtime.GOARCH
	f, err := os.Open(filepath.Join(tools.CopyPublished, name+".tar.gz"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	var entries []string
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, fmt.Sprintf("%s %o", h.Name, h.Mode&0o777))
	}
	if want := []string{name + "/LICENSE 644", name + "/app 755"}; !slices.Equal(entries, want) {
		t.Errorf("entries %v, want %v", entries, want)
	}
	raw, _ := os.ReadFile(filepath.Join(tools.CopyPublished, "checksums.txt"))
	sum, _ := os.ReadFile(filepath.Join(tools.CopyPublished, name+".tar.gz"))
	if want := fmt.Sprintf("%x  %s.tar.gz\n", sha256.Sum256(sum), name); string(raw) != want {
		t.Errorf("checksums.txt %q, want %q", raw, want)
	}
}

// A build command replaces the Go builds: whatever it leaves in dist/
// ships, VERSION being the tag.
func TestBinariesBuildCommand(t *testing.T) {
	r, tools, _, c := binariesApp(t)
	r.Cfg.Build = `mkdir -p dist && echo "$VERSION" > dist/app.txt`
	tools.CopyPublished = t.TempDir()
	cut(t, r, release.Cut{Version: release.Version{Major: 1}, Final: true})
	if got := base(tools.Published[0].Files); !slices.Equal(got, []string{"app.txt", "checksums.txt"}) {
		t.Errorf("files %v", got)
	}
	if raw, _ := os.ReadFile(filepath.Join(tools.CopyPublished, "app.txt")); string(raw) != "v1.0.0\n" {
		t.Errorf("app.txt %q", raw)
	}
	if rec, _, _ := r.B.ReleaseFor(c); len(rec.Assets) != 1 || rec.Assets[0].Name != "app.txt" {
		t.Errorf("assets %+v", rec.Assets)
	}
}

func TestBuildCommandMustFillDist(t *testing.T) {
	r, tools, _, _ := binariesApp(t)
	r.Cfg.Build = "true"
	_, err := r.Cut(context.Background(), release.Cut{Version: release.Version{Major: 1}, Final: true})
	if err == nil || !strings.Contains(err.Error(), "dist/") || len(tools.Published) != 0 {
		t.Errorf("err %v, published %v", err, tools.Published)
	}
}

func TestInfer(t *testing.T) {
	_, _, fx := releasetest.NewRunner(t)
	fx.GoMain(t, "cmd/app")
	releasetest.Git(t, fx.App, "remote", "set-url", "origin", "git@github.com:acme/app.git") // after the push
	for p, c := range map[string]string{"cmd/zed/main.go": "package main\n\nfunc main() {}\n", "Makefile": "all:\n\techo\n\ndist: all\n\techo\n"} {
		os.MkdirAll(filepath.Dir(filepath.Join(fx.App, p)), 0o755)
		if err := os.WriteFile(filepath.Join(fx.App, p), []byte(c), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s, err := release.Infer(context.Background(), fx.App)
	// cmd/app is the program (the repo is acme/app): cmd/zed is one of its tools
	if err != nil || !slices.Equal(s.Binaries, []string{"./cmd/app"}) || s.Build != "make dist" ||
		!slices.Equal(s.Platforms, config.DefaultPlatforms) || s.Remote != "origin" || s.Forge != "github" || s.Repo != "acme/app" || s.Note != "" {
		t.Errorf("%+v, %v", s, err)
	}
}

func TestInferNonGitHubRemote(t *testing.T) {
	_, _, fx := releasetest.NewRunner(t)
	fx.GoMain(t, "cmd/app")
	releasetest.Git(t, fx.App, "remote", "set-url", "origin", "https://gitlab.com/acme/app.git")
	s, _ := release.Infer(context.Background(), fx.App)
	if s.Forge != "" || s.Note != "https://gitlab.com/acme/app.git isn't on github.com: git-board publishes GitHub releases only, for now." {
		t.Errorf("%+v", s)
	}
}

func TestInferNoMain(t *testing.T) {
	_, _, fx := releasetest.NewRunner(t)
	s, _ := release.Infer(context.Background(), fx.App)
	if len(s.Binaries) != 0 || s.Note != "No main package found: no cmd/*/main.go and no main.go." {
		t.Errorf("%+v", s)
	}
}

// git-board's own repo infers its own release: the Done-when of GB-19.
func TestInferGitBoard(t *testing.T) {
	s, err := release.Infer(context.Background(), "../..") // cmd/smoketui is a tool: the repo's name picks cmd/git-board
	if err != nil || !slices.Equal(s.Binaries, []string{"./cmd/git-board"}) || s.Build != "make dist" {
		t.Errorf("%+v, %v", s, err)
	}
}

// With no [branches] base, the trunk is the remote's default branch, the
// way branches.Suggest reads it: a program on main releases from main.
func TestBaseIsTheRemoteDefaultBranch(t *testing.T) {
	r, _, fx := releasetest.NewRunner(t)
	r.Base, r.Branches.Base = "", ""
	if got := r.BaseBranch(); got != "dev" {
		t.Errorf("no remote HEAD: %q, want dev", got)
	}
	head := filepath.Join(fx.App, ".git", "refs", "remotes", "origin", "HEAD")
	if err := os.WriteFile(head, []byte("ref: refs/remotes/origin/main\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := r.BaseBranch(); got != "main" {
		t.Errorf("remote HEAD on main: %q", got)
	}
}

// A binaries-only draft has no environment checks: nothing runs anywhere.
func TestBinariesDraftHasNoChecks(t *testing.T) {
	r, _, _, _ := binariesApp(t)
	d, err := r.Draft(context.Background(), true)
	if err != nil || len(d.Checks) != 0 {
		t.Errorf("checks %+v, %v", d.Checks, err)
	}
}
