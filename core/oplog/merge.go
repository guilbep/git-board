package oplog

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/guilbep/git-board/core/gitstore"
)

// Merge joins the entity's local head with other, a head from elsewhere,
// and returns the entity as it then is. other is read and checked before
// the ref moves. When the ref moved since it was read, it merges again with
// the new local head. The log is built from the commits already read.
func Merge(ctx context.Context, s *gitstore.Store, kind, id string, other gitstore.Hash) (*Log, error) {
	if err := checkName(kind, id); err != nil {
		return nil, err
	}
	theirs, err := walk(s, other)
	if err != nil {
		return nil, err
	}
	name := RefName(kind, id)
	for try := 1; ; try++ {
		head, err := s.Ref(name)
		if err != nil {
			return nil, err
		}
		next, nodes := other, theirs // no local ref, or other contains it
		if head != gitstore.ZeroHash {
			ours, err := walk(s, head)
			if err != nil {
				return nil, err
			}
			_, oursHasOther := ours[other]
			_, theirsHasHead := theirs[head]
			switch {
			case oursHasOther:
				next, nodes = head, ours
			case !theirsHasHead:
				p := pack{Clock: 1 + max(ours[head].Clock, theirs[other].Clock), Ops: []Op{}}
				c, n, err := writePack(s, p, head, other)
				if err != nil {
					return nil, err
				}
				nodes = maps.Clone(ours)
				maps.Copy(nodes, theirs)
				next, nodes[c] = c, n
			}
		}
		if next != head {
			err = updateRef(s, ctx, name, next, head)
			if errors.Is(err, gitstore.ErrRefMoved) && try < tries {
				continue
			}
			if err != nil {
				return nil, err
			}
		}
		return &Log{Kind: kind, ID: id, Head: next, Entries: order(nodes)}, nil
	}
}

// MergeFetched merges every refs/board-remotes/<remote>/<kind>/<id> into
// its local ref, which is created when absent. A ref there whose name isn't
// an id isn't an entity, and is left as it is. It returns how many local
// refs moved, failed merges included. An entity that fails to merge doesn't
// stop the others.
func MergeFetched(ctx context.Context, s *gitstore.Store, remote string) (int, error) {
	moved := 0
	var errs []error
	for _, kind := range gitstore.Kinds {
		prefix := "refs/board-remotes/" + remote + "/" + kind + "/"
		refs, err := s.Refs(prefix)
		if err != nil {
			return moved, errors.Join(append(errs, err)...)
		}
		for _, name := range slices.Sorted(maps.Keys(refs)) {
			if err := ctx.Err(); err != nil {
				return moved, errors.Join(append(errs, err)...)
			}
			id := strings.TrimPrefix(name, prefix)
			if !IsID(id) {
				continue
			}
			local := RefName(kind, id)
			before, err := s.Ref(local)
			if err != nil {
				return moved, errors.Join(append(errs, err)...)
			}
			if refs[name] == before {
				continue // nothing new
			}
			if _, err := Merge(ctx, s, kind, id, refs[name]); err != nil {
				errs = append(errs, fmt.Errorf("oplog: merge %s: %w", name, err))
			}
			after, err := s.Ref(local)
			if err != nil {
				return moved, errors.Join(append(errs, err)...)
			}
			if after != before {
				moved++
			}
		}
	}
	return moved, errors.Join(errs...)
}
