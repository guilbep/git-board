package oplog

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/guilbep/git-board/core/gitstore"
)

// kinds are the entity kinds. Other refs under refs/board/, like
// git-board's refs/board/seq, are not entities.
var kinds = []string{"issue", "person", "sync"}

// Merge joins the entity's local head with other, a head from elsewhere,
// and returns the entity as it then is. other is read and checked before
// the ref moves.
func Merge(s *gitstore.Store, kind, id string, other gitstore.Hash) (*Log, error) {
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
		next := other // no local ref, or other contains it
		if head != gitstore.ZeroHash {
			ours, err := walk(s, head)
			if err != nil {
				return nil, err
			}
			_, oursHasOther := ours[other]
			_, theirsHasHead := theirs[head]
			switch {
			case oursHasOther:
				next = head
			case !theirsHasHead:
				p := pack{Clock: 1 + max(ours[head].Clock, theirs[other].Clock), Ops: []Op{}}
				if next, _, err = writePack(s, p, head, other); err != nil {
					return nil, err
				}
			}
		}
		if next != head {
			err = s.UpdateRef(name, next, head)
			if errors.Is(err, gitstore.ErrRefMoved) && try < tries {
				continue
			}
			if err != nil {
				return nil, err
			}
		}
		return Load(s, kind, id)
	}
}

// MergeFetched merges every refs/board-remotes/<remote>/<kind>/<id> into
// its local ref, which is created when absent. It returns how many refs
// moved. An entity that fails to merge doesn't stop the others.
func MergeFetched(s *gitstore.Store, remote string) (int, error) {
	moved := 0
	var errs []error
	for _, kind := range kinds {
		prefix := "refs/board-remotes/" + remote + "/" + kind + "/"
		refs, err := s.Refs(prefix)
		if err != nil {
			return moved, err
		}
		for _, name := range slices.Sorted(maps.Keys(refs)) {
			id := strings.TrimPrefix(name, prefix)
			if strings.Contains(id, "/") {
				continue
			}
			before, err := s.Ref(RefName(kind, id))
			if err != nil {
				return moved, err
			}
			if refs[name] == before {
				continue // nothing new
			}
			l, err := Merge(s, kind, id, refs[name])
			if err != nil {
				errs = append(errs, fmt.Errorf("oplog: merge %s: %w", name, err))
				continue
			}
			if l.Head != before {
				moved++
			}
		}
	}
	return moved, errors.Join(errs...)
}
