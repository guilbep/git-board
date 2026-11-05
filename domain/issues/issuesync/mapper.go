package issuesync

import (
	"context"

	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/kernel"
	bsync "github.com/guilbep/git-board/sync"
)

// Mapper is the issues' mapper on the sync engine (bsync.Mapper): the
// run of this package, whose report it keeps. Every forge carries
// issues.
type Mapper struct {
	Options Options
	Report  Report // the last run's
}

var _ bsync.Mapper = (*Mapper)(nil)

func (m *Mapper) Kind() string             { return "issue" }
func (m *Mapper) Carries(forge.Forge) bool { return true }

// Run runs the issues' sync one way.
func (m *Mapper) Run(ctx context.Context, b *kernel.Board, way bsync.Way, fs []forge.Forge) error {
	var err error
	switch {
	case way == bsync.Pull:
		m.Report, err = Pull(ctx, b, m.Options, fs...)
	case way == bsync.Push:
		m.Report, err = Push(ctx, b, m.Options, fs...)
	default:
		m.Report, err = Sync(ctx, b, m.Options, fs...)
	}
	return err
}
