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
	Commit, Version string // the full sha; the version as written, e.g. v1.4.20-rc.2
	Rollback        bool   // may move back
	Hotfix          bool   // may move off the running line, except in the last environment
}

// Deploy makes env run t: the images of t.Commit, built once (or reused
// from the registry) and tagged t.Version, the chart packaged at
// t.Version, then each target's overlay, one config repo after the other.
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
	repos, err := r.configRepos(ctx)
	if err != nil {
		return err
	}
	chart := ""
	if r.Cfg.Chart != nil {
		chart = r.Cfg.Chart.Registry
	}
	path := func(pattern string) string { return strings.ReplaceAll(pattern, "{env}", env) }
	moving, listed := 0, map[string]bool{}
	for _, d := range r.Cfg.Deploy {
		o, err := repos[d.Repo].Read(ctx, path(d.Path))
		if err != nil {
			return fmt.Errorf("%s on %s: %w", env, d.Repo, err)
		}
		dep := o.Deployed(chart)
		move, err := r.moves(ctx, env, d.Repo, dep, t)
		if err != nil {
			return err
		}
		if move {
			moving++
		}
		for _, img := range r.Cfg.Images {
			if _, ok := dep.Images[d.Registry+"/"+img.Name]; ok {
				listed[img.Name] = true
			}
		}
	}
	for _, img := range r.Cfg.Images {
		if !listed[img.Name] {
			return fmt.Errorf("image %s is in no overlay of %s", img.Name, env)
		}
	}
	if moving == 0 {
		return nil
	}
	arts, err := r.ensureImages(ctx, t.Commit)
	if err != nil {
		return err
	}
	for _, a := range arts {
		if err := r.Tools.Tag(ctx, a.Registry+"/"+a.Image+":"+t.Commit, t.Version); err != nil {
			return err
		}
	}
	if r.Cfg.Chart != nil {
		if err := r.ensureChart(ctx, t.Commit, t.Version); err != nil {
			return err
		}
	}
	for _, d := range r.Cfg.Deploy {
		p := path(d.Path)
		app := p
		if i := strings.LastIndex(p, "/overlays/"+env); i >= 0 {
			app = p[:i]
		}
		msg := fmt.Sprintf("chore(manifest): update %s (%s) %s [by %s via git-board]", app, env, t.Version, r.Who)
		err := r.do(env+" on "+d.Repo, func() (string, error) {
			// Update re-runs this on a fresh fetch: the checks hold for what it writes on.
			commit, err := repos[d.Repo].Update(ctx, p, msg, func(o *Overlay) (bool, error) {
				dep := o.Deployed(chart)
				if move, err := r.moves(ctx, env, d.Repo, dep, t); err != nil || !move {
					return false, err
				}
				want := Deployed{Commit: t.Commit, Version: t.Version, Images: map[string]Image{}}
				for _, a := range arts {
					name := a.Registry + "/" + a.Image
					if _, ok := dep.Images[name]; ok && a.Registry == d.Registry {
						want.Images[name] = Image{Tag: t.Version, Digest: a.Digest}
					}
				}
				return o.Set(want, chart, r.Cfg.VersionValues)
			})
			if commit == "" {
				return "unchanged", err
			}
			return "pushed " + commit[:7], err
		})
		if err != nil {
			return err
		}
	}
	return nil
}

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

// ensureImages makes each image of commit exist on every deploy registry as
// <registry>/<image>:<commit>, labelled with commit. One already there is
// kept. One missing from the first registry is built there from a checkout
// of commit and scanned; then it is copied wherever it is missing. It
// returns each image's digest on each registry, image by image.
func (r *Runner) ensureImages(ctx context.Context, commit string) ([]gitbugio.Artifact, error) {
	var regs []string
	for _, d := range r.Cfg.Deploy {
		if !slices.Contains(regs, d.Registry) {
			regs = append(regs, d.Registry)
		}
	}
	if len(regs) == 0 {
		return nil, errors.New("no deploy target: nowhere to push images")
	}
	tree := "" // the checkout of commit, made for the first build
	defer func() {
		if tree != "" {
			r.removeCheckout(tree)
		}
	}()
	var arts []gitbugio.Artifact
	for _, img := range r.Cfg.Images {
		err := r.do(img.Name, func() (string, error) {
			src := regs[0] + "/" + img.Name + ":" + commit
			digest, ok, err := r.builtFor(ctx, src, commit)
			if err != nil {
				return "", err
			}
			detail := "reused " + digest
			if !ok {
				if tree == "" {
					if tree, err = r.checkout(ctx, commit); err != nil {
						return "", err
					}
				}
				b := Build{Context: filepath.Join(tree, img.Context), Ref: src, Revision: commit, Builder: r.Cfg.Builder}
				if digest, err = r.Tools.Build(ctx, b); err != nil {
					return "", err
				}
				if err := r.Tools.Scan(ctx, src); err != nil {
					return "", fmt.Errorf("%s: %w", img.Name, err)
				}
				detail = "built " + digest
			}
			arts = append(arts, gitbugio.Artifact{Image: img.Name, Registry: regs[0], Digest: digest})
			for _, reg := range regs[1:] {
				dst := reg + "/" + img.Name + ":" + commit
				d, ok, err := r.builtFor(ctx, dst, commit)
				if err != nil {
					return "", err
				}
				if !ok {
					if err := r.Tools.Copy(ctx, src, dst); err != nil {
						return "", err
					}
					d = digest
				}
				arts = append(arts, gitbugio.Artifact{Image: img.Name, Registry: reg, Digest: d})
			}
			return detail, nil
		})
		if err != nil {
			return nil, err
		}
	}
	return arts, nil
}

// ensureChart packages the chart of commit at version and pushes it.
func (r *Runner) ensureChart(ctx context.Context, commit, version string) error {
	return r.do("chart", func() (string, error) {
		tree, err := r.checkout(ctx, commit)
		if err != nil {
			return "", err
		}
		defer r.removeCheckout(tree)
		file, err := r.Tools.PackageChart(ctx, filepath.Join(tree, r.Cfg.Chart.Path), version, tree)
		if err != nil {
			return "", err
		}
		digest, err := r.Tools.PushChart(ctx, file, r.Cfg.Chart.Registry)
		return "pushed " + version + " " + digest, err
	})
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
