package issues

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/guilbep/git-board/next/kernel"
)

// Moved is one operation of an issue's history that changed its labels
// or its status: the labels added and removed, as git-board reads them
// (Shown), or the status set.
type Moved struct {
	At             time.Time
	Added, Removed []string
	Status         string // "open" or "closed" when the operation sets it
}

// History is an issue's label and status changes, in time order: what
// Insights replays (v1's store.History).
type History struct {
	ID      string
	Created time.Time
	Changes []Moved
}

// History is the label and status changes of the issues with these ids
// (every issue when nil), by id. A deleted or broken issue is left out,
// as a list leaves it, and told to Notice.
func (s *Service) History(ctx context.Context, ids map[string]bool) ([]History, error) {
	ctx = s.Board.Snapshot(ctx)
	var mu sync.Mutex
	var out []History
	err := Kind.Logs(ctx, s.Board, func(id kernel.ID, log []kernel.Entry, err error) {
		if ids != nil && !ids[string(id)] {
			return
		}
		if err == nil {
			var st State
			if st, err = Kind.Fold(id, log); err == nil && st.Deleted {
				return
			}
		}
		h := History{}
		if err == nil {
			h = s.historyOf(id, log)
		}
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			s.notice("issue %s: %v", id, err)
			return
		}
		out = append(out, h)
	})
	if err != nil {
		return nil, fmt.Errorf("listing the issues: %w", err)
	}
	slices.SortFunc(out, func(a, b History) int { return strings.Compare(a.ID, b.ID) })
	return out, nil
}

// historyOf replays the labels and the status as the state folds them: a
// change that changes nothing isn't one.
func (s *Service) historyOf(id kernel.ID, log []kernel.Entry) History {
	h := History{ID: string(id), Created: time.Unix(log[0].Time, 0)}
	labels, status := map[string]bool{}, string(Open)
	for _, e := range log {
		switch e.Kind {
		case "change-labels", "set-status":
		default:
			continue
		}
		d, err := decode(e.Data)
		if err != nil {
			continue
		}
		if e.Kind == "set-status" {
			if string(d.Status) != status {
				status = string(d.Status)
				h.Changes = append(h.Changes, Moved{At: time.Unix(e.Time, 0), Status: status})
			}
			continue
		}
		added, removed := applyLabels(labels, d.Add, d.Remove)
		if len(added)+len(removed) > 0 {
			h.Changes = append(h.Changes, Moved{At: time.Unix(e.Time, 0), Added: s.Schema.Shown(added), Removed: s.Schema.Shown(removed)})
		}
	}
	// the operations come in the log's order; the replay wants time
	sort.SliceStable(h.Changes, func(i, j int) bool { return h.Changes[i].At.Before(h.Changes[j].At) })
	return h
}
