// Package memstore is a datastore in memory: a dry run's throwaway copy of
// a board kept elsewhere than git, and the reference the conformance tests
// hold the other datastores to.
package memstore

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"sync"

	"github.com/guilbep/git-board/core/datastore"
)

// Store keeps packs in memory. Its versions count the entity's writes.
type Store struct {
	mu       sync.Mutex
	folder   string
	entities map[[2]string]*entity
	count    []byte
	countV   int
}

type entity struct {
	packs   [][]byte
	version int
}

var (
	_ datastore.Datastore = (*Store)(nil)
	_ datastore.Counter   = (*Store)(nil)
)

// New is an empty store whose machine folder is folder.
func New(folder string) *Store {
	return &Store{folder: folder, entities: map[[2]string]*entity{}}
}

// Copy is a copy of every entity in from, and its counter when it keeps
// one, in a new store whose folder is folder.
func Copy(ctx context.Context, from datastore.Datastore, folder string) (*Store, error) {
	s := New(folder)
	for _, kind := range datastore.Kinds {
		heads, err := from.Heads(ctx, kind)
		if err != nil {
			return nil, err
		}
		for _, id := range slices.Sorted(maps.Keys(heads)) {
			packs, _, err := from.Packs(ctx, kind, id)
			if err != nil {
				return nil, fmt.Errorf("copying %s %s: %w", kind, id, err)
			}
			s.entities[[2]string{kind, id}] = &entity{packs: packs, version: 1}
		}
	}
	if c, ok := from.(datastore.Counter); ok {
		v, _, err := c.Count(ctx)
		if err != nil {
			return nil, err
		}
		if v != nil {
			s.count, s.countV = v, 1
		}
	}
	return s, nil
}

// Heads are a kind's entities and their versions.
func (s *Store) Heads(_ context.Context, kind string) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := map[string]string{}
	for k, e := range s.entities {
		if k[0] == kind {
			out[k[1]] = strconv.Itoa(e.version)
		}
	}
	return out, nil
}

// Packs are an entity's packs and its version.
func (s *Store) Packs(_ context.Context, kind, id string) ([][]byte, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entities[[2]string{kind, id}]
	if !ok {
		return nil, "", fmt.Errorf("%w: %s %s", datastore.ErrNotFound, kind, id)
	}
	return slices.Clone(e.packs), strconv.Itoa(e.version), nil
}

// Add stores a pack if the entity is still at prev.
func (s *Store) Add(_ context.Context, kind, id string, pack []byte, prev string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := [2]string{kind, id}
	e, ok := s.entities[k]
	switch {
	case !ok && prev != "", ok && prev != strconv.Itoa(e.version):
		return "", fmt.Errorf("%w: %s %s", datastore.ErrMoved, kind, id)
	case !ok:
		e = &entity{}
		s.entities[k] = e
	}
	e.packs = append(e.packs, slices.Clone(pack))
	e.version++
	return strconv.Itoa(e.version), nil
}

// Folder is the machine's folder.
func (s *Store) Folder() string { return s.folder }

// Count reads the counter.
func (s *Store) Count(context.Context) ([]byte, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.count == nil {
		return nil, "", nil
	}
	return slices.Clone(s.count), strconv.Itoa(s.countV), nil
}

// SetCount writes the counter if it is still at prev.
func (s *Store) SetCount(_ context.Context, value []byte, prev string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := ""
	if s.count != nil {
		cur = strconv.Itoa(s.countV)
	}
	if prev != cur {
		return "", fmt.Errorf("%w: the counter", datastore.ErrMoved)
	}
	s.count = slices.Clone(value)
	s.countV++
	return strconv.Itoa(s.countV), nil
}
