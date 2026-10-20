package config

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
)

// Release is the [release] table: how the project's code gets to its
// environments (internal/release). Absent or without environments, the
// project has no releases.
type Release struct {
	Remote       string   `toml:"remote"`       // the authority for tags and release records, default origin
	Environments []string `toml:"environments"` // ordered, e.g. dev, stage, prod
	// Follow is the environment that follows the trunk's head. Absent: "dev"
	// when listed. "": none does.
	Follow        *string  `toml:"follow"`
	Checks        string   `toml:"checks"`         // the command a merge must pass, e.g. "make ci"
	Images        []Image  `toml:"images"`         // what to build
	Chart         *Chart   `toml:"chart"`          // the helm chart to package
	VersionValues []string `toml:"version_values"` // dotted paths, under the chart's valuesInline, that carry the version
	Builder       string   `toml:"builder"`        // a remote buildx builder, instead of the local one
	Deploy        []Target `toml:"deploy"`         // the config repos holding the overlays
	// Binaries are main packages built per platform and published as
	// archives on the forge's release: "./cmd/git-board". A table with
	// binaries and no environments is enabled: finals only, deployed nowhere.
	Binaries  []string `toml:"binaries"`
	Platforms []string `toml:"platforms"` // os/arch; empty: DefaultPlatforms
	// Build is a command run with sh -c in a clean checkout of the tag
	// instead of the Go builds; it leaves the archives in dist/.
	Build string `toml:"build"`
}

// DefaultPlatforms are the platforms binaries are built for when
// platforms says none.
var DefaultPlatforms = []string{"linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64", "windows/amd64"}

// BuildPlatforms are the platforms to build the binaries for.
func (r Release) BuildPlatforms() []string {
	if len(r.Platforms) > 0 {
		return r.Platforms
	}
	return DefaultPlatforms
}

// Image is an image to build: its name and docker build context.
type Image struct {
	Name    string `toml:"name"`
	Context string `toml:"context"`
}

// Chart is the helm chart: its directory and the OCI registry it goes to.
type Chart struct {
	Path     string `toml:"path"`
	Registry string `toml:"registry"`
}

// Target is a config repo's overlays: Path has {env}; Registry is where its
// images are.
type Target struct {
	Repo     string `toml:"repo"`
	Path     string `toml:"path"`
	Registry string `toml:"registry"`
}

// Enabled is whether the project has releases: environments to deploy
// to, or binaries to publish.
func (r Release) Enabled() bool { return len(r.Environments) > 0 || len(r.Binaries) > 0 }

// RemoteName is the authority remote.
func (r Release) RemoteName() string { return cmp.Or(r.Remote, "origin") }

// Followed is the environment that follows the trunk, "" when none does.
func (r Release) Followed() string {
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
func (r Release) Candidates() []string {
	from := slices.Index(r.Environments, r.Followed()) + 1 // none: -1 + 1
	if from >= len(r.Environments)-1 {
		return nil
	}
	return r.Environments[from : len(r.Environments)-1]
}

// Last is the environment that takes finals, "" when there are none.
func (r Release) Last() string {
	if len(r.Environments) == 0 {
		return ""
	}
	return r.Environments[len(r.Environments)-1]
}

// checkRelease refuses a table that can't name its environments, or
// deploys somewhere the same for all of them.
func checkRelease(r Release) error {
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
		case len(r.Images) > 0 && strings.TrimSpace(t.Registry) == "": // a tag-only app has no image to push
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
