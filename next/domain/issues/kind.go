// Package issues is the issue domain: the issue kind (its operations and
// state, docs/spec/data-model.md › Issues), its fields as labels (ADR
// 0007), its filter grammar, and an issue as every surface shows it: in a
// list (Issue), with its history (Thread), change by change (Log). The
// JSON tags are a stable format (docs/guide/cli.md, JSON output).
package issues

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/guilbep/git-board/next/kernel"
)

// Status is an issue's status: "open" or "closed".
type Status string

const (
	Open   Status = "open"
	Closed Status = "closed"
)

// Comment is one comment. ID is its operation's id.
type Comment struct {
	ID           kernel.OpID
	Author       kernel.ID
	Body         string
	Time, Edited int64 // Edited: the last edit-text's time, 0 if none
	Origin       *kernel.Origin
	Meta         map[string]string // nil if none
}

// Change is who last changed a field, when, and from where.
type Change struct {
	Author kernel.ID
	Time   int64
	Origin *kernel.Origin
}

// State is what an issue's operations add up to (data-model.md › Issue
// state).
type State struct {
	ID               kernel.ID
	Author           kernel.ID
	Title, Body      string
	BodyOp           kernel.OpID // the create's: the body's target
	Created, Updated int64       // Updated: the last operation's time, in the order
	Status           Status
	Labels           []string // sorted, unique
	Comments         []Comment
	Origin           *kernel.Origin
	Meta             map[string]string // the create's, as set-meta left it
	// Changes is, for each field, the operation that last changed it:
	// title, body, status, label:<label>, comment:<id>, deleted.
	Changes map[string]Change
	Deleted bool      // a delete hid it (ADR 0019)
	Removed []Comment // the comments a delete hid, without text or meta

	made     bool                  // create was applied
	labels   map[string]bool       // Labels, as a set
	all      []Comment             // every comment, hidden ones too, by index
	comments map[kernel.OpID]int   // a comment's ids -> its index
	origins  map[kernel.Origin]int // a comment's origin -> its index
	removed  map[int]bool          // a comment's index -> hidden
}

// Kind is the issue kind.
var Kind = kernel.Define(kernel.Spec[State, Row]{
	Name:  "issue",
	Rev:   1,
	New:   func(id kernel.ID) State { return State{ID: id} },
	Apply: apply,
	Index: index,
	Row:   rowCodec{},
})

var (
	ErrUnknownOp = errors.New("issue: unknown operation")
	ErrBadTarget = errors.New("issue: edit of an unknown text")
)

var ops = []string{"create", "comment", "edit-text", "set-title", "set-status", "change-labels", "set-meta", "delete"}

// data is the union of every operation's data.
type data struct {
	Title  string            `json:"title"`
	Body   string            `json:"body"`
	Target kernel.OpID       `json:"target"`
	Status Status            `json:"status"`
	Add    []string          `json:"add"`
	Remove []string          `json:"remove"`
	Meta   map[string]string `json:"meta"`
}

// apply is one operation, as data-model.md › Issue state says.
func apply(is *State, e kernel.Entry) error {
	if e.Author == "" {
		return fmt.Errorf("issue: operation %s has no author", e.ID)
	}
	if !slices.Contains(ops, e.Kind) {
		return fmt.Errorf("%w: %q, operation %s", ErrUnknownOp, e.Kind, e.ID)
	}
	var d data
	if len(e.Data) > 0 {
		if err := json.Unmarshal(e.Data, &d); err != nil {
			return fmt.Errorf("issue: operation %s: %w", e.ID, err)
		}
	}
	if !is.made && e.Kind != "create" {
		return fmt.Errorf("issue: the first operation is %q, want create", e.Kind)
	}
	ch := Change{kernel.ID(e.Author), e.Time, e.Origin}
	// a target: the body's, or a comment met earlier, by any of its ids
	target := func() (int, bool, error) {
		if strings.HasPrefix(string(d.Target), "pending:") {
			return 0, false, fmt.Errorf("%w: %q, operation %s", ErrBadTarget, d.Target, e.ID)
		}
		if d.Target == is.BodyOp {
			return -1, true, nil
		}
		i, ok := is.comments[d.Target]
		if !ok {
			return 0, false, fmt.Errorf("%w: %q, operation %s", ErrBadTarget, d.Target, e.ID)
		}
		return i, !is.removed[i], nil
	}
	switch e.Kind {
	case "create":
		if is.made {
			return fmt.Errorf("issue: second create, operation %s", e.ID)
		}
		*is = State{ID: is.ID, Author: ch.Author, Title: d.Title, Body: d.Body, BodyOp: e.ID,
			Created: e.Time, Status: Open, Origin: e.Origin, made: true,
			Changes: map[string]Change{"title": ch, "body": ch, "status": ch},
			Labels:  []string{}, labels: map[string]bool{}, comments: map[kernel.OpID]int{}}
	case "comment":
		// a comment with an earlier comment's origin is that comment,
		// pulled again: its id names the earlier one
		i, dup := len(is.all), false
		if e.Origin != nil {
			if is.origins == nil {
				is.origins = map[kernel.Origin]int{}
			}
			if j, ok := is.origins[*e.Origin]; ok {
				i, dup = j, true
			} else {
				is.origins[*e.Origin] = i
			}
		}
		if !dup {
			is.all = append(is.all, Comment{ID: e.ID, Author: ch.Author, Body: d.Body, Time: e.Time, Origin: e.Origin, Meta: d.Meta})
			is.Changes["comment:"+string(e.ID)] = ch
		}
		is.comments[e.ID] = i
		is.settleComments()
	case "edit-text":
		i, live, err := target()
		switch {
		case err != nil:
			return err
		case i < 0:
			is.Body = d.Body
			is.Changes["body"] = ch
		case live: // an edit of a hidden comment, from a clone that hadn't its delete, changes nothing
			c := &is.all[i]
			c.Body, c.Edited = d.Body, e.Time
			is.Changes["comment:"+string(c.ID)] = ch
			is.settleComments()
		}
	case "set-meta":
		i, live, err := target()
		if err != nil {
			return err
		}
		meta := &is.Meta
		if i >= 0 {
			if !live {
				break
			}
			meta = &is.all[i].Meta
		}
		if *meta == nil {
			*meta = map[string]string{}
		}
		maps.Copy(*meta, d.Meta)
		is.settleComments()
	case "delete":
		i, live, err := target()
		switch {
		case err != nil:
			return err
		case i < 0:
			is.Deleted = true
			is.Changes["deleted"] = ch
		case live:
			if is.removed == nil {
				is.removed = map[int]bool{}
			}
			is.removed[i] = true
			c := &is.all[i]
			c.Body, c.Meta, c.Edited = "", nil, 0
			delete(is.Changes, "comment:"+string(c.ID))
			is.settleComments()
		}
	case "set-title":
		is.Title = d.Title
		is.Changes["title"] = ch
	case "set-status":
		if d.Status != Open && d.Status != Closed {
			return fmt.Errorf("issue: operation %s: bad status %q", e.ID, d.Status)
		}
		is.Status = d.Status
		is.Changes["status"] = ch
	case "change-labels":
		for _, l := range slices.Concat(d.Add, d.Remove) {
			if l == "" || strings.Contains(l, "\n") {
				return fmt.Errorf("issue: operation %s: bad label %q", e.ID, l)
			}
		}
		for _, a := range d.Add {
			is.labels[a] = true
			is.Changes["label:"+a] = ch
		}
		for _, r := range d.Remove {
			delete(is.labels, r)
			is.Changes["label:"+r] = ch
		}
		is.Labels = slices.Sorted(maps.Keys(is.labels))
		if is.Labels == nil {
			is.Labels = []string{}
		}
	}
	is.Updated = e.Time
	return nil
}

// settleComments makes Comments and Removed what the comments so far
// are: the live ones, and the hidden ones without their text.
func (is *State) settleComments() {
	if len(is.removed) == 0 {
		is.Comments = is.all
		return
	}
	is.Comments, is.Removed = make([]Comment, 0, len(is.all)-len(is.removed)), nil
	for i, c := range is.all {
		if is.removed[i] {
			is.Removed = append(is.Removed, c)
		} else {
			is.Comments = append(is.Comments, c)
		}
	}
}
