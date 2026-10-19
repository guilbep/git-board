package release

import (
	"cmp"
	"context"
	"errors"
	"time"

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

// base is the trunk branch, e.g. dev.
func (r *Runner) base() string { return cmp.Or(r.Base, r.Branches.Base, "dev") }

// trunk is the trunk on the authority remote, e.g. origin/dev.
func (r *Runner) trunk() string { return r.Cfg.RemoteName() + "/" + r.base() }

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

// reconcileGrace is how long Reconcile leaves a record-less tag to the cut
// that pushed it.
// ponytail: a heuristic for a cut in flight; the upgrade is a lease the cut
// writes, which Reconcile honours
const reconcileGrace = time.Hour

// Reconcile records every release tag's commit that has no record yet, and
// pushes the records, so one an earlier push missed goes too. Offline it
// does nothing: without the pull it would duplicate other clones' records.
// A tag younger than reconcileGrace is left to its cut, which may still be
// building: the cut's own record has the chart and the notes. A commit's
// artifacts are each image's digest on each deploy registry (see artifact);
// one the registry lacks is left out, even all of them. Previous is the
// newest final before the tag recorded.
func (r *Runner) Reconcile(ctx context.Context) error {
	if r.Offline {
		return nil
	}
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
	for i, t := range tags { // newest first
		if recorded[t.Commit] || time.Since(t.Date) < reconcileGrace {
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
				digest, ok, err := r.artifact(ctx, d.Registry+"/"+img.Name, t)
				if err != nil {
					return err
				}
				if ok {
					rec.Artifacts = append(rec.Artifacts, gitbugio.Artifact{Image: img.Name, Registry: d.Registry, Digest: digest})
				}
			}
		}
		if _, err := r.B.CreateRelease(rec); err != nil {
			return err
		}
	}
	return r.B.PushReleases(r.Cfg.RemoteName())
}

// artifact is the digest of image (<registry>/<name>) for tag t: under the
// tag, unchecked (reuse_unchanged_images re-tagged unchanged images, so a
// legacy tag can name an older build), else under t's commit when its
// revision label is that commit (a cut's tag is pushed before the images
// are tagged). ok is false when neither has it.
func (r *Runner) artifact(ctx context.Context, image string, t Tag) (digest string, ok bool, err error) {
	digest, _, err = r.Tools.Revision(ctx, image+":"+t.Name)
	if !errors.Is(err, ErrNoImage) {
		return digest, err == nil, err
	}
	return r.builtFor(ctx, image+":"+t.Commit, t.Commit)
}

// builtFor returns ref's digest and whether ref was built from commit (its
// revision label is commit); ok is false when the registry lacks ref.
func (r *Runner) builtFor(ctx context.Context, ref, commit string) (digest string, ok bool, err error) {
	digest, rev, err := r.Tools.Revision(ctx, ref)
	if errors.Is(err, ErrNoImage) {
		return "", false, nil
	}
	return digest, err == nil && rev == commit, err
}
