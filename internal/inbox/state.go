// Package inbox is one identity's inbox: what it follows and has seen (its
// file, in git-bug's local storage), and the rows: what happened that
// concerns it (rows.go). Pure but for the file; the server gathers the
// threads.
package inbox

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"github.com/git-bug/git-bug/repository"

	"github.com/guilbep/git-board/internal/gitbugio"
)

// State is one identity's file.
type State struct {
	Seen     map[string][]string `json:"seen,omitempty"`     // issue id → the event ids seen
	Follow   []string            `json:"follow,omitempty"`   // issues followed by hand
	Unfollow []string            `json:"unfollow,omitempty"` // and unfollowed
	Views    []string            `json:"views,omitempty"`    // views followed, by name
	Fields   bool                `json:"fields,omitempty"`   // also field changes on issues you're in
}

func path(identityID string) string {
	return filepath.Join("git-board", "inbox", identityID+".json")
}

// Load reads identityID's file; none yet is an empty State.
func Load(fs repository.LocalStorage, identityID string) (State, error) {
	f, err := fs.Open(path(identityID))
	if errors.Is(err, os.ErrNotExist) {
		return State{}, nil
	}
	if err != nil {
		return State{}, err
	}
	defer f.Close()
	var s State
	if err := json.NewDecoder(f).Decode(&s); err != nil {
		return State{}, fmt.Errorf("reading %s: %w (deleting it starts over, your follows with it)", path(identityID), err)
	}
	return s, nil
}

// Save writes it through a temporary file and a rename: a crash leaves the
// old file or the new one, never half of one.
func Save(fs repository.LocalStorage, identityID string, s State) error {
	p := path(identityID)
	if err := fs.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	f, err := fs.Create(p + ".tmp")
	if err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(s); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return fs.Rename(p+".tmp", p)
}

// MarkSeen takes every event th has now as seen, counted or not.
func (s *State) MarkSeen(th gitbugio.Thread) {
	if s.Seen == nil {
		s.Seen = map[string][]string{}
	}
	ids := make([]string, len(th.Events))
	for i, e := range th.Events {
		ids[i] = e.ID
	}
	s.Seen[th.ID] = ids
}

// SetFollow is Follow (true), Unfollow (false), or back to automatic (nil).
func (s *State) SetFollow(id string, follow *bool) {
	other := func(x string) bool { return x == id }
	s.Follow = slices.DeleteFunc(s.Follow, other)
	s.Unfollow = slices.DeleteFunc(s.Unfollow, other)
	switch {
	case follow == nil:
	case *follow:
		s.Follow = append(s.Follow, id)
	default:
		s.Unfollow = append(s.Unfollow, id)
	}
}

// Set is what you clicked for an issue: true, false, or nil (automatic).
func (s State) Set(id string) *bool {
	yes, no := true, false
	switch {
	case slices.Contains(s.Follow, id):
		return &yes
	case slices.Contains(s.Unfollow, id):
		return &no
	}
	return nil
}

// Prune forgets what was seen of the issues not in keep: out of the
// window, they can't show again.
func (s *State) Prune(keep map[string]bool) {
	for id := range s.Seen {
		if !keep[id] {
			delete(s.Seen, id)
		}
	}
}
