// Package releases is the release kind: a record of each released
// commit (docs/spec/data-model.md, Releases), the versions and tags that
// name it, the [release] table that says where an app's code goes, and
// the GitOps overlay that says what runs there. It drives no tool and
// reaches no registry: tools/ship cuts and deploys with it.
package releases

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"github.com/guilbep/git-board/conf"
)

// Config is the [release] table of .git-board.toml: how the project's
// code gets to its environments, and the environments' roles. The
// application reads and checks it (internal/config); internal/ship
// cuts and deploys by it. Absent or without environments, the project
// has no releases.
type Config struct {
	Remote       string   `toml:"remote" json:"remote"`             // the authority for tags and release records, default origin
	Environments []string `toml:"environments" json:"environments"` // ordered, e.g. dev, stage, prod
	// Follow is the environment that follows the trunk's head. Absent: "dev"
	// when listed. "": none does.
	Follow        *string  `toml:"follow" json:"follow"`
	Checks        string   `toml:"checks" json:"checks"`                 // the command a merge must pass, e.g. "make ci"
	Images        []Image  `toml:"images" json:"images"`                 // what to build
	Chart         *Chart   `toml:"chart" json:"chart"`                   // the helm chart to package
	VersionValues []string `toml:"version_values" json:"version_values"` // dotted paths, under the chart's valuesInline, that carry the version
	Builder       string   `toml:"builder" json:"builder"`               // a remote buildx builder, instead of the local one
	Deploy        []Target `toml:"deploy" json:"deploy"`                 // the config repos holding the overlays
	// Binaries are main packages built per platform and published as
	// archives on the forge's release: "./cmd/git-board". A table with
	// binaries and no environments is enabled: finals only, deployed nowhere.
	Binaries  []string `toml:"binaries" json:"binaries"`
	Platforms []string `toml:"platforms" json:"platforms"` // os/arch; empty: DefaultPlatforms
	// Build is a command run with sh -c in a clean checkout of the tag
	// instead of the Go builds; it leaves the archives in dist/.
	Build string `toml:"build" json:"build"`
}

// DefaultPlatforms are the platforms binaries are built for when
// platforms says none.
var DefaultPlatforms = []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "windows/amd64"}

// BuildPlatforms are the platforms to build the binaries for.
func (r Config) BuildPlatforms() []string {
	if len(r.Platforms) > 0 {
		return r.Platforms
	}
	return DefaultPlatforms
}

// Image is an image to build: its name and docker build context.
type Image struct {
	Name    string `toml:"name" json:"name"`
	Context string `toml:"context" json:"context"`
}

// Chart is the helm chart: its directory and the OCI registry it goes to.
type Chart struct {
	Path     string `toml:"path" json:"path"`
	Registry string `toml:"registry" json:"registry"`
}

// Target is a config repo's overlays: Path has {env}; Registry is where its
// images are.
type Target struct {
	Repo     string `toml:"repo" json:"repo"`
	Path     string `toml:"path" json:"path"`
	Registry string `toml:"registry" json:"registry"`
}

// Enabled is whether the project has releases: environments to deploy
// to, or binaries to publish.
func (r Config) Enabled() bool { return len(r.Environments) > 0 || len(r.Binaries) > 0 }

// RemoteName is the authority remote.
func (r Config) RemoteName() string { return cmp.Or(r.Remote, "origin") }

// Followed is the environment that follows the trunk, "" when none does.
func (r Config) Followed() string {
	if r.Follow != nil {
		return *r.Follow
	}
	if slices.Contains(r.Environments, "dev") {
		return "dev"
	}
	return ""
}

// Candidates are the environments that take candidates: after Followed and
// before Last.
func (r Config) Candidates() []string {
	from := slices.Index(r.Environments, r.Followed()) + 1 // none: -1 + 1
	if from >= len(r.Environments)-1 {
		return nil
	}
	return r.Environments[from : len(r.Environments)-1]
}

// Last is the environment that takes finals, "" when there are none.
func (r Config) Last() string {
	if len(r.Environments) == 0 {
		return ""
	}
	return r.Environments[len(r.Environments)-1]
}

// FollowsLast tells whether the last environment follows the trunk: a
// single-environment app, whose cut tags what it runs and deploys nowhere.
// An app with no environment (binaries only) deploys nowhere too.
func (r Config) FollowsLast() bool {
	f := r.Followed()
	return r.Last() == "" || f != "" && f == r.Last()
}

// CutsFinal tells whether a cut is a final: asked for, or the app has no
// candidate environment.
func (r Config) CutsFinal(asked bool) bool { return asked || len(r.Candidates()) == 0 }

// DeployTo is the environment a cut goes to: the first candidate one, the
// last for a final.
func (r Config) DeployTo(final bool) string {
	if final {
		return r.Last()
	}
	return r.Candidates()[0]
}

// Declare declares [release] on f.
func Declare(f *conf.File) *conf.Handle[Config] {
	return conf.Declare(f, conf.Table[Config]{Name: "release", Check: check})
}

// check refuses a table that can't name its environments, or deploys
// somewhere the same for all of them.
func check(r *Config) error {
	for i, e := range r.Environments {
		if strings.TrimSpace(e) == "" {
			return fmt.Errorf("release: an environment needs a name")
		}
		if slices.Contains(r.Environments[:i], e) {
			return fmt.Errorf("release: environment %q listed twice", e)
		}
	}
	if f := r.Followed(); r.Follow != nil && f != "" && !slices.Contains(r.Environments, f) {
		return fmt.Errorf("release: follow %q isn't in environments", f)
	}
	for _, t := range r.Deploy {
		switch {
		case strings.TrimSpace(t.Repo) == "":
			return fmt.Errorf("release: deploy needs a repo")
		case !strings.Contains(t.Path, "{env}"):
			return fmt.Errorf("release: deploy %s: path needs {env}", t.Repo)
		case len(r.Images) > 0 && strings.TrimSpace(t.Registry) == "":
			return fmt.Errorf("release: deploy %s: registry needs a value, for the images", t.Repo)
		}
	}
	for _, i := range r.Images {
		if strings.TrimSpace(i.Name) == "" {
			return fmt.Errorf("release: image needs a name")
		}
	}
	for _, b := range r.Binaries {
		if strings.TrimSpace(b) == "" {
			return fmt.Errorf("release: a binary needs a package path")
		}
	}
	if len(r.Binaries) == 0 {
		if r.Build != "" {
			return fmt.Errorf("release: build needs binaries")
		}
		if len(r.Platforms) > 0 {
			return fmt.Errorf("release: platforms needs binaries")
		}
	}
	for _, p := range r.Platforms {
		if os, arch, ok := strings.Cut(p, "/"); !ok || os == "" || arch == "" || strings.Contains(arch, "/") {
			return fmt.Errorf("release: platform %q isn't os/arch", p)
		}
	}
	return nil
}
