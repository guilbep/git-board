// Package forge is what sync needs of a forge: a project's issues as they
// are now.
package forge

import "context"

// Account is a user of a forge.
type Account struct{ ID, Login string }

// Comment is a comment on a forge issue. Time is in Unix seconds.
type Comment struct {
	ID, Body string
	Author   Account
	Time     int64
}

// Issue is a forge issue. Status is open or closed. Created is in Unix
// seconds.
type Issue struct {
	ID, Number, URL string
	Author          Account
	Created         int64
	Title, Body     string
	Status          string
	Labels          []string
	Comments        []Comment
}

// Forge is one forge project. This is the trial's subset; M3 widens it.
type Forge interface {
	// Key names the project, like github.com/guilbep/git-board-test.
	Key() string
	// Issues lists the project's issues, in the forge's order.
	Issues(ctx context.Context) ([]Issue, error)
}
