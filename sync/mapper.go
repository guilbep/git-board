package sync

import (
	"context"
	"fmt"

	"github.com/guilbep/git-board/forge"
	"github.com/guilbep/git-board/kernel"

	"github.com/guilbep/git-board/obs"
)

// Way is a run's way: pull, push, or both (sync).
type Way string

// The ways.
const (
	Pull Way = "pull"
	Push Way = "push"
	Both Way = "sync"
)

// Pulls tells whether the way pulls; Pushes, whether it pushes.
func (w Way) Pulls() bool  { return w == Pull || w == Both }
func (w Way) Pushes() bool { return w == Push || w == Both }

// Mapper says how one kind meets the forges (design.md, Sync): the issues,
// whose fields are compared with the last seen values, pulled and pushed
// (domain/issues/issuesync), and the releases, published only
// (domain/releases/releasesync), and the GitHub Project's fields
// (tools/ghproject). A run asks each in turn, over the forges that
// carry its kind; a mapper keeps its own report.
type Mapper interface {
	// Kind is the kind it maps: issue, release, project.
	Kind() string
	// Carries tells whether a forge carries the kind: a mapper is never
	// given one that doesn't.
	Carries(f forge.Forge) bool
	// Run runs one way over the forges.
	Run(ctx context.Context, b *kernel.Board, way Way, forges []forge.Forge) error
}

// Run runs each mapper, in order, over the forges that carry its kind; a
// mapper no forge carries is skipped. The first error stops the run, and
// names its kind.
func Run(ctx context.Context, b *kernel.Board, way Way, forges []forge.Forge, ms ...Mapper) error {
	for _, m := range ms {
		var fs []forge.Forge
		for _, f := range forges {
			if m.Carries(f) {
				fs = append(fs, f)
			}
		}
		if len(fs) == 0 {
			continue
		}
		mctx, end := obs.Start(ctx, "sync "+m.Kind()+"s", obs.String("way", string(way)), obs.Int("forges", int64(len(fs))))
		err := m.Run(mctx, b, way, fs)
		end(err)
		if err != nil {
			if m.Kind() == "issue" {
				return err // the issues' errors are said as v1 says them
			}
			return fmt.Errorf("%s: %w", m.Kind(), err)
		}
	}
	return nil
}
