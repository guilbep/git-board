package release

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
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
	CacheDir string       // where the config repos are cloned
	Who      string       // login for commit messages
	Step     func(Step)   // may be nil
	OnTag    func(string) // may be nil: told the tag a cut or promote pushes, or tries to, once it knows it
	Offline  bool         // no fetch of the app remote or config repos
}

// New is the Runner of the app repo at dir: cfg's [release] table and
// branches, the real Tools, the config repos cloned under the git common
// dir, and the git-bug user (else git's user.name) as Who.
func New(cfg *config.Config, dir string, b *gitbugio.Backend) (*Runner, error) {
	ctx := context.Background()
	common, err := Git(ctx, dir, "rev-parse", "--git-common-dir")
	if err != nil {
		return nil, err
	}
	if !filepath.IsAbs(common) {
		common = filepath.Join(dir, common)
	}
	who := ""
	if me, ok, err := b.Me(); err != nil {
		return nil, err
	} else if ok {
		who = me.Login
	}
	if who == "" {
		who, _ = Git(ctx, dir, "config", "user.name") // ponytail: unset is fine, commits just say no one
	}
	return &Runner{
		Cfg: cfg.Release, Branches: cfg.Branches, Dir: dir, B: b, Tools: Exec(),
		CacheDir: filepath.Join(common, "git-board", "release"), Who: who,
	}, nil
}

// base is the trunk branch, e.g. dev.
func (r *Runner) base() string { return cmp.Or(r.Base, r.Branches.Base, "dev") }

// BaseBranch is the trunk branch, e.g. dev.
func (r *Runner) BaseBranch() string { return r.base() }

// trunk is the trunk on the authority remote, e.g. origin/dev.
func (r *Runner) trunk() string { return r.Cfg.RemoteName() + "/" + r.base() }

// fetch brings the authority remote's branches, tags and release records,
// unless Offline.
func (r *Runner) fetch(ctx context.Context) error {
	if r.Offline {
		return nil
	}
	if _, err := Git(ctx, r.Dir, "fetch", "--tags", r.Cfg.RemoteName()); err != nil {
		return fmt.Errorf("%w: %w", ErrFetch, err)
	}
	if err := r.B.PullReleases(r.Cfg.RemoteName()); err != nil {
		return fmt.Errorf("%w: %w", ErrFetch, err)
	}
	return nil
}

// ErrFetch is a failed fetch of the authority remote: no network, often.
var ErrFetch = errors.New("fetching the remote")

// reconcileGrace is how long Reconcile leaves a record-less tag to the cut
// that pushed it.
// ponytail: a heuristic for a cut in flight; the upgrade is a lease the cut
// writes, which Reconcile honours
const reconcileGrace = time.Hour

// Reconcile pushes and finishes the cut made offline, if any (see
// pushPending), records every release tag's commit that has no record yet,
// and pushes the records, so one an earlier push missed goes too. Offline
// it does nothing: without the pull it would duplicate other clones'
// records.
// A tag younger than reconcileGrace is left to its cut, which may still be
// building: the cut's own record has the chart and the notes. A commit's
// artifacts are each image's digest on each deploy registry (see artifact);
// one the registry lacks is left out, even all of them. Previous is the
// newest final before the tag recorded.
func (r *Runner) Reconcile(ctx context.Context) error {
	_, _, err := r.reconcile(ctx)
	return err
}

// reconcile is Reconcile, returning apart the cut made offline it pushed
// and that cut's failure (see pushPending), which Reconcile only says.
func (r *Runner) reconcile(ctx context.Context) (pushed []Tag, pendErr, err error) {
	if r.Offline {
		return nil, nil, nil
	}
	if err := r.fetch(ctx); err != nil {
		return nil, nil, err
	}
	pushed, pendErr = r.pushPending(ctx)
	tags, err := ReleaseTags(ctx, r.Dir)
	if err != nil {
		return pushed, pendErr, err
	}
	recs, err := r.B.Releases()
	if err != nil {
		return pushed, pendErr, err
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
			var seen []string // targets may share a registry
			for _, d := range r.Cfg.Deploy {
				if slices.Contains(seen, d.Registry) {
					continue
				}
				seen = append(seen, d.Registry)
				digest, ok, err := r.artifact(ctx, d.Registry+"/"+img.Name, t)
				if err != nil {
					return pushed, pendErr, err
				}
				if ok {
					rec.Artifacts = append(rec.Artifacts, gitbugio.Artifact{Image: img.Name, Registry: d.Registry, Digest: digest})
				}
			}
		}
		if _, err := r.B.CreateRelease(rec); err != nil {
			return pushed, pendErr, err
		}
	}
	return pushed, pendErr, r.B.PushReleases(r.Cfg.RemoteName())
}

// pushPending pushes the cut made offline, if any, and finishes it as a
// cut would, with the notes drafted now. It checks it again first, on what
// was just fetched: one refused, its name taken included, is dropped; one
// that fails to be checked or pushed is kept for the next online command.
// Once its tag is pushed, a failure to finish is left to a deploy of it or
// a Reconcile past the grace, as for a cut. Each is said in a step, and
// the first failure is returned for an online Cut, which was that cut:
// nothing else stops on it.
func (r *Runner) pushPending(ctx context.Context) ([]Tag, error) {
	pending, err := Pending(ctx, r.Dir)
	if err != nil || len(pending) == 0 {
		return nil, err
	}
	st, repos, err := r.status(ctx)
	if err != nil {
		return nil, err
	}
	var pushed []Tag
	var failed error
	say := func(f string, a ...any) { _ = r.do("tag", func() (string, error) { return fmt.Sprintf(f, a...), nil }) }
	for _, t := range pending {
		where, err := r.check(ctx, st, repos, Cut{}, t, false)
		if err == nil {
			err = r.push(ctx, t, ", cut offline")
		}
		var no refused
		if err != nil && !errors.Is(err, ErrTaken) && !errors.As(err, &no) {
			say("kept %s, cut offline: %s; the next online command tries again", t.Name, err)
			failed = cmp.Or(failed, fmt.Errorf("%s, cut offline: %w", t.Name, err))
			continue
		}
		if _, derr := Git(ctx, r.Dir, "update-ref", "-d", pendingRefs+t.Name); derr != nil {
			return pushed, cmp.Or(failed, derr)
		}
		if err != nil {
			why := err.Error()
			if errors.Is(err, ErrTaken) {
				why = "another clone took it"
			}
			say("dropped %s, cut offline: %s; cut again", t.Name, why)
			continue
		}
		pushed = append(pushed, t)
		d, err := r.draft(ctx, st, Draft{From: t.Commit}, t.Version.RC == 0)
		if err == nil {
			err = r.finish(ctx, t, d, where, "")
		}
		if err != nil {
			say("pushed %s, cut offline, but didn't finish: %s; a deploy of it finishes it", t.Name, err)
			failed = cmp.Or(failed, err)
		}
	}
	return pushed, failed
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
