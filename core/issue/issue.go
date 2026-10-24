// Package issue folds an issue's operations into its state.
package issue

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

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
	Time, Edited     int64 // Edited: the time of the last edit-text, 0 if none
	Origin           *oplog.Origin
}

// Issue is the state its operations add up to. BodyOp is the create
// operation's id, the target of an edit of the body.
type Issue struct {
	ID, Author, Title, Body, BodyOp string
	Created, Updated                int64 // Updated: the time of the last operation in the order
	Status                          Status
	Labels                          []string // sorted, unique
	Comments                        []Comment
	Origin                          *oplog.Origin
	// Changes holds, for each field, the operation that last changed it:
	// title, body, status, label:<label> and comment:<comment id>.
	Changes map[string]Change
}

// Change is who last changed a field, when, and from where.
type Change struct {
	Author string
	Time   int64
	Origin *oplog.Origin
}

var (
	ErrUnknownOp = errors.New("issue: unknown operation")
	ErrBadTarget = errors.New("issue: edit of an unknown text")
)

var kinds = []string{"create", "comment", "edit-text", "set-title", "set-status", "change-labels"}

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
	comments := map[string]int{}      // comment id -> index in is.Comments
	origins := map[oplog.Origin]int{} // a comment's origin -> its index
	for _, e := range l.Entries {
		if e.Author == "" {
			return nil, fmt.Errorf("issue: operation %s has no author", e.ID)
		}
		if !slices.Contains(kinds, e.Kind) {
			return nil, fmt.Errorf("%w: %q, operation %s", ErrUnknownOp, e.Kind, e.ID)
		}
		var d data
		if len(e.Data) > 0 {
			if err := json.Unmarshal(e.Data, &d); err != nil {
				return nil, fmt.Errorf("issue: operation %s: %w", e.ID, err)
			}
		}
		if is == nil && e.Kind != "create" {
			return nil, fmt.Errorf("issue: the first operation is %q, want create", e.Kind)
		}
		ch := Change{e.Author, e.Time, e.Origin}
		switch e.Kind {
		case "create":
			if is != nil {
				return nil, fmt.Errorf("issue: second create, operation %s", e.ID)
			}
			is = &Issue{ID: l.ID, Author: e.Author, Title: d.Title, Body: d.Body, BodyOp: e.ID,
				Created: e.Time, Status: Open, Origin: e.Origin,
				Changes: map[string]Change{"title": ch, "body": ch, "status": ch}}
		case "comment":
			// A comment with an earlier comment's origin is that comment,
			// pulled again: its id names the earlier one.
			i, dup := len(is.Comments), false
			if e.Origin != nil {
				if j, ok := origins[*e.Origin]; ok {
					i, dup = j, true
				} else {
					origins[*e.Origin] = i
				}
			}
			if !dup {
				is.Comments = append(is.Comments, Comment{ID: e.ID, Author: e.Author, Body: d.Body, Time: e.Time, Origin: e.Origin})
				is.Changes["comment:"+e.ID] = ch
			}
			comments[e.ID] = i
		case "edit-text":
			if strings.HasPrefix(d.Target, "pending:") {
				// The id of an operation not written yet, which Create and Do
				// check with: stored, it would name nothing.
				return nil, fmt.Errorf("%w: %q, operation %s", ErrBadTarget, d.Target, e.ID)
			}
			if d.Target == is.BodyOp {
				is.Body = d.Body
				is.Changes["body"] = ch
			} else if i, ok := comments[d.Target]; ok {
				is.Comments[i].Body = d.Body
				is.Comments[i].Edited = e.Time
				is.Changes["comment:"+is.Comments[i].ID] = ch
			} else {
				return nil, fmt.Errorf("%w: %q, operation %s", ErrBadTarget, d.Target, e.ID)
			}
		case "set-title":
			is.Title = d.Title
			is.Changes["title"] = ch
		case "set-status":
			if d.Status != Open && d.Status != Closed {
				return nil, fmt.Errorf("issue: operation %s: bad status %q", e.ID, d.Status)
			}
			is.Status = d.Status
			is.Changes["status"] = ch
		case "change-labels":
			for _, label := range slices.Concat(d.Add, d.Remove) {
				if label == "" || strings.Contains(label, "\n") {
					return nil, fmt.Errorf("issue: operation %s: bad label %q", e.ID, label)
				}
			}
			for _, a := range d.Add {
				labels[a] = true
				is.Changes["label:"+a] = ch
			}
			for _, r := range d.Remove {
				delete(labels, r)
				is.Changes["label:"+r] = ch
			}
		default:
			return nil, fmt.Errorf("%w: %q, operation %s", ErrUnknownOp, e.Kind, e.ID)
		}
		is.Updated = e.Time
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
