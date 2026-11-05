package view

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/guilbep/git-board/next/kernel"
)

// A kind's rows are read whole by every list: 20,000 of them through
// sqlite take 30 to 60 ms, where the budget for a whole ls is 100
// (design.md, section 4). So the rows are also kept in one file per kind
// beside view.db, read whole: a copy of the rows table at a mark, which
// names it in its header. A file of another mark is stale, and the next
// list writes it again from the table; the table stays the truth.

// row is one row as the file keeps it.
type row struct {
	id               kernel.ID
	version          kernel.Version
	created, updated int64
	hidden           bool
	row              []byte
}

// rowsCache is what a View read of its files last, by kind: a process
// listing again at the same mark reads nothing.
type rowsCache struct {
	mu   sync.Mutex
	at   map[string]kernel.Mark
	rows map[string][]row
}

func (v *View) rowsPath(kind string) string { return filepath.Join(v.dir, "rows-"+kind) }

// all is every row of a kind at its current mark, by (created, id).
func (v *View) all(ctx context.Context, kind string) ([]row, error) {
	m, err := mark(ctx, v.db, kind)
	if err != nil {
		return nil, err
	}
	v.cache.mu.Lock()
	defer v.cache.mu.Unlock()
	if at, ok := v.cache.at[kind]; ok && at == m {
		return v.cache.rows[kind], nil
	}
	data, err := os.ReadFile(v.rowsPath(kind))
	rows, ok := []row(nil), false
	if err == nil {
		rows, ok = parseRows(data, m)
	}
	if !ok {
		if m, rows, err = v.readRows(ctx, kind); err != nil {
			return nil, err
		}
		if err := writeRows(v.rowsPath(kind), m, rows); err != nil {
			return nil, err
		}
	}
	v.cache.at[kind], v.cache.rows[kind] = m, rows
	return rows, nil
}

// patchRows brings a kind's rows file from mark before to mark after,
// with the rows a write changed (nil: gone), when the file is at before:
// a write of a few rows doesn't leave the next list to read 20,000 from
// the table. Another writer between the two leaves the file behind, and
// the next list reads the table: it is never wrong, at worst slower.
func (v *View) patchRows(kind string, before, after kernel.Mark, changed map[kernel.ID]*row) {
	v.cache.mu.Lock()
	defer v.cache.mu.Unlock()
	rows, ok := v.cache.rows[kind], v.cache.at[kind] == before
	if !ok {
		data, err := os.ReadFile(v.rowsPath(kind))
		if err != nil {
			return
		}
		if rows, ok = parseRows(data, before); !ok {
			return
		}
	}
	next := make([]row, 0, len(rows)+len(changed))
	for _, r := range rows {
		if _, moved := changed[r.id]; !moved {
			next = append(next, r)
		}
	}
	for _, r := range changed {
		if r != nil {
			next = append(next, *r)
		}
	}
	slices.SortFunc(next, func(a, b row) int {
		return cmp.Or(cmp.Compare(a.created, b.created), strings.Compare(string(a.id), string(b.id)))
	})
	if err := writeRows(v.rowsPath(kind), after, next); err != nil {
		return
	}
	v.cache.at[kind], v.cache.rows[kind] = after, next
}

// readRows reads a kind's mark and rows from the table, in one snapshot.
func (v *View) readRows(ctx context.Context, kind string) (kernel.Mark, []row, error) {
	tx, err := v.ro.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return kernel.Mark{}, nil, err
	}
	defer tx.Rollback()
	m, err := mark(ctx, tx, kind)
	if err != nil {
		return m, nil, err
	}
	rs, err := tx.QueryContext(ctx, "SELECT id, version, created, updated, hidden, row FROM rows WHERE kind = ? ORDER BY created, id", kind)
	if err != nil {
		return m, nil, err
	}
	defer rs.Close()
	var out []row
	for rs.Next() {
		var r row
		if err := rs.Scan(&r.id, &r.version, &r.created, &r.updated, &r.hidden, &r.row); err != nil {
			return m, nil, err
		}
		out = append(out, r)
	}
	return m, out, rs.Err()
}

// magic starts a rows file: its format, and the view's version.
var magic = fmt.Appendf(nil, "git-board view rows %d\n", version)

func writeRows(path string, m kernel.Mark, rows []row) error {
	size := len(magic) + 64
	for _, r := range rows {
		size += len(r.id) + len(r.version) + len(r.row) + 32
	}
	b := append(make([]byte, 0, size), magic...)
	b = binary.AppendUvarint(b, uint64(m.Rev))
	b = append(b, m.Sum[:]...)
	b = binary.AppendUvarint(b, uint64(len(rows)))
	for _, r := range rows {
		b = binary.AppendUvarint(b, uint64(len(r.id)))
		b = append(b, r.id...)
		b = binary.AppendUvarint(b, uint64(len(r.version)))
		b = append(b, r.version...)
		b = binary.AppendVarint(b, r.created)
		b = binary.AppendVarint(b, r.updated)
		if r.hidden {
			b = append(b, 1)
		} else {
			b = append(b, 0)
		}
		b = binary.AppendUvarint(b, uint64(len(r.row)))
		b = append(b, r.row...)
	}
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), path) // a reader sees the old file or the new one
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}

// parseRows reads a rows file, false when it isn't at m or is cut short.
// Every id and version is a piece of one string, every row a piece of
// the file's bytes: a 20,000-row file is read with three allocations.
func parseRows(data []byte, m kernel.Mark) ([]row, bool) {
	if !bytes.HasPrefix(data, magic) {
		return nil, false
	}
	whole := string(data)
	i, bad := len(magic), false
	uv := func() uint64 {
		x, n := binary.Uvarint(data[i:])
		if n <= 0 {
			bad = true
			return 0
		}
		i += n
		return x
	}
	sv := func() int64 {
		x, n := binary.Varint(data[i:])
		if n <= 0 {
			bad = true
			return 0
		}
		i += n
		return x
	}
	span := func() (int, int) {
		n := uv()
		if bad || n > uint64(len(data)-i) {
			bad = true
			return i, i
		}
		i += int(n)
		return i - int(n), i
	}
	if rev := uv(); bad || int(rev) != m.Rev || len(data)-i < 32 || !bytes.Equal(data[i:i+32], m.Sum[:]) {
		return nil, false
	}
	i += 32
	n := uv()
	if bad || n > uint64(len(data)-i) {
		return nil, false
	}
	rows := make([]row, n)
	for k := range rows {
		r := &rows[k]
		a, b := span()
		r.id = kernel.ID(whole[a:b])
		a, b = span()
		r.version = kernel.Version(whole[a:b])
		r.created, r.updated = sv(), sv()
		if bad || i >= len(data) {
			return nil, false
		}
		r.hidden = data[i] == 1
		i++
		a, b = span()
		r.row = data[a:b:b]
		if bad {
			return nil, false
		}
	}
	return rows, i == len(data)
}

// sorted is rows (by created, then id) in q's order.
func sorted(rows []row, q kernel.Query) []row {
	switch q.Sort {
	case "":
		rows = slices.Clone(rows)
		slices.SortFunc(rows, func(a, b row) int { return strings.Compare(string(a.id), string(b.id)) })
	case "updated":
		rows = slices.Clone(rows)
		slices.SortStableFunc(rows, func(a, b row) int {
			return cmp.Or(cmp.Compare(a.updated, b.updated), strings.Compare(string(a.id), string(b.id)))
		})
	}
	if q.Desc {
		if q.Sort == "created" {
			rows = slices.Clone(rows)
		}
		slices.Reverse(rows)
	}
	return rows
}
