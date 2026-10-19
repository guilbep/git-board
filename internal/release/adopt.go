package release

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/guilbep/git-board/internal/config"
)

// Proposal is the commit an overlay is thought to run, for Adopt.
type Proposal struct {
	Env, Repo, Path string
	Runs            string // newTag, plus the first image's digest prefix when newTag isn't a release tag
	Commit          string // "" when unknown
	From            string // "tag", "label", ""
}

// firstImage is the overlay's first images[] entry: its name, newTag, digest.
func (o *Overlay) firstImage() (name, tag, digest string) {
	if imgs := items(get(o.root(), "images")); len(imgs) > 0 {
		return scalar(imgs[0], "name"), scalar(imgs[0], "newTag"), scalar(imgs[0], "digest")
	}
	return "", "", ""
}

// Proposals proposes, for each deploy target's overlay of each environment
// not carrying git-board/commit yet, the commit it runs: the release tag
// its first image's newTag names, else that image's revision label. A
// commit Dir doesn't have is left out (Commit "").
func (r *Runner) Proposals(ctx context.Context) ([]Proposal, error) {
	if err := r.fetch(ctx); err != nil {
		return nil, err
	}
	repos, err := r.configRepos(ctx)
	if err != nil {
		return nil, err
	}
	tags, err := ReleaseTags(ctx, r.Dir)
	if err != nil {
		return nil, err
	}
	var ps []Proposal
	for _, env := range r.Cfg.Environments {
		for _, d := range r.Cfg.Deploy {
			path := strings.ReplaceAll(d.Path, "{env}", env)
			o, err := repos[d.Repo].Read(ctx, path)
			if err != nil {
				return nil, fmt.Errorf("%s on %s: %w", env, d.Repo, err)
			}
			if o.Deployed("").Commit != "" {
				continue
			}
			name, tag, digest := o.firstImage()
			p := Proposal{Env: env, Repo: d.Repo, Path: path, Runs: tag}
			if _, isTag := ParseTag(tag); !isTag && digest != "" {
				hex := strings.TrimPrefix(digest, "sha256:")
				p.Runs += " · sha256:" + hex[:min(6, len(hex))]
			}
			for _, t := range tags {
				if t.Name == tag {
					p.Commit, p.From = t.Commit, "tag"
				}
			}
			if p.Commit == "" && name != "" && tag != "" {
				ref := name + ":" + tag
				if digest != "" {
					ref = name + "@" + digest
				}
				switch _, rev, err := r.Tools.Revision(ctx, ref); {
				case errors.Is(err, ErrNoImage):
				case err != nil:
					return nil, fmt.Errorf("%s on %s: %s: %w", env, d.Repo, name, err)
				case rev != "":
					if full, err := Git(ctx, r.Dir, "rev-parse", "--verify", rev+"^{commit}"); err == nil {
						p.Commit, p.From = full, "label"
					}
				}
			}
			ps = append(ps, p)
		}
	}
	return ps, nil
}

// Adopt marks each proposal's overlay as running its Commit, writing only
// git-board/commit and git-board/version (the overlay's first image's
// newTag). Every Commit is checked before anything is written; an overlay
// adopted meanwhile is left alone.
func (r *Runner) Adopt(ctx context.Context, ps []Proposal) error {
	if err := r.online("adopt"); err != nil {
		return err
	}
	fulls := make([]string, len(ps))
	for i, p := range ps {
		if !slices.ContainsFunc(r.Cfg.Deploy, func(d config.Target) bool {
			return d.Repo == p.Repo && strings.ReplaceAll(d.Path, "{env}", p.Env) == p.Path
		}) || !slices.Contains(r.Cfg.Environments, p.Env) {
			return fmt.Errorf("%s on %s: adopt: %s is not a deploy target", p.Env, p.Repo, p.Path)
		}
		full, err := Git(ctx, r.Dir, "rev-parse", "--verify", p.Commit+"^{commit}")
		if err != nil || p.Commit == "" {
			return fmt.Errorf("%s on %s: adopt: no commit %q in the app repo", p.Env, p.Repo, p.Commit)
		}
		fulls[i] = full
	}
	repos, err := r.configRepos(ctx)
	if err != nil {
		return err
	}
	for i, p := range ps {
		c := repos[p.Repo]
		msg := fmt.Sprintf("chore(manifest): adopt %s (%s) [by %s via git-board]", appOf(p.Path, p.Env), p.Env, r.Who)
		err := r.do(p.Env+" on "+p.Repo, func() (string, error) {
			commit, err := c.Update(ctx, p.Path, msg, func(o *Overlay) (bool, error) {
				if o.Deployed("").Commit != "" {
					return false, nil
				}
				_, tag, _ := o.firstImage()
				return o.Set(Deployed{Commit: fulls[i], Version: tag}, "", nil)
			})
			switch {
			case err != nil:
				return "", fmt.Errorf("%s on %s: %w", p.Env, p.Repo, err)
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
