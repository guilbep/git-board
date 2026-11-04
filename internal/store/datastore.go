package store

import (
	"errors"
	"os"

	"github.com/guilbep/git-board/core/datastore"
)

// inGit says the board is kept in git: what only git does (a remote's fetch,
// push and merge, the reset and switched marks, the git counter) works.
func (b *Backend) inGit() bool { return b.ds == b.s }

// missing is the error for what the board's datastore can't do: the
// sentence the CLI prints and the server answers.
func (b *Backend) missing(what string) error {
	return datastore.Missing(what + " needs a board kept in git; this board is kept in " + b.kind())
}

// kind names where the board is kept, for a user to read.
func (b *Backend) kind() string {
	if n, ok := b.ds.(interface{ Name() string }); ok {
		return n.Name()
	}
	if b.inGit() {
		return "git"
	}
	return "another datastore"
}

// needGit is nil for a board kept in git, else missing(what).
func (b *Backend) needGit(what string) error {
	if b.inGit() {
		return nil
	}
	return b.missing(what)
}

// copyJournal copies the sync journal at from to to, when there is one.
func copyJournal(from, to string) error {
	data, err := os.ReadFile(from)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return os.WriteFile(to, data, 0o644)
}
