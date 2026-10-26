package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"time"

	"github.com/guilbep/git-board/core/oplog"
	"github.com/guilbep/git-board/internal/insights"
)

// History is the label and status changes of the issues with these ids
// (every issue when nil), with their times: what the Insights page replays.
func (b *Backend) History(ids map[string]bool) ([]insights.Issue, error) {
	issues, err := b.allIssues()
	if err != nil {
		return nil, fmt.Errorf("listing the issues: %w", err)
	}
	var res []insights.Issue
	for _, id := range slices.Sorted(maps.Keys(issues)) {
		if ids != nil && !ids[id] {
			continue
		}
		_, l, err := b.loadIssue(id)
		if err != nil {
			return nil, err
		}
		res = append(res, b.historyOf(id, issues[id].Created, l.Entries))
	}
	return res, nil
}

// historyOf replays the labels and the status, as the issue's state
// folds them: a change that changes nothing isn't one.
func (b *Backend) historyOf(id string, created int64, entries []oplog.Entry) insights.Issue {
	is := insights.Issue{ID: id, Created: time.Unix(created, 0)}
	labels, status := map[string]bool{}, "open"
	for _, e := range entries {
		var d struct {
			Status      string
			Add, Remove []string
		}
		_ = json.Unmarshal(e.Data, &d)
		switch e.Kind {
		case "change-labels":
			var c insights.Change
			c.Added, c.Removed = applyLabels(labels, d.Add, d.Remove)
			if len(c.Added)+len(c.Removed) > 0 {
				c.At, c.Added, c.Removed = time.Unix(e.Time, 0), b.shown(c.Added), b.shown(c.Removed)
				is.Changes = append(is.Changes, c)
			}
		case "set-status":
			if d.Status != status {
				status = d.Status
				is.Changes = append(is.Changes, insights.Change{At: time.Unix(e.Time, 0), Status: status})
			}
		}
	}
	// the operations come in the log's order; the replay wants time
	sort.SliceStable(is.Changes, func(i, j int) bool { return is.Changes[i].At.Before(is.Changes[j].At) })
	return is
}

// RefsFingerprint changes whenever an issue, a person or a release record
// changes in git: their refs and the commits they point to, hashed. Cheap:
// the refs only.
func (b *Backend) RefsFingerprint() string {
	return b.fingerprint("refs/board/issue/", "refs/board/person/", "refs/board/release/")
}

// ReleasesFingerprint is RefsFingerprint for the releases alone: their
// records and the tags.
// ponytail: every tag, not only release tags; another tag moving costs
// the Releases page a reload
func (b *Backend) ReleasesFingerprint() string {
	return b.fingerprint("refs/board/release/", "refs/tags/")
}

func (b *Backend) fingerprint(prefixes ...string) string {
	h := sha256.New()
	for _, p := range prefixes {
		refs, err := b.s.Refs(p)
		if err != nil {
			continue
		}
		for _, k := range slices.Sorted(maps.Keys(refs)) {
			fmt.Fprintf(h, "%s %s\n", k, refs[k])
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// applyLabels applies a change-labels to labels, as the issue's state
// does: add, then remove. It answers what it changed.
func applyLabels(labels map[string]bool, add, remove []string) (added, removed []string) {
	before := maps.Clone(labels)
	for _, a := range add {
		labels[a] = true
	}
	for _, r := range remove {
		delete(labels, r)
	}
	for _, a := range add {
		if labels[a] && !before[a] && !slices.Contains(added, a) {
			added = append(added, a)
		}
	}
	for _, r := range remove {
		if before[r] && !labels[r] && !slices.Contains(removed, r) {
			removed = append(removed, r)
		}
	}
	return added, removed
}
