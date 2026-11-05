package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/guilbep/git-board/conf"
	"github.com/guilbep/git-board/domain/issues"
	"github.com/guilbep/git-board/domain/issues/mdfiles"
	"github.com/guilbep/git-board/domain/people"
	"github.com/guilbep/git-board/kernel"
	"github.com/guilbep/git-board/view"

	"github.com/guilbep/git-board/store"
	"github.com/guilbep/git-board/store/forgecopy"
	"github.com/guilbep/git-board/store/git"
	"github.com/guilbep/git-board/store/md"
	"github.com/guilbep/git-board/store/pg"
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
	case *forgecopy.Store:
		return "the forges alone"
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
	if m, ok := dst.(*md.Store); ok && err == nil {
		err = renderFiles(ctx, dir, m)
	}
	return Copied{Copied: c, To: Where(dst)}, err
}

// renderFiles renders a Markdown folder's files from its packs, a board
// just copied in, through a view of its own: the clone's stays its
// datastore's.
func renderFiles(ctx context.Context, dir string, m *md.Store) error {
	f, err := conf.Load(configPath(dir))
	if err != nil {
		return err
	}
	schema, probs := issues.Declare(f).Schema()
	if len(probs) > 0 {
		return fmt.Errorf("%s: %w", conf.FileName, probs[0])
	}
	if err := os.MkdirAll(m.Folder(), 0o755); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(m.Folder(), "copy-view")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	v, err := view.Open(ctx, filepath.Join(tmp, "view.db"))
	if err != nil {
		return err
	}
	kb, err := kernel.Open(ctx, m, v, issues.Kind, people.Kind)
	if err != nil {
		v.Close()
		return err
	}
	defer v.Close()
	svc := &issues.Service{Board: kb, Schema: schema, Avatars: people.Forge, Folder: m.Folder()}
	return mdfiles.Open(m.Dir(), m.Folder(), m.LockFiles, svc, func(string, ...any) {}).Render(ctx)
}
