package issue

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

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

// NewCommentMeta is a comment with git-board's metadata.
func NewCommentMeta(author, body string, meta map[string]string) oplog.Op {
	return newOp("comment", author, m{"body": body, "meta": meta})
}

// SetMeta sets metadata on the body (target the create operation) or a
// comment.
func SetMeta(author, target string, meta map[string]string) oplog.Op {
	return newOp("set-meta", author, m{"target": target, "meta": meta})
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

// Delete hides the issue (target its create operation) or a comment. It
// is a tombstone: the text stays in the log (docs/adr/0019).
func Delete(author, target string) oplog.Op {
	return newOp("delete", author, m{"target": target})
}

// IsRemoved says a delete hid the comment with this id.
func (is *Issue) IsRemoved(id string) bool {
	return slices.ContainsFunc(is.Removed, func(c Comment) bool { return c.ID == id })
}

// Create writes a new issue. The first operation must be a create.
func Create(ctx context.Context, s oplog.Datastore, ops ...oplog.Op) (*Issue, error) {
	if len(ops) == 0 || ops[0].Kind != "create" {
		return nil, errors.New("issue: the first operation must be a create")
	}
	if _, err := Apply(&oplog.Log{Entries: pending(nil, ops)}); err != nil {
		return nil, err
	}
	l, err := oplog.Create(ctx, s, "issue", "", ops)
	if err != nil {
		return nil, err
	}
	return Apply(l)
}

// Load reads an issue: LoadContext with no deadline.
func Load(s oplog.Datastore, id string) (*Issue, error) {
	return LoadContext(context.Background(), s, id)
}

// LoadContext reads an issue; ctx bounds the datastore's read.
func LoadContext(ctx context.Context, s oplog.Datastore, id string) (*Issue, error) {
	l, err := oplog.LoadContext(ctx, s, "issue", id)
	if err != nil {
		return nil, err
	}
	return Apply(l)
}

// LoadAt reads an issue at version, one its datastore's Heads gave
// (oplog.LoadAt).
func LoadAt(ctx context.Context, s oplog.Datastore, id, version string) (*Issue, error) {
	l, err := oplog.LoadAt(ctx, s, "issue", id, version)
	if err != nil {
		return nil, err
	}
	return Apply(l)
}

// List returns the ids of the issues, sorted.
func List(s oplog.Datastore) ([]string, error) { return oplog.List(s, "issue") }

// ListContext is List; ctx bounds the datastore's read.
func ListContext(ctx context.Context, s oplog.Datastore) ([]string, error) {
	return oplog.ListContext(ctx, s, "issue")
}

// Do appends ops to an issue and returns its new state.
func Do(ctx context.Context, s oplog.Datastore, id string, ops ...oplog.Op) (*Issue, error) {
	l, err := oplog.LoadContext(ctx, s, "issue", id)
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

// pending is entries followed by ops, which have no id yet. Applying it
// checks ops before they are written.
func pending(entries []oplog.Entry, ops []oplog.Op) []oplog.Entry {
	out := slices.Clone(entries)
	for i, op := range ops {
		out = append(out, oplog.Entry{Op: op, ID: fmt.Sprintf("pending:%d", i)})
	}
	return out
}
