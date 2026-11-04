// Package oplog keeps an entity as a log of operations, in packs a
// datastore keeps (ADR 0002). It encodes, hashes and orders them; the
// datastore never reads them. git.go is its git half.
package oplog

import (
	"cmp"
	"context"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/guilbep/git-board/core/datastore"
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

// Log is an entity: its version, and its operations in order.
type Log struct {
	Kind, ID string
	// Version is the datastore's for the entity as read: in git, its head
	// commit.
	Version string
	Entries []Entry
	// clock is the highest of its packs', merges' included. 0 when unknown.
	clock uint64
}

// Datastore is where entities are kept.
type Datastore = datastore.Datastore

// ErrNotFound is returned when an entity has no pack.
var ErrNotFound = datastore.ErrNotFound

// ErrBadName is returned for a kind or an id that names no entity.
var ErrBadName = errors.New("oplog: bad kind or id")

// tries is how many times a write is tried on a moved entity.
const tries = 5

// pack is a commit's blob. Its fields are in the order they are written.
type pack struct {
	Clock uint64 `json:"clock"`
	Nonce string `json:"nonce,omitempty"`
	Ops   []Op   `json:"ops"`
}

// node is one pack, as read: its hash, its fields.
type node struct {
	hash string // the git blob id of its bytes
	pack
}

// Hash is a pack's id: the git blob id of its bytes,
// sha1("blob <len>\x00" + bytes). It needs no git, so it is the same in
// every datastore.
func Hash(data []byte) string {
	h := sha1.New()
	fmt.Fprintf(h, "blob %d\x00", len(data))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// RefName is an entity's ref in git, and its name in messages.
func RefName(kind, id string) string { return "refs/board/" + kind + "/" + id }

// IsID says whether id is an entity id: 40 lowercase hex digits.
func IsID(id string) bool { return idPattern.MatchString(id) }

var idPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

// checkKind refuses a kind that isn't one of datastore.Kinds.
func checkKind(kind string) error {
	if !slices.Contains(datastore.Kinds, kind) {
		return fmt.Errorf("%w: kind %q", ErrBadName, kind)
	}
	return nil
}

// checkName refuses a kind or an id that names no entity.
func checkName(kind, id string) error {
	if !IsID(id) {
		return fmt.Errorf("%w: id %q", ErrBadName, id)
	}
	return checkKind(kind)
}

// checkOps refuses what a reader rejects: an operation with no kind or no
// time.
func checkOps(ops []Op) error {
	for i, op := range ops {
		if op.Kind == "" {
			return fmt.Errorf("operation %d: no kind", i)
		}
		if op.Time == 0 {
			return fmt.Errorf("operation %d: no time", i)
		}
	}
	return nil
}

// Create writes an entity's first pack. An empty id means the first
// pack's hash.
func Create(ctx context.Context, ds Datastore, kind, id string, ops []Op) (*Log, error) {
	err := checkName(kind, id)
	if id == "" {
		err = checkKind(kind)
	}
	if err != nil {
		return nil, err
	}
	if len(ops) == 0 {
		return nil, errors.New("oplog: an entity starts with an operation")
	}
	if err := checkOps(ops); err != nil {
		return nil, fmt.Errorf("oplog: %w", err)
	}
	nonce := make([]byte, 16)
	rand.Read(nonce) // never fails, since Go 1.24
	data, n, err := encode(pack{Clock: 1, Nonce: hex.EncodeToString(nonce), Ops: ops})
	if err != nil {
		return nil, err
	}
	if id == "" {
		id = n.hash
	}
	v, err := ds.Add(ctx, kind, id, data, "")
	if err != nil {
		return nil, fmt.Errorf("oplog: create %s: %w", RefName(kind, id), err)
	}
	return &Log{Kind: kind, ID: id, Version: v, Entries: n.entries(), clock: 1}, nil
}

// Load reads an entity and orders its operations.
func Load(ds Datastore, kind, id string) (*Log, error) {
	if err := checkName(kind, id); err != nil {
		return nil, err
	}
	packs, v, err := ds.Packs(context.Background(), kind, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("oplog: %s: %w", RefName(kind, id), err)
	}
	nodes := make([]node, 0, len(packs))
	for _, data := range packs {
		n, err := decode(data)
		if err != nil {
			return nil, fmt.Errorf("oplog: %s: %w", RefName(kind, id), err)
		}
		nodes = append(nodes, n)
	}
	return build(kind, id, v, nodes), nil
}

// build is the log of an entity's packs.
func build(kind, id, version string, nodes []node) *Log {
	l := &Log{Kind: kind, ID: id, Version: version, Entries: order(nodes)}
	for _, n := range nodes {
		l.clock = max(l.clock, n.Clock)
	}
	return l
}

// Append writes ops in one pack on the entity's version. When the entity
// moved, it reloads and writes them again on the new version. Appending
// nothing writes nothing. Once the write lands, l gets the new version and
// the new entries, which sort last, without reading the entity again.
func (l *Log) Append(ctx context.Context, ds Datastore, ops []Op) error {
	if err := checkName(l.Kind, l.ID); err != nil {
		return err
	}
	if len(ops) == 0 {
		return nil
	}
	if err := checkOps(ops); err != nil {
		return fmt.Errorf("oplog: %w", err)
	}
	name := RefName(l.Kind, l.ID)
	if l.clock == 0 { // a log not read here: its clock is unknown
		fresh, err := Load(ds, l.Kind, l.ID)
		if err != nil {
			return err
		}
		*l = *fresh
	}
	for try := 1; ; try++ {
		data, n, err := encode(pack{Clock: l.clock + 1, Ops: ops})
		if err != nil {
			return err
		}
		v, err := ds.Add(ctx, l.Kind, l.ID, data, l.Version)
		if err == nil {
			l.Version, l.Entries, l.clock = v, slices.Concat(l.Entries, n.entries()), n.Clock
			return nil
		}
		if !errors.Is(err, datastore.ErrMoved) || try == tries {
			return fmt.Errorf("oplog: append %s: %w", name, err)
		}
		fresh, err := Load(ds, l.Kind, l.ID)
		if err != nil {
			return err
		}
		*l = *fresh
	}
}

// List returns the ids of a kind's entities, sorted. A name that isn't an
// id isn't an entity, and is left out.
func List(ds Datastore, kind string) ([]string, error) {
	if err := checkKind(kind); err != nil {
		return nil, err
	}
	heads, err := ds.Heads(context.Background(), kind)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(heads))
	for id := range heads {
		if IsID(id) {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	return ids, nil
}

// encode is a pack's bytes, and the node a reader reads from them: its ops
// are decoded from the stored bytes, so entries built from them are the ones
// a Load gives.
func encode(p pack) ([]byte, node, error) {
	data, err := json.Marshal(p)
	if err != nil {
		return nil, node{}, err
	}
	n, err := decode(data)
	return data, n, err
}

// decode reads a pack. Unknown fields and the nonce are ignored. An
// operation with no kind or no time is refused.
func decode(data []byte) (n node, err error) {
	n.hash = Hash(data)
	// ponytail: encoding/json matches keys in any case and keeps the last of
	// two equal keys; the spec says case-sensitive and unique. A strict
	// decoder if another implementation ever disagrees.
	if err := json.Unmarshal(data, &n.pack); err != nil {
		return n, fmt.Errorf("pack %s: %w", n.hash, err)
	}
	if err := checkOps(n.Ops); err != nil {
		return n, fmt.Errorf("pack %s: %w", n.hash, err)
	}
	return n, nil
}

// order sorts the packs by (clock, hash) and lists their operations. Two
// packs with the same bytes count as one.
func order(nodes []node) []Entry {
	packs := map[string]node{}
	for _, n := range nodes {
		packs[n.hash] = n
	}
	sorted := slices.SortedFunc(maps.Values(packs), func(a, b node) int {
		return cmp.Or(cmp.Compare(a.Clock, b.Clock), strings.Compare(a.hash, b.hash))
	})
	var out []Entry
	for _, n := range sorted {
		out = append(out, n.entries()...)
	}
	return out
}

// entries are a pack's operations, with their ids and its clock.
func (n node) entries() []Entry {
	out := make([]Entry, len(n.Ops))
	for i, op := range n.Ops {
		out[i] = Entry{Op: op, ID: fmt.Sprintf("%s:%d", n.hash, i), Clock: n.Clock}
	}
	return out
}
