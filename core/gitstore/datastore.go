package gitstore

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/go-git/go-git/v5/plumbing"

	"github.com/guilbep/git-board/core/datastore"
)

// The git datastore (ADR 0002): an entity is a chain of commits under
// refs/board/<kind>/<id>, each holding one pack in a tree entry "pack". Its
// version is its head commit. Only git has parents, so the two checks that
// read them are made here: a pack's clock is 1 + its parents' highest, and a
// pack with no operation is a merge.

var _ datastore.Datastore = (*Store)(nil)

// EntityRef is an entity's ref.
func EntityRef(kind, id string) string { return "refs/board/" + kind + "/" + id }

var entityID = regexp.MustCompile(`^[0-9a-f]{40}$`)

// Node is one entity commit: its parents, its pack's blob and bytes, and the
// pack's clock.
type Node struct {
	Parents []Hash
	Blob    Hash
	Pack    []byte
	Clock   uint64
}

// header is what the git datastore reads of a pack: its clock, and how many
// operations it holds.
type header struct {
	Clock uint64            `json:"clock"`
	Ops   []json.RawMessage `json:"ops"`
}

// Heads are a kind's entities by id, each with its head commit. A ref whose
// name isn't an id isn't an entity, and is left out.
func (s *Store) Heads(_ context.Context, kind string) (map[string]string, error) {
	prefix := EntityRef(kind, "")
	refs, err := s.Refs(prefix)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(refs))
	for name, h := range refs {
		if id := strings.TrimPrefix(name, prefix); entityID.MatchString(id) {
			out[id] = h.String()
		}
	}
	return out, nil
}

// Packs are every pack reachable from the entity's head, and the head.
func (s *Store) Packs(_ context.Context, kind, id string) ([][]byte, string, error) {
	name := EntityRef(kind, id)
	head, err := s.Ref(name)
	if err != nil {
		return nil, "", err
	}
	if head == ZeroHash {
		return nil, "", fmt.Errorf("%w: %s", datastore.ErrNotFound, name)
	}
	nodes, err := s.Walk(head)
	if err != nil {
		return nil, "", err
	}
	packs := make([][]byte, 0, len(nodes))
	for _, n := range nodes {
		packs = append(packs, n.Pack)
	}
	return packs, head.String(), nil
}

// Add writes pack in a commit on prev, and moves the entity's ref there if
// it is still at prev. A pack whose clock isn't 1 + prev's (1 with no prev)
// is refused: Walk would refuse the entity after it.
func (s *Store) Add(ctx context.Context, kind, id string, pack []byte, prev string) (string, error) {
	var parents []Hash
	old := ZeroHash
	want := uint64(1)
	if prev != "" {
		old = NewHash(prev)
		parents = []Hash{old}
		n, err := s.ReadNode(old)
		if err != nil {
			return "", err
		}
		want = n.Clock + 1
	}
	var hd header
	if err := json.Unmarshal(pack, &hd); err != nil {
		return "", fmt.Errorf("gitstore: %s %s: pack: %w", kind, id, err)
	}
	if hd.Clock != want {
		return "", fmt.Errorf("gitstore: %s %s: a pack of clock %d on one of clock %d: copying a board into git with packs written apart isn't done yet (ADR 0002)", kind, id, hd.Clock, want-1)
	}
	c, _, err := s.WritePack(pack, parents...)
	if err != nil {
		return "", err
	}
	if err := s.UpdateRef(ctx, EntityRef(kind, id), c, old); err != nil {
		return "", err
	}
	return c.String(), nil
}

// Folder is <common dir>/board.
func (s *Store) Folder() string { return filepath.Join(s.commonDir, "board") }

// WritePack writes pack as a blob, in a tree as "pack", in a commit on
// parents. It moves no ref.
func (s *Store) WritePack(pack []byte, parents ...Hash) (commit, blob Hash, err error) {
	if blob, err = s.WriteBlob(pack); err != nil {
		return commit, blob, err
	}
	tree, err := s.WriteTree([]TreeEntry{{Name: "pack", Blob: blob}})
	if err != nil {
		return commit, blob, err
	}
	commit, err = s.WriteCommit(Commit{Tree: tree, Parents: parents})
	return commit, blob, err
}

// NewHash parses a hash.
func NewHash(s string) Hash { return plumbing.NewHash(s) }

// ReadNode reads an entity commit and its pack. Other tree entries are
// ignored. A pack with no operation is refused unless it merges.
func (s *Store) ReadNode(h Hash) (n Node, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("commit %s: %w", h, err)
		}
	}()
	c, err := s.ReadCommit(h)
	if err != nil {
		return n, err
	}
	entries, err := s.ReadTree(c.Tree)
	if err != nil {
		return n, err
	}
	n.Parents = c.Parents
	for _, e := range entries {
		if e.Name == "pack" {
			n.Blob = e.Blob
		}
	}
	if n.Blob == ZeroHash {
		return n, errors.New("no pack")
	}
	if n.Pack, err = s.ReadBlob(n.Blob); err != nil {
		return n, err
	}
	var hd header
	if err := json.Unmarshal(n.Pack, &hd); err != nil {
		return n, fmt.Errorf("pack: %w", err)
	}
	n.Clock = hd.Clock
	if len(hd.Ops) == 0 && len(n.Parents) < 2 {
		return n, errors.New("a pack with no operation")
	}
	return n, nil
}

// Walk reads every entity commit reachable from head and checks their
// clocks.
// ponytail: every load reads the whole history; keep the nodes per commit
// if long entities make it slow.
func (s *Store) Walk(head Hash) (map[Hash]Node, error) {
	nodes := map[Hash]Node{}
	for todo := []Hash{head}; len(todo) > 0; {
		h := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		if _, ok := nodes[h]; ok {
			continue
		}
		n, err := s.ReadNode(h)
		if err != nil {
			return nil, err
		}
		nodes[h] = n
		todo = append(todo, n.Parents...)
	}
	for h, n := range nodes {
		want := uint64(1)
		for _, p := range n.Parents {
			want = max(want, nodes[p].Clock+1)
		}
		if n.Clock != want {
			return nil, fmt.Errorf("commit %s: clock %d, want %d", h, n.Clock, want)
		}
	}
	return nodes, nil
}
