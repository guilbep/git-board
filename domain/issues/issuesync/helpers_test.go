package issuesync

import (
	"context"
	"testing"

	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/domain/people"
	"github.com/guilbep/git-board/kernel"
	"github.com/guilbep/git-board/kernel/pack"
	bsync "github.com/guilbep/git-board/sync"
)

// Delete hides the issue (target its create) or a comment.
func Delete(author, target string) kernel.Op { return newOp("delete", author, m{"target": target}) }

// loadDirectory is every person's directory.
func loadDirectory(s *kernel.Board) (*Directory, error) {
	ids, err := ListPersons(s)
	if err != nil {
		return nil, err
	}
	d := personNewDirectory()
	for _, id := range ids {
		p, err := LoadPerson(s, id)
		if err != nil {
			return nil, err
		}
		d.Put(p)
	}
	return d, nil
}

// entityLog is an entity's operations in order, as v1's oplog.Load gave.
type entityLog struct{ Entries []kernel.Entry }

func loadLog(s *kernel.Board, kind, id string) (*entityLog, error) {
	var log []kernel.Entry
	var err error
	switch kind {
	case "issue":
		log, _, err = issues.Kind.Log(context.Background(), s, kernel.ID(id))
	case "person":
		log, _, err = people.Kind.Log(context.Background(), s, kernel.ID(id))
	}
	if err != nil {
		return nil, err
	}
	return &entityLog{log}, nil
}

var _ = testing.Short

// Link is a link, as v1's tests write one.
type Link = bsync.LinkData

// Records are the sync entities' records.
func Records(s *kernel.Board) (map[string][]bsync.Record, error) {
	return bsync.Records(context.Background(), s, nil)
}

// Links are the sync entities' effective links, by issue.
func Links(s *kernel.Board) (map[string][]Link, error) {
	recs, err := Records(s)
	if err != nil {
		return nil, err
	}
	out := map[string][]Link{}
	for id, rs := range recs {
		for _, r := range rs {
			out[id] = append(out[id], r.LinkData)
		}
	}
	return out, nil
}

// stateLog is the sync entity's operations, in order.
func stateLog(t *testing.T, s *kernel.Board, key string) []kernel.Entry {
	t.Helper()
	log, _, err := bsync.Kind.Log(context.Background(), s, bsync.StateID(key))
	if err != nil {
		t.Fatal(err)
	}
	return log
}

// rawCreate writes an entity's first pack as is, past every check: a
// broken one, as another writer may leave.
func rawCreate(t *testing.T, s *kernel.Board, kind string, id kernel.ID, ops []kernel.Op) (kernel.ID, error) {
	t.Helper()
	data, err := pack.Encode(pack.Pack{Clock: 1, Ops: ops})
	if err != nil {
		return "", err
	}
	if id == "" {
		id = kernel.ID(pack.Hash(data))
	}
	_, err = s.Store().Add(context.Background(), kind, string(id), data, "")
	return id, err
}
