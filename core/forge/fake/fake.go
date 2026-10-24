// Package fake is a forge kept in memory, for tests.
package fake

import (
	"context"
	"slices"
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

// Add appends an issue.
func (f *Forge) Add(is forge.Issue) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.issues = append(f.issues, clone(is))
}

// Edit changes the issue with this number. It does nothing if there is none.
func (f *Forge) Edit(number string, change func(*forge.Issue)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i := range f.issues {
		if f.issues[i].Number == number {
			change(&f.issues[i])
		}
	}
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
