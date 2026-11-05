// Package releases is the release kind (ADR 0001). M2 reads its table of
// .git-board.toml, [release], with v1's rules; the kind comes with M6.
package releases

import (
	"fmt"
	"slices"
	"strings"

	"github.com/guilbep/git-board/next/conf"
)

// Config is the [release] table.
type Config struct {
	Remote       string   `toml:"remote"`       // the authority for tags and records, default origin
	Environments []string `toml:"environments"` // ordered: dev, stage, prod
	// Follow is the environment that follows the trunk's head. Absent:
	// "dev" when listed. "": none does.
	Follow        *string  `toml:"follow"`
	Checks        string   `toml:"checks"`
	Images        []Image  `toml:"images"`
	Chart         *Chart   `toml:"chart"`
	VersionValues []string `toml:"version_values"`
	Builder       string   `toml:"builder"`
	Deploy        []Target `toml:"deploy"`
	Binaries      []string `toml:"binaries"`
	Platforms     []string `toml:"platforms"`
	Build         string   `toml:"build"`
}

// Image is an image to build.
type Image struct {
	Name    string `toml:"name"`
	Context string `toml:"context"`
}

// Chart is the helm chart and its registry.
type Chart struct {
	Path     string `toml:"path"`
	Registry string `toml:"registry"`
}

// Target is a config repo's overlays: Path has {env}.
type Target struct {
	Repo     string `toml:"repo"`
	Path     string `toml:"path"`
	Registry string `toml:"registry"`
}

// Followed is the environment following the trunk: follow, else dev
// when listed.
func (r Config) Followed() string {
	if r.Follow != nil {
		return *r.Follow
	}
	if slices.Contains(r.Environments, "dev") {
		return "dev"
	}
	return ""
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
