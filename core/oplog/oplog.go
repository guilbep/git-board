// Package oplog keeps an entity as a log of operations, in a chain of git
// commits under its ref.
package oplog

import (
	"cmp"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/guilbep/git-board/core/gitstore"
)

// Origin says where on a forge an operation came from.
type Origin struct {
	Forge string `json:"forge"`
	Event string `json:"event"`
}

// Op is one change to an entity.
type Op struct {
	Kind   string          `json:"kind"`
	Author string          `json:"author"`
	Time   int64           `json:"time"`
	Origin *Origin         `json:"origin,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

// Entry is an operation in its log, with its id and its commit's clock.
type Entry struct {
	Op
	ID    string
	Clock uint64
}

// Log is an entity: its head commit, and its operations in order.
type Log struct {
	Kind, ID string
	Head     gitstore.Hash
	Entries  []Entry
}

// ErrNotFound is returned when an entity has no ref.
var ErrNotFound = errors.New("oplog: no such entity")

// tries is how many times a write is tried on a moved ref.
const tries = 5

// pack is a commit's blob. Its fields are in the order they are written.
type pack struct {
	Clock uint64 `json:"clock"`
	Nonce string `json:"nonce,omitempty"`
	Ops   []Op   `json:"ops"`
}

// node is one commit of an entity.
type node struct {
	parents []gitstore.Hash
	blob    gitstore.Hash // the pack's
	pack
}

// RefName is an entity's ref.
func RefName(kind, id string) string { return "refs/board/" + kind + "/" + id }

// Create writes an entity's first commit. An empty id means the first
// pack's hash.
func Create(s *gitstore.Store, kind, id string, ops []Op) (*Log, error) {
	if len(ops) == 0 {
		return nil, errors.New("oplog: an entity starts with an operation")
	}
	nonce := make([]byte, 16)
	rand.Read(nonce) // never fails, since Go 1.24
	c, blob, err := writePack(s, pack{Clock: 1, Nonce: hex.EncodeToString(nonce), Ops: ops})
	if err != nil {
		return nil, err
	}
	if id == "" {
		id = blob.String()
	}
	if err := s.UpdateRef(RefName(kind, id), c, gitstore.ZeroHash); err != nil {
		return nil, fmt.Errorf("oplog: create %s: %w", RefName(kind, id), err)
	}
	return Load(s, kind, id)
}

// Load reads an entity and orders its operations.
func Load(s *gitstore.Store, kind, id string) (*Log, error) {
	name := RefName(kind, id)
	head, err := s.Ref(name)
	if err != nil {
		return nil, err
	}
	if head == gitstore.ZeroHash {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, name)
	}
	nodes, err := walk(s, head)
	if err != nil {
		return nil, err
	}
	return &Log{Kind: kind, ID: id, Head: head, Entries: order(nodes)}, nil
}

// Append writes ops in one commit on the head. When the ref moved, it
// reloads and writes them again on the new head. Appending nothing writes
// nothing. l is reloaded after.
func (l *Log) Append(s *gitstore.Store, ops []Op) error {
	if len(ops) == 0 {
		return nil
	}
	for try := 1; ; try++ {
		head, err := readNode(s, l.Head)
		if err != nil {
			return err
		}
		c, _, err := writePack(s, pack{Clock: head.Clock + 1, Ops: ops}, l.Head)
		if err != nil {
			return err
		}
		err = s.UpdateRef(RefName(l.Kind, l.ID), c, l.Head)
		if err != nil && (!errors.Is(err, gitstore.ErrRefMoved) || try == tries) {
			return err
		}
		fresh, lerr := Load(s, l.Kind, l.ID)
		if lerr != nil {
			return lerr
		}
		*l = *fresh
		if err == nil {
			return nil
		}
	}
}

// List returns the ids of a kind's entities, sorted.
func List(s *gitstore.Store, kind string) ([]string, error) {
	prefix := RefName(kind, "")
	refs, err := s.Refs(prefix)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(refs))
	for name := range refs {
		ids = append(ids, strings.TrimPrefix(name, prefix))
	}
	slices.Sort(ids)
	return ids, nil
}

// writePack writes p in a commit on parents.
func writePack(s *gitstore.Store, p pack, parents ...gitstore.Hash) (commit, blob gitstore.Hash, err error) {
	data, err := json.Marshal(p)
	if err != nil {
		return commit, blob, err
	}
	if blob, err = s.WriteBlob(data); err != nil {
		return commit, blob, err
	}
	tree, err := s.WriteTree([]gitstore.TreeEntry{{Name: "pack", Blob: blob}})
	if err != nil {
		return commit, blob, err
	}
	commit, err = s.WriteCommit(gitstore.Commit{Tree: tree, Parents: parents})
	return commit, blob, err
}

// readNode reads a commit and its pack. Other tree entries, unknown pack
// fields and the nonce are ignored.
func readNode(s *gitstore.Store, h gitstore.Hash) (n node, err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("oplog: commit %s: %w", h, err)
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
	n.parents = c.Parents
	for _, e := range entries {
		if e.Name == "pack" {
			n.blob = e.Blob
		}
	}
	if n.blob == gitstore.ZeroHash {
		return n, errors.New("no pack")
	}
	data, err := s.ReadBlob(n.blob)
	if err != nil {
		return n, err
	}
	if err := json.Unmarshal(data, &n.pack); err != nil {
		return n, fmt.Errorf("pack: %w", err)
	}
	if len(n.Ops) == 0 && len(n.parents) < 2 {
		return n, errors.New("a pack with no operation")
	}
	return n, nil
}

// walk reads every commit reachable from head and checks their clocks.
// ponytail: every load reads the whole history; keep the nodes per commit
// if long entities make it slow.
func walk(s *gitstore.Store, head gitstore.Hash) (map[gitstore.Hash]node, error) {
	nodes := map[gitstore.Hash]node{}
	for todo := []gitstore.Hash{head}; len(todo) > 0; {
		h := todo[len(todo)-1]
		todo = todo[:len(todo)-1]
		if _, ok := nodes[h]; ok {
			continue
		}
		n, err := readNode(s, h)
		if err != nil {
			return nil, err
		}
		nodes[h] = n
		todo = append(todo, n.parents...)
	}
	for h, n := range nodes {
		want := uint64(1)
		for _, p := range n.parents {
			want = max(want, nodes[p].Clock+1)
		}
		if n.Clock != want {
			return nil, fmt.Errorf("oplog: commit %s: clock %d, want %d", h, n.Clock, want)
		}
	}
	return nodes, nil
}

// order sorts the packs by (clock, hash) and lists their operations. Two
// commits with the same pack blob count as one pack.
func order(nodes map[gitstore.Hash]node) []Entry {
	packs := map[gitstore.Hash]node{}
	for _, n := range nodes {
		packs[n.blob] = n
	}
	sorted := slices.SortedFunc(maps.Values(packs), func(a, b node) int {
		return cmp.Or(cmp.Compare(a.Clock, b.Clock), strings.Compare(a.blob.String(), b.blob.String()))
	})
	var out []Entry
	for _, n := range sorted {
		for i, op := range n.Ops {
			out = append(out, Entry{Op: op, ID: fmt.Sprintf("%s:%d", n.blob, i), Clock: n.Clock})
		}
	}
	return out
}
