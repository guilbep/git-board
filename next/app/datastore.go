package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/guilbep/git-board/next/store"
	"github.com/guilbep/git-board/next/store/git"
	"github.com/guilbep/git-board/next/store/md"
	"github.com/guilbep/git-board/next/store/pg"
)

// Kinds are every kind a datastore keeps, the ones v2 reads not yet too:
// a copy carries them all.
var Kinds = []string{"issue", "person", "release", "sync"}

// Where says where a datastore keeps the board, for a user to read, as
// v1 says it: git, PostgreSQL and its server without the password, or
// Markdown files and the folder.
func Where(ds store.Datastore) string {
	switch d := ds.(type) {
	case *pg.Store:
		return "PostgreSQL " + d.Where()
	case *md.Store:
		return "Markdown files " + d.Dir()
	case *git.Store:
		return "git"
	}
	return "another datastore"
}

// Kept is where the clone keeps its board, and what that datastore can
// do.
type Kept struct {
	Where   string
	Replica bool // pull, push, start over: git
	Counter bool // project numbers
}

// Datastore says where the clone at dir keeps its board.
func Datastore(ctx context.Context, dir string) (Kept, error) {
	g, err := git.Open(ctx, dir)
	if err != nil {
		return Kept{}, err
	}
	defer g.Close()
	set := settings(ctx, dir)
	if set.err != nil {
		return Kept{}, set.err
	}
	ds, err := OpenDatastore(ctx, g, dir, set.datastore)
	if err != nil {
		return Kept{}, err
	}
	defer ds.Close()
	_, inGit := ds.(*git.Store)
	return Kept{Where: Where(ds), Replica: inGit, Counter: ds.Caps().Counter != nil}, nil
}

// Copied is what a datastore copy did.
type Copied struct {
	store.Copied
	To string // where, as Where says it
}

// CopyDatastore copies the board kept in from into to, each a
// board.datastore value (datastores.md, Copying a board in): from ""
// is git, to "" the datastore the clone names. Only what to lacks is
// written, so a second copy writes nothing. from is left as it was.
func CopyDatastore(ctx context.Context, dir, from, to string) (Copied, error) {
	g, err := git.Open(ctx, dir)
	if err != nil {
		return Copied{}, err
	}
	defer g.Close()
	if to == "" {
		set := settings(ctx, dir)
		if set.err != nil {
			return Copied{}, set.err
		}
		to = set.datastore
	}
	if to == "" || to == "git" {
		if from == "" || from == "git" {
			return Copied{}, &Error{Class: Usage, Err: errors.New("the board is kept in git: name a datastore first (git config " + DatastoreKey + " postgres://…), or one with --to")}
		}
	}
	src, err := OpenDatastore(ctx, g, dir, from)
	if err != nil {
		return Copied{}, err
	}
	defer src.Close()
	dst, err := OpenDatastore(ctx, g, dir, to)
	if err != nil {
		return Copied{}, err
	}
	defer dst.Close()
	if Where(src) == Where(dst) {
		return Copied{}, &Error{Class: Usage, Err: fmt.Errorf("the board is copied into the datastore it is kept in: %s", Where(dst))}
	}
	c, err := store.Copy(ctx, src, dst, Kinds...)
	if errors.Is(err, store.ErrMissing) {
		err = &Error{Class: Missing, Code: "missing", Err: err}
	}
	return Copied{Copied: c, To: Where(dst)}, err
}
