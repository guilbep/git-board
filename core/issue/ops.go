package issue

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/guilbep/git-board/core/gitstore"
	"github.com/guilbep/git-board/core/oplog"
)

// newOp makes an operation made now. Its data is a plain struct, which
// always marshals.
func newOp(kind, author string, d any) oplog.Op {
	raw, _ := json.Marshal(d)
	return oplog.Op{Kind: kind, Author: author, Time: time.Now().Unix(), Data: raw}
}

type m = map[string]any

func NewCreate(author, title, body string) oplog.Op {
	return newOp("create", author, m{"title": title, "body": body})
}

func NewComment(author, body string) oplog.Op {
	return newOp("comment", author, m{"body": body})
}

func EditText(author, target, body string) oplog.Op {
	return newOp("edit-text", author, m{"target": target, "body": body})
}

func SetTitle(author, title string) oplog.Op {
	return newOp("set-title", author, m{"title": title})
}

func SetStatus(author string, s Status) oplog.Op {
	return newOp("set-status", author, m{"status": s})
}

func ChangeLabels(author string, add, remove []string) oplog.Op {
	return newOp("change-labels", author, m{"add": add, "remove": remove})
}

// Create writes a new issue. The first operation must be a create.
func Create(s *gitstore.Store, ops ...oplog.Op) (*Issue, error) {
	if len(ops) == 0 || ops[0].Kind != "create" {
		return nil, errors.New("issue: the first operation must be a create")
	}
	l, err := oplog.Create(s, "issue", "", ops)
	if err != nil {
		return nil, err
	}
	return Apply(l)
}

// Load reads an issue.
func Load(s *gitstore.Store, id string) (*Issue, error) {
	l, err := oplog.Load(s, "issue", id)
	if err != nil {
		return nil, err
	}
	return Apply(l)
}

// List returns the ids of the issues, sorted.
func List(s *gitstore.Store) ([]string, error) { return oplog.List(s, "issue") }

// Do appends ops to an issue and returns its new state.
func Do(s *gitstore.Store, id string, ops ...oplog.Op) (*Issue, error) {
	l, err := oplog.Load(s, "issue", id)
	if err != nil {
		return nil, err
	}
	if err := l.Append(s, ops); err != nil {
		return nil, err
	}
	return Apply(l)
}
