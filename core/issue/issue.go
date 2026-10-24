// Package issue folds an issue's operations into its state.
package issue

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/guilbep/git-board/core/oplog"
)

// Status is an issue's status.
type Status string

const (
	Open   Status = "open"
	Closed Status = "closed"
)

// Comment is one comment of an issue. ID is its operation's id.
type Comment struct {
	ID, Author, Body string
	Time             int64
	Origin           *oplog.Origin
}

// Issue is the state its operations add up to. BodyOp is the create
// operation's id, the target of an edit of the body.
type Issue struct {
	ID, Author, Title, Body, BodyOp string
	Created                         int64
	Status                          Status
	Labels                          []string // sorted, unique
	Comments                        []Comment
	Origin                          *oplog.Origin
}

var (
	ErrUnknownOp = errors.New("issue: unknown operation")
	ErrBadTarget = errors.New("issue: edit of an unknown text")
)

// data is the union of the fields of every operation's data.
type data struct {
	Title  string   `json:"title"`
	Body   string   `json:"body"`
	Target string   `json:"target"`
	Status Status   `json:"status"`
	Add    []string `json:"add"`
	Remove []string `json:"remove"`
}

// Apply folds the log's operations, in order, into an issue.
func Apply(l *oplog.Log) (*Issue, error) {
	var is *Issue
	labels := map[string]bool{}
	comments := map[string]int{} // comment id -> index in is.Comments
	for _, e := range l.Entries {
		var d data
		if len(e.Data) > 0 {
			if err := json.Unmarshal(e.Data, &d); err != nil {
				return nil, fmt.Errorf("issue: operation %s: %w", e.ID, err)
			}
		}
		if is == nil && e.Kind != "create" {
			return nil, fmt.Errorf("issue: the first operation is %q, want create", e.Kind)
		}
		switch e.Kind {
		case "create":
			if is != nil {
				return nil, fmt.Errorf("issue: second create, operation %s", e.ID)
			}
			is = &Issue{ID: l.ID, Author: e.Author, Title: d.Title, Body: d.Body, BodyOp: e.ID,
				Created: e.Time, Status: Open, Origin: e.Origin}
		case "comment":
			comments[e.ID] = len(is.Comments)
			is.Comments = append(is.Comments, Comment{ID: e.ID, Author: e.Author, Body: d.Body, Time: e.Time, Origin: e.Origin})
		case "edit-text":
			if d.Target == is.BodyOp {
				is.Body = d.Body
			} else if i, ok := comments[d.Target]; ok {
				is.Comments[i].Body = d.Body
			} else {
				return nil, fmt.Errorf("%w: %q, operation %s", ErrBadTarget, d.Target, e.ID)
			}
		case "set-title":
			is.Title = d.Title
		case "set-status":
			if d.Status != Open && d.Status != Closed {
				return nil, fmt.Errorf("issue: operation %s: bad status %q", e.ID, d.Status)
			}
			is.Status = d.Status
		case "change-labels":
			for _, a := range d.Add {
				labels[a] = true
			}
			for _, r := range d.Remove {
				delete(labels, r)
			}
		default:
			return nil, fmt.Errorf("%w: %q, operation %s", ErrUnknownOp, e.Kind, e.ID)
		}
	}
	if is == nil {
		return nil, errors.New("issue: no operation")
	}
	is.Labels = []string{}
	for k := range labels {
		is.Labels = append(is.Labels, k)
	}
	slices.Sort(is.Labels)
	return is, nil
}
