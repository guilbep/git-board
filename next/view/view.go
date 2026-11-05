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
	"slices"
	"strings"

	_ "modernc.org/sqlite" // database/sql's "sqlite", FTS5 included

	"github.com/guilbep/git-board/next/kernel"
	"github.com/guilbep/git-board/next/kernel/search"
)

// version is the schema's: a file of another is built again.
const version = 1

const schema = `
CREATE TABLE marks (kind TEXT PRIMARY KEY, rev INTEGER NOT NULL, sum BLOB NOT NULL, heads BLOB NOT NULL, texts INTEGER NOT NULL);
CREATE TABLE rows (n INTEGER PRIMARY KEY, kind TEXT NOT NULL, id TEXT NOT NULL, version TEXT NOT NULL,
	created INTEGER NOT NULL, updated INTEGER NOT NULL, hidden INTEGER NOT NULL, texted INTEGER NOT NULL,
	row BLOB NOT NULL, UNIQUE (kind, id));
CREATE INDEX rows_created ON rows (kind, created, id);
CREATE INDEX rows_updated ON rows (kind, updated, id);
CREATE INDEX rows_texted ON rows (kind, texted);
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
	dir   string
	cache rowsCache
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
	v := &View{db: db, ro: ro, dir: filepath.Dir(path), cache: rowsCache{at: map[string]kernel.Mark{}, rows: map[string][]row{}}}
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
}

func mark(ctx context.Context, q querier, kind string) (kernel.Mark, error) {
	var m kernel.Mark
	var texts int
	var sum []byte
	err := q.QueryRowContext(ctx, "SELECT rev, sum, texts FROM marks WHERE kind = ?", kind).Scan(&m.Rev, &sum, &texts)
	if errors.Is(err, sql.ErrNoRows) {
		return kernel.Mark{}, nil
	}
	copy(m.Sum[:], sum)
	m.Texts = texts == 1
	return m, err
}

// Heads is the listing a kind's rows are at.
func (v *View) Heads(ctx context.Context, kind string) ([]kernel.Head, error) {
	return heads(ctx, v.db, kind)
}

func heads(ctx context.Context, q querier, kind string) ([]kernel.Head, error) {
	var b []byte
	err := q.QueryRowContext(ctx, "SELECT heads FROM marks WHERE kind = ?", kind).Scan(&b)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return kernel.ParseListing(b), err
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
	var base []kernel.Head
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
		m = kernel.Mark{Rev: w.Rev}
	} else if m.Rev != w.Rev {
		return tx.Rollback()
	} else if base, err = heads(ctx, tx, kind); err != nil {
		return err
	}
	st, err := prepare(ctx, tx, kind)
	if err != nil {
		return err
	}
	defer st.close()
	at := make(map[kernel.ID]kernel.Version, len(base))
	for _, h := range base {
		at[h.ID] = h.Version
	}
	for _, p := range w.Put {
		cur, there := at[p.ID]
		if cur != p.Base || there == (p.Base == "") {
			continue // another process moved it meanwhile
		}
		if there {
			if err := st.remove(ctx, p.ID); err != nil {
				return err
			}
		}
		if p.Err != "" {
			_, err = st.broken.ExecContext(ctx, kind, p.ID, p.Version, p.Err)
		} else {
			err = st.insert(ctx, p)
		}
		if err != nil {
			return err
		}
		at[p.ID] = p.Version
	}
	for _, d := range w.Drop {
		if cur, there := at[d.ID]; !there || cur != d.Version {
			continue
		}
		if err := st.remove(ctx, d.ID); err != nil {
			return err
		}
		delete(at, d.ID)
	}
	list := make([]kernel.Head, 0, len(at))
	for id, ver := range at {
		list = append(list, kernel.Head{ID: id, Version: ver})
	}
	slices.SortFunc(list, func(a, b kernel.Head) int { return strings.Compare(string(a.ID), string(b.ID)) })
	texts := 0
	if m.Texts || w.Texts {
		texts = 1
	}
	sum := kernel.Sum(list)
	if _, err := tx.ExecContext(ctx, "INSERT OR REPLACE INTO marks (kind, rev, sum, heads, texts) VALUES (?, ?, ?, ?, ?)",
		kind, m.Rev, sum[:], kernel.Listing(list), texts); err != nil {
		return err
	}
	return tx.Commit()
}

// stmts are one write's statements, each prepared once: a first build
// runs each thousands of times.
type stmts struct {
	kind                                   string
	tx                                     *sql.Tx
	row, term, text, broken, find, unbreak *sql.Stmt
	drop                                   []*sql.Stmt
	fields                                 map[string]int
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
	st.row = prep(`INSERT INTO rows (kind, id, version, created, updated, hidden, texted, row) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`)
	st.term = prep(`INSERT OR IGNORE INTO terms (term, n) VALUES (?, ?)`)
	st.text = prep(`INSERT INTO texts (rowid, t0, t1, t2, t3) VALUES (?, ?, ?, ?, ?)`)
	st.broken = prep(`INSERT INTO broken (kind, id, version, error) VALUES (?, ?, ?, ?)`)
	st.find = prep(`SELECT n FROM rows WHERE kind = ? AND id = ?`)
	st.unbreak = prep(`DELETE FROM broken WHERE kind = ? AND id = ?`)
	for _, q := range []string{"DELETE FROM terms WHERE n = ?", "DELETE FROM texts WHERE rowid = ?", "DELETE FROM rows WHERE n = ?"} {
		st.drop = append(st.drop, prep(q))
	}
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
	for _, s := range append([]*sql.Stmt{st.row, st.term, st.text, st.broken, st.find, st.unbreak}, st.drop...) {
		if s != nil {
			s.Close()
		}
	}
}

// remove takes an entity's row, terms, texts and error out.
func (st *stmts) remove(ctx context.Context, id kernel.ID) error {
	if _, err := st.unbreak.ExecContext(ctx, st.kind, id); err != nil {
		return err
	}
	var n int64
	err := st.find.QueryRowContext(ctx, st.kind, id).Scan(&n)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, s := range st.drop {
		if _, err := s.ExecContext(ctx, n); err != nil {
			return err
		}
	}
	return nil
}

func (st *stmts) insert(ctx context.Context, p kernel.Put) error {
	d := p.Doc
	res, err := st.row.ExecContext(ctx, st.kind, p.ID, p.Version, d.Created, d.Updated, d.Hidden, p.Texted, d.Row)
	if err != nil {
		return err
	}
	n, err := res.LastInsertId()
	if err != nil {
		return err
	}
	for _, t := range d.Terms {
		if _, err := st.term.ExecContext(ctx, t, n); err != nil {
			return err
		}
	}
	if !p.Texted || len(d.Texts) == 0 {
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
	_, err = st.text.ExecContext(ctx, n, texts[0], texts[1], texts[2], texts[3])
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
	all, err := v.all(ctx, kind)
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

// Untexted are the rows whose texts aren't kept yet, hidden ones too.
func (v *View) Untexted(ctx context.Context, kind string) ([]kernel.Head, error) {
	rows, err := v.db.QueryContext(ctx, "SELECT id, version FROM rows WHERE kind = ? AND texted = 0", kind)
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
