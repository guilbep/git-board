package app

import (
	"context"
	"errors"

	"github.com/guilbep/git-board/next/domain/cycle"
	"github.com/guilbep/git-board/next/domain/cycle/work"
	"github.com/guilbep/git-board/next/domain/issues"
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
	IssueCycle = cycle.Cycle
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

// Cycle is the issue's step, as the viewer (me, "" for the person who
// writes) sees it, with its evidence. PRs are not read: the forge's
// cache is the server's, later.
func (b *Board) Cycle(ctx context.Context, name, me string) (cycle.Cycle, error) {
	if !b.CycleOn() {
		return cycle.Cycle{}, ErrCycleOff
	}
	th, err := b.Issues.Show(ctx, name)
	if err != nil {
		return cycle.Cycle{}, err
	}
	m, err := b.viewer(ctx, me)
	if err != nil {
		return cycle.Cycle{}, err
	}
	in, err := b.Evidence(ctx, th, m.Login, nil, nil)
	if err != nil {
		return cycle.Cycle{}, err
	}
	return cycle.Compute(in), nil
}

// Work is the Work page: every open issue's step, what needs the viewer
// and what waits on others; team, what needs anyone and every other open
// issue.
func (b *Board) Work(ctx context.Context, me string, team bool) (work.Groups, error) {
	if !b.CycleOn() {
		return work.Groups{}, ErrCycleOff
	}
	ctx = b.Kernel.Snapshot(ctx) // one listing for the page
	m, err := b.viewer(ctx, me)
	if err != nil {
		return work.Groups{}, err
	}
	list, err := b.Issues.List(ctx, issues.ListReq{Filter: issues.DefaultFilter, Me: m.Login})
	if err != nil {
		return work.Groups{}, err
	}
	rows, err := b.workRows(ctx, list, m)
	if err != nil {
		return work.Groups{}, err
	}
	return work.Group(rows, m.Login, team), nil
}
