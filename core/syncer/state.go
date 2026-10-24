// Package syncer moves issues between git-board and a forge.
package syncer

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/oplog"
)

// StateID is the id of a forge project's sync entity: the SHA-1 of its key,
// in hex.
func StateID(forgeKey string) string {
	h := sha1.Sum([]byte(forgeKey))
	return hex.EncodeToString(h[:])
}

// state is what a sync entity's operations add up to.
type state struct {
	log    *oplog.Log                   // nil when the entity doesn't exist
	links  map[string]string            // forge id -> issue id, the effective links
	linked map[string]bool              // issue ids with an effective link
	seen   map[string]map[string]string // issue id -> field -> last seen value
}

type linkData struct {
	Issue   string `json:"issue"`
	ForgeID string `json:"forge_id"`
	Number  string `json:"number"`
	URL     string `json:"url"`
}

type seenData struct {
	Issue string `json:"issue"`
	Field string `json:"field"`
	Value string `json:"value"`
}

func loadState(s *gitstore.Store, key string) (*state, error) {
	st := &state{links: map[string]string{}, linked: map[string]bool{}, seen: map[string]map[string]string{}}
	l, err := oplog.Load(s, "sync", StateID(key))
	if errors.Is(err, oplog.ErrNotFound) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	st.log = l
	for _, e := range l.Entries {
		if e.Author == "" {
			return nil, fmt.Errorf("syncer: operation %s has no author", e.ID)
		}
		switch e.Kind {
		case "link":
			var d linkData
			if err := json.Unmarshal(e.Data, &d); err != nil {
				return nil, fmt.Errorf("syncer: operation %s: %w", e.ID, err)
			}
			if _, taken := st.links[d.ForgeID]; !taken && !st.linked[d.Issue] {
				st.links[d.ForgeID] = d.Issue
				st.linked[d.Issue] = true
			}
		case "seen":
			var d seenData
			if err := json.Unmarshal(e.Data, &d); err != nil {
				return nil, fmt.Errorf("syncer: operation %s: %w", e.ID, err)
			}
			if !st.linked[d.Issue] || !knownField(d.Field) {
				continue
			}
			if st.seen[d.Issue] == nil {
				st.seen[d.Issue] = map[string]string{}
			}
			st.seen[d.Issue][d.Field] = d.Value
		default:
			return nil, fmt.Errorf("syncer: unknown operation %q, operation %s", e.Kind, e.ID)
		}
	}
	return st, nil
}

func knownField(f string) bool {
	switch f {
	case "title", "body", "status", "labels", "assignees":
		return true
	}
	return strings.HasPrefix(f, "comment:")
}

func linkOp(author string, now int64, issue, forgeID, number, url string) oplog.Op {
	raw, _ := json.Marshal(linkData{issue, forgeID, number, url})
	return oplog.Op{Kind: "link", Author: author, Time: now, Data: raw}
}

func seenOp(author string, now int64, issue, field, value string) oplog.Op {
	raw, _ := json.Marshal(seenData{issue, field, value})
	return oplog.Op{Kind: "seen", Author: author, Time: now, Data: raw}
}

// write appends ops to the entity, creating it if needed, and reloads st.
// It refuses an op with no author, which would break the entity.
// ponytail: it reloads the whole sync entity after each write, so a first
// pull of n issues is O(n²) reads; update links and seen in place when it
// matters (M6's mass first pull will).
func (st *state) write(ctx context.Context, s *gitstore.Store, key string, ops []oplog.Op) error {
	if len(ops) == 0 {
		return nil
	}
	for i, op := range ops {
		if op.Author == "" {
			return fmt.Errorf("syncer: operation %d has no author", i)
		}
	}
	if err := writeOps(ctx, s, StateID(key), st.log, ops); err != nil {
		return err
	}
	fresh, err := loadState(s, key)
	if err != nil {
		return err
	}
	*st = *fresh
	return nil
}

// writeOps appends ops to l, or creates the sync entity id with them when l
// is nil. A create that finds the entity made by another write since then
// appends to it instead; not there yet, the other write still holds the
// ref's lock, so it creates again. It tries 5 times, as oplog's writes do.
func writeOps(ctx context.Context, s *gitstore.Store, id string, l *oplog.Log, ops []oplog.Op) error {
	for try := 1; ; try++ {
		if l != nil {
			return l.Append(ctx, s, ops)
		}
		_, err := oplog.Create(ctx, s, "sync", id, ops)
		if !errors.Is(err, gitstore.ErrRefMoved) || try == 5 {
			return err
		}
		if l, err = oplog.Load(s, "sync", id); err != nil && !errors.Is(err, oplog.ErrNotFound) {
			return err
		}
	}
}
