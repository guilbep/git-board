package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/guilbep/git-board/next/domain/issues"
	"github.com/guilbep/git-board/next/store"
	"github.com/guilbep/git-board/next/store/forgecopy"
	"github.com/guilbep/git-board/next/store/git"
	"github.com/guilbep/git-board/next/store/md"
	"github.com/guilbep/git-board/next/store/pg"
)

// NumbersRemote is the one remote that hands numbers out on git.
const NumbersRemote = "origin"

// DatastoreName is the board's datastore as a sentence names it: git,
// Markdown files, PostgreSQL.
func (b *Board) DatastoreName() string {
	switch b.Kernel.Store().(type) {
	case *git.Store, *forgecopy.Store:
		return "git"
	case *md.Store:
		return "Markdown files"
	case *pg.Store:
		return "PostgreSQL"
	}
	return "another datastore"
}

// inGit says the board is kept in the repository: its numbers are
// origin's counter's, handed out by a push.
func (b *Board) inGit() bool {
	_, ok := b.Kernel.Store().(*git.Store)
	return ok
}

// viaOrigin is numbering on git: origin's counter, fetched and pushed,
// which is the sync's (M4). Without origin, v1's refusal.
func (b *Board) viaOrigin(ctx context.Context) error {
	out, err := gitRun(ctx, b.Dir, "remote")
	if err != nil {
		return err
	}
	if !slices.Contains(strings.Fields(out), NumbersRemote) {
		return fmt.Errorf("no remote %q", NumbersRemote)
	}
	return missing(fmt.Sprintf("numbering through %s's counter comes with the sync (M4)", NumbersRemote))
}

// InitNumbers makes the project number its issues, prefix-N from start:
// the datastore's counter, started (numbers init). It answers where.
func (b *Board) InitNumbers(ctx context.Context, prefix string, start int) (string, error) {
	if err := issues.CheckNumbers(prefix, start); err != nil {
		return "", err
	}
	if b.inGit() {
		return "", b.viaOrigin(ctx)
	}
	err := b.Issues.InitNumbers(ctx, prefix, start)
	if errors.Is(err, issues.ErrNoCounter) {
		return "", missing("project numbers need a datastore that counts; this board is kept in " + b.DatastoreName() + " without one")
	}
	return "in " + b.DatastoreName(), sentence(err)
}

// Number numbers the issues without a number (numbers), and says how
// many.
func (b *Board) Number(ctx context.Context) (int, error) {
	if b.inGit() {
		return 0, b.viaOrigin(ctx)
	}
	n, err := b.Issues.Number(ctx)
	return n, sentence(err)
}

// sentence is what a datastore can't, as v1 says it: a Markdown folder
// off its default branch, its sentence alone.
func sentence(err error) error {
	if errors.Is(err, store.ErrMissing) {
		return missing(strings.TrimPrefix(err.Error(), store.ErrMissing.Error()+": "))
	}
	return err
}

// missing is what the datastore can't (ADR 0002 §3), said as v1 says it:
// the sentence alone.
func missing(sentence string) error {
	return &Error{Class: Missing, Code: "missing", Err: sentenceError{sentence}}
}

type sentenceError struct{ s string }

func (e sentenceError) Error() string { return e.s }
func (e sentenceError) Unwrap() error { return store.ErrMissing }
