package app

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/guilbep/git-board/next/store/git"
)

// A git remote's board: fetch, pull and push (storage.md, Fetch and
// push), as v1's git board fetch, pull and push.
// From origin, a pull takes the numbers and a push numbers what origin
// then has (origin.go).
// Between two boards that started over apart, both refuse (reset.go).

// remoteOf is the board's git store, refusing another datastore and a
// remote the repository doesn't have.
func (b *Board) remoteOf(ctx context.Context, what, remote string) (*git.Store, error) {
	g, ok := b.Kernel.Store().(*git.Store)
	if !ok {
		return nil, missing(what + " needs a board kept in git; this board is kept in " + b.keptIn())
	}
	out, err := gitRun(ctx, b.Dir, "remote")
	if err != nil {
		return nil, err
	}
	if !slices.Contains(strings.Fields(out), remote) {
		return nil, fmt.Errorf("no remote %q", remote)
	}
	return g, nil
}

// Ahead is what a pull would receive and a push would send, of one kind.
type Ahead struct {
	Receive int `json:"receive"`
	Send    int `json:"send"`
}

// Fetch brings the remote's board to its copies here, and says what a
// pull would receive and a push would send, of issues and of people.
// Nothing of this clone's board changes.
func (b *Board) Fetch(ctx context.Context, remote string) (issues, people Ahead, err error) {
	g, err := b.remoteOf(ctx, "fetching the board from a remote", remote)
	if err != nil {
		return issues, people, err
	}
	if _, err := g.Fetch(ctx, remote, Kinds...); err != nil {
		return issues, people, err
	}
	if issues.Receive, issues.Send, err = g.Ahead(ctx, remote, "issue"); err != nil {
		return issues, people, err
	}
	people.Receive, people.Send, err = g.Ahead(ctx, remote, "person")
	return issues, people, err
}

// Pull fetches the remote's board, then merges it into this clone's,
// every clone's operations kept; from origin, it takes the numbers too.
// It answers a note: the numbers it sent.
func (b *Board) Pull(ctx context.Context, remote string) (string, error) {
	g, err := b.remoteOf(ctx, "git board pull", remote)
	if err != nil {
		return "", err
	}
	if err := b.resetGuard(ctx, remote); err != nil {
		return "", err
	}
	if _, err := g.Fetch(ctx, remote, Kinds...); err != nil {
		return "", err
	}
	if _, err := g.Merge(ctx, remote, Kinds...); err != nil {
		return "", err
	}
	if err := b.switched(ctx); err != nil {
		return "", err
	}
	return b.pullNumbers(ctx, remote)
}

// Push pushes this clone's board to the remote, not forced: a remote that
// holds changes this clone doesn't have refuses it. On origin, it then
// numbers the issues origin has, and its note says how many.
func (b *Board) Push(ctx context.Context, remote string) (string, error) {
	g, err := b.remoteOf(ctx, "git board push", remote)
	if err != nil {
		return "", err
	}
	if err := b.resetGuard(ctx, remote); err != nil {
		return "", err
	}
	if err := g.Push(ctx, remote, Kinds...); err != nil {
		if msg := err.Error(); strings.Contains(msg, "non-fast-forward") || strings.Contains(msg, "fetch first") {
			return "", fmt.Errorf("%s has changes this repository doesn't have: git board pull first", remote)
		}
		return "", err
	}
	if remote != NumbersRemote {
		return "", nil
	}
	n, err := b.numberIssues(ctx, remote)
	switch {
	case err != nil || n == 0:
		return "", err
	case n == 1:
		return "numbered 1 issue", nil
	}
	return fmt.Sprintf("numbered %d issues", n), nil
}
