package git

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/guilbep/git-board/next/kernel/pack"
	"github.com/guilbep/git-board/next/store"
)

// A remote's board (storage.md, Fetch and push): its entity refs are
// fetched to copies, refs/board-remotes/<remote>/<kind>/<id>, out of
// refs/remotes/, so branch lists and git fetch --prune leave them; a merge
// joins each copy with the local ref; a push sends the local refs, never
// forced. As v1's gitstore and oplog.Merge.

var remoteName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)

// checkRemote refuses a name git would read as an option, or that would
// leave refs/board-remotes/.
func checkRemote(remote string) error {
	if !remoteName.MatchString(remote) || strings.Contains(remote, "..") {
		return fmt.Errorf("not a remote: %q", remote)
	}
	return nil
}

func copyPrefix(remote, kind string) string { return "refs/board-remotes/" + remote + "/" + kind + "/" }

// Fetch brings the remote's entity refs of kinds to their copies here,
// pruning the copies of refs it no longer has (a merge of the copies would
// bring back what it deleted), and answers the copies' heads.
func (s *Store) Fetch(ctx context.Context, remote string, kinds ...string) ([]store.Head, error) {
	if err := checkRemote(remote); err != nil {
		return nil, err
	}
	args := []string{"fetch", "--prune", "--quiet", remote}
	for _, k := range kinds {
		args = append(args, "+refs/board/"+k+"/*:"+copyPrefix(remote, k)+"*")
	}
	if _, err := s.git(ctx, nil, args...); err != nil {
		return nil, err
	}
	return s.copies(ctx, remote, kinds)
}

// copies are the heads of the remote's copies here, entities alone.
func (s *Store) copies(ctx context.Context, remote string, kinds []string) ([]store.Head, error) {
	var out []store.Head
	for _, k := range kinds {
		refs, err := s.refs(ctx, copyPrefix(remote, k))
		if err != nil {
			return nil, err
		}
		for _, name := range slices.Sorted(maps.Keys(refs)) {
			if id := strings.TrimPrefix(name, copyPrefix(remote, k)); store.IsID(id) {
				out = append(out, store.Head{Kind: k, ID: id, Version: refs[name]})
			}
		}
	}
	return out, nil
}

// refs are the refs under prefix and their commits.
func (s *Store) refs(ctx context.Context, prefix string) (map[string]string, error) {
	out, err := s.git(ctx, nil, "for-each-ref", "--format=%(objectname) %(refname)", prefix)
	if err != nil {
		return nil, err
	}
	m := map[string]string{}
	for line := range strings.SplitSeq(strings.TrimSpace(string(out)), "\n") {
		if h, name, ok := strings.Cut(line, " "); ok {
			m[name] = h
		}
	}
	return m, nil
}

// Merge merges each of the remote's copies of kinds into its local ref,
// created when absent, and answers how many local refs moved. An entity
// that fails to merge doesn't stop the others.
func (s *Store) Merge(ctx context.Context, remote string, kinds ...string) (int, error) {
	if err := checkRemote(remote); err != nil {
		return 0, err
	}
	heads, err := s.copies(ctx, remote, kinds)
	if err != nil {
		return 0, err
	}
	moved := 0
	var errs []error
	for _, h := range heads {
		if err := ctx.Err(); err != nil {
			return moved, errors.Join(append(errs, err)...)
		}
		ok, err := s.merge(ctx, h.Kind, h.ID, h.Version)
		if err != nil {
			errs = append(errs, fmt.Errorf("git: merge %s%s: %w", copyPrefix(remote, h.Kind), h.ID, err))
		}
		if ok {
			moved++
		}
	}
	return moved, errors.Join(errs...)
}

// merge joins the entity's local head with other: other when there is no
// local head or other holds it, nothing when the head holds other, else a
// merge commit on both whose pack is empty, its clock one more than the
// higher of theirs. When the ref moved meanwhile, it merges again.
func (s *Store) merge(ctx context.Context, kind, id, other string) (bool, error) {
	name := refName(kind, id)
	for try := 1; ; try++ {
		refs, err := s.refs(ctx, name)
		if err != nil {
			return false, err
		}
		head := refs[name]
		next := other
		switch {
		case head == other:
			return false, nil
		case head == "":
			head = zero
		case s.holds(ctx, head, other):
			return false, nil
		case !s.holds(ctx, other, head):
			ours, err := s.node(ctx, head)
			if err != nil {
				return false, err
			}
			theirs, err := s.node(ctx, other)
			if err != nil {
				return false, err
			}
			data, err := pack.Encode(pack.Pack{Clock: 1 + max(ours.clock, theirs.clock), Ops: []pack.Op{}})
			if err != nil {
				return false, err
			}
			if next, err = s.writePack(data, []string{head, other}); err != nil {
				return false, err
			}
		}
		err = s.updateRef(ctx, name, next, head)
		if errors.Is(err, store.ErrMoved) && try < 5 {
			continue
		}
		return err == nil, err
	}
}

// holds says commit a has b in its history, or is it.
func (s *Store) holds(ctx context.Context, a, b string) bool {
	_, err := s.git(ctx, nil, "merge-base", "--is-ancestor", b, a)
	return err == nil
}

// Push sends the local refs of kinds to the remote, not forced, then
// fetches them back, so that the copies here say what it has. A kind with
// no ref is left out: git fails a refspec that matches none.
func (s *Store) Push(ctx context.Context, remote string, kinds ...string) error {
	if err := checkRemote(remote); err != nil {
		return err
	}
	args := []string{"push", "--quiet", remote}
	var sent []string
	for _, k := range kinds {
		refs, err := s.refs(ctx, "refs/board/"+k+"/")
		if err != nil {
			return err
		}
		if len(refs) > 0 {
			args = append(args, "refs/board/"+k+"/*:refs/board/"+k+"/*")
			sent = append(sent, k)
		}
	}
	if len(sent) == 0 {
		return nil
	}
	_, err := s.git(ctx, nil, args...)
	_, ferr := s.Fetch(ctx, remote, kinds...)
	return errors.Join(err, ferr)
}

// Ahead counts what a merge of the remote's copies of kind would receive
// and what a push would send: an entity the other side lacks, or whose
// head there its history doesn't hold.
func (s *Store) Ahead(ctx context.Context, remote, kind string) (receive, send int, err error) {
	if err := checkRemote(remote); err != nil {
		return 0, 0, err
	}
	localPrefix, theirPrefix := "refs/board/"+kind+"/", copyPrefix(remote, kind)
	local, err := s.refs(ctx, localPrefix)
	if err != nil {
		return 0, 0, err
	}
	theirs, err := s.refs(ctx, theirPrefix)
	if err != nil {
		return 0, 0, err
	}
	for name, mine := range local {
		their, ok := theirs[theirPrefix+strings.TrimPrefix(name, localPrefix)]
		switch {
		case !ok:
			send++
		case their == mine:
		default: // one side's history holds the other's: only it moves
			if !s.holds(ctx, their, mine) {
				send++
			}
			if !s.holds(ctx, mine, their) {
				receive++
			}
		}
	}
	for name := range theirs {
		if _, ok := local[localPrefix+strings.TrimPrefix(name, theirPrefix)]; !ok {
			receive++
		}
	}
	return receive, send, nil
}
