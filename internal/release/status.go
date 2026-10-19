package release

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/guilbep/git-board/internal/gitbugio"
)

// TargetState is what one deploy target's overlay says runs in an
// environment.
type TargetState struct {
	Repo     string   `json:"repo"`
	Path     string   `json:"path"` // the overlay's directory, {env} filled in
	Deployed Deployed `json:"deployed"`
	Since    *Change  `json:"since"` // the overlay's newest commit, nil when unknown
}

// EnvState is what runs in an environment, across its targets.
type EnvState struct {
	Name    string        `json:"name"`
	Role    string        `json:"role"` // follow, candidate, last, or "" (before follow)
	Targets []TargetState `json:"targets"`
	Commit  string        `json:"commit"`  // the targets' when they agree
	Version string        `json:"version"` // the targets' when they agree
	Split   bool          `json:"split"`   // targets run different commits, an unadopted one's being ""
	Adopted bool          `json:"adopted"` // every target has git-board/commit
	Behind  []string      `json:"behind"`  // follow only: subjects of the trunk's merges it doesn't run
}

// ReleaseRow is a release tag, its commit's record and where that commit
// runs.
type ReleaseRow struct {
	Tag    Tag                     `json:"tag"`
	Names  []string                `json:"names"`  // every release tag on its commit, newest first
	Record *gitbugio.ReleaseRecord `json:"record"` // nil until Reconcile makes it
	RunsIn []string                `json:"runsIn"` // environments whose commit is its commit
}

// Status is what runs where, and the releases.
type Status struct {
	Envs     []EnvState   `json:"envs"`
	Releases []ReleaseRow `json:"releases"` // one per release tag, newest first
	Head     string       `json:"head"`     // the trunk's head on the authority remote
}

// Status reads what runs where from the overlays. Unless Offline it first
// fetches the authority remote (branches, tags, release records) and each
// config repo, once. It writes nothing.
func (r *Runner) Status(ctx context.Context) (Status, error) {
	if err := r.fetch(ctx); err != nil {
		return Status{}, err
	}
	return r.status(ctx)
}

// status is Status without the fetch of the app remote; the config repos
// are still fetched unless Offline.
func (r *Runner) status(ctx context.Context) (Status, error) {
	head, err := Git(ctx, r.Dir, "rev-parse", r.trunk())
	if err != nil {
		return Status{}, err
	}
	st := Status{Head: head}
	repos, err := r.configRepos(ctx)
	if err != nil {
		return Status{}, err
	}
	for _, env := range r.Cfg.Environments {
		e, err := r.envState(ctx, env, repos)
		if err != nil {
			return Status{}, err
		}
		st.Envs = append(st.Envs, e)
	}
	tags, err := ReleaseTags(ctx, r.Dir)
	if err != nil {
		return Status{}, err
	}
	recs, err := r.B.Releases()
	if err != nil {
		return Status{}, err
	}
	records := map[string]*gitbugio.ReleaseRecord{}
	for i := range recs {
		records[recs[i].Commit] = &recs[i]
	}
	for _, t := range tags {
		row := ReleaseRow{Tag: t, Record: records[t.Commit]}
		for _, at := range TagsAt(tags, t.Commit) {
			row.Names = append(row.Names, at.Name)
		}
		for _, e := range st.Envs {
			if e.Commit == t.Commit {
				row.RunsIn = append(row.RunsIn, e.Name)
			}
		}
		st.Releases = append(st.Releases, row)
	}
	return st, nil
}

// configRepos opens each deploy target's config repo once, keyed by its URL,
// and fetches it unless Offline.
func (r *Runner) configRepos(ctx context.Context) (map[string]*ConfigRepo, error) {
	repos := map[string]*ConfigRepo{}
	for _, t := range r.Cfg.Deploy {
		if repos[t.Repo] != nil {
			continue
		}
		c, err := OpenConfigRepo(ctx, r.CacheDir, t.Repo)
		if err != nil {
			return nil, err
		}
		if !r.Offline {
			if err := c.Fetch(ctx); err != nil {
				return nil, err
			}
		}
		repos[t.Repo] = c
	}
	return repos, nil
}

func (r *Runner) envState(ctx context.Context, env string, repos map[string]*ConfigRepo) (EnvState, error) {
	e := EnvState{Name: env, Adopted: len(r.Cfg.Deploy) > 0}
	switch {
	case env == r.Cfg.Followed():
		e.Role = "follow"
	case env == r.Cfg.Last():
		e.Role = "last"
	case slices.Contains(r.Cfg.Candidates(), env):
		e.Role = "candidate"
	}
	chart := ""
	if r.Cfg.Chart != nil {
		chart = r.Cfg.Chart.Registry
	}
	versionsAgree := true
	for _, t := range r.Cfg.Deploy {
		c, path := repos[t.Repo], strings.ReplaceAll(t.Path, "{env}", env)
		o, err := c.Read(ctx, path)
		if err != nil {
			return e, fmt.Errorf("%s in %s: %w", env, t.Repo, err)
		}
		ts := TargetState{Repo: t.Repo, Path: path, Deployed: o.Deployed(chart)}
		h, err := c.History(ctx, path, 1)
		if err != nil {
			return e, fmt.Errorf("%s in %s: %w", env, t.Repo, err)
		}
		if len(h) > 0 {
			ts.Since = &h[0]
		}
		if len(e.Targets) > 0 {
			e.Split = e.Split || ts.Deployed.Commit != e.Targets[0].Deployed.Commit
			versionsAgree = versionsAgree && ts.Deployed.Version == e.Targets[0].Deployed.Version
		}
		e.Adopted = e.Adopted && ts.Deployed.Commit != ""
		e.Targets = append(e.Targets, ts)
	}
	if len(e.Targets) == 0 {
		return e, nil
	}
	if !e.Split {
		e.Commit = e.Targets[0].Deployed.Commit
	}
	if versionsAgree {
		e.Version = e.Targets[0].Deployed.Version
	}
	if e.Role == "follow" && e.Commit != "" {
		ms, err := Merges(ctx, r.Dir, e.Commit, r.trunk())
		if err != nil {
			return e, err
		}
		for _, m := range ms {
			e.Behind = append(e.Behind, m.Subject)
		}
	}
	return e, nil
}
