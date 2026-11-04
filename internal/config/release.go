package config

import (
	"fmt"
	"slices"
	"strings"

	"github.com/guilbep/git-board/core/release"
)

// checkRelease refuses a table that can't name its environments, or
// deploys somewhere the same for all of them.
func checkRelease(r release.Config) error {
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
