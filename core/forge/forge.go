// Package forge is what sync needs of a forge project: its issues as they
// are now, and the writes a push makes.
package forge

import (
	"context"
	"errors"
	"fmt"
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

// Refused is a write the forge refused some values of, for good, Values
// as the write named them: every other value landed. It is ErrRefused.
type Refused struct {
	Values []string
	Why    string
}

func (e *Refused) Error() string {
	return fmt.Sprintf("forge: refused %q: %s", e.Values, e.Why)
}

func (e *Refused) Is(target error) bool { return target == ErrRefused }

// Visibility is who may read a project.
type Visibility string

const (
	Public   Visibility = "public"   // anyone
	Internal Visibility = "internal" // every user signed in to the forge
	Private  Visibility = "private"  // its members
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
// missing from it changed at Updated. A Partial issue, as a listing gives
// it, lacks its comments, and on some forges its Changed: Complete reads
// them.
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
	Partial          bool
}

// Ref names a forge issue: its id, which links are keyed on, and its
// number, which the forge's API names it by.
type Ref struct{ ID, Number string }

// Ref is the issue's.
func (is Issue) Ref() Ref { return Ref{is.ID, is.Number} }

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
	// Visibility says who may read the project.
	Visibility(ctx context.Context) (Visibility, error)
	// As is the same project through the account's own token, or
	// ErrNoToken.
	As(ctx context.Context, a Account) (Forge, error)

	// Issues lists the issues updated at or after since, in the forge's
	// order, without the confidential ones. since 0 lists them all. since is
	// the last run's start, as recorded: an adapter whose forge moves its
	// update times late, as GitLab does, lists from its own margin earlier.
	// An issue listed may be Partial: what costs a request per issue is
	// left to Complete, so that a listing takes a request per page.
	Issues(ctx context.Context, since int64) ([]Issue, error)
	// Complete reads what a listing left out of an issue it listed, and
	// gives back one that isn't Partial as it is.
	Complete(ctx context.Context, is Issue) (Issue, error)
	// Issue reads one issue, whole: ErrNotFound, or ErrConfidential. The writes
	// name it the same way, by its Ref.
	Issue(ctx context.Context, issue Ref) (Issue, error)

	// The writes. Each fails with ErrRefused when the forge refuses it for
	// good, and ErrNotFound when the issue or comment is gone.
	CreateIssue(ctx context.Context, title, body string) (Issue, error)
	CreateComment(ctx context.Context, issue Ref, body string) (Comment, error)
	EditComment(ctx context.Context, issue Ref, commentID, body string) error
	SetTitle(ctx context.Context, issue Ref, title string) error
	SetBody(ctx context.Context, issue Ref, body string) error
	SetStatus(ctx context.Context, issue Ref, status string) error
	// ChangeLabels adds and removes labels, leaving the others. A *Refused
	// names the labels it refused; the others landed.
	ChangeLabels(ctx context.Context, issue Ref, add, remove []string) error
	// SetAssignees sets the whole set, and returns what the forge kept.
	SetAssignees(ctx context.Context, issue Ref, users []Account) ([]Account, error)
}

// Replier is a forge whose comments form threads (GitLab's discussions):
// Reply posts a comment in the thread holding toCommentID, ErrNotFound when
// that comment is gone. GitHub's comments are flat; it has none.
type Replier interface {
	Reply(ctx context.Context, issue Ref, toCommentID, body string) (Comment, error)
}
