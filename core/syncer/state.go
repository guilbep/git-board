// Package syncer moves issues between git-board and forges.
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
// ponytail: the sync entity is one log that grows forever, loaded whole at
// every run. M6's mass first pull needs a snapshot of this state, or the
// index to hold it, before it pulls thousands of issues.
type state struct {
	log      *oplog.Log                   // nil when the entity doesn't exist
	links    map[string]string            // forge id -> issue id, the effective links
	link     map[string]linkData          // issue id -> its effective link
	numbers  map[string]string            // forge number -> issue id, the effective links
	anyLink  map[string]bool              // issue ids with any link, effective or not
	seen     map[string]map[string]string // issue id -> field -> last seen value
	posted   map[[2]string]string         // (issue id, comment id) -> forge comment id
	postedBy map[string]string            // forge comment id -> git-board comment id
	refused  map[[3]string]bool           // (issue, field, value)
	lastRun  int64
	wrote    bool // a write since it loaded
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

type postedData struct {
	Issue   string `json:"issue"`
	Comment string `json:"comment"`
	ForgeID string `json:"forge_id"`
}

type refusedData struct {
	Issue  string `json:"issue"`
	Field  string `json:"field"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

type runData struct {
	Started int64 `json:"started"`
}

func emptyState() *state {
	return &state{links: map[string]string{}, link: map[string]linkData{}, numbers: map[string]string{},
		anyLink: map[string]bool{}, seen: map[string]map[string]string{}, posted: map[[2]string]string{},
		postedBy: map[string]string{}, refused: map[[3]string]bool{}}
}

func loadState(s *gitstore.Store, key string) (*state, error) {
	st := emptyState()
	l, err := oplog.Load(s, "sync", StateID(key))
	if errors.Is(err, oplog.ErrNotFound) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	st.log = l
	for _, e := range l.Entries {
		if err := st.apply(e); err != nil {
			return nil, err
		}
	}
	return st, nil
}

// apply folds one operation into the state, as data-model.md's state rules
// say.
func (st *state) apply(e oplog.Entry) error {
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
		var d linkData
		if err := decode(&d); err != nil {
			return err
		}
		st.anyLink[d.Issue] = true
		if _, taken := st.links[d.ForgeID]; !taken && st.link[d.Issue] == (linkData{}) {
			st.links[d.ForgeID] = d.Issue
			st.link[d.Issue] = d
			st.numbers[d.Number] = d.Issue
		}
	case "seen":
		var d seenData
		if err := decode(&d); err != nil {
			return err
		}
		if _, ok := st.link[d.Issue]; !ok || !knownField(d.Field) {
			return nil
		}
		if st.seen[d.Issue] == nil {
			st.seen[d.Issue] = map[string]string{}
		}
		st.seen[d.Issue][d.Field] = d.Value
	case "posted":
		var d postedData
		if err := decode(&d); err != nil {
			return err
		}
		_, linked := st.link[d.Issue]
		// a comment's id is its pack's hash and index: two issues can hold
		// the same one, the same text by the same person in the same second
		_, posted := st.posted[[2]string{d.Issue, d.Comment}]
		_, recorded := st.postedBy[d.ForgeID]
		if linked && !posted && !recorded {
			st.posted[[2]string{d.Issue, d.Comment}] = d.ForgeID
			st.postedBy[d.ForgeID] = d.Comment
		}
	case "refused":
		var d refusedData
		if err := decode(&d); err != nil {
			return err
		}
		st.refused[[3]string{d.Issue, d.Field, d.Value}] = true
	case "run":
		var d runData
		if err := decode(&d); err != nil {
			return err
		}
		st.lastRun = max(st.lastRun, d.Started)
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

func syncOp(kind, author string, now int64, d any) oplog.Op {
	raw, _ := json.Marshal(d)
	return oplog.Op{Kind: kind, Author: author, Time: now, Data: raw}
}

func linkOp(author string, now int64, issue, forgeID, number, url string) oplog.Op {
	return syncOp("link", author, now, linkData{issue, forgeID, number, url})
}

func seenOp(author string, now int64, issue, field, value string) oplog.Op {
	return syncOp("seen", author, now, seenData{issue, field, value})
}

func postedOp(author string, now int64, issue, comment, forgeID string) oplog.Op {
	return syncOp("posted", author, now, postedData{issue, comment, forgeID})
}

func refusedOp(author string, now int64, issue, field, value, reason string) oplog.Op {
	return syncOp("refused", author, now, refusedData{issue, field, value, reason})
}

func runOp(author string, now, started int64) oplog.Op {
	return syncOp("run", author, now, runData{started})
}

// write appends ops to the entity, creating it if needed, and folds them
// into st. It refuses an op with no author, which would break the entity.
// When another write landed in between, it reloads st whole.
func (st *state) write(ctx context.Context, s *gitstore.Store, key string, ops []oplog.Op) error {
	if len(ops) == 0 {
		return nil
	}
	for i, op := range ops {
		if op.Author == "" {
			return fmt.Errorf("syncer: operation %d has no author", i)
		}
	}
	n := 0
	if st.log != nil {
		n = len(st.log.Entries)
	}
	l, err := writeOps(ctx, s, StateID(key), st.log, ops)
	if err != nil {
		return err
	}
	if len(l.Entries) != n+len(ops) {
		fresh, err := loadState(s, key)
		if err != nil {
			return err
		}
		*st = *fresh
	} else {
		st.log = l
		for _, e := range l.Entries[n:] {
			if err := st.apply(e); err != nil {
				return err
			}
		}
	}
	st.wrote = true
	return nil
}

// writeOps appends ops to l, or creates the sync entity id with them when l
// is nil. A create that finds the entity made by another write since then
// appends to it instead; not there yet, the other write still holds the
// ref's lock, so it creates again. It tries 5 times, as oplog's writes do.
func writeOps(ctx context.Context, s *gitstore.Store, id string, l *oplog.Log, ops []oplog.Op) (*oplog.Log, error) {
	for try := 1; ; try++ {
		if l != nil {
			return l, l.Append(ctx, s, ops)
		}
		created, err := oplog.Create(ctx, s, "sync", id, ops)
		if err == nil {
			return created, nil
		}
		if !errors.Is(err, gitstore.ErrRefMoved) || try == 5 {
			return nil, err
		}
		if l, err = oplog.Load(s, "sync", id); err != nil && !errors.Is(err, oplog.ErrNotFound) {
			return nil, err
		}
	}
}
