// Package watch reports the entities whose versions changed, by polling
// the datastore's heads.
package watch

import (
	"cmp"
	"context"
	"slices"
	"time"

	"github.com/guilbep/git-board/core/datastore"
	"github.com/guilbep/git-board/core/oplog"
)

// Change is an entity that appeared, moved or was removed. Version is its
// new version, "" when it was removed.
type Change struct {
	Kind, ID string
	Version  string
}

// Watch polls every kind's heads until ctx ends, and then closes the channel.
// The first poll is the baseline: it runs before Watch returns and reports
// nothing. Each later poll runs every after the last, and sends its changes
// as one batch, sorted by kind then id; with no change it sends nothing. A
// poll that fails to list any kind is skipped, and the next compares with the
// last that succeeded; a skipped baseline makes the next poll the baseline.
// An every of 0 or less means 1 second. A name that isn't an id is
// ignored.
//
// ponytail: polls every interval; inotify-style notification if polling ever costs too much
func Watch(ctx context.Context, s oplog.Datastore, every time.Duration) <-chan []Change {
	return watch(ctx, func(kind string) (map[string]string, error) { return s.Heads(ctx, kind) }, every)
}

type key struct{ kind, id string }

// lister lists a kind's heads: the datastore's, or a test's.
type lister func(kind string) (map[string]string, error)

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
func poll(list lister) (map[key]string, error) {
	heads := map[key]string{}
	for _, kind := range datastore.Kinds {
		m, err := list(kind)
		if err != nil {
			return nil, err
		}
		for id, v := range m {
			if oplog.IsID(id) {
				heads[key{kind, id}] = v
			}
		}
	}
	return heads, nil
}

// diff gives the entities whose version differs from old to cur, sorted.
func diff(old, cur map[key]string) []Change {
	var out []Change
	for k, h := range cur {
		if old[k] != h {
			out = append(out, Change{k.kind, k.id, h})
		}
	}
	for k := range old {
		if _, ok := cur[k]; !ok {
			out = append(out, Change{k.kind, k.id, ""})
		}
	}
	slices.SortFunc(out, func(a, b Change) int {
		return cmp.Or(cmp.Compare(a.Kind, b.Kind), cmp.Compare(a.ID, b.ID))
	})
	return out
}
