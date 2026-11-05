// Package mem is a datastore in memory, for tests and tools: it keeps
// each entity's packs in the order they were added, and a version is how
// many it had, so a version names its packs for good, as a git commit
// does.
package mem

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/guilbep/git-board/kernel/pack"
	"github.com/guilbep/git-board/store"
)

// Store is a board in memory.
type Store struct {
	mu       sync.Mutex
	entities map[string]map[string]*entity // by kind, by id
	folder   string
}

type entity struct {
	packs [][]byte
	clock uint64 // the highest of its packs'
}

var _ store.Datastore = (*Store)(nil)

// New is an empty board whose own folder is folder.
func New(folder string) *Store {
	return &Store{entities: map[string]map[string]*entity{}, folder: folder}
}

func (s *Store) Caps() store.Caps { return store.Caps{Pinned: true} }
func (s *Store) Folder() string   { return s.folder }
func (s *Store) Close() error     { return nil }

// Heads lists the kinds' entities, sorted by kind then id.
func (s *Store) Heads(_ context.Context, kinds ...string) ([]store.Head, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []store.Head
	for _, kind := range kinds {
		for id, e := range s.entities[kind] {
			out = append(out, store.Head{Kind: kind, ID: id, Version: strconv.Itoa(len(e.packs))})
		}
	}
	slices.SortFunc(out, func(a, b store.Head) int { return cmp.Or(strings.Compare(a.Kind, b.Kind), strings.Compare(a.ID, b.ID)) })
	return out, nil
}

// Load gives each ref's packs at its version.
func (s *Store) Load(_ context.Context, refs []store.Ref, each func(store.Ref, [][]byte, error) error) error {
	for _, r := range refs {
		s.mu.Lock()
		e := s.entities[r.Kind][r.ID]
		var packs [][]byte
		var err error
		switch n, perr := strconv.Atoi(r.Version); {
		case e == nil:
			err = fmt.Errorf("%w: %s %s", store.ErrNotFound, r.Kind, r.ID)
		case r.Version == "":
			packs, r.Version = e.packs, strconv.Itoa(len(e.packs))
		case perr != nil || n < 1 || n > len(e.packs):
			err = fmt.Errorf("%w: %s %s at %q", store.ErrNotFound, r.Kind, r.ID, r.Version)
		default:
			packs = e.packs[:n:n]
		}
		s.mu.Unlock()
		if err := each(r, packs, err); err != nil {
			return err
		}
	}
	return nil
}

// Add appends a pack if the entity is at prev; its clock must be one more
// than the entity's.
func (s *Store) Add(_ context.Context, kind, id string, data []byte, prev string) (string, error) {
	if !store.IsID(id) || kind == "" || strings.ContainsAny(kind, "/.") {
		return "", fmt.Errorf("mem: bad name %s %s", kind, id)
	}
	clock, empty, err := pack.Header(data)
	if err != nil {
		return "", fmt.Errorf("mem: %s %s: pack: %w", kind, id, err)
	}
	if empty {
		return "", fmt.Errorf("mem: %s %s: %w", kind, id, pack.ErrEmpty)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	e := s.entities[kind][id]
	cur := ""
	if e != nil {
		cur = strconv.Itoa(len(e.packs))
	}
	if cur != prev {
		return "", fmt.Errorf("%w: %s %s at %q, not %q", store.ErrMoved, kind, id, cur, prev)
	}
	if e == nil {
		e = &entity{}
		if s.entities[kind] == nil {
			s.entities[kind] = map[string]*entity{}
		}
		s.entities[kind][id] = e
	}
	if clock != e.clock+1 {
		return "", fmt.Errorf("mem: %s %s: a pack of clock %d on one of clock %d", kind, id, clock, e.clock)
	}
	e.packs = append(slices.Clip(e.packs), slices.Clone(data))
	e.clock = clock
	return strconv.Itoa(len(e.packs)), nil
}
