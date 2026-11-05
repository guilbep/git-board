package app

import (
	"context"
	"fmt"

	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/domain/releases"
)

// Book is the board's release records as the release runner keeps them
// (tools/ship's Book): the release domain's service, with the
// remote's records pulled and pushed, and the issues a draft names.
type Book struct {
	*releases.Service
	b *Board
}

// Book is the board's release records, written as the clone's board.me.
func (b *Board) Book() *Book {
	return &Book{Service: &releases.Service{Board: b.Kernel, Me: b.Issues.Me}, b: b}
}

// Pull fetches remote's board and merges it: the records and the persons
// who wrote them with the rest, as v1's. Nothing on a board kept
// elsewhere than git: every clone reads the same records there.
func (k *Book) Pull(ctx context.Context, remote string) error {
	if !k.b.inGit() {
		return nil
	}
	if _, err := k.b.Pull(ctx, remote); err != nil {
		return fmt.Errorf("pulling the releases from %s: %w", remote, err)
	}
	return nil
}

// Push pushes the records to remote, and the persons who wrote them.
func (k *Book) Push(ctx context.Context, remote string) error {
	if !k.b.inGit() {
		return nil
	}
	g, err := k.b.remoteOf(ctx, "pushing the releases", remote)
	if err == nil {
		err = g.Push(ctx, remote, "person", releases.Kind.Name())
	}
	if err != nil {
		return fmt.Errorf("pushing the releases to %s: %w", remote, err)
	}
	return nil
}

// Issues are every issue, closed ones too: what a release's draft names.
func (k *Book) Issues(ctx context.Context) ([]issues.Issue, error) {
	return k.b.Issues.List(ctx, issues.ListReq{Filter: "is:open,closed"})
}

// Board is the board the records are on.
func (k *Book) Board() *Board { return k.b }
