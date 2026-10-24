// Package watch reports the entities whose heads changed, by polling their
// refs.
package watch

import (
	"cmp"
	"context"
	"slices"
	"strings"
	"time"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/oplog"
)

// Change is an entity that appeared, moved or was removed. Head is its new
// head, ZeroHash when it was removed.
type Change struct {
	Kind, ID string
	Head     gitstore.Hash
}

// Watch polls every kind's refs until ctx ends, and then closes the channel.
// The first poll is the baseline: it runs before Watch returns and reports
// nothing. Each later poll runs every after the last, and sends its changes
// as one batch, sorted by kind then id; with no change it sends nothing. A
// poll that fails to list any kind is skipped, and the next compares with the
// last that succeeded; a skipped baseline makes the next poll the baseline.
// An every of 0 or less means 1 second. A ref whose name isn't an id is
// ignored.
//
// ponytail: polls every interval; inotify-style notification if polling ever costs too much
func Watch(ctx context.Context, s *gitstore.Store, every time.Duration) <-chan []Change {
	return watch(ctx, s.Refs, every)
}

type key struct{ kind, id string }

// lister lists the refs under a prefix: Store.Refs, or a test's.
type lister func(prefix string) (map[string]gitstore.Hash, error)

func watch(ctx context.Context, list lister, every time.Duration) <-chan []Change {
	if every <= 0 {
		every = time.Second
	}
	ch := make(chan []Change)
	last, _ := poll(list) // nil when it failed: the next poll is the baseline
	go func() {
		defer close(ch)
		for {
			select {
			case <-time.After(every):
			case <-ctx.Done():
				return
			}
			heads, err := poll(list)
			if err != nil {
				continue
			}
			if changes := diff(last, heads); last != nil && len(changes) > 0 {
				select {
				case ch <- changes:
				case <-ctx.Done():
					return
				}
			}
			last = heads
		}
	}()
	return ch
}

// poll lists every kind's heads by entity.
func poll(list lister) (map[key]gitstore.Hash, error) {
	heads := map[key]gitstore.Hash{}
	for _, kind := range gitstore.Kinds {
		prefix := oplog.RefName(kind, "")
		refs, err := list(prefix)
		if err != nil {
			return nil, err
		}
		for name, h := range refs {
			if id := strings.TrimPrefix(name, prefix); oplog.IsID(id) {
				heads[key{kind, id}] = h
			}
		}
	}
	return heads, nil
}

// diff gives the entities whose head differs from old to cur, sorted.
func diff(old, cur map[key]gitstore.Hash) []Change {
	var out []Change
	for k, h := range cur {
		if old[k] != h {
			out = append(out, Change{k.kind, k.id, h})
		}
	}
	for k := range old {
		if _, ok := cur[k]; !ok {
			out = append(out, Change{k.kind, k.id, gitstore.ZeroHash})
		}
	}
	slices.SortFunc(out, func(a, b Change) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.ID, b.ID))
	})
	return out
}
