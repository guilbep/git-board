package syncer

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/oplog"
)

// Link joins an issue to a forge issue: its id, number and web page.
type Link struct{ Issue, ForgeID, Number, URL string }

// Record is a link with what its sync entity holds of the issue: the value
// each field had on the forge when they last agreed, texts without sync's
// header and marker, and the forge comment each comment sync posted is.
type Record struct {
	Link
	// Forge is the sync entity's id: StateID of the project's key.
	Forge string
	Seen  map[string]string // field -> last seen
	// Posted is the entity's: each comment sync posted there, git-board's
	// id to the forge's. Comment ids name one issue's comment only.
	Posted map[string]string
}

// Links answers every sync entity's links, as their state rules hold them,
// by issue, each issue's sorted by URL.
func Links(s *gitstore.Store) (map[string][]Link, error) {
	recs, err := Records(s)
	if err != nil {
		return nil, err
	}
	out := map[string][]Link{}
	for id, rs := range recs {
		for _, r := range rs {
			out[id] = append(out[id], r.Link)
		}
	}
	return out, nil
}

// Records is Links with each link's record.
// ponytail: folds every sync entity whole, as a run does; a snapshot of
// the state would serve this too.
func Records(s *gitstore.Store) (map[string][]Record, error) {
	ids, err := oplog.List(s, "sync")
	if err != nil {
		return nil, err
	}
	out := map[string][]Record{}
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
		posted := map[string]map[string]string{} // issue -> its posted comments
		for k, f := range st.posted {
			if posted[k[0]] == nil {
				posted[k[0]] = map[string]string{}
			}
			posted[k[0]][k[1]] = f
		}
		for issue, d := range st.link {
			seen := maps.Clone(st.seen[issue])
			for f, v := range seen {
				if f == "body" || strings.HasPrefix(f, "comment:") {
					seen[f] = unwrap(v).body
				}
			}
			out[issue] = append(out[issue], Record{Link: Link(d), Forge: id, Seen: seen, Posted: posted[issue]})
		}
	}
	for _, rs := range out {
		slices.SortFunc(rs, func(a, b Record) int { return cmp.Compare(a.URL, b.URL) })
	}
	return out, nil
}
