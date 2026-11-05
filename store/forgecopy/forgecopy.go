// Package forgecopy is the datastore of a board kept on the forges alone
// (board.datastore forge, ADR 0002): a copy the sync fills, in a bare git
// repository of the clone's own folder, <common dir>/board/forge-copy.git,
// in git's format (docs/spec/storage.md). The clone's refs/board/* are
// never written: the forges are the board, the copy is what this machine
// read of them.
package forgecopy

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/guilbep/git-board/store"
	"github.com/guilbep/git-board/store/git"
)

// Name is board.datastore's value for a board kept on the forges alone.
const Name = "forge"

// Store is the forges' copy.
type Store struct {
	*git.Store
	folder string // the clone's own
}

var _ store.Datastore = (*Store)(nil)

// Dir is the copy's repository in the clone's folder.
func Dir(folder string) string { return filepath.Join(folder, "forge-copy.git") }

// Open opens the copy in folder, the clone's own, making it when it
// isn't there.
func Open(ctx context.Context, folder string) (*Store, error) {
	dir := Dir(folder)
	if _, err := os.Stat(filepath.Join(dir, "HEAD")); errors.Is(err, fs.ErrNotExist) {
		if err := os.MkdirAll(folder, 0o755); err != nil {
			return nil, err
		}
		if out, err := exec.CommandContext(ctx, "git", "init", "-q", "--bare", dir).CombinedOutput(); err != nil {
			return nil, fmt.Errorf("making the forge-only copy: %w: %s", err, strings.TrimSpace(string(out)))
		}
	}
	g, err := git.OpenGitDir(dir) // inside the clone's .git: never the clone found above it
	if err != nil {
		return nil, fmt.Errorf("opening the forge-only copy: %w", err)
	}
	return &Store{Store: g, folder: folder}, nil
}

// Caps: git's, but no counter: the forges number their issues.
func (s *Store) Caps() store.Caps {
	c := s.Store.Caps()
	c.Counter = nil
	return c
}

// Folder is the clone's folder, not the copy's.
func (s *Store) Folder() string { return s.folder }
