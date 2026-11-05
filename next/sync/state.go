// Package sync keeps a board's kinds the same as on the forges they are
// linked to (docs/spec/sync.md, docs/rewrite/design.md, Sync): one
// engine over mappers. Its records are operations in the sync entities,
// refs/board/sync/<SHA-1 of the forge key>; this file folds them, as
// data-model.md's state rules say.
package sync

import (
	"cmp"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
	gosync "sync"

	"github.com/guilbep/git-board/next/kernel"
)

// StateID is the id of a forge project's sync entity: the SHA-1 of its
// key, in hex.
func StateID(forgeKey string) kernel.ID {
	h := sha1.Sum([]byte(forgeKey))
	return kernel.ID(hex.EncodeToString(h[:]))
}

// State is what a sync entity's operations add up to.
type State struct {
	ID       kernel.ID
	Links    map[string]string            // forge id -> issue id, the effective links
	Link     map[string]LinkData          // issue id -> its effective link
	Numbers  map[string]string            // forge number -> issue id, the effective links
	AnyLink  map[string]bool              // issue ids with any link, effective or not
	Seen     map[string]map[string]string // issue id -> field -> last seen value
	Posted   map[[2]string]string         // (issue id, comment id) -> forge comment id
	PostedBy map[string]string            // forge comment id -> git-board comment id
	Refused  map[[3]string]bool           // (issue, field, value)
	LastRun  int64
}

// LinkData is a link operation's data.
type LinkData struct {
	Issue   string `json:"issue"`
	ForgeID string `json:"forge_id"`
	Number  string `json:"number"`
	URL     string `json:"url"`
}

// SeenData is a seen operation's.
type SeenData struct {
	Issue string `json:"issue"`
	Field string `json:"field"`
	Value string `json:"value"`
}

// PostedData is a posted operation's.
type PostedData struct {
	Issue   string `json:"issue"`
	Comment string `json:"comment"`
	ForgeID string `json:"forge_id"`
}

// RefusedData is a refused operation's.
type RefusedData struct {
	Issue  string `json:"issue"`
	Field  string `json:"field"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

// RunData is a run operation's.
type RunData struct {
	Started int64 `json:"started"`
}

// New is an empty state.
func New(id kernel.ID) State {
	return State{ID: id, Links: map[string]string{}, Link: map[string]LinkData{}, Numbers: map[string]string{},
		AnyLink: map[string]bool{}, Seen: map[string]map[string]string{}, Posted: map[[2]string]string{},
		PostedBy: map[string]string{}, Refused: map[[3]string]bool{}}
}

// Kind is the sync entities' kind. Its ids are given: StateID of the
// forge key. The view keeps nothing of it: the records are read whole
// (Records), as a run reads them.
var Kind = kernel.Define(kernel.Spec[State, struct{}]{
	Name:  "sync",
	Given: true,
	New:   New,
	Apply: func(st *State, e kernel.Entry) error { return st.Apply(e) },
	Index: func(*State) kernel.Doc[struct{}] { return kernel.Doc[struct{}]{} },
})

// Apply folds one operation into the state, as data-model.md's state
// rules say.
func (st *State) Apply(e kernel.Entry) error {
	if e.Author == "" {
		return fmt.Errorf("syncer: operation %s has no author", e.ID)
	}
	decode := func(v any) error {
		if err := json.Unmarshal(e.Data, v); err != nil {
			return fmt.Errorf("syncer: operation %s: %w", e.ID, err)
		}
		return nil
	}
	switch e.Kind {
	case "link":
		var d LinkData
		if err := decode(&d); err != nil {
			return err
		}
		st.AnyLink[d.Issue] = true
		if _, taken := st.Links[d.ForgeID]; !taken && st.Link[d.Issue] == (LinkData{}) {
			st.Links[d.ForgeID] = d.Issue
			st.Link[d.Issue] = d
			st.Numbers[d.Number] = d.Issue
		}
	case "seen":
		var d SeenData
		if err := decode(&d); err != nil {
			return err
		}
		if _, ok := st.Link[d.Issue]; !ok || !knownField(d.Field) {
			return nil
		}
		if st.Seen[d.Issue] == nil {
			st.Seen[d.Issue] = map[string]string{}
		}
		st.Seen[d.Issue][d.Field] = d.Value
	case "posted":
		var d PostedData
		if err := decode(&d); err != nil {
			return err
		}
		_, linked := st.Link[d.Issue]
		// a comment's id is its pack's hash and index: two issues can hold
		// the same one, the same text by the same person in the same second
		_, posted := st.Posted[[2]string{d.Issue, d.Comment}]
		_, recorded := st.PostedBy[d.ForgeID]
		if linked && !posted && !recorded {
			st.Posted[[2]string{d.Issue, d.Comment}] = d.ForgeID
			st.PostedBy[d.ForgeID] = d.Comment
		}
	case "refused":
		var d RefusedData
		if err := decode(&d); err != nil {
			return err
		}
		st.Refused[[3]string{d.Issue, d.Field, d.Value}] = true
	case "run":
		var d RunData
		if err := decode(&d); err != nil {
			return err
		}
		st.LastRun = max(st.LastRun, d.Started)
	default:
		return fmt.Errorf("syncer: unknown operation %q, operation %s", e.Kind, e.ID)
	}
	return nil
}

func knownField(f string) bool {
	switch f {
	case "title", "body", "status", "labels", "assignees":
		return true
	}
	return strings.HasPrefix(f, "comment:")
}

// Record is a link with what its sync entity holds of the issue: the
// value each field had on the forge when they last agreed, texts without
// sync's header and marker, and the forge comment each comment sync
// posted is.
type Record struct {
	LinkData
	// Forge is the sync entity's id: StateID of the project's key.
	Forge kernel.ID
	Seen  map[string]string // field -> last seen
	// Posted is each comment sync posted there, git-board's id to the
	// forge's. Comment ids name one issue's comment only.
	Posted map[string]string
}

// Records are every sync entity's effective links with their records,
// by issue, each issue's sorted by URL. An entity that doesn't fold is
// told to broken and left out, as a query leaves a broken entity out.
func Records(ctx context.Context, b *kernel.Board, broken func(id kernel.ID, err error)) (map[string][]Record, error) {
	var mu gosync.Mutex
	states := map[kernel.ID]State{}
	err := Kind.Logs(ctx, b, func(id kernel.ID, log []kernel.Entry, err error) {
		var st State
		if err == nil {
			st, err = Kind.Fold(id, log)
		}
		mu.Lock()
		defer mu.Unlock()
		if err != nil {
			if broken != nil {
				broken(id, err)
			}
			return
		}
		states[id] = st
	})
	if err != nil {
		return nil, err
	}
	out := map[string][]Record{}
	for _, id := range slices.Sorted(maps.Keys(states)) {
		out = recordsOf(states[id], out)
	}
	for _, rs := range out {
		slices.SortStableFunc(rs, func(a, b Record) int { return cmp.Compare(a.URL, b.URL) })
	}
	return out, nil
}

// recordsOf adds the records of st's effective links to out.
func recordsOf(st State, out map[string][]Record) map[string][]Record {
	posted := map[string]map[string]string{} // issue -> its posted comments
	for k, f := range st.Posted {
		if posted[k[0]] == nil {
			posted[k[0]] = map[string]string{}
		}
		posted[k[0]][k[1]] = f
	}
	for issue, d := range st.Link {
		seen := make(map[string]string, len(st.Seen[issue]))
		for f, v := range st.Seen[issue] {
			if f == "body" || strings.HasPrefix(f, "comment:") {
				v = Unwrap(v).Body
			}
			seen[f] = v
		}
		out[issue] = append(out[issue], Record{LinkData: d, Forge: st.ID, Seen: seen, Posted: posted[issue]})
	}
	return out
}
