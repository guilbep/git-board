package release

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/guilbep/git-board/internal/gitbugio"
)

// ErrTaken means the tag a cut would push exists already: another clone
// cut it first.
var ErrTaken = errors.New("tag taken")

// ErrNoTag means no release tag has the name asked for.
var ErrNoTag = errors.New("no release tag")

// Cut is what to cut.
type Cut struct {
	Version Version // final form; zero: the draft's proposal
	// Final cuts the final rather than the next candidate; Hotfix may leave
	// the line the last environment runs; Deploy deploys once cut.
	Final, Hotfix, Deploy bool
	Commit                string // any name of it; "": the draft's From
	Notes                 string // "": the draft's
}

// Cut tags c.Commit with c.Version's next candidate, or its final, and
// makes the release: the tag pushed first, then the images ensured and
// tagged with it, the chart at it and the record with its notes; then,
// with c.Deploy, it deploys to the first candidate environment, or the
// last for a final. A commit with a record keeps it, and its notes but for
// c.Notes; its chart at the tag is left to the deploy, which packages it.
//
// Before writing anything it refuses: offline; a dev-only app; a commit
// that doesn't contain what the last environment runs, and a final when no
// candidate on the commit ran in the environment before last (both unless
// c.Hotfix); a yanked commit; with c.Deploy, an environment that isn't
// adopted (ErrNotAdopted) or wouldn't move forward (ErrOlder; a hotfix may
// leave the line of an environment but the last). A tag another clone has
// is ErrTaken, and nothing else is done. Once the tag is pushed it is
// returned, even with a later error: a Deploy finishes the images, the
// chart and the environment, and a Reconcile past its grace the record.
func (r *Runner) Cut(ctx context.Context, c Cut) (Tag, error) {
	if err := cmp.Or(r.online("a cut"), r.devOnly()); err != nil {
		return Tag{}, err
	}
	if err := r.Reconcile(ctx); err != nil {
		return Tag{}, err
	}
	return r.cut(ctx, c, false)
}

// Promote cuts the final of candidate rcTag on its commit and deploys it to
// the last environment. It ships what ran: an image no registry has for
// the commit is refused before the tag, never built.
func (r *Runner) Promote(ctx context.Context, rcTag string) (Tag, error) {
	if err := cmp.Or(r.online("a promote"), r.devOnly()); err != nil {
		return Tag{}, err
	}
	if err := r.Reconcile(ctx); err != nil {
		return Tag{}, err
	}
	rc, err := r.tagNamed(ctx, rcTag)
	if err != nil {
		return Tag{}, err
	}
	if rc.Version.RC == 0 {
		return Tag{}, fmt.Errorf("%s isn't a candidate", rcTag)
	}
	return r.cut(ctx, Cut{Final: true, Commit: rc.Commit, Version: rc.Version.Final(), Deploy: true}, true)
}

// Rollback makes env run release tag again, back from what it runs, and
// comments on the record of what it ran: "Rolled back from <env> to <tag>:
// <reason>". It ships what ran: an image no registry has for the tag's
// commit is refused, never built. Offline it refuses.
func (r *Runner) Rollback(ctx context.Context, env, tag, reason string) error {
	if err := r.online("a rollback"); err != nil {
		return err
	}
	if err := r.Reconcile(ctx); err != nil {
		return err
	}
	t, err := r.tagNamed(ctx, tag)
	if err != nil {
		return err
	}
	repos, err := r.configRepos(ctx)
	if err != nil {
		return err
	}
	e, err := r.envState(ctx, env, repos)
	if err != nil {
		return err
	}
	where, err := r.where(env, deployedOf(e)...)
	if err != nil {
		return err
	}
	rec, _, err := r.B.ReleaseFor(t.Commit)
	if err != nil {
		return err
	}
	if err := r.built(ctx, t.Commit, rec.Artifacts, where, "rollback"); err != nil {
		return err
	}
	if err := r.deploy(ctx, env, Target{Commit: t.Commit, Version: t.Name, Rollback: true}); err != nil {
		return err
	}
	seen := map[string]bool{t.Commit: true}
	for _, tg := range e.Targets {
		c := tg.Deployed.Commit
		if c == "" || seen[c] {
			continue
		}
		seen[c] = true
		rec, ok, err := r.B.ReleaseFor(c)
		if err != nil {
			return err
		}
		if !ok { // ponytail: no record (a dev build, a tag without images), no comment; the overlay's history has the rollback
			continue
		}
		if err := r.B.CommentRelease(rec.ID, fmt.Sprintf("Rolled back from %s to %s: %s", env, tag, reason)); err != nil {
			return err
		}
	}
	return r.B.PushReleases(r.Cfg.RemoteName())
}

// online refuses what writes to the remotes while Offline.
func (r *Runner) online(what string) error {
	if r.Offline {
		return fmt.Errorf("offline: %s writes to the remotes", what)
	}
	return nil
}

func (r *Runner) tagNamed(ctx context.Context, name string) (Tag, error) {
	tags, err := ReleaseTags(ctx, r.Dir)
	if err != nil {
		return Tag{}, err
	}
	if i := slices.IndexFunc(tags, func(t Tag) bool { return t.Name == name }); i >= 0 {
		return tags[i], nil
	}
	return Tag{}, fmt.Errorf("%w %s", ErrNoTag, name)
}

// cut is Cut after Reconcile; a promote never builds.
func (r *Runner) cut(ctx context.Context, c Cut, promote bool) (Tag, error) {
	st, repos, err := r.status(ctx)
	if err != nil {
		return Tag{}, err
	}
	d, err := r.from(ctx, st)
	if err != nil {
		return Tag{}, err
	}
	if c.Commit != "" {
		full, err := Git(ctx, r.Dir, "rev-parse", "--verify", c.Commit+"^{commit}")
		if err != nil {
			return Tag{}, err
		}
		d = Draft{From: full}
	}
	final := r.final(c.Final)
	if d, err = r.draft(ctx, st, d, final); err != nil {
		return Tag{}, err
	}
	commit, tags := d.From, tagsOf(st)
	v := tagFor(cmp.Or(c.Version, d.Proposed).Final(), final, tags)
	t := Tag{Name: v.Tag(), Commit: commit, Message: v.Tag() + gitBoardMark, Version: v}
	if slices.ContainsFunc(tags, func(o Tag) bool { return o.Name == t.Name }) {
		return Tag{}, fmt.Errorf("%s: %w", t.Name, ErrTaken)
	}
	if err := r.refuse(ctx, st, repos, c, t, tags); err != nil {
		return Tag{}, err
	}
	rec, recorded, err := r.B.ReleaseFor(commit)
	if err != nil {
		return Tag{}, err
	}
	if rec.Yanked != "" {
		return Tag{}, fmt.Errorf("%.7s is yanked: %s", commit, rec.Yanked)
	}
	where, err := r.where("any environment", deployedOf(st.Envs...)...)
	if err != nil {
		return Tag{}, err
	}
	if promote {
		if err := r.built(ctx, commit, rec.Artifacts, where, "promote"); err != nil {
			return Tag{}, err
		}
	}
	if c.Deploy {
		if err := r.takes(ctx, envNamed(st, r.deployTo(final)), commit, c.Hotfix); err != nil {
			return Tag{}, err
		}
	}

	err = r.do("tag", func() (string, error) {
		taken, err := PushTag(ctx, r.Dir, r.Cfg.RemoteName(), t)
		if taken {
			return "", fmt.Errorf("%s: %w", t.Name, ErrTaken)
		}
		return "pushed " + t.Name, err
	})
	if err != nil {
		return Tag{}, err
	}
	arts, err := r.ensureImages(ctx, Target{Commit: commit, Version: t.Name}, rec.Artifacts, where)
	if err != nil {
		return t, err
	}
	var chart gitbugio.ChartArtifact
	if r.Cfg.Chart != nil && !recorded { // R19: a recorded commit's chart is the deploy's to package
		digest, err := r.ensureChart(ctx, commit, t.Name)
		if err != nil {
			return t, err
		}
		chart = gitbugio.ChartArtifact{Version: t.Name, Digest: digest}
	}
	err = r.do("record", func() (string, error) {
		detail := "kept " + rec.ID[:min(7, len(rec.ID))]
		notes := c.Notes
		if !recorded {
			prev := ""
			if d.Previous != nil {
				prev = d.Previous.Name
			}
			created, err := r.B.CreateRelease(gitbugio.ReleaseRecord{Commit: commit, Previous: prev, Artifacts: arts, Chart: chart})
			if err != nil {
				return "", err
			}
			rec, detail, notes = created, "created "+created.ID[:7], cmp.Or(notes, d.Notes)
		}
		if notes != "" && notes != rec.Notes {
			if err := r.B.SetReleaseNotes(rec.ID, notes); err != nil {
				return "", err
			}
			detail += ", notes set"
		}
		return detail + ", pushed", r.B.PushReleases(r.Cfg.RemoteName())
	})
	if err != nil {
		return t, err
	}
	if c.Deploy {
		return t, r.deploy(ctx, r.deployTo(final), Target{Commit: commit, Version: t.Name, Hotfix: c.Hotfix})
	}
	return t, nil
}

// refuse is why t can't be cut, unless c.Hotfix: its commit doesn't
// contain what the last environment runs, or it is a final and no
// candidate on its commit ran in the environment before last, by the
// history of its overlays in repos.
func (r *Runner) refuse(ctx context.Context, st Status, repos map[string]*ConfigRepo, c Cut, t Tag, tags []Tag) error {
	if c.Hotfix {
		return nil
	}
	why, err := r.contains(ctx, envNamed(st, r.Cfg.Last()), t.Commit)
	if err != nil || why != "" {
		return cmp.Or(err, errors.New(why))
	}
	cands := r.Cfg.Candidates()
	if t.Version.RC > 0 || len(cands) == 0 {
		return nil
	}
	env := cands[len(cands)-1]
	var names []string
	for _, at := range TagsAt(tags, t.Commit) {
		if at.Version.RC > 0 {
			names = append(names, at.Name)
		}
	}
	for _, d := range r.Cfg.Deploy {
		if len(names) == 0 {
			break
		}
		// ponytail: the overlay's last 1000 changes; a candidate that ran before them is cut again
		h, err := repos[d.Repo].History(ctx, strings.ReplaceAll(d.Path, "{env}", env), 1000)
		if err != nil {
			return fmt.Errorf("%s on %s: %w", env, d.Repo, err)
		}
		if slices.ContainsFunc(h, func(ch Change) bool { return slices.Contains(names, ch.Version) }) {
			return nil
		}
	}
	return fmt.Errorf("no candidate of %.7s ran in %s: cut one and deploy it there first", t.Commit, env)
}

// takes refuses, before anything is written, a deploy of commit to e that
// deploy would refuse: e not adopted, or not moving forward (what Draft
// checks); a hotfix may leave the line of an environment but the last.
func (r *Runner) takes(ctx context.Context, e EnvState, commit string, hotfix bool) error {
	if !e.Adopted {
		return fmt.Errorf("%s: %w", e.Name, ErrNotAdopted)
	}
	if hotfix && e.Name != r.Cfg.Last() {
		return nil
	}
	why, err := r.contains(ctx, e, commit)
	if err != nil || why == "" {
		return err
	}
	return fmt.Errorf("%s: %w", why, ErrOlder)
}

// built refuses an image of commit that none of the registries where lists
// for it has, by the record or as built from commit: verb ships what ran,
// it doesn't build. A registry lacking an image another has gets a copy.
func (r *Runner) built(ctx context.Context, commit string, recorded []gitbugio.Artifact, where map[string][]string, verb string) error {
	for _, img := range r.Cfg.Images {
		found := false
		for _, reg := range where[img.Name] {
			d, err := r.existing(ctx, recorded, img.Name, reg, commit)
			if err != nil {
				return fmt.Errorf("%s: %w", img.Name, err)
			}
			if found = d != ""; found {
				break
			}
		}
		if !found {
			return fmt.Errorf("%s for %.7s is in no registry: %s ships what ran, it doesn't build", img.Name, commit, verb)
		}
	}
	return nil
}
