// Package view keeps the board's read model in sqlite: every kind's rows,
// terms and texts, keyed by the version each was folded at, and per kind
// the listing they are at (docs/rewrite/design.md, The read path). It is
// disposable: a file of another version, or one sqlite can't read, is
// deleted and built again. The kernel decides what goes in; the view
// stores it and answers queries.
package view

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // database/sql's "sqlite", FTS5 included

	"github.com/guilbep/git-board/next/kernel"
	"github.com/guilbep/git-board/next/kernel/search"
)

// version is the schema's: a file of another is built again.
const version = 2

const schema = `
CREATE TABLE marks (kind TEXT PRIMARY KEY, rev INTEGER NOT NULL, sum BLOB NOT NULL, search INTEGER NOT NULL);
CREATE TABLE rows (n INTEGER PRIMARY KEY, kind TEXT NOT NULL, id TEXT NOT NULL, version TEXT NOT NULL,
	created INTEGER NOT NULL, updated INTEGER NOT NULL, hidden INTEGER NOT NULL, search INTEGER NOT NULL,
	row BLOB NOT NULL, UNIQUE (kind, id));
CREATE INDEX rows_unsearchable ON rows (kind) WHERE search = 0;
CREATE TABLE terms (term TEXT NOT NULL, n INTEGER NOT NULL, PRIMARY KEY (term, n)) WITHOUT ROWID;
CREATE INDEX terms_n ON terms (n);
CREATE TABLE fields (kind TEXT NOT NULL, field TEXT NOT NULL, col INTEGER NOT NULL, PRIMARY KEY (kind, field));
CREATE VIRTUAL TABLE texts USING fts5 (t0, t1, t2, t3, tokenize = 'unicode61');
CREATE TABLE broken (kind TEXT NOT NULL, id TEXT NOT NULL, version TEXT NOT NULL, error TEXT NOT NULL, PRIMARY KEY (kind, id));
`

// cols is how many text fields a kind may have: the texts table's columns.
const cols = 4

// View is a board's view in one sqlite file, and its rows files.
type View struct {
	db    *sql.DB // its writes take the lock at once (BEGIN IMMEDIATE)
	ro    *sql.DB // reads in one snapshot
	files *rowFiles
}

var _ kernel.View = (*View)(nil)

// Open opens the view at path, building its schema when the file is
// missing, unreadable or of another version.
func Open(ctx context.Context, path string) (*View, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	v, err := open(ctx, path)
	if err == nil {
		return v, nil
	}
	for _, f := range []string{path, path + "-wal", path + "-shm"} {
		if err := os.Remove(f); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	return open(ctx, path)
}

func open(ctx context.Context, path string) (*View, error) {
	dsn := "file:" + path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)"
	db, err := sql.Open("sqlite", dsn+"&_txlock=immediate")
	if err != nil {
		return nil, err
	}
	ro, err := sql.Open("sqlite", dsn+"&_txlock=deferred")
	if err != nil {
		db.Close()
		return nil, err
	}
	v := &View{db: db, ro: ro, files: newRowFiles(filepath.Dir(path))}
	var have int
	if err := db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&have); err != nil {
		v.Close()
		return nil, err
	}
	switch have {
	case version:
		return v, nil
	case 0: // a new file, or one another process is making
		tx, err := db.BeginTx(ctx, nil)
		if err != nil {
			v.Close()
			return nil, err
		}
		defer tx.Rollback()
		if err := tx.QueryRowContext(ctx, "PRAGMA user_version").Scan(&have); err != nil {
			v.Close()
			return nil, err
		}
		if have == 0 {
			if _, err := tx.ExecContext(ctx, schema+fmt.Sprintf("PRAGMA user_version = %d;", version)); err != nil {
				db.Close()
				return nil, err
			}
		}
		if err := tx.Commit(); err != nil {
			v.Close()
			return nil, err
		}
		return v, nil
	}
	v.Close()
	return nil, fmt.Errorf("view: version %d, not %d", have, version)
}

// Close closes the file.
func (v *View) Close() error { return errors.Join(v.db.Close(), v.ro.Close()) }

// Mark is what the view holds of a kind; a kind it never held has rev 0.
func (v *View) Mark(ctx context.Context, kind string) (kernel.Mark, error) {
	return mark(ctx, v.db, kind)
}

type querier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func mark(ctx context.Context, q querier, kind string) (kernel.Mark, error) {
	var m kernel.Mark
	var search int
	var sum []byte
	err := q.QueryRowContext(ctx, "SELECT rev, sum, search FROM marks WHERE kind = ?", kind).Scan(&m.Rev, &sum, &search)
	if errors.Is(err, sql.ErrNoRows) {
		return kernel.Mark{}, nil
	}
	copy(m.Sum[:], sum)
	m.Search = search == 1
	return m, err
}

// Heads is the listing a kind's rows are at.
func (v *View) Heads(ctx context.Context, kind string) ([]kernel.Head, error) {
	return heads(ctx, v.db, kind)
}

// heads are a kind's rows' and errors' versions, by id: every entity the
// view has is one or the other.
func heads(ctx context.Context, q querier, kind string) ([]kernel.Head, error) {
	rows, err := q.QueryContext(ctx, "SELECT id, version FROM rows WHERE kind = ?1 UNION ALL SELECT id, version FROM broken WHERE kind = ?1 ORDER BY 1", kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []kernel.Head
	for rows.Next() {
		var h kernel.Head
		if err := rows.Scan(&h.ID, &h.Version); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// Write changes a kind's rows in one transaction, each where its stored
// version is still its base. A reset starts the kind over only while its
// rows are of another rev: two processes building a new view at once,
// the second leaves the first's rows, which may be newer than its own. A
// write folded by another rev than the view's is a stale process's: it is
// left out, and the next refresh catches up.
func (v *View) Write(ctx context.Context, kind string, w kernel.Write) (err error) {
	tx, err := v.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			tx.Rollback()
		}
	}()
	m, err := mark(ctx, tx, kind)
	if err != nil {
		return err
	}
	emptied := false               // the kind has no rows: a lookup finds nothing
	if w.Reset && m.Rev != w.Rev { // another process may have started over meanwhile: then its rows stand
		for _, q := range []string{
			"DELETE FROM terms WHERE n IN (SELECT n FROM rows WHERE kind = ?1)",
			"DELETE FROM texts WHERE rowid IN (SELECT n FROM rows WHERE kind = ?1)",
			"DELETE FROM rows WHERE kind = ?1",
			"DELETE FROM broken WHERE kind = ?1",
			"DELETE FROM fields WHERE kind = ?1",
		} {
			if _, err := tx.ExecContext(ctx, q, kind); err != nil {
				return err
			}
		}
		m, emptied = kernel.Mark{Rev: w.Rev}, true
	} else if m.Rev != w.Rev {
		return tx.Rollback()
	}
	st, err := prepare(ctx, tx, kind)
	if err != nil {
		return err
	}
	defer st.close()
	// the sum moves by each head changed (kernel.Sum): a write reads the
	// heads it changes, not the kind's 20,000
	sum := m.Sum
	over := map[kernel.ID]*kernel.Version{} // this write's heads, nil: dropped
	get := func(id kernel.ID) (kernel.Version, bool, error) {
		if v, ok := over[id]; ok {
			if v == nil {
				return "", false, nil
			}
			return *v, true, nil
		}
		if emptied {
			return "", false, nil
		}
		return st.version(ctx, id)
	}
	changed := map[kernel.ID]*row{} // what the rows file takes: nil a row gone
	for _, p := range w.Put {
		cur, there, err := get(p.ID)
		if err != nil {
			return err
		}
		if cur != p.Base || there == (p.Base == "") {
			continue // another process moved it meanwhile
		}
		r, err := st.put(ctx, p, there)
		if err != nil {
			return err
		}
		changed[p.ID] = r
		if there {
			sum = kernel.SubSum(sum, kernel.HeadSum(kernel.Head{ID: p.ID, Version: cur}))
		}
		sum = kernel.AddSum(sum, kernel.HeadSum(kernel.Head{ID: p.ID, Version: p.Version}))
		over[p.ID] = &p.Version
	}
	for _, d := range w.Drop {
		cur, there, err := get(d.ID)
		if err != nil {
			return err
		}
		if !there || cur != d.Version {
			continue
		}
		if err := st.remove(ctx, d.ID); err != nil {
			return err
		}
		changed[d.ID] = nil
		sum = kernel.SubSum(sum, kernel.HeadSum(kernel.Head{ID: d.ID, Version: cur}))
		over[d.ID] = nil
	}
	search := 0
	if m.Search || w.Search {
		search = 1
	}
	next := kernel.Mark{Rev: m.Rev, Sum: sum, Search: search == 1}
	if _, err := tx.ExecContext(ctx, "INSERT OR REPLACE INTO marks (kind, rev, sum, search) VALUES (?, ?, ?, ?)",
		kind, next.Rev, next.Sum[:], search); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if !w.Reset {
		v.files.patchRows(kind, m, next, changed)
	}
	return nil
}

// stmts are one write's statements, each prepared once: a first build
// runs each thousands of times.
type stmts struct {
	kind                                             string
	tx                                               *sql.Tx
	insertRow, updateRow, deleteRow, find, head      *sql.Stmt
	term, dropTerms, text, dropText, broken, unbreak *sql.Stmt
	fields                                           map[string]int
}

func prepare(ctx context.Context, tx *sql.Tx, kind string) (*stmts, error) {
	st := &stmts{kind: kind, tx: tx, fields: map[string]int{}}
	var err error
	prep := func(q string) *sql.Stmt {
		if err != nil {
			return nil
		}
		var s *sql.Stmt
		s, err = tx.PrepareContext(ctx, q)
		return s
	}
	st.insertRow = prep(`INSERT INTO rows (kind, id, version, created, updated, hidden, search, row) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	st.updateRow = prep(`UPDATE rows SET version = ?, created = ?, updated = ?, hidden = ?, search = ?, row = ? WHERE n = ?`)
	st.deleteRow = prep(`DELETE FROM rows WHERE n = ?`)
	st.find = prep(`SELECT n, search FROM rows WHERE kind = ? AND id = ?`)
	st.head = prep(`SELECT version FROM rows WHERE kind = ?1 AND id = ?2 UNION ALL SELECT version FROM broken WHERE kind = ?1 AND id = ?2`)
	st.term = prep(`INSERT OR IGNORE INTO terms (term, n) VALUES (?, ?)`)
	st.dropTerms = prep(`DELETE FROM terms WHERE n = ?`)
	st.text = prep(`INSERT INTO texts (rowid, t0, t1, t2, t3) VALUES (?, ?, ?, ?, ?)`)
	st.dropText = prep(`DELETE FROM texts WHERE rowid = ?`)
	st.broken = prep(`INSERT INTO broken (kind, id, version, error) VALUES (?, ?, ?, ?)`)
	st.unbreak = prep(`DELETE FROM broken WHERE kind = ? AND id = ?`)
	if err != nil {
		st.close()
		return nil, err
	}
	rows, err := tx.QueryContext(ctx, "SELECT field, col FROM fields WHERE kind = ?", kind)
	if err != nil {
		st.close()
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var f string
		var c int
		if err := rows.Scan(&f, &c); err != nil {
			st.close()
			return nil, err
		}
		st.fields[f] = c
	}
	return st, rows.Err()
}

func (st *stmts) close() {
	for _, s := range []*sql.Stmt{st.insertRow, st.updateRow, st.deleteRow, st.find, st.head, st.term, st.dropTerms, st.text, st.dropText, st.broken, st.unbreak} {
		if s != nil {
			s.Close()
		}
	}
}

// existing is an entity's row: its number, and whether it has terms and
// texts. 0 for none.
func (st *stmts) existing(ctx context.Context, id kernel.ID) (int64, bool, error) {
	var n int64
	var search bool
	err := st.find.QueryRowContext(ctx, st.kind, id).Scan(&n, &search)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return n, search, err
}

// version is an entity's head in the view: its row's version, or its
// error's.
func (st *stmts) version(ctx context.Context, id kernel.ID) (kernel.Version, bool, error) {
	var v kernel.Version
	err := st.head.QueryRowContext(ctx, st.kind, id).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	return v, err == nil, err
}

// unsearch takes a row's terms and texts out.
func (st *stmts) unsearch(ctx context.Context, n int64) error {
	if _, err := st.dropTerms.ExecContext(ctx, n); err != nil {
		return err
	}
	_, err := st.dropText.ExecContext(ctx, n)
	return err
}

// remove takes an entity's row, its terms, texts and error out.
func (st *stmts) remove(ctx context.Context, id kernel.ID) error {
	if _, err := st.unbreak.ExecContext(ctx, st.kind, id); err != nil {
		return err
	}
	n, search, err := st.existing(ctx, id)
	if err != nil || n == 0 {
		return err
	}
	if search {
		if err := st.unsearch(ctx, n); err != nil {
			return err
		}
	}
	_, err = st.deleteRow.ExecContext(ctx, n)
	return err
}

// put writes an entity's row, in place of the one there (there), or its
// error; the row as the rows file takes it, nil for an error.
func (st *stmts) put(ctx context.Context, p kernel.Put, there bool) (*row, error) {
	var n int64
	var search bool
	if there {
		if _, err := st.unbreak.ExecContext(ctx, st.kind, p.ID); err != nil {
			return nil, err
		}
		var err error
		if n, search, err = st.existing(ctx, p.ID); err != nil {
			return nil, err
		}
	}
	if p.Err != "" {
		if n != 0 {
			if err := st.remove(ctx, p.ID); err != nil {
				return nil, err
			}
		}
		_, err := st.broken.ExecContext(ctx, st.kind, p.ID, p.Version, p.Err)
		return nil, err
	}
	d := p.Doc
	if n != 0 {
		if search {
			if err := st.unsearch(ctx, n); err != nil {
				return nil, err
			}
		}
		if _, err := st.updateRow.ExecContext(ctx, p.Version, d.Created, d.Updated, d.Hidden, p.Search, d.Row, n); err != nil {
			return nil, err
		}
	} else {
		res, err := st.insertRow.ExecContext(ctx, st.kind, p.ID, p.Version, d.Created, d.Updated, d.Hidden, p.Search, d.Row)
		if err != nil {
			return nil, err
		}
		if n, err = res.LastInsertId(); err != nil {
			return nil, err
		}
	}
	r := &row{id: p.ID, version: p.Version, created: d.Created, updated: d.Updated, hidden: d.Hidden, row: d.Row}
	if !p.Search {
		return r, nil
	}
	return r, st.search(ctx, n, d)
}

// search writes a row's terms and texts.
func (st *stmts) search(ctx context.Context, n int64, d kernel.Indexed) error {
	for _, t := range d.Terms {
		if _, err := st.term.ExecContext(ctx, t, n); err != nil {
			return err
		}
	}
	if len(d.Texts) == 0 {
		return nil
	}
	var texts [cols]string
	for _, t := range d.Texts {
		c, ok := st.fields[t.Field]
		if !ok {
			if c = len(st.fields); c >= cols {
				return fmt.Errorf("view: %s has more than %d text fields", st.kind, cols)
			}
			if _, err := st.tx.ExecContext(ctx, "INSERT INTO fields (kind, field, col) VALUES (?, ?, ?)", st.kind, t.Field, c); err != nil {
				return err
			}
			st.fields[t.Field] = c
		}
		texts[c] = t.Body
	}
	_, err := st.text.ExecContext(ctx, n, texts[0], texts[1], texts[2], texts[3])
	return err
}

// Rows answers the visible rows matching q: the kind's rows read whole,
// narrowed by the ids the terms and the text match.
func (v *View) Rows(ctx context.Context, kind string, q kernel.Query) ([]kernel.Row, error) {
	switch q.Sort {
	case "", "created", "updated":
	default:
		return nil, fmt.Errorf("view: sort by created or updated, not %q", q.Sort)
	}
	keep, err := v.matching(ctx, kind, q)
	if err != nil {
		return nil, err
	}
	m, err := mark(ctx, v.db, kind)
	if err != nil {
		return nil, err
	}
	all, err := v.files.all(ctx, kind, m, v.readRows)
	if err != nil {
		return nil, err
	}
	out := make([]kernel.Row, 0, len(all))
	for _, r := range sorted(all, q) {
		if r.hidden || keep != nil && !keep[r.id] {
			continue
		}
		out = append(out, kernel.Row{ID: r.id, Version: r.version, Row: r.row})
		if q.Limit > 0 && len(out) == q.Limit {
			break
		}
	}
	return out, nil
}

// matching are the ids q's terms and text match; nil when q has none.
func (v *View) matching(ctx context.Context, kind string, q kernel.Query) (map[kernel.ID]bool, error) {
	where, args := []string{"r.kind = ?"}, []any{kind}
	for _, t := range q.All {
		where, args = append(where, "r.n IN (SELECT n FROM terms WHERE term = ?)"), append(args, t)
	}
	for _, group := range q.Any {
		if len(group) == 0 {
			continue
		}
		where = append(where, "r.n IN (SELECT n FROM terms WHERE term IN (?"+strings.Repeat(", ?", len(group)-1)+"))")
		for _, t := range group {
			args = append(args, t)
		}
	}
	if len(q.Not) > 0 {
		where = append(where, "r.n NOT IN (SELECT n FROM terms WHERE term IN (?"+strings.Repeat(", ?", len(q.Not)-1)+"))")
		for _, t := range q.Not {
			args = append(args, t)
		}
	}
	if q.Text != "" {
		m, ok, err := v.match(ctx, kind, q.Text, q.In)
		if err != nil {
			return nil, err
		}
		if !ok {
			return map[kernel.ID]bool{}, nil // looked for nowhere
		}
		if m != "" {
			where, args = append(where, "r.n IN (SELECT rowid FROM texts WHERE texts MATCH ?)"), append(args, m)
		}
	}
	if len(where) == 1 {
		return nil, nil
	}
	rows, err := v.db.QueryContext(ctx, "SELECT r.id FROM rows r WHERE "+strings.Join(where, " AND "), args...)
	if err != nil {
		return nil, fmt.Errorf("view: %w", err)
	}
	defer rows.Close()
	keep := map[kernel.ID]bool{}
	for rows.Next() {
		var id kernel.ID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		keep[id] = true
	}
	return keep, rows.Err()
}

// match is text as an FTS5 query (queries.md, Text): each word quoted,
// so FTS5's own syntax is never read, as a prefix, in the columns of the
// fields in. "" matches every row: no word has a token. false: in names
// no field this kind has text in, so nothing matches.
func (v *View) match(ctx context.Context, kind, text string, in []string) (string, bool, error) {
	filter := ""
	if len(in) > 0 {
		var names []string
		for _, f := range in {
			var c int
			err := v.db.QueryRowContext(ctx, "SELECT col FROM fields WHERE kind = ? AND field = ?", kind, f).Scan(&c)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return "", false, err
			}
			names = append(names, fmt.Sprintf("t%d", c))
		}
		if len(names) == 0 {
			return "", false, nil
		}
		filter = "{" + strings.Join(names, " ") + "} : "
	}
	var words []string
	for _, w := range strings.Fields(text) {
		if strings.IndexFunc(w, search.IsToken) >= 0 {
			words = append(words, filter+`"`+strings.ReplaceAll(w, `"`, `""`)+`"*`)
		}
	}
	return strings.Join(words, " "), true, nil
}

// Resolve answers up to two visible ids starting with prefix.
func (v *View) Resolve(ctx context.Context, kind, prefix string) ([]kernel.ID, error) {
	rows, err := v.db.QueryContext(ctx, "SELECT id FROM rows WHERE kind = ? AND hidden = 0 AND id >= ? AND id < ? ORDER BY id LIMIT 2",
		kind, prefix, prefix+"g")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []kernel.ID
	for rows.Next() {
		var id kernel.ID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// Unsearchable are the rows whose terms and texts aren't kept yet,
// hidden ones too.
func (v *View) Unsearchable(ctx context.Context, kind string) ([]kernel.Head, error) {
	rows, err := v.db.QueryContext(ctx, "SELECT id, version FROM rows WHERE kind = ? AND search = 0", kind)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []kernel.Head
	for rows.Next() {
		var h kernel.Head
		if err := rows.Scan(&h.ID, &h.Version); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// Broken are the entities that failed to load, by kind then id.
func (v *View) Broken(ctx context.Context) ([]kernel.Broken, error) {
	rows, err := v.db.QueryContext(ctx, "SELECT kind, id, error FROM broken ORDER BY kind, id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []kernel.Broken
	for rows.Next() {
		var b kernel.Broken
		var msg string
		if err := rows.Scan(&b.Kind, &b.ID, &msg); err != nil {
			return nil, err
		}
		b.Err = errors.New(msg)
		out = append(out, b)
	}
	return out, rows.Err()
}
