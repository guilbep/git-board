package gitbugio

import (
	"fmt"
	"sort"

	"github.com/git-bug/git-bug/cache"
	bugent "github.com/git-bug/git-bug/entities/bug"
	"github.com/git-bug/git-bug/entities/common"
	"github.com/git-bug/git-bug/query"

	"github.com/guilbep/git-board/internal/insights"
)

// History is the label and status changes of the issues with these ids
// (every issue when nil), with their times, as git-bug keeps them: what
// the Insights page replays.
func (b *Backend) History(ids map[string]bool) ([]insights.Issue, error) {
	all, err := b.c.Bugs().Query(query.NewQuery())
	if err != nil {
		return nil, fmt.Errorf("listing bugs: %w", err)
	}
	var res []insights.Issue
	for _, id := range all {
		if ids != nil && !ids[id.String()] {
			continue
		}
		bug, err := b.c.Bugs().Resolve(id)
		if err != nil {
			return nil, fmt.Errorf("resolving %s: %w", id, err)
		}
		res = append(res, historyOf(bug))
	}
	return res, nil
}

func historyOf(bug *cache.BugCache) insights.Issue {
	snap := bug.Snapshot()
	is := insights.Issue{ID: bug.Id().String(), Created: snap.CreateTime}
	for _, op := range snap.Operations {
		switch o := op.(type) {
		case *bugent.LabelChangeOperation:
			is.Changes = append(is.Changes, insights.Change{At: o.Time(), Added: labelStrings(o.Added), Removed: labelStrings(o.Removed)})
		case *bugent.SetStatusOperation:
			status := "open"
			if o.Status == common.ClosedStatus {
				status = "closed"
			}
			is.Changes = append(is.Changes, insights.Change{At: o.Time(), Status: status})
		}
	}
	// the operations come in git-bug's logical order; the replay wants time
	sort.SliceStable(is.Changes, func(i, j int) bool { return is.Changes[i].At.Before(is.Changes[j].At) })
	return is
}
