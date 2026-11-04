// Command git-board keeps a repository's issues as its own: operations in
// git (or PostgreSQL, or Markdown files), synced with GitHub and GitLab,
// read and written from the command line, a TUI and a web UI.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guilbep/git-board/internal/fields"
	"github.com/guilbep/git-board/internal/store"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(report(os.Stderr, err))
	}
}

// The exit codes, for scripts (docs/guide/cli.md, Exit codes).
const (
	exitError    = 1 // anything else
	exitUsage    = 2 // a wrong flag or argument, an assignment refused
	exitNotFound = 3 // no such issue, comment or person; an ambiguous prefix
	exitNoMe     = 4 // no one chosen to write as: git board me
	exitConflict = 5 // someone changed the text meanwhile
)

// usageError is a command line the command can't run: a flag, the
// arguments.
type usageError struct{ error }

func (e usageError) Unwrap() error { return e.error }

// report prints err to w, with a hint when one helps, and returns the exit
// code it means.
func report(w io.Writer, err error) int {
	fmt.Fprintln(w, "error:", err)
	code := exitCode(err)
	if code == exitNoMe {
		fmt.Fprintln(w, "hint: git board me --new writes as you (git user.name and user.email); git board me --ls lists who is on the board, git board me <id> writes as one of them")
	}
	return code
}

func exitCode(err error) int {
	var usage usageError
	switch {
	case errors.Is(err, store.ErrNoMe):
		return exitNoMe
	case errors.Is(err, store.ErrChangedMeanwhile):
		return exitConflict
	case errors.Is(err, store.ErrNotFound), errors.Is(err, store.ErrAmbiguous), errors.Is(err, store.ErrNoSuchComment):
		return exitNotFound
	case errors.As(err, &usage), errors.Is(err, fields.ErrInvalid), errors.Is(err, fields.ErrUnchecked),
		strings.HasPrefix(err.Error(), "unknown command"):
		return exitUsage
	}
	return exitError
}

// usageErrors marks the errors of a wrong command line, every command's:
// its flags and its arguments.
func usageErrors(root *cobra.Command) {
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error { return usageError{err} })
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		if args := c.Args; args != nil {
			c.Args = func(cmd *cobra.Command, a []string) error {
				if err := args(cmd, a); err != nil {
					return usageError{err}
				}
				return nil
			}
		}
		for _, sub := range c.Commands() {
			walk(sub)
		}
	}
	walk(root)
}
