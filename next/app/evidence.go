package app

import (
	"context"

	"github.com/guilbep/git-board/next/domain/cycle"
	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/tools/branches"
	"github.com/guilbep/git-board/next/tools/code"
)

// The cycle's evidence (v1's internal/evidence): what the cycle computes
// an issue's step from, read here: its branches against the base, their
// files and the plan file through git, and its PRs from the caller's
// cache. The cycle computes; this reads.

// Evidence is the issue's cycle.Input, read from the board, git and the
// PRs (nil: none). me is the viewer's login. Render, when set, is the
// plan's Markdown as HTML.
func (b *Board) Evidence(ctx context.Context, th issues.Thread, me string, pulls cycle.Pulls, render func(string) string) (cycle.Input, error) {
	return b.EvidenceIn(ctx, branches.NewAncestry(b.Code()), th, me, pulls, render)
}

// EvidenceIn is Evidence, reading the branches and their histories
// through anc: one Ancestry for many issues reads their base's history
// once, and with a repository that keeps its reads (code.Kept), the
// branches once.
func (b *Board) EvidenceIn(ctx context.Context, anc *branches.Ancestry, th issues.Thread, me string, pulls cycle.Pulls, render func(string) string) (cycle.Input, error) {
	cfg := b.Config
	th.Inherited = cfg.Schema.RolesFor(th.Labels)
	sug, err := branches.Suggest(ctx, anc.Repo(), cfg.Branches, th.Issue)
	if err != nil {
		return cycle.Input{}, err
	}
	linked, err := branches.LinkedIn(ctx, anc, th.Issue, sug.Start)
	if err != nil {
		return cycle.Input{}, err
	}
	// what the person git-board writes as wrote is the viewer's, with or
	// without a login (--me names the viewer)
	v := issues.InboxMe{ID: string(b.Issues.Me), Login: me}
	var options []string
	if cfg.Schema != nil {
		options = cfg.Schema.Fields["status"].Options
	}
	src := cycle.Sources{
		Stages: cfg.Cycle.Stages(options),
		Kind:   branches.Kind(cfg.Branches, th.AllLabels()),
		Me:     v,
		Base:   sug.Start,
		Repo:   repo{ctx, b.Code()},
		Pulls:  pulls,
		Render: render,
	}
	for _, br := range linked {
		src.Branches = append(src.Branches, cycle.Branch{Name: br.Name, Remote: br.Remote, Head: br.LastCommit.SHA, At: br.LastCommit.Date,
			Ahead: br.Ahead, Behind: br.Behind, Merged: br.Merged, InBase: br.InBase})
	}
	return cycle.Gather(th, src)
}

// Code is the repository's code, beside its board.
func (b *Board) Code() code.Repo { return code.Repo{Dir: b.Dir} }

// repo is the code as the cycle reads it (cycle.Repo).
type repo struct {
	ctx context.Context
	r   code.Repo
}

func (g repo) ChangedFiles(base, ref string) ([]string, error) {
	return g.r.ChangedFiles(g.ctx, base, ref)
}
func (g repo) FirstCommitAdding(base, ref, path string) string {
	return g.r.FirstCommitAdding(g.ctx, base, ref, path)
}
func (g repo) GrepFiles(ref, dir, pattern string) ([]string, error) {
	return g.r.GrepFiles(g.ctx, ref, dir, pattern)
}
func (g repo) ReadFile(ref, path string) ([]byte, error) { return g.r.ReadFile(g.ctx, ref, path) }
func (g repo) Author(sha string) (string, error) {
	c, err := g.r.ReadCommit(g.ctx, sha)
	return c.Author, err
}
