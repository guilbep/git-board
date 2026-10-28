package gitstore

import (
	"context"
	"fmt"
	"strings"
)

// Kinds are the entity kinds, listed once. git-board reads release's
// operations. refs/board/seq is git-board's own and is not one of them.
var Kinds = []string{"issue", "person", "sync", "release"}

// checkRemote refuses a remote name git would take for an option, or none.
func checkRemote(remote string) error {
	if remote == "" || strings.HasPrefix(remote, "-") {
		return fmt.Errorf("gitstore: bad remote name %q", remote)
	}
	return nil
}

// Fetch brings the remote's entity refs to refs/board-remotes/<remote>/<kind>/:
// out of refs/remotes/, so branch lists and git fetch --prune leave them.
// It prunes the copies of refs the remote no longer has: a merge of the
// copies would bring back what the remote deleted (git board push --prune).
func (s *Store) Fetch(ctx context.Context, remote string) error {
	if err := checkRemote(remote); err != nil {
		return err
	}
	args := []string{"fetch", "--prune", remote}
	for _, k := range Kinds {
		args = append(args, fmt.Sprintf("+refs/board/%s/*:refs/board-remotes/%s/%s/*", k, remote, k))
	}
	if _, err := s.git(ctx, args...); err != nil {
		return err
	}
	s.mu.Lock()
	s.fs.Reindex() // the fetch may have written a pack
	s.mu.Unlock()
	return nil
}

// Push sends the entity refs to the remote. It is not forced.
// git fails when no refspec matches a ref, so kinds with no ref are left
// out, and with none at all there is nothing to do.
func (s *Store) Push(ctx context.Context, remote string) error {
	if err := checkRemote(remote); err != nil {
		return err
	}
	args := []string{"push", remote}
	for _, k := range Kinds {
		refs, err := s.Refs("refs/board/" + k + "/")
		if err != nil {
			return err
		}
		if len(refs) > 0 {
			args = append(args, fmt.Sprintf("refs/board/%s/*:refs/board/%s/*", k, k))
		}
	}
	if len(args) == 2 {
		return nil
	}
	_, err := s.git(ctx, args...)
	return err
}
