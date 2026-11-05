package app

import (
	"errors"
	"strings"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/domain/people"
	"github.com/guilbep/git-board/next/kernel"
)

// Class is what kind of failure an error is: one table gives each its
// exit code and its HTTP status (design.md, Errors and exit codes).
type Class int

const (
	Internal  Class = iota
	Usage           // a wrong request: a flag, a value, a config key
	NotFound        // no such entity, or an ambiguous prefix
	NoMe            // no one to write as
	Changed         // the text changed meanwhile
	Busy            // a sync or a release runs; try again
	Missing         // the datastore can't (ADR 0002 §3)
	Forbidden       // the server's host and origin checks (ADR 0013)
	Off             // an experimental feature is off
)

// Error is an error with its class and the API's stable code.
type Error struct {
	Class Class
	Code  string // "changed", "missing"…; "" for none
	Err   error  // what the user reads
}

func (e *Error) Error() string { return e.Err.Error() }
func (e *Error) Unwrap() error { return e.Err }

// Classify is err's class: its own when it is an *Error, else the
// domains' errors' (a not found, an ambiguous prefix, no one to write
// as, a usage error, a text changed meanwhile).
func Classify(err error) Class {
	var e *Error
	switch {
	case errors.As(err, &e):
		return e.Class
	case errors.Is(err, kernel.ErrNotFound), errors.Is(err, kernel.ErrAmbiguous), errors.Is(err, issues.ErrNoSuchComment):
		return NotFound
	case errors.Is(err, people.ErrNoMe):
		return NoMe
	case errors.Is(err, issues.ErrChanged):
		return Changed
	case errors.Is(err, issues.ErrUsage), strings.HasPrefix(err.Error(), "unknown command"):
		return Usage
	}
	return Internal
}

// ExitCode is the exit code of err's class (docs/guide/cli.md, Exit
// codes).
func ExitCode(err error) int {
	switch Classify(err) {
	case Usage:
		return 2
	case NotFound:
		return 3
	case NoMe:
		return 4
	case Changed:
		return 5
	}
	return 1
}

// HTTPStatus is the HTTP status of err's class.
func HTTPStatus(err error) int {
	switch Classify(err) {
	case Usage:
		return 400
	case NotFound, Off:
		return 404
	case NoMe, Changed, Busy, Missing:
		return 409
	case Forbidden:
		return 403
	}
	return 500
}
