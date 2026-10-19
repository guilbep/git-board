package release

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/gitbugio"
)

var (
	// ErrNotAdopted means an overlay has no git-board/commit yet.
	ErrNotAdopted = errors.New("not adopted yet")
	// ErrOlder means an environment would move to a commit that doesn't
	// descend from the one it runs.
	ErrOlder = errors.New("would move back")
)

// Target is what an environment is to run.
type Target struct {
	Commit, Version string // any name of the commit; the version as written, e.g. v1.4.20-rc.2
	Rollback        bool   // may move back
	Hotfix          bool   // may move off the running line, except in the last environment
}

// Deploy makes env run t: the images of t.Commit, by the digests its
// release record holds, else built once (or reused from the registry) and
// tagged t.Version; the chart at t.Version (left alone for a record without
// one); then each target's overlay, one config repo after the other.
// Before building anything it refuses an overlay not adopted (ErrNotAdopted)
// or running a commit t.Commit doesn't descend from (ErrOlder, unless
// t.Rollback, or t.Hotfix and env isn't the last). A target already running
// t is left alone, so a Deploy that stopped half way is finished by the next.
func (r *Runner) Deploy(ctx context.Context, env string, t Target) error {
	if err := r.Reconcile(ctx); err != nil {
		return err
	}
	return r.deploy(ctx, env, t)
}

// DeployFollowed deploys the trunk's head to the followed environment, at
// its dev version.
func (r *Runner) DeployFollowed(ctx context.Context) error {
	env := r.Cfg.Followed()
	if env == "" {
		return errors.New("no environment follows the trunk")
	}
	if err := r.Reconcile(ctx); err != nil {
		return err
	}
	head, err := Git(ctx, r.Dir, "rev-parse", r.trunk())
	if err != nil {
		return err
	}
	version, err := r.devVersion(ctx, head)
	if err != nil {
		return err
	}
	return r.deploy(ctx, env, Target{Commit: head, Version: version})
}

// devVersion is commit's dev version: past the newest final release tag it
// contains, counting its commits since the newest release tag it contains.
func (r *Runner) devVersion(ctx context.Context, commit string) (string, error) {
	tags, err := ReleaseTags(ctx, r.Dir)
	if err != nil {
		return "", err
	}
	var final Version
	revs := commit // the commits counted: all of them, until a release tag is found
	// tags are newest first
	for _, t := range tags {
		ok, err := IsAncestor(ctx, r.Dir, t.Commit, commit)
		if err != nil {
			return "", err
		}
		if !ok {
			continue
		}
		if revs == commit {
			revs = t.Commit + ".." + commit
		}
		if t.Version.RC == 0 {
			final = t.Version
			break
		}
	}
	n, err := Git(ctx, r.Dir, "rev-list", "--count", revs)
	if err != nil {
		return "", err
	}
	since, err := strconv.Atoi(n)
	if err != nil {
		return "", err
	}
	return DevVersion(final, since, commit), nil
}

func (r *Runner) deploy(ctx context.Context, env string, t Target) error {
	full, err := Git(ctx, r.Dir, "rev-parse", "--verify", t.Commit+"^{commit}")
	if err != nil {
		return err
	}
	t.Commit = full
	repos, err := r.configRepos(ctx)
	if err != nil {
		return err
	}
	path := func(pattern string) string { return strings.ReplaceAll(pattern, "{env}", env) }
	moving, where := 0, map[string][]string{}
	for _, d := range r.Cfg.Deploy {
		o, err := repos[d.Repo].Read(ctx, path(d.Path))
		if err != nil {
			return fmt.Errorf("%s on %s: %w", env, d.Repo, err)
		}
		dep := o.Deployed("")
		move, err := r.moves(ctx, env, d.Repo, dep, t)
		if err != nil {
			return err
		}
		if move {
			moving++
		}
		for _, img := range r.Cfg.Images {
			if _, ok := dep.Images[d.Registry+"/"+img.Name]; ok && !slices.Contains(where[img.Name], d.Registry) {
				where[img.Name] = append(where[img.Name], d.Registry)
			}
		}
	}
	for _, img := range r.Cfg.Images {
		if len(where[img.Name]) == 0 {
			return fmt.Errorf("image %s is in no overlay of %s", img.Name, env)
		}
	}
	if moving == 0 {
		return nil
	}
	rec, recorded, err := r.B.ReleaseFor(t.Commit)
	if err != nil {
		return err
	}
	arts, err := r.ensureImages(ctx, t, rec.Artifacts, where)
	if err != nil {
		return err
	}
	chart, chartVersion := "", "" // the chart entry, and its version: "" leaves it as it is
	if r.Cfg.Chart != nil {
		chart = r.Cfg.Chart.Registry
		switch {
		case recorded && rec.Chart.Version == "": // a release from before git-board: its chart is unknown
		case recorded && rec.Chart.Version == t.Version:
			chartVersion = t.Version
		default:
			if _, err := r.ensureChart(ctx, t.Commit, t.Version); err != nil {
				return err
			}
			chartVersion = t.Version
		}
	}
	for _, d := range r.Cfg.Deploy {
		p := path(d.Path)
		msg := fmt.Sprintf("chore(manifest): update %s (%s) %s [by %s via git-board]", appOf(p, env), env, t.Version, r.Who)
		err := r.do(env+" on "+d.Repo, func() (string, error) {
			// Update re-runs this on a fresh fetch: the checks hold for what it writes on.
			commit, err := repos[d.Repo].Update(ctx, p, msg, func(o *Overlay) (bool, error) {
				dep := o.Deployed("")
				if move, err := r.moves(ctx, env, d.Repo, dep, t); err != nil || !move {
					return false, err
				}
				want := Deployed{Commit: t.Commit, Version: t.Version, Chart: chartVersion, Images: map[string]Image{}}
				for _, a := range arts {
					name := a.Registry + "/" + a.Image
					if _, ok := dep.Images[name]; ok && a.Registry == d.Registry {
						want.Images[name] = Image{Tag: t.Version, Digest: a.Digest}
					}
				}
				return o.Set(want, chart, r.Cfg.VersionValues)
			})
			switch {
			case err != nil:
				return "", fmt.Errorf("%s on %s: %w", env, d.Repo, err)
			case commit == "":
				return "unchanged", nil
			}
			return "pushed " + commit[:7], nil
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// appOf is the app an overlay path names, for commit messages: the path
// without its trailing /overlays/<env>.
func appOf(path, env string) string { return strings.TrimSuffix(path, "/overlays/"+env) }

// moves reports whether an overlay running dep is to move to t, refusing
// one not adopted and, unless t allows it, a move back.
func (r *Runner) moves(ctx context.Context, env, repo string, dep Deployed, t Target) (bool, error) {
	switch {
	case dep.Commit == "":
		return false, fmt.Errorf("%s on %s: %w", env, repo, ErrNotAdopted)
	case dep.Commit == t.Commit && dep.Version == t.Version:
		return false, nil
	case t.Rollback, t.Hotfix && env != r.Cfg.Last():
		return true, nil
	}
	ok, err := IsAncestor(ctx, r.Dir, dep.Commit, t.Commit)
	if err == nil && !ok {
		err = ErrOlder
	}
	if err != nil {
		return false, fmt.Errorf("%s on %s: %w", env, repo, err)
	}
	return true, nil
}

// ensureImages makes each image of t.Commit exist on the registries where
// lists for it, each tagged t.Version, and returns its digest on each, image
// by image. A registry's digest is the release record's (recorded), else
// that of <registry>/<image>:<commit> when built from the commit. When no
// registry has one, the image is built on the first; a registry still
// lacking it gets a copy.
func (r *Runner) ensureImages(ctx context.Context, t Target, recorded []gitbugio.Artifact, where map[string][]string) ([]gitbugio.Artifact, error) {
	tree := "" // the checkout of the commit, made for the first build
	defer func() {
		if tree != "" {
			r.removeCheckout(tree)
		}
	}()
	var arts []gitbugio.Artifact
	for _, img := range r.Cfg.Images {
		regs := where[img.Name]
		err := r.do(img.Name, func() (string, error) {
			digests := map[string]string{}
			src, digest, detail := "", "", ""
			for _, reg := range regs {
				d, err := r.existing(ctx, recorded, img.Name, reg, t.Commit)
				if err != nil {
					return "", fmt.Errorf("%s: %w", img.Name, err)
				}
				if d != "" && src == "" {
					src, digest, detail = reg+"/"+img.Name+"@"+d, d, "reused "+d
				}
				digests[reg] = d
			}
			if src == "" {
				if tree == "" {
					var err error
					if tree, err = r.checkout(ctx, t.Commit); err != nil {
						return "", err
					}
				}
				d, err := r.build(ctx, img, regs[0], t.Commit, tree)
				if err != nil {
					return "", fmt.Errorf("%s: %w", img.Name, err)
				}
				src, digest, detail = regs[0]+"/"+img.Name+"@"+d, d, "built "+d
				digests[regs[0]] = d
			}
			for _, reg := range regs {
				if digests[reg] == "" {
					if err := r.Tools.Copy(ctx, src, reg+"/"+img.Name+":"+t.Commit); err != nil {
						return "", fmt.Errorf("%s: %w", img.Name, err)
					}
					digests[reg] = digest
				}
				if err := r.Tools.Tag(ctx, reg+"/"+img.Name+"@"+digests[reg], t.Version); err != nil {
					return "", fmt.Errorf("%s: %w", img.Name, err)
				}
				arts = append(arts, gitbugio.Artifact{Image: img.Name, Registry: reg, Digest: digests[reg]})
			}
			return detail, nil
		})
		if err != nil {
			return nil, err
		}
	}
	return arts, nil
}

// existing is img's digest on reg for commit: the record's, else that of
// <reg>/<img>:<commit> when built from commit, else "".
func (r *Runner) existing(ctx context.Context, recorded []gitbugio.Artifact, img, reg, commit string) (string, error) {
	for _, a := range recorded {
		if a.Image == img && a.Registry == reg {
			return a.Digest, nil
		}
	}
	d, ok, err := r.builtFor(ctx, reg+"/"+img+":"+commit, commit)
	if !ok {
		return "", err
	}
	return d, nil
}

// build builds img from tree on reg under the scratch tag scan-<commit>,
// scans it, and only when the scan passes tags it <commit>: an image that
// failed its scan is never found as built.
func (r *Runner) build(ctx context.Context, img config.Image, reg, commit, tree string) (string, error) {
	scratch := reg + "/" + img.Name + ":scan-" + commit
	digest, err := r.Tools.Build(ctx, Build{Context: filepath.Join(tree, img.Context), Ref: scratch, Revision: commit, Builder: r.Cfg.Builder})
	if err != nil {
		return "", err
	}
	if err := r.Tools.Scan(ctx, scratch); err != nil {
		return "", err
	}
	return digest, r.Tools.Tag(ctx, scratch, commit)
}

// ensureChart packages the chart of commit at version, pushes it and
// returns its digest.
func (r *Runner) ensureChart(ctx context.Context, commit, version string) (digest string, err error) {
	err = r.do("chart", func() (string, error) {
		tree, err := r.checkout(ctx, commit)
		if err != nil {
			return "", err
		}
		defer r.removeCheckout(tree)
		file, err := r.Tools.PackageChart(ctx, filepath.Join(tree, r.Cfg.Chart.Path), version, tree)
		if err != nil {
			return "", err
		}
		digest, err = r.Tools.PushChart(ctx, file, r.Cfg.Chart.Registry)
		return "pushed " + version + " " + digest, err
	})
	return digest, err
}

// checkout puts commit in a scratch worktree of Dir, so what is built is the
// commit, whatever Dir has checked out.
func (r *Runner) checkout(ctx context.Context, commit string) (string, error) {
	dir, err := os.MkdirTemp("", "git-board-release-")
	if err != nil {
		return "", err
	}
	if _, err := Git(ctx, r.Dir, "worktree", "add", "--detach", dir, commit); err != nil {
		_ = os.RemoveAll(dir)
		return "", err
	}
	return dir, nil
}

func (r *Runner) removeCheckout(dir string) {
	_, _ = Git(context.Background(), r.Dir, "worktree", "remove", "--force", dir)
	_ = os.RemoveAll(dir)
}

// do reports f as the step name: running, then done with f's detail, or
// failed with its error.
func (r *Runner) do(name string, f func() (string, error)) error {
	report := func(detail, state string) {
		if r.Step != nil {
			r.Step(Step{Name: name, Detail: detail, State: state})
		}
	}
	report("", "running")
	detail, err := f()
	if err != nil {
		report(err.Error(), "failed")
		return err
	}
	report(detail, "done")
	return nil
}
