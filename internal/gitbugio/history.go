package gitbugio

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	all, err := b.c.Bugs().Query(context.Background(), query.NewQuery())
	if err != nil {
		return nil, fmt.Errorf("listing bugs: %w", err)
	}
	var res []insights.Issue
	for _, id := range all {
		if ids != nil && !ids[id.String()] {
			continue
		}
		bug, err := b.c.Bugs().Resolve(context.Background(), id)
		if err != nil {
			return nil, fmt.Errorf("resolving %s: %w", id, err)
		}
		res = append(res, b.historyOf(bug))
	}
	return res, nil
}

func (b *Backend) historyOf(bug *cache.BugCache) insights.Issue {
	snap := bug.Snapshot()
	is := insights.Issue{ID: bug.Id().String(), Created: snap.CreateTime}
	for _, op := range snap.Operations {
		switch o := op.(type) {
		case *bugent.LabelChangeOperation:
			is.Changes = append(is.Changes, insights.Change{At: o.Time(), Added: b.shown(labelStrings(o.Added)), Removed: b.shown(labelStrings(o.Removed))})
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

// RefsFingerprint changes whenever an issue, a person or a release record
// changes in git: their refs and the commits they point to, hashed. Cheap:
// the refs only.
func (b *Backend) RefsFingerprint() string {
	return b.fingerprint("bugs", "identities", ReleasesNamespace)
}

// ReleasesFingerprint is RefsFingerprint for the releases alone: their
// records and the tags.
// ponytail: every tag, not only release tags; another tag moving costs
// the Releases page a reload
func (b *Backend) ReleasesFingerprint() string {
	return b.fingerprint(ReleasesNamespace, "tags")
}

func (b *Backend) fingerprint(namespaces ...string) string {
	h := sha256.New()
	for _, ns := range namespaces {
		refs, err := b.repo.ListRefs(ns)
		if err != nil {
			continue
		}
		keys := make([]string, 0, len(refs))
		for k := range refs {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(h, "%s %s\n", k, refs[k])
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
