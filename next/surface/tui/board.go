package tui

import (
	"context"
	"slices"
	"strings"

	"github.com/guilbep/git-board/next/app"
	"github.com/guilbep/git-board/next/domain/issues"
)

// board is the Backend over an open board: every read and write is the
// application's, as the CLI's.
type board struct {
	ctx context.Context
	b   *app.Board
	me  string // --me, for @me
}

// Over is the Backend of an open board; me is what @me means ("": the
// person who writes). The TUI closes the board when it quits.
func Over(ctx context.Context, b *app.Board, me string) Backend { return &board{ctx, b, me} }

func (o *board) Match(filter string, among []issues.Issue) ([]issues.Issue, error) {
	list, err := o.b.Issues.List(o.ctx, issues.ListReq{Filter: filter, Me: o.me})
	if err != nil {
		return nil, err
	}
	in := map[string]bool{}
	for _, is := range list {
		in[string(is.ID)] = true
	}
	return slices.DeleteFunc(slices.Clone(among), func(is issues.Issue) bool { return !in[string(is.ID)] }), nil
}

func (o *board) Set(id, field, value string) ([]string, string, error) {
	said, err := o.b.Issues.Set(o.ctx, issues.SetReq{Name: id, Assign: []string{field + "=" + value}, MeName: o.me})
	if err != nil {
		return nil, "", err
	}
	labels, err := o.Labels(id)
	return labels, strings.Join(said, "; "), err
}

func (o *board) Labels(id string) ([]string, error) {
	th, err := o.b.Issues.Show(o.ctx, id)
	return th.Labels, err
}

func (o *board) Close() error { return o.b.Close() }
