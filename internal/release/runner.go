package release

import (
	"cmp"
	"context"
	"errors"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

// Step is one step of a release job, as it is reported.
type Step struct {
	Name   string `json:"name"`
	Detail string `json:"detail"`
	State  string `json:"state"` // running, done, failed
}

// Runner reads and moves an app's releases.
type Runner struct {
	Cfg      config.Release
	Branches config.Branches
	Base     string            // the trunk branch; "": Branches.Base, else dev
	Dir      string            // the app repo's work tree
	B        *gitbugio.Backend // the app repo's, for the release records
	Tools    Tools
	CacheDir string     // where the config repos are cloned
	Who      string     // login for commit messages
	Step     func(Step) // may be nil
	Offline  bool       // no fetch of the app remote or config repos
}

// trunk is the trunk on the authority remote, e.g. origin/dev.
func (r *Runner) trunk() string {
	return r.Cfg.RemoteName() + "/" + cmp.Or(r.Base, r.Branches.Base, "dev")
}

// fetch brings the authority remote's branches, tags and release records,
// unless Offline.
func (r *Runner) fetch(ctx context.Context) error {
	if r.Offline {
		return nil
	}
	if _, err := Git(ctx, r.Dir, "fetch", "--tags", r.Cfg.RemoteName()); err != nil {
		return err
	}
	return r.B.PullReleases(r.Cfg.RemoteName())
}

// Reconcile records every release tag's commit that has no record yet, and
// pushes the records it makes. A commit's artifacts are each image's digest
// on each deploy registry under its newest tag; an image the registry lacks
// is left out. Previous is the newest final before that tag.
func (r *Runner) Reconcile(ctx context.Context) error {
	if err := r.fetch(ctx); err != nil {
		return err
	}
	tags, err := ReleaseTags(ctx, r.Dir)
	if err != nil {
		return err
	}
	recs, err := r.B.Releases()
	if err != nil {
		return err
	}
	recorded := map[string]bool{}
	for _, rec := range recs {
		recorded[rec.Commit] = true
	}
	made := false
	for i, t := range tags { // newest first
		if recorded[t.Commit] {
			continue
		}
		recorded[t.Commit] = true
		rec := gitbugio.ReleaseRecord{Commit: t.Commit}
		for _, p := range tags[i+1:] {
			if p.Version.RC == 0 {
				rec.Previous = p.Name
				break
			}
		}
		for _, img := range r.Cfg.Images {
			for _, d := range r.Cfg.Deploy {
				// no revision check: reuse_unchanged_images re-tagged
				// unchanged images, so a legacy tag can name an older build
				digest, _, err := r.Tools.Revision(ctx, d.Registry+"/"+img.Name+":"+t.Name)
				if errors.Is(err, ErrNoImage) {
					continue
				}
				if err != nil {
					return err
				}
				rec.Artifacts = append(rec.Artifacts, gitbugio.Artifact{Image: img.Name, Registry: d.Registry, Digest: digest})
			}
		}
		if _, err := r.B.CreateRelease(rec); err != nil {
			return err
		}
		made = true
	}
	if !made {
		return nil
	}
	return r.B.PushReleases(r.Cfg.RemoteName())
}
