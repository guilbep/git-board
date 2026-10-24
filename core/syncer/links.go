package syncer

import (
	"cmp"
	"slices"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/oplog"
)

// Link joins an issue to a forge issue: its id, number and web page.
type Link struct{ Issue, ForgeID, Number, URL string }

// Links answers every sync entity's links, as their state rules hold them,
// by issue, each issue's sorted by URL.
// ponytail: folds every sync entity whole, as a run does; M6's snapshot
// serves this too.
func Links(s *gitstore.Store) (map[string][]Link, error) {
	ids, err := oplog.List(s, "sync")
	if err != nil {
		return nil, err
	}
	out := map[string][]Link{}
	for _, id := range ids {
		l, err := oplog.Load(s, "sync", id)
		if err != nil {
			return nil, err
		}
		st := emptyState()
		for _, e := range l.Entries {
			if err := st.apply(e); err != nil {
				return nil, err
			}
		}
		for _, d := range st.link {
			out[d.Issue] = append(out[d.Issue], Link(d))
		}
	}
	for _, links := range out {
		slices.SortFunc(links, func(a, b Link) int { return cmp.Compare(a.URL, b.URL) })
	}
	return out, nil
}
