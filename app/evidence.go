package app

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/guilbep/git-board/domain/cycle"
	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/tools/branches"
	"github.com/guilbep/git-board/tools/code"
)

// The cycle's evidence (v1's internal/evidence): what the cycle computes
// an issue's step from, read here: its branches against the base, their
// files and the plan file through git, and its PRs from the caller's
// cache. The cycle computes; this reads.

// Evidence is the issue's cycle.Input, read from the board, git and the
// PRs (nil: none). me is the viewer's login. Render, when set, is the
// plan's Markdown as HTML.
func (b *Board) Evidence(ctx context.Context, th issues.Thread, me string, pulls cycle.Pulls, render func(string) string) (cycle.Input, error) {
	return b.EvidenceIn(ctx, b.keptCode(ctx), th, me, pulls, render)
}

// codeKept is the code's reads an issue's cycle makes, kept across
// requests while the branches stay where they are (refsStamp): the
// branches, origin's HEAD, the plan files grepped, the base's history.
// Opening an issue started 4 gits for them, ~60 ms.
type codeKept = kept[*branches.Ancestry]

// keptCode is the board's kept Ancestry, started over when a ref moved.
func (b *Board) keptCode(ctx context.Context) *branches.Ancestry {
	stamp := ""
	if common := b.forges.commonDir(ctx, b.Dir); common != "" {
		stamp = refsStamp(common)
	}
	anc, _ := b.code.get(stamp, func() (*branches.Ancestry, error) {
		r := b.Code()
		r.Kept = code.NewKept()
		return branches.NewAncestry(r), nil
	})
	return anc
}

// CodeStamp names where the branches are (refsStamp), and the PRs read
// (PullCache's generation): what the Work page's rows read beside the
// board; "" when it can't be told.
func (b *Board) CodeStamp(ctx context.Context) string {
	common := b.forges.commonDir(ctx, b.Dir)
	if common == "" {
		return ""
	}
	gen := uint64(0)
	if b.PRs != nil {
		gen = b.PRs.Gen()
	}
	return fmt.Sprintf("%s\nprs %d", refsStamp(common), gen)
}

// refsStamp names where the branches are, from the files git keeps them
// in, without a git: packed-refs, the reftable folder, and each folder
// under refs/heads and refs/remotes, whose time a ref written (a file
// renamed into place), made or deleted moves.
// ponytail: folders' times, not each ref file's: a filesystem that keeps
// whole seconds (HFS+) could miss two writes in one second; stat every
// ref file if one does.
func refsStamp(common string) string {
	var b strings.Builder
	stat := func(p string) {
		if st, err := os.Stat(p); err == nil {
			fmt.Fprintf(&b, "%s %d %d\n", p, st.Size(), st.ModTime().UnixNano())
		}
	}
	stat(filepath.Join(common, "packed-refs"))
	stat(filepath.Join(common, "reftable"))
	for _, d := range []string{"heads", "remotes"} {
		filepath.WalkDir(filepath.Join(common, "refs", d), func(p string, e fs.DirEntry, err error) error {
			if err == nil && e.IsDir() {
				stat(p)
			}
			return nil
		})
	}
	return b.String()
}

// EvidenceIn is Evidence, reading the branches and their histories
// through anc: one Ancestry for many issues reads their base's history
// once, and with a repository that keeps its reads (code.Kept), the
// branches once.
func (b *Board) EvidenceIn(ctx context.Context, anc *branches.Ancestry, th issues.Thread, me string, pulls cycle.Pulls, render func(string) string) (cycle.Input, error) {
	cfg := b.Config
	th.Inherited = cfg.Schema.RolesFor(th.Labels)
	start, err := branches.Start(ctx, anc.Repo(), cfg.Branches)
	if err != nil {
		return cycle.Input{}, err
	}
	linked, err := branches.LinkedIn(ctx, anc, th.Issue, start)
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
		Base:   start,
		Repo:   repo{ctx, anc.Repo()}, // its reads kept with the branches'
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
