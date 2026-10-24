package people

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/oplog"
)

func newOp(kind, author string, d any) oplog.Op {
	raw, _ := json.Marshal(d)
	return oplog.Op{Kind: kind, Author: author, Time: time.Now().Unix(), Data: raw}
}

type m = map[string]any

func NewCreate(author, name, email string) oplog.Op {
	return newOp("create", author, m{"name": name, "email": email})
}

func SetName(author, name string) oplog.Op { return newOp("set-name", author, m{"name": name}) }

func SetEmail(author, email string) oplog.Op { return newOp("set-email", author, m{"email": email}) }

func AddAccount(author string, a Account) oplog.Op {
	return newOp("add-account", author, m{"server": a.Server, "id": a.ID, "login": a.Login})
}

func RemoveAccount(author, server, id string) oplog.Op {
	return newOp("remove-account", author, m{"server": server, "id": id})
}

// Create writes a new person. The first operation must be a create. Every
// operation is in the first pack, so any may have an empty author.
func Create(ctx context.Context, s *gitstore.Store, ops ...oplog.Op) (*Person, error) {
	if len(ops) == 0 || ops[0].Kind != "create" {
		return nil, errors.New("people: the first operation must be a create")
	}
	// ponytail: the id "pending" makes the pending ids "pending:<i>" first-pack ones.
	if _, err := Apply(&oplog.Log{ID: "pending", Entries: pending(nil, ops)}); err != nil {
		return nil, err
	}
	l, err := oplog.Create(ctx, s, "person", "", ops)
	if err != nil {
		return nil, err
	}
	return Apply(l)
}

// Load reads a person.
func Load(s *gitstore.Store, id string) (*Person, error) {
	l, err := oplog.Load(s, "person", id)
	if err != nil {
		return nil, err
	}
	return Apply(l)
}

// List returns the ids of the persons, sorted.
func List(s *gitstore.Store) ([]string, error) { return oplog.List(s, "person") }

// Do appends ops to a person and returns its new state.
func Do(ctx context.Context, s *gitstore.Store, id string, ops ...oplog.Op) (*Person, error) {
	l, err := oplog.Load(s, "person", id)
	if err != nil {
		return nil, err
	}
	if _, err := Apply(&oplog.Log{ID: l.ID, Entries: pending(l.Entries, ops)}); err != nil {
		return nil, err
	}
	if err := l.Append(ctx, s, ops); err != nil {
		return nil, err
	}
	return Apply(l)
}

// pending is entries followed by ops, which have no id yet.
func pending(entries []oplog.Entry, ops []oplog.Op) []oplog.Entry {
	out := slices.Clone(entries)
	for i, op := range ops {
		out = append(out, oplog.Entry{Op: op, ID: fmt.Sprintf("pending:%d", i)})
	}
	return out
}

// Me is the person id in the local config key board.me, or "" when unset.
func Me(ctx context.Context, s *gitstore.Store) (string, error) { return s.Config(ctx, "board.me") }

// SetMe sets board.me. It doesn't check the id.
func SetMe(ctx context.Context, s *gitstore.Store, id string) error {
	return s.SetConfig(ctx, "board.me", id)
}
