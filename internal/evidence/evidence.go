// Package evidence reads what the cycle computes an issue's step from
// (core/cycle): its branches against the base (internal/branches), their
// files and the plan file through the store, its PRs from the server's
// cache, and the plan rendered (internal/markdown). The core computes;
// this reads.
package evidence

import (
	"github.com/guilbep/git-board/core/cycle"
	"github.com/guilbep/git-board/core/inbox"
	"github.com/guilbep/git-board/core/model"
	"github.com/guilbep/git-board/internal/branches"
	"github.com/guilbep/git-board/internal/config"
	"github.com/guilbep/git-board/internal/markdown"
	"github.com/guilbep/git-board/internal/store"
)

// Gather is the issue's cycle.Input, read from the board, git and the PR
// cache (nil: none). me is the viewer's login.
func Gather(b *store.Backend, cfg *config.Config, th model.Thread, me string, pulls cycle.Pulls) (cycle.Input, error) {
	return GatherIn(branches.NewAncestry(b), b, cfg, th, me, pulls)
}

// GatherIn is Gather, reading the branches' histories through anc: the
// Work page gathers every open issue with one, so their base's history is
// read once a request, not once an issue.
func GatherIn(anc *branches.Ancestry, b *store.Backend, cfg *config.Config, th model.Thread, me string, pulls cycle.Pulls) (cycle.Input, error) {
	th.Inherited = cfg.RolesFor(th.Labels)
	sug, err := branches.Suggest(b, cfg.Branches, th.Issue)
	if err != nil {
		return cycle.Input{}, err
	}
	linked, err := branches.LinkedIn(anc, th.Issue, sug.Start)
	if err != nil {
		return cycle.Input{}, err
	}
	// What the identity git-board writes as wrote is the viewer's, with or
	// without a login (--me names the viewer).
	v := inbox.Me{Login: me}
	if p, ok, err := b.Me(); err == nil && ok {
		v.ID = p.ID
	}
	src := cycle.Sources{
		Stages: cfg.Stages(),
		Kind:   cfg.Branches.Kind(th.AllLabels()),
		Me:     v,
		Base:   sug.Start,
		Repo:   backend{b},
		Pulls:  pulls,
		// ponytail: links as the file writes them, plan files carry no uploads
		Render: func(md string) string { return markdown.Render(md, func(u string) string { return u }) },
	}
	for _, br := range linked {
		src.Branches = append(src.Branches, cycle.Branch{Name: br.Name, Remote: br.Remote, Head: br.LastCommit.SHA, At: br.LastCommit.Date,
			Ahead: br.Ahead, Behind: br.Behind, Merged: br.Merged, InBase: br.InBase})
	}
	return cycle.Gather(th, src)
}

// backend is the store as the cycle reads git (cycle.Repo).
type backend struct{ *store.Backend }

func (g backend) Author(sha string) (string, error) {
	c, err := g.ReadCommit(sha)
	return c.Author, err
}
