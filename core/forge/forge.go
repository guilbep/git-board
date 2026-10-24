// Package forge is what sync needs of a forge project: its issues as they
// are now, and the writes a push makes.
package forge

import (
	"context"
	"errors"
)

var (
	// ErrNotFound: no such issue or comment, or no access to it.
	ErrNotFound = errors.New("forge: not found")
	// ErrConfidential: the issue is confidential, and sync never reads it.
	ErrConfidential = errors.New("forge: confidential issue")
	// ErrRefused: the forge refused the write for good, as the account may
	// not make it.
	ErrRefused = errors.New("forge: refused")
	// ErrNoToken: the account has no token on this forge.
	ErrNoToken = errors.New("forge: no token")
)

// Account is a user of a forge.
type Account struct{ ID, Login string }

// Comment is a comment on a forge issue. Times are in Unix seconds; Edited
// is its text's last edit, 0 if never.
type Comment struct {
	ID, Body     string
	Author       Account
	Time, Edited int64
}

// Issue is a forge issue. Status is open or closed. Times are in Unix
// seconds. Changed holds when title, body and status last changed; a field
// missing from it changed at Updated.
type Issue struct {
	ID, Number, URL  string
	Author           Account
	Created, Updated int64
	Title, Body      string
	Status           string
	Labels           []string
	Assignees        []Account
	Comments         []Comment
	Changed          map[string]int64
}

// Forge is one forge project, through one account's token. An adapter
// maps the forge's answers to the errors above, and paces its own writes.
// ponytail: rate limits, their pacing and their retries are the adapters'
// (M4); the engine has none.
type Forge interface {
	// Key names the project, like github.com/guilbep/git-board-test.
	Key() string
	// Server is the forge's host, like github.com. With a user's id, it
	// names the user's account.
	Server() string
	// Me is the token's owner.
	Me(ctx context.Context) (Account, error)
	// Public says whether anyone may read the project.
	Public(ctx context.Context) (bool, error)
	// As is the same project through the account's own token, or
	// ErrNoToken.
	As(ctx context.Context, a Account) (Forge, error)

	// Issues lists the issues updated at or after since, in the forge's
	// order, without the confidential ones. since 0 lists them all. since is
	// the last run's start, as recorded: an adapter whose forge moves its
	// update times late, as GitLab does, lists from its own margin earlier.
	Issues(ctx context.Context, since int64) ([]Issue, error)
	// Issue reads one issue: ErrNotFound, or ErrConfidential.
	Issue(ctx context.Context, id string) (Issue, error)

	// The writes. Each fails with ErrRefused when the forge refuses it for
	// good, and ErrNotFound when the issue or comment is gone.
	CreateIssue(ctx context.Context, title, body string) (Issue, error)
	CreateComment(ctx context.Context, issueID, body string) (Comment, error)
	EditComment(ctx context.Context, issueID, commentID, body string) error
	SetTitle(ctx context.Context, issueID, title string) error
	SetBody(ctx context.Context, issueID, body string) error
	SetStatus(ctx context.Context, issueID, status string) error
	// ChangeLabels adds and removes labels, leaving the others.
	ChangeLabels(ctx context.Context, issueID string, add, remove []string) error
	// SetAssignees sets the whole set, and returns what the forge kept.
	SetAssignees(ctx context.Context, issueID string, users []Account) ([]Account, error)
}
