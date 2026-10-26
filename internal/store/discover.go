package store

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/guilbep/git-board/core/forge/github"
	"github.com/guilbep/git-board/core/forge/gitlab"
	"github.com/guilbep/git-board/internal/config"
)

// Forges are the forges a sync runs with: the settings' [forges.<name>]
// tables, or with none, those found in every URL and push URL of the git
// remotes (config.Discover), remotes by name. skipped hears each URL that
// names no forge, and why.
func (b *Backend) Forges(cfg *config.Config, skipped func(string)) ([]config.Forge, error) {
	if !cfg.Discover {
		return cfg.Forges, nil
	}
	urls, pushURLs, err := b.remoteURLs()
	if err != nil {
		return nil, err
	}
	var all []string
	for _, name := range slices.Sorted(maps.Keys(urls)) {
		all = append(all, urls[name]...)
		all = append(all, pushURLs[name]...)
	}
	forges, skips := config.Discover(all)
	for _, s := range skips {
		skipped("skipped " + s)
	}
	return forges, nil
}

// ResolveForges gives each forge found in the remotes its project as a
// table names it, read once for the run through the adapter: GitLab's
// numeric id, as links are keyed on it; GitHub's owner/repo as GitHub
// spells it, as Key() keeps case and a URL's case is the user's typing.
// Its Key() is then that of the table. One whose project can't be read is
// left out, and left hears why; the others go on. A table's project stays
// as written.
func ResolveForges(ctx context.Context, forges []config.Forge, left func(f config.Forge, why error)) []config.Forge {
	var out []config.Forge
	seen := map[string]string{} // key -> forge name
	for _, f := range forges {
		var read func(context.Context, config.Forge) (string, error)
		switch {
		case f.Discovered && f.Kind == "gitlab" && f.Project == f.Path:
			read = gitlabID
		case f.Discovered && f.Kind == "github":
			read = githubName
		}
		if read != nil {
			project, err := read(ctx, f)
			if err != nil {
				left(f, err)
				continue
			}
			f.Project = project
		}
		key := strings.ToLower(f.Host + "/" + f.Project)
		if other, ok := seen[key]; ok { // two paths of one project: a rename's old one
			left(f, fmt.Errorf("the same project as %s", other))
			continue
		}
		seen[key] = f.Name
		out = append(out, f)
	}
	return out
}

// githubName reads the repository's full_name through the adapter.
func githubName(ctx context.Context, f config.Forge) (string, error) {
	g, err := github.New(ctx, github.Options{Project: f.Project, Host: f.Host, Login: f.Login, API: githubAPI(f)})
	if err != nil {
		return "", err
	}
	return g.FullName(ctx)
}

// gitlabID reads the project's id through the adapter, opened by its path.
func gitlabID(ctx context.Context, f config.Forge) (string, error) {
	g, err := gitlab.New(ctx, f.BaseURL(), f.Project, f.Login)
	if err != nil {
		return "", err
	}
	return g.ProjectID(ctx)
}
