package view

import (
	"bytes"
	"cmp"
	"context"
	"database/sql"
	"encoding/binary"
	"fmt"
	"maps"
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

// rowFiles are a view's rows files, in dir, and what this process read
// of them last, by kind: a process listing again at the same mark reads
// nothing.
type rowFiles struct {
	dir  string
	mu   sync.Mutex
	at   map[string]kernel.Mark
	rows map[string][]row
}

func newRowFiles(dir string) *rowFiles {
	return &rowFiles{dir: dir, at: map[string]kernel.Mark{}, rows: map[string][]row{}}
}

func (v *rowFiles) rowsPath(kind string) string { return filepath.Join(v.dir, "rows-"+kind) }

// all is every row of a kind at mark m, by (created, id): from this
// process's copy, else the files, else the table (read: a kind's mark and
// rows in one snapshot), written to the files for the next.
func (v *rowFiles) all(ctx context.Context, kind string, m kernel.Mark, read func(context.Context, string) (kernel.Mark, []row, error)) ([]row, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if at, ok := v.at[kind]; ok && at == m {
		return v.rows[kind], nil
	}
	rows, records, ok := v.readFiles(kind, m)
	if ok && records > maxRecords { // the patches grew: one file again
		if writeRows(v.rowsPath(kind), m, rows) == nil {
			os.Remove(v.patchPath(kind))
		}
	}
	if !ok {
		var err error
		if m, rows, err = read(ctx, kind); err != nil {
			return nil, err
		}
		if err := writeRows(v.rowsPath(kind), m, rows); err != nil {
			return nil, err
		}
		os.Remove(v.patchPath(kind))
	}
	v.at[kind], v.rows[kind] = m, rows
	return rows, nil
}

// A write changes a few rows: rewriting the rows file (6 MB at 20,000
// issues) would cost it 40 ms of a 50 ms budget. It appends what it
// changed to a patch file beside, instead: a record from one mark to the
// next. A list reads the rows file and the records that follow on from
// its mark; past maxRecords it writes one file again. Records that don't
// lead to the view's mark (another writer, a cut) leave the list to read
// the table, as a stale file does.
const maxRecords = 32

func (v *rowFiles) patchPath(kind string) string { return v.rowsPath(kind) + ".patch" }

// readFiles reads a kind's rows file and its patches, ok when they come
// to mark m; records is how many patches it took.
func (v *rowFiles) readFiles(kind string, m kernel.Mark) (rows []row, records int, ok bool) {
	data, err := os.ReadFile(v.rowsPath(kind))
	if err != nil {
		return nil, 0, false
	}
	at, rows, ok := parseRows(data)
	if !ok {
		return nil, 0, false
	}
	if same(at, m) {
		return rows, 0, true
	}
	patches, err := os.ReadFile(v.patchPath(kind))
	if err != nil {
		return nil, 0, false
	}
	changed := map[kernel.ID]*row{} // every record's, the later winning: applied once
	for _, p := range parsePatches(patches) {
		if !same(p.before, at) {
			continue // an older file's
		}
		maps.Copy(changed, p.changed)
		at = p.after
		records++
		if same(at, m) {
			return applied(rows, changed), records, true
		}
	}
	return nil, 0, false
}

// same says two marks name the same rows.
func same(a, b kernel.Mark) bool { return a.Rev == b.Rev && a.Sum == b.Sum }

// applied is rows with the changed ones (nil: gone), still by (created, id).
func applied(rows []row, changed map[kernel.ID]*row) []row {
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
	return next
}

// patchRows records that a write took a kind's rows from mark before to
// mark after, with the rows it changed (nil: gone): appended to the
// patch file in one write, so a reader sees a record whole or cut short.
func (v *rowFiles) patchRows(kind string, before, after kernel.Mark, changed map[kernel.ID]*row) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.at[kind] == before {
		v.at[kind], v.rows[kind] = after, applied(v.rows[kind], changed)
	} else {
		delete(v.at, kind)
	}
	if _, err := os.Stat(v.rowsPath(kind)); err != nil {
		return // no file: the next list writes it
	}
	f, err := os.OpenFile(v.patchPath(kind), os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	if _, err := f.Write(appendPatch(nil, patch{before, after, changed})); err != nil {
		f.Close()
		os.Remove(v.patchPath(kind)) // a record cut short ends the file's use
	}
}

// patch is one record of a patch file.
type patch struct {
	before, after kernel.Mark
	changed       map[kernel.ID]*row
}

// patchMagic starts a record: a cut or foreign file stops the reading.
var patchMagic = fmt.Appendf(nil, "patch %d\n", version)

func appendMark(b []byte, m kernel.Mark) []byte {
	b = binary.AppendUvarint(b, uint64(m.Rev))
	return append(b, m.Sum[:]...)
}

func appendRow(b []byte, r row) []byte {
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
	return append(b, r.row...)
}

func appendPatch(b []byte, p patch) []byte {
	var body []byte
	body = appendMark(body, p.before)
	body = appendMark(body, p.after)
	body = binary.AppendUvarint(body, uint64(len(p.changed)))
	for _, id := range slices.Sorted(maps.Keys(p.changed)) {
		if r := p.changed[id]; r != nil {
			body = append(body, 1)
			body = appendRow(body, *r)
		} else {
			body = append(body, 0)
			body = binary.AppendUvarint(body, uint64(len(id)))
			body = append(body, id...)
		}
	}
	b = append(b, patchMagic...)
	b = binary.AppendUvarint(b, uint64(len(body)))
	return append(b, body...)
}

// parsePatches reads a patch file's records, up to the first that is
// cut short.
func parsePatches(data []byte) []patch {
	var res []patch
	for len(data) > 0 {
		if !bytes.HasPrefix(data, patchMagic) {
			return res
		}
		data = data[len(patchMagic):]
		n, k := binary.Uvarint(data)
		if k <= 0 || n > uint64(len(data)-k) {
			return res
		}
		body := data[k : k+int(n)]
		data = data[k+int(n):]
		d := decoder{data: body, whole: string(body)}
		var p patch
		p.before, p.after = d.mark(), d.mark()
		count := d.uv()
		if d.bad || count > uint64(len(body)) {
			return res
		}
		p.changed = make(map[kernel.ID]*row, count)
		for range count {
			if d.byte() == 1 {
				r := d.row()
				p.changed[r.id] = &r
			} else {
				a, b := d.span()
				p.changed[kernel.ID(d.whole[a:b])] = nil
			}
		}
		if d.bad || d.i != len(body) {
			return res
		}
		res = append(res, p)
	}
	return res
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
	b = appendMark(b, m)
	b = binary.AppendUvarint(b, uint64(len(rows)))
	for _, r := range rows {
		b = appendRow(b, r)
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

// parseRows reads a rows file and the mark it is at, false when it is
// cut short. Every id and version is a piece of one string, every row a
// piece of the file's bytes: a 20,000-row file is read with three
// allocations.
func parseRows(data []byte) (kernel.Mark, []row, bool) {
	if !bytes.HasPrefix(data, magic) {
		return kernel.Mark{}, nil, false
	}
	d := decoder{data: data, whole: string(data), i: len(magic)}
	m := d.mark()
	n := d.uv()
	if d.bad || n > uint64(len(data)-d.i) {
		return kernel.Mark{}, nil, false
	}
	rows := make([]row, n)
	for k := range rows {
		rows[k] = d.row()
		if d.bad {
			return kernel.Mark{}, nil, false
		}
	}
	return m, rows, d.i == len(data)
}

// decoder reads what appendMark and appendRow wrote; whole is data as a
// string, which ids and versions are pieces of.
type decoder struct {
	data  []byte
	whole string
	i     int
	bad   bool
}

func (d *decoder) uv() uint64 {
	x, n := binary.Uvarint(d.data[d.i:])
	if n <= 0 {
		d.bad = true
		return 0
	}
	d.i += n
	return x
}

func (d *decoder) sv() int64 {
	x, n := binary.Varint(d.data[d.i:])
	if n <= 0 {
		d.bad = true
		return 0
	}
	d.i += n
	return x
}

func (d *decoder) byte() byte {
	if d.i >= len(d.data) {
		d.bad = true
		return 0
	}
	d.i++
	return d.data[d.i-1]
}

func (d *decoder) span() (int, int) {
	n := d.uv()
	if d.bad || n > uint64(len(d.data)-d.i) {
		d.bad = true
		return d.i, d.i
	}
	d.i += int(n)
	return d.i - int(n), d.i
}

func (d *decoder) mark() kernel.Mark {
	var m kernel.Mark
	m.Rev = int(d.uv())
	if d.bad || len(d.data)-d.i < len(m.Sum) {
		d.bad = true
		return m
	}
	copy(m.Sum[:], d.data[d.i:])
	d.i += len(m.Sum)
	return m
}

func (d *decoder) row() row {
	var r row
	a, b := d.span()
	r.id = kernel.ID(d.whole[a:b])
	a, b = d.span()
	r.version = kernel.Version(d.whole[a:b])
	r.created, r.updated = d.sv(), d.sv()
	r.hidden = d.byte() == 1
	a, b = d.span()
	r.row = d.data[a:b:b]
	return r
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
