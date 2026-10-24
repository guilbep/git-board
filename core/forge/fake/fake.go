// Package fake is a forge kept in memory, for tests.
package fake

import (
	"context"
	"slices"
	"strings"
	"sync"

	"github.com/guilbep/git-board/core/forge"
)

// Forge is a forge in memory. Its issues stay in the order they were added.
type Forge struct {
	key    string
	mu     sync.Mutex
	issues []forge.Issue
}

// New makes an empty forge with the given key.
func New(key string) *Forge { return &Forge{key: key} }

func (f *Forge) Key() string { return f.key }

// Server is the key up to its first /.
func (f *Forge) Server() string {
	host, _, _ := strings.Cut(f.key, "/")
	return host
}

// Add appends an issue.
func (f *Forge) Add(is forge.Issue) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issues = append(f.issues, clone(is))
}

// Edit changes the first issue with this number. It is false when there is
// none.
func (f *Forge) Edit(number string, change func(*forge.Issue)) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := slices.IndexFunc(f.issues, func(is forge.Issue) bool { return is.Number == number })
	if i < 0 {
		return false
	}
	change(&f.issues[i])
	f.issues[i] = clone(f.issues[i]) // the change may have put the caller's slices in
	return true
}

// Issues returns copies of the issues.
func (f *Forge) Issues(context.Context) ([]forge.Issue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]forge.Issue, len(f.issues))
	for i, is := range f.issues {
		out[i] = clone(is)
	}
	return out, nil
}

func clone(is forge.Issue) forge.Issue {
	is.Labels = slices.Clone(is.Labels)
	is.Comments = slices.Clone(is.Comments)
	return is
}
