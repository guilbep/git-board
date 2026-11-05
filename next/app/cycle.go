package app

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/guilbep/git-board/next/domain/cycle"
	"github.com/guilbep/git-board/next/domain/cycle/work"
	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/tools/branches"
	"github.com/guilbep/git-board/next/tools/code"
	"github.com/guilbep/git-board/next/tools/ship"
)

// The development cycle's reads (experimental): an issue's step from its
// evidence, and the Work page's rows, every open issue's.

// ErrCycleOff is the cycle asked for on a board where it is off.
var ErrCycleOff = &Error{Class: Off, Err: errors.New(`the development cycle is experimental, and off on this board: [experimental] features = ["cycle"] in .git-board.toml turns it on`)}

// ErrNoLogin is a viewer without a login: no role and no field can name
// them.
var ErrNoLogin = &Error{Class: NoMe, Code: "no-login", Err: errors.New("your person has neither a name nor a forge login, so no role and no field can name you: give them a name, or pass --me")}

// IssueCycle is an issue's step; WorkGroups the Work page's rows.
type (
	IssueCycle = CycleView
	WorkGroups = work.Groups
)

// CycleOn says the cycle is on: [experimental] features has "cycle".
func (b *Board) CycleOn() bool { return b.Config.On("cycle") }

// viewer is who reads the cycle: me's login, else the person who writes.
func (b *Board) viewer(ctx context.Context, me string) (issues.InboxMe, error) {
	m, err := b.Issues.InboxMe(ctx, me)
	if err == nil && m.Login == "" {
		err = ErrNoLogin
	}
	return m, err
}

// CycleView is an issue's step with what the page shows beside its
// evidence: when its PR was last read from the forge, and the release
// environments its merged work runs in.
type CycleView struct {
	cycle.Cycle
	ID           string     `json:"id"`           // the issue's whole id
	Version      string     `json:"version"`      // the issue's version: an action's If-Match
	BodyVersion  int        `json:"body_version"` // the description's: spec-verdict's on
	PRCheckedAt  time.Time  `json:"pr_checked_at,omitzero"`
	Environments []IssueEnv `json:"environments"`
}

// IssueEnv is a release environment ([release] environments), the
// version it runs, and whether it runs this issue's merged work.
type IssueEnv struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	In      bool   `json:"in"`
}

// Cycle is the issue's step, as the viewer (me, "" for the person who
// writes) sees it, with its evidence: its PRs from the server's cache
// (b.PRs), each asked again of the forge once stale, and the
// environments.
func (b *Board) Cycle(ctx context.Context, name, me string) (CycleView, error) {
	return b.cycleRead(ctx, name, me, false)
}

// cycleRead is Cycle; force asks the forge for each PR now.
func (b *Board) cycleRead(ctx context.Context, name, me string, force bool) (CycleView, error) {
	if !b.CycleOn() {
		return CycleView{}, ErrCycleOff
	}
	th, err := b.Issues.Show(ctx, name)
	if err != nil {
		return CycleView{}, err
	}
	m, err := b.viewer(ctx, me)
	if err != nil {
		return CycleView{}, err
	}
	var pulls cycle.Pulls
	if b.PRs != nil {
		pulls = refreshing{ctx, b, b.PRs, force}
	}
	in, err := b.Evidence(ctx, th, m.Login, pulls, nil)
	if err != nil {
		return CycleView{}, err
	}
	v := CycleView{Cycle: cycle.Compute(in), ID: string(th.ID), Version: string(th.Version), BodyVersion: BodyVersion(th), PRCheckedAt: in.PRAt}
	v.Environments, err = b.issueEnvs(ctx, in)
	return v, err
}

// issueEnvs are the release environments, each with whether it runs one
// of the issue's merged lanes, as the trunk's records say (no fetch):
// none without [release] environments, or when the trunk isn't there.
func (b *Board) issueEnvs(ctx context.Context, in cycle.Input) ([]IssueEnv, error) {
	out := []IssueEnv{}
	if len(b.Config.Release.Environments) == 0 {
		return out, nil
	}
	run, err := b.Runner(ctx)
	if errors.Is(err, ErrNoRelease) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	run.Offline = true
	st, err := run.Status(ctx)
	if err != nil {
		// a release state that doesn't read (no trunk fetched yet) is no
		// environment to tell: the cycle reads all the same
		return out, nil
	}
	for _, e := range st.Envs {
		env := IssueEnv{Name: e.Name, Version: e.Version}
		for _, l := range in.Lanes {
			if env.In || !l.Done || e.Commit == "" || l.Head == "" {
				continue
			}
			if env.In, err = ship.IsAncestor(ctx, b.Dir, l.Head, e.Commit); err != nil {
				return nil, err
			}
		}
		out = append(out, env)
	}
	return out, nil
}

// Work is the Work page: every open issue's step, what needs the viewer
// and what waits on others; team, what needs anyone and every other open
// issue.
func (b *Board) Work(ctx context.Context, me string, team bool) (work.Groups, error) {
	if !b.CycleOn() {
		return work.Groups{}, ErrCycleOff
	}
	m, err := b.viewer(ctx, me)
	if err != nil {
		return work.Groups{}, err
	}
	list, err := b.Issues.List(ctx, issues.ListReq{Filter: issues.DefaultFilter, Me: m.Login})
	if err != nil {
		return work.Groups{}, err
	}
	// the branches and the base's history, read once for the page
	c := b.Code()
	c.Kept = code.NewKept()
	anc := branches.NewAncestry(c)
	// each issue's thread and evidence, a few at once: most of the time
	// is git's, in its own processes
	rows := make([]work.Row, len(list))
	errs := make([]error, len(list))
	next := make(chan int)
	var wg sync.WaitGroup
	for range max(2, runtime.GOMAXPROCS(0)) {
		wg.Go(func() {
			for i := range next {
				th, err := b.Issues.Show(ctx, list[i].ID)
				var in cycle.Input
				if err == nil {
					in, err = b.EvidenceIn(ctx, anc, th, m.Login, b.cachedPRs(), nil)
				}
				if errs[i] = err; err == nil {
					rows[i] = work.RowOf(th, cycle.Compute(in), m)
				}
			}
		})
	}
	for i := range list {
		next <- i
	}
	close(next)
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		return work.Groups{}, err
	}
	return work.Group(rows, m.Login, team), nil
}

// cachedPRs are the PRs as last read, asking the forge nothing: the Work
// page's, for every open issue at once; nil without a cache.
func (b *Board) cachedPRs() cycle.Pulls {
	if b.PRs == nil {
		return nil
	}
	return b.PRs
}
