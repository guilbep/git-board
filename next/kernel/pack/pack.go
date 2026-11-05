// Package pack is the format's bytes (docs/spec/storage.md, Packs): a
// pack's encoding, its hash, its header, and the order of an entity's
// operations. It knows no kind and no datastore.
package pack

import (
	"cmp"
	"crypto/rand"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Origin says where on a forge an operation came from.
type Origin struct {
	Forge string `json:"forge"`
	Event string `json:"event"`
}

// Op is one operation, as docs/spec/data-model.md writes it. The keys
// are in the order git-board writes them; author is written even empty,
// as v1 writes it.
type Op struct {
	Kind   string          `json:"kind"`
	Author string          `json:"author"`
	Time   int64           `json:"time"`
	Origin *Origin         `json:"origin,omitempty"`
	Data   json.RawMessage `json:"data,omitempty"`
}

// Pack is one commit's blob.
type Pack struct {
	Clock uint64 `json:"clock"`
	Nonce string `json:"nonce,omitempty"`
	Ops   []Op   `json:"ops"`
}

// Hash is a pack's id: the git object id of its blob,
// sha1("blob <len>\x00" + bytes). It needs no git, so every datastore
// names a pack alike.
func Hash(data []byte) string {
	h := sha1.New()
	h.Write(strconv.AppendInt([]byte("blob "), int64(len(data)), 10))
	h.Write([]byte{0})
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// Encode is p's bytes: compact, keys in the spec's order. A first pack
// (clock 1) with no nonce gets one.
func Encode(p Pack) ([]byte, error) {
	if p.Clock == 1 && p.Nonce == "" {
		nonce := make([]byte, 16)
		rand.Read(nonce) // never fails, since Go 1.24
		p.Nonce = hex.EncodeToString(nonce)
	}
	if err := check(p.Ops); err != nil {
		return nil, err
	}
	return json.Marshal(p)
}

// Decode reads a pack. Unknown fields and the nonce are ignored; an
// operation with no kind or no time is refused (storage.md, What a reader
// rejects). Go's decoder takes a key in any case and keeps the last of two
// equal keys: v1 reads so, and so does v2 (data-model.md, a known
// ceiling).
func Decode(data []byte) (Pack, error) {
	var p Pack
	if err := json.Unmarshal(data, &p); err != nil {
		return p, err
	}
	return p, check(p.Ops)
}

func check(ops []Op) error {
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

// Header is what a datastore that chains packs reads of one: its clock,
// and whether it holds no operation. It scans the top-level object
// without decoding the operations, as Decode would read it: keys in any
// case, the last of two equal keys. An encoding it can't scan so is
// decoded whole.
func Header(data []byte) (clock uint64, empty bool, err error) {
	if c, e, ok := scanHeader(data); ok {
		return c, e, nil
	}
	var h struct {
		Clock uint64            `json:"clock"`
		Ops   []json.RawMessage `json:"ops"`
	}
	if err := json.Unmarshal(data, &h); err != nil {
		return 0, false, err
	}
	return h.Clock, len(h.Ops) == 0, nil
}

// scanHeader is Header's fast path: false when the bytes need the
// decoder (an escape in a key, a clock that isn't a plain integer, JSON
// it doesn't follow).
func scanHeader(b []byte) (clock uint64, empty bool, ok bool) {
	i := skipSpace(b, 0)
	if i >= len(b) || b[i] != '{' {
		return 0, false, false
	}
	empty = true
	i++
	for {
		i = skipSpace(b, i)
		if i < len(b) && b[i] == '}' {
			return clock, empty, skipSpace(b, i+1) == len(b)
		}
		if i >= len(b) || b[i] != '"' {
			return 0, false, false
		}
		end := skipString(b, i)
		if end < 0 {
			return 0, false, false
		}
		key := b[i+1 : end-1]
		if slices.Contains(key, '\\') {
			return 0, false, false
		}
		i = skipSpace(b, end)
		if i >= len(b) || b[i] != ':' {
			return 0, false, false
		}
		i = skipSpace(b, i+1)
		vend := skipValue(b, i)
		if vend < 0 {
			return 0, false, false
		}
		switch v := b[i:vend]; {
		case strings.EqualFold(string(key), "clock"):
			n, err := strconv.ParseUint(string(v), 10, 64)
			if err != nil {
				return 0, false, false
			}
			clock = n
		case strings.EqualFold(string(key), "ops"):
			if string(v) == "null" {
				empty = true
				break
			}
			if v[0] != '[' {
				return 0, false, false
			}
			empty = skipSpace(v, 1) == len(v)-1
		}
		i = skipSpace(b, vend)
		switch {
		case i < len(b) && b[i] == ',':
			i++
		case i < len(b) && b[i] == '}':
		default:
			return 0, false, false
		}
	}
}

func skipSpace(b []byte, i int) int {
	for i < len(b) && (b[i] == ' ' || b[i] == '\n' || b[i] == '\t' || b[i] == '\r') {
		i++
	}
	return i
}

// skipString is the index after the string starting at b[i], -1 if it
// doesn't end.
func skipString(b []byte, i int) int {
	for j := i + 1; j < len(b); j++ {
		switch b[j] {
		case '\\':
			j++
		case '"':
			return j + 1
		}
	}
	return -1
}

// skipValue is the index after the value starting at b[i], -1 when it
// can't tell. Nested values are skipped by their brackets.
func skipValue(b []byte, i int) int {
	if i >= len(b) {
		return -1
	}
	switch b[i] {
	case '"':
		return skipString(b, i)
	case '{', '[':
		depth := 0
		for j := i; j < len(b); j++ {
			switch b[j] {
			case '"':
				end := skipString(b, j)
				if end < 0 {
					return -1
				}
				j = end - 1
			case '{', '[':
				depth++
			case '}', ']':
				if depth--; depth == 0 {
					return j + 1
				}
			}
		}
		return -1
	}
	j := i
	for j < len(b) && b[j] != ',' && b[j] != '}' && b[j] != ']' && b[j] != ' ' && b[j] != '\n' && b[j] != '\t' && b[j] != '\r' {
		j++
	}
	if j == i {
		return -1
	}
	return j
}

// Entry is an operation in its entity's order: its id, "<pack
// hash>:<index>", and its pack's clock.
type Entry struct {
	ID    string
	Clock uint64
	Op
}

// ErrEmpty is a pack with no operation where a merge isn't.
var ErrEmpty = errors.New("a pack with no operation")

// Order is an entity's operations from its packs: decoded, sorted by
// (clock, hash), each pack's ops in their stored order; two packs of the
// same bytes count once (storage.md, Order). It answers the highest
// clock, the next pack's being one more.
func Order(packs [][]byte) ([]Entry, uint64, error) {
	type node struct {
		hash string
		Pack
	}
	nodes := make([]node, 0, len(packs))
	for _, data := range packs {
		h := Hash(data)
		p, err := Decode(data)
		if err != nil {
			return nil, 0, fmt.Errorf("pack %s: %w", h, err)
		}
		nodes = append(nodes, node{h, p})
	}
	slices.SortFunc(nodes, func(a, b node) int {
		return cmp.Or(cmp.Compare(a.Clock, b.Clock), strings.Compare(a.hash, b.hash))
	})
	// the same bytes have the same hash and clock: neighbours once sorted
	nodes = slices.CompactFunc(nodes, func(a, b node) bool { return a.hash == b.hash })
	n := 0
	for _, x := range nodes {
		n += len(x.Ops)
	}
	out := make([]Entry, 0, n)
	var clock uint64
	for _, x := range nodes {
		clock = max(clock, x.Clock)
		for i, op := range x.Ops {
			out = append(out, Entry{ID: x.hash + ":" + strconv.Itoa(i), Clock: x.Clock, Op: op})
		}
	}
	return out, clock, nil
}
