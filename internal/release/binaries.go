package release

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// Setup is what Enable binary release proposes from a repository, and
// what the page confirms.
type Setup struct {
	Binaries  []string `json:"binaries"`
	Platforms []string `json:"platforms"`
	Build     string   `json:"build"`
	Remote    string   `json:"remote"`
	Forge     string   `json:"forge"` // github; "" when the remote isn't one git-board publishes to
	Repo      string   `json:"repo"`  // owner/name on the forge
	Note      string   `json:"note"`  // why it can't enable; "" when it can
}

// Infer is the binary release setup of dir: its main packages (cmd/*,
// else the root), "make dist" when the Makefile has a dist target, the
// default platforms, origin (else the first remote) and its forge.
func Infer(ctx context.Context, dir string) (Setup, error) {
	s := Setup{Platforms: config.DefaultPlatforms}
	mains, _ := filepath.Glob(filepath.Join(dir, "cmd", "*", "main.go"))
	sort.Strings(mains)
	for _, m := range mains {
		s.Binaries = append(s.Binaries, "./cmd/"+filepath.Base(filepath.Dir(m)))
	}
	if len(mains) == 0 {
		if src, err := os.ReadFile(filepath.Join(dir, "main.go")); err == nil && strings.Contains(string(src), "package main") {
			s.Binaries = []string{"."}
		}
	}
	if f, err := os.Open(filepath.Join(dir, "Makefile")); err == nil {
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			if strings.HasPrefix(sc.Text(), "dist:") {
				s.Build = "make dist"
			}
		}
		f.Close()
	}
	remotes, err := Git(ctx, dir, "remote")
	if err != nil {
		return s, err
	}
	names := strings.Fields(remotes)
	switch {
	case slices.Contains(names, "origin"):
		s.Remote = "origin"
	case len(names) > 0:
		s.Remote = names[0]
	}
	url := ""
	if s.Remote != "" {
		url, _ = Git(ctx, dir, "remote", "get-url", s.Remote)
		if s.Repo = gitbugio.GitHubRepoOf(url); s.Repo != "" {
			s.Forge = "github" // ponytail: GitHub only; gitlab and gitea are a case each here and in Publish
		}
	}
	abs, _ := filepath.Abs(dir)
	for _, name := range []string{path.Base(s.Repo), filepath.Base(abs)} {
		if slices.Contains(s.Binaries, "./cmd/"+name) {
			s.Binaries = []string{"./cmd/" + name} // ponytail: cmd/<repo name> is the program, the other mains its tools
			break
		}
	}
	switch {
	case len(s.Binaries) == 0:
		s.Note = "No main package found: no cmd/*/main.go and no main.go."
	case s.Remote == "":
		s.Note = "No remote: add one, then enable."
	case s.Forge == "":
		s.Note = url + " isn't on github.com: git-board publishes GitHub releases only, for now."
	}
	return s, nil
}

// binaries builds the archives of t in a clean checkout of it and
// publishes them on the forge's release with notes: the build and
// publish steps. Nothing when the table lists no binaries.
func (r *Runner) binaries(ctx context.Context, t Tag, notes string) ([]gitbugio.Asset, error) {
	if len(r.Cfg.Binaries) == 0 {
		return nil, nil
	}
	if r.Repo == "" {
		return nil, fmt.Errorf("%s isn't on github.com: git-board publishes GitHub releases only, for now", r.Cfg.RemoteName())
	}
	dir, err := r.checkout(ctx, t.Commit)
	if err != nil {
		return nil, err
	}
	defer r.removeCheckout(dir)
	dist := filepath.Join(dir, "dist")
	var assets []gitbugio.Asset
	err = r.do("build", func() (string, error) {
		what := "for " + strings.Join(r.Cfg.BuildPlatforms(), ", ")
		if r.Cfg.Build != "" {
			what = "from dist/"
			cmd := exec.CommandContext(ctx, "sh", "-c", r.Cfg.Build) // ponytail: the shell parses it, as make would
			cmd.Dir, cmd.Env = dir, append(os.Environ(), "VERSION="+t.Name)
			if out, err := cmd.CombinedOutput(); err != nil {
				return "", fmt.Errorf("build %q: %w: %s", r.Cfg.Build, err, strings.TrimSpace(string(out)))
			}
		} else {
			for _, pkg := range r.Cfg.Binaries {
				for _, p := range r.Cfg.BuildPlatforms() { // ponytail: one build at a time
					if err := goBuild(ctx, dir, pkg, binaryName(pkg, r.Dir), t.Name, p); err != nil {
						return "", err
					}
				}
			}
		}
		var err error
		if assets, err = checksums(dist); err != nil {
			return "", err
		}
		if len(assets) == 0 {
			return "", fmt.Errorf("build %q left nothing in dist/", r.Cfg.Build)
		}
		return fmt.Sprintf("%d archives %s", len(assets), what), nil
	})
	if err != nil {
		return nil, err
	}
	err = r.do("publish", func() (string, error) {
		files := make([]string, 0, len(assets)+1)
		for _, a := range assets {
			files = append(files, filepath.Join(dist, a.Name))
		}
		files = append(files, filepath.Join(dist, "checksums.txt"))
		return fmt.Sprintf("%s on GitHub, %d files", t.Name, len(files)), r.Tools.Publish(ctx, r.Repo, t.Name, notes, files)
	})
	return assets, err
}

// binaryName is what pkg builds: cmd/git-board gives git-board; the
// root package, the repository's directory name.
func binaryName(pkg, repoDir string) string {
	if n := path.Base(pkg); n != "." && n != "/" {
		return n
	}
	return filepath.Base(repoDir)
}

// goBuild builds pkg for platform (os/arch) as the Makefile's dist does,
// CGO off, stripped, and archives it in dir/dist as
// <name>_<version>_<os>_<arch>.tar.gz (.zip for windows), holding a
// directory of that name with the binary, LICENSE and README.md when dir
// has them.
func goBuild(ctx context.Context, dir, pkg, name, version, platform string) error {
	goos, arch, _ := strings.Cut(platform, "/")
	stage := filepath.Join(dir, "dist", fmt.Sprintf("%s_%s_%s_%s", name, version, goos, arch))
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	bin := name
	if goos == "windows" {
		bin += ".exe"
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-trimpath", "-ldflags", "-s -w", "-o", filepath.Join(stage, bin), pkg)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "CGO_ENABLED=0", "GOOS="+goos, "GOARCH="+arch)
	if out, err := cmd.CombinedOutput(); err != nil {
		if errors.Is(err, exec.ErrNotFound) {
			return fmt.Errorf("go isn't installed: %s (README.md, Install)", install["go"])
		}
		return fmt.Errorf("go build %s for %s: %w: %s", pkg, platform, err, strings.TrimSpace(string(out)))
	}
	for _, f := range []string{"LICENSE", "README.md"} {
		if src, err := os.ReadFile(filepath.Join(dir, f)); err == nil {
			if err := os.WriteFile(filepath.Join(stage, f), src, 0o644); err != nil {
				return err
			}
		}
	}
	if goos == "windows" {
		return zipDir(stage+".zip", stage)
	}
	return tarDir(stage+".tar.gz", stage)
}

// walkFiles calls fn with each regular file under dir: its path in the
// archive (<dir's name>/<relative path>), its info and its content.
func walkFiles(dir string, fn func(name string, fi fs.FileInfo, src io.Reader) error) error {
	return filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil || !d.Type().IsRegular() {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		return fn(filepath.Base(dir)+"/"+filepath.ToSlash(rel), fi, f)
	})
}

func tarDir(out, dir string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	err = walkFiles(dir, func(name string, fi fs.FileInfo, src io.Reader) error {
		h, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		h.Name = name
		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		_, err = io.Copy(tw, src)
		return err
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

func zipDir(out, dir string) error {
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	err = walkFiles(dir, func(name string, fi fs.FileInfo, src io.Reader) error {
		h, err := zip.FileInfoHeader(fi)
		if err != nil {
			return err
		}
		h.Name, h.Method = name, zip.Deflate
		w, err := zw.CreateHeader(h)
		if err != nil {
			return err
		}
		_, err = io.Copy(w, src)
		return err
	})
	if err != nil {
		return err
	}
	return zw.Close()
}

// checksums writes dist/checksums.txt over dist's files, in sha256sum's
// format, and returns them as assets; none when dist doesn't exist.
// ponytail: a build command's own checksums.txt is rewritten, the same
func checksums(dist string) ([]gitbugio.Asset, error) {
	entries, err := os.ReadDir(dist) // sorted by name
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var assets []gitbugio.Asset
	var b strings.Builder
	for _, e := range entries {
		if !e.Type().IsRegular() || e.Name() == "checksums.txt" {
			continue
		}
		f, err := os.Open(filepath.Join(dist, e.Name()))
		if err != nil {
			return nil, err
		}
		h := sha256.New()
		_, err = io.Copy(h, f)
		f.Close()
		if err != nil {
			return nil, err
		}
		sum := fmt.Sprintf("%x", h.Sum(nil))
		assets = append(assets, gitbugio.Asset{Name: e.Name(), SHA256: sum})
		fmt.Fprintf(&b, "%s  %s\n", sum, e.Name())
	}
	if len(assets) == 0 {
		return nil, nil
	}
	return assets, os.WriteFile(filepath.Join(dist, "checksums.txt"), []byte(b.String()), 0o644)
}
